package conv

import (
	"sort"
	"strings"

	"github.com/PMGopher/phar2go/internal/api"
)

// value is a converted expression.
type value struct {
	code string
	t    *api.Type
	// prec is the Go precedence of the expression: 7 for primary expressions, 6 for unary
	// ones, 5..1 for binary operators.
	prec int
	// konst is set for untyped constants (number literals).
	konst bool
	// call is set when code is a function call (valid as a statement).
	call bool
	// lvalue is set for variables, fields and array elements that PHP would copy on assignment.
	lvalue bool
}

func prim(code string, t *api.Type) value { return value{code: code, t: t, prec: 7} }

func callv(code string, t *api.Type) value { return value{code: code, t: t, prec: 7, call: true} }

// paren returns v's code, parenthesised if needed inside an operator of precedence p.
func paren(v value, p int) string {
	if v.prec < p {
		return "(" + v.code + ")"
	}
	return v.code
}

// typeStr formats a type as Go source (with package placeholders).
func (f *fctx) typeStr(t *api.Type) string {
	if t == nil {
		return "any"
	}
	switch t.K {
	case api.KBasic:
		if t.IsNil() {
			return "any"
		}
		if t.Name == "uint8" {
			return "byte"
		}
		if t.Name == "unsafe.Pointer" {
			return f.pkgRef("unsafe") + ".Pointer"
		}
		return t.Name
	case api.KNamed:
		if t.Name == "error" {
			return "error"
		}
		if n := t.ObjName(); n != "" && n[0] >= 'a' && n[0] <= 'z' && t.PkgPath() != "" && t.PkgPath() != api.PhpxPath {
			// An unexported type of the server: an interface can be written out as a literal.
			if ti := f.cv.typeInfo(t); ti != nil && ti.Interface && !ti.Unexported {
				return f.typeStr(&api.Type{K: api.KInterface, Methods: ti.Methods})
			}
			return "any"
		}
		i := strings.LastIndexByte(t.Name, '.')
		name := t.Name
		if i >= 0 {
			name = f.pkgRef(t.Name[:i]) + "." + t.Name[i+1:]
		}
		if t.Name == phpxArray {
			return name
		}
		if len(t.Args) > 0 {
			var args []string
			for _, a := range t.Args {
				args = append(args, f.typeStr(a))
			}
			name += "[" + strings.Join(args, ", ") + "]"
		}
		return name
	case api.KPointer:
		return "*" + f.typeStr(t.Elem)
	case api.KSlice:
		return "[]" + f.typeStr(t.Elem)
	case api.KArray:
		return "[" + itoa(int(t.Len)) + "]" + f.typeStr(t.Elem)
	case api.KMap:
		return "map[" + f.typeStr(t.Key) + "]" + f.typeStr(t.Elem)
	case api.KChan:
		return "chan " + f.typeStr(t.Elem)
	case api.KFunc:
		return "func" + f.sigStr(&api.Func{Params: t.Params, Results: t.Results, Variadic: t.Variadic}, nil)
	case api.KInterface:
		if len(t.Methods) == 0 {
			return "any"
		}
		var ms []string
		for n, m := range t.Methods {
			ms = append(ms, n+f.sigStr(m, nil))
		}
		sort.Strings(ms)
		return "interface{ " + strings.Join(ms, "; ") + " }"
	case api.KStruct:
		return "struct{}"
	case api.KTypeParam:
		return "any"
	case api.KTuple:
		return ""
	}
	return "any"
}

// sigStr formats a signature: "(a int, b string) (int, error)". names may be nil.
func (f *fctx) sigStr(s *api.Func, names []string) string {
	var ps []string
	for i, p := range s.Params {
		ts := f.typeStr(p)
		if s.Variadic && i == len(s.Params)-1 && p.K == api.KSlice {
			ts = "..." + f.typeStr(p.Elem)
		}
		if names != nil {
			ts = names[i] + " " + ts
		}
		ps = append(ps, ts)
	}
	out := "(" + strings.Join(ps, ", ") + ")"
	switch len(s.Results) {
	case 0:
	case 1:
		out += " " + f.typeStr(s.Results[0])
	default:
		var rs []string
		for _, r := range s.Results {
			rs = append(rs, f.typeStr(r))
		}
		out += " (" + strings.Join(rs, ", ") + ")"
	}
	return out
}

