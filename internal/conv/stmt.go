package conv

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// block converts a list of statements.
func (f *fctx) block(list []ast.Vertex) string {
	var out []string
	for _, s := range list {
		if code := f.stmt(s); code != "" {
			out = append(out, code)
		}
		if _, halt := s.(*ast.StmtHaltCompiler); halt {
			break
		}
	}
	return strings.Join(out, "\n")
}

func stmtList(n ast.Vertex) []ast.Vertex {
	switch x := n.(type) {
	case nil:
		return nil
	case *ast.StmtStmtList:
		return x.Stmts
	}
	return []ast.Vertex{n}
}

// terminates reports whether Go code ends with a terminating statement.
func terminates(code string) bool {
	code = strings.TrimSpace(code)
	i := strings.LastIndexByte(code, '\n')
	last := strings.TrimSpace(code[i+1:])
	return strings.HasPrefix(last, "return") || strings.HasPrefix(last, "panic(")
}

func (f *fctx) stmt(n ast.Vertex) string {
	switch x := n.(type) {
	case *ast.StmtExpression:
		return f.exprStmt(x.Expr)
	case *ast.StmtStmtList:
		return f.block(x.Stmts)
	case *ast.StmtEcho:
		var args []string
		for _, e := range x.Exprs {
			args = append(args, f.expr(e, nil).code)
		}
		return f.phpx("Echo") + "(" + strings.Join(args, ", ") + ")"
	case *ast.StmtIf:
		return f.ifStmt(x)
	case *ast.StmtWhile:
		return f.loopStmt(func(l *loop) string {
			c := f.expr(x.Cond, api.Bool)
			body := f.block(stmtList(x.Stmt))
			cond := f.cond(c)
			if cond == "true" {
				return "for {\n" + body + "\n}"
			}
			return "for " + cond + " {\n" + body + "\n}"
		})
	case *ast.StmtDo:
		return f.loopStmt(func(l *loop) string {
			body := f.block(stmtList(x.Stmt))
			c := f.cond(f.expr(x.Cond, api.Bool))
			ok := f.newTmp("more")
			return fmt.Sprintf("for %s := true; %s; %s = %s {\n%s\n}", ok, ok, ok, c, body)
		})
	case *ast.StmtFor:
		return f.forStmt(x)
	case *ast.StmtForeach:
		return f.foreachStmt(x)
	case *ast.StmtSwitch:
		return f.switchStmt(x)
	case *ast.StmtBreak:
		return f.breakStmt(x.Expr, false, n)
	case *ast.StmtContinue:
		return f.breakStmt(x.Expr, true, n)
	case *ast.StmtReturn:
		return f.returnStmt(x.Expr)
	case *ast.StmtThrow:
		return f.phpx("Throw") + "(" + f.expr(x.Expr, nil).code + ")"
	case *ast.StmtTry:
		return f.tryStmt(x)
	case *ast.StmtUnset:
		var out []string
		for _, v := range x.Vars {
			out = append(out, f.unset(v))
		}
		return strings.Join(out, "\n")
	case *ast.StmtStatic:
		var out []string
		for _, sv := range x.Vars {
			s := sv.(*ast.StmtStaticVar)
			name := varName(s.Var)
			l := f.lookupVar(name)
			if l == nil {
				continue
			}
			// A package-level variable keeps its value between calls.
			pkgVar := f.cv.uniqueName(camel(f.funcName()) + pascal(name))
			init := f.zero(l.t)
			if s.Expr != nil {
				init = f.coerce(f.expr(s.Expr, l.t), l.t)
			}
			if !f.dry {
				f.cv.staticVars = append(f.cv.staticVars, fmt.Sprintf("var %s %s = %s", pkgVar, f.typeStr(l.t), strings.ReplaceAll(init, "\n", " ")))
				f.cv.staticImports = append(f.cv.staticImports, f.imports)
			}
			l.goName = pkgVar
			l.param = true
			_ = out
		}
		return strings.Join(out, "\n")
	case *ast.StmtGlobal:
		return f.todo(n, "global variables aren't supported")
	case *ast.StmtNop, *ast.StmtInlineHtml, *ast.StmtDeclare, *ast.StmtHaltCompiler, *ast.StmtUseList, *ast.StmtGroupUseList,
		*ast.StmtClass, *ast.StmtInterface, *ast.StmtTrait, *ast.StmtEnum, *ast.StmtNamespace:
		return ""
	case *ast.StmtFunction:
		return f.todo(n, "nested function %s() isn't supported", identValue(x.Name))
	case *ast.StmtConstList:
		return f.todo(n, "const declarations inside functions aren't supported")
	case *ast.StmtLabel, *ast.StmtGoto:
		return f.todo(n, "goto isn't supported")
	}
	return f.todo(n, "unsupported statement %T", n)
}

