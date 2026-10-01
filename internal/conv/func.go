package conv

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// local is a PHP variable of a function.
type local struct {
	name     string
	goName   string
	t        *api.Type
	assigned []*api.Type
	sawNil   bool
	param    bool
	ptr      bool // a by-reference parameter: the Go variable is a pointer
	arrayMut bool // written with $x[...] = ...
	reads    int
	targets  int
	fixed    bool // the type is fixed (parameters)
	// forced is the type of a by-reference parameter the variable is passed to.
	forced *api.Type
}

// loop is a PHP construct break/continue can target.
type loop struct {
	isSwitch  bool
	label     string
	needLabel bool
	// inTry is the try depth the loop was entered at.
	tryDepth int
}

// fctx is the context of a function, method or closure being converted.
type fctx struct {
	cv     *converter
	cls    *class
	m      *method
	file   *phpFile
	recv   string
	static bool
	parent *fctx

	vars  map[string]*local
	order []string

	// results are the Go results of the function; retType is the PHP return type.
	results []*api.Type
	retType *api.Type
	// namedErr is set when the function has a named "err" result that a deferred
	// phpx.Recover fills.
	namedErr bool

	loops    []*loop
	tryDepth int
	// tryRet is the type of values returned through a try closure.
	imports *importSet
	tmp     *int
	dry     bool
	labelN  *int

	// selfVar is the expression for $this as a value.
	selfVar string
	// stmtNode is the expression being converted as a statement (its value is unused).
	stmtNode ast.Vertex
	// goArgs is set while converting the arguments of a call into the server.
	goArgs bool
	// genVar is the Yielder of the generator body being converted.
	genVar string
}

type importSet struct {
	paths map[string]bool
	// idents are the local names used in the file, which import aliases must not shadow.
	idents map[string]bool
}

func newImportSet() *importSet {
	return &importSet{paths: map[string]bool{}, idents: map[string]bool{}}
}

// pkgRef returns a placeholder for a package-qualified name, resolved when the file is written.
func (f *fctx) pkgRef(path string) string {
	if path == "" {
		return ""
	}
	if !f.dry && f.imports != nil {
		f.imports.paths[path] = true
	}
	return "\x01" + path + "\x02"
}

func (f *fctx) phpx(name string) string { return f.pkgRef(api.PhpxPath) + "." + name }

func (f *fctx) warn(n ast.Vertex, format string, args ...any) {
	if f.dry {
		return
	}
	file := ""
	if f.file != nil {
		file = f.file.Path
		if l := line(n); l > 0 {
			file = fmt.Sprintf("%s:%d", file, l)
		}
	}
	f.cv.warnf(file, format, args...)
}

// todo returns a TODO comment and counts it.
func (f *fctx) todo(n ast.Vertex, format string, args ...any) string {
	msg := fmt.Sprintf(format, args...)
	f.warn(n, "%s", msg)
	if !f.dry {
		f.cv.todos++
	}
	return "/* TODO(phar2go): " + strings.ReplaceAll(msg, "*/", "* /") + " */"
}

func (f *fctx) newTmp(prefix string) string {
	*f.tmp++
	return fmt.Sprintf("%s%d", prefix, *f.tmp)
}

func (f *fctx) child() *fctx {
	c := &fctx{cv: f.cv, cls: f.cls, m: f.m, file: f.file, recv: f.recv, static: f.static, parent: f,
		vars: map[string]*local{}, imports: f.imports, tmp: f.tmp, dry: f.dry, labelN: f.labelN, selfVar: f.selfVar}
	c.goArgs = false
	return c
}

// lookupVar finds a variable in this function (closures see captured variables through their
// own locals).
func (f *fctx) lookupVar(name string) *local {
	return f.vars[name]
}

func (f *fctx) addVar(name string) *local {
	if v, ok := f.vars[name]; ok {
		return v
	}
	v := &local{name: name, goName: f.uniqueLocal(localName(name))}
	if f.imports != nil {
		f.imports.idents[v.goName] = true
	}
	f.vars[name] = v
	f.order = append(f.order, name)
	return v
}

func (f *fctx) uniqueLocal(n string) string {
	taken := func(s string) bool {
		if s == f.recv || s == "phpx" {
			return true
		}
		for _, v := range f.vars {
			if v.goName == s {
				return true
			}
		}
		for p := f.parent; p != nil; p = p.parent {
			if s == p.recv {
				return true
			}
		}
		return false
	}
	name := n
	for i := 2; taken(name); i++ {
		name = fmt.Sprintf("%s%d", n, i)
	}
	return name
}

