package conv

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"
	"github.com/VKCOM/php-parser/pkg/conf"
	"github.com/VKCOM/php-parser/pkg/errors"
	"github.com/VKCOM/php-parser/pkg/parser"
	"github.com/VKCOM/php-parser/pkg/version"
	"github.com/VKCOM/php-parser/pkg/visitor/nsresolver"
	"github.com/VKCOM/php-parser/pkg/visitor/traverser"
)

// phpFile is a parsed PHP source file.
type phpFile struct {
	Path  string
	Src   []byte
	Root  ast.Vertex
	Names map[ast.Vertex]string
}

var (
	reReadonlyClass = regexp.MustCompile(`\breadonly\s+((?:final\s+|abstract\s+)*class\b)`)
	reTypedConst    = regexp.MustCompile(`\bconst\s+[?A-Za-z_\\][A-Za-z0-9_\\|?]*\s+([A-Za-z_][A-Za-z0-9_]*\s*=)`)
	reDNF           = regexp.MustCompile(`\(([A-Za-z_\\][A-Za-z0-9_\\]*(?:\s*&\s*[A-Za-z_\\][A-Za-z0-9_\\]*)+)\)\s*\|`)
)

// preprocess rewrites syntax newer than the parser understands (PHP 8.2+) without changing
// byte offsets where possible.
func preprocess(src []byte) []byte {
	s := string(src)
	s = reReadonlyClass.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", len("readonly")) + m[len("readonly"):]
	})
	s = reTypedConst.ReplaceAllStringFunc(s, func(m string) string {
		sub := reTypedConst.FindStringSubmatch(m)
		pad := len(m) - len("const ") - len(sub[1])
		return "const " + strings.Repeat(" ", pad) + sub[1]
	})
	s = reDNF.ReplaceAllStringFunc(s, func(m string) string {
		// (A&B)|null -> mixed: types are only hints for the conversion.
		return "mixed" + strings.Repeat(" ", len(m)-len("mixed")-1) + "|"
	})
	return []byte(s)
}

func parsePHP(path string, src []byte) (*phpFile, error) {
	var errs []string
	root, err := parser.Parse(preprocess(src), conf.Config{
		Version: &version.Version{Major: 8, Minor: 1},
		ErrorHandlerFunc: func(e *errors.Error) {
			errs = append(errs, e.String())
		},
	})
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: PHP syntax error: %s", path, strings.Join(errs, "; "))
	}
	nsr := nsresolver.NewNamespaceResolver()
	traverser.NewTraverser(nsr).Traverse(root)
	return &phpFile{Path: path, Src: src, Root: root, Names: nsr.ResolvedNames}, nil
}

// identValue returns the text of an Identifier, NamePart or Name node.
func identValue(n ast.Vertex) string {
	switch x := n.(type) {
	case *ast.Identifier:
		return string(x.Value)
	case *ast.NamePart:
		return string(x.Value)
	case *ast.Name:
		return nameParts(x.Parts)
	case *ast.NameFullyQualified:
		return nameParts(x.Parts)
	case *ast.NameRelative:
		return nameParts(x.Parts)
	case *ast.ExprVariable:
		return identValue(x.Name)
	}
	return ""
}

func nameParts(parts []ast.Vertex) string {
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteByte('\\')
		}
		sb.WriteString(identValue(p))
	}
	return sb.String()
}

// varName returns the name of a simple variable ($name), or "".
func varName(n ast.Vertex) string {
	v, ok := n.(*ast.ExprVariable)
	if !ok {
		return ""
	}
	if id, ok := v.Name.(*ast.Identifier); ok {
		return strings.TrimPrefix(string(id.Value), "$")
	}
	return ""
}

// docBefore returns the doc comment (/** ... */) that precedes offset in src.
func docBefore(src []byte, offset int) string {
	if offset <= 0 || offset > len(src) {
		return ""
	}
	before := bytes.TrimRight(src[:offset], " \t\r\n")
	// Skip attributes and modifiers written before the node's position.
	for {
		trimmed := bytes.TrimRight(before, " \t\r\n")
		if bytes.HasSuffix(trimmed, []byte("]")) {
			if i := bytes.LastIndex(trimmed, []byte("#[")); i >= 0 {
				before = trimmed[:i]
				continue
			}
		}
		before = trimmed
		break
	}
	if !bytes.HasSuffix(before, []byte("*/")) {
		return ""
	}
	start := bytes.LastIndex(before, []byte("/**"))
	if start < 0 {
		return ""
	}
	return string(before[start:])
}

var (
	reDocTag = regexp.MustCompile(`@([A-Za-z-]+)(?:[ \t]+([^\r\n*]*))?`)
)

// docTags parses the @tags of a doc comment.
func docTags(doc string) map[string][]string {
	out := map[string][]string{}
	for _, m := range reDocTag.FindAllStringSubmatch(doc, -1) {
		out[strings.ToLower(m[1])] = append(out[strings.ToLower(m[1])], strings.TrimSpace(m[2]))
	}
	return out
}

// docType returns the type of a @var/@return/@param tag: "@param Player[] $players" for
// name "players" gives "Player[]".
func docType(doc, tag, param string) string {
	for _, v := range docTags(doc)[tag] {
		f := splitDocType(v)
		if len(f) == 0 {
			continue
		}
		if param == "" {
			if strings.HasPrefix(f[0], "$") {
				if len(f) > 1 {
					return f[1]
				}
				continue
			}
			return f[0]
		}
		if len(f) > 1 && f[1] == "$"+param {
			return f[0]
		}
		if f[0] == "$"+param && len(f) > 1 {
			return f[1]
		}
	}
	return ""
}

// splitDocType splits a tag value into its type (which may contain spaces inside <> or {}) and
// the following words.
func splitDocType(v string) []string {
	depth := 0
	for i, c := range v {
		switch c {
		case '<', '{', '(':
			depth++
		case '>', '}', ')':
			depth--
		case ' ', '\t':
			if depth == 0 {
				rest := strings.Fields(v[i:])
				return append([]string{strings.TrimSpace(v[:i])}, rest...)
			}
		}
	}
	return []string{strings.TrimSpace(v)}
}

func pos(n ast.Vertex) int {
	if n == nil {
		return 0
	}
	if p := n.GetPosition(); p != nil {
		return p.StartPos
	}
	return 0
}

func line(n ast.Vertex) int {
	if n == nil {
		return 0
	}
	if p := n.GetPosition(); p != nil {
		return p.StartLine
	}
	return 0
}