func (f *fctx) funcName() string {
	name := "fn"
	if f.m != nil {
		name = f.m.Name
		if f.cls != nil {
			name = f.cls.GoName + pascal(name)
		}
	}
	return name
}

// exprStmt converts an expression used as a statement.
func (f *fctx) exprStmt(e ast.Vertex) string {
	switch x := e.(type) {
	case *ast.ExprAssign:
		return f.assignStmt(x.Var, x.Expr)
	case *ast.ExprAssignReference:
		return f.assignStmt(x.Var, x.Expr)
	case *ast.ExprAssignConcat, *ast.ExprAssignPlus, *ast.ExprAssignMinus, *ast.ExprAssignMul, *ast.ExprAssignDiv,
		*ast.ExprAssignMod, *ast.ExprAssignPow, *ast.ExprAssignBitwiseAnd, *ast.ExprAssignBitwiseOr,
		*ast.ExprAssignBitwiseXor, *ast.ExprAssignShiftLeft, *ast.ExprAssignShiftRight, *ast.ExprAssignCoalesce:
		return f.compoundStmt(e)
	case *ast.ExprPreInc:
		return f.incDecStmt(x.Var, "++")
	case *ast.ExprPostInc:
		return f.incDecStmt(x.Var, "++")
	case *ast.ExprPreDec:
		return f.incDecStmt(x.Var, "--")
	case *ast.ExprPostDec:
		return f.incDecStmt(x.Var, "--")
	case *ast.ExprBrackets:
		return f.exprStmt(x.Expr)
	case *ast.ExprBinaryBooleanAnd, *ast.ExprBinaryLogicalAnd:
		// $ok && doSomething();
		var l, r ast.Vertex
		if y, ok := x.(*ast.ExprBinaryBooleanAnd); ok {
			l, r = y.Left, y.Right
		} else {
			y := x.(*ast.ExprBinaryLogicalAnd)
			l, r = y.Left, y.Right
		}
		return "if " + f.cond(f.expr(l, api.Bool)) + " {\n" + f.exprStmt(r) + "\n}"
	case *ast.ExprBinaryBooleanOr, *ast.ExprBinaryLogicalOr:
		// $ok || die();
		var l, r ast.Vertex
		if y, ok := x.(*ast.ExprBinaryBooleanOr); ok {
			l, r = y.Left, y.Right
		} else {
			y := x.(*ast.ExprBinaryLogicalOr)
			l, r = y.Left, y.Right
		}
		c := f.expr(l, api.Bool)
		return "if !(" + f.cond(c) + ") {\n" + f.exprStmt(r) + "\n}"
	case *ast.ExprTernary:
		if x.IfTrue != nil {
			return "if " + f.cond(f.expr(x.Cond, api.Bool)) + " {\n" + f.exprStmt(x.IfTrue) + "\n} else {\n" + f.exprStmt(x.IfFalse) + "\n}"
		}
	}
	saved := f.stmtNode
	f.stmtNode = e
	v := f.expr(e, api.Void)
	f.stmtNode = saved
	if v.code == "" {
		return ""
	}
	if strings.Contains(v.code, f.phpx("Unsupported")+"(") && strings.HasPrefix(strings.TrimSpace(reTodo.ReplaceAllString(v.code, "")), f.phpx("Unsupported")+"(") {
		// The result isn't used: skip the statement (with a warning) rather than stop.
		return strings.Replace(v.code, f.phpx("Unsupported")+"(", f.phpx("Skipped")+"(", 1)
	}
	if v.call || v.t != nil && v.t.IsVoid() {
		return v.code
	}
	return "_ = " + v.code
}

var reTodo = regexp.MustCompile(`^/\* TODO\(phar2go\):.*?\*/\s*`)