// mergeTypes combines the types assigned to a variable.
func mergeTypes(ts []*api.Type, sawNil bool) *api.Type {
	var list []*api.Type
	for _, t := range ts {
		if t == nil || t.IsNil() || t.IsVoid() {
			continue
		}
		list = append(list, t)
	}
	if len(list) == 0 {
		return api.Any
	}
	first := list[0]
	same := true
	for _, t := range list[1:] {
		if !api.Identical(t, first) {
			same = false
			break
		}
	}
	if same {
		if sawNil && !first.Nilable() && !isArrayT(first) {
			return api.Any
		}
		return first
	}
	allNum, anyFloat := true, false
	allArr := true
	for _, t := range list {
		if !t.IsNumber() {
			allNum = false
		}
		if t.IsFloat() {
			anyFloat = true
		}
		if !isArrayT(t) && !isGoCollection(t) {
			allArr = false
		}
	}
	if allNum && !sawNil {
		if anyFloat {
			return api.Float
		}
		return api.Int
	}
	if allArr {
		return arrayT(nil, nil)
	}
	return api.Any
}

// inferLocals finds the function's variables and their types.
func (f *fctx) inferLocals(body []ast.Vertex) {
	// Collect every variable name.
	walkAll(body, func(n ast.Vertex) bool {
		switch x := n.(type) {
		case *ast.ExprClosure:
			for _, u := range x.Uses {
				if cu, ok := u.(*ast.ExprClosureUse); ok {
					if name := varName(cu.Var); name != "" && name != "this" {
						f.addVar(name).reads++
					}
				}
			}
			return false
		case *ast.ExprArrowFunction:
			// Variables the arrow function reads from this scope.
			inner := map[string]bool{}
			for _, p := range x.Params {
				inner[varName(p.(*ast.Parameter).Var)] = true
			}
			walk(x.Expr, func(m ast.Vertex) bool {
				if name := varName(m); name != "" && name != "this" && !inner[name] {
					f.addVar(name).reads++
				}
				return true
			})
			return false
		case *ast.StmtClass, *ast.StmtFunction:
			return false
		case *ast.ExprStaticPropertyFetch:
			// Class::$prop is not a variable.
			if _, ok := x.Class.(*ast.ExprVariable); ok {
				f.addVar(varName(x.Class)).reads++
			}
			return false
		case *ast.ExprVariable:
			if name := varName(x); name != "" && name != "this" {
				f.addVar(name).reads++
			}
		}
		return true
	})
	// Types from assignments, a few rounds so types flow between variables.
	for round := 0; round < 3; round++ {
		for _, v := range f.vars {
			if !v.fixed {
				v.assigned = nil
				v.sawNil = false
				v.targets = 0
			}
		}
		f.dry = true
		walkAll(body, func(n ast.Vertex) bool { return f.inferNode(n) })
		f.dry = f.parent != nil && f.parent.dry
		changed := false
		for _, v := range f.vars {
			if v.fixed {
				continue
			}
			t := mergeTypes(v.assigned, v.sawNil)
			if v.forced != nil {
				t = v.forced
			}
			if v.arrayMut && !isArrayT(t) && (t.IsAny() || isGoCollection(t)) {
				t = arrayT(nil, nil)
			}
			if v.t == nil || !api.Identical(v.t, t) {
				changed = true
			}
			v.t = t
		}
		if !changed && round > 0 {
			break
		}
	}
}

func (f *fctx) assignTo(target ast.Vertex, t *api.Type) {
	switch x := target.(type) {
	case *ast.ExprVariable:
		name := varName(x)
		if name == "" || name == "this" {
			return
		}
		v := f.addVar(name)
		v.targets++
		if t == nil {
			t = api.Any
		}
		if t.IsNil() {
			v.sawNil = true
			return
		}
		v.assigned = append(v.assigned, t)
	case *ast.ExprArrayDimFetch:
		if name := varName(x.Var); name != "" && name != "this" {
			v := f.addVar(name)
			v.arrayMut = true
			v.sawNil = v.sawNil || false
			v.assigned = append(v.assigned, arrayT(nil, nil))
			v.targets++
		}
	case *ast.ExprList:
		f.assignList(x.Items, t)
	case *ast.ExprArray:
		f.assignList(x.Items, t)
	}
}

func (f *fctx) assignList(items []ast.Vertex, t *api.Type) {
	_, ev := arrayElem(t)
	for _, it := range items {
		if ai, ok := it.(*ast.ExprArrayItem); ok && ai.Val != nil {
			f.assignTo(ai.Val, ev)
		}
	}
}

