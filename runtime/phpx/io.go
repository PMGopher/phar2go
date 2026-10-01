package phpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// JsonEncode is json_encode($value, $flags).
func JsonEncode(v any, flags ...int) any {
	f := 0
	if len(flags) > 0 {
		f = flags[0]
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if f&JSON_PRETTY_PRINT != 0 {
		enc.SetIndent("", "    ")
	}
	if err := enc.Encode(jsonValue(v)); err != nil {
		if f&JSON_THROW_ON_ERROR != 0 {
			Throw(NewException("JsonException", err.Error()))
		}
		return false
	}
	s := strings.TrimRight(buf.String(), "\n")
	if f&JSON_UNESCAPED_SLASHES == 0 {
		s = strings.ReplaceAll(s, "/", `\/`)
	}
	return s
}

// JsonEncodeString is json_encode for callers that need a string.
func JsonEncodeString(v any, flags ...int) string { return ToString(JsonEncode(v, flags...)) }

// JsonDecode is json_decode($json, $associative, $depth, $flags). Objects always decode to arrays.
func JsonDecode(s string, args ...any) any {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		if len(args) > 2 && ToInt(args[2])&JSON_THROW_ON_ERROR != 0 {
			Throw(NewException("JsonException", err.Error()))
		}
		return nil
	}
	return fromJSON(v)
}

func fromJSON(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		a := NewArray()
		for _, k := range sortedKeys(x) {
			a.Set(k, fromJSON(x[k]))
		}
		return a
	case []any:
		a := NewArray()
		for _, e := range x {
			a.Append(fromJSON(e))
		}
		return a
	}
	return v
}

func JsonLastError() int       { return 0 }
func JsonLastErrorMsg() string { return "No error" }

// YamlParse is yaml_parse($yaml).
func YamlParse(s string, _ ...any) any {
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		return false
	}
	return fromGo(v)
}

// YamlParseFile is yaml_parse_file($path).
func YamlParseFile(path string, _ ...any) any {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return YamlParse(string(b))
}

// YamlEmit is yaml_emit($value).
func YamlEmit(v any, _ ...any) string {
	b, err := yaml.Marshal(ToGo(v))
	if err != nil {
		return ""
	}
	return "---\n" + string(b) + "...\n"
}

// Serialize is serialize($value); it uses JSON, which unserialize() reads back.
func Serialize(v any) string { return ToString(JsonEncode(v)) }

// Unserialize reads values written by Serialize.
func Unserialize(s string, _ ...any) any { return JsonDecode(s, true) }

func FileExists(p string) bool { _, err := os.Stat(p); return err == nil }
func IsFile(p string) bool     { st, err := os.Stat(p); return err == nil && !st.IsDir() }
func IsDir(p string) bool      { st, err := os.Stat(p); return err == nil && st.IsDir() }
func IsReadable(p string) bool { return FileExists(p) }
func IsWritable(p string) bool { return FileExists(p) }

func FileGetContents(p string, _ ...any) any {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return httpGet(p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return string(b)
}

func FilePutContents(p string, data any, flags ...int) any {
	var s string
	if a, ok := data.(*Array); ok {
		s = Implode("", a)
	} else {
		s = ToString(data)
	}
	if len(flags) > 0 && flags[0]&FILE_APPEND != 0 {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return false
		}
		defer f.Close()
		n, err := f.WriteString(s)
		if err != nil {
			return false
		}
		return n
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		return false
	}
	return len(s)
}

// File is file($path, $flags): the lines of a file.
func File(p string, flags ...int) any {
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	f := 0
	if len(flags) > 0 {
		f = flags[0]
	}
	out := NewArray()
	lines := strings.SplitAfter(string(b), "\n")
	for _, l := range lines {
		if l == "" {
			continue
		}
		if f&FILE_IGNORE_NEW_LINES != 0 {
			l = strings.TrimRight(l, "\r\n")
		}
		if f&FILE_SKIP_EMPTY_LINES != 0 && strings.TrimRight(l, "\r\n") == "" {
			continue
		}
		out.Append(l)
	}
	return out
}

func Mkdir(p string, args ...any) bool {
	recursive := len(args) > 1 && ToBool(args[1])
	if recursive {
		return os.MkdirAll(p, 0o755) == nil
	}
	return os.Mkdir(p, 0o755) == nil
}

