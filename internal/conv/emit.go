package conv

import (
	"bytes"
	"fmt"
	"go/format"
	"path"
	"sort"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// goFile collects the code of one output file.
type goFile struct {
	name    string
	imports *importSet
	parts   []string
	idents  map[string]bool
}

func (cv *converter) newFctx(gf *goFile, c *class, m *method) *fctx {
	tmp, labels := 0, 0
	f := &fctx{cv: cv, cls: c, m: m, vars: map[string]*local{}, imports: gf.imports, tmp: &tmp, labelN: &labels}
	if c != nil {
		f.file = cv.fileOf(c, m)
		f.recv = c.Recv
		if c.Poly {
			f.selfVar = c.Recv + ".self"
		}
	} else if m != nil {
		f.file = cv.fileOf(nil, m)
	}
	if f.file == nil && len(cv.files) > 0 {
		f.file = cv.files[0]
	}
	return f
}

// emitAll writes the Go files.
func (cv *converter) emitAll(res *Result) {
	cv.constTypes()
	files := map[string]*goFile{}
	var order []string
	fileFor := func(p string) *goFile {
		name := cv.goFileName(p)
		if gf, ok := files[name]; ok {
			return gf
		}
		gf := &goFile{name: name, imports: newImportSet(), idents: map[string]bool{}}
		files[name] = gf
		order = append(order, name)
		return gf
	}
	for _, c := range cv.classList {
		if c.Kind == kindTrait {
			continue
		}
		gf := fileFor(c.File.Path)
		gf.parts = append(gf.parts, cv.emitClass(gf, c))
	}
	for _, fn := range sortedFuncs(cv.funcs) {
		gf := fileFor(cv.funcFile[fn].Path)
		gf.parts = append(gf.parts, cv.emitFunc(gf, fn))
	}
	{
		gf := fileFor("phar2go_runtime.php")
		var lines []string
		for _, s := range cv.staticVars {
			if strings.Contains(s, "Unsupported(") {
				// Never panic while the server starts.
				name := strings.Fields(s)[1]
				s = "var " + name + " any // TODO(phar2go): " + strings.TrimPrefix(s, "var "+name+" = ")
			}
			lines = append(lines, s)
		}
		var defs []string
		for _, g := range cv.defines {
			defs = append(defs, g)
		}
		sort.Strings(defs)
		for _, g := range defs {
			lines = append(lines, "var "+g+" any")
		}
		for _, is := range cv.staticImports {
			for p := range is.paths {
				gf.imports.paths[p] = true
			}
		}
		var classes, funcs []string
		for _, c := range cv.classList {
			if !strings.HasPrefix(c.File.Path, "phar2go-stubs/") {
				classes = append(classes, quote(c.FQCN))
			}
		}
		for k := range cv.extKnown {
			classes = append(classes, quote(k))
		}
		sort.Strings(classes)
		for _, fn := range sortedFuncs(cv.funcs) {
			funcs = append(funcs, quote(fn.Name))
		}
		f := cv.newFctx(gf, nil, nil)
		// Parents and interfaces, for is_a() and is_subclass_of().
		var parents []string
		for _, c := range cv.classList {
			if c.Kind == kindTrait || strings.HasPrefix(c.File.Path, "phar2go-stubs/") {
				continue
			}
			var ps []string
			seen := map[*class]bool{}
			var walkC func(k *class)
			walkC = func(k *class) {
				if k == nil || seen[k] {
					return
				}
				seen[k] = true
				if k != c {
					ps = append(ps, quote(k.FQCN))
				}
				if k.ExtParentPH != "" {
					ps = append(ps, quote(k.ExtParentPH))
					for _, a := range cv.extAncestors(k.ExtParent) {
						ps = append(ps, quote(a))
					}
				}
				for _, i := range k.IfaceNames {
					if cv.classes[strings.ToLower(i)] == nil {
						ps = append(ps, quote(i))
					}
				}
				for _, i := range k.Ifaces {
					walkC(i)
				}
				walkC(k.Parent)
			}
			walkC(c)
			if len(ps) > 0 {
				parents = append(parents, quote(c.FQCN)+": {"+strings.Join(ps, ", ")+"},")
			}
		}
		// Static methods, constructors and constants, for $class::method(), new $class() and
		// $class::CONST.
		var statics []string
		for _, c := range cv.classList {
			if c.Kind == kindTrait || c.Kind == kindInterface || strings.HasPrefix(c.File.Path, "phar2go-stubs/") {
				continue
			}
			for _, m := range c.Methods {
				if m.Static && m.HasBody {
					statics = append(statics, fmt.Sprintf("%s(%s, %s, %s)", f.phpx("RegisterStatic"), quote(c.FQCN), quote(m.Name), m.GoName))
				}
			}
			for _, k := range c.Consts {
				if k.Class == c {
					statics = append(statics, fmt.Sprintf("%s(%s, %s, func() any { return %s })", f.phpx("RegisterStatic"), quote(c.FQCN), quote("const:"+k.Name), k.GoName))
				}
			}
			if c.Kind == kindClass && !c.Abstract {
				statics = append(statics, fmt.Sprintf("%s(%s, New%s)", f.phpx("RegisterNew"), quote(c.FQCN), c.GoName))
			}
		}
		reg := fmt.Sprintf("// The plugin's classes, for class_exists(), is_a() and is_subclass_of().\nfunc init() {\n%s([]string{\n%s,\n}, []string{%s})\n%s(map[string][]string{\n%s\n})\n}",
			f.phpx("RegisterClasses"), strings.Join(classes, ",\n"), strings.Join(funcs, ", "), f.phpx("RegisterParents"), strings.Join(parents, "\n"))
		if len(statics) > 0 {
			reg += "\n\n// Static methods, constructors and constants, for $class::method() and new $class().\nfunc init() {\n" + strings.Join(statics, "\n") + "\n}"
		}
		if cv.usesServer {
			srv := f.pkgRef(cv.idx.Module+"/pocketmine/server") + ".Server"
			pl := "*" + cv.mainType
			reg += fmt.Sprintf("\n\n// PluginInstance is the plugin, set when the server creates it (see plugin.go).\nvar PluginInstance %s\n\n// phar2goServer is Server::getInstance(): the server that loaded the plugin.\nfunc phar2goServer() *%s {\nreturn PluginInstance.GetServer().(*%s)\n}", pl, srv, srv)
		}
		if len(lines) > 0 {
			gf.parts = append(gf.parts, "// Variables of PHP `static` declarations and define()d constants.\n"+strings.Join(lines, "\n"))
		}
		gf.parts = append(gf.parts, reg)
	}
	for _, name := range order {
		gf := files[name]
		src := cv.render(gf)
		formatted, err := format.Source(src)
		if err != nil {
			cv.warnf(name, "the generated Go code doesn't parse (%v); it was written unformatted", err)
			formatted = src
		}
		res.Files[name] = formatted
	}
}

// render resolves package placeholders and writes the file.
func (cv *converter) render(gf *goFile) []byte {
	body := strings.Join(gf.parts, "\n\n")
	// Identifiers used in the file (locals), which imports must not shadow.
	paths := make([]string, 0, len(gf.imports.paths))
	for p := range gf.imports.paths {
		if strings.Contains(body, "\x01"+p+"\x02") {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	aliases := map[string]string{}
	used := map[string]bool{}
	for _, p := range paths {
		name := cv.pkgName(p)
		alias := name
		if used[alias] {
			parent := path.Base(path.Dir(p))
			alias = name + parent
		}
		if gf.idents[alias] || gf.imports.idents[alias] || cv.pkgNames[alias] || used[alias] || alias == cv.opts.Package || cv.isRecvName(alias) {
			alias = "pm" + alias
		}
		for i := 2; used[alias]; i++ {
			alias = fmt.Sprintf("%s%d", name, i)
		}
		used[alias] = true
		aliases[p] = alias
	}
	for _, p := range paths {
		body = strings.ReplaceAll(body, "\x01"+p+"\x02", aliases[p])
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by phar2go from the PHP plugin; edit it freely.\n\npackage %s\n\n", cv.opts.Package)
	if len(paths) > 0 {
		buf.WriteString("import (\n")
		for _, p := range paths {
			ip := p
			if p == api.PhpxPath {
				ip = cv.opts.PhpxImport
			}
			if aliases[p] != path.Base(ip) {
				fmt.Fprintf(&buf, "\t%s %q\n", aliases[p], ip)
			} else {
				fmt.Fprintf(&buf, "\t%q\n", ip)
			}
		}
		buf.WriteString(")\n\n")
	}
	buf.WriteString(body)
	buf.WriteString("\n")
	return buf.Bytes()
}

func (cv *converter) isRecvName(n string) bool {
	for _, c := range cv.classList {
		if c.Recv == n {
			return true
		}
	}
	return false
}

func (cv *converter) pkgName(p string) string {
	if p == api.PhpxPath {
		return "phpx"
	}
	if pkg := cv.idx.Packages[p]; pkg != nil {
		return pkg.Name
	}
	return path.Base(p)
}

// constTypes computes the types of class constants.
func (cv *converter) constTypes() {
	for round := 0; round < 3; round++ {
		for _, c := range cv.classList {
			for _, k := range c.Consts {
				gf := &goFile{imports: newImportSet(), idents: map[string]bool{}}
				f := cv.newFctx(gf, c, nil)
				f.static = true
				f.dry = true
				v := f.expr(k.Expr, nil)
				k.Type = v.t
				if k.Type == nil || k.Type.IsNil() {
					k.Type = api.Any
				}
				k.isConst = isConstExpr(k.Expr, cv, c)
				if !k.isConst && v.konst && k.Type.IsNumber() {
					k.Type = api.Any
				}
			}
		}
	}
}

// isConstExpr reports whether a PHP expression can be a Go constant.
func isConstExpr(n ast.Vertex, cv *converter, c *class) bool {
	ok := true
	walk(n, func(m ast.Vertex) bool {
		switch x := m.(type) {
		case *ast.ScalarLnumber, *ast.ScalarDnumber, *ast.ScalarString, *ast.ExprBinaryConcat,
			*ast.ExprBinaryPlus, *ast.ExprBinaryMinus, *ast.ExprBinaryMul, *ast.ExprUnaryMinus,
			*ast.ExprBrackets, *ast.ExprBinaryShiftLeft, *ast.ExprBinaryBitwiseOr, *ast.ExprBinaryBitwiseAnd,
			*ast.Identifier, *ast.Name, *ast.NamePart, *ast.NameFullyQualified, *ast.NameRelative:
		case *ast.ExprConstFetch:
			switch strings.ToLower(identValue(x.Const)) {
			case "true", "false":
			default:
				ok = false
			}
		case *ast.ExprClassConstFetch:
			if strings.EqualFold(identValue(x.Const), "class") {
				return false
			}
			name := identValue(x.Class)
			var k *class
			switch strings.ToLower(name) {
			case "self", "static":
				k = c
			default:
				k = cv.classes[strings.ToLower(cv.resolveName(c.File, x.Class))]
			}
			if k == nil {
				// Server constants (TextFormat::RED) are constants too.
				return false
			}
			cd := k.findConst(identValue(x.Const))
			if cd == nil || !cd.isConst {
				ok = false
			}
			return false
		default:
			ok = false
		}
		return ok
	})
	return ok
}

func (cv *converter) docComment(c *class) string {
	kind := "class"
	switch c.Kind {
	case kindInterface:
		kind = "interface"
	case kindEnum:
		kind = "enum"
	}
	return fmt.Sprintf("// %s is converted from the PHP %s %s.", c.GoName, kind, c.FQCN)
}

func (cv *converter) emitClass(gf *goFile, c *class) string {
	var sb strings.Builder
	pf := cv.newFctx(gf, c, nil)
	pf.static = true
	// Constants.
	if len(c.Consts) > 0 {
		var consts, vars []string
		for _, k := range c.Consts {
			if k.Class != c {
				continue
			}
			f := cv.newFctx(gf, c, nil)
			f.static = true
			v := f.expr(k.Expr, nil)
			switch {
			case k.isConst && !strings.Contains(v.code, "Unsupported(") && !strings.Contains(v.code, "("):
				consts = append(consts, k.GoName+" = "+v.code)
			case strings.Contains(v.code, "Unsupported("):
				// Never panic while the server starts: leave it unset.
				vars = append(vars, k.GoName+" "+f.typeStr(k.Type)+" // "+strings.ReplaceAll(v.code, "\n", " "))
			default:
				vars = append(vars, k.GoName+" = "+f.arrayElemCode(v))
			}
		}
		if len(consts) > 0 {
			sb.WriteString("// Constants of " + c.FQCN + ".\nconst (\n" + strings.Join(consts, "\n") + "\n)\n\n")
		}
		if len(vars) > 0 {
			sb.WriteString("// Constants of " + c.FQCN + ".\nvar (\n" + strings.Join(vars, "\n") + "\n)\n\n")
		}
	}
	switch c.Kind {
	case kindInterface:
		sb.WriteString(cv.docComment(c) + "\ntype " + c.GoName + " interface {\n")
		for _, e := range c.ExtIfaces {
			if cv.isInterface(e) && !isAny(e) {
				sb.WriteString(pf.typeStr(e) + "\n")
			}
		}
		for _, i := range c.Ifaces {
			sb.WriteString(i.GoName + "\n")
		}
		for _, m := range c.Methods {
			if m.Static {
				continue
			}
			sb.WriteString(m.GoName + pf.sigStr(m.Sig, nil) + "\n")
		}
		sb.WriteString("}\n")
		cv.emitStatics(&sb, gf, c)
		return sb.String()
	case kindEnum:
		return sb.String() + cv.emitEnum(gf, c)
	}
	// The struct.
	sb.WriteString(cv.docComment(c) + "\ntype " + c.GoName + " struct {\n")
	if c.Parent != nil {
		sb.WriteString(c.Parent.GoName + "\n")
	} else if c.ExtEmbed != nil {
		sb.WriteString(pf.typeStr(c.ExtEmbed) + "\n")
	} else if c.Exception {
		sb.WriteString(pf.phpx("Exception") + "\n")
	}
	for _, t := range c.ExtTraits {
		sb.WriteString(pf.typeStr(t) + "\n")
	}
	if c.Poly {
		sb.WriteString("self " + c.IfaceName + "\n")
	}
	for _, p := range c.Props {
		if p.Static || p.Class != c {
			continue
		}
		if c.Parent != nil && c.Parent.findProp(p.Name) != nil {
			continue
		}
		sb.WriteString(p.GoName + " " + pf.typeStr(p.Type) + "\n")
	}
	sb.WriteString("}\n\n")
	if c.Poly {
		sb.WriteString(fmt.Sprintf("// %s is the interface of %s and its subclasses.\ntype %s interface {\n", c.IfaceName, c.GoName, c.IfaceName))
		ms := cv.localMethods[c.IfaceName]
		var names []string
		for n := range ms {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			sb.WriteString(n + pf.sigStr(ms[n], nil) + "\n")
		}
		sb.WriteString("}\n\n")
		sb.WriteString(fmt.Sprintf("func (%s *%s) %s() *%s { return %s }\n\n", c.Recv, c.GoName, asMethod(c), c.GoName, c.Recv))
	}
	cv.emitStatics(&sb, gf, c)
	sb.WriteString(cv.emitNew(gf, c))
	if c.listener {
		if tags := cv.listenerTags(c); tags != "" {
			sb.WriteString(tags)
		}
	}
	for _, m := range c.Methods {
		if m.Static && !m.HasBody {
			// An abstract static method: PHP would call the subclass's (late static binding).
			sb.WriteString(fmt.Sprintf("// %s is the abstract static method %s::%s().\nfunc %s%s {\npanic(%s(\"Error\", %q))\n}\n\n",
				m.GoName, c.Short, m.Name, m.GoName, pf.sigStr(m.Sig, nil), pf.phpx("NewError"), "Cannot call abstract method "+c.Short+"::"+m.Name+"()"))
			continue
		}
		if m.Abstract || !m.HasBody {
			continue
		}
		sb.WriteString(cv.emitMethod(gf, c, m))
	}
	for _, ad := range c.adapters {
		sb.WriteString(cv.emitAdapter(gf, c, ad))
	}
	sb.WriteString(fmt.Sprintf("// PhpClass is the PHP class name (get_class()).\nfunc (%s *%s) PhpClass() string { return %s }\n\n", c.Recv, c.GoName, quote(c.FQCN)))
	if cv.implementsJSON(c) {
		sb.WriteString(fmt.Sprintf("// MarshalJSON encodes the value jsonSerialize() returns.\nfunc (%s *%s) MarshalJSON() ([]byte, error) {\nreturn %s(%s(%s.JsonSerialize()))\n}\n\n",
			c.Recv, c.GoName, pf.pkgRef("encoding/json")+".Marshal", pf.phpx("JSONValue"), c.Recv))
	}
	return sb.String()
}

// implementsJSON reports whether the class needs a MarshalJSON wrapping its jsonSerialize().
func (cv *converter) implementsJSON(c *class) bool {
	m := c.findMethod("jsonSerialize")
	if m == nil || m.GoName == "MarshalJSON" {
		return false
	}
	// Once, on the class that declares it (subclasses inherit it, and dispatch through self).
	return m.Class == c
}

func (cv *converter) emitStatics(sb *strings.Builder, gf *goFile, c *class) {
	var vars []string
	for _, p := range c.Props {
		if !p.Static || p.Class != c {
			continue
		}
		f := cv.newFctx(gf, c, nil)
		f.static = true
		init := ""
		if p.Default != nil {
			v := f.expr(p.Default, p.Type)
			init = " = " + f.storeCode(v, p.Type)
			if strings.Contains(v.code, "Unsupported(") {
				init = " // " + strings.ReplaceAll(v.code, "\n", " ")
			}
		} else if isArrayT(p.Type) {
			init = " = " + f.phpx("NewArray") + "()"
		}
		vars = append(vars, p.GoName+" "+f.typeStr(p.Type)+init)
	}
	if len(vars) > 0 {
		sb.WriteString("// Static properties of " + c.FQCN + ".\nvar (\n" + strings.Join(vars, "\n") + "\n)\n\n")
	}
}

// emitNew writes the New function and the property initialiser.
func (cv *converter) emitNew(gf *goFile, c *class) string {
	var sb strings.Builder
	f := cv.newFctx(gf, c, nil)
	r := c.Recv
	// init<Name> sets the property defaults.
	sb.WriteString(fmt.Sprintf("// init%s sets the default values of the properties.\nfunc (%s *%s) init%s() {\n", c.GoName, r, c.GoName, c.GoName))
	if c.Parent != nil && !c.Parent.noInit {
		sb.WriteString(fmt.Sprintf("%s.%s.init%s()\n", r, c.Parent.GoName, c.Parent.GoName))
	}
	if c.Exception && c.Parent == nil {
		var chain []string
		for k := c; k != nil; k = k.Parent {
			chain = append(chain, quote(k.FQCN))
		}
		root := "Exception"
		if c.ExtParentPH != "" {
			root = c.ExtParentPH
		}
		_ = root
		sb.WriteString(fmt.Sprintf("%s.SetClasses(%s)\n", r, strings.Join(append(chain, quote(c.rootThrowable())), ", ")))
	} else if c.Exception {
		var chain []string
		for k := c; k != nil; k = k.Parent {
			chain = append(chain, quote(k.FQCN))
		}
		sb.WriteString(fmt.Sprintf("%s.SetClasses(%s)\n", r, strings.Join(append(chain, quote(c.rootThrowable())), ", ")))
	}
	for _, p := range c.Props {
		if p.Static || p.Class != c || p.Promoted {
			continue
		}
		if c.Parent != nil && c.Parent.findProp(p.Name) != nil && p.Default == nil {
			continue
		}
		switch {
		case p.Default != nil && !isNullLit(p.Default):
			v := f.expr(p.Default, p.Type)
			sb.WriteString(fmt.Sprintf("%s.%s = %s\n", r, p.GoName, f.storeCode(v, p.Type)))
		case isArrayT(p.Type):
			sb.WriteString(fmt.Sprintf("%s.%s = %s()\n", r, p.GoName, f.phpx("NewArray")))
		}
	}
	sb.WriteString("}\n\n")
	if strings.HasSuffix(sb.String(), "() {\n}\n\n") {
		// Nothing to initialise.
		sb.Reset()
		c.noInit = true
	}
	if c.Abstract {
		return sb.String()
	}
	// New<Name>.
	sig := cv.newSig(c)
	ctor := c.constructor()
	var params, args []string
	for i, pt := range sig.Params {
		name := "arg" + itoa(i)
		if i < len(sig.ParamNames) && sig.ParamNames[i] != "" {
			name = localName(sig.ParamNames[i])
		}
		if name == "obj" {
			name = "obj_"
		}
		ts := f.typeStr(pt)
		if sig.Variadic && i == len(sig.Params)-1 {
			ts = "..." + f.typeStr(pt.Elem)
			args = append(args, name+"...")
		} else {
			args = append(args, name)
		}
		params = append(params, name+" "+ts)
	}
	sb.WriteString(fmt.Sprintf("// New%s creates a %s (new %s(...) in PHP).\nfunc New%s(%s) *%s {\nobj := &%s{}\n",
		c.GoName, c.GoName, c.Short, c.GoName, strings.Join(params, ", "), c.GoName, c.GoName))
	if !c.noInit {
		sb.WriteString("obj.init" + c.GoName + "()\n")
	}
	// Polymorphic ancestors dispatch through self.
	path := "obj"
	for k := c; k != nil; k = k.Parent {
		if k != c {
			path += "." + k.GoName
		}
		if k.Poly {
			sb.WriteString(path + ".self = obj\n")
		}
	}
	switch {
	case ctor != nil:
		sb.WriteString("obj." + ctor.GoName + "(" + strings.Join(args, ", ") + ")\n")
	case c.Exception:
		sb.WriteString("obj.InitException(" + quote(c.FQCN) + ", " + strings.Join(args, ", ") + ")\n")
	}
	sb.WriteString("return obj\n}\n\n")
	return sb.String()
}

func (c *class) rootThrowable() string {
	for k := c; k != nil; k = k.Parent {
		if k.Parent == nil {
			if k.ExtParentPH != "" {
				return k.ExtParentPH
			}
		}
	}
	return "Exception"
}

// listenerTags writes EventHandlerTags for @priority/@handleCancelled/@notHandler doc tags.
func (cv *converter) listenerTags(c *class) string {
	var entries []string
	for _, m := range c.Methods {
		if m.Static || m.Private || m.Protected {
			continue
		}
		tags := docTags(m.Doc)
		var kv []string
		if v, ok := tags["priority"]; ok && len(v) > 0 {
			kv = append(kv, fmt.Sprintf("%q: %q", "priority", strings.ToUpper(strings.Fields(v[0] + " ")[0])))
		}
		if v, ok := tags["handlecancelled"]; ok {
			val := "true"
			if len(v) > 0 && v[0] != "" {
				val = strings.Fields(v[0])[0]
			}
			kv = append(kv, fmt.Sprintf("%q: %q", "handleCancelled", val))
		}
		if _, ok := tags["nothandler"]; ok {
			kv = append(kv, fmt.Sprintf("%q: %q", "notHandler", ""))
		}
		if len(kv) > 0 {
			entries = append(entries, fmt.Sprintf("%q: {%s},", m.GoName, strings.Join(kv, ", ")))
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return fmt.Sprintf("// EventHandlerTags gives the PHP doc-comment tags of the event handlers (@priority, ...).\nfunc (%s *%s) EventHandlerTags() map[string]map[string]string {\nreturn map[string]map[string]string{\n%s\n}\n}\n\n",
		c.Recv, c.GoName, strings.Join(entries, "\n"))
}

// emitMethod writes one method (or static method as a function).
func (cv *converter) emitMethod(gf *goFile, c *class, m *method) string {
	f := cv.newFctx(gf, c, m)
	f.static = m.Static
	header := ""
	if m.Static {
		header = "func " + m.GoName
	} else {
		header = fmt.Sprintf("func (%s *%s) %s", c.Recv, c.GoName, m.GoName)
	}
	return cv.emitBody(gf, f, m, header, fmt.Sprintf("// %s is converted from %s::%s().\n", m.GoName, c.Short, m.Name))
}

func (cv *converter) emitFunc(gf *goFile, fn *function) string {
	f := cv.newFctx(gf, nil, fn.m)
	f.static = true
	return cv.emitBody(gf, f, fn.m, "func "+fn.GoName, fmt.Sprintf("// %s is converted from the PHP function %s().\n", fn.GoName, fn.Name))
}

// emitBody writes a function: signature, parameter adaptation, locals and statements.
func (cv *converter) emitBody(gf *goFile, f *fctx, m *method, header, doc string) string {
	sig := m.Sig
	f.retType = m.Return
	f.results = sig.Results
	var params []string
	var prologue []string
	// Parameters.
	for i, p := range m.Params {
		lv := f.addVar(p.Name)
		lv.param, lv.fixed = true, true
		gf.idents[lv.goName] = true
		var goT *api.Type
		if i < len(sig.Params) {
			goT = sig.Params[i]
		} else {
			goT = p.GoType
		}
		variadic := sig.Variadic && i == len(sig.Params)-1
		phpT := p.Type
		if m.Adopted && i >= len(sig.Params) {
			// A PHP parameter the server's method doesn't have: it gets its default value.
			lv.t = p.Type
			init := f.zero(p.Type)
			if p.Default != nil {
				init = f.coerce(f.expr(p.Default, p.Type), p.Type)
			}
			prologue = append(prologue, "var "+lv.goName+" "+f.typeStr(p.Type)+" = "+init, "_ = "+lv.goName)
			continue
		}
		if m.Adopted {
			// The Go type comes from the server; PHP code sees the PHP type.
			if variadic {
				lv.t = arrayT(api.Int, goT.Elem)
				params = append(params, lv.goName+"Args "+f.paramTypeStr(goT, true))
				prologue = append(prologue, lv.goName+" := "+f.phpx("ToArray")+"("+lv.goName+"Args)")
				continue
			}
			if isAny(phpT) || api.Identical(phpT, goT) || (cv.assignable(goT, phpT) && !isArrayT(phpT)) ||
				cv.isInterface(goT) && !isAny(goT) && !cv.isInterface(phpT) && !isArrayT(phpT) && phpT.K == api.KPointer {
				// An interface of the server stays an interface: methods it lacks are reached
				// through the concrete type when called.
				lv.t = goT
				params = append(params, lv.goName+" "+f.typeStr(goT))
				continue
			}
			lv.t = phpT
			params = append(params, lv.goName+"Arg "+f.typeStr(goT))
			prologue = append(prologue, lv.goName+" := "+f.coerce(value{code: lv.goName + "Arg", t: goT, prec: 7}, phpT))
			continue
		}
		switch {
		case variadic:
			lv.t = p.Type
			params = append(params, lv.goName+"Args "+f.paramTypeStr(goT, true))
			prologue = append(prologue, lv.goName+" := "+f.phpx("ToArray")+"("+lv.goName+"Args)")
		case p.ByRef:
			lv.t = p.Type
			lv.ptr = true
			params = append(params, lv.goName+" "+f.typeStr(goT))
		default:
			lv.t = p.Type
			params = append(params, lv.goName+" "+f.typeStr(goT))
		}
		if p.Promote != nil {
			src := lv.goName
			if lv.ptr {
				src = "*" + src
			}
			prologue = append(prologue, fmt.Sprintf("%s.%s = %s", f.recv, p.Promote.GoName, f.coerce(value{code: src, t: lv.t, prec: 7}, p.Promote.Type)))
		}
	}
	// Go parameters PHP's method doesn't declare.
	for i := len(m.Params); i < len(sig.Params); i++ {
		variadic := sig.Variadic && i == len(sig.Params)-1
		params = append(params, "_ "+f.paramTypeStr(sig.Params[i], variadic))
	}
	// Arrays are values in PHP: a function that changes an array parameter changes its copy.
	modified := modifiedVars(m.Body)
	for _, p := range m.Params {
		lv := f.vars[p.Name]
		if isArrayT(lv.t) && !p.ByRef && modified[p.Name] && !p.Variadic {
			prologue = append(prologue, lv.goName+" = "+lv.goName+".Clone()")
		}
	}
	// Results.
	results := resultsStr(f, sig.Results)
	hasErr := len(sig.Results) > 0 && sig.Results[len(sig.Results)-1].K == api.KNamed && sig.Results[len(sig.Results)-1].Name == "error"
	if hasErr && m.Adopted {
		var named []string
		for i, r := range sig.Results {
			n := "err"
			if i < len(sig.Results)-1 {
				n = "_"
				if len(sig.Results) > 1 {
					n = "ret" + itoa(i)
				}
			}
			named = append(named, n+" "+f.typeStr(r))
		}
		results = " (" + strings.Join(named, ", ") + ")"
		prologue = append([]string{"defer " + f.phpx("Recover") + "(&err)"}, prologue...)
	}
	f.inferLocals(m.Body)
	for _, v := range f.vars {
		gf.idents[v.goName] = true
	}
	// Converted parameters that the body never reads.
	for _, p := range m.Params {
		lv := f.vars[p.Name]
		if strings.Contains(strings.Join(prologue, "\n"), lv.goName+" := ") {
			prologue = append(prologue, "_ = "+lv.goName)
		}
	}
	body := f.block(m.Body)
	decls := f.declareLocals(body)
	if len(sig.Results) > 0 && !terminates(body) && !endsWithReturn(m.Body) {
		body += "\n" + f.returnStmt(nil)
	}
	all := append(prologue, decls...)
	if strings.TrimSpace(body) != "" {
		all = append(all, body)
	}
	return doc + header + "(" + strings.Join(params, ", ") + ")" + results + " {\n" + strings.Join(all, "\n") + "\n}\n\n"
}

// modifiedVars returns the variables a function body writes into with $x[...] = or array
// functions that take the array by reference.
func modifiedVars(body []ast.Vertex) map[string]bool {
	out := map[string]bool{}
	walkAll(body, func(n ast.Vertex) bool {
		switch x := n.(type) {
		case *ast.ExprClosure, *ast.ExprArrowFunction:
			return false
		case *ast.ExprAssign:
			if d, ok := x.Var.(*ast.ExprArrayDimFetch); ok {
				for {
					if inner, ok := d.Var.(*ast.ExprArrayDimFetch); ok {
						d = inner
						continue
					}
					break
				}
				if name := varName(d.Var); name != "" {
					out[name] = true
				}
			}
		case *ast.StmtUnset:
			for _, v := range x.Vars {
				if d, ok := v.(*ast.ExprArrayDimFetch); ok {
					if name := varName(d.Var); name != "" {
						out[name] = true
					}
				}
			}
		case *ast.ExprFunctionCall:
			switch strings.ToLower(lastSeg(identValue(x.Function))) {
			case "array_push", "array_unshift", "array_pop", "array_shift", "array_splice", "sort", "rsort", "usort", "uasort", "uksort", "ksort", "krsort", "asort", "arsort", "shuffle":
				if len(x.Args) > 0 {
					if a, ok := x.Args[0].(*ast.Argument); ok {
						if name := varName(a.Expr); name != "" {
							out[name] = true
						}
					}
				}
			}
		}
		return true
	})
	return out
}

func (cv *converter) emitEnum(gf *goFile, c *class) string {
	f := cv.newFctx(gf, c, nil)
	f.static = true
	vt := f.enumValueType(c)
	var sb strings.Builder
	sb.WriteString(cv.docComment(c) + "\ntype " + c.GoName + " struct {\nname string\nvalue " + f.typeStr(vt) + "\n}\n\n")
	sb.WriteString("// Cases of " + c.FQCN + ".\nvar (\n")
	var names []string
	for _, e := range c.EnumCases {
		val := f.zero(vt)
		if e.Value != nil {
			val = f.coerce(f.expr(e.Value, vt), vt)
		}
		sb.WriteString(fmt.Sprintf("%s = &%s{name: %q, value: %s}\n", e.GoName, c.GoName, e.Name, val))
		names = append(names, e.GoName)
	}
	sb.WriteString(")\n\n")
	sb.WriteString(fmt.Sprintf("// %sCases is %s::cases().\nfunc %sCases() *%s {\nreturn %s(%s)\n}\n\n", c.GoName, c.Short, c.GoName, f.phpx("Array"), f.phpx("List"), strings.Join(names, ", ")))
	sb.WriteString(fmt.Sprintf("// %sTryFrom is %s::tryFrom().\nfunc %sTryFrom(v any) *%s {\nfor _, c := range []*%s{%s} {\nif %s(c.value, v) {\nreturn c\n}\n}\nreturn nil\n}\n\n",
		c.GoName, c.Short, c.GoName, c.GoName, c.GoName, strings.Join(names, ", "), f.phpx("LooseEq")))
	sb.WriteString(fmt.Sprintf("// %sFrom is %s::from().\nfunc %sFrom(v any) *%s {\nif c := %sTryFrom(v); c != nil {\nreturn c\n}\npanic(%s(\"ValueError\", %s(v)+\" is not a valid backing value for enum %s\"))\n}\n\n",
		c.GoName, c.Short, c.GoName, c.GoName, c.GoName, f.phpx("NewError"), f.phpx("ToString"), strings.ReplaceAll(c.FQCN, `\`, `\\`)))
	sb.WriteString(fmt.Sprintf("// Name is the case's name.\nfunc (%s *%s) Name() string { return %s.name }\n\n", c.Recv, c.GoName, c.Recv))
	cv.emitStatics(&sb, gf, c)
	for _, m := range c.Methods {
		if m.Abstract || !m.HasBody {
			continue
		}
		sb.WriteString(cv.emitMethod(gf, c, m))
	}
	return sb.String()
}

// emitAdapter writes a method that implements an interface method through another method.
func (cv *converter) emitAdapter(gf *goFile, c *class, ad *adapter) string {
	f := cv.newFctx(gf, c, nil)
	var params, args []string
	for i, pt := range ad.Sig.Params {
		name := "a" + itoa(i)
		params = append(params, name+" "+f.paramTypeStr(pt, ad.Sig.Variadic && i == len(ad.Sig.Params)-1))
		args = append(args, name)
	}
	header := fmt.Sprintf("func (%s *%s) %s(%s)%s {\n", c.Recv, c.GoName, ad.GoName, strings.Join(params, ", "), resultsStr(f, ad.Sig.Results))
	if ad.Target == "" {
		body := f.phpx("Throw") + "(" + f.phpx("NewError") + "(\"Error\", " + quote("method "+c.Short+"::"+ad.PHPName+"() isn't implemented") + "))"
		if len(ad.Sig.Results) > 0 {
			var zs []string
			for _, r := range ad.Sig.Results {
				zs = append(zs, f.zero(r))
			}
			body += "\nreturn " + strings.Join(zs, ", ")
		}
		cv.todos++
		cv.warnf(c.File.Path, "%s::%s() is required by an interface but has no implementation", c.FQCN, ad.PHPName)
		return fmt.Sprintf("// %s is required by an interface. TODO(phar2go): implement it.\n%s%s\n}\n\n", ad.GoName, header, body)
	}
	ts := ad.TargetSig
	var cargs []string
	for i, pt := range ts.Params {
		if i < len(args) {
			cargs = append(cargs, f.coerce(value{code: args[i], t: ad.Sig.Params[i], prec: 7}, pt))
		} else {
			cargs = append(cargs, f.zero(pt))
		}
	}
	call := c.Recv + "." + ad.Target + "(" + strings.Join(cargs, ", ") + ")"
	var body string
	switch {
	case len(ad.Sig.Results) == 0:
		body = call
	case len(ts.Results) == 0:
		body = call + "\nreturn " + f.zero(ad.Sig.Results[0])
	default:
		body = "return " + f.coerce(value{code: call, t: ts.Results[0], prec: 7, call: true}, ad.Sig.Results[0])
	}
	return fmt.Sprintf("// %s implements %s() with %s.\n%s%s\n}\n\n", ad.GoName, ad.PHPName, ad.Target, header, body)
}

// extAncestors returns the PHP names of the classes a server type extends (through embedding).
func (cv *converter) extAncestors(t *api.Type) []string {
	var out []string
	seen := map[string]bool{}
	var walkT func(t *api.Type)
	walkT = func(t *api.Type) {
		if t == nil {
			return
		}
		ti := cv.typeInfo(t.Deref())
		if ti == nil || seen[t.Deref().Name] {
			return
		}
		seen[t.Deref().Name] = true
		for _, e := range ti.Embedded {
			if ei := cv.typeInfo(e.Deref()); ei != nil && ei.PHP != "" {
				out = append(out, ei.PHP)
			}
			walkT(e)
		}
	}
	walkT(t)
	return out
}

// endsWithReturn reports whether PHP statements end with a return statement (a Go terminating
// statement once converted).
func endsWithReturn(stmts []ast.Vertex) bool {
	if len(stmts) == 0 {
		return false
	}
	_, ok := stmts[len(stmts)-1].(*ast.StmtReturn)
	return ok
}
