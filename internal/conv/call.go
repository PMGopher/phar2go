package conv

import (
	"fmt"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// argList returns the PHP arguments of a call.
type phpArg struct {
	expr   ast.Vertex
	name   string
	spread bool
	node   ast.Vertex
}

func phpArgs(args []ast.Vertex) []phpArg {
	var out []phpArg
	for _, a := range args {
		arg, ok := a.(*ast.Argument)
		if !ok {
			continue
		}
		pa := phpArg{expr: arg.Expr, spread: arg.VariadicTkn != nil, node: a}
		if arg.Name != nil {
			pa.name = identValue(arg.Name)
		}
		out = append(out, pa)
	}
	return out
}

// callArgs converts the arguments of a call to a function with signature sig. m is the plugin
// method being called (for default values and by-reference parameters), or nil.
func (f *fctx) callArgs(sig *api.Func, args []ast.Vertex, m *method) string {
	list := phpArgs(args)
	n := len(sig.Params)
	// Named arguments.
	if len(sig.ParamNames) > 0 {
		named := false
		for _, a := range list {
			if a.name != "" {
				named = true
			}
		}
		if named {
			ordered := make([]phpArg, 0, n)
			var pos []phpArg
			byName := map[string]phpArg{}
			for _, a := range list {
				if a.name == "" {
					pos = append(pos, a)
				} else {
					byName[a.name] = a
				}
			}
			for i, pn := range sig.ParamNames {
				if i < len(pos) {
					ordered = append(ordered, pos[i])
					continue
				}
				if a, ok := byName[pn]; ok {
					ordered = append(ordered, a)
					continue
				}
				ordered = append(ordered, phpArg{})
			}
			// Trim trailing missing.
			for len(ordered) > 0 && ordered[len(ordered)-1].expr == nil {
				ordered = ordered[:len(ordered)-1]
			}
			list = ordered
		}
	}
	var out []string
	for i := 0; i < n; i++ {
		pt := sig.Params[i]
		variadic := sig.Variadic && i == n-1
		if variadic {
			et := pt.Elem
			if et == nil {
				et = api.Any
			}
			// A spread among several arguments: build the slice.
			hasSpread := false
			for j := i; j < len(list); j++ {
				if list[j].spread {
					hasSpread = true
				}
			}
			if hasSpread && len(list)-i > 1 {
				var parts []string
				for j := i; j < len(list); j++ {
					v := f.expr(list[j].expr, nil)
					if list[j].spread {
						parts = append(parts, f.phpx("Spread")+"("+v.code+")")
					} else {
						parts = append(parts, f.arrayElemCode(v))
					}
				}
				out = append(out, f.phpx("ToSlice")+"["+f.typeStr(et)+"]("+f.phpx("Args")+"("+strings.Join(parts, ", ")+"))...")
				break
			}
			for j := i; j < len(list); j++ {
				a := list[j]
				if a.spread {
					v := f.expr(a.expr, nil)
					out = append(out, f.coerce(v, pt)+"...")
					continue
				}
				out = append(out, f.argCode(a.expr, et, m, j))
			}
			break
		}
		if i < len(list) && list[i].expr != nil {
			a := list[i]
			if a.spread {
				v := f.expr(a.expr, nil)
				out = append(out, f.todo(a.node, "spread arguments (...$args) to a fixed parameter list")+f.phpx("As")+"["+f.typeStr(pt)+"]("+f.phpx("Index")+"("+v.code+", "+itoa(i)+"))")
				continue
			}
			out = append(out, f.argCode(a.expr, pt, m, i))
			continue
		}
		// A missing argument: the PHP default, or the zero value.
		if m != nil && i < len(m.Params) && m.Params[i].Default != nil {
			out = append(out, f.defaultArg(m, m.Params[i], pt))
			continue
		}
		if def, ok := f.extDefault(i, pt); ok {
			out = append(out, def)
			continue
		}
		out = append(out, f.zero(pt))
	}
	return strings.Join(out, ", ")
}

func (f *fctx) extDefault(i int, pt *api.Type) (string, bool) {
	return "", false
}

// argCode converts one argument for a parameter of type pt.
func (f *fctx) argCode(e ast.Vertex, pt *api.Type, m *method, i int) string {
	byRef := m != nil && i < len(m.Params) && m.Params[i].ByRef
	if byRef {
		return f.refArg(e, pt)
	}
	v := f.expr(e, pt)
	if pt != nil && pt.IsAny() {
		return f.arrayElemCode(v)
	}
	if isArrayT(pt) && isArrayT(v.t) && v.lvalue {
		// PHP passes arrays by value.
		return paren(v, 7) + ".Clone()"
	}
	return f.coerce(v, pt)
}

// refArg passes a variable by reference (as a pointer of type pt).
func (f *fctx) refArg(e ast.Vertex, pt *api.Type) string {
	if name := varName(e); name != "" && name != "this" {
		v := f.lookupVar(name)
		if v == nil {
			v = f.addVar(name)
		}
		if v.ptr {
			return v.goName
		}
		if pt != nil && pt.K == api.KPointer && v.t != nil && !api.Identical(v.t, pt.Elem) {
			f.warn(e, "by-reference argument $%s has type %s but the parameter wants %s", name, f.typeStr(v.t), f.typeStr(pt.Elem))
		}
		return "&" + v.goName
	}
	if pf, ok := e.(*ast.ExprPropertyFetch); ok {
		v := f.expr(pf, nil)
		if v.lvalue {
			return "&" + v.code
		}
	}
	v := f.expr(e, nil)
	return f.todo(e, "by-reference argument that isn't a variable") + f.phpx("Ptr") + "(" + f.coerce(v, pt.Elem) + ")"
}

// defaultArg converts the default value of a parameter of a plugin method, in that method's
// class context.
func (f *fctx) defaultArg(m *method, p *param, pt *api.Type) string {
	g := &fctx{cv: f.cv, cls: m.Class, m: m, file: f.cv.fileOf(m.Class, m), recv: f.recv, static: true,
		vars: map[string]*local{}, imports: f.imports, tmp: f.tmp, dry: f.dry, labelN: f.labelN}
	if g.file == nil {
		g.file = f.file
	}
	want := pt
	if p.ByRef && pt.K == api.KPointer {
		want = pt.Elem
	}
	v := g.expr(p.Default, want)
	code := g.coerce(v, want)
	if p.ByRef {
		if isNilV(v) || code == "nil" {
			return "new(" + f.typeStr(want) + ")"
		}
		return f.phpx("Ptr") + "[" + f.typeStr(want) + "](" + code + ")"
	}
	return code
}

// results turns a call with Go results into a PHP value.
func (f *fctx) callResults(code string, res []*api.Type) value {
	switch len(res) {
	case 0:
		return callv(code, api.Void)
	case 1:
		if res[0].K == api.KNamed && res[0].Name == "error" {
			return callv(f.phpx("Check")+"("+code+")", api.Void)
		}
		return callv(code, res[0])
	case 2:
		switch {
		case res[1].K == api.KNamed && res[1].Name == "error":
			return callv(f.phpx("Must")+"("+code+")", res[0])
		case res[1].IsBool():
			return callv(f.phpx("OkOr")+"("+code+")", res[0])
		}
		return callv(f.phpx("First")+"("+code+")", res[0])
	}
	// Three or more results: keep the first, throw a trailing error.
	var names []string
	for i := range res {
		names = append(names, "r"+itoa(i))
	}
	check := ""
	last := res[len(res)-1]
	if last.K == api.KNamed && last.Name == "error" {
		check = "if " + names[len(names)-1] + " != nil {\n" + f.phpx("Throw") + "(" + names[len(names)-1] + ")\n}\n"
	}
	for i := 1; i < len(names); i++ {
		if check == "" || i < len(names)-1 {
			names[i] = "_"
		}
	}
	return callv(fmt.Sprintf("func() %s {\n%s := %s\n%sreturn r0\n}()", f.typeStr(res[0]), strings.Join(names, ", "), code, check), res[0])
}

// methodCall converts $obj->name(...).
func (f *fctx) methodCall(objN, nameN ast.Vertex, args []ast.Vertex, nullsafe bool, n ast.Vertex) value {
	name := identValue(nameN)
	if name == "" {
		obj := f.expr(objN, nil)
		mn := f.expr(nameN, api.String)
		return callv(f.phpx("Call")+"("+obj.code+", "+f.coerce(mn, api.String)+f.anyArgs(args)+")", api.Any)
	}
	isThis := varName(objN) == "this" && f.cls != nil && !f.static
	var obj value
	if isThis {
		obj = prim(f.recv, api.Ptr(api.Named(f.cls.GoName)))
		if f.cls.Kind == kindTrait {
			obj = prim(f.recv, api.Any)
		}
	} else {
		obj = f.expr(objN, nil)
	}
	if nullsafe {
		tmp := f.newTmp("ns")
		inner := f.callOn(value{code: tmp, t: obj.t, prec: 7}, name, args, false, n)
		t := inner.t
		if t.IsVoid() {
			return callv(fmt.Sprintf("func() { if %s := %s; !%s(%s) { %s } }()", tmp, obj.code, f.phpx("IsNull"), tmp, inner.code), api.Void)
		}
		if !t.Nilable() {
			t = api.Any
		}
		return callv(fmt.Sprintf("func() %s { %s := %s; if %s(%s) { return nil }; return %s }()", f.typeStr(t), tmp, obj.code, f.phpx("IsNull"), tmp, f.coerce(inner, t)), t)
	}
	return f.callOn(obj, name, args, isThis, n)
}

// callOn calls PHP method name on obj.
func (f *fctx) callOn(obj value, name string, args []ast.Vertex, isThis bool, n ast.Vertex) value {
	t := obj.t
	recv := paren(obj, 7)
	if t == nil || isAny(t) || isArrayT(t) || t.IsNil() {
		if strings.EqualFold(name, "__invoke") {
			return callv(f.phpx("Invoke")+"("+obj.code+f.anyArgs(args)+")", api.Any)
		}
		return callv(f.phpx("Call")+"("+obj.code+", "+quote(name)+f.anyArgs(args)+")", api.Any)
	}
	// Plugin classes.
	if c := f.cv.localClassOf(t); c != nil {
		isIface := t.K == api.KNamed && (c.Kind == kindInterface || c.Poly && t.Name == c.IfaceName)
		var m *method
		if c.Kind == kindInterface {
			m = c.findMethodIfaces(name)
		} else {
			m = c.findMethod(name)
			if m == nil && c.Poly {
				// An abstract method of an interface the class implements.
				for _, i := range c.Ifaces {
					if m = i.findMethodIfaces(name); m != nil {
						break
					}
				}
			}
		}
		if m != nil && m.Static {
			return f.callResults(m.GoName+"("+f.callArgs(m.Sig, args, m)+")", m.Sig.Results)
		}
		if m != nil && m.Key != "__construct" {
			switch {
			case isThis && f.cls.Poly && !m.Private && f.selfVar != "":
				recv = f.selfVar
			case isIface && m.Private && c.Poly:
				recv += "." + asMethod(c) + "()"
			}
			return f.methodResult(recv+"."+m.GoName+"("+f.callArgs(m.Sig, args, m)+")", m)
		}
		// A method of the server type the class extends.
		ms := f.cv.localMethods[c.GoName]
		if gn := findMethodName(ms, name); gn != "" {
			if isIface {
				recv += "." + asMethod(c) + "()"
			}
			sig := ms[gn]
			return f.callResults(recv+"."+gn+"("+f.callArgs(sig, args, nil)+")", sig.Results)
		}
		if strings.EqualFold(name, "call") && len(args) == 0 && c.isEvent && !isIface {
			// $event->call(): dispatch to the listeners.
			return callv(f.pkgRef(f.cv.idx.Module+"/pocketmine/event")+".Call("+obj.code+")", api.Void)
		}
		if isIface {
			// Maybe a method of a subclass.
			return callv(f.phpx("Call")+"("+obj.code+", "+quote(name)+f.anyArgs(args)+")", api.Any)
		}
		f.warn(n, "method %s::%s() not found", c.FQCN, name)
		return callv(f.phpx("Call")+"("+obj.code+", "+quote(name)+f.anyArgs(args)+")", api.Any)
	}
	if v, ok := f.specialMethod(obj, name, args); ok {
		return v
	}
	ms := f.cv.methodSet(t)
	var extra []string
	if ti := f.cv.typeInfo(t.Deref()); ti != nil && ti.PHP != "" {
		if target, ok := f.cv.idx.PHPMembers[strings.ToLower(ti.PHP+"::"+name)]; ok {
			extra = append(extra, target[strings.LastIndexByte(target, '.')+1:])
		}
	}
	if gn := findMethodName(ms, name, extra...); gn != "" {
		sig := ms[gn]
		if len(sig.Results) == 0 && n != f.stmtNode && !t.IsAny() {
			// PHP's fluent setters return $this; Go's return nothing.
			tmp := f.newTmp("o")
			return callv(fmt.Sprintf("func() %s {\n%s := %s\n%s.%s(%s)\nreturn %s\n}()", f.typeStr(t), tmp, obj.code, tmp, gn, f.callArgs(sig, args, nil), tmp), t)
		}
		return f.callResults(recv+"."+gn+"("+f.callArgs(sig, args, nil)+")", sig.Results)
	}
	// A getter of a field: $pos->getX() -> pos.X, $loc->getWorld() -> loc.World.
	if len(args) == 0 && len(name) > 3 && strings.EqualFold(name[:3], "get") {
		if ti := f.cv.typeInfo(t.Deref()); ti != nil {
			if ft, ok := ti.Fields[pascal(name[3:])]; ok {
				return value{code: recv + "." + pascal(name[3:]), t: ft, prec: 7}
			}
		}
	}
	// An interface without the method: assert to the concrete type that has it.
	if f.cv.isInterface(t) {
		if ct, gn := f.cv.concreteFor(t, goMethodNames(name)); ct != nil {
			sig := f.cv.methodSet(ct)[gn]
			return f.callResults(recv+".("+f.typeStr(ct)+")."+gn+"("+f.callArgs(sig, args, nil)+")", sig.Results)
		}
	}
	f.warn(n, "method %s() not found on %s in pocketmine-go; it is called dynamically", name, strings.TrimPrefix(f.cv.prettyType(t), "*"))
	return callv(f.phpx("Call")+"("+obj.code+", "+quote(name)+f.anyArgs(args)+")", api.Any)
}

// methodResult is the value of a call to a plugin method (whose Go signature may have been
// adopted from the server).
func (f *fctx) methodResult(code string, m *method) value {
	res := m.Sig.Results
	if m.Adopted {
		return f.callResults(code, res)
	}
	if len(res) == 0 {
		return callv(code, api.Void)
	}
	return callv(code, res[0])
}

// anyArgs converts arguments for a dynamic call (", a, b").
func (f *fctx) anyArgs(args []ast.Vertex) string {
	var out []string
	spreads := 0
	list := phpArgs(args)
	for _, a := range list {
		v := f.expr(a.expr, nil)
		if a.spread {
			spreads++
			out = append(out, f.phpx("Spread")+"("+v.code+")")
			continue
		}
		out = append(out, f.arrayElemCode(v))
	}
	if len(out) == 0 {
		return ""
	}
	if spreads == 1 && len(out) == 1 {
		return ", " + f.phpx("ToArray") + "(" + f.expr(list[0].expr, nil).code + ").Values()..."
	}
	if spreads > 0 {
		return ", " + f.phpx("Args") + "(" + strings.Join(out, ", ") + ")..."
	}
	return ", " + strings.Join(out, ", ")
}

func (cv *converter) prettyType(t *api.Type) string {
	f := &fctx{cv: cv, dry: true}
	s := f.typeStr(t)
	s = strings.ReplaceAll(s, "\x02", "")
	var sb strings.Builder
	for {
		i := strings.IndexByte(s, '\x01')
		if i < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:i])
		s = s[i+1:]
		// Keep the last path element.
		j := strings.IndexByte(s, '.')
		if j < 0 {
			break
		}
		p := s[:j]
		sb.WriteString(p[strings.LastIndexByte(p, '/')+1:])
		s = s[j:]
	}
	return sb.String()
}