// zero is the zero value of a type.
func (f *fctx) zero(t *api.Type) string {
	if t == nil {
		return "nil"
	}
	switch {
	case t.IsString():
		return `""`
	case t.IsBool():
		return "false"
	case t.IsNumber():
		return "0"
	case t.Nilable() || isArrayT(t):
		return "nil"
	}
	if t.K == api.KNamed {
		if ti := f.cv.typeInfo(t); ti != nil {
			switch u := ti.Underlying; {
			case u.K == api.KStruct, u.K == api.KArray:
				return f.typeStr(t) + "{}"
			case u.K == api.KBasic && u.IsString():
				return f.typeStr(t) + `("")`
			case u.K == api.KBasic && u.IsBool():
				return f.typeStr(t) + "(false)"
			case u.K == api.KBasic:
				return f.typeStr(t) + "(0)"
			}
			return "nil"
		}
	}
	if t.K == api.KArray || t.K == api.KStruct {
		return f.typeStr(t) + "{}"
	}
	return "*new(" + f.typeStr(t) + ")"
}

// underlying returns the underlying type of a named type of the server (or t).
func (f *fctx) underlying(t *api.Type) *api.Type {
	if t != nil && t.K == api.KNamed {
		if ti := f.cv.typeInfo(t); ti != nil && ti.Underlying != nil {
			return ti.Underlying
		}
	}
	return t
}

// coerce converts v to type to, the way PHP would juggle it.
func (f *fctx) coerce(v value, to *api.Type) string {
	if to == nil || to.IsVoid() {
		return v.code
	}
	from := v.t
	if from == nil {
		from = api.Any
	}
	if from.IsVoid() {
		return v.code
	}
	if v.konst {
		u := f.underlying(to)
		switch {
		case u.IsNumber() && (from.IsInt() || u.IsFloat()):
			if to.K == api.KNamed {
				return f.typeStr(to) + "(" + v.code + ")"
			}
			return v.code
		case to.IsAny():
			return v.code
		}
	}
	if from.IsNil() {
		if to.Nilable() || isArrayT(to) {
			return "nil"
		}
		return f.zero(to)
	}
	if f.cv.assignable(from, to) {
		return v.code
	}
	if to.IsAny() || (to.K == api.KInterface && len(to.Methods) == 0) {
		return v.code
	}
	// A plugin class where the server class it extends is wanted: the embedded value.
	if c := f.cv.localClassOf(from); c != nil && to.K == api.KPointer && to.Elem.K == api.KNamed && !isArrayT(to) {
		recv := paren(v, 7)
		if c.Poly && from.K == api.KNamed && from.Name == c.IfaceName {
			recv += "." + asMethod(c) + "()"
		}
		path := recv
		for k := c; k != nil; k = k.Parent {
			if k.ExtEmbed != nil {
				if api.Identical(k.ExtEmbed, to.Elem) {
					return "&" + path + "." + k.ExtEmbed.ObjName()
				}
				break
			}
			if k.Parent != nil {
				path += "." + k.Parent.GoName
			}
		}
	}
	ut := f.underlying(to)
	uf := f.underlying(from)
	// From untyped values.
	if isAny(from) || from.IsNil() || f.cv.isInterface(from) && !f.cv.isInterface(to) && from.K != api.KNamed {
		return f.fromAny(v, to, ut)
	}
	switch {
	case ut.IsString():
		if uf.IsString() {
			return f.typeStr(to) + "(" + v.code + ")"
		}
		if to.IsString() {
			return f.phpx("ToString") + "(" + v.code + ")"
		}
		return f.typeStr(to) + "(" + f.phpx("ToString") + "(" + v.code + "))"
	case ut.IsBool():
		if uf.IsBool() {
			return f.typeStr(to) + "(" + v.code + ")"
		}
		return f.cond(v)
	case ut.IsNumber():
		if uf.IsNumber() {
			return f.typeStr(to) + "(" + v.code + ")"
		}
		if uf.IsBool() || uf.IsString() || isArrayT(from) {
			conv := "ToInt"
			if ut.IsFloat() {
				conv = "ToFloat"
			}
			c := f.phpx(conv) + "(" + v.code + ")"
			if to.IsInt() && conv == "ToInt" || to.IsFloat() && conv == "ToFloat" && to.Name == "float64" {
				return c
			}
			return f.typeStr(to) + "(" + c + ")"
		}
	case isArrayT(to):
		return f.phpx("ToArray") + "(" + v.code + ")"
	case to.K == api.KSlice:
		if isArrayT(from) || from.K == api.KSlice || from.K == api.KArray {
			if to.Elem.K == api.KBasic && to.Elem.Name == "uint8" && from.IsString() {
				return "[]byte(" + v.code + ")"
			}
			return f.phpx("ToSlice") + "[" + f.typeStr(to.Elem) + "](" + v.code + ")"
		}
		if to.Elem.Name == "uint8" && uf.IsString() {
			return "[]byte(" + v.code + ")"
		}
	case to.K == api.KMap:
		if isArrayT(from) || from.K == api.KMap {
			return f.phpx("ToMap") + "[" + f.typeStr(to.Key) + ", " + f.typeStr(to.Elem) + "](" + v.code + ")"
		}
	case to.K == api.KPointer && !isArrayT(to):
		if f.cv.assignable(from, to.Elem) || api.Identical(from, to.Elem) || to.Elem.K == api.KBasic || f.underlying(to.Elem).K == api.KBasic {
			return f.phpx("Ptr") + "[" + f.typeStr(to.Elem) + "](" + f.coerce(v, to.Elem) + ")"
		}
	}
	if from.K == api.KPointer && !isArrayT(from) && (api.Identical(from.Elem, to) || f.cv.assignable(from.Elem, to)) && !to.Nilable() {
		return f.phpx("Deref") + "(" + v.code + ")"
	}
	if from.K == api.KNamed && to.K == api.KNamed && f.underlying(from).K == api.KBasic && f.underlying(to).K == api.KBasic {
		return f.typeStr(to) + "(" + v.code + ")"
	}
	return f.phpx("As") + "[" + f.typeStr(to) + "](" + v.code + ")"
}