func (f *fctx) ifStmt(x *ast.StmtIf) string {
	var sb strings.Builder
	sb.WriteString("if " + f.cond(f.expr(x.Cond, api.Bool)) + " {\n" + f.block(stmtList(x.Stmt)) + "\n}")
	for _, ei := range x.ElseIf {
		e := ei.(*ast.StmtElseIf)
		sb.WriteString(" else if " + f.cond(f.expr(e.Cond, api.Bool)) + " {\n" + f.block(stmtList(e.Stmt)) + "\n}")
	}
	if x.Else != nil {
		e := x.Else.(*ast.StmtElse)
		if inner, ok := e.Stmt.(*ast.StmtIf); ok {
			sb.WriteString(" else {\n" + f.ifStmt(inner) + "\n}")
		} else {
			sb.WriteString(" else {\n" + f.block(stmtList(e.Stmt)) + "\n}")
		}
	}
	return sb.String()
}

// loopStmt pushes a loop while converting it, and adds its label if needed.
func (f *fctx) loopStmt(gen func(l *loop) string) string {
	l := &loop{tryDepth: f.tryDepth}
	f.loops = append(f.loops, l)
	code := gen(l)
	f.loops = f.loops[:len(f.loops)-1]
	if l.needLabel {
		return l.label + ":\n" + code
	}
	return code
}

func (f *fctx) forStmt(x *ast.StmtFor) string {
	var pre []string
	for _, e := range x.Init {
		pre = append(pre, f.exprStmt(e))
	}
	code := f.loopStmt(func(l *loop) string {
		cond := ""
		for i, c := range x.Cond {
			v := f.expr(c, api.Bool)
			if i == len(x.Cond)-1 {
				cond = f.cond(v)
			}
		}
		var posts []string
		for _, e := range x.Loop {
			posts = append(posts, f.exprStmt(e))
		}
		body := f.block(stmtList(x.Stmt))
		post := ""
		switch {
		case len(posts) == 1 && !strings.Contains(posts[0], "\n") && !strings.HasPrefix(posts[0], "if ") && !strings.Contains(posts[0], ":="):
			post = posts[0]
		case len(posts) > 0:
			post = "func() {\n" + strings.Join(posts, "\n") + "\n}()"
		}
		if cond == "" && post == "" {
			return "for {\n" + body + "\n}"
		}
		return "for ; " + cond + "; " + post + " {\n" + body + "\n}"
	})
	if len(pre) > 0 {
		return strings.Join(pre, "\n") + "\n" + code
	}
	return code
}

func (f *fctx) foreachStmt(x *ast.StmtForeach) string {
	c := f.expr(x.Expr, nil)
	kt, vt := iterTypes(c.t)
	return f.loopStmt(func(l *loop) string {
		e := f.newTmp("e")
		var header string
		var keyCode, valCode string
		if api.Identical(c.t, generatorT) {
			// Lazy: the generator runs as the loop goes.
			gv := f.newTmp("g")
			header = "for " + gv + " := " + c.code + "; " + gv + ".Valid(); " + gv + ".Next() {"
			var lines []string
			if x.Key != nil {
				lines = append(lines, f.assignValue(x.Key, value{code: gv + ".Key()", t: api.Any, prec: 7}))
			}
			lines = append(lines, f.assignValue(x.Var, value{code: gv + ".Current()", t: api.Any, prec: 7}))
			lines = append(lines, f.block(stmtList(x.Stmt)))
			return header + "\n" + strings.Join(lines, "\n") + "\n}"
		}
		switch {
		case isArrayT(c.t):
			header = "for _, " + e + " := range " + paren(c, 7) + ".Entries() {"
			keyCode, valCode = e+".Key", e+".Val"
			if !isAny(vt) {
				valCode = f.phpx("As") + "[" + f.typeStr(vt) + "](" + e + ".Val)"
			}
			if !isAny(kt) {
				keyCode = f.phpx("As") + "[" + f.typeStr(kt) + "](" + e + ".Key)"
			}
		case c.t != nil && (c.t.K == api.KSlice || c.t.K == api.KArray || c.t.K == api.KMap):
			k := f.newTmp("k")
			if x.Key == nil {
				k = "_"
			}
			header = "for " + k + ", " + e + " := range " + c.code + " {"
			keyCode, valCode = k, e
		default:
			header = "for _, " + e + " := range " + f.phpx("Iter") + "(" + c.code + ") {"
			keyCode, valCode = e+".Key", e+".Val"
		}
		var lines []string
		if x.Key != nil {
			lines = append(lines, f.assignValue(x.Key, value{code: keyCode, t: kt, prec: 7}))
		}
		lines = append(lines, f.assignValue(x.Var, value{code: valCode, t: vt, prec: 7}))
		body := f.block(stmtList(x.Stmt))
		lines = append(lines, body)
		if x.AmpersandTkn != nil {
			if isArrayT(c.t) {
				cur := f.expr(x.Var, nil)
				lines = append(lines, paren(c, 7)+".Set("+e+".Key, "+f.arrayElemCode(cur)+")")
			} else {
				f.warn(x, "foreach by reference over a value that isn't a PHP array: changes aren't written back")
			}
		}
		return header + "\n" + strings.Join(lines, "\n") + "\n}"
	})
}