// specialMethod handles PHP methods whose Go counterpart differs in shape.
func (f *fctx) specialMethod(obj value, name string, args []ast.Vertex) (value, bool) {
	t := obj.t.Deref()
	switch {
	case t.K == api.KNamed && t.Name == f.cv.idx.Module+"/pocketmine/math.Vector3":
		// Vector3 is a value type: its methods return new values.
	}
	return value{}, false
}

// staticCall converts Class::method(...), parent::method(...) and self::method(...).
func (f *fctx) staticCall(x *ast.ExprStaticCall) value {
	name := identValue(x.Call)
	c, php, kind := f.staticClass(x.Class)
	if name == "" {
		return callv(f.todo(x, "dynamic static method names aren't supported")+f.phpx("Unsupported")+`("static call")`, api.Any)
	}
	lname := strings.ToLower(name)
	switch {
	case kind == "dynamic":
		cls := f.expr(x.Class, nil)
		return callv(f.phpx("Call")+"("+cls.code+", "+quote(name)+f.anyArgs(x.Args)+")", api.Any)
	case kind == "parent" && c == nil:
		return f.parentExtCall(php, name, x.Args, x)
	case c != nil:
		if c.Kind == kindEnum {
			switch lname {
			case "cases":
				return callv(c.GoName+"Cases()", arrayT(api.Int, f.cv.classType(c)))
			case "from":
				return callv(c.GoName+"From("+f.expr(phpArgs(x.Args)[0].expr, nil).code+")", f.cv.classType(c))
			case "tryfrom":
				return callv(c.GoName+"TryFrom("+f.expr(phpArgs(x.Args)[0].expr, nil).code+")", f.cv.classType(c))
			}
		}
		m := c.findMethod(name)
		if m == nil {
			if lname == "__construct" && kind == "parent" {
				// The parent has no constructor of its own: maybe its server parent does.
				if ext, extPHP := c.rootExt(); ext != nil {
					return f.parentExtCall(extPHP, name, x.Args, x)
				}
				if c.Exception {
					return f.parentExtCall("Exception", name, x.Args, x)
				}
				return callv("", api.Void)
			}
			// A server method of the class's parent.
			if kind == "parent" || kind == "self" {
				if ext, extPHP := c.rootExt(); ext != nil {
					return f.parentExtCall(extPHP, name, x.Args, x)
				}
				if c.Exception {
					return f.parentExtCall("Exception", name, x.Args, x)
				}
			}
			return callv(f.todo(x, "method %s::%s() not found", c.FQCN, name)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(x.Args)+")", api.Any)
		}
		if m.Static {
			return f.methodResult(m.GoName+"("+f.callArgs(m.Sig, x.Args, m)+")", m)
		}
		if f.static || f.cls == nil {
			return callv(f.todo(x, "non-static method %s::%s() called statically", c.FQCN, name)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(x.Args)+")", api.Any)
		}
		if m.Key == "__construct" {
			return callv(f.recv+"."+m.GoName+"("+f.callArgs(m.Sig, x.Args, m)+")", api.Void)
		}
		// parent::foo() / self::foo(): call that class's implementation (no virtual dispatch).
		recv := f.recv
		if kind == "parent" || c != f.cls {
			recv += "." + c.GoName
		}
		return f.methodResult(recv+"."+m.GoName+"("+f.callArgs(m.Sig, x.Args, m)+")", m)
	}
	return f.extStaticCall(php, name, x.Args, x)
}

