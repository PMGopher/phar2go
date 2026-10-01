package phpx

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"html"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func Strlen(s string) int                    { return len(s) }
func Strtolower(s string) string             { return strings.ToLower(s) }
func Strtoupper(s string) string             { return strings.ToUpper(s) }
func MbStrtolower(s string, _ ...any) string { return strings.ToLower(s) }
func MbStrtoupper(s string, _ ...any) string { return strings.ToUpper(s) }
func MbStrlen(s string, _ ...any) int        { return utf8.RuneCountInString(s) }
func Strrev(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func Ucfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func Lcfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func Ucwords(s string, delimiters ...string) string {
	delims := " \t\r\n\f\v"
	if len(delimiters) > 0 {
		delims = delimiters[0]
	}
	b := []byte(s)
	up := true
	for i, c := range b {
		if up && c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
		up = strings.IndexByte(delims, c) >= 0
	}
	return string(b)
}

const trimDefault = " \t\n\r\x00\x0B"

func trimChars(chars []string) string {
	if len(chars) == 0 {
		return trimDefault
	}
	c := chars[0]
	// Ranges like "a..z".
	for {
		i := strings.Index(c, "..")
		if i <= 0 || i+2 >= len(c) {
			break
		}
		from, to := c[i-1], c[i+2]
		var r strings.Builder
		for ch := int(from); ch <= int(to); ch++ {
			r.WriteByte(byte(ch))
		}
		c = c[:i-1] + r.String() + c[i+3:]
	}
	return c
}

func Trim(s string, chars ...string) string  { return strings.Trim(s, trimChars(chars)) }
func Ltrim(s string, chars ...string) string { return strings.TrimLeft(s, trimChars(chars)) }
func Rtrim(s string, chars ...string) string { return strings.TrimRight(s, trimChars(chars)) }
func Chop(s string, chars ...string) string  { return Rtrim(s, chars...) }

// substrRange resolves PHP's offset/length arguments.
func substrRange(n, offset int, length []any) (int, int) {
	if offset < 0 {
		offset += n
		if offset < 0 {
			offset = 0
		}
	}
	if offset > n {
		return n, n
	}
	end := n
	if len(length) > 0 && !IsNull(length[0]) {
		l := ToInt(length[0])
		if l < 0 {
			end = n + l
			if end < offset {
				end = offset
			}
		} else if offset+l < n {
			end = offset + l
		}
	}
	return offset, end
}

func Substr(s string, offset int, length ...any) string {
	a, b := substrRange(len(s), offset, length)
	return s[a:b]
}

func MbSubstr(s string, offset int, length ...any) string {
	r := []rune(s)
	if len(length) > 1 {
		length = length[:1]
	}
	a, b := substrRange(len(r), offset, length)
	return string(r[a:b])
}

func SubstrCount(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	return strings.Count(haystack, needle)
}

// Strpos returns the position of needle, or false.
func Strpos(haystack, needle string, offset ...int) any {
	off := 0
	if len(offset) > 0 {
		off = offset[0]
		if off < 0 {
			off += len(haystack)
		}
		if off < 0 || off > len(haystack) {
			return false
		}
	}
	i := strings.Index(haystack[off:], needle)
	if i < 0 {
		return false
	}
	return i + off
}

func Stripos(haystack, needle string, offset ...int) any {
	return Strpos(strings.ToLower(haystack), strings.ToLower(needle), offset...)
}

func Strrpos(haystack, needle string, _ ...int) any {
	i := strings.LastIndex(haystack, needle)
	if i < 0 {
		return false
	}
	return i
}

func MbStrpos(haystack, needle string, _ ...any) any {
	i := strings.Index(haystack, needle)
	if i < 0 {
		return false
	}
	return utf8.RuneCountInString(haystack[:i])
}

func Strstr(haystack, needle string, beforeNeedle ...bool) any {
	i := strings.Index(haystack, needle)
	if i < 0 {
		return false
	}
	if len(beforeNeedle) > 0 && beforeNeedle[0] {
		return haystack[:i]
	}
	return haystack[i:]
}

func Stristr(haystack, needle string, beforeNeedle ...bool) any {
	i := strings.Index(strings.ToLower(haystack), strings.ToLower(needle))
	if i < 0 {
		return false
	}
	if len(beforeNeedle) > 0 && beforeNeedle[0] {
		return haystack[:i]
	}
	return haystack[i:]
}

func Strrchr(haystack string, needle string) any {
	if needle == "" {
		return false
	}
	i := strings.LastIndexByte(haystack, needle[0])
	if i < 0 {
		return false
	}
	return haystack[i:]
}

func StrContains(haystack, needle string) bool   { return strings.Contains(haystack, needle) }
func StrStartsWith(haystack, needle string) bool { return strings.HasPrefix(haystack, needle) }
func StrEndsWith(haystack, needle string) bool   { return strings.HasSuffix(haystack, needle) }

func StrRepeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

const (
	STR_PAD_RIGHT = 1
	STR_PAD_LEFT  = 0
	STR_PAD_BOTH  = 2
)

func StrPad(s string, length int, pad ...any) string {
	padStr := " "
	padType := STR_PAD_RIGHT
	if len(pad) > 0 {
		padStr = ToString(pad[0])
	}
	if len(pad) > 1 {
		padType = ToInt(pad[1])
	}
	n := length - len(s)
	if n <= 0 || padStr == "" {
		return s
	}
	mk := func(k int) string {
		return strings.Repeat(padStr, k/len(padStr)+1)[:k]
	}
	switch padType {
	case STR_PAD_LEFT:
		return mk(n) + s
	case STR_PAD_BOTH:
		left := n / 2
		return mk(left) + s + mk(n-left)
	}
	return s + mk(n)
}

func StrSplit(s string, length ...int) *Array {
	l := 1
	if len(length) > 0 && length[0] > 0 {
		l = length[0]
	}
	out := NewArray()
	if s == "" {
		out.Append("")
		return out
	}
	for i := 0; i < len(s); i += l {
		end := i + l
		if end > len(s) {
			end = len(s)
		}
		out.Append(s[i:end])
	}
	return out
}

func MbStrSplit(s string, length ...any) *Array {
	l := 1
	if len(length) > 0 && ToInt(length[0]) > 0 {
		l = ToInt(length[0])
	}
	r := []rune(s)
	out := NewArray()
	for i := 0; i < len(r); i += l {
		end := i + l
		if end > len(r) {
			end = len(r)
		}
		out.Append(string(r[i:end]))
	}
	return out
}

// Explode is explode($separator, $string, $limit).
func Explode(sep string, s string, limit ...int) *Array {
	if sep == "" {
		Throw(NewError("ValueError", "explode(): Argument #1 ($separator) cannot be empty"))
	}
	var parts []string
	if len(limit) > 0 && limit[0] != 0 && limit[0] < (1<<31-1) {
		l := limit[0]
		if l > 0 {
			parts = strings.SplitN(s, sep, l)
		} else {
			parts = strings.Split(s, sep)
			if -l >= len(parts) {
				parts = nil
			} else {
				parts = parts[:len(parts)+l]
			}
		}
	} else {
		parts = strings.Split(s, sep)
	}
	out := &Array{index: make(map[any]*entry, len(parts))}
	for _, p := range parts {
		out.Append(p)
	}
	return out
}

// Implode is implode($separator, $array) (either argument order).
func Implode(a any, b ...any) string {
	var sep string
	var pieces any
	if len(b) == 0 {
		pieces = a
	} else if _, ok := a.(string); ok {
		sep, pieces = a.(string), b[0]
	} else {
		pieces, sep = a, ToString(b[0])
	}
	var sb strings.Builder
	for i, e := range Iter(pieces) {
		if i > 0 {
			sb.WriteString(sep)
		}
		sb.WriteString(ToString(e.Val))
	}
	return sb.String()
}

func Join(a any, b ...any) string { return Implode(a, b...) }

// StrReplace is str_replace($search, $replace, $subject); arrays work for all three.
func StrReplace(search, replace, subject any) any {
	n := 0
	do := func(s string) string {
		if sa, ok := search.(*Array); ok {
			ra, isArr := replace.(*Array)
			rv := ra.Values()
			for i, e := range sa.Values() {
				r := ""
				if isArr {
					if i < len(rv) {
						r = ToString(rv[i])
					}
				} else {
					r = ToString(replace)
				}
				needle := ToString(e)
				if needle == "" {
					continue
				}
				n += strings.Count(s, needle)
				s = strings.ReplaceAll(s, needle, r)
			}
			return s
		}
		needle := ToString(search)
		if needle == "" {
			return s
		}
		n += strings.Count(s, needle)
		return strings.ReplaceAll(s, needle, ToString(replace))
	}
	var out any
	if sub, ok := subject.(*Array); ok {
		res := NewArray()
		for _, e := range sub.Entries() {
			res.Set(e.Key, do(ToString(e.Val)))
		}
		out = res
	} else {
		out = do(ToString(subject))
	}
	_ = n
	return out
}

// StrReplaceString is str_replace on a string subject.
func StrReplaceString(search, replace any, subject string) string {
	return ToString(StrReplace(search, replace, subject))
}

func StrIreplace(search, replace, subject any) any {
	do := func(s, needle, r string) string {
		if needle == "" {
			return s
		}
		var sb strings.Builder
		ls, ln := strings.ToLower(s), strings.ToLower(needle)
		for {
			i := strings.Index(ls, ln)
			if i < 0 {
				sb.WriteString(s)
				return sb.String()
			}
			sb.WriteString(s[:i])
			sb.WriteString(r)
			s, ls = s[i+len(needle):], ls[i+len(needle):]
		}
	}
	s := ToString(subject)
	if sa, ok := search.(*Array); ok {
		for i, e := range sa.Values() {
			r := ToString(replace)
			if ra, ok := replace.(*Array); ok {
				r = ToString(Index(ra, i))
			}
			s = do(s, ToString(e), r)
		}
		return s
	}
	return do(s, ToString(search), ToString(replace))
}

// Strtr is strtr($string, $from, $to) or strtr($string, $replacePairs).
func Strtr(s string, args ...any) string {
	if len(args) == 1 {
		pairs := ToArray(args[0])
		var olds []string
		for _, e := range pairs.Entries() {
			olds = append(olds, ToString(e.Key), ToString(e.Val))
		}
		return strings.NewReplacer(olds...).Replace(s)
	}
	if len(args) < 2 {
		return s
	}
	from, to := ToString(args[0]), ToString(args[1])
	if len(to) < len(from) {
		from = from[:len(to)]
	}
	b := []byte(s)
	for i, c := range b {
		if j := strings.IndexByte(from, c); j >= 0 {
			b[i] = to[j]
		}
	}
	return string(b)
}

func Strcmp(a, b string) int     { return strings.Compare(a, b) }
func Strcasecmp(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }
func Strncmp(a, b string, n int) int {
	if len(a) > n {
		a = a[:n]
	}
	if len(b) > n {
		b = b[:n]
	}
	return strings.Compare(a, b)
}
func Strncasecmp(a, b string, n int) int { return Strncmp(strings.ToLower(a), strings.ToLower(b), n) }
func Strnatcmp(a, b string) int          { return natCompare(a, b, false) }
func Strnatcasecmp(a, b string) int      { return natCompare(a, b, true) }

func natCompare(a, b string, fold bool) int {
	if fold {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			i, j := 0, 0
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			x, _ := strconv.ParseFloat(a[:i], 64)
			y, _ := strconv.ParseFloat(b[:j], 64)
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return sign(len(a) - len(b))
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func Nl2br(s string) string { return strings.ReplaceAll(s, "\n", "<br />\n") }

func Wordwrap(s string, width int, args ...any) string {
	brk := "\n"
	cut := false
	if len(args) > 0 {
		brk = ToString(args[0])
	}
	if len(args) > 1 {
		cut = ToBool(args[1])
	}
	words := strings.Split(s, " ")
	var lines []string
	line := ""
	for _, w := range words {
		for cut && len(w) > width && width > 0 {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, w[:width])
			w = w[width:]
		}
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			lines = append(lines, line)
			line = w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, brk)
}

func Ord(s string) int {
	if s == "" {
		return 0
	}
	return int(s[0])
}

func Chr(n int) string { return string([]byte{byte(((n % 256) + 256) % 256)}) }

func Dechex(n int) string { return strconv.FormatUint(uint64(n), 16) }
func Decbin(n int) string { return strconv.FormatUint(uint64(n), 2) }
func Decoct(n int) string { return strconv.FormatUint(uint64(n), 8) }
func Hexdec(s string) int {
	n, _ := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	return int(n)
}
func Bindec(s string) int     { n, _ := strconv.ParseUint(s, 2, 64); return int(n) }
func Octdec(s string) int     { n, _ := strconv.ParseUint(s, 8, 64); return int(n) }
func Bin2hex(s string) string { return hex.EncodeToString([]byte(s)) }
func Hex2bin(s string) any {
	b, err := hex.DecodeString(s)
	if err != nil {
		return false
	}
	return string(b)
}

func Base64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func Base64Decode(s string, _ ...bool) any {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
		if err != nil {
			return false
		}
	}
	return string(b)
}

func hashHex(h hash.Hash, s string, raw []bool) string {
	h.Write([]byte(s))
	sum := h.Sum(nil)
	if len(raw) > 0 && raw[0] {
		return string(sum)
	}
	return hex.EncodeToString(sum)
}

func Md5(s string, raw ...bool) string  { return hashHex(md5.New(), s, raw) }
func Sha1(s string, raw ...bool) string { return hashHex(sha1.New(), s, raw) }
func Crc32(s string) int                { return int(crc32.ChecksumIEEE([]byte(s))) }

// Hash is hash($algo, $data).
func Hash(algo string, data string, raw ...bool) string {
	switch strings.ToLower(algo) {
	case "md5":
		return Md5(data, raw...)
	case "sha1":
		return Sha1(data, raw...)
	case "sha256":
		return hashHex(sha256.New(), data, raw)
	case "sha512":
		return hashHex(sha512.New(), data, raw)
	case "sha384":
		return hashHex(sha512.New384(), data, raw)
	case "crc32b":
		return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(data)))
	}
	Throw(NewError("ValueError", "hash(): Argument #1 ($algo) must be a valid hashing algorithm"))
	return ""
}