// endsFlow reports whether PHP case statements end with break/return/continue/throw.
func endsFlow(stmts []ast.Vertex) bool {
	if len(stmts) == 0 {
		return false
	}
	switch x := stmts[len(stmts)-1].(type) {
	case *ast.StmtBreak, *ast.StmtReturn, *ast.StmtContinue, *ast.StmtThrow:
		return true
	case *ast.StmtExpression:
		_, isExit := x.Expr.(*ast.ExprExit)
		_, isThrow := x.Expr.(*ast.ExprThrow)
		return isExit || isThrow
	case *ast.StmtStmtList:
		return endsFlow(x.Stmts)
	}
	return false
}

func (f *fctx) switchStmt(x *ast.StmtSwitch) string {
	subj := f.expr(x.Cond, nil)
	l := &loop{isSwitch: true, tryDepth: f.tryDepth}
	f.loops = append(f.loops, l)
	defer func() { f.loops = f.loops[:len(f.loops)-1] }()

	type group struct {
		conds []ast.Vertex
		isDef bool
		stmts []ast.Vertex
	}
	var groups []*group
	cur := &group{}
	for _, cs := range x.Cases {
		switch c := cs.(type) {
		case *ast.StmtCase:
			cur.conds = append(cur.conds, c.Cond)
			if len(c.Stmts) > 0 {
				cur.stmts = c.Stmts
				groups = append(groups, cur)
				cur = &group{}
			}
		case *ast.StmtDefault:
			cur.isDef = true
			if len(c.Stmts) > 0 {
				cur.stmts = c.Stmts
				groups = append(groups, cur)
				cur = &group{}
			}
		}
	}
	if len(cur.conds) > 0 || cur.isDef {
		groups = append(groups, cur)
	}
	// Direct Go switch when the subject and all cases are of the same simple type.
	direct := subj.t != nil && (subj.t.IsString() || subj.t.IsInt())
	switchTrue := subj.code == "true"
	seen := map[string]bool{}
	var caseCodes [][]string
	for _, g := range groups {
		var codes []string
		for _, c := range g.conds {
			v := f.expr(c, subj.t)
			switch {
			case switchTrue:
				codes = append(codes, f.cond(v))
			case direct && v.t != nil && api.Identical(v.t, subj.t):
				if (v.prec == 7 && (strings.HasPrefix(v.code, "\"") || v.konst)) && seen[v.code] {
					continue
				}
				seen[v.code] = true
				codes = append(codes, v.code)
			default:
				direct = false
				codes = append(codes, v.code)
			}
		}
		caseCodes = append(caseCodes, codes)
	}
	tmp := f.newTmp("sw")
	var sb strings.Builder
	switch {
	case switchTrue:
		sb.WriteString("switch {\n")
	case direct:
		sb.WriteString("switch " + subj.code + " {\n")
	default:
		sb.WriteString("switch " + tmp + " := " + subj.code + "; {\n")
		for i, g := range groups {
			var conds []string
			for j, c := range g.conds {
				if j < len(caseCodes[i]) {
					v := f.expr(c, subj.t)
					conds = append(conds, f.phpx("LooseEq")+"("+tmp+", "+v.code+")")
				}
			}
			caseCodes[i] = conds
		}
	}
	for i, g := range groups {
		codes := caseCodes[i]
		switch {
		case g.isDef && len(codes) == 0:
			sb.WriteString("default:\n")
		case g.isDef:
			// "case x: default:" - default covers it.
			sb.WriteString("default:\n")
		case len(codes) == 0:
			sb.WriteString("case false:\n")
		default:
			sb.WriteString("case " + strings.Join(codes, ", ") + ":\n")
		}
		stmts := g.stmts
		// A trailing break is implicit in Go.
		if n := len(stmts); n > 0 {
			if b, ok := stmts[n-1].(*ast.StmtBreak); ok && b.Expr == nil {
				stmts = stmts[:n-1]
			}
		}
		sb.WriteString(f.block(stmts) + "\n")
		if !endsFlow(g.stmts) && i < len(groups)-1 {
			sb.WriteString("fallthrough\n")
		}
	}
	sb.WriteString("}")
	code := sb.String()
	if l.needLabel {
		code = l.label + ":\n" + code
	}
	return code
}