// parentExtCall converts parent::method() where the parent is a server class.
func (f *fctx) parentExtCall(php, name string, args []ast.Vertex, n ast.Vertex) value {
	lname := strings.ToLower(name)
	if f.cls != nil && (f.cls.Exception || isThrowableClass(php)) && f.cls.ExtParent == nil {
		if lname == "__construct" {
			list := phpArgs(args)
			var parts []string
			for _, a := range list {
				parts = append(parts, f.expr(a.expr, nil).code)
			}
			if len(parts) == 0 {
				parts = append(parts, `""`)
			}
			return callv(f.recv+".InitException("+quote(f.cls.FQCN)+", "+strings.Join(parts, ", ")+")", api.Void)
		}
		ms := f.cv.methodSet(exceptionT)
		if gn := findMethodName(ms, name); gn != "" {
			return f.callResults(f.recv+".Exception."+gn+"("+f.callArgs(ms[gn], args, nil)+")", ms[gn].Results)
		}
	}
	var embed *api.Type
	field := f.recv
	for k := f.cls; k != nil; k = k.Parent {
		if k.ExtEmbed != nil {
			embed = k.ExtEmbed
			break
		}
		if k.Parent != nil {
			field += "." + k.Parent.GoName
		}
	}
	if embed == nil {
		return callv(f.todo(n, "parent::%s() of %s has no Go counterpart", name, php)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(args)+")", api.Any)
	}
	field += "." + embed.ObjName()
	if lname == "__construct" {
		code, ok := f.extInit(php, embed, args, n)
		if !ok {
			return callv("", api.Void)
		}
		return value{code: field + " = " + code, t: api.Void, prec: 1, call: true}
	}
	ms := f.cv.methodSet(api.Ptr(embed))
	if gn := findMethodName(ms, name); gn != "" {
		return f.callResults(field+"."+gn+"("+f.callArgs(ms[gn], args, nil)+")", ms[gn].Results)
	}
	switch lname {
	case "onenable", "ondisable", "onload", "onrun", "oncancel", "oncompletion":
		// Optional hooks with no base implementation.
		return callv("", api.Void)
	}
	return callv(f.todo(n, "parent::%s() not found on %s", name, php)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(args)+")", api.Any)
}