func Rmdir(p string) bool     { return os.Remove(p) == nil }
func Unlink(p string) bool    { return os.Remove(p) == nil }
func Rename(a, b string) bool { return os.Rename(a, b) == nil }
func Copy(a, b string) bool {
	data, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	return os.WriteFile(b, data, 0o644) == nil
}
func Touch(p string, _ ...any) bool {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	f.Close()
	now := time.Now()
	return os.Chtimes(p, now, now) == nil
}
func Filesize(p string) any {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return int(st.Size())
}
func Filemtime(p string) any {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return int(st.ModTime().Unix())
}

func Scandir(p string, order ...int) any {
	entries, err := os.ReadDir(p)
	if err != nil {
		return false
	}
	names := []string{".", ".."}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(order) > 0 && order[0] == SCANDIR_SORT_DESCENDING {
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
	}
	out := NewArray()
	for _, n := range names {
		out.Append(n)
	}
	return out
}

func Glob(pattern string, _ ...int) *Array {
	m, _ := filepath.Glob(pattern)
	out := NewArray()
	for _, p := range m {
		out.Append(filepath.ToSlash(p))
	}
	return out
}

func Basename(p string, suffix ...string) string {
	b := filepath.Base(strings.TrimRight(p, "/\\"))
	if len(suffix) > 0 && strings.HasSuffix(b, suffix[0]) && b != suffix[0] {
		b = strings.TrimSuffix(b, suffix[0])
	}
	return b
}

func Dirname(p string, levels ...int) string {
	n := 1
	if len(levels) > 0 {
		n = levels[0]
	}
	for i := 0; i < n; i++ {
		p = filepath.Dir(strings.TrimRight(p, "/\\"))
	}
	return filepath.ToSlash(p)
}

func Realpath(p string) any {
	a, err := filepath.Abs(p)
	if err != nil || !FileExists(a) {
		return false
	}
	return a
}

func Pathinfo(p string, _ ...int) *Array {
	out := NewArray()
	out.Set("dirname", Dirname(p))
	base := filepath.Base(p)
	out.Set("basename", base)
	if ext := filepath.Ext(base); ext != "" {
		out.Set("extension", ext[1:])
		out.Set("filename", strings.TrimSuffix(base, ext))
	} else {
		out.Set("filename", base)
	}
	return out
}

func Getcwd() string { d, _ := os.Getwd(); return d }

func SysGetTempDir() string { return os.TempDir() }

func Tempnam(dir, prefix string) string {
	f, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return ""
	}
	f.Close()
	return f.Name()
}

// Time is time().
func Time() int { return int(time.Now().Unix()) }

func nowNano() int64 { return time.Now().UnixNano() }

// Microtime is microtime($asFloat).
func Microtime(asFloat ...bool) any {
	now := time.Now()
	if len(asFloat) > 0 && asFloat[0] {
		return float64(now.UnixNano()) / 1e9
	}
	return fmt.Sprintf("%.8f %d", float64(now.Nanosecond())/1e9, now.Unix())
}

// MicrotimeFloat is microtime(true).
func MicrotimeFloat() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// Hrtime is hrtime($asNumber).
func Hrtime(asNumber ...bool) any {
	n := time.Now().UnixNano()
	if len(asNumber) > 0 && asNumber[0] {
		return int(n)
	}
	return List(int(n/1e9), int(n%1e9))
}

func Sleep(s int) int { time.Sleep(time.Duration(s) * time.Second); return 0 }
func Usleep(us int)   { time.Sleep(time.Duration(us) * time.Microsecond) }

// Date is date($format, $timestamp).
func Date(format string, ts ...any) string {
	t := time.Now()
	if len(ts) > 0 && !IsNull(ts[0]) {
		t = time.Unix(int64(ToInt(ts[0])), 0)
	}
	return formatDate(format, t)
}

// Gmdate is gmdate($format, $timestamp).
func Gmdate(format string, ts ...any) string {
	t := time.Now().UTC()
	if len(ts) > 0 && !IsNull(ts[0]) {
		t = time.Unix(int64(ToInt(ts[0])), 0).UTC()
	}
	return formatDate(format, t)
}

