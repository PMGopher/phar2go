// Package api describes the Go API of pocketmine-go: the packages, types, functions and constants
// a converted plugin can use, plus the PocketMine-MP (PHP) names each of them is a port of.
//
// The index is generated from a pocketmine-go checkout (see Generate) and embedded in phar2go, so
// converting a plugin doesn't need the server's source code.
package api

import (
	"strings"
)

// Kind is the kind of a Type.
type Kind uint8

const (
	KInvalid   Kind = iota
	KBasic          // Name: int, string, bool, float64, ...
	KNamed          // Name: "<pkg path>.<Name>", Args: type arguments
	KPointer        // Elem
	KSlice          // Elem
	KArray          // Elem, Len
	KMap            // Key, Elem
	KFunc           // Params, Results, Variadic
	KInterface      // Methods (an interface literal: any, interface{ M() })
	KStruct         // a struct literal
	KChan           // Elem
	KTypeParam      // Name
	KTuple          // Results: several results of a call
)

// Type is a Go type. It is a small, serialisable stand-in for go/types.Type.
type Type struct {
	K        Kind             `json:"k"`
	Name     string           `json:"n,omitempty"`
	Args     []*Type          `json:"a,omitempty"`
	Elem     *Type            `json:"e,omitempty"`
	Key      *Type            `json:"y,omitempty"`
	Len      int64            `json:"l,omitempty"`
	Params   []*Type          `json:"p,omitempty"`
	Results  []*Type          `json:"r,omitempty"`
	Variadic bool             `json:"v,omitempty"`
	Methods  map[string]*Func `json:"m,omitempty"`
	// Iface is set on named interface types.
	Iface bool `json:"i,omitempty"`
}

// Func is a function or method signature.
type Func struct {
	Params     []*Type  `json:"p,omitempty"`
	ParamNames []string `json:"pn,omitempty"`
	Results    []*Type  `json:"r,omitempty"`
	Variadic   bool     `json:"v,omitempty"`
	// TypeParams is the number of type parameters of a generic function.
	TypeParams int `json:"tp,omitempty"`
}

// Common types.
var (
	Any     = &Type{K: KInterface}
	String  = &Type{K: KBasic, Name: "string"}
	Int     = &Type{K: KBasic, Name: "int"}
	Int64   = &Type{K: KBasic, Name: "int64"}
	Float   = &Type{K: KBasic, Name: "float64"}
	Bool    = &Type{K: KBasic, Name: "bool"}
	Error   = &Type{K: KNamed, Name: "error"}
	Invalid = &Type{K: KInvalid}
	// UntypedNil is the type of nil.
	UntypedNil = &Type{K: KBasic, Name: "untyped nil"}
	Void       = &Type{K: KTuple}
)

func Ptr(t *Type) *Type       { return &Type{K: KPointer, Elem: t} }
func SliceOf(t *Type) *Type   { return &Type{K: KSlice, Elem: t} }
func MapOf(k, v *Type) *Type  { return &Type{K: KMap, Key: k, Elem: v} }
func Named(name string) *Type { return &Type{K: KNamed, Name: name} }
func Basic(name string) *Type { return &Type{K: KBasic, Name: name} }
func FuncType(f *Func) *Type {
	return &Type{K: KFunc, Params: f.Params, Results: f.Results, Variadic: f.Variadic}
}
func (t *Type) IsAny() bool    { return t != nil && t.K == KInterface && len(t.Methods) == 0 }
func (t *Type) IsVoid() bool   { return t != nil && t.K == KTuple && len(t.Results) == 0 }
func (t *Type) IsNil() bool    { return t != nil && t.K == KBasic && t.Name == "untyped nil" }
func (t *Type) IsString() bool { return t != nil && t.K == KBasic && t.Name == "string" }
func (t *Type) IsBool() bool   { return t != nil && t.K == KBasic && t.Name == "bool" }

// IsInt reports whether t is one of Go's integer types.
func (t *Type) IsInt() bool {
	if t == nil || t.K != KBasic {
		return false
	}
	switch t.Name {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune":
		return true
	}
	return false
}

// IsFloat reports whether t is float32 or float64.
func (t *Type) IsFloat() bool {
	return t != nil && t.K == KBasic && (t.Name == "float64" || t.Name == "float32")
}

func (t *Type) IsNumber() bool { return t.IsInt() || t.IsFloat() }

// Nilable reports whether nil is a value of t.
func (t *Type) Nilable() bool {
	if t == nil {
		return false
	}
	switch t.K {
	case KPointer, KSlice, KMap, KFunc, KInterface, KChan:
		return true
	case KNamed:
		return t.Name == "error" || t.Iface
	}
	return t.IsNil()
}