func Htmlspecialchars(s string, _ ...any) string       { return html.EscapeString(s) }
func HtmlspecialcharsDecode(s string, _ ...any) string { return html.UnescapeString(s) }
func Addslashes(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `"`, `\"`, "\x00", `\0`)
	return r.Replace(s)
}
func Stripslashes(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}
func StripTags(s string, _ ...any) string {
	var sb strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func Uniqid(prefix ...any) string {
	p := ""
	if len(prefix) > 0 {
		p = ToString(prefix[0])
	}
	return fmt.Sprintf("%s%013x", p, nowNano()/1000)
}

func StrShuffle(s string) string {
	b := []byte(s)
	rand.Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })
	return string(b)
}

func StrWordCount(s string, _ ...any) int {
	return len(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' && r != '-' }))
}

func CtypeDigit(v any) bool {
	s, ok := v.(string)
	if !ok || s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func ctype(v any, f func(r rune) bool) bool {
	s, ok := v.(string)
	if !ok || s == "" {
		return false
	}
	for _, r := range s {
		if r > 127 || !f(r) {
			return false
		}
	}
	return true
}

func CtypeAlpha(v any) bool { return ctype(v, unicode.IsLetter) }
func CtypeAlnum(v any) bool {
	return ctype(v, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
}
func CtypeUpper(v any) bool { return ctype(v, unicode.IsUpper) }
func CtypeLower(v any) bool { return ctype(v, unicode.IsLower) }
func CtypeSpace(v any) bool { return ctype(v, unicode.IsSpace) }
func CtypePunct(v any) bool { return ctype(v, unicode.IsPunct) }
func CtypeXdigit(v any) bool {
	return ctype(v, func(r rune) bool { return strings.ContainsRune("0123456789abcdefABCDEF", r) })
}

func NumberFormat(n any, args ...any) string {
	decimals := 0
	decPoint, thousands := ".", ","
	if len(args) > 0 {
		decimals = ToInt(args[0])
	}
	if len(args) > 1 {
		decPoint = ToString(args[1])
	}
	if len(args) > 2 {
		thousands = ToString(args[2])
	}
	f := Round(ToFloat(n), decimals)
	s := strconv.FormatFloat(absf(f), 'f', decimals, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	var sb strings.Builder
	if f < 0 {
		sb.WriteByte('-')
	}
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			sb.WriteString(thousands)
		}
		sb.WriteRune(c)
	}
	if decimals > 0 {
		sb.WriteString(decPoint)
		sb.WriteString(frac)
	}
	return sb.String()
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func Intval(v any, base ...int) int {
	if s, ok := v.(string); ok && len(base) > 0 && base[0] != 10 {
		n, _ := strconv.ParseInt(strings.TrimSpace(s), base[0], 64)
		return int(n)
	}
	return ToInt(v)
}
func Floatval(v any) float64  { return ToFloat(v) }
func Doubleval(v any) float64 { return ToFloat(v) }
func Strval(v any) string     { return ToString(v) }
func Boolval(v any) bool      { return ToBool(v) }

func IsString(v any) bool { _, ok := v.(string); return ok }
func IsInt(v any) bool {
	switch normScalar(v).(type) {
	case int:
		return true
	}
	return false
}
func IsInteger(v any) bool { return IsInt(v) }
func IsLong(v any) bool    { return IsInt(v) }
func IsFloat(v any) bool {
	_, ok := normScalar(v).(float64)
	return ok
}
func IsDouble(v any) bool { return IsFloat(v) }
func IsBool(v any) bool   { _, ok := v.(bool); return ok }
func IsArray(v any) bool  { a, ok := v.(*Array); return ok && a != nil }
func IsNullV(v any) bool  { return IsNull(v) }
func IsScalar(v any) bool { return isScalar(normScalar(v)) }
func IsIterable(v any) bool {
	return IsArray(v)
}
func IsCountable(v any) bool { return IsArray(v) }
func Gettype(v any) string {
	switch TypeName(v) {
	case "int":
		return "integer"
	case "float":
		return "double"
	case "bool":
		return "boolean"
	case "null":
		return "NULL"
	case "string", "array":
		return TypeName(v)
	}
	return "object"
}
func GetDebugType(v any) string { return TypeName(v) }
func GetClass(v any) string     { return ClassName(v) }

// FilterVar is filter_var() for the validation filters.
func FilterVar(v any, filter ...int) any {
	f := FILTER_DEFAULT
	if len(filter) > 0 {
		f = filter[0]
	}
	s := strings.TrimSpace(ToString(v))
	switch f {
	case FILTER_VALIDATE_INT:
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		return false
	case FILTER_VALIDATE_FLOAT:
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n
		}
		return false
	case FILTER_VALIDATE_BOOL:
		switch strings.ToLower(s) {
		case "1", "true", "on", "yes":
			return true
		case "0", "false", "off", "no", "":
			return false
		}
		return nil
	case FILTER_VALIDATE_EMAIL:
		if i := strings.IndexByte(s, '@'); i > 0 && strings.Contains(s[i:], ".") {
			return s
		}
		return false
	case FILTER_VALIDATE_URL:
		if strings.Contains(s, "://") {
			return s
		}
		return false
	case FILTER_VALIDATE_IP:
		if net.ParseIP(s) != nil {
			return s
		}
		return false
	}
	return ToString(v)
}

// VersionCompare is version_compare($a, $b, $operator).
func VersionCompare(a, b string, op ...string) any {
	c := cmpVersions(a, b)
	if len(op) == 0 {
		return c
	}
	switch op[0] {
	case "<", "lt":
		return c < 0
	case "<=", "le":
		return c <= 0
	case ">", "gt":
		return c > 0
	case ">=", "ge":
		return c >= 0
	case "==", "eq":
		return c == 0
	case "!=", "<>", "ne":
		return c != 0
	}
	return nil
}

func cmpVersions(a, b string) int {
	norm := func(s string) []string {
		s = strings.NewReplacer("-", ".", "_", ".", "+", ".").Replace(s)
		return strings.Split(s, ".")
	}
	pa, pb := norm(a), norm(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		switch {
		case ex == nil && ey == nil:
			if nx != ny {
				return sign(nx - ny)
			}
		case x == "":
			return -1
		case y == "":
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return 0
}

// MbConvertCase is mb_convert_case().
func MbConvertCase(s string, mode int, _ ...any) string {
	switch mode {
	case MB_CASE_UPPER:
		return strings.ToUpper(s)
	case MB_CASE_LOWER:
		return strings.ToLower(s)
	}
	return Ucwords(strings.ToLower(s))
}

func MbStrwidth(s string, _ ...any) int { return utf8.RuneCountInString(s) }
func MbStrrev(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
func Lcg() float64                { return rand.Float64() }
func Assert(v any, _ ...any) bool { return true }
func Utf8Encode(s string) string  { return s }
func Utf8Decode(s string) string  { return s }
func Quotemeta(s string) string   { return regexpQuote(s) }
func regexpQuote(s string) string {
	return strings.NewReplacer(`.`, `\.`, `\`, `\\`, `+`, `\+`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `^`, `\^`, `]`, `\]`, `$`, `\$`, `(`, `\(`, `)`, `\)`).Replace(s)
}
func SimilarText(a, b string, _ ...any) int { return similarText(a, b) }
func similarText(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	maxLen, pa, pb := 0, 0, 0
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			k := 0
			for i+k < len(a) && j+k < len(b) && a[i+k] == b[j+k] {
				k++
			}
			if k > maxLen {
				maxLen, pa, pb = k, i, j
			}
		}
	}
	if maxLen == 0 {
		return 0
	}
	return maxLen + similarText(a[:pa], b[:pb]) + similarText(a[pa+maxLen:], b[pb+maxLen:])
}
func Levenshtein(a, b string, _ ...int) int {
	d := make([]int, len(b)+1)
	for j := range d {
		d[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev := d[0]
		d[0] = i
		for j := 1; j <= len(b); j++ {
			tmp := d[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[j] = min(d[j]+1, d[j-1]+1, prev+cost)
			prev = tmp
		}
	}
	return d[len(b)]
}
func Soundex(s string) string              { return s }
func Metaphone(s string, _ ...int) string  { return s }
func Fwrite(_ any, s string, _ ...int) any { Echo(s); return len(s) }
func Fputs(h any, s string, _ ...int) any  { return Fwrite(h, s) }
