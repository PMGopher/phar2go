package phpx

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// IsNull is $v === null. Typed nil pointers, maps, slices and functions are null too.
func IsNull(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return rv.IsNil()
	}
	return false
}

// ToString is (string) $v.
func ToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case float64:
		return FloatString(x)
	case float32:
		return FloatString(float64(x))
	case *Array:
		return "Array"
	case []byte:
		return string(x)
	case error:
		if t, ok := x.(Throwable); ok {
			return t.ToString()
		}
		return x.Error()
	case interface{ ToString() string }:
		return x.ToString()
	case interface{ String() string }:
		return x.String()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return FloatString(rv.Float())
	case reflect.Bool:
		if rv.Bool() {
			return "1"
		}
		return ""
	}
	return fmt.Sprint(v)
}

// FloatString formats a float like PHP's string conversion (precision 14).
func FloatString(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case math.IsNaN(f):
		return "NAN"
	case f == 0:
		if math.Signbit(f) {
			return "-0"
		}
		return "0"
	}
	s := strconv.FormatFloat(f, 'G', 14, 64)
	if i := strings.IndexByte(s, 'E'); i >= 0 {
		mant, exp := s[:i], s[i+1:]
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		sign := exp[0]
		exp = strings.TrimLeft(exp[1:], "0")
		if exp == "" {
			exp = "0"
		}
		return mant + "E" + string(sign) + exp
	}
	return s
}

// ToInt is (int) $v.
func ToInt(v any) int {
	switch x := v.(type) {
	case nil:
		return 0
	case int:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return floatToInt(x)
	case string:
		n, _ := parseNumericPrefix(x)
		if f, ok := n.(float64); ok {
			return floatToInt(f)
		}
		return n.(int)
	case *Array:
		if x.Len() > 0 {
			return 1
		}
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return floatToInt(rv.Float())
	case reflect.Bool:
		if rv.Bool() {
			return 1
		}
		return 0
	case reflect.String:
		return ToInt(rv.String())
	}
	if IsNull(v) {
		return 0
	}
	return 1
}

func floatToInt(f float64) int {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int(f)
}

// ToFloat is (float) $v.
func ToFloat(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		n, _ := parseNumericPrefix(x)
		if f, ok := n.(float64); ok {
			return f
		}
		return float64(n.(int))
	case bool:
		if x {
			return 1
		}
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return ToFloat(rv.String())
	}
	return float64(ToInt(v))
}

// ToBool is (bool) $v: PHP's truthiness.
func ToBool(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != "" && x != "0"
	case *Array:
		return x != nil && x.Len() > 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.String:
		return ToBool(rv.String())
	case reflect.Bool:
		return rv.Bool()
	case reflect.Slice, reflect.Map:
		return rv.Len() > 0
	}
	return !IsNull(v)
}

// parseNumericPrefix parses the leading number of s like PHP's (int)/(float) casts. ok reports
// whether the whole string (ignoring surrounding whitespace) is numeric.
func parseNumericPrefix(s string) (any, bool) {
	t := strings.TrimLeft(s, " \t\n\r\v\f")
	end := 0
	isFloat := false
	if end < len(t) && (t[end] == '+' || t[end] == '-') {
		end++
	}
	digits := 0
	for end < len(t) && t[end] >= '0' && t[end] <= '9' {
		end++
		digits++
	}
	if end < len(t) && t[end] == '.' {
		j := end + 1
		frac := 0
		for j < len(t) && t[j] >= '0' && t[j] <= '9' {
			j++
			frac++
		}
		if digits > 0 || frac > 0 {
			isFloat = true
			end = j
			digits += frac
		}
	}
	if digits > 0 && end < len(t) && (t[end] == 'e' || t[end] == 'E') {
		j := end + 1
		if j < len(t) && (t[j] == '+' || t[j] == '-') {
			j++
		}
		if j < len(t) && t[j] >= '0' && t[j] <= '9' {
			for j < len(t) && t[j] >= '0' && t[j] <= '9' {
				j++
			}
			isFloat = true
			end = j
		}
	}
	if digits == 0 {
		return 0, false
	}
	num := t[:end]
	whole := strings.TrimRight(t[end:], " \t\n\r\v\f") == ""
	if !isFloat {
		if n, err := strconv.ParseInt(num, 10, 64); err == nil {
			return int(n), whole
		}
		isFloat = true
	}
	f, _ := strconv.ParseFloat(num, 64)
	return f, whole
}