func (f *fctx) fromAny(v value, to, ut *api.Type) string {
	conv := ""
	switch {
	case ut.IsString():
		conv = "ToString"
	case ut.IsBool():
		conv = "ToBool"
	case ut.IsFloat():
		conv = "ToFloat"
	case ut.IsInt():
		conv = "ToInt"
	case isArrayT(to):
		return f.phpx("ToArray") + "(" + v.code + ")"
	case to.K == api.KSlice:
		return f.phpx("ToSlice") + "[" + f.typeStr(to.Elem) + "](" + v.code + ")"
	case to.K == api.KMap:
		return f.phpx("ToMap") + "[" + f.typeStr(to.Key) + ", " + f.typeStr(to.Elem) + "](" + v.code + ")"
	}
	if conv != "" {
		c := f.phpx(conv) + "(" + v.code + ")"
		switch {
		case conv == "ToString" && to.IsString(), conv == "ToBool" && to.IsBool(),
			conv == "ToFloat" && to.K == api.KBasic && to.Name == "float64", conv == "ToInt" && to.K == api.KBasic && to.Name == "int":
			return c
		}
		return f.typeStr(to) + "(" + c + ")"
	}
	return f.phpx("As") + "[" + f.typeStr(to) + "](" + v.code + ")"
}

// cond converts a value to a Go bool (PHP truthiness).
func (f *fctx) cond(v value) string {
	t := v.t
	switch {
	case t == nil:
	case t.IsBool():
		return v.code
	case t.IsInt() || t.IsFloat():
		return paren(v, 4) + " != 0"
	case isArrayT(t):
		return paren(v, 7) + ".Len() > 0"
	case t.K == api.KPointer || t.K == api.KFunc || t.K == api.KChan:
		return paren(v, 4) + " != nil"
	case t.K == api.KSlice || t.K == api.KMap:
		return "len(" + v.code + ") > 0"
	case t.K == api.KNamed && !f.cv.isInterface(t):
		u := f.underlying(t)
		if u.IsBool() {
			return "bool(" + v.code + ")"
		}
		if u.IsNumber() {
			return paren(v, 4) + " != 0"
		}
		if u.K == api.KStruct {
			return "true"
		}
	case f.cv.isInterface(t) && !isAny(t):
		return "!" + f.phpx("IsNull") + "(" + v.code + ")"
	}
	return f.phpx("ToBool") + "(" + v.code + ")"
}

// str converts a value to a Go string (PHP string conversion).
func (f *fctx) str(v value) string {
	if v.t != nil && v.t.IsString() {
		return v.code
	}
	if v.konst && v.t != nil && v.t.IsInt() {
		return quote(v.code)
	}
	return f.phpx("ToString") + "(" + v.code + ")"
}

// isNilV reports whether v is the nil literal.
func isNilV(v value) bool { return v.t != nil && v.t.IsNil() }
