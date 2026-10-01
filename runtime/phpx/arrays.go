package phpx

import (
	"math/rand/v2"
	"sort"
	"strings"
)

// Count is count($v).
func Count(v any, _ ...int) int { return Len(v) }

// Sizeof is sizeof($v).
func Sizeof(v any) int { return Len(v) }

// InArray is in_array($needle, $haystack, $strict).
func InArray(needle any, haystack any, strict ...bool) bool {
	s := len(strict) > 0 && strict[0]
	for _, e := range Iter(haystack) {
		if s && StrictEq(e.Val, needle) || !s && LooseEq(e.Val, needle) {
			return true
		}
	}
	return false
}

// ArraySearch is array_search($needle, $haystack, $strict): the key, or false.
func ArraySearch(needle any, haystack any, strict ...bool) any {
	s := len(strict) > 0 && strict[0]
	for _, e := range Iter(haystack) {
		if s && StrictEq(e.Val, needle) || !s && LooseEq(e.Val, needle) {
			return e.Key
		}
	}
	return false
}

func ArrayKeyExists(key any, a any) bool {
	if arr, ok := a.(*Array); ok {
		return arr.Has(key)
	}
	for _, e := range Iter(a) {
		if LooseEq(e.Key, key) {
			return true
		}
	}
	return false
}

func KeyExists(key any, a any) bool { return ArrayKeyExists(key, a) }

func ArrayKeys(a any, filter ...any) *Array {
	out := NewArray()
	for _, e := range Iter(a) {
		if len(filter) > 0 && !LooseEq(e.Val, filter[0]) {
			continue
		}
		out.Append(e.Key)
	}
	return out
}

func ArrayValues(a any) *Array {
	out := NewArray()
	for _, e := range Iter(a) {
		out.Append(e.Val)
	}
	return out
}

func ArrayKeyFirst(a any) any {
	es := Iter(a)
	if len(es) == 0 {
		return nil
	}
	return es[0].Key
}

func ArrayKeyLast(a any) any {
	es := Iter(a)
	if len(es) == 0 {
		return nil
	}
	return es[len(es)-1].Key
}

// ArrayMerge is array_merge(...$arrays): string keys are overwritten, int keys renumbered.
func ArrayMerge(arrays ...any) *Array {
	out := NewArray()
	for _, a := range arrays {
		for _, e := range Iter(a) {
			if _, ok := e.Key.(int); ok {
				out.Append(e.Val)
			} else {
				out.Set(e.Key, e.Val)
			}
		}
	}
	return out
}

func ArrayMergeRecursive(arrays ...any) *Array {
	out := NewArray()
	for _, a := range arrays {
		for _, e := range Iter(a) {
			if _, ok := e.Key.(int); ok {
				out.Append(e.Val)
				continue
			}
			if old, ok := out.Lookup(e.Key); ok {
				oa, ok1 := old.(*Array)
				na, ok2 := e.Val.(*Array)
				if ok1 && ok2 {
					out.Set(e.Key, ArrayMergeRecursive(oa, na))
					continue
				}
				if ok1 {
					m := oa.Clone()
					m.Append(e.Val)
					out.Set(e.Key, m)
					continue
				}
				out.Set(e.Key, List(old, e.Val))
				continue
			}
			out.Set(e.Key, e.Val)
		}
	}
	return out
}

func ArrayReplace(arrays ...any) *Array {
	out := NewArray()
	for _, a := range arrays {
		for _, e := range Iter(a) {
			out.Set(e.Key, e.Val)
		}
	}
	return out
}

