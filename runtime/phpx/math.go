package phpx

import (
	"math"
	"math/rand/v2"
)

const (
	PHP_INT_MAX                  = math.MaxInt64
	PHP_INT_MIN                  = math.MinInt64
	PHP_INT_SIZE                 = 8
	PHP_FLOAT_EPSILON            = 2.220446049250313e-16
	PHP_FLOAT_MAX                = math.MaxFloat64
	PHP_FLOAT_MIN                = 2.2250738585072014e-308
	M_PI                         = math.Pi
	M_E                          = math.E
	M_SQRT2                      = math.Sqrt2
	PHP_EOL                      = "\n"
	PHP_OS                       = "Linux"
	PHP_OS_FAMILY                = "Linux"
	PHP_VERSION                  = "8.2.0"
	PHP_ROUND_HALF_UP            = 1
	PHP_ROUND_HALF_DOWN          = 2
	PHP_ROUND_HALF_EVEN          = 3
	PHP_ROUND_HALF_ODD           = 4
	DIRECTORY_SEPARATOR          = "/"
	JSON_PRETTY_PRINT            = 128
	JSON_UNESCAPED_SLASHES       = 64
	JSON_UNESCAPED_UNICODE       = 256
	JSON_THROW_ON_ERROR          = 4194304
	JSON_ERROR_NONE              = 0
	JSON_BIGINT_AS_STRING        = 2
	JSON_OBJECT_AS_ARRAY         = 1
	JSON_PRESERVE_ZERO_FRACTION  = 1024
	COUNT_RECURSIVE              = 1
	COUNT_NORMAL                 = 0
	LOCK_EX                      = 2
	FILE_APPEND                  = 8
	FILE_IGNORE_NEW_LINES        = 2
	FILE_SKIP_EMPTY_LINES        = 4
	E_ALL                        = 32767
	E_USER_ERROR                 = 256
	E_USER_WARNING               = 512
	E_USER_NOTICE                = 1024
	E_USER_DEPRECATED            = 16384
	PREG_SPLIT_NO_EMPTY          = 1
	PREG_SPLIT_DELIM_CAPTURE     = 2
	PREG_PATTERN_ORDER           = 1
	PREG_SET_ORDER               = 2
	SCANDIR_SORT_ASCENDING       = 0
	SCANDIR_SORT_DESCENDING      = 1
	SQLITE3_ASSOC                = 1
	SQLITE3_NUM                  = 2
	SQLITE3_BOTH                 = 3
	SQLITE3_INTEGER              = 1
	SQLITE3_FLOAT                = 2
	SQLITE3_TEXT                 = 3
	SQLITE3_BLOB                 = 4
	SQLITE3_NULL                 = 5
	SQLITE3_OPEN_READONLY        = 1
	SQLITE3_OPEN_READWRITE       = 2
	SQLITE3_OPEN_CREATE          = 4
	MYSQLI_ASSOC                 = 1
	MYSQLI_NUM                   = 2
	MYSQLI_BOTH                  = 3
	MYSQLI_STORE_RESULT          = 0
	MYSQLI_REPORT_OFF            = 0
	MYSQLI_REPORT_ERROR          = 1
	MYSQLI_REPORT_STRICT         = 2
	MYSQLI_REPORT_ALL            = 255
	ENT_QUOTES                   = 3
	ENT_COMPAT                   = 2
	ENT_HTML5                    = 48
	ENT_NOQUOTES                 = 0
	PATHINFO_DIRNAME             = 1
	PATHINFO_BASENAME            = 2
	PATHINFO_EXTENSION           = 4
	PATHINFO_FILENAME            = 8
	LOCK_SH                      = 1
	LOCK_UN                      = 3
	LOCK_NB                      = 4
	SEEK_SET                     = 0
	SEEK_CUR                     = 1
	SEEK_END                     = 2
	PREG_OFFSET_CAPTURE          = 256
	PREG_UNMATCHED_AS_NULL       = 512
	PREG_SPLIT_OFFSET_CAPTURE    = 4
	SORT_ASC                     = 4
	SORT_DESC                    = 3
	SORT_LOCALE_STRING           = 5
	E_ERROR                      = 1
	E_WARNING                    = 2
	E_NOTICE                     = 8
	E_DEPRECATED                 = 8192
	E_STRICT                     = 2048
	FILTER_VALIDATE_INT          = 257
	FILTER_VALIDATE_BOOLEAN      = 258
	FILTER_VALIDATE_BOOL         = 258
	FILTER_VALIDATE_FLOAT        = 259
	FILTER_VALIDATE_REGEXP       = 272
	FILTER_VALIDATE_URL          = 273
	FILTER_VALIDATE_EMAIL        = 274
	FILTER_VALIDATE_IP           = 275
	FILTER_DEFAULT               = 516
	FILTER_NULL_ON_FAILURE       = 134217728
	MB_CASE_UPPER                = 0
	MB_CASE_LOWER                = 1
	MB_CASE_TITLE                = 2
	JSON_HEX_TAG                 = 1
	JSON_HEX_QUOT                = 8
	JSON_NUMERIC_CHECK           = 32
	JSON_FORCE_OBJECT            = 16
	JSON_PARTIAL_OUTPUT_ON_ERROR = 512
	JSON_INVALID_UTF8_IGNORE     = 1048576
	JSON_INVALID_UTF8_SUBSTITUTE = 2097152
	M_SQRT1_2                    = 0.70710678118654752440
	M_LN2                        = 0.69314718055994530942
	M_LN10                       = 2.30258509299404568402
	PHP_MAJOR_VERSION            = 8
	PHP_MINOR_VERSION            = 2
	PHP_RELEASE_VERSION          = 0
	PHP_VERSION_ID               = 80200
	PHP_FLOAT_DIG                = 15
	PHP_MAXPATHLEN               = 4096
)