// inferNode records the assignments a node makes; it returns whether to look inside it.
func (f *fctx) inferNode(n ast.Vertex) bool {
	switch x := n.(type) {
	case *ast.ExprClosure, *ast.ExprArrowFunction, *ast.StmtClass, *ast.StmtFunction:
		return false
	case *ast.ExprAssign:
		f.assignTo(x.Var, f.typeOf(x.Expr))
	case *ast.ExprAssignReference:
		f.assignTo(x.Var, f.typeOf(x.Expr))
	case *ast.ExprAssignCoalesce:
		f.assignTo(x.Var, f.typeOf(x.Expr))
		if v := f.vars[varName(x.Var)]; v != nil {
			v.sawNil = true
		}
	case *ast.ExprAssignConcat:
		f.assignTo(x.Var, api.String)
	case *ast.ExprAssignPlus, *ast.ExprAssignMinus, *ast.ExprAssignMul:
		var r ast.Vertex
		switch y := x.(type) {
		case *ast.ExprAssignPlus:
			r = y.Expr
		case *ast.ExprAssignMinus:
			r = y.Expr
		case *ast.ExprAssignMul:
			r = y.Expr
		}
		rt := f.typeOf(r)
		if rt.IsNumber() {
			f.assignTo(assignVar(x), rt)
		} else {
			f.assignTo(assignVar(x), api.Any)
		}
	case *ast.ExprAssignDiv, *ast.ExprAssignPow:
		f.assignTo(assignVar(x), api.Float)
	case *ast.ExprAssignMod, *ast.ExprAssignBitwiseAnd, *ast.ExprAssignBitwiseOr, *ast.ExprAssignBitwiseXor, *ast.ExprAssignShiftLeft, *ast.ExprAssignShiftRight:
		f.assignTo(assignVar(x), api.Int)
	case *ast.ExprPreInc, *ast.ExprPostInc, *ast.ExprPreDec, *ast.ExprPostDec:
		var target ast.Vertex
		switch y := x.(type) {
		case *ast.ExprPreInc:
			target = y.Var
		case *ast.ExprPostInc:
			target = y.Var
		case *ast.ExprPreDec:
			target = y.Var
		case *ast.ExprPostDec:
			target = y.Var
		}
		if v := f.vars[varName(target)]; v != nil && len(v.assigned) == 0 {
			f.assignTo(target, api.Int)
		}
	case *ast.StmtForeach:
		ct := f.typeOf(x.Expr)
		kt, vt := iterTypes(ct)
		if x.Key != nil {
			f.assignTo(x.Key, kt)
		}
		f.assignTo(x.Var, vt)
	case *ast.StmtCatch:
		if x.Var != nil {
			t := throwableT
			if len(x.Types) == 1 {
				t = f.cv.classRef(f.cv.resolveName(f.file, x.Types[0]), f.cls)
			}
			f.assignTo(x.Var, t)
		}
	case *ast.StmtStatic:
		for _, sv := range x.Vars {
			if s, ok := sv.(*ast.StmtStaticVar); ok {
				t := api.Any
				if s.Expr != nil {
					t = f.typeOf(s.Expr)
				}
				f.assignTo(s.Var, t)
			}
		}
	case *ast.StmtGlobal:
		for _, gv := range x.Vars {
			f.assignTo(gv, api.Any)
		}
	case *ast.ExprMethodCall, *ast.ExprStaticCall, *ast.ExprNew:
		f.inferByRef(n)
	case *ast.ExprFunctionCall:
		f.inferByRef(n)
		// By-reference outputs of built-in functions.
		name := strings.ToLower(lastSeg(identValue(x.Function)))
		switch name {
		case "preg_match", "preg_match_all":
			if len(x.Args) > 2 {
				if a, ok := x.Args[2].(*ast.Argument); ok {
					f.assignTo(a.Expr, arrayT(nil, nil))
				}
			}
		case "array_push", "array_unshift", "array_pop", "array_shift", "array_splice", "sort", "rsort", "usort", "uasort", "uksort", "ksort", "krsort", "asort", "arsort", "shuffle", "natsort", "natcasesort", "array_walk":
			if len(x.Args) > 0 {
				if a, ok := x.Args[0].(*ast.Argument); ok {
					if v := f.vars[varName(a.Expr)]; v != nil {
						v.arrayMut = true
					}
				}
			}
		}
	}
	return true
}