func formatDate(format string, t time.Time) string {
	var sb strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		switch c {
		case 'd':
			sb.WriteString(t.Format("02"))
		case 'D':
			sb.WriteString(t.Format("Mon"))
		case 'j':
			sb.WriteString(t.Format("2"))
		case 'l':
			sb.WriteString(t.Format("Monday"))
		case 'N':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			sb.WriteString(fmt.Sprint(wd))
		case 'w':
			sb.WriteString(fmt.Sprint(int(t.Weekday())))
		case 'z':
			sb.WriteString(fmt.Sprint(t.YearDay() - 1))
		case 'F':
			sb.WriteString(t.Format("January"))
		case 'm':
			sb.WriteString(t.Format("01"))
		case 'M':
			sb.WriteString(t.Format("Jan"))
		case 'n':
			sb.WriteString(t.Format("1"))
		case 't':
			sb.WriteString(fmt.Sprint(time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()))
		case 'Y':
			sb.WriteString(t.Format("2006"))
		case 'y':
			sb.WriteString(t.Format("06"))
		case 'a':
			sb.WriteString(t.Format("pm"))
		case 'A':
			sb.WriteString(t.Format("PM"))
		case 'g':
			sb.WriteString(t.Format("3"))
		case 'G':
			sb.WriteString(fmt.Sprint(t.Hour()))
		case 'h':
			sb.WriteString(t.Format("03"))
		case 'H':
			sb.WriteString(t.Format("15"))
		case 'i':
			sb.WriteString(t.Format("04"))
		case 's':
			sb.WriteString(t.Format("05"))
		case 'v':
			sb.WriteString(fmt.Sprintf("%03d", t.Nanosecond()/1e6))
		case 'u':
			sb.WriteString(fmt.Sprintf("%06d", t.Nanosecond()/1e3))
		case 'e':
			sb.WriteString(t.Location().String())
		case 'T':
			sb.WriteString(t.Format("MST"))
		case 'P':
			sb.WriteString(t.Format("-07:00"))
		case 'O':
			sb.WriteString(t.Format("-0700"))
		case 'U':
			sb.WriteString(fmt.Sprint(t.Unix()))
		case 'c':
			sb.WriteString(t.Format(time.RFC3339))
		case 'r':
			sb.WriteString(t.Format(time.RFC1123Z))
		case 'S':
			d := t.Day()
			switch {
			case d == 1 || d == 21 || d == 31:
				sb.WriteString("st")
			case d == 2 || d == 22:
				sb.WriteString("nd")
			case d == 3 || d == 23:
				sb.WriteString("rd")
			default:
				sb.WriteString("th")
			}
		case '\\':
			if i+1 < len(format) {
				i++
				sb.WriteByte(format[i])
			}
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// Mktime is mktime($hour, $minute, $second, $month, $day, $year).
func Mktime(args ...int) int {
	now := time.Now()
	v := []int{now.Hour(), now.Minute(), now.Second(), int(now.Month()), now.Day(), now.Year()}
	copy(v, args)
	return int(time.Date(v[5], time.Month(v[3]), v[4], v[0], v[1], v[2], 0, time.Local).Unix())
}

// Strtotime is strtotime($datetime): common absolute formats and "+N unit" offsets.
func Strtotime(s string, base ...int) any {
	now := time.Now()
	if len(base) > 0 {
		now = time.Unix(int64(base[0]), 0)
	}
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "now", "":
		return int(now.Unix())
	case "today":
		y, m, d := now.Date()
		return int(time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix())
	case "tomorrow":
		y, m, d := now.Date()
		return int(time.Date(y, m, d+1, 0, 0, 0, 0, now.Location()).Unix())
	case "yesterday":
		y, m, d := now.Date()
		return int(time.Date(y, m, d-1, 0, 0, 0, 0, now.Location()).Unix())
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "2006/01/02", "02-01-2006", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return int(t.Unix())
		}
	}
	// "+1 day", "-2 hours", "+1 week 2 days"
	fields := strings.Fields(s)
	t := now
	ok := false
	for i := 0; i+1 < len(fields); i += 2 {
		n := ToInt(fields[i])
		unit := strings.TrimSuffix(fields[i+1], "s")
		switch unit {
		case "sec", "second":
			t = t.Add(time.Duration(n) * time.Second)
		case "min", "minute":
			t = t.Add(time.Duration(n) * time.Minute)
		case "hour":
			t = t.Add(time.Duration(n) * time.Hour)
		case "day":
			t = t.AddDate(0, 0, n)
		case "week":
			t = t.AddDate(0, 0, 7*n)
		case "month":
			t = t.AddDate(0, n, 0)
		case "year":
			t = t.AddDate(n, 0, 0)
		default:
			return false
		}
		ok = true
	}
	if !ok {
		return false
	}
	return int(t.Unix())
}