// breakStmt converts break/continue [n].
func (f *fctx) breakStmt(levelN ast.Vertex, isContinue bool, n ast.Vertex) string {
	level := 1
	if levelN != nil {
		if lit, ok := levelN.(*ast.ScalarLnumber); ok {
			fmt.Sscan(string(lit.Value), &level)
		}
	}
	if level < 1 || level > len(f.loops) {
		if f.tryDepth > 0 {
			return f.todo(n, "break/continue out of a try block isn't supported")
		}
		return f.todo(n, "break/continue outside a loop")
	}
	target := f.loops[len(f.loops)-level]
	kw := "break"
	if isContinue && !target.isSwitch {
		kw = "continue"
	}
	if target.tryDepth < f.tryDepth {
		// Leaving a try closure.
		if level != 1 && f.innermostLoop() != target {
			return f.todo(n, "break/continue %d out of a try block", level)
		}
		if kw == "break" {
			return "return 2, nil"
		}
		return "return 3, nil"
	}
	// Is a plain break/continue enough?
	plain := false
	if kw == "break" {
		plain = level == 1
	} else {
		plain = true
		for i := len(f.loops) - 1; i > len(f.loops)-level; i-- {
			if !f.loops[i].isSwitch {
				plain = false
			}
		}
	}
	if plain {
		return kw
	}
	if target.label == "" {
		*f.labelN++
		target.label = fmt.Sprintf("loop%d", *f.labelN)
	}
	target.needLabel = true
	return kw + " " + target.label
}

func (f *fctx) innermostLoop() *loop {
	for i := len(f.loops) - 1; i >= 0; i-- {
		if !f.loops[i].isSwitch {
			return f.loops[i]
		}
	}
	return nil
}

// returnStmt converts return [expr].
func (f *fctx) returnStmt(e ast.Vertex) string {
	if f.tryDepth > 0 {
		if e == nil {
			return "return 1, nil"
		}
		want := f.retType
		if want != nil && want.IsVoid() {
			want = nil
		}
		v := f.expr(e, want)
		if v.t != nil && v.t.IsVoid() {
			return v.code + "\nreturn 1, nil"
		}
		return "return 1, " + f.arrayElemCode(v)
	}
	res := f.results
	hasErr := len(res) > 0 && res[len(res)-1].K == api.KNamed && res[len(res)-1].Name == "error"
	vals := res
	if hasErr {
		vals = res[:len(res)-1]
	}
	if len(vals) == 0 {
		suffix := "return"
		if hasErr {
			suffix = "return nil"
		}
		if e == nil {
			return suffix
		}
		v := f.expr(e, nil)
		if v.call || v.t.IsVoid() {
			return v.code + "\n" + suffix
		}
		return suffix
	}
	var code string
	if e == nil {
		code = f.zero(vals[0])
	} else {
		v := f.expr(e, vals[0])
		if v.t != nil && v.t.IsVoid() {
			return v.code + "\nreturn " + f.zero(vals[0]) + errSuffix(hasErr)
		}
		code = f.storeCode(v, vals[0])
		if f.m != nil && f.m.ByRefRet {
			code = f.coerce(v, vals[0])
		}
	}
	for _, extra := range vals[1:] {
		code += ", " + f.zero(extra)
	}
	return "return " + code + errSuffix(hasErr)
}

func errSuffix(hasErr bool) string {
	if hasErr {
		return ", nil"
	}
	return ""
}

