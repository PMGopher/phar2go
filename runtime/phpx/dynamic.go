package phpx

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unsafe"
)

// goNames returns the Go names a PHP method or property name may have been converted to.
func goNames(name string) []string {
	if name == "" {
		return nil
	}
	pascal := strings.ToUpper(name[:1]) + name[1:]
	out := []string{pascal, name}
	for _, prefix := range []string{"get", "Get"} {
		if strings.HasPrefix(name, prefix) && len(name) > 3 && unicode.IsUpper(rune(name[3])) {
			out = append(out, name[3:])
		}
	}
	return out
}

// methodByPHPName finds the method a PHP method name was converted to.
func methodByPHPName(v reflect.Value, name string) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}
	for _, n := range goNames(name) {
		if m := v.MethodByName(n); m.IsValid() {
			return m
		}
	}
	// Case-insensitive, like PHP method names.
	t := v.Type()
	for i := 0; i < t.NumMethod(); i++ {
		if strings.EqualFold(t.Method(i).Name, name) {
			return v.Method(i)
		}
	}
	return reflect.Value{}
}

// Call is $obj->method(...$args) on a value whose type isn't known when converting.
func Call(obj any, method string, args ...any) any {
	if IsNull(obj) {
		Throw(NewError("Error", fmt.Sprintf("Call to a member function %s() on null", method)))
	}
	m := methodByPHPName(reflect.ValueOf(obj), method)
	if !m.IsValid() {
		if a, ok := obj.(*Array); ok {
			_ = a
		}
		Throw(NewError("Error", fmt.Sprintf("Call to undefined method %s::%s()", ClassName(obj), method)))
	}
	return callValue(m, args)
}

// Invoke calls a PHP callable: a closure, [$object, "method"], or "Class::method" is not
// supported.
func Invoke(fn any, args ...any) any {
	switch f := fn.(type) {
	case func():
		f()
		return nil
	case func() any:
		return f()
	case func(any) any:
		return f(arg(args, 0))
	case func(any):
		f(arg(args, 0))
		return nil
	case func(any, any) any:
		return f(arg(args, 0), arg(args, 1))
	case func(any, any):
		f(arg(args, 0), arg(args, 1))
		return nil
	case *Array:
		if f.Len() == 2 {
			return Call(f.Get(0), ToString(f.Get(1)), args...)
		}
	}
	if IsNull(fn) {
		Throw(NewError("Error", "Value not callable"))
	}
	rv := reflect.ValueOf(fn)
	if rv.Kind() != reflect.Func {
		Throw(NewError("Error", "Value of type "+TypeName(fn)+" is not callable"))
	}
	return callValue(rv, args)
}