// ArrayMap is array_map($callback, $array, ...$arrays).
func ArrayMap(callback any, a any, more ...any) *Array {
	out := NewArray()
	if len(more) == 0 {
		es := Iter(a)
		_, strKeys := a.(*Array)
		for _, e := range es {
			var v any
			if IsNull(callback) {
				v = e.Val
			} else {
				v = Invoke(callback, e.Val)
			}
			if strKeys {
				out.Set(e.Key, v)
			} else {
				out.Append(v)
			}
		}
		if arr, ok := a.(*Array); ok && arr.IsList() {
			out.reset(out.Entries(), true)
		}
		return out
	}
	lists := [][]Entry{Iter(a)}
	for _, m := range more {
		lists = append(lists, Iter(m))
	}
	n := 0
	for _, l := range lists {
		if len(l) > n {
			n = len(l)
		}
	}
	for i := 0; i < n; i++ {
		args := make([]any, len(lists))
		for j, l := range lists {
			if i < len(l) {
				args[j] = l[i].Val
			}
		}
		if IsNull(callback) {
			out.Append(List(args...))
		} else {
			out.Append(Invoke(callback, args...))
		}
	}
	return out
}

const (
	ARRAY_FILTER_USE_KEY  = 2
	ARRAY_FILTER_USE_BOTH = 1
)

// ArrayFilter is array_filter($array, $callback, $mode).
func ArrayFilter(a any, args ...any) *Array {
	out := NewArray()
	mode := 0
	if len(args) > 1 {
		mode = ToInt(args[1])
	}
	for _, e := range Iter(a) {
		var keep bool
		switch {
		case len(args) == 0 || IsNull(args[0]):
			keep = ToBool(e.Val)
		case mode == ARRAY_FILTER_USE_KEY:
			keep = ToBool(Invoke(args[0], e.Key))
		case mode == ARRAY_FILTER_USE_BOTH:
			keep = ToBool(Invoke(args[0], e.Val, e.Key))
		default:
			keep = ToBool(Invoke(args[0], e.Val))
		}
		if keep {
			out.Set(e.Key, e.Val)
		}
	}
	return out
}

// ArrayWalk is array_walk($array, $callback): the callback gets the value and the key.
func ArrayWalk(a *Array, callback any, extra ...any) bool {
	for _, e := range a.Entries() {
		args := []any{e.Val, e.Key}
		args = append(args, extra...)
		Invoke(callback, args...)
	}
	return true
}

func ArrayReduce(a any, callback any, initial ...any) any {
	var acc any
	if len(initial) > 0 {
		acc = initial[0]
	}
	for _, e := range Iter(a) {
		acc = Invoke(callback, acc, e.Val)
	}
	return acc
}

// ArrayPush is array_push($array, ...$values).
func ArrayPush(a *Array, values ...any) int {
	for _, v := range values {
		a.Append(v)
	}
	return a.Len()
}

// ArrayPop is array_pop($array).
func ArrayPop(a *Array) any {
	if a == nil || a.Len() == 0 {
		return nil
	}
	es := a.Entries()
	last := es[len(es)-1]
	a.Unset(last.Key)
	a.next = 0
	for _, e := range a.Entries() {
		if n, ok := e.Key.(int); ok && n >= a.next {
			a.next = n + 1
		}
	}
	return last.Val
}

// ArrayShift is array_shift($array).
func ArrayShift(a *Array) any {
	if a == nil || a.Len() == 0 {
		return nil
	}
	es := a.Entries()
	a.reset(es[1:], true)
	return es[0].Val
}

// ArrayUnshift is array_unshift($array, ...$values).
func ArrayUnshift(a *Array, values ...any) int {
	es := a.Entries()
	nes := make([]Entry, 0, len(es)+len(values))
	for i, v := range values {
		nes = append(nes, Entry{i, v})
	}
	a.reset(append(nes, es...), true)
	return a.Len()
}

// ArraySlice is array_slice($array, $offset, $length, $preserveKeys).
func ArraySlice(a any, offset int, args ...any) *Array {
	es := Iter(a)
	start, end := substrRange(len(es), offset, args)
	preserve := len(args) > 1 && ToBool(args[1])
	out := NewArray()
	for _, e := range es[start:end] {
		if _, ok := e.Key.(int); ok && !preserve {
			out.Append(e.Val)
		} else {
			out.Set(e.Key, e.Val)
		}
	}
	return out
}