// PkgPath is the package of a named type ("" for builtins and types of the plugin itself).
func (t *Type) PkgPath() string {
	if t.K != KNamed {
		return ""
	}
	i := strings.LastIndexByte(t.Name, '.')
	if i < 0 {
		return ""
	}
	return t.Name[:i]
}

// ObjName is the name of a named type without its package.
func (t *Type) ObjName() string {
	i := strings.LastIndexByte(t.Name, '.')
	return t.Name[i+1:]
}

// Deref returns the element of a pointer type, or t.
func (t *Type) Deref() *Type {
	if t != nil && t.K == KPointer {
		return t.Elem
	}
	return t
}

// Identical reports whether a and b are the same type.
func Identical(a, b *Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || a.K != b.K {
		return false
	}
	switch a.K {
	case KBasic, KTypeParam:
		return normBasic(a.Name) == normBasic(b.Name)
	case KNamed:
		if a.Name == "phpx.Array" && b.Name == a.Name {
			// The arguments only carry the element type of a PHP array.
			return true
		}
		if a.Name != b.Name || len(a.Args) != len(b.Args) {
			return false
		}
		for i := range a.Args {
			if !Identical(a.Args[i], b.Args[i]) {
				return false
			}
		}
		return true
	case KPointer, KSlice, KChan:
		return Identical(a.Elem, b.Elem)
	case KArray:
		return a.Len == b.Len && Identical(a.Elem, b.Elem)
	case KMap:
		return Identical(a.Key, b.Key) && Identical(a.Elem, b.Elem)
	case KFunc:
		return sameList(a.Params, b.Params) && sameList(a.Results, b.Results) && a.Variadic == b.Variadic
	case KTuple:
		return sameList(a.Results, b.Results)
	case KInterface:
		if len(a.Methods) != len(b.Methods) {
			return false
		}
		for n, m := range a.Methods {
			o, ok := b.Methods[n]
			if !ok || !SameSig(m, o) {
				return false
			}
		}
		return true
	}
	return false
}

func normBasic(n string) string {
	switch n {
	case "byte":
		return "uint8"
	case "rune":
		return "int32"
	}
	return n
}

func sameList(a, b []*Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Identical(a[i], b[i]) {
			return false
		}
	}
	return true
}

// SameSig reports whether two signatures are identical.
func SameSig(a, b *Func) bool {
	return a.Variadic == b.Variadic && sameList(a.Params, b.Params) && sameList(a.Results, b.Results)
}

// Subst replaces type parameters in t with args (by name).
func Subst(t *Type, args map[string]*Type) *Type {
	if t == nil || len(args) == 0 {
		return t
	}
	switch t.K {
	case KTypeParam:
		if a, ok := args[t.Name]; ok {
			return a
		}
		return t
	case KNamed:
		if len(t.Args) == 0 {
			return t
		}
		c := *t
		c.Args = substList(t.Args, args)
		return &c
	case KPointer, KSlice, KChan, KArray:
		c := *t
		c.Elem = Subst(t.Elem, args)
		return &c
	case KMap:
		c := *t
		c.Key = Subst(t.Key, args)
		c.Elem = Subst(t.Elem, args)
		return &c
	case KFunc, KTuple:
		c := *t
		c.Params = substList(t.Params, args)
		c.Results = substList(t.Results, args)
		return &c
	}
	return t
}

func substList(l []*Type, args map[string]*Type) []*Type {
	out := make([]*Type, len(l))
	for i, t := range l {
		out[i] = Subst(t, args)
	}
	return out
}

// SubstFunc applies Subst to a signature.
func SubstFunc(f *Func, args map[string]*Type) *Func {
	if len(args) == 0 {
		return f
	}
	c := *f
	c.Params = substList(f.Params, args)
	c.Results = substList(f.Results, args)
	return &c
}

// HasTypeParam reports whether t mentions a type parameter.
func HasTypeParam(t *Type) bool {
	if t == nil {
		return false
	}
	switch t.K {
	case KTypeParam:
		return true
	case KNamed:
		for _, a := range t.Args {
			if HasTypeParam(a) {
				return true
			}
		}
	case KPointer, KSlice, KChan, KArray:
		return HasTypeParam(t.Elem)
	case KMap:
		return HasTypeParam(t.Key) || HasTypeParam(t.Elem)
	case KFunc, KTuple:
		for _, p := range t.Params {
			if HasTypeParam(p) {
				return true
			}
		}
		for _, p := range t.Results {
			if HasTypeParam(p) {
				return true
			}
		}
	}
	return false
}