// extInit finds how to initialise an embedded server type (parent::__construct).
func (f *fctx) extInit(php string, embed *api.Type, args []ast.Vertex, n ast.Vertex) (string, bool) {
	pkg := f.cv.idx.Packages[embed.PkgPath()]
	if pkg == nil {
		return "", false
	}
	tn := embed.ObjName()
	var cands []string
	if target, ok := f.cv.idx.PHPMembers[strings.ToLower(php+"::__construct")]; ok && !strings.Contains(strings.TrimPrefix(target, pkg.Path+"."), ".") {
		cands = append(cands, strings.TrimPrefix(target, pkg.Path+"."))
	}
	cands = append(cands, "Init"+tn, "New"+tn+"Base", "New"+tn)
	for _, c := range cands {
		fn, ok := pkg.Funcs[c]
		if !ok || len(fn.Results) == 0 || fn.TypeParams > 0 {
			continue
		}
		r := fn.Results[0]
		code := f.pkgRef(pkg.Path) + "." + c + "(" + f.callArgs(fn, args, nil) + ")"
		if len(fn.Results) == 2 && fn.Results[1].Name == "error" {
			code = f.phpx("Must") + "(" + code + ")"
		}
		switch {
		case api.Identical(r, embed):
			return code, true
		case r.K == api.KPointer && api.Identical(r.Elem, embed):
			return "*" + code, true
		}
	}
	if len(phpArgs(args)) > 0 {
		f.warn(n, "parent::__construct() of %s: no Go constructor found; its arguments are ignored", php)
	}
	return "", false
}