// IsNumeric is is_numeric($v).
func IsNumeric(v any) bool {
	switch x := v.(type) {
	case int, int64, int32, float64, float32, uint, uint64, uint32:
		return true
	case string:
		if x == "" || strings.TrimRight(x, " \t\n\r\v\f") != x && strings.TrimSpace(x) == "" {
			return false
		}
		_, ok := parseNumericPrefix(x)
		return ok
	}
	return false
}

// ToNumber converts v to an int or a float64, like PHP's arithmetic does.
func ToNumber(v any) any {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return x
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		n, _ := parseNumericPrefix(x)
		return n
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return ToNumber(rv.String())
	}
	return ToInt(v)
}

// Ptr returns a pointer to a copy of v.
func Ptr[T any](v T) *T { return &v }

// Deref returns *p, or the zero value if p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Must returns v, or throws err.
func Must[T any](v T, err error) T {
	if err != nil {
		Throw(err)
	}
	return v
}

// Check throws err if it isn't nil.
func Check(err error) {
	if err != nil {
		Throw(err)
	}
}

// OkOr returns v if ok, or the zero value (null) otherwise.
func OkOr[T any](v T, ok bool) T {
	if !ok {
		var zero T
		return zero
	}
	return v
}

// First returns the first of two results.
func First[A, B any](a A, _ B) A { return a }

// Ternary returns a if cond, else b. Both are evaluated.
func Ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// Coalesce returns v unless it is null, else def (the ?? operator for already evaluated values).
func Coalesce[T any](v T, def T) T {
	if IsNull(v) {
		return def
	}
	return v
}

// Elvis is $a ?: $b.
func Elvis[T any](a, b T) T {
	if ToBool(a) {
		return a
	}
	return b
}

// As converts v to T: a type assertion that also converts PHP scalars and arrays, and returns
// the zero value (null) when v can't be converted.
func As[T any](v any) T {
	if t, ok := v.(T); ok {
		return t
	}
	var zero T
	rt := reflect.TypeOf(&zero).Elem()
	if out, ok := convertTo(v, rt); ok {
		return out.Interface().(T)
	}
	return zero
}

// Is reports whether v holds a T (instanceof).
func Is[T any](v any) bool {
	if IsNull(v) {
		return false
	}
	_, ok := v.(T)
	return ok
}

var (
	arrayType = reflect.TypeOf((*Array)(nil))
	anyType   = reflect.TypeOf((*any)(nil)).Elem()
)

// convertTo converts v to type t following PHP's conversion rules where they make sense.
func convertTo(v any, t reflect.Type) (reflect.Value, bool) {
	if v == nil {
		return reflect.Zero(t), true
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(t) {
		out := reflect.New(t).Elem()
		out.Set(rv)
		return out, true
	}
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(ToString(v)).Convert(t), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(ToInt(v))).Convert(t), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(uint64(ToInt(v))).Convert(t), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(ToFloat(v)).Convert(t), true
	case reflect.Bool:
		return reflect.ValueOf(ToBool(v)).Convert(t), true
	case reflect.Interface:
		if rv.Type().Implements(t) {
			out := reflect.New(t).Elem()
			out.Set(rv)
			return out, true
		}
		return reflect.Zero(t), false
	case reflect.Pointer:
		if t == arrayType {
			return reflect.ValueOf(ToArray(v)), true
		}
		// A plugin object where the server type it extends is wanted: the embedded value.
		if e := embeddedPtr(rv, t); e.IsValid() {
			return e, true
		}
		// A value where a pointer to it is wanted.
		if rv.Type() == t.Elem() {
			p := reflect.New(t.Elem())
			p.Elem().Set(rv)
			return p, true
		}
		if out, ok := convertTo(v, t.Elem()); ok && t.Elem().Kind() != reflect.Struct {
			p := reflect.New(t.Elem())
			p.Elem().Set(out)
			return p, true
		}
	case reflect.Slice:
		if a, ok := v.(*Array); ok {
			s := reflect.MakeSlice(t, 0, a.Len())
			for _, e := range a.Values() {
				ev, ok := convertTo(e, t.Elem())
				if !ok {
					ev = reflect.Zero(t.Elem())
				}
				s = reflect.Append(s, ev)
			}
			return s, true
		}
		if rv.Kind() == reflect.Slice {
			s := reflect.MakeSlice(t, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				ev, ok := convertTo(rv.Index(i).Interface(), t.Elem())
				if !ok {
					ev = reflect.Zero(t.Elem())
				}
				s = reflect.Append(s, ev)
			}
			return s, true
		}
		if t.Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf([]byte(ToString(v))).Convert(t), true
		}
	case reflect.Map:
		if a, ok := v.(*Array); ok {
			m := reflect.MakeMapWithSize(t, a.Len())
			for _, e := range a.Entries() {
				kv, ok1 := convertTo(e.Key, t.Key())
				vv, ok2 := convertTo(e.Val, t.Elem())
				if ok1 && ok2 {
					m.SetMapIndex(kv, vv)
				}
			}
			return m, true
		}
	case reflect.Func:
		if rv.Kind() == reflect.Func {
			return adaptFunc(rv, t), true
		}
		if name, ok := v.(string); ok {
			if target, ok := stringCallable(name); ok {
				return adaptFunc(reflect.ValueOf(target), t), true
			}
		}
		if a, ok := v.(*Array); ok && a.Len() == 2 {
			// [$object, "method"]
			if m := methodByPHPName(reflect.ValueOf(a.Get(0)), ToString(a.Get(1))); m.IsValid() {
				return adaptFunc(m, t), true
			}
		}
	}
	if rv.Type().ConvertibleTo(t) && rv.Kind() != reflect.String && t.Kind() != reflect.String {
		return rv.Convert(t), true
	}
	return reflect.Zero(t), false
}