func Abs(v any) any {
	switch n := ToNumber(v).(type) {
	case int:
		if n < 0 {
			return -n
		}
		return n
	case float64:
		return math.Abs(n)
	}
	return 0
}

func Floor(v any) float64 { return math.Floor(ToFloat(v)) }
func Ceil(v any) float64  { return math.Ceil(ToFloat(v)) }

// Round is round($num, $precision).
func Round(v any, precision ...int) float64 {
	f := ToFloat(v)
	p := 0
	if len(precision) > 0 {
		p = precision[0]
	}
	pow := math.Pow(10, float64(p))
	x := f * pow
	if math.IsInf(x, 0) {
		return f
	}
	r := math.Round(x)
	// Fix values like 1.005 * 100 = 100.49999...
	if math.Abs(x-r) == 0.5 {
		r = math.Round(x)
	} else if pre := math.Round(x*1e9) / 1e9; math.Abs(pre-math.Trunc(pre)) == 0.5 {
		r = math.Trunc(pre) + math.Copysign(1, pre)
	}
	return r / pow
}

func Sqrt(v any) float64     { return math.Sqrt(ToFloat(v)) }
func Sin(v any) float64      { return math.Sin(ToFloat(v)) }
func Cos(v any) float64      { return math.Cos(ToFloat(v)) }
func Tan(v any) float64      { return math.Tan(ToFloat(v)) }
func Asin(v any) float64     { return math.Asin(ToFloat(v)) }
func Acos(v any) float64     { return math.Acos(ToFloat(v)) }
func Atan(v any) float64     { return math.Atan(ToFloat(v)) }
func Atan2(y, x any) float64 { return math.Atan2(ToFloat(y), ToFloat(x)) }
func Exp(v any) float64      { return math.Exp(ToFloat(v)) }
func Log10(v any) float64    { return math.Log10(ToFloat(v)) }
func Log2(v any) float64     { return math.Log2(ToFloat(v)) }
func Deg2rad(v any) float64  { return ToFloat(v) * math.Pi / 180 }
func Rad2deg(v any) float64  { return ToFloat(v) * 180 / math.Pi }
func Pi() float64            { return math.Pi }
func Fmod(a, b any) float64  { return math.Mod(ToFloat(a), ToFloat(b)) }
func Hypot(a, b any) float64 { return math.Hypot(ToFloat(a), ToFloat(b)) }
func IsNan(v any) bool       { return math.IsNaN(ToFloat(v)) }
func IsInfinite(v any) bool  { return math.IsInf(ToFloat(v), 0) }
func IsFinite(v any) bool    { f := ToFloat(v); return !math.IsInf(f, 0) && !math.IsNaN(f) }
func Log(v any, base ...any) float64 {
	if len(base) > 0 {
		return math.Log(ToFloat(v)) / math.Log(ToFloat(base[0]))
	}
	return math.Log(ToFloat(v))
}

func Intdiv(a, b int) int {
	if b == 0 {
		Throw(NewError("DivisionByZeroError", "Division by zero"))
	}
	return a / b
}

// PowFn is pow($base, $exp).
func PowFn(a, b any) any { return Pow(a, b) }

// Max is max(...$values) or max($array).
func Max(values ...any) any {
	if len(values) == 1 {
		values = ToArray(values[0]).Values()
	}
	if len(values) == 0 {
		Throw(NewError("ValueError", "max(): Argument #1 ($value) must contain at least one element"))
	}
	m := values[0]
	for _, v := range values[1:] {
		if Compare(v, m) > 0 {
			m = v
		}
	}
	return m
}

// Min is min(...$values) or min($array).
func Min(values ...any) any {
	if len(values) == 1 {
		values = ToArray(values[0]).Values()
	}
	if len(values) == 0 {
		Throw(NewError("ValueError", "min(): Argument #1 ($value) must contain at least one element"))
	}
	m := values[0]
	for _, v := range values[1:] {
		if Compare(v, m) < 0 {
			m = v
		}
	}
	return m
}

// MaxInt / MinInt are max/min for ints.
func MaxInt(a int, b ...int) int {
	for _, x := range b {
		if x > a {
			a = x
		}
	}
	return a
}

func MinInt(a int, b ...int) int {
	for _, x := range b {
		if x < a {
			a = x
		}
	}
	return a
}

func MaxFloat(a float64, b ...float64) float64 {
	for _, x := range b {
		a = math.Max(a, x)
	}
	return a
}

func MinFloat(a float64, b ...float64) float64 {
	for _, x := range b {
		a = math.Min(a, x)
	}
	return a
}

func randRange(args []int) int {
	if len(args) < 2 {
		return rand.IntN(math.MaxInt32)
	}
	lo, hi := args[0], args[1]
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo + rand.IntN(hi-lo+1)
}

func MtRand(args ...int) int   { return randRange(args) }
func Rand(args ...int) int     { return randRange(args) }
func RandomInt(lo, hi int) int { return randRange([]int{lo, hi}) }
func MtGetrandmax() int        { return math.MaxInt32 }
func Getrandmax() int          { return math.MaxInt32 }
func LcgValue() float64        { return rand.Float64() }
func MtSrand(_ ...any)         {}
func Srand(_ ...any)           {}
func RandomBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.IntN(256))
	}
	return string(b)
}

// INF and NAN are PHP's INF and NAN constants.
var (
	INF = math.Inf(1)
	NAN = math.NaN()
)
