package api

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"
	"github.com/VKCOM/php-parser/pkg/conf"
	"github.com/VKCOM/php-parser/pkg/parser"
	"github.com/VKCOM/php-parser/pkg/version"
	"github.com/VKCOM/php-parser/pkg/visitor/nsresolver"
	"github.com/VKCOM/php-parser/pkg/visitor/traverser"
)

// PHPConst is the value of a constant of a PocketMine-MP class.
type PHPConst struct {
	// Kind is "int", "float", "string" or "bool".
	Kind  string `json:"k"`
	Value string `json:"v"`
}

type rawConst struct {
	class string
	expr  ast.Vertex
	names map[ast.Vertex]string
}

// GeneratePHPConsts reads the class constants with literal values from PocketMine-MP's source
// (its src folder), for constants pocketmine-go doesn't have.
func GeneratePHPConsts(src string) (map[string]PHPConst, error) {
	raw := map[string]*rawConst{}
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".php") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		root, err := parser.Parse(data, conf.Config{Version: &version.Version{Major: 8, Minor: 1}})
		if err != nil || root == nil {
			return nil
		}
		nsr := nsresolver.NewNamespaceResolver()
		traverser.NewTraverser(nsr).Traverse(root)
		var walk func(stmts []ast.Vertex)
		walk = func(stmts []ast.Vertex) {
			for _, s := range stmts {
				var class string
				var body []ast.Vertex
				switch x := s.(type) {
				case *ast.StmtNamespace:
					walk(x.Stmts)
					continue
				case *ast.StmtClass:
					class, body = nsr.ResolvedNames[x], x.Stmts
				case *ast.StmtInterface:
					class, body = nsr.ResolvedNames[x], x.Stmts
				case *ast.StmtEnum:
					class, body = nsr.ResolvedNames[x], x.Stmts
				default:
					continue
				}
				for _, m := range body {
					cl, ok := m.(*ast.StmtClassConstList)
					if !ok {
						continue
					}
					for _, c := range cl.Consts {
						k, ok := c.(*ast.StmtConstant)
						if !ok {
							continue
						}
						name := string(k.Name.(*ast.Identifier).Value)
						raw[strings.ToLower(class+"::"+name)] = &rawConst{class: class, expr: k.Expr, names: nsr.ResolvedNames}
					}
				}
			}
		}
		if r, ok := root.(*ast.Root); ok {
			walk(r.Stmts)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[string]PHPConst{}
	var eval func(key string, depth int) (PHPConst, bool)
	var evalExpr func(rc *rawConst, n ast.Vertex, depth int) (PHPConst, bool)
	eval = func(key string, depth int) (PHPConst, bool) {
		if v, ok := out[key]; ok {
			return v, true
		}
		rc, ok := raw[key]
		if !ok || depth > 10 {
			return PHPConst{}, false
		}
		v, ok := evalExpr(rc, rc.expr, depth+1)
		if ok {
			out[key] = v
		}
		return v, ok
	}
	evalExpr = func(rc *rawConst, n ast.Vertex, depth int) (PHPConst, bool) {
		switch x := n.(type) {
		case *ast.ScalarLnumber:
			s := strings.ReplaceAll(string(x.Value), "_", "")
			v, err := strconv.ParseInt(s, 0, 64)
			if err != nil {
				return PHPConst{}, false
			}
			return PHPConst{"int", strconv.FormatInt(v, 10)}, true
		case *ast.ScalarDnumber:
			v, err := strconv.ParseFloat(strings.ReplaceAll(string(x.Value), "_", ""), 64)
			if err != nil {
				return PHPConst{}, false
			}
			return PHPConst{"float", strconv.FormatFloat(v, 'g', -1, 64)}, true
		case *ast.ScalarString:
			s := string(x.Value)
			if len(s) < 2 {
				return PHPConst{}, false
			}
			body := s[1 : len(s)-1]
			if s[0] == '"' && strings.ContainsAny(body, `\$`) {
				return PHPConst{}, false
			}
			if s[0] == '\'' {
				body = strings.NewReplacer(`\\`, `\`, `\'`, `'`).Replace(body)
			}
			return PHPConst{"string", body}, true
		case *ast.ExprConstFetch:
			switch strings.ToLower(string(x.Const.(*ast.Name).Parts[len(x.Const.(*ast.Name).Parts)-1].(*ast.NamePart).Value)) {
			case "true":
				return PHPConst{"bool", "true"}, true
			case "false":
				return PHPConst{"bool", "false"}, true
			}
		case *ast.ExprUnaryMinus:
			v, ok := evalExpr(rc, x.Expr, depth)
			if !ok || v.Kind != "int" && v.Kind != "float" {
				return PHPConst{}, false
			}
			if strings.HasPrefix(v.Value, "-") {
				v.Value = v.Value[1:]
			} else {
				v.Value = "-" + v.Value
			}
			return v, true
		case *ast.ExprBrackets:
			return evalExpr(rc, x.Expr, depth)
		case *ast.ExprClassConstFetch:
			cls := rc.names[x.Class]
			if cls == "" {
				if id, ok := x.Class.(*ast.Name); ok && len(id.Parts) == 1 {
					switch strings.ToLower(string(id.Parts[0].(*ast.NamePart).Value)) {
					case "self", "static":
						cls = rc.class
					}
				}
			}
			id, ok := x.Const.(*ast.Identifier)
			if !ok || cls == "" {
				return PHPConst{}, false
			}
			return eval(strings.ToLower(strings.TrimPrefix(cls, `\`)+"::"+string(id.Value)), depth)
		case *ast.ExprBinaryConcat:
			l, ok1 := evalExpr(rc, x.Left, depth)
			r, ok2 := evalExpr(rc, x.Right, depth)
			if !ok1 || !ok2 {
				return PHPConst{}, false
			}
			return PHPConst{"string", l.Value + r.Value}, true
		case *ast.ExprBinaryShiftLeft, *ast.ExprBinaryShiftRight, *ast.ExprBinaryBitwiseOr, *ast.ExprBinaryBitwiseAnd,
			*ast.ExprBinaryPlus, *ast.ExprBinaryMinus, *ast.ExprBinaryMul:
			var ln, rn ast.Vertex
			switch y := x.(type) {
			case *ast.ExprBinaryShiftLeft:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryShiftRight:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryBitwiseOr:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryBitwiseAnd:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryPlus:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryMinus:
				ln, rn = y.Left, y.Right
			case *ast.ExprBinaryMul:
				ln, rn = y.Left, y.Right
			}
			l, ok1 := evalExpr(rc, ln, depth)
			r, ok2 := evalExpr(rc, rn, depth)
			if !ok1 || !ok2 || l.Kind != "int" || r.Kind != "int" {
				return PHPConst{}, false
			}
			a, _ := strconv.ParseInt(l.Value, 10, 64)
			b, _ := strconv.ParseInt(r.Value, 10, 64)
			var v int64
			switch x.(type) {
			case *ast.ExprBinaryShiftLeft:
				v = a << uint(b)
			case *ast.ExprBinaryShiftRight:
				v = a >> uint(b)
			case *ast.ExprBinaryBitwiseOr:
				v = a | b
			case *ast.ExprBinaryBitwiseAnd:
				v = a & b
			case *ast.ExprBinaryPlus:
				v = a + b
			case *ast.ExprBinaryMinus:
				v = a - b
			case *ast.ExprBinaryMul:
				v = a * b
			}
			return PHPConst{"int", strconv.FormatInt(v, 10)}, true
		}
		return PHPConst{}, false
	}
	for key := range raw {
		eval(key, 0)
	}
	return out, nil
}
