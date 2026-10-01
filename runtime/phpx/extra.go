package phpx

import (
	"bufio"
	"io"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
)

const (
	INI_SCANNER_NORMAL         = 0
	INI_SCANNER_RAW            = 1
	INI_SCANNER_TYPED          = 2
	M_PI_2                     = math.Pi / 2
	M_PI_4                     = math.Pi / 4
	M_1_PI                     = 1 / math.Pi
	M_2_PI                     = 2 / math.Pi
	PHP_BINARY                 = "pocketmine-go"
	FILTER_VALIDATE_DOMAIN     = 277
	FILTER_FLAG_HOSTNAME       = 1048576
	FILTER_FLAG_ALLOW_FRACTION = 4096
	FILTER_FLAG_IPV4           = 1048576
	FILTER_FLAG_IPV6           = 2097152
	RESOURCE_PATH              = "resources/"
)

// ParseIniString is parse_ini_string($ini, $sections, $mode).
func ParseIniString(ini string, args ...any) any {
	sections := len(args) > 0 && ToBool(args[0])
	mode := INI_SCANNER_NORMAL
	if len(args) > 1 {
		mode = ToInt(args[1])
	}
	out := NewArray()
	cur := out
	for _, raw := range strings.Split(strings.ReplaceAll(ini, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			if sections {
				cur = out.Sub(strings.TrimSpace(line[1 : len(line)-1]))
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		var v any = val
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			v = val[1 : len(val)-1]
		} else if mode != INI_SCANNER_RAW {
			if i := strings.IndexByte(val, ';'); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
			switch strings.ToLower(val) {
			case "true", "on", "yes":
				v = "1"
				if mode == INI_SCANNER_TYPED {
					v = true
				}
			case "false", "off", "no", "none":
				v = ""
				if mode == INI_SCANNER_TYPED {
					v = false
				}
			case "null":
				v = ""
				if mode == INI_SCANNER_TYPED {
					v = nil
				}
			default:
				v = val
				if mode == INI_SCANNER_TYPED {
					if n, err := strconv.Atoi(val); err == nil {
						v = n
					}
				}
			}
		}
		if strings.HasSuffix(key, "[]") {
			cur.Sub(strings.TrimSuffix(key, "[]")).Append(v)
			continue
		}
		cur.Set(key, v)
	}
	return out
}

// ParseIniFile is parse_ini_file($path, $sections, $mode).
func ParseIniFile(path string, args ...any) any {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return ParseIniString(string(b), args...)
}

// Stripcslashes is stripcslashes($string).
func Stripcslashes(s string) string { return decodeCEscapes(s) }

func decodeCEscapes(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case 'a':
			sb.WriteByte(7)
		case 'v':
			sb.WriteByte('\v')
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'x':
			j := i + 1
			for j < len(s) && j < i+3 && isHexByte(s[j]) {
				j++
			}
			n, _ := strconv.ParseUint(s[i+1:j], 16, 8)
			sb.WriteByte(byte(n))
			i = j - 1
		default:
			if s[i] >= '0' && s[i] <= '7' {
				j := i
				for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
					j++
				}
				n, _ := strconv.ParseUint(s[i:j], 8, 8)
				sb.WriteByte(byte(n))
				i = j - 1
			} else {
				sb.WriteByte(s[i])
			}
		}
	}
	return sb.String()
}

func isHexByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// FileHandle is a PHP stream from fopen() or opendir().
type FileHandle struct {
	f       *os.File
	r       *bufio.Reader
	entries []string
	eof     bool
}

func (h *FileHandle) PhpClass() string { return "resource" }

// Fopen is fopen($path, $mode).
func Fopen(path string, mode string, _ ...any) any {
	flag := os.O_RDONLY
	switch strings.TrimRight(mode, "bt") {
	case "r":
	case "r+":
		flag = os.O_RDWR
	case "w":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "w+":
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "a+":
		flag = os.O_RDWR | os.O_CREATE | os.O_APPEND
	case "x":
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	case "c":
		flag = os.O_WRONLY | os.O_CREATE
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return false
	}
	return &FileHandle{f: f, r: bufio.NewReader(f)}
}

