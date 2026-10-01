package conv

import (
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/VKCOM/php-parser/pkg/ast"
)

var vertexType = reflect.TypeOf((*ast.Vertex)(nil)).Elem()

// children returns the child nodes of n.
func children(n ast.Vertex) []ast.Vertex {
	if n == nil {
		return nil
	}
	v := reflect.ValueOf(n)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}
	var out []ast.Vertex
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch {
		case f.Type() == vertexType:
			if !f.IsNil() {
				out = append(out, f.Interface().(ast.Vertex))
			}
		case f.Kind() == reflect.Slice && f.Type().Elem() == vertexType:
			for j := 0; j < f.Len(); j++ {
				if e := f.Index(j); !e.IsNil() {
					out = append(out, e.Interface().(ast.Vertex))
				}
			}
		}
	}
	return out
}

// walk calls fn for n and its descendants; fn returns false to skip a node's children.
func walk(n ast.Vertex, fn func(ast.Vertex) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, c := range children(n) {
		walk(c, fn)
	}
}

func walkAll(ns []ast.Vertex, fn func(ast.Vertex) bool) {
	for _, n := range ns {
		walk(n, fn)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true, "default": true,
	"defer": true, "else": true, "fallthrough": true, "for": true, "func": true, "go": true, "goto": true,
	"if": true, "import": true, "interface": true, "map": true, "package": true, "range": true,
	"return": true, "select": true, "struct": true, "switch": true, "type": true, "var": true,
}

var goPredeclared = map[string]bool{
	"append": true, "cap": true, "clear": true, "close": true, "complex": true, "copy": true, "delete": true,
	"imag": true, "len": true, "make": true, "max": true, "min": true, "new": true, "panic": true,
	"print": true, "println": true, "real": true, "recover": true, "any": true, "bool": true, "byte": true,
	"comparable": true, "error": true, "float32": true, "float64": true, "int": true, "int8": true,
	"int16": true, "int32": true, "int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true, "true": true, "false": true,
	"iota": true, "nil": true, "complex64": true, "complex128": true,
}

// ident makes s a valid Go identifier.
func ident(s string) string {
	var sb strings.Builder
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r) && r < 128:
			sb.WriteRune(r)
		case unicode.IsDigit(r) && r < 128:
			if i == 0 {
				sb.WriteByte('_')
			}
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	out := sb.String()
	if out == "" {
		out = "_x"
	}
	return out
}

// localName is the Go name of a PHP variable.
func localName(s string) string {
	s = ident(s)
	if goKeywords[s] || goPredeclared[s] || s == "_" {
		return s + "_"
	}
	return s
}

// pascal converts a PHP name (camelCase or snake_case) to an exported Go name.
func pascal(s string) string {
	s = ident(s)
	if strings.Contains(s, "_") || isUpperName(s) {
		return snakeToPascal(s)
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// camel converts a PHP name to an unexported Go name.
func camel(s string) string {
	s = ident(s)
	if strings.Contains(strings.Trim(s, "_"), "_") || isUpperName(s) {
		s = snakeToPascal(s)
	}
	r := []rune(s)
	// Lower the leading run of capitals: "UUIDMap" -> "uuidMap".
	i := 0
	for i < len(r) && unicode.IsUpper(r[i]) {
		i++
	}
	switch {
	case i == 0:
	case i == 1 || i == len(r):
		for j := 0; j < i; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	default:
		for j := 0; j < i-1; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	}
	out := string(r)
	if goKeywords[out] || goPredeclared[out] {
		out += "_"
	}
	return out
}

func isUpperName(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter && len(s) > 1
}

func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var sb strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if isUpperName(p) || len(p) == 1 {
			p = strings.ToLower(p)
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		sb.WriteString(string(r))
	}
	out := sb.String()
	if out == "" {
		return "X"
	}
	if unicode.IsDigit(rune(out[0])) {
		out = "N" + out
	}
	return out
}

// normName lower-cases and strips underscores, for fuzzy name matching.
func normName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// quote returns a Go string literal.
func quote(s string) string {
	return strconv.Quote(s)
}
