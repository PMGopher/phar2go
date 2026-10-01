package phpx

import (
	"regexp"
	"strings"
	"sync"
)

var regexCache sync.Map

// compilePCRE compiles a PHP (PCRE) pattern with delimiters and flags, like "/^a+$/i", to a Go
// regular expression. Lookarounds and backreferences aren't supported by Go and throw.
func compilePCRE(pattern string) *regexp.Regexp {
	if r, ok := regexCache.Load(pattern); ok {
		return r.(*regexp.Regexp)
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
	for _, m := range mods {
		switch m {
		case 'i', 'm', 's', 'U':
			flags += string(m)
		case 'x':
			body = stripExtended(body)
		}
	}
	if open != '/' {
		body = strings.ReplaceAll(body, `\`+string(open), string(open))
	} else {
		body = strings.ReplaceAll(body, `\/`, `/`)
	}
	if flags != "" {
		body = "(?" + flags + ")" + body
	}
	re, err := regexp.Compile(body)
	if err != nil {
		Throw(NewError("ValueError", "preg: unsupported regular expression "+pattern+": "+err.Error()))
	}
	regexCache.Store(pattern, re)
	return re
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

func groupsArray(re *regexp.Regexp, s string, loc []int) *Array {
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

// convertReplacement converts $1, \1 and ${1} references to Go's ${1}.
func convertReplacement(r string) string {
	var sb strings.Builder
	for i := 0; i < len(r); i++ {
		c := r[i]
		if (c == '$' || c == '\\') && i+1 < len(r) {
			j := i + 1
			brace := c == '$' && r[j] == '{'
			if brace {
				j++
			}
			k := j
			for k < len(r) && k-j < 2 && isDigit(r[k]) {
				k++
			}
			if k > j {
				sb.WriteString("${" + r[j:k] + "}")
				if brace && k < len(r) && r[k] == '}' {
					k++
				}
				i = k - 1
				continue
			}
		}
		if c == '$' {
			sb.WriteString("$$")
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
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
			repl := convertReplacement(ToString(r))
			if len(limit) > 0 && limit[0] >= 0 {
				n := limit[0]
				s = re.ReplaceAllStringFunc(s, func(m string) string {
					if n == 0 {
						return m
					}
					n--
					return re.ReplaceAllString(m, repl)
				})
				continue
			}
			s = re.ReplaceAllString(s, repl)
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
	var sb strings.Builder
	last := 0
	for _, loc := range re.FindAllStringSubmatchIndex(s, -1) {
		sb.WriteString(s[last:loc[0]])
		sb.WriteString(ToString(Invoke(callback, groupsArray(re, s, loc))))
		last = loc[1]
	}
	sb.WriteString(s[last:])
	return sb.String()
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
	for _, p := range re.Split(subject, limit) {
		if noEmpty && p == "" {
			continue
		}
		out.Append(p)
	}
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
		if re.MatchString(ToString(e.Val)) {
			out.Set(e.Key, e.Val)
		}
	}
	return out
}