func handleOf(h any) *FileHandle {
	if fh, ok := h.(*FileHandle); ok {
		return fh
	}
	return nil
}

// Fgets is fgets($handle).
func Fgets(h any, _ ...any) any {
	if fh := handleOf(h); fh != nil && fh.r != nil {
		line, err := fh.r.ReadString('\n')
		if line == "" && err != nil {
			fh.eof = true
			return false
		}
		return line
	}
	if r, ok := h.(io.Reader); ok {
		var sb strings.Builder
		buf := make([]byte, 1)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.WriteByte(buf[0])
				if buf[0] == '\n' {
					break
				}
			}
			if err != nil {
				break
			}
		}
		if sb.Len() == 0 {
			return false
		}
		return sb.String()
	}
	return false
}

// Fread is fread($handle, $length).
func Fread(h any, length int) any {
	var r io.Reader
	if fh := handleOf(h); fh != nil {
		r = fh.r
	} else if rr, ok := h.(io.Reader); ok {
		r = rr
	}
	if r == nil {
		return false
	}
	buf := make([]byte, length)
	n, _ := io.ReadFull(r, buf)
	return string(buf[:n])
}

// Feof is feof($handle).
func Feof(h any) bool {
	if fh := handleOf(h); fh != nil {
		if fh.eof {
			return true
		}
		_, err := fh.r.Peek(1)
		return err != nil
	}
	return true
}

// Fflush is fflush($handle).
func Fflush(h any) bool {
	if fh := handleOf(h); fh != nil {
		return fh.f.Sync() == nil
	}
	return true
}

// IsResource is is_resource($v).
func IsResource(v any) bool {
	switch v.(type) {
	case *FileHandle, io.Reader, io.Writer:
		return !IsNull(v)
	}
	return false
}

// GetResourceType is get_resource_type($v).
func GetResourceType(v any) string {
	if IsResource(v) {
		return "stream"
	}
	return "Unknown"
}

// Opendir is opendir($path).
func Opendir(path string, _ ...any) any {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	names := []string{".", ".."}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return &FileHandle{entries: names}
}

// Readdir is readdir($handle).
func Readdir(h any) any {
	fh := handleOf(h)
	if fh == nil || len(fh.entries) == 0 {
		return false
	}
	n := fh.entries[0]
	fh.entries = fh.entries[1:]
	return n
}

// Closedir is closedir($handle).
func Closedir(_ any) {}

// IgbinarySerialize is igbinary_serialize($value) (as JSON: values round-trip through
// IgbinaryUnserialize).
func IgbinarySerialize(v any) any { return Serialize(v) }

// IgbinaryUnserialize is igbinary_unserialize($data).
func IgbinaryUnserialize(s string) any { return Unserialize(s) }

// Morton2dEncode is morton2d_encode($x, $y): the bits of x and y interleaved.
func Morton2dEncode(x, y int) int {
	return int(spread2(uint64(uint32(x))) | spread2(uint64(uint32(y)))<<1)
}

// Morton2dDecode is morton2d_decode($hash).
func Morton2dDecode(h int) *Array {
	return List(int(int32(compact2(uint64(h)))), int(int32(compact2(uint64(h)>>1))))
}

// Morton3dEncode is morton3d_encode($x, $y, $z).
func Morton3dEncode(x, y, z int) int {
	return int(spread3(uint64(x)&0x1fffff) | spread3(uint64(y)&0x1fffff)<<1 | spread3(uint64(z)&0x1fffff)<<2)
}

// Morton3dDecode is morton3d_decode($hash).
func Morton3dDecode(h int) *Array {
	sx := func(v uint64) int { return int(int64(v<<43) >> 43) }
	u := uint64(h)
	return List(sx(compact3(u)), sx(compact3(u>>1)), sx(compact3(u>>2)))
}

