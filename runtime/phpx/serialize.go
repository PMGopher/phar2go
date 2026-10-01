package phpx

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// Objects passed through serialize()/igbinary_serialize(): PHP plugins serialize objects to
// send them to async tasks (other threads). Go's tasks share the plugin's memory, so the
// string holds a key to a deep copy of the value, made when serializing (like PHP's copy).
var (
	serialized   = map[int]any{}
	serializedMu sync.Mutex
	serialNext   int
)

const serialPrefix = "\x00phar2go-serialized:"

func serializeObject(v any) string {
	serializedMu.Lock()
	defer serializedMu.Unlock()
	serialNext++
	serialized[serialNext] = deepCopy(v)
	return serialPrefix + strconv.Itoa(serialNext)
}

func unserializeObject(s string) (any, bool) {
	id, err := strconv.Atoi(strings.TrimPrefix(s, serialPrefix))
	if err != nil {
		return nil, false
	}
	serializedMu.Lock()
	v, ok := serialized[id]
	delete(serialized, id)
	serializedMu.Unlock()
	return v, ok
}

// hasObject reports whether v is or contains an object (not only arrays and scalars).
func hasObject(v any) bool {
	switch x := v.(type) {
	case nil, string, int, float64, bool:
		return false
	case *Array:
		for _, e := range x.Values() {
			if hasObject(e) {
				return true
			}
		}
		return false
	}
	return true
}

// deepCopy copies a value with everything it points to, keeping shared and cyclic references
// (an object's "self") shared and cyclic in the copy.
func deepCopy(v any) any {
	if v == nil {
		return nil
	}
	seen := map[uintptr]reflect.Value{}
	return copyValue(reflect.ValueOf(v), seen).Interface()
}

func copyValue(v reflect.Value, seen map[uintptr]reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		if a, ok := v.Interface().(*Array); ok {
			if c, ok := seen[v.Pointer()]; ok {
				return c
			}
			out := NewArray()
			seen[v.Pointer()] = reflect.ValueOf(out)
			for _, e := range a.Entries() {
				var ev any
				if e.Val != nil {
					ev = copyValue(reflect.ValueOf(e.Val), seen).Interface()
				}
				out.Set(e.Key, ev)
			}
			return reflect.ValueOf(out)
		}
		if c, ok := seen[v.Pointer()]; ok {
			return c
		}
		if v.Elem().Kind() == reflect.Func || strings.HasPrefix(v.Elem().Type().PkgPath(), "pocketmine-go/") {
			// Server objects (worlds, players) are shared, not copied.
			return v
		}
		cp := reflect.New(v.Elem().Type())
		seen[v.Pointer()] = cp
		cp.Elem().Set(copyValue(v.Elem(), seen))
		return cp
	case reflect.Struct:
		cp := reflect.New(v.Type()).Elem()
		cp.Set(v)
		for i := 0; i < v.NumField(); i++ {
			f := cp.Field(i)
			switch f.Kind() {
			case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Struct:
			default:
				continue
			}
			src := v.Field(i)
			if !src.CanInterface() {
				src = reflect.NewAt(src.Type(), unsafe.Pointer(src.UnsafeAddr())).Elem()
			}
			if !f.CanSet() {
				f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
			}
			f.Set(copyValue(src, seen))
		}
		return cp
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(copyValue(v.Elem(), seen))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		cp := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			cp.Index(i).Set(copyValue(v.Index(i), seen))
		}
		return cp
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		cp := reflect.MakeMapWithSize(v.Type(), v.Len())
		it := v.MapRange()
		for it.Next() {
			cp.SetMapIndex(it.Key(), copyValue(it.Value(), seen))
		}
		return cp
	}
	return v
}