// ArraySplice is array_splice($array, $offset, $length, $replacement).
func ArraySplice(a *Array, offset int, args ...any) *Array {
	es := a.Entries()
	start, end := substrRange(len(es), offset, args)
	removed := NewArray()
	for _, e := range es[start:end] {
		removed.Append(e.Val)
	}
	var repl []Entry
	if len(args) > 1 {
		for _, e := range Iter(ToArray(args[1])) {
			repl = append(repl, Entry{0, e.Val})
		}
	}
	nes := append(append(append([]Entry{}, es[:start]...), repl...), es[end:]...)
	a.reset(nes, true)
	return removed
}

func ArrayReverse(a any, preserveKeys ...bool) *Array {
	es := Iter(a)
	out := NewArray()
	pk := len(preserveKeys) > 0 && preserveKeys[0]
	for i := len(es) - 1; i >= 0; i-- {
		if _, ok := es[i].Key.(int); ok && !pk {
			out.Append(es[i].Val)
		} else {
			out.Set(es[i].Key, es[i].Val)
		}
	}
	return out
}

func ArrayUnique(a any, _ ...int) *Array {
	out := NewArray()
	var seen []any
outer:
	for _, e := range Iter(a) {
		for _, s := range seen {
			if LooseEq(s, e.Val) && ToString(s) == ToString(e.Val) {
				continue outer
			}
		}
		seen = append(seen, e.Val)
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArrayFlip(a any) *Array {
	out := NewArray()
	for _, e := range Iter(a) {
		out.Set(e.Val, e.Key)
	}
	return out
}

func ArrayCombine(keys, values any) *Array {
	out := NewArray()
	ks, vs := Iter(keys), Iter(values)
	for i := range ks {
		if i < len(vs) {
			out.Set(ks[i].Val, vs[i].Val)
		}
	}
	return out
}

func ArrayFill(start, count int, value any) *Array {
	out := NewArray()
	for i := 0; i < count; i++ {
		out.Set(start+i, value)
	}
	return out
}

func ArrayFillKeys(keys any, value any) *Array {
	out := NewArray()
	for _, e := range Iter(keys) {
		out.Set(e.Val, value)
	}
	return out
}

func ArrayPad(a any, size int, value any) *Array {
	out := ArrayValues(a)
	n := size
	if n < 0 {
		n = -n
	}
	if out.Len() >= n {
		return ToArray(a).Clone()
	}
	pad := make([]any, n-out.Len())
	for i := range pad {
		pad[i] = value
	}
	if size > 0 {
		for _, v := range pad {
			out.Append(v)
		}
		return out
	}
	return List(append(pad, out.Values()...)...)
}

func ArrayColumn(a any, column any, indexKey ...any) *Array {
	out := NewArray()
	for _, e := range Iter(a) {
		var v any
		if IsNull(column) {
			v = e.Val
		} else {
			v = Index(e.Val, column)
			if IsNull(v) && IsObject(e.Val) {
				v = Prop(e.Val, ToString(column))
			}
		}
		if len(indexKey) > 0 && !IsNull(indexKey[0]) {
			out.Set(Index(e.Val, indexKey[0]), v)
		} else {
			out.Append(v)
		}
	}
	return out
}

func ArrayChunk(a any, size int, preserveKeys ...bool) *Array {
	out := NewArray()
	var cur *Array
	pk := len(preserveKeys) > 0 && preserveKeys[0]
	for _, e := range Iter(a) {
		if cur == nil || cur.Len() >= size {
			cur = NewArray()
			out.Append(cur)
		}
		if pk {
			cur.Set(e.Key, e.Val)
		} else {
			cur.Append(e.Val)
		}
	}
	return out
}

func ArrayDiff(a any, others ...any) *Array {
	out := NewArray()
outer:
	for _, e := range Iter(a) {
		for _, o := range others {
			for _, oe := range Iter(o) {
				if ToString(oe.Val) == ToString(e.Val) {
					continue outer
				}
			}
		}
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArrayDiffKey(a any, others ...any) *Array {
	out := NewArray()
outer:
	for _, e := range Iter(a) {
		for _, o := range others {
			if ArrayKeyExists(e.Key, o) {
				continue outer
			}
		}
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArrayDiffAssoc(a any, others ...any) *Array {
	out := NewArray()
outer:
	for _, e := range Iter(a) {
		for _, o := range others {
			oa := ToArray(o)
			if v, ok := oa.Lookup(e.Key); ok && ToString(v) == ToString(e.Val) {
				continue outer
			}
		}
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArrayIntersect(a any, others ...any) *Array {
	out := NewArray()
outer:
	for _, e := range Iter(a) {
		for _, o := range others {
			found := false
			for _, oe := range Iter(o) {
				if ToString(oe.Val) == ToString(e.Val) {
					found = true
					break
				}
			}
			if !found {
				continue outer
			}
		}
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArrayIntersectKey(a any, others ...any) *Array {
	out := NewArray()
outer:
	for _, e := range Iter(a) {
		for _, o := range others {
			if !ArrayKeyExists(e.Key, o) {
				continue outer
			}
		}
		out.Set(e.Key, e.Val)
	}
	return out
}

func ArraySum(a any) any {
	var sum any = 0
	for _, e := range Iter(a) {
		sum = Add(sum, e.Val)
	}
	return sum
}

func ArrayProduct(a any) any {
	var p any = 1
	for _, e := range Iter(a) {
		p = Mul(p, e.Val)
	}
	return p
}

func ArrayCountValues(a any) *Array {
	out := NewArray()
	for _, e := range Iter(a) {
		out.Set(e.Val, ToInt(out.Get(e.Val))+1)
	}
	return out
}

func ArrayIsList(a any) bool { return ToArray(a).IsList() }

func ArrayRand(a any, num ...int) any {
	es := Iter(a)
	if len(es) == 0 {
		Throw(NewError("ValueError", "array_rand(): Argument #1 ($array) cannot be empty"))
	}
	n := 1
	if len(num) > 0 {
		n = num[0]
	}
	if n <= 1 {
		return es[rand.IntN(len(es))].Key
	}
	idx := rand.Perm(len(es))[:min(n, len(es))]
	sort.Ints(idx)
	out := NewArray()
	for _, i := range idx {
		out.Append(es[i].Key)
	}
	return out
}

func Shuffle(a *Array) bool {
	vs := a.Values()
	rand.Shuffle(len(vs), func(i, j int) { vs[i], vs[j] = vs[j], vs[i] })
	a.reset(nil, false)
	for _, v := range vs {
		a.Append(v)
	}
	return true
}

func Range(start, end any, step ...any) *Array {
	out := NewArray()
	if s, ok := start.(string); ok && len(s) == 1 && !isDigit(s[0]) {
		e := ToString(end)
		if e == "" {
			return out
		}
		a, b := s[0], e[0]
		if a <= b {
			for c := int(a); c <= int(b); c++ {
				out.Append(string(rune(c)))
			}
		} else {
			for c := int(a); c >= int(b); c-- {
				out.Append(string(rune(c)))
			}
		}
		return out
	}
	st := any(1)
	if len(step) > 0 {
		st = ToNumber(step[0])
	}
	if IsFloat(ToNumber(start)) || IsFloat(ToNumber(end)) || IsFloat(st) {
		a, b, s := ToFloat(start), ToFloat(end), absf(ToFloat(st))
		if s == 0 {
			s = 1
		}
		if a <= b {
			for x := a; x <= b+1e-9; x += s {
				out.Append(x)
			}
		} else {
			for x := a; x >= b-1e-9; x -= s {
				out.Append(x)
			}
		}
		return out
	}
	a, b, s := ToInt(start), ToInt(end), ToInt(st)
	if s < 0 {
		s = -s
	}
	if s == 0 {
		s = 1
	}
	if a <= b {
		for x := a; x <= b; x += s {
			out.Append(x)
		}
	} else {
		for x := a; x >= b; x -= s {
			out.Append(x)
		}
	}
	return out
}

func sortArray(a *Array, less func(x, y Entry) bool, keepKeys bool) bool {
	if a == nil {
		return true
	}
	es := a.Entries()
	sortEntries(es, less)
	if keepKeys {
		a.reset(es, false)
	} else {
		a.reset(nil, false)
		for _, e := range es {
			a.Append(e.Val)
		}
	}
	return true
}

const (
	SORT_REGULAR       = 0
	SORT_NUMERIC       = 1
	SORT_STRING        = 2
	SORT_NATURAL       = 6
	SORT_FLAG_CASE     = 8
)

func flagLess(flags []int) func(a, b any) bool {
	f := 0
	if len(flags) > 0 {
		f = flags[0]
	}
	switch f &^ SORT_FLAG_CASE {
	case SORT_NUMERIC:
		return func(a, b any) bool { return ToFloat(a) < ToFloat(b) }
	case SORT_STRING:
		if f&SORT_FLAG_CASE != 0 {
			return func(a, b any) bool { return strings.ToLower(ToString(a)) < strings.ToLower(ToString(b)) }
		}
		return func(a, b any) bool { return ToString(a) < ToString(b) }
	case SORT_NATURAL:
		return func(a, b any) bool { return natCompare(ToString(a), ToString(b), f&SORT_FLAG_CASE != 0) < 0 }
	}
	return func(a, b any) bool { return Compare(a, b) < 0 }
}

func Sort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(x.Val, y.Val) }, false)
}
func Rsort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(y.Val, x.Val) }, false)
}
func Asort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(x.Val, y.Val) }, true)
}
func Arsort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(y.Val, x.Val) }, true)
}
func Ksort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(x.Key, y.Key) }, true)
}
func Krsort(a *Array, flags ...int) bool {
	l := flagLess(flags)
	return sortArray(a, func(x, y Entry) bool { return l(y.Key, x.Key) }, true)
}
func Usort(a *Array, cmp any) bool {
	return sortArray(a, func(x, y Entry) bool { return ToInt(Invoke(cmp, x.Val, y.Val)) < 0 }, false)
}
func Uasort(a *Array, cmp any) bool {
	return sortArray(a, func(x, y Entry) bool { return ToInt(Invoke(cmp, x.Val, y.Val)) < 0 }, true)
}
func Uksort(a *Array, cmp any) bool {
	return sortArray(a, func(x, y Entry) bool { return ToInt(Invoke(cmp, x.Key, y.Key)) < 0 }, true)
}
func Natsort(a *Array) bool {
	return sortArray(a, func(x, y Entry) bool { return natCompare(ToString(x.Val), ToString(y.Val), false) < 0 }, true)
}
func Natcasesort(a *Array) bool {
	return sortArray(a, func(x, y Entry) bool { return natCompare(ToString(x.Val), ToString(y.Val), true) < 0 }, true)
}

// Reset returns the first value of an array (reset($array)).
func Reset(a any) any {
	es := Iter(a)
	if len(es) == 0 {
		return false
	}
	return es[0].Val
}

// End returns the last value of an array (end($array)).
func End(a any) any {
	es := Iter(a)
	if len(es) == 0 {
		return false
	}
	return es[len(es)-1].Val
}

func Current(a any) any { return Reset(a) }

func Key(a any) any {
	es := Iter(a)
	if len(es) == 0 {
		return nil
	}
	return es[0].Key
}

func IteratorToArray(it any, preserveKeys ...bool) *Array {
	out := NewArray()
	pk := len(preserveKeys) == 0 || preserveKeys[0]
	for _, e := range Iter(it) {
		if pk {
			out.Set(e.Key, e.Val)
		} else {
			out.Append(e.Val)
		}
	}
	return out
}

// Compact is not supported (it needs variable names); converted code builds the array itself.

// ArrayMapStrings applies f to each string of a list.
func ArrayFlipKeys(a any) *Array { return ArrayFlip(a) }