// extStaticCall converts a static call on a server class.
func (f *fctx) extStaticCall(php, name string, args []ast.Vertex, n ast.Vertex) value {
	lname := strings.ToLower(name)
	key := strings.ToLower(php + "::" + name)
	if target, ok := f.cv.idx.PHPMembers[key]; ok {
		if fn := f.cv.idx.Func(target); fn != nil && fn.TypeParams == 0 {
			i := strings.LastIndexByte(target, '.')
			return f.callResults(f.pkgRef(target[:i])+"."+target[i+1:]+"("+f.callArgs(fn, args, nil)+")", fn.Results)
		}
	}
	pkg := f.cv.extPackage(php)
	short := lastSeg(php)
	if pkg == nil {
		if isThrowableClass(php) {
			return callv(f.todo(n, "static call %s::%s()", php, name)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(args)+")", api.Any)
		}
		f.cv.external[php] = true
		return callv(f.todo(n, "class %s doesn't exist in pocketmine-go", php)+f.phpx("Unsupported")+"("+quote(php+"::"+name)+f.anyArgs(args)+")", api.Any)
	}
	ref := f.pkgRef(pkg.Path) + "."
	typeName := short
	if t := f.cv.extClassType(php); t != nil {
		typeName = t.Deref().ObjName()
	}
	// Registries: VanillaItems::DIAMOND() -> item.VanillaItem("diamond").
	if strings.HasPrefix(short, "Vanilla") && isUpperName(name) && len(phpArgs(args)) == 0 {
		if fn, ok := pkg.Funcs["Vanilla"+pascal(strings.ToLower(name))]; ok && len(fn.Params) == 0 && len(fn.Results) > 0 {
			return callv(ref+"Vanilla"+pascal(strings.ToLower(name))+"()", fn.Results[0])
		}
		singular := strings.TrimSuffix(short, "s")
		for _, cand := range []string{singular, strings.TrimSuffix(singular, "e"), "Get" + singular} {
			if fn, ok := pkg.Funcs[cand]; ok && len(fn.Params) == 1 && fn.Params[0].IsString() && len(fn.Results) > 0 {
				return f.callResults(ref+cand+"("+quote(strings.ToLower(name))+")", fn.Results)
			}
		}
	}
	// Enum-like constants as methods: GameMode::CREATIVE().
	if isUpperName(strings.ReplaceAll(name, "_", "")) && len(phpArgs(args)) == 0 {
		if v, ok := f.extConst(php, name); ok {
			return v
		}
	}
	// Singletons.
	if lname == "getinstance" {
		for _, cand := range []string{"Get" + typeName, typeName + "Instance", "Get" + short, "Instance", "Get"} {
			if fn, ok := pkg.Funcs[cand]; ok && len(fn.Params) == 0 && len(fn.Results) > 0 {
				return f.callResults(ref+cand+"()", fn.Results)
			}
		}
		if lname == "getinstance" && strings.EqualFold(php, `pocketmine\Server`) {
			if fn, ok := pkg.Funcs["GetInstance"]; ok {
				return f.callResults(ref+"GetInstance()", fn.Results)
			}
			return callv(f.todo(n, "Server::getInstance() has no equivalent; pass the server from your plugin (GetServer())")+f.phpx("Unsupported")+`("Server::getInstance")`, api.Any)
		}
	}
	cands := []string{typeName + pascal(name), pascal(name) + typeName, short + pascal(name), pascal(name)}
	if strings.HasPrefix(lname, "create") || strings.HasPrefix(lname, "make") {
		cands = append(cands, "New"+typeName)
	}
	for _, cand := range cands {
		if fn, ok := pkg.Funcs[cand]; ok && fn.TypeParams == 0 {
			return f.callResults(ref+cand+"("+f.callArgs(fn, args, nil)+")", fn.Results)
		}
	}
	return callv(f.todo(n, "static method %s::%s() has no pocketmine-go equivalent", php, name)+f.phpx("Unsupported")+"("+quote(php+"::"+name)+f.anyArgs(args)+")", api.Any)
}

