// Package phpx is the runtime support of plugins converted from PHP by phar2go: PHP arrays,
// PHP's loose types and comparisons, exceptions, and the PHP standard library functions the
// plugin used.
//
// Code written by hand doesn't need it; it is what keeps the converted code close to the PHP it
// came from. It may only use the standard library and gopkg.in/yaml.v3 (which the server already
// depends on).
package phpx

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

// Array is a PHP array: an ordered map with int and string keys.
//
// Arrays are values in PHP. A *Array is shared when copied, so converted code clones an array
// where PHP would copy it.
type Array struct {
	entries []*entry
	index   map[any]*entry
	next    int
	live    int
}

type entry struct {
	key     any
	val     any
	deleted bool
}

// Entry is a key/value pair of an Array.
type Entry struct {
	Key any
	Val any
}

// NewArray returns an empty array.
func NewArray() *Array { return &Array{index: map[any]*entry{}} }

// List returns a list ([a, b, c]).
func List(values ...any) *Array {
	a := &Array{index: make(map[any]*entry, len(values))}
	for _, v := range values {
		a.Append(v)
	}
	return a
}

// Map returns an array from key/value pairs: Map(k1, v1, k2, v2, ...).
func Map(kv ...any) *Array {
	a := &Array{index: make(map[any]*entry, len(kv)/2)}
	for i := 0; i+1 < len(kv); i += 2 {
		if _, next := kv[i].(nextKey); next {
			a.Append(kv[i+1])
			continue
		}
		a.Set(kv[i], kv[i+1])
	}
	return a
}

type nextKey struct{}

// NextKey is a key for Map meaning "the next integer key" (an element without a key).
var NextKey any = nextKey{}

// NormKey converts a value to an array key like PHP does: integers and integer strings become
// int, floats are truncated, bools become 0/1 and null becomes "".
func NormKey(k any) any {
	switch v := k.(type) {
	case int:
		return v
	case string:
		if n, ok := intKey(v); ok {
			return n
		}
		return v
	case nil:
		return ""
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int64:
		return int(v)
	case int32:
		return int(v)
	case int16:
		return int(v)
	case int8:
		return int(v)
	case uint:
		return int(v)
	case uint64:
		return int(v)
	case uint32:
		return int(v)
	case uint16:
		return int(v)
	case uint8:
		return int(v)
	}
	if s, ok := k.(interface{ String() string }); ok {
		return NormKey(s.String())
	}
	return ToString(k)
}