// adaptFunc wraps fn (any signature) as a function of type t, converting arguments and results.
func adaptFunc(fn reflect.Value, t reflect.Type) reflect.Value {
	if fn.Type().AssignableTo(t) {
		return fn
	}
	return reflect.MakeFunc(t, func(args []reflect.Value) []reflect.Value {
		in := make([]any, len(args))
		for i, a := range args {
			in[i] = a.Interface()
		}
		res := callValue(fn, in)
		out := make([]reflect.Value, t.NumOut())
		for i := range out {
			if i == 0 {
				v, ok := convertTo(res, t.Out(0))
				if !ok {
					v = reflect.Zero(t.Out(0))
				}
				out[i] = v
			} else {
				out[i] = reflect.Zero(t.Out(i))
			}
		}
		return out
	})
}

// ToArray is (array) $v: Go slices and maps become arrays, null an empty array, and any other
// value an array holding it.
func ToArray(v any) *Array {
	switch x := v.(type) {
	case *Array:
		if x == nil {
			return NewArray()
		}
		return x
	case nil:
		return NewArray()
	case []any:
		return List(x...)
	case []string:
		a := &Array{index: make(map[any]*entry, len(x))}
		for _, s := range x {
			a.Append(s)
		}
		return a
	case map[string]any:
		a := NewArray()
		for _, k := range sortedKeys(x) {
			a.Set(k, fromGo(x[k]))
		}
		return a
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return NewArray()
		}
		a := &Array{index: make(map[any]*entry, rv.Len())}
		for i := 0; i < rv.Len(); i++ {
			a.Append(rv.Index(i).Interface())
		}
		return a
	case reflect.Map:
		a := NewArray()
		keys := rv.MapKeys()
		sortValues(keys)
		for _, k := range keys {
			a.Set(k.Interface(), rv.MapIndex(k).Interface())
		}
		return a
	}
	if IsNull(v) {
		return NewArray()
	}
	return List(v)
}

// fromGo converts nested values decoded from YAML/JSON (map[string]any, []any) to arrays.
func fromGo(v any) any {
	switch x := v.(type) {
	case map[string]any:
		a := NewArray()
		for _, k := range sortedKeys(x) {
			a.Set(k, fromGo(x[k]))
		}
		return a
	case map[any]any:
		a := NewArray()
		for k, val := range x {
			a.Set(k, fromGo(val))
		}
		return a
	case []any:
		a := NewArray()
		for _, e := range x {
			a.Append(fromGo(e))
		}
		return a
	case int64:
		return int(x)
	case uint64:
		return int(x)
	}
	return v
}