// tryStmt converts try/catch/finally into phpx.Try with closures.
func (f *fctx) tryStmt(x *ast.StmtTry) string {
	f.tryDepth++
	body := f.block(x.Stmts)
	var catches []string
	var finally string
	for _, c := range x.Catches {
		switch y := c.(type) {
		case *ast.StmtCatch:
			var classes []string
			for _, t := range y.Types {
				full := strings.ToLower(f.cv.resolveName(f.file, t))
				if c := f.cv.classes[full]; c != nil {
					full = strings.ToLower(c.FQCN)
				}
				classes = append(classes, quote(full))
			}
			e := f.newTmp("ex")
			var lines []string
			if y.Var != nil {
				t := throwableT
				if len(y.Types) == 1 {
					t = f.cv.classRef(f.cv.resolveName(f.file, y.Types[0]), f.cls)
				}
				lines = append(lines, f.assignValue(y.Var, value{code: f.coerceCatch(e, t), t: t, prec: 7}))
			} else {
				lines = append(lines, "_ = "+e)
			}
			lines = append(lines, f.block(y.Stmts))
			code := strings.Join(lines, "\n")
			if !terminates(code) {
				code += "\nreturn 0, nil"
			}
			catches = append(catches, fmt.Sprintf("%s{Classes: []string{%s}, Body: func(%s %s) (int, any) {\n%s\n}}",
				f.phpx("Catch"), strings.Join(classes, ", "), e, f.phpx("Throwable"), code))
		case *ast.StmtFinally:
			f.tryDepth--
			finally = "func() {\n" + f.block(y.Stmts) + "\n}"
			f.tryDepth++
		}
	}
	f.tryDepth--
	if !terminates(body) {
		body += "\nreturn 0, nil"
	}
	if finally == "" {
		finally = "nil"
	}
	call := fmt.Sprintf("%s(func() (int, any) {\n%s\n}, %s", f.phpx("Try"), body, finally)
	for _, c := range catches {
		call += ",\n" + c
	}
	call += ")"
	// What to do with the control code.
	var handle []string
	ret := f.returnFromTry()
	if ret != "" {
		handle = append(handle, "if ctl == 1 {\n"+ret+"\n}")
	}
	if l := f.innermostLoop(); l != nil {
		if l.tryDepth < f.tryDepth {
			handle = append(handle, "if ctl == 2 || ctl == 3 {\nreturn ctl, nil\n}")
		} else {
			handle = append(handle, "if ctl == 2 {\nbreak\n}", "if ctl == 3 {\ncontinue\n}")
		}
	}
	if !strings.Contains(body+strings.Join(catches, ""), "return 1,") && !strings.Contains(body+strings.Join(catches, ""), "return 2,") && !strings.Contains(body+strings.Join(catches, ""), "return 3,") {
		return call
	}
	return "if ctl, rv := " + call + "; ctl != 0 {\n_ = rv\n" + strings.Join(handle, "\n") + "\n}"
}

// coerceCatch converts the caught throwable for the catch variable's type.
func (f *fctx) coerceCatch(e string, t *api.Type) string {
	if api.Identical(t, throwableT) || isAny(t) {
		return e
	}
	return f.phpx("As") + "[" + f.typeStr(t) + "](" + e + ")"
}

// returnFromTry is the code returning the value rv that a try closure returned.
func (f *fctx) returnFromTry() string {
	if f.tryDepth > 0 {
		return "return 1, rv"
	}
	res := f.results
	hasErr := len(res) > 0 && res[len(res)-1].K == api.KNamed && res[len(res)-1].Name == "error"
	vals := res
	if hasErr {
		vals = res[:len(res)-1]
	}
	if len(vals) == 0 {
		if hasErr {
			return "return nil"
		}
		return "return"
	}
	code := f.coerce(value{code: "rv", t: api.Any, prec: 7}, vals[0])
	for _, extra := range vals[1:] {
		code += ", " + f.zero(extra)
	}
	return "return " + code + errSuffix(hasErr)
}

func (f *fctx) unset(v ast.Vertex) string {
	switch x := v.(type) {
	case *ast.ExprVariable:
		cur := f.expr(x, nil)
		return cur.code + " = " + f.zero(cur.t)
	case *ast.ExprArrayDimFetch:
		base := f.expr(x.Var, nil)
		k := f.expr(x.Dim, nil)
		switch {
		case isArrayT(base.t):
			return paren(base, 7) + ".Unset(" + k.code + ")"
		case base.t != nil && base.t.K == api.KMap:
			return "delete(" + base.code + ", " + f.coerce(k, base.t.Key) + ")"
		}
		return f.phpx("UnsetIndex") + "(" + base.code + ", " + k.code + ")"
	case *ast.ExprPropertyFetch:
		cur := f.expr(x, nil)
		if cur.lvalue {
			return cur.code + " = " + f.zero(cur.t)
		}
	}
	return f.todo(v, "unsupported unset()")
}