func arg(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// callValue calls a function value with PHP arguments, converting them to the parameter types,
// and returns its first result (or nil). A trailing error result is thrown.
func callValue(fn reflect.Value, args []any) any {
	t := fn.Type()
	n := t.NumIn()
	var in []reflect.Value
	for i := 0; i < n; i++ {
		pt := t.In(i)
		if t.IsVariadic() && i == n-1 {
			et := pt.Elem()
			for j := i; j < len(args); j++ {
				v, _ := convertTo(args[j], et)
				in = append(in, v)
			}
			break
		}
		var a any
		if i < len(args) {
			a = args[i]
		}
		v, ok := convertTo(a, pt)
		if !ok {
			v = reflect.Zero(pt)
		}
		in = append(in, v)
	}
	out := fn.Call(in)
	if len(out) == 0 {
		return nil
	}
	if last := out[len(out)-1]; t.Out(len(out)-1) == errorType && !last.IsNil() {
		Throw(last.Interface())
	}
	if len(out) == 2 && t.Out(1).Kind() == reflect.Bool && !out[1].Bool() {
		return nil
	}
	if t.Out(0) == errorType {
		return nil
	}
	return out[0].Interface()
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// fieldByPHPName finds the field a PHP property was converted to, exported or not.
func fieldByPHPName(obj any, name string) (reflect.Value, bool) {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	for _, n := range []string{name, strings.ToUpper(name[:1]) + name[1:], name + "_"} {
		f := v.FieldByName(n)
		if !f.IsValid() {
			continue
		}
		if !f.CanSet() && f.CanAddr() {
			f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		}
		return f, true
	}
	return reflect.Value{}, false
}

// Prop is $obj->name on a value whose type isn't known when converting.
func Prop(obj any, name string) any {
	if a, ok := obj.(*Array); ok {
		return a.Get(name)
	}
	if f, ok := fieldByPHPName(obj, name); ok {
		return f.Interface()
	}
	if m := methodByPHPName(reflect.ValueOf(obj), "get"+strings.ToUpper(name[:1])+name[1:]); m.IsValid() && m.Type().NumIn() == 0 {
		return callValue(m, nil)
	}
	return nil
}

// SetProp is $obj->name = $value on a value whose type isn't known when converting.
func SetProp(obj any, name string, value any) any {
	if f, ok := fieldByPHPName(obj, name); ok {
		if v, ok := convertTo(value, f.Type()); ok {
			f.Set(v)
		}
		return value
	}
	Throw(NewError("Error", fmt.Sprintf("Cannot set property %s::$%s", ClassName(obj), name)))
	return nil
}

// Index is $container[$key] on a value whose type isn't known when converting.
func Index(container any, key any) any {
	switch c := container.(type) {
	case *Array:
		return c.Get(key)
	case string:
		i := ToInt(key)
		if i < 0 {
			i += len(c)
		}
		if i < 0 || i >= len(c) {
			return ""
		}
		return c[i : i+1]
	case nil:
		return nil
	case ArrayAccess:
		return c.OffsetGet(key)
	}
	rv := reflect.ValueOf(container)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		i := ToInt(key)
		if i < 0 || i >= rv.Len() {
			return nil
		}
		return rv.Index(i).Interface()
	case reflect.Map:
		k, ok := convertTo(key, rv.Type().Key())
		if !ok {
			return nil
		}
		v := rv.MapIndex(k)
		if !v.IsValid() {
			return nil
		}
		return v.Interface()
	}
	return nil
}

// SetIndex is $container[$key] = $value. A nil key appends ($container[] = $value).
func SetIndex(container any, key any, value any) any {
	switch c := container.(type) {
	case *Array:
		if key == nil {
			return c.Append(value)
		}
		return c.Set(key, value)
	case ArrayAccess:
		c.OffsetSet(key, value)
		return value
	}
	rv := reflect.ValueOf(container)
	if rv.Kind() == reflect.Map && !rv.IsNil() {
		k, ok1 := convertTo(key, rv.Type().Key())
		v, ok2 := convertTo(value, rv.Type().Elem())
		if ok1 && ok2 {
			rv.SetMapIndex(k, v)
		}
		return value
	}
	if rv.Kind() == reflect.Slice {
		i := ToInt(key)
		if i >= 0 && i < rv.Len() {
			if v, ok := convertTo(value, rv.Type().Elem()); ok {
				rv.Index(i).Set(v)
			}
		}
		return value
	}
	Throw(NewError("Error", "Cannot use a value of type "+TypeName(container)+" as an array"))
	return nil
}

// ArrayAccess is PHP's ArrayAccess interface.
type ArrayAccess interface {
	OffsetExists(offset any) bool
	OffsetGet(offset any) any
	OffsetSet(offset any, value any)
	OffsetUnset(offset any)
}

// Isset is isset($container[$key]) on a value whose type isn't known when converting.
func Isset(container any, key any) bool {
	switch c := container.(type) {
	case *Array:
		return c.Isset(key)
	case ArrayAccess:
		return c.OffsetExists(key)
	case string:
		i := ToInt(key)
		return i >= 0 && i < len(c)
	}
	return !IsNull(Index(container, key))
}

// UnsetIndex is unset($container[$key]).
func UnsetIndex(container any, key any) {
	switch c := container.(type) {
	case *Array:
		c.Unset(key)
		return
	case ArrayAccess:
		c.OffsetUnset(key)
		return
	}
	rv := reflect.ValueOf(container)
	if rv.Kind() == reflect.Map && !rv.IsNil() {
		if k, ok := convertTo(key, rv.Type().Key()); ok {
			rv.SetMapIndex(k, reflect.Value{})
		}
	}
}

// Len is count($v) for any countable value.
func Len(v any) int {
	switch c := v.(type) {
	case *Array:
		return c.Len()
	case nil:
		return 0
	case interface{ Count() int }:
		return c.Count()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.String, reflect.Chan:
		return rv.Len()
	}
	Throw(NewError("TypeError", "count(): Argument #1 ($value) must be of type Countable|array, "+TypeName(v)+" given"))
	return 0
}

// Iter returns the key/value pairs foreach visits for any iterable value.
func Iter(v any) []Entry {
	switch c := v.(type) {
	case *Array:
		return c.Entries()
	case nil:
		return nil
	case []Entry:
		return c
	case interface{ PhpIterate() []Entry }:
		return c.PhpIterate()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]Entry, rv.Len())
		for i := range out {
			out[i] = Entry{i, rv.Index(i).Interface()}
		}
		return out
	case reflect.Map:
		keys := rv.MapKeys()
		sortValues(keys)
		out := make([]Entry, len(keys))
		for i, k := range keys {
			out[i] = Entry{k.Interface(), rv.MapIndex(k).Interface()}
		}
		return out
	case reflect.Pointer, reflect.Struct:
		// Public properties of an object.
		out := []Entry{}
		e := rv
		for e.Kind() == reflect.Pointer {
			if e.IsNil() {
				return nil
			}
			e = e.Elem()
		}
		if e.Kind() == reflect.Struct {
			for i := 0; i < e.NumField(); i++ {
				if f := e.Type().Field(i); f.IsExported() && !f.Anonymous {
					out = append(out, Entry{f.Name, e.Field(i).Interface()})
				}
			}
		}
		return out
	}
	return nil
}

