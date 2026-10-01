package phpx

import (
	"fmt"
	"io/fs"
	"math"
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
	if _, ok := obj.(*StubObject); ok {
		return nil
	}
	if IsNull(obj) {
		switch strings.ToLower(method) {
		case "starttiming", "stoptiming", "time":
			// PocketMine-MP's timings (TimingsHandler): pocketmine-go has its own.
			return nil
		}
		Throw(NewError("Error", fmt.Sprintf("Call to a member function %s() on null", method)))
	}
	if fi, ok := obj.(fs.FileInfo); ok {
		if v, ok := fileInfoMethod(fi, method); ok {
			return v
		}
	}
	m := methodByPHPName(reflect.ValueOf(obj), method)
	if !m.IsValid() {
		if fn, ok := methodShims[reflect.TypeOf(obj).String()+"::"+strings.ToLower(method)]; ok {
			return fn(obj, args...)
		}
		// $world->getServer(), $entity->getServer(): there is one server.
		if strings.EqualFold(method, "getServer") && len(args) == 0 && Server != nil {
			return Server()
		}
		// $world->getFullLight($pos) -> world.GetFullLightAt(x, y, z).
		if len(args) == 1 {
			if at := methodByPHPName(reflect.ValueOf(obj), method+"At"); at.IsValid() && at.Type().NumIn() == 3 {
				x, y, z := BlockXYZ(args[0])
				return callValue(at, []any{x, y, z})
			}
		}
		// $pos->asVector3(): the value itself, or the embedded Vector3 of a Location.
		if len(args) == 0 && len(method) > 2 && strings.EqualFold(method[:2], "as") {
			want := method[2:]
			rv := reflect.ValueOf(obj)
			for rv.Kind() == reflect.Pointer && !rv.IsNil() {
				rv = rv.Elem()
			}
			if strings.EqualFold(rv.Type().Name(), want) {
				return rv.Interface()
			}
			if rv.Kind() == reflect.Struct {
				if fv := rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, want) }); fv.IsValid() && fv.CanInterface() {
					return fv.Interface()
				}
			}
		}
		// A getter of a field: $pos->getX() -> pos.X.
		if len(args) == 0 && len(method) > 3 && strings.EqualFold(method[:3], "get") {
			rv := reflect.ValueOf(obj)
			for rv.Kind() == reflect.Pointer && !rv.IsNil() {
				rv = rv.Elem()
			}
			if rv.Kind() == reflect.Struct {
				if fv := rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, method[3:]) }); fv.IsValid() && fv.CanInterface() {
					return FromGo(fv.Interface())
				}
			}
		}
		Throw(NewError("Error", fmt.Sprintf("Call to undefined method %s::%s()", ClassName(obj), method)))
	}
	if defs, ok := dynamicDefaults[strings.ToLower(method)]; ok && len(args) < m.Type().NumIn() {
		for i := len(args); i < len(defs) && i < m.Type().NumIn(); i++ {
			args = append(args, defs[i])
		}
	}
	return callValue(m, args)
}

// dynamicDefaults are PHP default arguments of common PocketMine-MP methods, for calls on
// values whose type isn't known when converting ($pos->up() is $pos->up(1)).
var dynamicDefaults = map[string][]any{
	"up": {1}, "down": {1}, "north": {1}, "south": {1}, "east": {1}, "west": {1},
	"getside": {nil, 1},
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
			if class, ok := f.Get(0).(string); ok {
				return CallStatic(class, ToString(f.Get(1)), args...)
			}
			return Call(f.Get(0), ToString(f.Get(1)), args...)
		}
	case string:
		if target, ok := stringCallable(f); ok {
			return Invoke(target, args...)
		}
		Throw(NewError("Error", "Call to undefined function "+f+"()"))
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
	if _, ok := obj.(*StubObject); ok {
		return value
	}
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
	case interface {
		Rewind()
		Valid() bool
		Current() any
		Key() any
		Next()
	}:
		var out []Entry
		for c.Rewind(); c.Valid(); c.Next() {
			out = append(out, Entry{c.Key(), c.Current()})
		}
		return out
	case interface{ GetIterator() any }:
		return Iter(c.GetIterator())
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

// Callable is $obj->method(...) (a first-class callable) on a value whose type isn't known
// when converting.
func Callable(obj any, method string) func(args ...any) any {
	return func(args ...any) any { return Call(obj, method, args...) }
}

// StaticCallable is Class::method(...) with a class known at run time.
func StaticCallable(class any, method string) func(args ...any) any {
	return func(args ...any) any { return CallStatic(class, method, args...) }
}

// StubObject stands for an object of a PocketMine-MP class that pocketmine-go doesn't have and
// that only affects what clients see (network packets and their data types). Its methods do
// nothing and return null.
type StubObject struct{ Class string }

func (s *StubObject) PhpClass() string { return s.Class }

// Stub creates a StubObject (new Packet(...), Packet::create(...)), warning once per class.
func Stub(what string, _ ...any) any {
	if _, seen := skipped.LoadOrStore("stub:"+what, true); !seen {
		LogWarning("phar2go: " + what + " has no pocketmine-go equivalent; it is ignored")
	}
	class := what
	if i := strings.Index(what, "::"); i >= 0 {
		class = what[:i]
	}
	return &StubObject{Class: strings.TrimPrefix(class, "new ")}
}

var functions = map[string]any{}

// RegisterFunction records a function of the converted plugin, for callables given as strings.
func RegisterFunction(name string, fn any) { functions[strings.ToLower(name)] = fn }