// closure converts a closure or arrow function.
func (f *fctx) closure(params []ast.Vertex, uses []ast.Vertex, retNode ast.Vertex, stmts []ast.Vertex, arrow ast.Vertex, want *api.Type, n ast.Vertex) value {
	g := f.child()
	g.loops = nil
	g.tryDepth = 0
	// Parameters.
	var ps []*param
	for _, pv := range params {
		p := pv.(*ast.Parameter)
		pa := &param{Name: varName(p.Var), TypeNode: p.Type, Default: p.DefaultValue, ByRef: p.AmpersandTkn != nil, Variadic: p.VariadicTkn != nil}
		pa.Type = f.cv.phpType(f.file, p.Type, "", f.cls)
		if pa.Type == nil || pa.Type.IsVoid() {
			pa.Type = api.Any
		}
		ps = append(ps, pa)
	}
	isGen := arrow == nil && containsYield(stmts)
	var sig *api.Func
	if isGen && (want == nil || want.K != api.KFunc) {
		sig = &api.Func{Results: []*api.Type{generatorT}}
		for _, p := range ps {
			sig.Params = append(sig.Params, p.Type)
		}
	} else if want != nil && want.K == api.KFunc {
		sig = &api.Func{Params: want.Params, Results: want.Results, Variadic: want.Variadic}
	} else {
		sig = &api.Func{}
		for _, p := range ps {
			sig.Params = append(sig.Params, p.Type)
		}
		ret := f.cv.phpType(f.file, retNode, "", f.cls)
		switch {
		case ret != nil && !ret.IsVoid():
			sig.Results = []*api.Type{ret}
		case ret != nil:
		case arrow != nil:
			sig.Results = []*api.Type{api.Any}
		case returnsValue(stmts):
			sig.Results = []*api.Type{api.Any}
		}
	}
	g.results = sig.Results
	if len(sig.Results) > 0 {
		g.retType = sig.Results[0]
	} else {
		g.retType = api.Void
	}
	// Captured variables.
	type capture struct {
		name  string
		outer *local
	}
	var byVal []capture
	addCapture := func(name string, byRef bool) {
		outer := f.lookupVar(name)
		if outer == nil {
			outer = f.addVar(name)
		}
		v := &local{name: name, goName: outer.goName, t: outer.t, param: true, fixed: true, ptr: outer.ptr}
		g.vars[name] = v
		g.order = append(g.order, name)
		if !byRef {
			byVal = append(byVal, capture{name, outer})
		}
	}
	for _, u := range uses {
		cu, ok := u.(*ast.ExprClosureUse)
		if !ok {
			continue
		}
		addCapture(varName(cu.Var), cu.AmpersandTkn != nil)
	}
	inner := map[string]bool{}
	for _, p := range ps {
		inner[p.Name] = true
	}
	if arrow != nil {
		walk(arrow, func(m ast.Vertex) bool {
			if _, isArrow := m.(*ast.ExprArrowFunction); isArrow && m != arrow {
				return true
			}
			if name := varName(m); name != "" && name != "this" && !inner[name] && g.vars[name] == nil {
				if f.lookupVar(name) != nil {
					addCapture(name, false)
				}
			}
			return true
		})
	}
	// Go parameters.
	var goParams []string
	var prologue []string
	for i, pt := range sig.Params {
		goT := pt
		if sig.Variadic && i == len(sig.Params)-1 {
			goT = pt.Elem
		}
		if i >= len(ps) {
			goParams = append(goParams, "_ "+g.paramTypeStr(pt, sig.Variadic && i == len(sig.Params)-1))
			continue
		}
		p := ps[i]
		lv := g.addVar(p.Name)
		lv.param, lv.fixed = true, true
		phpT := p.Type
		if isAny(phpT) {
			phpT = goT
		}
		if sig.Variadic && i == len(sig.Params)-1 {
			lv.t = arrayT(api.Int, goT)
			goName := lv.goName + "Args"
			goParams = append(goParams, goName+" "+g.paramTypeStr(pt, true))
			prologue = append(prologue, lv.goName+" := "+g.phpx("ToArray")+"("+goName+")")
			continue
		}
		if api.Identical(phpT, pt) || f.cv.assignable(pt, phpT) && !isArrayT(phpT) || isAny(p.Type) {
			lv.t = pt
			goParams = append(goParams, lv.goName+" "+g.typeStr(pt))
			continue
		}
		lv.t = phpT
		goName := lv.goName + "Arg"
		goParams = append(goParams, goName+" "+g.typeStr(pt))
		prologue = append(prologue, lv.goName+" := "+g.coerce(value{code: goName, t: pt, prec: 7}, phpT))
	}
	// Extra PHP parameters the Go signature doesn't have.
	for i := len(sig.Params); i < len(ps); i++ {
		p := ps[i]
		lv := g.addVar(p.Name)
		lv.t = p.Type
		lv.fixed = true
		lv.param = true
		init := g.zero(p.Type)
		if p.Default != nil {
			init = g.coerce(g.expr(p.Default, p.Type), p.Type)
		}
		prologue = append(prologue, "var "+lv.goName+" "+g.typeStr(p.Type)+" = "+init)
	}
	var body string
	if arrow != nil {
		g.inferLocals([]ast.Vertex{&ast.StmtReturn{Expr: arrow}})
		if want == nil || want.K != api.KFunc {
			if retNode == nil {
				t := g.typeOf(arrow)
				if t.IsVoid() {
					sig.Results = nil
				} else if !t.IsNil() {
					sig.Results = []*api.Type{t}
				}
				g.results = sig.Results
				if len(sig.Results) > 0 {
					g.retType = sig.Results[0]
				}
			}
		}
		body = g.returnStmt(arrow)
		if len(sig.Results) == 0 {
			v := g.expr(arrow, nil)
			if v.call || v.t.IsVoid() {
				body = v.code
			} else {
				body = "_ = " + v.code
			}
		}
	} else if isGen {
		outerResults := sig.Results
		g.results, g.retType = []*api.Type{api.Any}, api.Any
		g.inferLocals(stmts)
		g.genVar = g.uniqueLocal("gen")
		inner := g.block(stmts)
		innerDecls := g.declareLocals(inner)
		if !terminates(inner) {
			inner += "\nreturn nil"
		}
		genCode := g.phpx("NewGenerator") + "(func(" + g.genVar + " *" + g.phpx("Yielder") + ") any {\n" + strings.Join(append(innerDecls, inner), "\n") + "\n})"
		if len(outerResults) > 0 {
			body = "return " + g.coerce(value{code: genCode, t: generatorT, prec: 7}, outerResults[0])
		} else {
			body = "_ = " + genCode
		}
		g.order = nil
	} else {
		g.inferLocals(stmts)
		body = g.block(stmts)
		if len(sig.Results) > 0 && !terminates(body) && !(endsWithReturn(stmts) && g.tryDepth == 0) {
			body += "\n" + g.returnStmt(nil)
		}
	}
	decls := g.declareLocals(body)
	all := append(prologue, decls...)
	all = append(all, body)
	code := "func(" + strings.Join(goParams, ", ") + ")" + resultsStr(g, sig.Results) + " {\n" + strings.Join(all, "\n") + "\n}"
	t := &api.Type{K: api.KFunc, Params: sig.Params, Results: sig.Results, Variadic: sig.Variadic}
	if len(byVal) > 0 {
		// Capture by value: PHP copies the variables when the closure is created.
		var names, args []string
		for _, c := range byVal {
			ct := c.outer.t
			if ct == nil {
				ct = api.Any
			}
			names = append(names, c.outer.goName+" "+g.typeStr(ct))
			arg := c.outer.goName
			if c.outer.ptr {
				arg = "*" + arg
			}
			if isArrayT(ct) {
				arg = f.phpx("Clone") + "(" + arg + ")"
			}
			args = append(args, arg)
		}
		code = "func(" + strings.Join(names, ", ") + ") " + f.typeStr(t) + " {\nreturn " + code + "\n}(" + strings.Join(args, ", ") + ")"
	}
	return value{code: code, t: t, prec: 7, call: false}
}

func (f *fctx) paramTypeStr(t *api.Type, variadic bool) string {
	if variadic && t.K == api.KSlice {
		return "..." + f.typeStr(t.Elem)
	}
	return f.typeStr(t)
}

func resultsStr(f *fctx, res []*api.Type) string {
	switch len(res) {
	case 0:
		return ""
	case 1:
		return " " + f.typeStr(res[0])
	}
	var rs []string
	for _, r := range res {
		rs = append(rs, f.typeStr(r))
	}
	return " (" + strings.Join(rs, ", ") + ")"
}
