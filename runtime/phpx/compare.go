package phpx

import (
	"math"
	"reflect"
	"sort"
	"strings"
)

// StrictEq is $a === $b.
func StrictEq(a, b any) bool {
	if IsNull(a) || IsNull(b) {
		return IsNull(a) && IsNull(b)
	}
	a, b = normScalar(a), normScalar(b)
	switch x := a.(type) {
	case *Array:
		y, ok := b.(*Array)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		if x.Len() != y.Len() {
			return false
		}
		ye := y.Entries()
		for i, e := range x.Entries() {
			if e.Key != ye[i].Key || !StrictEq(e.Val, ye[i].Val) {
				return false
			}
		}
		return true
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Comparable() {
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

// normScalar converts sized Go numbers to int/float64 so they compare like PHP numbers.
func normScalar(v any) any {
	switch x := v.(type) {
	case int, float64, string, bool:
		return v
	case float32:
		return float64(x)
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
		if _, ok := v.(interface{ String() string }); !ok {
			return rv.String()
		}
	}
	return v
}

// LooseEq is $a == $b (PHP 8 rules).
func LooseEq(a, b any) bool { return Compare(a, b) == 0 && looseComparable(a, b) }

// comparable reports false for pairs PHP considers unequal but unordered (e.g. different objects).
func looseComparable(a, b any) bool {
	if IsNull(a) || IsNull(b) {
		return true
	}
	a, b = normScalar(a), normScalar(b)
	if isScalar(a) || isScalar(b) {
		return true
	}
	if _, ok := a.(*Array); ok {
		_, ok2 := b.(*Array)
		return ok2
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Comparable() && a == b {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func isScalar(v any) bool {
	switch v.(type) {
	case int, float64, string, bool:
		return true
	}
	return false
}

// Compare is $a <=> $b (PHP 8 rules).
func Compare(a, b any) int {
	a, b = normScalar(a), normScalar(b)
	// null and bool compare as bools, except null <=> string compares strings.
	if IsNull(a) {
		if s, ok := b.(string); ok {
			return strings.Compare("", s)
		}
		return cmpBool(false, ToBool(b))
	}
	if IsNull(b) {
		if s, ok := a.(string); ok {
			return strings.Compare(s, "")
		}
		return cmpBool(ToBool(a), false)
	}
	if x, ok := a.(bool); ok {
		return cmpBool(x, ToBool(b))
	}
	if y, ok := b.(bool); ok {
		return cmpBool(ToBool(a), y)
	}
	switch x := a.(type) {
	case string:
		switch y := b.(type) {
		case string:
			nx, okx := parseNumericPrefix(x)
			ny, oky := parseNumericPrefix(y)
			if okx && oky {
				return cmpNum(nx, ny)
			}
			return sign(strings.Compare(x, y))
		case int, float64:
			if n, ok := parseNumericPrefix(x); ok {
				return cmpNum(n, y)
			}
			return sign(strings.Compare(x, ToString(y)))
		case *Array:
			return -1
		}
		return sign(strings.Compare(x, ToString(b)))
	case int, float64:
		switch y := b.(type) {
		case int, float64:
			return cmpNum(x, y)
		case string:
			if n, ok := parseNumericPrefix(y); ok {
				return cmpNum(x, n)
			}
			return sign(strings.Compare(ToString(x), y))
		case *Array:
			return -1
		}
		return cmpNum(x, 1)
	case *Array:
		y, ok := b.(*Array)
		if !ok {
			return 1
		}
		if x.Len() != y.Len() {
			return cmpNum(x.Len(), y.Len())
		}
		for _, e := range x.Entries() {
			v, ok := y.Lookup(e.Key)
			if !ok {
				return 1
			}
			if c := Compare(e.Val, v); c != 0 {
				return c
			}
		}
		return 0
	}
	if isScalar(b) {
		return -Compare(b, a)
	}
	if looseComparable(a, b) {
		return 0
	}
	return 1
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

func cmpNum(a, b any) int {
	if x, ok := a.(int); ok {
		if y, ok := b.(int); ok {
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		}
	}
	x, y := ToFloat(a), ToFloat(b)
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// Less is $a < $b.
func Less(a, b any) bool { return Compare(a, b) < 0 }

// LessEq is $a <= $b.
func LessEq(a, b any) bool { return Compare(a, b) <= 0 }

// Greater is $a > $b.
func Greater(a, b any) bool { return Compare(a, b) > 0 }

// GreaterEq is $a >= $b.
func GreaterEq(a, b any) bool { return Compare(a, b) >= 0 }

func arith(a, b any, fi func(x, y int) (int, bool), ff func(x, y float64) float64) any {
	x, y := ToNumber(a), ToNumber(b)
	if xi, ok := x.(int); ok {
		if yi, ok := y.(int); ok && fi != nil {
			if r, ok := fi(xi, yi); ok {
				return r
			}
		}
	}
	return ff(ToFloat(x), ToFloat(y))
}

// Add is $a + $b. Arrays are unioned.
func Add(a, b any) any {
	if x, ok := a.(*Array); ok {
		if y, ok := b.(*Array); ok {
			out := x.Clone()
			for _, e := range y.Entries() {
				if !out.Has(e.Key) {
					out.Set(e.Key, e.Val)
				}
			}
			return out
		}
	}
	return arith(a, b, func(x, y int) (int, bool) {
		r := x + y
		return r, (r > x) == (y > 0) || y == 0
	}, func(x, y float64) float64 { return x + y })
}

// Sub is $a - $b.
func Sub(a, b any) any {
	return arith(a, b, func(x, y int) (int, bool) {
		r := x - y
		return r, (r < x) == (y > 0) || y == 0
	}, func(x, y float64) float64 { return x - y })
}

// Mul is $a * $b.
func Mul(a, b any) any {
	return arith(a, b, func(x, y int) (int, bool) {
		if x == 0 || y == 0 {
			return 0, true
		}
		r := x * y
		return r, r/y == x
	}, func(x, y float64) float64 { return x * y })
}

// Div is $a / $b: an int when the division is exact, else a float.
func Div(a, b any) any {
	x, y := ToNumber(a), ToNumber(b)
	if ToFloat(y) == 0 {
		Throw(NewError("DivisionByZeroError", "Division by zero"))
	}
	if xi, ok := x.(int); ok {
		if yi, ok := y.(int); ok && xi%yi == 0 {
			return xi / yi
		}
	}
	return ToFloat(x) / ToFloat(y)
}

// Mod is $a % $b.
func Mod(a, b any) int {
	y := ToInt(b)
	if y == 0 {
		Throw(NewError("DivisionByZeroError", "Modulo by zero"))
	}
	return ToInt(a) % y
}

// IntMod is $a % $b for ints, throwing on a zero divisor.
func IntMod(a, b int) int {
	if b == 0 {
		Throw(NewError("DivisionByZeroError", "Modulo by zero"))
	}
	return a % b
}

// Pow is $a ** $b.
func Pow(a, b any) any {
	return arith(a, b, func(x, y int) (int, bool) {
		if y < 0 {
			return 0, false
		}
		r := 1
		for i := 0; i < y; i++ {
			n := r * x
			if x != 0 && n/x != r {
				return 0, false
			}
			r = n
		}
		return r, true
	}, math.Pow)
}

// Neg is -$a.
func Neg(a any) any {
	switch x := ToNumber(a).(type) {
	case int:
		return -x
	case float64:
		return -x
	}
	return 0
}

// Inc is $a + 1 for ++ on untyped values (strings are incremented like numbers).
func Inc(a any) any {
	if IsNull(a) {
		return 1
	}
	return Add(a, 1)
}

// Dec is $a - 1 for -- on untyped values.
func Dec(a any) any {
	if IsNull(a) {
		return nil
	}
	return Sub(a, 1)
}

// FloatDiv is $a / $b for numbers, as a float.
func FloatDiv(a, b float64) float64 {
	if b == 0 {
		Throw(NewError("DivisionByZeroError", "Division by zero"))
	}
	return a / b
}

func sortValues(vs []reflect.Value) {
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i].Interface(), vs[j].Interface()) < 0 })
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