// Phar2goNew is `new $class(...$args)` for phar2go's PHP helpers (ReflectionClass).
func Phar2goNew(class any, args ...any) any { return New(class, args...) }

// stringCallable resolves a callable given as a string: "function" or "Class::method".
func stringCallable(name string) (any, bool) {
	if i := strings.Index(name, "::"); i >= 0 {
		class, method := name[:i], name[i+2:]
		return func(args ...any) any { return CallStatic(class, method, args...) }, true
	}
	l := strings.ToLower(strings.TrimPrefix(name, "\\"))
	if fn, ok := functions[l]; ok {
		return fn, true
	}
	if fn, ok := builtinFunctions[l]; ok {
		return fn, true
	}
	return nil, false
}

// builtinFunctions are the PHP functions plugins commonly pass as callables
// (array_map("trim", ...), usort($a, "strcmp"), ...).
var builtinFunctions map[string]any

func init() {
	builtinFunctions = map[string]any{
		"strtolower": Strtolower, "strtoupper": Strtoupper, "ucfirst": Ucfirst, "lcfirst": Lcfirst,
		"ucwords": Ucwords, "trim": Trim, "ltrim": Ltrim, "rtrim": Rtrim, "strlen": Strlen,
		"strrev": Strrev, "strval": Strval, "intval": Intval, "floatval": Floatval, "boolval": Boolval,
		"is_numeric": IsNumeric, "is_string": IsString, "is_int": IsInt, "is_float": IsFloat,
		"is_bool": IsBool, "is_array": IsArray, "is_null": IsNull, "is_object": IsObject,
		"is_callable": IsCallable, "abs": Abs, "floor": Floor, "ceil": Ceil, "round": Round,
		"sqrt": Sqrt, "strcmp": Strcmp, "strcasecmp": Strcasecmp, "strnatcmp": Strnatcmp,
		"strnatcasecmp": Strnatcasecmp, "count": Count, "json_encode": JsonEncode,
		"json_decode": JsonDecode, "base64_encode": Base64Encode, "base64_decode": Base64Decode,
		"md5": Md5, "sha1": Sha1, "crc32": Crc32, "htmlspecialchars": Htmlspecialchars,
		"strip_tags": StripTags, "addslashes": Addslashes, "stripslashes": Stripslashes,
		"array_sum": ArraySum, "array_values": ArrayValues, "array_keys": ArrayKeys,
		"array_unique": ArrayUnique, "array_reverse": ArrayReverse, "array_filter": ArrayFilter,
		"max": Max, "min": Min, "implode": Implode, "explode": Explode, "str_repeat": StrRepeat,
		"mb_strtolower": MbStrtolower, "mb_strtoupper": MbStrtoupper, "mb_strlen": MbStrlen,
		"nl2br": Nl2br, "ord": Ord, "chr": Chr, "dechex": Dechex, "hexdec": Hexdec,
		"bin2hex": Bin2hex, "var_dump": VarDump, "print_r": PrintR, "serialize": Serialize,
		"unserialize": Unserialize, "spl_object_id": SplObjectId, "spl_object_hash": SplObjectHash,
		"gettype": Gettype, "get_class": GetClass, "file_exists": FileExists, "is_dir": IsDir,
		"is_file": IsFile, "unlink": Unlink, "basename": Basename, "dirname": Dirname,
		"array_merge": ArrayMerge, "in_array": InArray, "sprintf": Sprintf, "microtime": Microtime,
		"time": Time, "mt_rand": MtRand, "rand": Rand,
	}
}

// fileInfoMethod is SplFileInfo's methods on the fs.FileInfo values Go APIs return (like
// getResources()).
func fileInfoMethod(fi fs.FileInfo, method string) (any, bool) {
	switch strings.ToLower(method) {
	case "getfilename", "getbasename", "getpathname", "getrealpath", "__tostring":
		return fi.Name(), true
	case "getextension":
		name := fi.Name()
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			return name[i+1:], true
		}
		return "", true
	case "getsize":
		return int(fi.Size()), true
	case "isdir":
		return fi.IsDir(), true
	case "isfile":
		return fi.Mode().IsRegular(), true
	case "getmtime", "getctime", "getatime":
		return int(fi.ModTime().Unix()), true
	case "isreadable":
		return true, true
	}
	return nil, false
}

// ClosureFromCallable is Closure::fromCallable(): any PHP callable as a function.
func ClosureFromCallable(fn any) func(args ...any) any {
	return func(args ...any) any { return Invoke(fn, args...) }
}

// methodShims are PocketMine-MP methods that pocketmine-go has as constants or unexported
// methods, by Go type and lower-case PHP name.
var methodShims = map[string]func(obj any, args ...any) any{}

func init() {
	methodShims["*world.World::getminy"] = func(any, ...any) any { return -64 }
	methodShims["*world.World::getmaxy"] = func(any, ...any) any { return 320 }
	methodShims["*world.World::getpotentialblockskylightat"] = func(w any, args ...any) any {
		return WorldPotentialBlockSkyLightAt(w, arg(args, 0), arg(args, 1), arg(args, 2))
	}
}

// BlockXYZ is the block coordinates (floored) of a position (a Vector3, Position or Location).
func BlockXYZ(pos any) (int, int, int) {
	rv := reflect.ValueOf(pos)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return 0, 0, 0
	}
	get := func(n string) int {
		if f := rv.FieldByName(n); f.IsValid() && f.CanFloat() {
			return int(math.Floor(f.Float()))
		}
		return ToInt(Call(pos, "get"+n))
	}
	return get("X"), get("Y"), get("Z")
}

// Server returns the server the plugin runs in (set by the plugin's plugin.go).
var Server func() any