func intKey(s string) (int, bool) {
	if s == "" || len(s) > 20 {
		return 0, false
	}
	if s[0] == '0' && len(s) > 1 {
		return 0, false
	}
	if s[0] == '-' && (len(s) == 1 || s[1] == '0') {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && !(i == 0 && c == '-') {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func (a *Array) init() {
	if a.index == nil {
		a.index = map[any]*entry{}
	}
}

// Len is count($a).
func (a *Array) Len() int {
	if a == nil {
		return 0
	}
	return a.live
}

// Get returns $a[k], or nil if it isn't set.
func (a *Array) Get(k any) any {
	if a == nil || a.index == nil {
		return nil
	}
	if e, ok := a.index[NormKey(k)]; ok {
		return e.val
	}
	return nil
}

// Lookup returns $a[k] and whether the key exists.
func (a *Array) Lookup(k any) (any, bool) {
	if a == nil || a.index == nil {
		return nil, false
	}
	e, ok := a.index[NormKey(k)]
	if !ok {
		return nil, false
	}
	return e.val, true
}

// Has is array_key_exists(k, $a).
func (a *Array) Has(k any) bool {
	if a == nil || a.index == nil {
		return false
	}
	_, ok := a.index[NormKey(k)]
	return ok
}

// Isset is isset($a[k]): the key exists and the value isn't null.
func (a *Array) Isset(k any) bool {
	v, ok := a.Lookup(k)
	return ok && !IsNull(v)
}

// Set is $a[k] = v. It returns v.
func (a *Array) Set(k any, v any) any {
	a.init()
	key := NormKey(k)
	if e, ok := a.index[key]; ok {
		e.val = v
		return v
	}
	e := &entry{key: key, val: v}
	a.entries = append(a.entries, e)
	a.index[key] = e
	a.live++
	if n, ok := key.(int); ok && n >= a.next {
		a.next = n + 1
	}
	return v
}

// Append is $a[] = v. It returns v.
func (a *Array) Append(v any) any {
	return a.Set(a.next, v)
}

// Unset is unset($a[k]).
func (a *Array) Unset(k any) {
	if a == nil || a.index == nil {
		return
	}
	key := NormKey(k)
	if e, ok := a.index[key]; ok {
		e.deleted = true
		delete(a.index, key)
		a.live--
		if len(a.entries) > 32 && a.live < len(a.entries)/2 {
			a.compact()
		}
	}
}

func (a *Array) compact() {
	out := make([]*entry, 0, a.live)
	for _, e := range a.entries {
		if !e.deleted {
			out = append(out, e)
		}
	}
	a.entries = out
}

// Entries returns the key/value pairs in order. Iterating it is foreach, which works on a copy.
func (a *Array) Entries() []Entry {
	if a == nil {
		return nil
	}
	out := make([]Entry, 0, a.live)
	for _, e := range a.entries {
		if !e.deleted {
			out = append(out, Entry{e.key, e.val})
		}
	}
	return out
}

// Keys is array_keys($a).
func (a *Array) Keys() []any {
	if a == nil {
		return nil
	}
	out := make([]any, 0, a.live)
	for _, e := range a.entries {
		if !e.deleted {
			out = append(out, e.key)
		}
	}
	return out
}

// Values is array_values($a) as a Go slice.
func (a *Array) Values() []any {
	if a == nil {
		return nil
	}
	out := make([]any, 0, a.live)
	for _, e := range a.entries {
		if !e.deleted {
			out = append(out, e.val)
		}
	}
	return out
}

// Clone returns a copy of the array (PHP's assignment by value). Nested arrays are copied too.
func (a *Array) Clone() *Array {
	if a == nil {
		return nil
	}
	c := &Array{index: make(map[any]*entry, a.live), next: a.next}
	c.entries = make([]*entry, 0, a.live)
	for _, e := range a.entries {
		if e.deleted {
			continue
		}
		v := e.val
		if sub, ok := v.(*Array); ok {
			v = sub.Clone()
		}
		ne := &entry{key: e.key, val: v}
		c.entries = append(c.entries, ne)
		c.index[e.key] = ne
		c.live++
	}
	return c
}

// IsList is array_is_list($a).
func (a *Array) IsList() bool {
	i := 0
	for _, e := range a.entries {
		if e.deleted {
			continue
		}
		if n, ok := e.key.(int); !ok || n != i {
			return false
		}
		i++
	}
	return true
}

// First returns the first value, or nil.
func (a *Array) First() any {
	for _, e := range a.entries {
		if !e.deleted {
			return e.val
		}
	}
	return nil
}

// Last returns the last value, or nil.
func (a *Array) Last() any {
	if a == nil {
		return nil
	}
	for i := len(a.entries) - 1; i >= 0; i-- {
		if !a.entries[i].deleted {
			return a.entries[i].val
		}
	}
	return nil
}

// Sub returns $a[k] as an array, creating it if needed ($a[k][] = ... in PHP).
func (a *Array) Sub(k any) *Array {
	if v, ok := a.Lookup(k); ok {
		if sub, ok := v.(*Array); ok {
			return sub
		}
	}
	sub := NewArray()
	a.Set(k, sub)
	return sub
}

// reset rebuilds the array from entries (keys reindexed if renumber).
func (a *Array) reset(entries []Entry, renumber bool) {
	a.entries = a.entries[:0]
	a.index = make(map[any]*entry, len(entries))
	a.live = 0
	a.next = 0
	for _, e := range entries {
		if renumber {
			if _, ok := e.Key.(int); ok {
				a.Append(e.Val)
				continue
			}
		}
		a.Set(e.Key, e.Val)
	}
}

// MarshalJSON encodes the array like json_encode: lists become JSON arrays, other arrays objects.
func (a *Array) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	if a.IsList() {
		return json.Marshal(jsonValues(a.Values()))
	}
	buf := []byte{'{'}
	first := true
	for _, e := range a.Entries() {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		k, _ := json.Marshal(ToString(e.Key))
		buf = append(buf, k...)
		buf = append(buf, ':')
		v, err := json.Marshal(jsonValue(e.Val))
		if err != nil {
			return nil, err
		}
		buf = append(buf, v...)
	}
	return append(buf, '}'), nil
}

func jsonValues(vs []any) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = jsonValue(v)
	}
	return out
}

func jsonValue(v any) any {
	switch x := v.(type) {
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return 0
		}
	case JsonSerializable:
		return jsonValue(x.JsonSerialize())
	}
	return v
}

// JsonSerializable is PHP's JsonSerializable interface.
type JsonSerializable interface {
	JsonSerialize() any
}

// sortEntries sorts entries with less.
func sortEntries(entries []Entry, less func(a, b Entry) bool) {
	sort.SliceStable(entries, func(i, j int) bool { return less(entries[i], entries[j]) })
}

// JSONValue prepares a value for encoding/json the way json_encode sees it.
func JSONValue(v any) any { return jsonValue(v) }
