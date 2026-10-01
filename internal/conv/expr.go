package conv

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// expr converts an expression. want is the type the context expects (or nil); it guides
// literals and closures, but the result must still be coerced by the caller.
func (f *fctx) expr(n ast.Vertex, want *api.Type) value {
	switch x := n.(type) {
	case nil:
		return prim("nil", api.UntypedNil)
	case *ast.ExprBrackets:
		v := f.expr(x.Expr, want)
		return v
	case *ast.ScalarLnumber:
		return f.intLit(string(x.Value))
	case *ast.ScalarDnumber:
		s := strings.ReplaceAll(string(x.Value), "_", "")
		if fv, err := strconv.ParseFloat(s, 64); err == nil {
			s = strconv.FormatFloat(fv, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
		}
		return value{code: s, t: api.Float, prec: 7, konst: true}
	case *ast.ScalarString:
		return prim(quote(unquotePHP(x)), api.String)
	case *ast.ScalarEncapsed:
		return f.encapsed(x.Parts)
	case *ast.ScalarHeredoc:
		return f.heredoc(x)
	case *ast.ScalarMagicConstant:
		return f.magic(string(x.Value), n)
	case *ast.ExprConstFetch:
		return f.constFetch(x)
	case *ast.ExprVariable:
		return f.variable(x)
	case *ast.ExprArray:
		return f.arrayLit(x.Items, want)
	case *ast.ExprList:
		return f.arrayLit(x.Items, want)
	case *ast.ExprArrayDimFetch:
		return f.dimFetch(x)
	case *ast.ExprPropertyFetch:
		return f.propFetch(x.Var, x.Prop, false)
	case *ast.ExprNullsafePropertyFetch:
		return f.propFetch(x.Var, x.Prop, true)
	case *ast.ExprStaticPropertyFetch:
		return f.staticPropFetch(x)
	case *ast.ExprClassConstFetch:
		return f.classConst(x)
	case *ast.ExprMethodCall:
		return f.methodCall(x.Var, x.Method, x.Args, false, n)
	case *ast.ExprNullsafeMethodCall:
		return f.methodCall(x.Var, x.Method, x.Args, true, n)
	case *ast.ExprStaticCall:
		return f.staticCall(x)
	case *ast.ExprFunctionCall:
		return f.funcCall(x, want)
	case *ast.ExprNew:
		return f.newExpr(x)
	case *ast.ExprClosure:
		return f.closure(x.Params, x.Uses, x.ReturnType, x.Stmts, nil, want, n)
	case *ast.ExprArrowFunction:
		return f.closure(x.Params, nil, x.ReturnType, nil, x.Expr, want, n)
	case *ast.ExprAssign:
		return f.assignExpr(x.Var, x.Expr, n)
	case *ast.ExprAssignReference:
		return f.assignExpr(x.Var, x.Expr, n)
	case *ast.ExprAssignCoalesce, *ast.ExprAssignConcat, *ast.ExprAssignPlus, *ast.ExprAssignMinus,
		*ast.ExprAssignMul, *ast.ExprAssignDiv, *ast.ExprAssignMod, *ast.ExprAssignPow,
		*ast.ExprAssignBitwiseAnd, *ast.ExprAssignBitwiseOr, *ast.ExprAssignBitwiseXor,
		*ast.ExprAssignShiftLeft, *ast.ExprAssignShiftRight:
		return f.compoundAssignExpr(n)
	case *ast.ExprPreInc:
		return f.incDec(x.Var, "PreInc", "++", n)
	case *ast.ExprPostInc:
		return f.incDec(x.Var, "PostInc", "++", n)
	case *ast.ExprPreDec:
		return f.incDec(x.Var, "PreDec", "--", n)
	case *ast.ExprPostDec:
		return f.incDec(x.Var, "PostDec", "--", n)
	case *ast.ExprBinaryConcat:
		l, r := f.expr(x.Left, api.String), f.expr(x.Right, api.String)
		return value{code: paren(value{code: f.str(l), prec: strPrec(l)}, 4) + " + " + paren(value{code: f.str(r), prec: strPrec(r)}, 5), t: api.String, prec: 4}
	case *ast.ExprBinaryPlus:
		return f.arith(x.Left, x.Right, "+", "Add")
	case *ast.ExprBinaryMinus:
		return f.arith(x.Left, x.Right, "-", "Sub")
	case *ast.ExprBinaryMul:
		return f.arith(x.Left, x.Right, "*", "Mul")
	case *ast.ExprBinaryDiv:
		return f.div(x.Left, x.Right)
	case *ast.ExprBinaryMod:
		l, r := f.expr(x.Left, api.Int), f.expr(x.Right, api.Int)
		if l.t.IsInt() && r.t.IsInt() {
			return value{code: paren(l, 5) + " % " + paren(r, 6), t: api.Int, prec: 5, konst: l.konst && r.konst}
		}
		return callv(f.phpx("Mod")+"("+l.code+", "+r.code+")", api.Int)
	case *ast.ExprBinaryPow:
		l, r := f.expr(x.Left, nil), f.expr(x.Right, nil)
		return callv(f.phpx("Pow")+"("+l.code+", "+r.code+")", api.Any)
	case *ast.ExprBinaryBitwiseAnd:
		return f.intOp(x.Left, x.Right, "&")
	case *ast.ExprBinaryBitwiseOr:
		return f.intOp(x.Left, x.Right, "|")
	case *ast.ExprBinaryBitwiseXor:
		return f.intOp(x.Left, x.Right, "^")
	case *ast.ExprBinaryShiftLeft:
		return f.intOp(x.Left, x.Right, "<<")
	case *ast.ExprBinaryShiftRight:
		return f.intOp(x.Left, x.Right, ">>")
	case *ast.ExprBinaryBooleanAnd:
		return value{code: paren(value{code: f.cond(f.expr(x.Left, api.Bool)), prec: condPrec(f, x.Left)}, 2) + " && " + paren(value{code: f.cond(f.expr(x.Right, api.Bool)), prec: condPrec(f, x.Right)}, 3), t: api.Bool, prec: 2}
	case *ast.ExprBinaryLogicalAnd:
		return value{code: paren(value{code: f.cond(f.expr(x.Left, api.Bool)), prec: condPrec(f, x.Left)}, 2) + " && " + paren(value{code: f.cond(f.expr(x.Right, api.Bool)), prec: condPrec(f, x.Right)}, 3), t: api.Bool, prec: 2}
	case *ast.ExprBinaryBooleanOr:
		return value{code: paren(value{code: f.cond(f.expr(x.Left, api.Bool)), prec: condPrec(f, x.Left)}, 1) + " || " + paren(value{code: f.cond(f.expr(x.Right, api.Bool)), prec: condPrec(f, x.Right)}, 2), t: api.Bool, prec: 1}
	case *ast.ExprBinaryLogicalOr:
		return value{code: paren(value{code: f.cond(f.expr(x.Left, api.Bool)), prec: condPrec(f, x.Left)}, 1) + " || " + paren(value{code: f.cond(f.expr(x.Right, api.Bool)), prec: condPrec(f, x.Right)}, 2), t: api.Bool, prec: 1}
	case *ast.ExprBinaryLogicalXor:
		return value{code: "(" + f.cond(f.expr(x.Left, api.Bool)) + ") != (" + f.cond(f.expr(x.Right, api.Bool)) + ")", t: api.Bool, prec: 3}
	case *ast.ExprBooleanNot:
		v := f.expr(x.Expr, api.Bool)
		c := value{code: f.cond(v), prec: condPrec(f, x.Expr)}
		return value{code: "!" + paren(c, 6), t: api.Bool, prec: 6}
	case *ast.ExprBinaryIdentical:
		return f.identical(x.Left, x.Right, false)
	case *ast.ExprBinaryNotIdentical:
		return f.identical(x.Left, x.Right, true)
	case *ast.ExprBinaryEqual:
		return f.equal(x.Left, x.Right, false)
	case *ast.ExprBinaryNotEqual:
		return f.equal(x.Left, x.Right, true)
	case *ast.ExprBinarySmaller:
		return f.compare(x.Left, x.Right, "<", "Less")
	case *ast.ExprBinarySmallerOrEqual:
		return f.compare(x.Left, x.Right, "<=", "LessEq")
	case *ast.ExprBinaryGreater:
		return f.compare(x.Left, x.Right, ">", "Greater")
	case *ast.ExprBinaryGreaterOrEqual:
		return f.compare(x.Left, x.Right, ">=", "GreaterEq")
	case *ast.ExprBinarySpaceship:
		l, r := f.expr(x.Left, nil), f.expr(x.Right, nil)
		return callv(f.phpx("Compare")+"("+l.code+", "+r.code+")", api.Int)
	case *ast.ExprBinaryCoalesce:
		return f.coalesce(x.Left, x.Right, want)
	case *ast.ExprTernary:
		return f.ternary(x, want)
	case *ast.ExprUnaryMinus:
		v := f.expr(x.Expr, want)
		if v.t.IsNumber() {
			return value{code: "-" + paren(v, 6), t: v.t, prec: 6, konst: v.konst}
		}
		return callv(f.phpx("Neg")+"("+v.code+")", api.Any)
	case *ast.ExprUnaryPlus:
		v := f.expr(x.Expr, want)
		if v.t.IsNumber() {
			return v
		}
		return callv(f.phpx("ToNumber")+"("+v.code+")", api.Any)
	case *ast.ExprBitwiseNot:
		v := f.expr(x.Expr, api.Int)
		return value{code: "^" + paren(value{code: f.coerce(v, api.Int), prec: 7}, 6), t: api.Int, prec: 6}
	case *ast.ExprCastInt:
		return f.cast(x.Expr, api.Int)
	case *ast.ExprCastDouble:
		return f.cast(x.Expr, api.Float)
	case *ast.ExprCastString:
		return f.cast(x.Expr, api.String)
	case *ast.ExprCastBool:
		return f.cast(x.Expr, api.Bool)
	case *ast.ExprCastArray:
		v := f.expr(x.Expr, nil)
		if isArrayT(v.t) {
			return v
		}
		return callv(f.phpx("ToArray")+"("+v.code+")", arrayT(nil, nil))
	case *ast.ExprCastObject:
		return f.expr(x.Expr, nil)
	case *ast.ExprCastUnset:
		return prim("nil", api.UntypedNil)
	case *ast.ExprErrorSuppress:
		return f.expr(x.Expr, want)
	case *ast.ExprClone:
		v := f.expr(x.Expr, nil)
		return callv(f.phpx("CloneObject")+"("+v.code+")", v.t)
	case *ast.ExprIsset:
		return f.isset(x.Vars)
	case *ast.ExprEmpty:
		v := f.expr(x.Expr, nil)
		return value{code: "!" + paren(value{code: f.cond(v), prec: condPrecV(f, v)}, 6), t: api.Bool, prec: 6}
	case *ast.ExprInstanceOf:
		return f.instanceOf(x)
	case *ast.ExprPrint:
		v := f.expr(x.Expr, nil)
		return callv(f.phpx("Echo")+"("+v.code+")", api.Void)
	case *ast.ExprExit:
		arg := ""
		if x.Expr != nil {
			arg = f.expr(x.Expr, nil).code
		}
		return callv(f.phpx("Exit")+"("+arg+")", api.Void)
	case *ast.ExprThrow:
		v := f.expr(x.Expr, nil)
		t := want
		if t == nil || t.IsVoid() {
			return callv(f.phpx("Throw")+"("+v.code+")", api.Void)
		}
		return callv(f.phpx("ThrowV")+"["+f.typeStr(t)+"]("+v.code+")", t)
	case *ast.ExprMatch:
		return f.match(x, want)
	case *ast.ExprInclude, *ast.ExprIncludeOnce, *ast.ExprRequire, *ast.ExprRequireOnce:
		return callv(f.todo(n, "include/require of PHP files isn't supported")+f.phpx("Unsupported")+`("include")`, api.Any)
	case *ast.ExprEval:
		return callv(f.todo(n, "eval() isn't supported")+f.phpx("Unsupported")+`("eval")`, api.Any)
	case *ast.ExprShellExec:
		return callv(f.todo(n, "shell execution isn't supported")+f.phpx("Unsupported")+`("shell_exec")`, api.Any)
	case *ast.ExprYield, *ast.ExprYieldFrom:
		return callv(f.todo(n, "generators (yield) aren't supported")+f.phpx("Unsupported")+`("yield")`, api.Any)
	}
	return callv(f.todo(n, "unsupported expression %T", n)+f.phpx("Unsupported")+`("expression")`, api.Any)
}

func strPrec(v value) int {
	if v.t != nil && v.t.IsString() {
		return v.prec
	}
	return 7
}

func condPrec(f *fctx, n ast.Vertex) int {
	saved := f.dry
	f.dry = true
	v := f.expr(n, api.Bool)
	f.dry = saved
	return condPrecV(f, v)
}

// condPrecV is the precedence of f.cond(v)'s code.
func condPrecV(f *fctx, v value) int {
	t := v.t
	switch {
	case t != nil && t.IsBool():
		return v.prec
	case t != nil && (t.IsNumber() || t.K == api.KPointer || t.K == api.KFunc || isArrayT(t) || t.K == api.KSlice || t.K == api.KMap):
		return 3
	}
	return 7
}

func (f *fctx) intLit(s string) value {
	s = strings.ReplaceAll(s, "_", "")
	ls := strings.ToLower(s)
	var n uint64
	var err error
	switch {
	case strings.HasPrefix(ls, "0x"):
		n, err = strconv.ParseUint(ls[2:], 16, 64)
	case strings.HasPrefix(ls, "0b"):
		n, err = strconv.ParseUint(ls[2:], 2, 64)
	case strings.HasPrefix(ls, "0o"):
		n, err = strconv.ParseUint(ls[2:], 8, 64)
		s = "0o" + ls[2:]
	case len(ls) > 1 && ls[0] == '0':
		n, err = strconv.ParseUint(ls[1:], 8, 64)
		s = "0o" + ls[1:]
	default:
		n, err = strconv.ParseUint(ls, 10, 64)
	}
	if err != nil || n > 1<<63-1 {
		f2, _ := strconv.ParseFloat(s, 64)
		return value{code: strconv.FormatFloat(f2, 'g', -1, 64), t: api.Float, prec: 7, konst: true}
	}
	return value{code: s, t: api.Int, prec: 7, konst: true}
}

// unquotePHP decodes a PHP string literal.
func unquotePHP(s *ast.ScalarString) string {
	raw := string(s.Value)
	if len(raw) < 2 {
		return raw
	}
	q := raw[0]
	body := raw[1 : len(raw)-1]
	if q == '\'' {
		return strings.NewReplacer(`\\`, `\`, `\'`, `'`).Replace(body)
	}
	if q == '"' {
		return decodeEscapes(body, '"')
	}
	return raw
}

// decodeEscapes decodes the escape sequences of a double-quoted PHP string.
func decodeEscapes(s string, quoteChar byte) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			sb.WriteByte(c)
			continue
		}
		i++
		switch e := s[i]; e {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case 'v':
			sb.WriteByte('\v')
		case 'e':
			sb.WriteByte(27)
		case 'f':
			sb.WriteByte('\f')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			n, _ := strconv.ParseUint(s[i:j], 8, 16)
			sb.WriteByte(byte(n))
			i = j - 1
		case 'x':
			j := i + 1
			for j < len(s) && j < i+3 && isHex(s[j]) {
				j++
			}
			if j == i+1 {
				sb.WriteString(`\x`)
				continue
			}
			n, _ := strconv.ParseUint(s[i+1:j], 16, 8)
			sb.WriteByte(byte(n))
			i = j - 1
		case 'u':
			if i+1 < len(s) && s[i+1] == '{' {
				end := strings.IndexByte(s[i:], '}')
				if end > 0 {
					n, err := strconv.ParseUint(s[i+2:i+end], 16, 32)
					if err == nil {
						sb.WriteRune(rune(n))
						i += end
						continue
					}
				}
			}
			sb.WriteString(`\u`)
		case '\\', '$':
			sb.WriteByte(e)
		case '"':
			if quoteChar == '"' {
				sb.WriteByte('"')
			} else {
				sb.WriteString(`\"`)
			}
		default:
			sb.WriteByte('\\')
			sb.WriteByte(e)
		}
	}
	return sb.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// encapsed converts an interpolated string.
func (f *fctx) encapsed(parts []ast.Vertex) value {
	var codes []string
	for _, p := range parts {
		switch x := p.(type) {
		case *ast.ScalarEncapsedStringPart:
			codes = append(codes, quote(decodeEscapes(string(x.Value), '"')))
		case *ast.ScalarEncapsedStringVar:
			v := f.expr(&ast.ExprVariable{Name: x.Name}, nil)
			if x.Dim != nil {
				v = f.dimFetch(&ast.ExprArrayDimFetch{Var: &ast.ExprVariable{Name: x.Name}, Dim: x.Dim})
			}
			codes = append(codes, f.str(v))
		case *ast.ScalarEncapsedStringBrackets:
			codes = append(codes, f.str(f.expr(x.Var, nil)))
		default:
			codes = append(codes, f.str(f.expr(p, nil)))
		}
	}
	if len(codes) == 0 {
		return prim(`""`, api.String)
	}
	if len(codes) == 1 {
		return value{code: codes[0], t: api.String, prec: 7}
	}
	return value{code: strings.Join(codes, " + "), t: api.String, prec: 4}
}

func (f *fctx) heredoc(x *ast.ScalarHeredoc) value {
	open := string(x.OpenHeredocTkn.Value)
	nowdoc := strings.Contains(open, "'")
	// The closing marker's indentation is removed from every line.
	closing := ""
	if x.CloseHeredocTkn != nil {
		closing = string(x.CloseHeredocTkn.Value)
	}
	indent := closing[:len(closing)-len(strings.TrimLeft(closing, " \t"))]
	var codes []string
	for i, p := range x.Parts {
		switch y := p.(type) {
		case *ast.ScalarEncapsedStringPart:
			s := string(y.Value)
			if indent != "" {
				lines := strings.Split(s, "\n")
				for j := range lines {
					if j > 0 || i == 0 {
						lines[j] = strings.TrimPrefix(lines[j], indent)
					}
				}
				s = strings.Join(lines, "\n")
			}
			if i == len(x.Parts)-1 {
				s = strings.TrimSuffix(s, "\n")
				s = strings.TrimSuffix(s, "\r")
			}
			if !nowdoc {
				s = decodeEscapes(s, 0)
			}
			codes = append(codes, quote(s))
		case *ast.ScalarEncapsedStringVar:
			v := f.expr(&ast.ExprVariable{Name: y.Name}, nil)
			if y.Dim != nil {
				v = f.dimFetch(&ast.ExprArrayDimFetch{Var: &ast.ExprVariable{Name: y.Name}, Dim: y.Dim})
			}
			codes = append(codes, f.str(v))
		case *ast.ScalarEncapsedStringBrackets:
			codes = append(codes, f.str(f.expr(y.Var, nil)))
		default:
			codes = append(codes, f.str(f.expr(p, nil)))
		}
	}
	if len(codes) == 0 {
		return prim(`""`, api.String)
	}
	if len(codes) == 1 {
		return value{code: codes[0], t: api.String, prec: 7}
	}
	return value{code: strings.Join(codes, " + "), t: api.String, prec: 4}
}

func (f *fctx) magic(name string, n ast.Vertex) value {
	switch strings.ToUpper(name) {
	case "__CLASS__":
		if f.cls != nil {
			return prim(quote(f.cls.FQCN), api.String)
		}
		return prim(`""`, api.String)
	case "__FUNCTION__":
		if f.m != nil {
			return prim(quote(f.m.Name), api.String)
		}
	case "__METHOD__":
		if f.m != nil && f.cls != nil {
			return prim(quote(f.cls.FQCN+"::"+f.m.Name), api.String)
		}
	case "__LINE__":
		return value{code: itoa(line(n)), t: api.Int, prec: 7, konst: true}
	case "__NAMESPACE__":
		if f.cls != nil {
			if i := strings.LastIndexByte(f.cls.FQCN, '\\'); i >= 0 {
				return prim(quote(f.cls.FQCN[:i]), api.String)
			}
		}
	case "__DIR__", "__FILE__":
		f.warn(n, "%s has no meaning in a compiled plugin; it is \".\"", name)
		return prim(`"."`, api.String)
	}
	return prim(`""`, api.String)
}

func (f *fctx) constFetch(x *ast.ExprConstFetch) value {
	full := identValue(x.Const)
	name := lastSeg(full)
	switch strings.ToLower(name) {
	case "true":
		return prim("true", api.Bool)
	case "false":
		return prim("false", api.Bool)
	case "null":
		return prim("nil", api.UntypedNil)
	case "php_eol":
		return prim(`"\n"`, api.String)
	case "php_int_max":
		return value{code: "math.MaxInt", t: api.Int, prec: 7, konst: true}.withPkg(f, "math")
	case "php_int_min":
		return value{code: "math.MinInt", t: api.Int, prec: 7, konst: true}.withPkg(f, "math")
	case "php_float_epsilon":
		return value{code: f.phpx("PHP_FLOAT_EPSILON"), t: api.Float, prec: 7, konst: true}
	case "inf":
		return prim(f.phpx("INF"), api.Float)
	case "nan":
		return prim(f.phpx("NAN"), api.Float)
	case "m_pi":
		return value{code: "math.Pi", t: api.Float, prec: 7, konst: true}.withPkg(f, "math")
	}
	if g, ok := f.cv.defines[strings.ToLower(name)]; ok {
		return prim(g, api.Any)
	}
	if c, ok := f.cv.idx.Packages[api.PhpxPath].Consts[name]; ok {
		return value{code: f.phpx(name), t: c.Type, prec: 7, konst: c.Type.IsNumber()}
	}
	if t, ok := f.cv.idx.Packages[api.PhpxPath].Vars[name]; ok {
		return prim(f.phpx(name), t)
	}
	return callv(f.todo(x, "unknown constant %s", full)+f.phpx("Unsupported")+"("+quote("constant "+full)+")", api.Any)
}

// withPkg rewrites a "pkg.Name" code to use the package placeholder.
func (v value) withPkg(f *fctx, pkg string) value {
	v.code = f.pkgRef(pkg) + strings.TrimPrefix(v.code, pkg)
	return v
}

// thisValue is $this as a value.
func (f *fctx) thisValue() value {
	if f.cls == nil || f.static {
		return callv(f.phpx("Unsupported")+`("$this outside an object")`, api.Any)
	}
	if f.cls.Poly && f.selfVar != "" {
		return prim(f.selfVar, f.cv.classType(f.cls))
	}
	return prim(f.recv, f.cv.classType(f.cls))
}

func (f *fctx) variable(x *ast.ExprVariable) value {
	name := varName(x)
	if name == "" {
		return callv(f.todo(x, "variable variables ($$name) aren't supported")+f.phpx("Unsupported")+`("variable variable")`, api.Any)
	}
	if name == "this" {
		return f.thisValue()
	}
	v := f.lookupVar(name)
	if v == nil {
		v = f.addVar(name)
	}
	t := v.t
	if t == nil {
		t = api.Any
	}
	if v.ptr {
		return value{code: "*" + v.goName, t: t, prec: 6, lvalue: true}
	}
	return value{code: v.goName, t: t, prec: 7, lvalue: true}
}

// arrayLit converts an array literal.
func (f *fctx) arrayLit(items []ast.Vertex, want *api.Type) value {
	type item struct {
		key, val ast.Vertex
		spread   bool
	}
	var list []item
	for _, it := range items {
		ai, ok := it.(*ast.ExprArrayItem)
		if !ok || ai.Val == nil {
			continue
		}
		list = append(list, item{ai.Key, ai.Val, ai.EllipsisTkn != nil})
	}
	hasKeys := false
	hasSpread := false
	for _, it := range list {
		if it.key != nil {
			hasKeys = true
		}
		if it.spread {
			hasSpread = true
		}
	}
	// Go collections the context wants.
	if want != nil && !hasSpread {
		switch {
		case want.K == api.KSlice && !hasKeys:
			var vals []string
			for _, it := range list {
				vals = append(vals, f.coerce(f.expr(it.val, want.Elem), want.Elem))
			}
			return prim(f.typeStr(want)+"{"+strings.Join(vals, ", ")+"}", want)
		case want.K == api.KMap && (hasKeys || len(list) == 0):
			var vals []string
			for _, it := range list {
				if it.key == nil {
					continue
				}
				k := f.coerce(f.expr(it.key, want.Key), want.Key)
				v := f.coerceGo(f.expr(it.val, want.Elem), want.Elem)
				vals = append(vals, k+": "+v)
			}
			return prim(f.typeStr(want)+"{"+strings.Join(vals, ", ")+"}", want)
		}
	}
	if len(list) == 0 {
		return callv(f.phpx("NewArray")+"()", arrayT(nil, nil))
	}
	// Element type, when all elements agree.
	var elem *api.Type
	same := true
	var vals []value
	for _, it := range list {
		v := f.expr(it.val, nil)
		vals = append(vals, v)
		if it.spread {
			same = false
			continue
		}
		if elem == nil {
			elem = v.t
		} else if !api.Identical(elem, v.t) {
			same = false
		}
	}
	// PHP arrays are heterogeneous: element types only come from doc comments.
	_ = same
	t := arrayT(nil, nil)
	if hasSpread {
		var parts []string
		var cur []string
		flush := func() {
			if len(cur) > 0 {
				parts = append(parts, f.phpx("List")+"("+strings.Join(cur, ", ")+")")
				cur = nil
			}
		}
		for i, it := range list {
			if it.spread {
				flush()
				parts = append(parts, vals[i].code)
				continue
			}
			cur = append(cur, f.arrayElemCode(vals[i]))
		}
		flush()
		return callv(f.phpx("ArrayMerge")+"("+strings.Join(parts, ", ")+")", arrayT(nil, nil))
	}
	var codes []string
	for i, it := range list {
		if hasKeys {
			k := "nil"
			if it.key != nil {
				k = f.expr(it.key, nil).code
			} else {
				k = f.phpx("NextKey")
			}
			codes = append(codes, k, f.arrayElemCode(vals[i]))
		} else {
			codes = append(codes, f.arrayElemCode(vals[i]))
		}
	}
	if hasKeys {
		return callv(f.phpx("Map")+"("+strings.Join(codes, ", ")+")", t)
	}
	return callv(f.phpx("List")+"("+strings.Join(codes, ", ")+")", t)
}

// arrayElemCode is the code of a value stored in an array (arrays are copied).
func (f *fctx) arrayElemCode(v value) string {
	if isArrayT(v.t) && v.lvalue {
		return paren(v, 7) + ".Clone()"
	}
	if isGoCollection(v.t) {
		return f.phpx("FromGo") + "(" + v.code + ")"
	}
	return v.code
}

// coerceGo converts a PHP value for a Go API that takes plain Go values (configs).
func (f *fctx) coerceGo(v value, to *api.Type) string {
	if to != nil && to.IsAny() && isArrayT(v.t) {
		return f.phpx("ToGo") + "(" + v.code + ")"
	}
	return f.coerce(v, to)
}

// dimFetch converts $a[$k] (reading).
func (f *fctx) dimFetch(x *ast.ExprArrayDimFetch) value {
	base := f.expr(x.Var, nil)
	if x.Dim == nil {
		return callv(f.todo(x, "[] used for reading")+f.phpx("Unsupported")+`("[] read")`, api.Any)
	}
	t := base.t
	switch {
	case isArrayT(t):
		_, vt := arrayElem(t)
		k := f.expr(x.Dim, nil)
		code := paren(base, 7) + ".Get(" + k.code + ")"
		if !isAny(vt) {
			return value{code: f.phpx("As") + "[" + f.typeStr(vt) + "](" + code + ")", t: vt, prec: 7, call: true, lvalue: isArrayT(vt)}
		}
		return value{code: code, t: api.Any, prec: 7, call: true, lvalue: true}
	case t != nil && (t.K == api.KSlice || t.K == api.KArray):
		k := f.expr(x.Dim, api.Int)
		return callv(f.phpx("SliceAt")+"("+base.code+", "+f.coerce(k, api.Int)+")", t.Elem)
	case t != nil && t.K == api.KMap:
		k := f.expr(x.Dim, t.Key)
		return value{code: paren(base, 7) + "[" + f.coerce(k, t.Key) + "]", t: t.Elem, prec: 7}
	case t != nil && t.IsString():
		k := f.expr(x.Dim, api.Int)
		return callv(f.phpx("StrAt")+"("+base.code+", "+f.coerce(k, api.Int)+")", api.String)
	}
	k := f.expr(x.Dim, nil)
	return value{code: f.phpx("Index") + "(" + base.code + ", " + k.code + ")", t: api.Any, prec: 7, call: true, lvalue: true}
}

// propFetch converts $obj->prop.
func (f *fctx) propFetch(objN, propN ast.Vertex, nullsafe bool) value {
	name := identValue(propN)
	if name == "" {
		return callv(f.todo(propN, "dynamic property names aren't supported")+f.phpx("Unsupported")+`("dynamic property")`, api.Any)
	}
	if varName(objN) == "this" && f.cls != nil && !f.static {
		if p := f.cls.findProp(name); p != nil && !p.Static {
			return value{code: f.recv + "." + p.GoName, t: p.Type, prec: 7, lvalue: true}
		}
		if f.cls.Kind == kindEnum {
			switch name {
			case "name":
				return prim(f.recv+".name", api.String)
			case "value":
				return prim(f.recv+".value", f.enumValueType(f.cls))
			}
		}
	}
	obj := f.expr(objN, nil)
	if nullsafe {
		inner := f.fieldOf(obj, name, propN)
		t := inner.t
		if !t.Nilable() {
			t = api.Any
		}
		tmp := f.newTmp("ns")
		innerTmp := f.fieldOf(value{code: tmp, t: obj.t, prec: 7}, name, propN)
		return callv(fmt.Sprintf("func() %s { %s := %s; if %s(%s) { return nil }; return %s }()", f.typeStr(t), tmp, obj.code, f.phpx("IsNull"), tmp, f.coerce(innerTmp, t)), t)
	}
	return f.fieldOf(obj, name, propN)
}

func (f *fctx) enumValueType(c *class) *api.Type {
	switch c.EnumBacking {
	case "int":
		return api.Int
	case "string":
		return api.String
	}
	return api.Any
}

// fieldOf reads property name of an object value.
func (f *fctx) fieldOf(obj value, name string, n ast.Vertex) value {
	t := obj.t
	if c := f.cv.localClassOf(t); c != nil {
		if c.Kind == kindEnum {
			switch name {
			case "name":
				return prim(paren(obj, 7)+".name", api.String)
			case "value":
				return prim(paren(obj, 7)+".value", f.enumValueType(c))
			}
		}
		if p := c.findProp(name); p != nil && !p.Static {
			recv := paren(obj, 7)
			if c.Poly && t.Name == c.IfaceName {
				recv += "." + asMethod(c) + "()"
			}
			return value{code: recv + "." + p.GoName, t: p.Type, prec: 7, lvalue: true}
		}
		if c.Poly && t.Name == c.IfaceName {
			// A property of a subclass: read it dynamically.
			return callv(f.phpx("Prop")+"("+obj.code+", "+quote(name)+")", api.Any)
		}
	}
	if t != nil && !isAny(t) {
		if ti := f.cv.typeInfo(t.Deref()); ti != nil {
			for _, fn := range []string{pascal(name), name} {
				if ft, ok := ti.Fields[fn]; ok {
					return value{code: paren(obj, 7) + "." + fn, t: ft, prec: 7, lvalue: true}
				}
			}
			// A getter for it.
			if ms := f.cv.methodSet(t); ms != nil {
				if gn := findMethodName(ms, "get"+pascal(name)); gn != "" && len(ms[gn].Params) == 0 && len(ms[gn].Results) > 0 {
					return callv(paren(obj, 7)+"."+gn+"()", ms[gn].Results[0])
				}
			}
		}
	}
	return value{code: f.phpx("Prop") + "(" + obj.code + ", " + quote(name) + ")", t: api.Any, prec: 7, call: true}
}

// staticClass resolves the class of a static access (self, static, parent or a name).
func (f *fctx) staticClass(n ast.Vertex) (local *class, phpName string, kind string) {
	name := identValue(n)
	switch strings.ToLower(name) {
	case "self", "static":
		return f.cls, "", "self"
	case "parent":
		if f.cls != nil && f.cls.Parent != nil {
			return f.cls.Parent, "", "parent"
		}
		if f.cls != nil {
			return nil, f.cls.ExtParentPH, "parent"
		}
		return nil, "", "parent"
	}
	if name == "" {
		return nil, "", "dynamic"
	}
	full := f.cv.resolveName(f.file, n)
	if c := f.cv.classes[strings.ToLower(full)]; c != nil {
		return c, full, "class"
	}
	return nil, full, "ext"
}

func (f *fctx) staticPropFetch(x *ast.ExprStaticPropertyFetch) value {
	name := varName(x.Prop)
	c, php, _ := f.staticClass(x.Class)
	if c != nil {
		if p := c.findProp(name); p != nil && p.Static {
			return value{code: p.GoName, t: p.Type, prec: 7, lvalue: true}
		}
	}
	return callv(f.todo(x, "static property %s::$%s not found", php, name)+f.phpx("Unsupported")+"("+quote("static property "+name)+")", api.Any)
}

// classConst converts Class::CONST and Class::class.
func (f *fctx) classConst(x *ast.ExprClassConstFetch) value {
	name := identValue(x.Const)
	c, php, kind := f.staticClass(x.Class)
	if strings.EqualFold(name, "class") {
		switch {
		case kind == "dynamic":
			v := f.expr(x.Class, nil)
			return callv(f.phpx("ClassName")+"("+v.code+")", api.String)
		case c != nil:
			return prim(quote(c.FQCN), api.String)
		}
		return prim(quote(php), api.String)
	}
	if c != nil {
		if c.Kind == kindEnum {
			for _, e := range c.EnumCases {
				if e.Name == name {
					return prim(e.GoName, f.cv.classType(c))
				}
			}
		}
		if cd := c.findConst(name); cd != nil {
			return value{code: cd.GoName, t: cd.Type, prec: 7, konst: cd.isConst && cd.Type != nil && cd.Type.IsNumber()}
		}
		return callv(f.todo(x, "constant %s::%s not found", c.FQCN, name)+f.phpx("Unsupported")+"("+quote("constant "+name)+")", api.Any)
	}
	if v, ok := f.extConst(php, name); ok {
		return v
	}
	return callv(f.todo(x, "constant %s::%s has no pocketmine-go equivalent", php, name)+f.phpx("Unsupported")+"("+quote(php+"::"+name)+")", api.Any)
}

// phpLimits are pocketmine\utils\Limits' constants.
var phpLimits = map[string]string{
	"UINT8_MAX": "255", "INT8_MIN": "-128", "INT8_MAX": "127", "UINT16_MAX": "65535",
	"INT16_MIN": "-32768", "INT16_MAX": "32767", "UINT32_MAX": "4294967295",
	"INT32_MIN": "-2147483648", "INT32_MAX": "2147483647", "UINT64_MAX": "-1",
	"INT64_MIN": "-9223372036854775808", "INT64_MAX": "9223372036854775807",
}

// extConst finds the Go constant for a constant of a server class.
func (f *fctx) extConst(php, name string) (value, bool) {
	if strings.EqualFold(php, `pocketmine\utils\Limits`) {
		if v, ok := phpLimits[name]; ok {
			return value{code: v, t: api.Int, prec: 7, konst: true}, true
		}
	}
	pkg := f.cv.extPackage(php)
	if pkg == nil {
		return value{}, false
	}
	short := lastSeg(php)
	var cands []string
	if t := f.cv.extClassType(php); t != nil {
		cands = append(cands, t.Deref().ObjName()+pascal(name))
	}
	cands = append(cands, short+pascal(name), pascal(name), pascal(strings.ToLower(name)), name)
	for _, c := range cands {
		if k, ok := pkg.Consts[c]; ok {
			return value{code: f.pkgRef(pkg.Path) + "." + c, t: k.Type, prec: 7, konst: k.Type.IsNumber() && k.Type.K == api.KBasic}, true
		}
		if t, ok := pkg.Vars[c]; ok {
			return prim(f.pkgRef(pkg.Path)+"."+c, t), true
		}
	}
	// Fuzzy: ignore case and underscores.
	want := []string{normName(short + name), normName(name)}
	for _, w := range want {
		for cn, k := range pkg.Consts {
			if normName(cn) == w {
				return value{code: f.pkgRef(pkg.Path) + "." + cn, t: k.Type, prec: 7, konst: k.Type.IsNumber() && k.Type.K == api.KBasic}, true
			}
		}
	}
	return value{}, false
}

// cast converts (int)/(float)/(string)/(bool) casts.
func (f *fctx) cast(n ast.Vertex, to *api.Type) value {
	v := f.expr(n, to)
	if v.t != nil && api.Identical(v.t, to) {
		return v
	}
	switch {
	case to.IsInt() && v.t.IsFloat():
		return callv("int("+v.code+")", api.Int)
	case to.IsFloat() && v.t.IsInt():
		return callv("float64("+v.code+")", api.Float)
	case to.IsBool():
		return value{code: f.cond(v), t: api.Bool, prec: condPrecV(f, v)}
	case to.IsString():
		return callv(f.str(v), api.String)
	case to.IsInt():
		return callv(f.phpx("ToInt")+"("+v.code+")", api.Int)
	case to.IsFloat():
		return callv(f.phpx("ToFloat")+"("+v.code+")", api.Float)
	}
	return prim(f.coerce(v, to), to)
}

// arith converts + - *.
func (f *fctx) arith(ln, rn ast.Vertex, op, fn string) value {
	l, r := f.expr(ln, nil), f.expr(rn, nil)
	lt, rt := l.t, r.t
	if op == "+" && isArrayT(lt) {
		return callv(f.phpx("ToArray")+"("+f.phpx("Add")+"("+l.code+", "+r.code+"))", arrayT(nil, nil))
	}
	if lt.IsNumber() && rt.IsNumber() {
		p := 4
		if op == "*" {
			p = 5
		}
		switch {
		case lt.IsInt() && rt.IsInt():
			if !api.Identical(lt, rt) && !l.konst && !r.konst {
				return value{code: paren(l, p) + " " + op + " " + lt.Name + "(" + r.code + ")", t: lt, prec: p}
			}
			t := lt
			if l.konst {
				t = rt
			}
			return value{code: paren(l, p) + " " + op + " " + paren(r, p+1), t: t, prec: p, konst: l.konst && r.konst}
		default:
			lc, rc := paren(l, p), paren(r, p+1)
			if !lt.IsFloat() && !l.konst || lt.IsFloat() && lt.Name != "float64" {
				lc = "float64(" + l.code + ")"
			}
			if !rt.IsFloat() && !r.konst || rt.IsFloat() && rt.Name != "float64" {
				rc = "float64(" + r.code + ")"
			}
			return value{code: lc + " " + op + " " + rc, t: api.Float, prec: p, konst: l.konst && r.konst}
		}
	}
	return callv(f.phpx(fn)+"("+l.code+", "+r.code+")", api.Any)
}

func (f *fctx) div(ln, rn ast.Vertex) value {
	l, r := f.expr(ln, nil), f.expr(rn, nil)
	if l.t.IsNumber() && r.t.IsNumber() {
		lc, rc := paren(l, 5), paren(r, 6)
		if !(l.t.IsFloat() && l.t.Name == "float64") && !l.konst {
			lc = "float64(" + l.code + ")"
		} else if l.konst && l.t.IsInt() {
			lc = l.code + ".0"
		}
		if !(r.t.IsFloat() && r.t.Name == "float64") && !r.konst {
			rc = "float64(" + r.code + ")"
		}
		return value{code: lc + " / " + rc, t: api.Float, prec: 5}
	}
	return callv(f.phpx("Div")+"("+l.code+", "+r.code+")", api.Any)
}

func (f *fctx) intOp(ln, rn ast.Vertex, op string) value {
	l, r := f.expr(ln, api.Int), f.expr(rn, api.Int)
	lc := f.coerce(l, api.Int)
	rc := f.coerce(r, api.Int)
	if l.t.IsInt() {
		lc = paren(l, 5)
	}
	if r.t.IsInt() {
		rc = paren(r, 6)
	}
	t := api.Int
	if l.t.IsInt() {
		t = l.t
	}
	return value{code: lc + " " + op + " " + rc, t: t, prec: 5, konst: l.konst && r.konst}
}

// identical converts === and !==.
func (f *fctx) identical(ln, rn ast.Vertex, negate bool) value {
	l, r := f.expr(ln, nil), f.expr(rn, nil)
	not := ""
	eq := "=="
	if negate {
		not = "!"
		eq = "!="
	}
	if isNilV(r) {
		l, r = r, l
	}
	if isNilV(l) {
		if isNilV(r) {
			return prim(strconv.FormatBool(!negate), api.Bool)
		}
		if r.t != nil && (r.t.K == api.KPointer || r.t.K == api.KSlice || r.t.K == api.KMap || r.t.K == api.KFunc) && !isArrayT(r.t) {
			return value{code: paren(r, 4) + " " + eq + " nil", t: api.Bool, prec: 3}
		}
		if r.t != nil && !r.t.Nilable() && !isAny(r.t) && !isArrayT(r.t) {
			// A non-nullable value is never null (a call keeps go vet quiet about constants).
			return value{code: not + f.phpx("IsNull") + "(" + r.code + ")", t: api.Bool, prec: 6}
		}
		return value{code: not + f.phpx("IsNull") + "(" + r.code + ")", t: api.Bool, prec: 6}
	}
	// Comparisons with true/false.
	if r.t.IsBool() && (r.code == "true" || r.code == "false") {
		l, r = r, l
	}
	if l.t.IsBool() && (l.code == "true" || l.code == "false") {
		if r.t.IsBool() {
			if (l.code == "true") != negate {
				return r
			}
			return value{code: "!" + paren(r, 6), t: api.Bool, prec: 6}
		}
		if !isAny(r.t) {
			return prim(strconv.FormatBool(negate), api.Bool)
		}
	}
	if l.t != nil && r.t != nil && !isAny(l.t) && !isAny(r.t) {
		switch {
		case api.Identical(l.t, r.t) && (l.t.K == api.KBasic || l.t.K == api.KPointer && !isArrayT(l.t) || f.underlying(l.t).K == api.KBasic):
			return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		case l.konst && (r.t.IsNumber() || f.underlying(r.t).IsNumber()) && (l.t.IsInt() == f.underlying(r.t).IsInt()):
			return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		case r.konst && (l.t.IsNumber() || f.underlying(l.t).IsNumber()) && (r.t.IsInt() == f.underlying(l.t).IsInt()):
			return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		case l.t.IsString() != r.t.IsString() && l.t.K == api.KBasic && r.t.K == api.KBasic:
			return prim(strconv.FormatBool(negate), api.Bool)
		case (f.cv.isInterface(l.t) || f.cv.isInterface(r.t)) && !isArrayT(l.t) && !isArrayT(r.t) && l.t.K != api.KBasic && r.t.K != api.KBasic:
			return value{code: "any(" + l.code + ") " + eq + " any(" + r.code + ")", t: api.Bool, prec: 3}
		}
	}
	return value{code: not + f.phpx("StrictEq") + "(" + l.code + ", " + r.code + ")", t: api.Bool, prec: 6}
}

// equal converts == and !=.
func (f *fctx) equal(ln, rn ast.Vertex, negate bool) value {
	l, r := f.expr(ln, nil), f.expr(rn, nil)
	eq := "=="
	not := ""
	if negate {
		eq, not = "!=", "!"
	}
	if l.t != nil && r.t != nil {
		switch {
		case l.t.IsString() && r.t.IsString(), l.t.IsBool() && r.t.IsBool():
			return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		case l.t.IsNumber() && r.t.IsNumber():
			if api.Identical(l.t, r.t) || l.konst || r.konst {
				return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
			}
			return value{code: "float64(" + l.code + ") " + eq + " float64(" + r.code + ")", t: api.Bool, prec: 3}
		case isNilV(r) || isNilV(l):
			o := l
			if isNilV(l) {
				o = r
			}
			c := f.cond(o)
			if negate {
				return value{code: c, t: api.Bool, prec: condPrecV(f, o)}
			}
			return value{code: "!" + paren(value{code: c, prec: condPrecV(f, o)}, 6), t: api.Bool, prec: 6}
		case api.Identical(l.t, r.t) && (l.t.K == api.KPointer && !isArrayT(l.t)):
			return value{code: paren(l, 4) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		case r.t.IsBool() && !isAny(l.t):
			return value{code: f.cond(l) + " " + eq + " " + paren(r, 4), t: api.Bool, prec: 3}
		}
	}
	return value{code: not + f.phpx("LooseEq") + "(" + l.code + ", " + r.code + ")", t: api.Bool, prec: 6}
}

func (f *fctx) compare(ln, rn ast.Vertex, op, fn string) value {
	l, r := f.expr(ln, nil), f.expr(rn, nil)
	if l.t.IsNumber() && r.t.IsNumber() {
		if api.Identical(l.t, r.t) || l.konst || r.konst && (r.t.IsInt() || l.t.IsFloat()) {
			return value{code: paren(l, 4) + " " + op + " " + paren(r, 4), t: api.Bool, prec: 3}
		}
		return value{code: "float64(" + l.code + ") " + op + " float64(" + r.code + ")", t: api.Bool, prec: 3}
	}
	if l.t.IsString() && r.t.IsString() {
		return callv(f.phpx(fn)+"("+l.code+", "+r.code+")", api.Bool)
	}
	return callv(f.phpx(fn)+"("+l.code+", "+r.code+")", api.Bool)
}

// simple reports whether evaluating n eagerly is safe (no calls, no side effects).
func simple(n ast.Vertex) bool {
	ok := true
	walk(n, func(m ast.Vertex) bool {
		switch m.(type) {
		case *ast.ExprMethodCall, *ast.ExprNullsafeMethodCall, *ast.ExprStaticCall, *ast.ExprFunctionCall,
			*ast.ExprNew, *ast.ExprThrow, *ast.ExprAssign, *ast.ExprPreInc, *ast.ExprPostInc, *ast.ExprPreDec,
			*ast.ExprPostDec, *ast.ExprPropertyFetch, *ast.ExprNullsafePropertyFetch, *ast.ExprArrayDimFetch,
			*ast.ExprMatch, *ast.ExprExit, *ast.ExprClosure, *ast.ExprArrowFunction:
			ok = false
		}
		return ok
	})
	return ok
}

// unify finds a common type for two branches.
func (f *fctx) unify(a, b value, want *api.Type) *api.Type {
	switch {
	case want != nil && !want.IsVoid() && !isAny(want) && f.cv.assignable(a.t, want) && f.cv.assignable(b.t, want):
		return want
	case isNilV(a) && b.t != nil && (b.t.Nilable() || isArrayT(b.t)):
		return b.t
	case isNilV(b) && a.t != nil && (a.t.Nilable() || isArrayT(a.t)):
		return a.t
	case a.t != nil && b.t != nil && api.Identical(a.t, b.t):
		return a.t
	case a.t.IsNumber() && b.t.IsNumber():
		if a.t.IsInt() && b.t.IsInt() {
			return api.Int
		}
		return api.Float
	case a.t != nil && b.t != nil && f.cv.assignable(a.t, b.t) && !isNilV(a):
		return b.t
	case a.t != nil && b.t != nil && f.cv.assignable(b.t, a.t) && !isNilV(b):
		return a.t
	}
	return api.Any
}

func (f *fctx) ternary(x *ast.ExprTernary, want *api.Type) value {
	cond := f.expr(x.Cond, api.Bool)
	if x.IfTrue == nil {
		// $a ?: $b
		b := f.expr(x.IfFalse, want)
		t := f.unify(cond, b, want)
		if simple(x.IfFalse) {
			return callv(f.phpx("Elvis")+"["+f.typeStr(t)+"]("+f.coerce(cond, t)+", "+f.coerce(b, t)+")", t)
		}
		tmp := f.newTmp("v")
		return callv(fmt.Sprintf("func() %s { if %s := %s; %s { return %s }; return %s }()", f.typeStr(t), tmp, cond.code,
			f.cond(value{code: tmp, t: cond.t, prec: 7}), f.coerce(value{code: tmp, t: cond.t, prec: 7}, t), f.coerce(b, t)), t)
	}
	a, b := f.expr(x.IfTrue, want), f.expr(x.IfFalse, want)
	t := f.unify(a, b, want)
	if simple(x.IfTrue) && simple(x.IfFalse) {
		return callv(f.phpx("Ternary")+"["+f.typeStr(t)+"]("+f.cond(cond)+", "+f.coerce(a, t)+", "+f.coerce(b, t)+")", t)
	}
	if t.IsVoid() {
		t = api.Any
	}
	return callv(fmt.Sprintf("func() %s { if %s { return %s }; return %s }()", f.typeStr(t), f.cond(cond), f.coerce(a, t), f.coerce(b, t)), t)
}

func (f *fctx) coalesce(ln, rn ast.Vertex, want *api.Type) value {
	l := f.expr(ln, want)
	r := f.expr(rn, want)
	if l.t != nil && !l.t.Nilable() && !isAny(l.t) && !isArrayT(l.t) && !isNilV(l) {
		// Never null.
		return l
	}
	var t *api.Type
	switch {
	case isAny(l.t) && want != nil && !want.IsVoid() && !isAny(want):
		t = want
	default:
		t = f.unify(l, r, want)
	}
	if t.IsVoid() {
		t = api.Any
	}
	if _, isThrow := rn.(*ast.ExprThrow); isThrow || !simple(rn) {
		tmp := f.newTmp("v")
		rv := f.expr(rn, t)
		return callv(fmt.Sprintf("func() %s { if %s := %s; !%s(%s) { return %s }; return %s }()", f.typeStr(t), tmp, l.code,
			f.phpx("IsNull"), tmp, f.coerce(value{code: tmp, t: l.t, prec: 7}, t), f.coerce(rv, t)), t)
	}
	if isAny(l.t) && !isAny(t) {
		return callv(f.phpx("CoalesceAs")+"["+f.typeStr(t)+"]("+l.code+", "+f.coerce(r, t)+")", t)
	}
	return callv(f.phpx("Coalesce")+"["+f.typeStr(t)+"]("+f.coerce(l, t)+", "+f.coerce(r, t)+")", t)
}

// isset converts isset($a, $b[...], $c->d).
func (f *fctx) isset(vars []ast.Vertex) value {
	var parts []string
	for _, v := range vars {
		switch x := v.(type) {
		case *ast.ExprArrayDimFetch:
			if x.Dim == nil {
				continue
			}
			base := f.expr(x.Var, nil)
			k := f.expr(x.Dim, nil)
			switch {
			case isArrayT(base.t):
				parts = append(parts, paren(base, 7)+".Isset("+k.code+")")
			case base.t != nil && base.t.K == api.KMap:
				tmp := f.newTmp("ok")
				parts = append(parts, fmt.Sprintf("func() bool { _, %s := %s[%s]; return %s }()", tmp, base.code, f.coerce(k, base.t.Key), tmp))
			case base.t != nil && (base.t.K == api.KSlice || base.t.IsString()):
				parts = append(parts, f.coerce(k, api.Int)+" < len("+base.code+")")
			default:
				parts = append(parts, f.phpx("Isset")+"("+base.code+", "+k.code+")")
			}
		default:
			val := f.expr(v, nil)
			if val.t != nil && !val.t.Nilable() && !isAny(val.t) && !isArrayT(val.t) {
				parts = append(parts, "true")
				continue
			}
			parts = append(parts, "!"+f.phpx("IsNull")+"("+val.code+")")
		}
	}
	if len(parts) == 0 {
		return prim("false", api.Bool)
	}
	if len(parts) == 1 {
		return value{code: parts[0], t: api.Bool, prec: 3}
	}
	return value{code: strings.Join(parts, " && "), t: api.Bool, prec: 2}
}

func (f *fctx) instanceOf(x *ast.ExprInstanceOf) value {
	v := f.expr(x.Expr, nil)
	name := identValue(x.Class)
	if name == "" {
		cls := f.expr(x.Class, nil)
		return callv(f.phpx("LooseEq")+"("+f.phpx("ClassName")+"("+v.code+"), "+f.phpx("ClassName")+"("+cls.code+"))", api.Bool)
	}
	full := f.cv.resolveName(f.file, x.Class)
	switch strings.ToLower(name) {
	case "self", "static":
		if f.cls != nil {
			full = f.cls.FQCN
		}
	}
	if isThrowableClass(full) || strings.EqualFold(full, "throwable") {
		return callv(f.phpx("InstanceOfValue")+"("+v.code+", "+quote(strings.ToLower(full))+")", api.Bool)
	}
	t := f.cv.classRef(full, f.cls)
	if isAny(t) {
		f.warn(x, "instanceof %s: the class doesn't exist in pocketmine-go; this is always false", full)
		return callv(f.phpx("InstanceOfValue")+"("+v.code+", "+quote(strings.ToLower(full))+")", api.Bool)
	}
	if v.t != nil && api.Identical(v.t, t) {
		return value{code: "!" + f.phpx("IsNull") + "(" + v.code + ")", t: api.Bool, prec: 6}
	}
	return callv(f.phpx("Is")+"["+f.typeStr(t)+"]("+v.code+")", api.Bool)
}

func (f *fctx) match(x *ast.ExprMatch, want *api.Type) value {
	subj := f.expr(x.Expr, nil)
	// Result type.
	var vals []value
	for _, a := range x.Arms {
		arm := a.(*ast.MatchArm)
		saved := f.dry
		f.dry = true
		vals = append(vals, f.expr(arm.ReturnExpr, want))
		f.dry = saved
	}
	t := want
	if t == nil || t.IsVoid() || len(vals) > 0 {
		t = nil
		for _, v := range vals {
			if v.t != nil && v.t.IsVoid() {
				continue
			}
			if _, isThrow := v.t, false; isThrow {
				continue
			}
			if t == nil {
				t = v.t
				continue
			}
			t = f.unify(value{t: t}, v, want)
		}
	}
	if t == nil || t.IsNil() || t.IsVoid() {
		t = api.Any
	}
	tmp := f.newTmp("m")
	var sb strings.Builder
	fmt.Fprintf(&sb, "func() %s {\n%s := %s\n_ = %s\nswitch {\n", f.typeStr(t), tmp, subj.code, tmp)
	hasDefault := false
	subjIsTrue := subj.code == "true"
	for _, a := range x.Arms {
		arm := a.(*ast.MatchArm)
		var conds []string
		if arm.DefaultTkn != nil {
			sb.WriteString("default:\n")
			hasDefault = true
		} else {
			for _, e := range arm.Exprs {
				if subjIsTrue {
					conds = append(conds, f.cond(f.expr(e, api.Bool)))
					continue
				}
				cv := f.expr(e, subj.t)
				conds = append(conds, f.phpx("StrictEq")+"("+tmp+", "+cv.code+")")
			}
			sb.WriteString("case " + strings.Join(conds, ", ") + ":\n")
		}
		if th, ok := arm.ReturnExpr.(*ast.ExprThrow); ok {
			sb.WriteString(f.phpx("Throw") + "(" + f.expr(th.Expr, nil).code + ")\n")
			sb.WriteString("return " + f.zero(t) + "\n")
			continue
		}
		v := f.expr(arm.ReturnExpr, t)
		if v.t != nil && v.t.IsVoid() {
			sb.WriteString(v.code + "\nreturn " + f.zero(t) + "\n")
			continue
		}
		sb.WriteString("return " + f.coerce(v, t) + "\n")
	}
	sb.WriteString("}\n")
	if !hasDefault {
		sb.WriteString(f.phpx("Throw") + "(" + f.phpx("NewError") + `("UnhandledMatchError", "Unhandled match case"))` + "\n")
	}
	sb.WriteString("return " + f.zero(t) + "\n}()")
	return callv(sb.String(), t)
}