func DateDefaultTimezoneSet(tz string) bool {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false
	}
	time.Local = loc
	return true
}

func DateDefaultTimezoneGet() string { return time.Local.String() }

func Checkdate(month, day, year int) bool {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.Year() == year && int(t.Month()) == month && t.Day() == day
}

// Getenv is getenv($name).
func Getenv(name string, _ ...any) any {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	return v
}

func PhpUname(_ ...string) string      { return "Linux" }
func PhpSapiName() string              { return "cli" }
func Phpversion(_ ...string) string    { return PHP_VERSION }
func MemoryGetUsage(_ ...bool) int     { return 0 }
func MemoryGetPeakUsage(_ ...bool) int { return 0 }
func GcCollectCycles() int             { return 0 }
func SetTimeLimit(_ int) bool          { return true }
func IniSet(_ string, _ any) any       { return false }
func IniGet(_ string) any              { return false }
func ErrorReporting(_ ...int) int      { return E_ALL }
func FunctionExists(name string) bool  { return knownFunctions[strings.ToLower(name)] }
func ExtensionLoaded(_ string) bool    { return false }
func ClassExists(name string, _ ...bool) bool {
	return knownClasses[strings.ToLower(strings.TrimPrefix(name, "\\"))]
}
func InterfaceExists(name string, _ ...bool) bool { return ClassExists(name) }
func TraitExists(name string, _ ...bool) bool     { return ClassExists(name) }
func EnumExists(name string, _ ...bool) bool      { return ClassExists(name) }

var (
	knownClasses   = map[string]bool{}
	knownFunctions = map[string]bool{}
)

// RegisterClasses records the classes and functions of the converted plugin (and the server
// classes it uses), for class_exists() and function_exists().
func RegisterClasses(classes []string, functions []string) {
	for _, c := range classes {
		knownClasses[strings.ToLower(c)] = true
	}
	for _, f := range functions {
		knownFunctions[strings.ToLower(f)] = true
	}
}
func TriggerError(msg string, _ ...int) bool {
	fmt.Fprintln(os.Stderr, msg)
	return true
}
func ErrorLog(msg string, _ ...any) bool {
	fmt.Fprintln(os.Stderr, msg)
	return true
}

// Exit is exit()/die(): it can't stop the server, so it throws.
func Exit(v ...any) {
	msg := "exit() called"
	if len(v) > 0 {
		if s, ok := v[0].(string); ok {
			msg = s
		}
	}
	Throw(NewError("Error", msg))
}

var classParents = map[string][]string{}

// RegisterParents records the parent classes and interfaces of the plugin's classes.
func RegisterParents(parents map[string][]string) {
	for c, ps := range parents {
		l := make([]string, len(ps))
		for i, p := range ps {
			l[i] = strings.ToLower(p)
		}
		classParents[strings.ToLower(c)] = l
	}
}

// IsA is is_a($objectOrClass, $class, $allowString).
func IsA(v any, class string, allowString ...bool) bool {
	name := ""
	if s, ok := v.(string); ok {
		if len(allowString) == 0 || !allowString[0] {
			return false
		}
		name = s
	} else {
		name = ClassName(v)
	}
	return isA(name, class, true)
}

// IsSubclassOf is is_subclass_of($objectOrClass, $class).
func IsSubclassOf(v any, class string, _ ...bool) bool {
	name, ok := v.(string)
	if !ok {
		name = ClassName(v)
	}
	return isA(name, class, false)
}

func isA(name, class string, self bool) bool {
	name = strings.ToLower(strings.TrimPrefix(name, "\\"))
	class = strings.ToLower(strings.TrimPrefix(class, "\\"))
	if name == class {
		return self
	}
	for _, p := range classParents[name] {
		if p == class {
			return true
		}
	}
	return false
}

// ClassImplements is class_implements($objectOrClass).
func ClassImplements(v any, _ ...bool) *Array {
	name, ok := v.(string)
	if !ok {
		name = ClassName(v)
	}
	out := NewArray()
	for _, p := range classParents[strings.ToLower(name)] {
		out.Set(p, p)
	}
	return out
}

// GetParentClass is get_parent_class($objectOrClass).
func GetParentClass(v any) any {
	name, ok := v.(string)
	if !ok {
		name = ClassName(v)
	}
	if ps := classParents[strings.ToLower(name)]; len(ps) > 0 {
		return ps[0]
	}
	return false
}