func assignVar(n ast.Vertex) ast.Vertex {
	switch x := n.(type) {
	case *ast.ExprAssignPlus:
		return x.Var
	case *ast.ExprAssignMinus:
		return x.Var
	case *ast.ExprAssignMul:
		return x.Var
	case *ast.ExprAssignDiv:
		return x.Var
	case *ast.ExprAssignPow:
		return x.Var
	case *ast.ExprAssignMod:
		return x.Var
	case *ast.ExprAssignBitwiseAnd:
		return x.Var
	case *ast.ExprAssignBitwiseOr:
		return x.Var
	case *ast.ExprAssignBitwiseXor:
		return x.Var
	case *ast.ExprAssignShiftLeft:
		return x.Var
	case *ast.ExprAssignShiftRight:
		return x.Var
	case *ast.ExprAssignConcat:
		return x.Var
	case *ast.ExprAssignCoalesce:
		return x.Var
	}
	return nil
}

// iterTypes returns the key and value types foreach gives for a collection.
func iterTypes(t *api.Type) (*api.Type, *api.Type) {
	switch {
	case t == nil:
		return api.Any, api.Any
	case isArrayT(t):
		return arrayElem(t)
	case t.K == api.KSlice || t.K == api.KArray:
		return api.Int, t.Elem
	case t.K == api.KMap:
		return t.Key, t.Elem
	}
	return api.Any, api.Any
}

// typeOf is the type of an expression, without generating code.
func (f *fctx) typeOf(n ast.Vertex) *api.Type {
	saved := f.dry
	f.dry = true
	v := f.expr(n, nil)
	f.dry = saved
	if v.t == nil {
		return api.Any
	}
	return v.t
}

// declareLocals returns the declarations of the function's variables (parameters excluded).
// body is the converted code, used to find variables that are only assigned.
func (f *fctx) declareLocals(body string) []string {
	var out []string
	var unread []string
	names := append([]string(nil), f.order...)
	for _, name := range names {
		v := f.vars[name]
		if v.param {
			continue
		}
		t := v.t
		if t == nil {
			t = api.Any
		}
		if isArrayT(t) {
			out = append(out, fmt.Sprintf("%s := %s()", v.goName, f.phpx("NewArray")))
		} else {
			out = append(out, fmt.Sprintf("var %s %s", v.goName, f.typeStr(t)))
		}
		if !readsVar(body, v.goName) {
			unread = append(unread, v.goName)
		}
	}
	sort.Strings(unread)
	for _, u := range unread {
		out = append(out, "_ = "+u)
	}
	return out
}

var reStringLit = regexp.MustCompile("\"(\\\\.|[^\"\\\\])*\"|`[^`]*`")

var rePlaceholder = regexp.MustCompile("\x01[^\x02]*\x02")

var reComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// readsVar reports whether Go code reads variable name (uses it other than as the left side of
// a plain assignment).
func readsVar(code, name string) bool {
	code = rePlaceholder.ReplaceAllString(code, "pkg")
	code = reStringLit.ReplaceAllString(code, `""`)
	code = reComment.ReplaceAllString(code, "")
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.])` + regexp.QuoteMeta(name) + `\b(\s*=[^=])?`)
	for _, m := range re.FindAllStringSubmatch(code, -1) {
		if m[2] == "" {
			return true
		}
	}
	return false
}

// inferByRef gives variables passed to by-reference parameters of plugin methods the
// parameter's type.
func (f *fctx) inferByRef(n ast.Vertex) {
	var m *method
	var args []ast.Vertex
	switch x := n.(type) {
	case *ast.ExprFunctionCall:
		switch x.Function.(type) {
		case *ast.Name, *ast.NameFullyQualified, *ast.NameRelative:
			if fn, ok := f.cv.funcs[strings.ToLower(lastSeg(identValue(x.Function)))]; ok {
				m = fn.m
			}
		}
		args = x.Args
	case *ast.ExprStaticCall:
		if c, _, _ := f.staticClass(x.Class); c != nil {
			m = c.findMethod(identValue(x.Call))
		}
		args = x.Args
	case *ast.ExprMethodCall:
		var c *class
		if varName(x.Var) == "this" {
			c = f.cls
		} else {
			c = f.cv.localClassOf(f.typeOf(x.Var))
		}
		if c != nil {
			m = c.findMethodIfaces(identValue(x.Method))
		}
		args = x.Args
	case *ast.ExprNew:
		if name := identValue(x.Class); name != "" {
			if c := f.cv.classes[strings.ToLower(f.cv.resolveName(f.file, x.Class))]; c != nil {
				m = c.constructor()
			}
		}
		args = x.Args
	}
	if m == nil {
		return
	}
	for i, a := range phpArgs(args) {
		if i < len(m.Params) && m.Params[i].ByRef && m.Params[i].Type != nil {
			if v := f.vars[varName(a.expr)]; v != nil && !v.fixed {
				f.assignTo(a.expr, m.Params[i].Type)
				v.forced = m.Params[i].Type
			}
		}
	}
}
