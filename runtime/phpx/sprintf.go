package phpx

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Sprintf is PHP's sprintf: %s %d %f %.2f %05d %x %X %o %b %c %e %u %% and argnum$ are supported.
func Sprintf(format string, args ...any) string {
	var sb strings.Builder
	argi := 0
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			sb.WriteByte(c)
			continue
		}
		i++
		if i >= len(format) {
			break
		}
		if format[i] == '%' {
			sb.WriteByte('%')
			continue
		}
		// %[argnum$][flags][width][.precision]specifier
		start := i
		argnum := -1
		j := i
		for j < len(format) && isDigit(format[j]) {
			j++
		}
		if j < len(format) && format[j] == '$' && j > i {
			argnum, _ = strconv.Atoi(format[i:j])
			argnum--
			i = j + 1
		}
		left, plus, padChar := false, false, byte(' ')
	flags:
		for i < len(format) {
			switch format[i] {
			case '-':
				left = true
			case '+':
				plus = true
			case '0':
				padChar = '0'
			case ' ':
				padChar = ' '
			case '\'':
				if i+1 < len(format) {
					i++
					padChar = format[i]
				}
			default:
				break flags
			}
			i++
		}
		width := 0
		for i < len(format) && isDigit(format[i]) {
			width = width*10 + int(format[i]-'0')
			i++
		}
		prec := -1
		if i < len(format) && format[i] == '.' {
			i++
			prec = 0
			for i < len(format) && isDigit(format[i]) {
				prec = prec*10 + int(format[i]-'0')
				i++
			}
		}
		if i >= len(format) {
			sb.WriteString(format[start-1:])
			break
		}
		spec := format[i]
		var arg any
		if argnum >= 0 {
			if argnum < len(args) {
				arg = args[argnum]
			}
		} else {
			if argi < len(args) {
				arg = args[argi]
			}
			argi++
		}
		var s string
		switch spec {
		case 'd', 'i':
			n := ToInt(arg)
			s = strconv.Itoa(n)
			if plus && n >= 0 {
				s = "+" + s
			}
		case 'u':
			s = strconv.FormatUint(uint64(ToInt(arg)), 10)
		case 'f', 'F':
			if prec < 0 {
				prec = 6
			}
			f := ToFloat(arg)
			s = strconv.FormatFloat(f, 'f', prec, 64)
			if plus && f >= 0 {
				s = "+" + s
			}
		case 'e', 'E':
			if prec < 0 {
				prec = 6
			}
			s = strconv.FormatFloat(ToFloat(arg), spec, prec, 64)
		case 'g', 'G':
			s = strconv.FormatFloat(ToFloat(arg), spec, prec, 64)
		case 's':
			s = ToString(arg)
			if prec >= 0 && prec < len(s) {
				s = s[:prec]
			}
		case 'x':
			s = strconv.FormatUint(uint64(ToInt(arg)), 16)
		case 'X':
			s = strings.ToUpper(strconv.FormatUint(uint64(ToInt(arg)), 16))
		case 'o':
			s = strconv.FormatUint(uint64(ToInt(arg)), 8)
		case 'b':
			s = strconv.FormatUint(uint64(ToInt(arg)), 2)
		case 'c':
			s = string([]byte{byte(ToInt(arg))})
		default:
			s = string(spec)
		}
		if len(s) < width {
			pad := strings.Repeat(string(padChar), width-len(s))
			switch {
			case left:
				if padChar == '0' {
					pad = strings.Repeat(" ", width-len(s))
				}
				s += pad
			case padChar == '0' && len(s) > 0 && (s[0] == '-' || s[0] == '+'):
				s = s[:1] + pad + s[1:]
			default:
				s = pad + s
			}
		}
		sb.WriteString(s)
	}
	return sb.String()
}

func Vsprintf(format string, args any) string { return Sprintf(format, ToArray(args).Values()...) }

func Printf(format string, args ...any) int {
	s := Sprintf(format, args...)
	fmt.Fprint(os.Stdout, s)
	return len(s)
}

// Echo is echo/print: it writes to the server's standard output.
func Echo(values ...any) {
	for _, v := range values {
		fmt.Fprint(os.Stdout, ToString(v))
	}
}

// PrintR is print_r($value, $return).
func PrintR(v any, ret ...bool) any {
	s := printR(v, 0)
	if len(ret) > 0 && ret[0] {
		return s
	}
	fmt.Fprint(os.Stdout, s)
	return true
}

func printR(v any, depth int) string {
	a, ok := v.(*Array)
	if !ok {
		return ToString(v)
	}
	ind := strings.Repeat("    ", depth*2)
	var sb strings.Builder
	sb.WriteString("Array\n" + ind + "(\n")
	for _, e := range a.Entries() {
		sb.WriteString(fmt.Sprintf("%s    [%s] => %s\n", ind, ToString(e.Key), printR(e.Val, depth+1)))
	}
	sb.WriteString(ind + ")\n")
	return sb.String()
}

// VarDump is var_dump(...$values).
func VarDump(values ...any) {
	for _, v := range values {
		fmt.Fprintln(os.Stdout, VarExport(v, true))
	}
}

// VarExport is var_export($value, $return).
func VarExport(v any, ret ...bool) any {
	var s string
	switch x := v.(type) {
	case nil:
		s = "NULL"
	case string:
		s = "'" + strings.ReplaceAll(strings.ReplaceAll(x, `\`, `\\`), "'", `\'`) + "'"
	case bool:
		s = strconv.FormatBool(x)
	case *Array:
		var sb strings.Builder
		sb.WriteString("array (\n")
		for _, e := range x.Entries() {
			sb.WriteString(fmt.Sprintf("  %s => %s,\n", ToString(VarExport(e.Key, true)), ToString(VarExport(e.Val, true))))
		}
		sb.WriteString(")")
		s = sb.String()
	default:
		s = ToString(v)
	}
	if len(ret) > 0 && ret[0] {
		return s
	}
	fmt.Fprint(os.Stdout, s)
	return nil
}
