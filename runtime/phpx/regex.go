package phpx

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PMGopher/phar2go/runtime/phpx/regexp2"
)

var regexCache sync.Map

// pcre is a compiled PHP regular expression: Go's regexp when it can handle the pattern,
// otherwise regexp2 (lookarounds, backreferences, atomic groups, ...).
type pcre interface {
	// FindStringSubmatchIndex returns byte offsets of the match and its groups (-1 when a
	// group didn't match), or nil.
	FindStringSubmatchIndex(s string) []int
	FindAllStringSubmatchIndex(s string, n int) [][]int
	NumSubexp() int
	SubexpNames() []string
}

type goRE struct{ *regexp.Regexp }

type re2 struct {
	re    *regexp2.Regexp
	names []string
}

func (r *re2) NumSubexp() int        { return len(r.names) - 1 }
func (r *re2) SubexpNames() []string { return r.names }

func (r *re2) FindStringSubmatchIndex(s string) []int {
	all := r.FindAllStringSubmatchIndex(s, 1)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func (r *re2) FindAllStringSubmatchIndex(s string, n int) [][]int {
	// regexp2 reports positions in runes.
	offsets := make([]int, 0, len(s)+1)
	for i := range s {
		offsets = append(offsets, i)
	}
	offsets = append(offsets, len(s))
	var out [][]int
	m, err := r.re.FindStringMatch(s)
	for err == nil && m != nil && (n < 0 || len(out) < n) {
		loc := make([]int, 2*len(r.names))
		for i := range loc {
			loc[i] = -1
		}
		for _, g := range m.Groups() {
			num := r.re.GroupNumberFromName(g.Name)
			if num < 0 || num >= len(r.names) || len(g.Captures) == 0 {
				continue
			}
			loc[2*num] = offsets[g.Index]
			loc[2*num+1] = offsets[g.Index+g.Length]
		}
		out = append(out, loc)
		m, err = r.re.FindNextMatch(m)
	}
	return out
}

// compilePCRE compiles a PHP (PCRE) pattern with delimiters and flags, like "/^a+$/i".
func compilePCRE(pattern string) pcre {
	if r, ok := regexCache.Load(pattern); ok {
		return r.(pcre)
	}
	p := strings.TrimLeft(pattern, " \t\n")
	if p == "" {
		Throw(NewError("ValueError", "preg: empty regular expression"))
	}
	open := p[0]
	close := open
	switch open {
	case '(':
		close = ')'
	case '{':
		close = '}'
	case '[':
		close = ']'
	case '<':
		close = '>'
	}
	end := strings.LastIndexByte(p, close)
	if end <= 0 {
		Throw(NewError("ValueError", "preg: no ending delimiter in "+pattern))
	}
	body, mods := p[1:end], p[end+1:]
	flags := ""
	var opts regexp2.RegexOptions
	for _, m := range mods {
		switch m {
		case 'i':
			flags += "i"
			opts |= regexp2.IgnoreCase
		case 'm':
			flags += "m"
			opts |= regexp2.Multiline
		case 's':
			flags += "s"
			opts |= regexp2.Singleline
		case 'U':
			flags += "U"
		case 'x':
			body = stripExtended(body)
		}
	}
	if open != '/' {
		body = strings.ReplaceAll(body, `\`+string(open), string(open))
	} else {
		body = strings.ReplaceAll(body, `\/`, `/`)
	}
	goBody := body
	if flags != "" {
		goBody = "(?" + flags + ")" + body
	}
	var compiled pcre
	if re, err := regexp.Compile(goBody); err == nil {
		compiled = goRE{re}
	} else {
		re, err2 := regexp2.Compile(body, opts)
		if err2 != nil {
			Throw(NewError("ValueError", "preg: unsupported regular expression "+pattern+": "+err2.Error()))
		}
		nums := re.GetGroupNumbers()
		maxNum := 0
		for _, n := range nums {
			if n > maxNum {
				maxNum = n
			}
		}
		names := make([]string, maxNum+1)
		for _, n := range nums {
			if name := re.GroupNameFromNumber(n); name != strconv.Itoa(n) {
				names[n] = name
			}
		}
		compiled = &re2{re: re, names: names}
	}
	regexCache.Store(pattern, compiled)
	return compiled
}

// expand builds a replacement for one match: $1, \1 and ${1} refer to groups.
func expand(repl string, s string, loc []int) string {
	var sb strings.Builder
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if (c == '$' || c == '\\') && i+1 < len(repl) {
			j := i + 1
			brace := c == '$' && repl[j] == '{'
			if brace {
				j++
			}
			k := j
			for k < len(repl) && k-j < 2 && repl[k] >= '0' && repl[k] <= '9' {
				k++
			}
			if k > j {
				g, _ := strconv.Atoi(repl[j:k])
				if 2*g+1 < len(loc) && loc[2*g] >= 0 {
					sb.WriteString(s[loc[2*g]:loc[2*g+1]])
				}
				if brace && k < len(repl) && repl[k] == '}' {
					k++
				}
				i = k - 1
				continue
			}
			if c == '\\' && repl[j] == '\\' {
				sb.WriteByte('\\')
				i++
				continue
			}
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// replaceAll replaces up to limit matches (-1: all) using fn.
func replaceAll(re pcre, s string, limit int, fn func(loc []int) string) string {
	var sb strings.Builder
	last := 0
	for _, loc := range re.FindAllStringSubmatchIndex(s, limit) {
		sb.WriteString(s[last:loc[0]])
		sb.WriteString(fn(loc))
		last = loc[1]
	}
	sb.WriteString(s[last:])
	return sb.String()
}

func stripExtended(s string) string {
	var sb strings.Builder
	inClass := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			sb.WriteByte(c)
			i++
			sb.WriteByte(s[i])
			continue
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case !inClass && (c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			continue
		case !inClass && c == '#':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

func groupsArray(re pcre, s string, loc []int) *Array {
	out := NewArray()
	names := re.SubexpNames()
	for i := 0; i*2 < len(loc); i++ {
		v := ""
		if loc[i*2] >= 0 {
			v = s[loc[i*2]:loc[i*2+1]]
		}
		if i > 0 && names[i] != "" {
			out.Set(names[i], v)
		}
		out.Set(i, v)
	}
	// PHP drops trailing unmatched groups.
	return out
}

// PregMatch is preg_match($pattern, $subject, &$matches).
func PregMatch(pattern string, subject string, matches ...**Array) int {
	re := compilePCRE(pattern)
	loc := re.FindStringSubmatchIndex(subject)
	if len(matches) > 0 && matches[0] != nil {
		if loc == nil {
			*matches[0] = NewArray()
		} else {
			*matches[0] = groupsArray(re, subject, loc)
		}
	}
	if loc == nil {
		return 0
	}
	return 1
}

// PregMatchAll is preg_match_all($pattern, $subject, &$matches, $flags).
func PregMatchAll(pattern string, subject string, args ...any) int {
	re := compilePCRE(pattern)
	all := re.FindAllStringSubmatchIndex(subject, -1)
	if len(args) > 0 {
		if m, ok := args[0].(**Array); ok && m != nil {
			setOrder := len(args) > 1 && ToInt(args[1])&PREG_SET_ORDER != 0
			out := NewArray()
			if setOrder {
				for _, loc := range all {
					out.Append(groupsArray(re, subject, loc))
				}
			} else {
				n := re.NumSubexp() + 1
				names := re.SubexpNames()
				for g := 0; g < n; g++ {
					col := NewArray()
					for _, loc := range all {
						v := ""
						if loc[g*2] >= 0 {
							v = subject[loc[g*2]:loc[g*2+1]]
						}
						col.Append(v)
					}
					if g > 0 && names[g] != "" {
						out.Set(names[g], col.Clone())
					}
					out.Set(g, col)
				}
			}
			*m = out
		}
	}
	return len(all)
}

// PregReplace is preg_replace($pattern, $replacement, $subject, $limit).
func PregReplace(pattern any, replacement any, subject any, limit ...int) any {
	do := func(s string) string {
		pats := []any{pattern}
		if pa, ok := pattern.(*Array); ok {
			pats = pa.Values()
		}
		for i, p := range pats {
			r := replacement
			if ra, ok := replacement.(*Array); ok {
				r = Index(ra.Values(), i)
			}
			re := compilePCRE(ToString(p))
			repl := ToString(r)
			n := -1
			if len(limit) > 0 && limit[0] >= 0 {
				n = limit[0]
			}
			subject := s
			s = replaceAll(re, subject, n, func(loc []int) string { return expand(repl, subject, loc) })
		}
		return s
	}
	if sa, ok := subject.(*Array); ok {
		out := NewArray()
		for _, e := range sa.Entries() {
			out.Set(e.Key, do(ToString(e.Val)))
		}
		return out
	}
	return do(ToString(subject))
}

// PregReplaceCallback is preg_replace_callback($pattern, $callback, $subject).
func PregReplaceCallback(pattern any, callback any, subject any, _ ...any) any {
	re := compilePCRE(ToString(pattern))
	s := ToString(subject)
	return replaceAll(re, s, -1, func(loc []int) string { return ToString(Invoke(callback, groupsArray(re, s, loc))) })
}

// PregSplit is preg_split($pattern, $subject, $limit, $flags).
func PregSplit(pattern string, subject string, args ...int) *Array {
	re := compilePCRE(pattern)
	limit := -1
	if len(args) > 0 && args[0] > 0 {
		limit = args[0]
	}
	noEmpty := len(args) > 1 && args[1]&PREG_SPLIT_NO_EMPTY != 0
	out := NewArray()
	last := 0
	n := -1
	if limit > 0 {
		n = limit - 1
	}
	add := func(p string) {
		if !(noEmpty && p == "") {
			out.Append(p)
		}
	}
	for _, loc := range re.FindAllStringSubmatchIndex(subject, n) {
		if loc[1] == 0 && loc[0] == 0 {
			continue
		}
		add(subject[last:loc[0]])
		last = loc[1]
	}
	add(subject[last:])
	return out
}

// PregQuote is preg_quote($str, $delimiter).
func PregQuote(s string, delimiter ...string) string {
	q := regexp.QuoteMeta(s)
	q = strings.NewReplacer("#", `\#`, "-", `\-`, "=", `\=`, "!", `\!`, ":", `\:`, "<", `\<`, ">", `\>`).Replace(q)
	if len(delimiter) > 0 && delimiter[0] != "" && !strings.Contains(`.\+*?[^]$(){}=!<>|:-#`, delimiter[0]) {
		q = strings.ReplaceAll(q, delimiter[0], `\`+delimiter[0])
	}
	return q
}

// PregGrep is preg_grep($pattern, $array).
func PregGrep(pattern string, a any) *Array {
	re := compilePCRE(pattern)
	out := NewArray()
	for _, e := range Iter(a) {
		if re.FindStringSubmatchIndex(ToString(e.Val)) != nil {
			out.Set(e.Key, e.Val)
		}
	}
	return out
}