// newExpr converts new Class(...).
func (f *fctx) newExpr(x *ast.ExprNew) value {
	if sc, ok := x.Class.(*ast.StmtClass); ok {
		c := f.cv.anonByNode[sc]
		if c == nil {
			return callv(f.todo(x, "anonymous class")+f.phpx("Unsupported")+`("anonymous class")`, api.Any)
		}
		// Arguments of `new class(...)` go to its constructor.
		return f.newLocal(c, sc.Args, x)
	}
	name := identValue(x.Class)
	if name == "" {
		f.expr(x.Class, nil)
		return callv(f.todo(x, "new with a dynamic class name")+f.phpx("Unsupported")+`("new $class")`, api.Any)
	}
	full := f.cv.resolveName(f.file, x.Class)
	switch strings.ToLower(name) {
	case "self", "static":
		if f.cls != nil {
			return f.newLocal(f.cls, x.Args, x)
		}
	case "parent":
		if f.cls != nil && f.cls.Parent != nil {
			return f.newLocal(f.cls.Parent, x.Args, x)
		}
	}
	if c := f.cv.classes[strings.ToLower(full)]; c != nil {
		return f.newLocal(c, x.Args, x)
	}
	if isThrowableClass(full) {
		var parts []string
		for _, a := range phpArgs(x.Args) {
			parts = append(parts, f.expr(a.expr, nil).code)
		}
		msg := `""`
		if len(parts) > 0 {
			msg = f.str(f.expr(phpArgs(x.Args)[0].expr, api.String))
			parts = parts[1:]
		}
		extra := ""
		if len(parts) > 0 {
			extra = ", " + strings.Join(parts, ", ")
		}
		return callv(f.phpx("NewException")+"("+quote(full)+", "+msg+extra+")", throwableT)
	}
	switch strings.ToLower(full) {
	case "arrayobject", "arrayiterator":
		if len(x.Args) > 0 {
			return callv(f.phpx("ToArray")+"("+f.expr(phpArgs(x.Args)[0].expr, nil).code+")", arrayT(nil, nil))
		}
		return callv(f.phpx("NewArray")+"()", arrayT(nil, nil))
	case "stdclass":
		return callv(f.phpx("NewArray")+"()", arrayT(nil, nil))
	}
	return f.newExt(full, x.Args, x)
}