func spread2(x uint64) uint64 {
	x &= 0xffffffff
	x = (x | x<<16) & 0x0000ffff0000ffff
	x = (x | x<<8) & 0x00ff00ff00ff00ff
	x = (x | x<<4) & 0x0f0f0f0f0f0f0f0f
	x = (x | x<<2) & 0x3333333333333333
	x = (x | x<<1) & 0x5555555555555555
	return x
}

func compact2(x uint64) uint64 {
	x &= 0x5555555555555555
	x = (x | x>>1) & 0x3333333333333333
	x = (x | x>>2) & 0x0f0f0f0f0f0f0f0f
	x = (x | x>>4) & 0x00ff00ff00ff00ff
	x = (x | x>>8) & 0x0000ffff0000ffff
	x = (x | x>>16) & 0x00000000ffffffff
	return x
}

func spread3(x uint64) uint64 {
	x &= 0x1fffff
	x = (x | x<<32) & 0x1f00000000ffff
	x = (x | x<<16) & 0x1f0000ff0000ff
	x = (x | x<<8) & 0x100f00f00f00f00f
	x = (x | x<<4) & 0x10c30c30c30c30c3
	x = (x | x<<2) & 0x1249249249249249
	return x
}

func compact3(x uint64) uint64 {
	x &= 0x1249249249249249
	x = (x | x>>2) & 0x10c30c30c30c30c3
	x = (x | x>>4) & 0x100f00f00f00f00f
	x = (x | x>>8) & 0x1f0000ff0000ff
	x = (x | x>>16) & 0x1f00000000ffff
	x = (x | x>>32) & 0x1fffff
	return x
}

// ArrayWalkRecursive is array_walk_recursive($array, $callback).
func ArrayWalkRecursive(a *Array, callback any, extra ...any) bool {
	for _, e := range a.Entries() {
		if sub, ok := e.Val.(*Array); ok {
			ArrayWalkRecursive(sub, callback, extra...)
			continue
		}
		Invoke(callback, append([]any{e.Val, e.Key}, extra...)...)
	}
	return true
}

// ClassUses is class_uses($objectOrClass): traits are merged into classes when converting.
func ClassUses(_ any, _ ...bool) *Array { return NewArray() }

func bcNum(v any) *big.Float {
	f, _, _ := big.ParseFloat(strings.TrimSpace(ToString(v)), 10, 256, big.ToNearestEven)
	if f == nil {
		return new(big.Float)
	}
	return f
}

func bcScale(scale []int) int {
	if len(scale) > 0 {
		return scale[0]
	}
	return 0
}

func bcString(f *big.Float, scale int) string {
	s := f.Text('f', scale+2)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if scale == 0 {
			return s[:i]
		}
		if len(s) > i+1+scale {
			s = s[:i+1+scale]
		}
	}
	return s
}

func Bcadd(a, b any, scale ...int) string {
	return bcString(new(big.Float).SetPrec(256).Add(bcNum(a), bcNum(b)), bcScale(scale))
}
func Bcsub(a, b any, scale ...int) string {
	return bcString(new(big.Float).SetPrec(256).Sub(bcNum(a), bcNum(b)), bcScale(scale))
}
func Bcmul(a, b any, scale ...int) string {
	return bcString(new(big.Float).SetPrec(256).Mul(bcNum(a), bcNum(b)), bcScale(scale))
}
func Bcdiv(a, b any, scale ...int) string {
	d := bcNum(b)
	if d.Sign() == 0 {
		Throw(NewError("DivisionByZeroError", "Division by zero"))
	}
	return bcString(new(big.Float).SetPrec(256).Quo(bcNum(a), d), bcScale(scale))
}
func Bccomp(a, b any, _ ...int) int { return bcNum(a).Cmp(bcNum(b)) }