// FromGo converts a value from a Go API (slices, maps, nested config values) to PHP values.
func FromGo(v any) any {
	switch v.(type) {
	case map[string]any, map[any]any, []any:
		return fromGo(v)
	case int64:
		return int(v.(int64))
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 || rv.Kind() == reflect.Map {
		return ToArray(v)
	}
	return v
}

// ToGo converts PHP values to plain Go values (arrays to []any / map[string]any), for Go APIs
// such as configs that take any.
func ToGo(v any) any {
	a, ok := v.(*Array)
	if !ok {
		return v
	}
	if a.IsList() {
		out := make([]any, 0, a.Len())
		for _, e := range a.Values() {
			out = append(out, ToGo(e))
		}
		return out
	}
	out := make(map[string]any, a.Len())
	for _, e := range a.Entries() {
		out[ToString(e.Key)] = ToGo(e.Val)
	}
	return out
}

// ToSlice converts an array (or any list) to a Go slice.
func ToSlice[T any](v any) []T {
	if s, ok := v.([]T); ok {
		return s
	}
	if IsNull(v) {
		return nil
	}
	out, ok := convertTo(v, reflect.TypeOf((*[]T)(nil)).Elem())
	if !ok {
		return nil
	}
	return out.Interface().([]T)
}

// ToMap converts an array to a Go map.
func ToMap[K comparable, V any](v any) map[K]V {
	if m, ok := v.(map[K]V); ok {
		return m
	}
	if IsNull(v) {
		return nil
	}
	out, ok := convertTo(v, reflect.TypeOf((*map[K]V)(nil)).Elem())
	if !ok {
		return nil
	}
	return out.Interface().(map[K]V)
}

// SliceAt is $list[i] on a Go slice: the zero value (null) when out of range.
func SliceAt[T any](s []T, i int) T {
	if i < 0 || i >= len(s) {
		var zero T
		return zero
	}
	return s[i]
}

// MapAt is $map[k] on a Go map.
func MapAt[K comparable, V any](m map[K]V, k K) V { return m[k] }

// Clone copies arrays (PHP assignment by value) and leaves other values alone.
func Clone[T any](v T) T {
	if a, ok := any(v).(*Array); ok && a != nil {
		return any(a.Clone()).(T)
	}
	return v
}

// TypeName is get_debug_type($v) / gettype($v).
func TypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return "int"
	case float64, float32:
		return "float"
	case string:
		return "string"
	case *Array:
		return "array"
	}
	if IsNull(v) {
		return "null"
	}
	return ClassName(v)
}

// ClassName is get_class($v) / $v::class.
func ClassName(v any) string {
	if t, ok := v.(interface{ PhpClass() string }); ok {
		return t.PhpClass()
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// Number is the set of Go number types.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

// Set is an assignment used as an expression: *p = v, returning v.
func Set[T any](p *T, v T) T {
	*p = v
	return v
}

// PostInc is $x++ used as an expression.
func PostInc[T Number](p *T) T {
	old := *p
	*p++
	return old
}

// PreInc is ++$x used as an expression.
func PreInc[T Number](p *T) T {
	*p++
	return *p
}

// PostDec is $x-- used as an expression.
func PostDec[T Number](p *T) T {
	old := *p
	*p--
	return old
}

// PreDec is --$x used as an expression.
func PreDec[T Number](p *T) T {
	*p--
	return *p
}

// CoalesceAs is $v ?? $def where $v isn't typed: def if v is null, else v converted to T.
func CoalesceAs[T any](v any, def T) T {
	if IsNull(v) {
		return def
	}
	return As[T](v)
}

// ThrowV is a throw expression of type T.
func ThrowV[T any](e any) T {
	Throw(e)
	var zero T
	return zero
}

// Unsupported marks PHP code phar2go couldn't convert: it throws when reached.
func Unsupported(what string, _ ...any) any {
	Throw(NewError("Error", "not converted from PHP: "+what))
	return nil
}

// StrAt is $string[$i].
func StrAt(s string, i int) string {
	if i < 0 {
		i += len(s)
	}
	if i < 0 || i >= len(s) {
		return ""
	}
	return s[i : i+1]
}

// CloneObject is `clone $obj`: a shallow copy of the object (arrays inside are copied).
func CloneObject[T any](v T) T {
	if a, ok := any(v).(*Array); ok {
		return any(a.Clone()).(T)
	}
	if c, ok := any(v).(interface{ Clone() T }); ok {
		return c.Clone()
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Elem().Kind() == reflect.Struct {
		cp := reflect.New(rv.Elem().Type())
		cp.Elem().Set(rv.Elem())
		return cp.Interface().(T)
	}
	return v
}

// embeddedPtr finds a struct of type *t.Elem() embedded (at any depth) in the struct rv points
// to, and returns a pointer to it.
func embeddedPtr(rv reflect.Value, t reflect.Type) reflect.Value {
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct || t.Elem().Kind() != reflect.Struct {
		return reflect.Value{}
	}
	queue := []reflect.Value{rv.Elem()}
	for depth := 0; len(queue) > 0 && depth < 12; depth++ {
		var next []reflect.Value
		for _, sv := range queue {
			st := sv.Type()
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				if !f.Anonymous {
					continue
				}
				fv := sv.Field(i)
				switch {
				case f.Type == t.Elem():
					return fv.Addr()
				case f.Type == t && !fv.IsNil():
					return fv
				case f.Type.Kind() == reflect.Struct:
					next = append(next, fv)
				case f.Type.Kind() == reflect.Pointer && f.Type.Elem().Kind() == reflect.Struct && !fv.IsNil():
					next = append(next, fv.Elem())
				}
			}
		}
		queue = next
	}
	return reflect.Value{}
}