// newLocal constructs a plugin class.
func (f *fctx) newLocal(c *class, args []ast.Vertex, n ast.Vertex) value {
	if c.Abstract || c.Kind != kindClass && c.Kind != kindEnum {
		return callv(f.todo(n, "cannot instantiate abstract class %s", c.FQCN)+f.phpx("Unsupported")+"("+quote("new "+c.FQCN)+")", api.Any)
	}
	sig := f.cv.newSig(c)
	var m *method
	if ctor := c.constructor(); ctor != nil {
		m = ctor
	}
	return callv("New"+c.GoName+"("+f.callArgs(sig, args, m)+")", f.cv.classType(c))
}

// newSig is the signature of a plugin class's New function.
func (cv *converter) newSig(c *class) *api.Func {
	if ctor := c.constructor(); ctor != nil {
		return &api.Func{Params: ctor.Sig.Params, ParamNames: ctor.Sig.ParamNames, Variadic: ctor.Sig.Variadic}
	}
	if c.Exception {
		return &api.Func{Params: []*api.Type{api.Any, api.SliceOf(api.Any)}, ParamNames: []string{"message", "extra"}, Variadic: true}
	}
	return &api.Func{}
}

// newExt constructs a server class.
func (f *fctx) newExt(php string, args []ast.Vertex, n ast.Vertex) value {
	t := f.cv.extClassType(php)
	pkg := f.cv.extPackage(php)
	if pkg == nil {
		f.cv.external[php] = true
		return callv(f.todo(n, "class %s doesn't exist in pocketmine-go", php)+f.phpx("Unsupported")+"("+quote("new "+php)+f.anyArgs(args)+")", api.Any)
	}
	ref := f.pkgRef(pkg.Path) + "."
	typeName := lastSeg(php)
	if t != nil {
		typeName = t.Deref().ObjName()
	}
	var cands []string
	if target, ok := f.cv.idx.PHPMembers[strings.ToLower(php+"::__construct")]; ok {
		if i := strings.LastIndexByte(target, '.'); i >= 0 && target[:i] == pkg.Path {
			cands = append(cands, target[i+1:])
		}
	}
	cands = append(cands, "New"+typeName, "New"+lastSeg(php))
	nargs := len(phpArgs(args))
	for _, c := range cands {
		fn, ok := pkg.Funcs[c]
		if !ok || fn.TypeParams > 0 || len(fn.Results) == 0 {
			continue
		}
		if !fn.Variadic && nargs > len(fn.Params) {
			continue
		}
		return f.callResults(ref+c+"("+f.callArgs(fn, args, nil)+")", fn.Results)
	}
	if t != nil {
		if ti := f.cv.typeInfo(t.Deref()); ti != nil && !ti.Interface && ti.Underlying.K == api.KStruct {
			if nargs == 0 {
				return callv("&"+f.typeStr(t.Deref())+"{}", api.Ptr(t.Deref()))
			}
		}
	}
	return callv(f.todo(n, "no Go constructor found for %s", php)+f.phpx("Unsupported")+"("+quote("new "+php)+f.anyArgs(args)+")", api.Any)
}