// MethodExists is method_exists($obj, $name).
func MethodExists(obj any, name string) bool {
	return methodByPHPName(reflect.ValueOf(obj), name).IsValid()
}

// PropertyExists is property_exists($obj, $name).
func PropertyExists(obj any, name string) bool {
	_, ok := fieldByPHPName(obj, name)
	return ok
}

// IsCallable is is_callable($v).
func IsCallable(v any) bool {
	if IsNull(v) {
		return false
	}
	if a, ok := v.(*Array); ok && a.Len() == 2 {
		return MethodExists(a.Get(0), ToString(a.Get(1)))
	}
	return reflect.ValueOf(v).Kind() == reflect.Func
}

// IsObject is is_object($v).
func IsObject(v any) bool {
	if IsNull(v) {
		return false
	}
	switch v.(type) {
	case *Array, string, bool, int, float64:
		return false
	}
	k := reflect.ValueOf(v).Kind()
	return k == reflect.Pointer || k == reflect.Struct || k == reflect.Func
}

// SplObjectId is spl_object_id($obj).
func SplObjectId(v any) int {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return int(rv.Pointer())
	}
	return 0
}

// SplObjectHash is spl_object_hash($obj).
func SplObjectHash(v any) string { return fmt.Sprintf("%032x", SplObjectId(v)) }

type spread struct{ v any }

// Spread marks an argument list spread into a call (...$args), for Args.
func Spread(v any) any { return spread{v} }

// Args builds an argument list, expanding Spread values.
func Args(parts ...any) []any {
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		if s, ok := p.(spread); ok {
			for _, e := range Iter(s.v) {
				out = append(out, e.Val)
			}
			continue
		}
		out = append(out, p)
	}
	return out
}
