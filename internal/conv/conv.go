// Package conv converts the PHP code of a PocketMine-MP plugin to Go for pocketmine-go.
package conv

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// Options configure a conversion.
type Options struct {
	// Package is the Go package name of the plugin.
	Package string
	// PhpxImport is the import path of the phpx runtime in the output.
	PhpxImport string
	// MainClass is the FQCN of the plugin's main class (plugin.yml "main").
	MainClass string
	// PluginName is the plugin's name.
	PluginName string
}

// Result is the converted Go code.
type Result struct {
	// Files maps file names (in the plugin's package folder) to Go source.
	Files map[string][]byte
	// Warnings lists what couldn't be converted exactly.
	Warnings []string
	// TODOs counts the places marked with TODO(phar2go) in the code.
	TODOs int
	// Classes is the number of converted classes.
	Classes int
	// Methods is the number of converted methods and functions.
	Methods int
	// External lists PHP classes the plugin uses that pocketmine-go doesn't have.
	External []string
	// MainType is the Go type of the plugin's main class.
	MainType string
	// UsesServer is set when the code needs the plugin instance for Server::getInstance().
	UsesServer bool
	// UsesSQL is set when the plugin bundles libasynql: the converted plugin then needs the
	// database/sql drivers.
	UsesSQL bool
}

type converter struct {
	idx  *api.Index
	opts Options

	files      []*phpFile
	classes    map[string]*class
	classList  []*class
	byGoType   map[string]*class
	funcs      map[string]*function
	funcFile   map[*function]*phpFile
	anonCount  int
	anonByNode map[ast.Vertex]*class

	external     map[string]bool
	warnings     []string
	warnSeen     map[string]bool
	topLevelCode []string
	todos        int

	extCache      map[string]*api.Type
	concreteCache map[string]concreteResult
	uses          map[*phpFile]map[string]string
	localMethods  map[string]map[string]*api.Func
	pkgNames      map[string]bool
	defines       map[string]string // define()d constants -> Go name
	extKnown      map[string]bool   // server classes checked with class_exists()
	usesServer    bool
	mainType      string
	// srcRoot is the folder of the main class: file names are relative to it.
	srcRoot string

	// staticVars are package-level variables for `static $x` in functions.
	staticVars    []string
	staticImports []*importSet
}

// Convert converts the PHP files (path -> source) of a plugin.
func Convert(idx *api.Index, files map[string][]byte, opts Options) (*Result, error) {
	cv := &converter{
		idx: idx, opts: opts,
		classes: map[string]*class{}, byGoType: map[string]*class{},
		funcs: map[string]*function{}, funcFile: map[*function]*phpFile{},
		anonByNode: map[ast.Vertex]*class{},
		external:   map[string]bool{}, warnSeen: map[string]bool{},
		extCache: map[string]*api.Type{}, concreteCache: map[string]concreteResult{},
		uses: map[*phpFile]map[string]string{}, localMethods: map[string]map[string]*api.Func{},
		pkgNames: map[string]bool{}, defines: map[string]string{}, extKnown: map[string]bool{},
	}
	files, usesSQL := replaceVirions(files)
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var errs []string
	for _, p := range paths {
		f, err := parsePHP(p, files[p])
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		cv.files = append(cv.files, f)
	}
	if len(errs) > 0 && len(cv.files) == 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	for _, e := range errs {
		cv.warnf("", "%s (file skipped)", e)
	}
	cv.addStubs()
	for _, f := range cv.files {
		cv.collectClasses(f)
		cv.collectAnonClasses(f)
		cv.collectDefines(f)
	}
	cv.mergeTraits()
	cv.linkClasses()
	cv.prepare()

	res := &Result{Files: map[string][]byte{}}
	if mc := cv.classes[strings.ToLower(opts.MainClass)]; mc != nil {
		res.MainType = mc.GoName
		cv.mainType = mc.GoName
		if i := strings.LastIndexByte(mc.File.Path, '/'); i >= 0 {
			cv.srcRoot = mc.File.Path[:i+1]
		}
		if mc.Abstract || mc.Kind != kindClass {
			return nil, fmt.Errorf("the main class %s can't be instantiated", opts.MainClass)
		}
	}
	cv.emitAll(res)
	res.Warnings = cv.warnings
	res.UsesServer = cv.usesServer
	res.UsesSQL = usesSQL
	for _, f := range cv.files {
		// phar2go's SQLite3 and mysqli classes.
		if f.Path == "phar2go-stubs/SQLite3.php" || f.Path == "phar2go-stubs/mysqli.php" {
			res.UsesSQL = true
		}
	}
	res.TODOs = cv.todos
	for _, c := range cv.classList {
		if c.Kind != kindTrait {
			res.Classes++
			res.Methods += len(c.Methods)
		}
	}
	res.Methods += len(cv.funcs)
	for e := range cv.external {
		res.External = append(res.External, e)
	}
	sort.Strings(res.External)
	return res, nil
}

//go:embed stubs/*.php
var stubs embed.FS

//go:embed virions
var virionStubs embed.FS

// virionReplacements are libraries (virions) that can't work on pocketmine-go as they are:
// the anchor class identifies the library (whatever namespace it was shaded to) and its files
// are replaced by phar2go's version in virions/<dir>.
var virionReplacements = []struct {
	namespace string // the library's namespace, without the shading prefix
	anchor    string // the file of its main class
	dir       string
}{
	// libasynql needs PHP threads and the sqlite3/mysqli extensions; the replacement runs the
	// queries on Go's database/sql.
	{`poggit\libasynql`, "libasynql.php", "libasynql"},
	// SimplePacketHandler works on PocketMine-MP's packet classes.
	{`muqsit\simplepackethandler`, "SimplePacketHandler.php", "simplepackethandler"},
	// bStats sends plugin statistics from a PHP thread.
	{`bStats\PocketmineMp`, "Metrics.php", "bstats"},
	// libSQL runs queries on PHP threads with the sqlite3/mysqli extensions.
	{`cooldogedev\libSQL`, "ConnectionPool.php", "libsql"},
}

// replaceVirions swaps bundled libraries for phar2go's versions (see virionReplacements). It
// reports whether libasynql was replaced (the plugin then needs database drivers).
func replaceVirions(files map[string][]byte) (map[string][]byte, bool) {
	usesSQL := false
	reNS := regexp.MustCompile(`(?m)^\s*namespace\s+([^;\s]+)\s*;`)
	for _, v := range virionReplacements {
		var dir, ns string
		for p, src := range files {
			if strings.HasSuffix(p, "/"+v.anchor) {
				if m := reNS.FindSubmatch(src); m != nil && strings.HasSuffix(strings.ToLower(string(m[1])), strings.ToLower(v.namespace)) {
					dir, ns = p[:strings.LastIndexByte(p, '/')+1], string(m[1])
					break
				}
			}
		}
		if dir == "" {
			// Not bundled, but used: add the replacement under its usual namespace.
			used := false
			for _, src := range files {
				if strings.Contains(string(src), v.namespace+`\`) {
					used = true
					break
				}
			}
			if !used {
				continue
			}
			dir, ns = "phar2go-virions/"+v.dir+"/", v.namespace
		}
		out := make(map[string][]byte, len(files))
		for p, src := range files {
			// The library's own files: those in its namespace (its folder may hold others too).
			if strings.HasPrefix(p, dir) && strings.HasSuffix(p, ".php") {
				if m := reNS.FindSubmatch(src); m != nil && (strings.EqualFold(string(m[1]), ns) || strings.HasPrefix(strings.ToLower(string(m[1])), strings.ToLower(ns)+`\`)) {
					continue
				}
			}
			out[p] = src
		}
		root := "virions/" + v.dir
		fs.WalkDir(virionStubs, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, _ := virionStubs.ReadFile(p)
			out[dir+strings.TrimPrefix(p, root+"/")] = []byte(strings.ReplaceAll(string(data), "__NS__", ns))
			return nil
		})
		files = out
		if v.dir == "libasynql" || v.dir == "libsql" {
			usesSQL = true
		}
	}
	return files, usesSQL
}

// addStubs adds phar2go's PHP versions of classes the plugin uses but doesn't contain:
// PocketMine-MP traits pocketmine-go has no Go type for, and PHP's SPL classes.
func (cv *converter) addStubs() {
	defined := map[string]bool{}
	var src strings.Builder
	reDecl := regexp.MustCompile(`(?m)^\s*(?:abstract\s+|final\s+)*(?:class|interface|trait|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	for _, f := range cv.files {
		src.Write(f.Src)
		for _, m := range reDecl.FindAllSubmatch(f.Src, -1) {
			defined[strings.ToLower(string(m[1]))] = true
		}
	}
	all := src.String()
	entries, _ := stubs.ReadDir("stubs")
	added := map[string]bool{}
	// Stubs can use other stubs (ThreadSafeArray uses ArrayIterator): repeat until none is added.
	for changed := true; changed; {
		changed = false
		for _, e := range entries {
			if e.IsDir() || added[e.Name()] {
				continue
			}
			data, _ := stubs.ReadFile("stubs/" + e.Name())
			used := false
			for _, m := range reDecl.FindAllSubmatch(data, -1) {
				name := string(m[1])
				if !defined[strings.ToLower(name)] && regexp.MustCompile(`\b`+name+`\b`).MatchString(all) {
					used = true
				}
			}
			if !used {
				continue
			}
			added[e.Name()] = true
			if f, err := parsePHP("phar2go-stubs/"+e.Name(), data); err == nil {
				cv.files = append(cv.files, f)
				all += string(data)
				for _, m := range reDecl.FindAllSubmatch(data, -1) {
					defined[strings.ToLower(string(m[1]))] = true
				}
				changed = true
			}
		}
	}
}

func (cv *converter) warnf(file string, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if file != "" {
		msg = file + ": " + msg
	}
	if cv.warnSeen[msg] {
		return
	}
	cv.warnSeen[msg] = true
	cv.warnings = append(cv.warnings, msg)
}

func (cv *converter) collectAnonClasses(f *phpFile) {
	walk(f.Root, func(n ast.Vertex) bool {
		if nw, ok := n.(*ast.ExprNew); ok {
			if sc, ok := nw.Class.(*ast.StmtClass); ok {
				c := cv.addClass(f, sc, kindClass)
				cv.anonByNode[sc] = c
			}
		}
		return true
	})
}

// collectDefines records define("NAME", value) calls.
func (cv *converter) collectDefines(f *phpFile) {
	walk(f.Root, func(n ast.Vertex) bool {
		if fc, ok := n.(*ast.ExprFunctionCall); ok && strings.EqualFold(lastSeg(identValue(fc.Function)), "define") && len(fc.Args) >= 1 {
			if a, ok := fc.Args[0].(*ast.Argument); ok {
				if s, ok := a.Expr.(*ast.ScalarString); ok {
					name := unquotePHP(s)
					cv.defines[strings.ToLower(name)] = "const" + pascal(strings.ToLower(name))
				}
			}
		}
		return true
	})
}

func lastSeg(s string) string {
	if i := strings.LastIndexByte(s, '\\'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// uniqueName reserves a package-level Go name.
func (cv *converter) uniqueName(base string) string {
	name := base
	for i := 2; cv.pkgNames[name] || goKeywords[name] || goPredeclared[name] || name == cv.opts.Package; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	cv.pkgNames[name] = true
	return name
}

// prepare names everything and computes the Go signatures.
func (cv *converter) prepare() {
	cv.pkgNames["init"] = true
	cv.pkgNames["main"] = true
	cv.pkgNames["files"] = true
	cv.pkgNames["PluginInstance"] = true
	// Class names: short names, qualified with namespace parts on collisions.
	shortCount := map[string]int{}
	for _, c := range cv.classList {
		shortCount[strings.ToLower(pascal(c.Short))]++
	}
	for _, c := range cv.classList {
		name := pascal(c.Short)
		if c.anonID > 0 {
			name = pascal(c.Short)
		} else if shortCount[strings.ToLower(name)] > 1 {
			parts := strings.Split(c.FQCN, "\\")
			if len(parts) > 1 {
				name = pascal(parts[len(parts)-2]) + name
			}
		}
		c.GoName = cv.uniqueName(name)
		cv.byGoType[c.GoName] = c
	}
	for _, c := range cv.classList {
		if c.Kind == kindClass && (c.Abstract || len(c.Subclasses) > 0) {
			c.Poly = true
			c.IfaceName = cv.uniqueName(c.GoName + "Like")
			cv.byGoType[c.IfaceName] = c
		}
	}
	// Embedded server types.
	for _, c := range cv.classList {
		if c.ExtParent == nil {
			continue
		}
		ti := cv.typeInfo(c.ExtParent.Deref())
		if ti == nil {
			continue
		}
		if !ti.Interface {
			if ti.Underlying.K == api.KStruct {
				c.ExtEmbed = c.ExtParent.Deref()
			}
			continue
		}
		// An interface: embed the package's base struct for it (Task -> TaskBase).
		pkg := cv.idx.Packages[c.ExtParent.PkgPath()]
		base := c.ExtParent.ObjName() + "Base"
		if t, ok := pkg.Types[base]; ok && !t.Interface {
			c.ExtEmbed = api.Named(pkg.Path + "." + base)
		} else {
			cv.warnf(c.File.Path, "%s extends %s: no base type to embed was found", c.FQCN, c.ExtParentPH)
		}
	}
	// A class extending a server class without a constructor of its own takes the server
	// constructor's arguments (new class($id, $name) extends SpawnEgg{...}).
	for _, c := range cv.classList {
		if c.Kind == kindClass && c.ExtEmbed != nil && c.constructor() == nil {
			cv.addImplicitCtor(c)
		}
	}
	// Static and instance member names, signatures.
	for _, c := range cv.classList {
		cv.nameMembers(c)
	}
	for _, c := range cv.classList {
		for _, m := range c.Methods {
			cv.methodSig(c, m)
		}
	}
	for _, fn := range sortedFuncs(cv.funcs) {
		fn.GoName = cv.uniqueName(camel(fn.Name))
		fn.m.GoName = fn.GoName
		cv.methodSig(nil, fn.m)
		_ = fn
	}
	for _, c := range cv.classList {
		cv.propTypes(c)
	}
	for _, c := range cv.classList {
		cv.buildMethodSet(c)
	}
	for _, c := range cv.classList {
		cv.chooseRecv(c)
	}
}

func sortedFuncs(m map[string]*function) []*function {
	var out []*function
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (cv *converter) nameMembers(c *class) {
	used := map[string]bool{"self": true, "phpSelf": true}
	if c.ExtEmbed != nil {
		used[c.ExtEmbed.ObjName()] = true
	}
	if c.Parent != nil {
		used[c.Parent.GoName] = true
	}
	for _, p := range c.Props {
		if p.Static {
			p.GoName = cv.uniqueName(camel(c.GoName) + pascal(p.Name))
			continue
		}
		// Inherited (redeclared) properties keep the parent's field.
		if c.Parent != nil {
			if pp := c.Parent.findProp(p.Name); pp != nil && !pp.Static {
				p.GoName = pp.GoName
				p.Base = pp
				for pp.Base != nil {
					pp = pp.Base
				}
				p.Base = pp
				continue
			}
		}
		name := camel(p.Name)
		for used[name] {
			name += "_"
		}
		used[name] = true
		p.GoName = name
	}
	for _, k := range c.Consts {
		k.GoName = cv.uniqueName(c.GoName + pascal(k.Name))
	}
	for _, e := range c.EnumCases {
		e.GoName = cv.uniqueName(c.GoName + pascal(e.Name))
	}
	for _, m := range c.Methods {
		if m.Static {
			base := c.GoName + pascal(m.Name)
			if m.Private {
				base = camel(c.GoName) + pascal(m.Name)
			}
			m.GoName = cv.uniqueName(base)
			continue
		}
		if m.Key == "__construct" {
			m.GoName = "construct" + c.GoName
			continue
		}
	}
	// Instance method names are decided with the signatures (methodSig); avoid field clashes.
	_ = used
}

// optionalHooks are methods the server calls through optional interfaces, so they aren't in
// the base type's method set: type -> PHP name -> Go name and signature.
var optionalHooks = map[string]map[string]struct {
	name string
	sig  *api.Func
}{
	"pocketmine/plugin.PluginBase": {
		"onenable":  {"OnEnable", &api.Func{Results: []*api.Type{api.Error}}},
		"ondisable": {"OnDisable", &api.Func{}},
		"onload":    {"OnLoad", &api.Func{}},
	},
}

// extMethodFor finds a server method that a plugin method implements or overrides.
func (cv *converter) extMethodFor(c *class, m *method) (string, *api.Func) {
	for k := c; k != nil; k = k.Parent {
		if k.ExtEmbed != nil {
			if hooks, ok := optionalHooks[strings.TrimPrefix(k.ExtEmbed.Name, cv.idx.Module+"/")]; ok {
				if h, ok := hooks[m.Key]; ok {
					return h.name, h.sig
				}
			}
		}
	}
	var types []*api.Type
	for k := c; k != nil; k = k.Parent {
		if k.ExtEmbed != nil {
			types = append(types, k.ExtParent, api.Ptr(k.ExtEmbed))
			types = append(types, cv.completionIfaces(k.ExtEmbed)...)
		}
	}
	for k := c; k != nil; k = k.Parent {
		for _, t := range k.ExtTraits {
			types = append(types, api.Ptr(t))
		}
	}
	types = append(types, c.allExtIfaces()...)
	if c.Exception {
		types = append(types, exceptionT)
	}
	for _, t := range types {
		ms := cv.methodSet(t)
		if ms == nil {
			if ti := cv.typeInfo(t.Deref()); ti != nil {
				ms = ti.Methods
			}
		}
		var extra []string
		if ti := cv.typeInfo(t.Deref()); ti != nil && ti.PHP != "" {
			if target, ok := cv.idx.PHPMembers[strings.ToLower(ti.PHP+"::"+m.Name)]; ok {
				extra = append(extra, target[strings.LastIndexByte(target, '.')+1:])
			}
		}
		if n := findMethodName(ms, m.Name, extra...); n != "" {
			return n, ms[n]
		}
	}
	return "", nil
}

// completionIfaces returns the interfaces of a server base struct's package that the struct
// almost implements: the missing methods are the abstract methods of the PHP class it ports
// (command.Command lacks Execute of command.CommandLike).
func (cv *converter) completionIfaces(embed *api.Type) []*api.Type {
	key := "completion|" + embed.Name
	if r, ok := cv.concreteCache[key]; ok {
		if r.t == nil {
			return nil
		}
		return r.t.Params
	}
	pkg := cv.idx.Packages[embed.PkgPath()]
	have := cv.methodSet(api.Ptr(embed))
	var out []*api.Type
	if pkg != nil {
		var names []string
		for n := range pkg.Types {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			ti := pkg.Types[n]
			if !ti.Interface || ti.Unexported || len(ti.Methods) < 3 {
				continue
			}
			missing, shared := 0, 0
			for m := range ti.Methods {
				if _, ok := have[m]; ok {
					shared++
				} else {
					missing++
				}
			}
			if missing >= 1 && missing <= 3 && shared >= 2*missing {
				out = append(out, &api.Type{K: api.KNamed, Name: pkg.Path + "." + n, Iface: true})
			}
		}
	}
	cv.concreteCache[key] = concreteResult{t: &api.Type{Params: out}}
	return out
}

// methodSig decides the Go name and signature of a method.
func (cv *converter) methodSig(c *class, m *method) {
	f := cv.fileOf(c, m)
	// PHP-side types of the parameters and the result.
	for _, p := range m.Params {
		p.Type = cv.phpType(f, p.TypeNode, docType(m.Doc, "param", p.Name), c)
		if p.Type == nil {
			p.Type = api.Any
			if p.Default != nil {
				if t := literalType(p.Default); t != nil && !t.IsNil() {
					p.Type = t
				}
			}
		}
		if p.Type.IsVoid() {
			p.Type = api.Any
		}
		// A parameter with a null default can be null.
		if p.Default != nil && isNullLit(p.Default) && !p.Type.Nilable() && !isArrayT(p.Type) {
			p.Type = api.Any
		}
		if p.Variadic {
			p.GoType = p.Type
			p.Type = arrayT(api.Int, p.Type)
		} else if p.ByRef {
			p.GoType = api.Ptr(p.Type)
		} else {
			p.GoType = p.Type
		}
	}
	ret := cv.phpType(f, m.ReturnNode, docType(m.Doc, "return", ""), c)
	if m.HasBody && containsYield(m.Body) {
		ret = generatorT
	}
	if ret == nil {
		if m.HasBody && returnsValue(m.Body) {
			ret = api.Any
		} else if m.HasBody || m.Key == "__construct" {
			ret = api.Void
		} else {
			ret = api.Any
		}
	}
	m.Return = ret
	if m.Static || c == nil {
		m.Sig = cv.phpSig(m)
		return
	}
	if m.Key == "__construct" {
		m.Sig = cv.phpSig(m)
		m.Sig.Results = nil
		return
	}
	// Overrides of server methods use the server's signature.
	if name, sig := cv.extMethodFor(c, m); sig != nil && !hiddenTypes(sig) {
		m.GoName, m.Sig, m.Adopted = name, sig, true
		return
	}
	// Overrides of plugin methods keep the overridden method's name and signature.
	if c.Parent != nil || len(c.Ifaces) > 0 {
		var base *method
		if c.Parent != nil {
			base = c.Parent.findMethodIfaces(m.Name)
		}
		if base == nil {
			for _, i := range c.Ifaces {
				if base = i.findMethodIfaces(m.Name); base != nil {
					break
				}
			}
		}
		if base != nil && base.Sig != nil && !base.Static {
			m.GoName, m.Sig, m.Adopted = base.GoName, base.Sig, base.Adopted
			return
		}
	}
	switch m.Key {
	case "__tostring":
		m.GoName = "String"
		m.Sig = &api.Func{Results: []*api.Type{api.String}}
		return
	case "jsonserialize":
		m.GoName = "JsonSerialize"
		m.Sig = &api.Func{Results: []*api.Type{api.Any}}
		return
	case "__invoke":
		m.GoName = "Invoke"
	}
	if m.GoName == "" {
		if m.Private {
			m.GoName = camel(m.Name)
			for c.findPropGo(m.GoName) || m.GoName == c.Recv {
				m.GoName += "Fn"
			}
		} else {
			m.GoName = pascal(m.Name)
		}
	}
	m.Sig = cv.phpSig(m)
}

func (c *class) findPropGo(goName string) bool {
	for k := c; k != nil; k = k.Parent {
		for _, p := range k.Props {
			if !p.Static && p.GoName == goName {
				return true
			}
		}
	}
	return false
}

func (cv *converter) phpSig(m *method) *api.Func {
	sig := &api.Func{}
	for i, p := range m.Params {
		sig.Params = append(sig.Params, p.GoType)
		sig.ParamNames = append(sig.ParamNames, p.Name)
		if p.Variadic && i == len(m.Params)-1 {
			sig.Variadic = true
			sig.Params[i] = api.SliceOf(p.GoType)
		}
	}
	if !m.Return.IsVoid() {
		sig.Results = []*api.Type{m.Return}
	}
	return sig
}

func (cv *converter) fileOf(c *class, m *method) *phpFile {
	if c != nil {
		if m != nil && m.FromTrait != nil {
			return m.FromTrait.File
		}
		return c.File
	}
	for fn, f := range cv.funcFile {
		if fn.m == m {
			return f
		}
	}
	return nil
}

// returnsValue reports whether a function body returns a value.
func returnsValue(body []ast.Vertex) bool {
	found := false
	walkAll(body, func(n ast.Vertex) bool {
		switch x := n.(type) {
		case *ast.ExprClosure, *ast.ExprArrowFunction, *ast.StmtClass, *ast.StmtFunction:
			return false
		case *ast.StmtReturn:
			if x.Expr != nil {
				found = true
			}
		case *ast.ExprYield, *ast.ExprYieldFrom:
			found = true
		}
		return !found
	})
	return found
}

// propTypes decides the types of properties.
func (cv *converter) propTypes(c *class) {
	for _, p := range c.Props {
		if p.Promoted {
			if ctor := c.method("__construct"); ctor != nil {
				for _, pa := range ctor.Params {
					if pa.Promote == p {
						p.Type = pa.Type
					}
				}
			}
			if p.Type != nil {
				continue
			}
		}
		f := c.File
		if p.File != nil {
			f = p.File
		}
		p.Type = cv.phpType(f, p.TypeNode, docType(p.Doc, "var", ""), c)
		if p.Type == nil && p.Default != nil {
			p.Type = literalType(p.Default)
			if p.Type != nil && p.Type.IsNil() {
				p.Type = nil
			}
		}
		if p.Type == nil {
			p.Type = cv.inferPropType(c, p)
		}
		if p.Type == nil || p.Type.IsVoid() {
			p.Type = api.Any
		}
		// A property with a null default can't be a plain scalar.
		if p.Default != nil && isNullLit(p.Default) && !p.Type.Nilable() && !isArrayT(p.Type) {
			p.Type = api.Any
		}
		if (p.TypeNode == nil || isNullableNode(p.TypeNode)) && !p.Type.Nilable() && !isArrayT(p.Type) && p.Default == nil && !p.Promoted {
			// Untyped and uninitialised: it is null until assigned.
			if p.TypeNode != nil {
				p.Type = api.Any
			}
		}
	}
}

func isNullableNode(n ast.Vertex) bool {
	_, ok := n.(*ast.Nullable)
	return ok
}

// inferPropType guesses the type of an untyped property from what the class assigns to it.
func (cv *converter) inferPropType(c *class, p *prop) *api.Type {
	var types []*api.Type
	for _, m := range c.Methods {
		params := map[string]*api.Type{}
		for _, pa := range m.Params {
			params[pa.Name] = pa.Type
		}
		walkAll(m.Body, func(n ast.Vertex) bool {
			as, ok := n.(*ast.ExprAssign)
			if !ok {
				return true
			}
			pf, ok := as.Var.(*ast.ExprPropertyFetch)
			if !ok || varName(pf.Var) != "this" || identValue(pf.Prop) != p.Name {
				return true
			}
			var t *api.Type
			switch e := as.Expr.(type) {
			case *ast.ExprVariable:
				if vn := varName(e); vn == "this" {
					t = cv.classType(c)
				} else {
					t = params[vn]
				}
			case *ast.ExprNew:
				if _, anon := e.Class.(*ast.StmtClass); anon {
					if ac := cv.anonByNode[e.Class]; ac != nil {
						t = cv.classType(ac)
					}
				} else if n := identValue(e.Class); n != "" {
					t = cv.classRef(cv.resolveName(c.File, e.Class), c)
				}
			case *ast.ExprArray:
				t = arrayT(nil, nil)
			default:
				t = literalType(e)
			}
			if t == nil {
				t = api.Any
			}
			if !t.IsNil() {
				types = append(types, t)
			}
			return true
		})
	}
	if len(types) == 0 {
		return nil
	}
	for _, t := range types[1:] {
		if !api.Identical(t, types[0]) {
			return api.Any
		}
	}
	return types[0]
}

// literalType is the type of a literal expression, or nil.
func literalType(n ast.Vertex) *api.Type {
	switch x := n.(type) {
	case *ast.ScalarLnumber:
		return api.Int
	case *ast.ScalarDnumber:
		return api.Float
	case *ast.ScalarString, *ast.ScalarEncapsed, *ast.ScalarHeredoc:
		return api.String
	case *ast.ExprArray:
		return arrayT(nil, nil)
	case *ast.ExprUnaryMinus:
		return literalType(x.Expr)
	case *ast.ExprConstFetch:
		switch strings.ToLower(identValue(x.Const)) {
		case "true", "false":
			return api.Bool
		case "null":
			return api.UntypedNil
		}
	}
	return nil
}

func isNullLit(n ast.Vertex) bool {
	c, ok := n.(*ast.ExprConstFetch)
	return ok && strings.EqualFold(identValue(c.Const), "null")
}

// buildMethodSet records the Go method set of each plugin type.
func (cv *converter) buildMethodSet(c *class) {
	ms := map[string]*api.Func{}
	switch c.Kind {
	case kindInterface:
		var add func(k *class)
		add = func(k *class) {
			for _, m := range k.Methods {
				if !m.Static {
					ms[m.GoName] = m.Sig
				}
			}
			for _, i := range k.Ifaces {
				add(i)
			}
		}
		add(c)
		for _, e := range c.ExtIfaces {
			for n, f := range cv.methodSet(e) {
				ms[n] = f
			}
		}
		cv.localMethods[c.GoName] = ms
		return
	case kindTrait:
		return
	}
	chain := c.ancestors()
	for i := len(chain) - 1; i >= 0; i-- {
		k := chain[i]
		if k.ExtEmbed != nil {
			for n, f := range cv.methodSet(api.Ptr(k.ExtEmbed)) {
				ms[n] = f
			}
		}
		for _, t := range k.ExtTraits {
			for n, f := range cv.methodSet(api.Ptr(t)) {
				ms[n] = f
			}
		}
		if k.Exception {
			for n, f := range cv.methodSet(exceptionT) {
				ms[n] = f
			}
		}
		for _, m := range k.Methods {
			if !m.Static && m.Key != "__construct" && m.HasBody {
				ms[m.GoName] = m.Sig
			}
		}
	}
	if c.Kind == kindEnum {
		ms["Name"] = &api.Func{Results: []*api.Type{api.String}}
	}
	// Methods of the plugin's interfaces that the class gets under another Go name from the
	// server type it extends (getAliases -> Aliases), or that nothing implements.
	if !c.Abstract || c.Poly {
		seen := map[string]bool{}
		var visit func(k *class)
		visit = func(k *class) {
			for _, i := range k.Ifaces {
				var walkI func(i *class)
				walkI = func(i *class) {
					if seen["i:"+i.GoName] {
						return
					}
					seen["i:"+i.GoName] = true
					for _, m := range i.Methods {
						if m.Static || m.Sig == nil {
							continue
						}
						if have, ok := ms[m.GoName]; ok && api.SameSig(have, m.Sig) {
							continue
						}
						if _, ok := ms[m.GoName]; ok {
							continue
						}
						ad := &adapter{GoName: m.GoName, Sig: m.Sig, PHPName: m.Name}
						if t := findMethodName(ms, m.Name); t != "" {
							ad.Target, ad.TargetSig = t, ms[t]
						}
						c.adapters = append(c.adapters, ad)
						ms[m.GoName] = m.Sig
					}
					for _, sup := range i.Ifaces {
						walkI(sup)
					}
				}
				walkI(i)
			}
		}
		for _, k := range c.ancestors() {
			visit(k)
		}
	}
	marshal := &api.Func{Results: []*api.Type{api.SliceOf(api.Basic("uint8")), api.Error}}
	if c.findMethodIfaces("jsonSerialize") != nil {
		ms["MarshalJSON"] = marshal
	}
	for _, k := range c.ancestors() {
		if k.Poly {
			ms[asMethod(k)] = &api.Func{Results: []*api.Type{api.Ptr(api.Named(k.GoName))}}
		}
	}
	cv.localMethods[c.GoName] = ms
	if c.Poly {
		im := map[string]*api.Func{}
		for k := c; k != nil; k = k.Parent {
			for _, m := range k.Methods {
				if !m.Static && !m.Private && m.Key != "__construct" {
					if _, ok := im[m.GoName]; !ok {
						im[m.GoName] = m.Sig
					}
				}
			}
		}
		// Abstract methods of implemented interfaces.
		for _, i := range c.Ifaces {
			for n, f := range cv.localMethods[i.GoName] {
				if _, ok := im[n]; !ok {
					im[n] = f
				}
			}
		}
		im[asMethod(c)] = &api.Func{Results: []*api.Type{api.Ptr(api.Named(c.GoName))}}
		if c.findMethodIfaces("jsonSerialize") != nil {
			im["MarshalJSON"] = marshal
		}
		cv.localMethods[c.IfaceName] = im
	}
}

// asMethod is the name of the method that returns the struct behind a polymorphic class value.
func asMethod(c *class) string { return "as" + c.GoName }

// chooseRecv picks the receiver name of a class's methods.
func (cv *converter) chooseRecv(c *class) {
	r := strings.ToLower(c.GoName[:1])
	clash := false
	for _, m := range c.Methods {
		walkAll(m.Body, func(n ast.Vertex) bool {
			if v := varName(n); v != "" && localName(v) == r {
				clash = true
			}
			return !clash
		})
		for _, p := range m.Params {
			if localName(p.Name) == r {
				clash = true
			}
		}
	}
	if clash || cv.isImportName(r) {
		r = "this"
	}
	c.Recv = r
}

func (cv *converter) isImportName(n string) bool {
	return n == "phpx"
}

// goFileName is the Go file a PHP file's code goes into: its path below the plugin's source
// root, in snake case ("entity/ai/FloatGoal.php" -> "entity_ai_float_goal.go").
func (cv *converter) goFileName(p string) string {
	rel := p
	if cv.srcRoot != "" && strings.HasPrefix(p, cv.srcRoot) {
		rel = strings.TrimPrefix(p, cv.srcRoot)
	} else {
		rel = strings.TrimPrefix(rel, "src/")
	}
	rel = strings.TrimSuffix(rel, path.Ext(rel))
	var parts []string
	for _, seg := range strings.Split(rel, "/") {
		if seg != "" {
			parts = append(parts, snake(seg))
		}
	}
	name := strings.Join(parts, "_")
	if strings.HasSuffix(name, "_test") {
		name += "_"
	}
	return name + ".go"
}

// snake converts a PHP file or folder name to snake case.
func snake(base string) string {
	var sb strings.Builder
	for i, r := range base {
		if r >= 'A' && r <= 'Z' {
			if i > 0 && !(base[i-1] >= 'A' && base[i-1] <= 'Z') && base[i-1] != '_' && base[i-1] != '-' {
				sb.WriteByte('_')
			}
			sb.WriteRune(r + 32)
			continue
		}
		if r == '-' || r == '.' || r == ' ' {
			sb.WriteByte('_')
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// hiddenTypes reports whether a signature uses unexported types of another package, which a
// plugin can't name (so it can't implement the method).
func hiddenTypes(sig *api.Func) bool {
	var hidden func(t *api.Type) bool
	hidden = func(t *api.Type) bool {
		if t == nil {
			return false
		}
		if t.K == api.KNamed && t.PkgPath() != "" && t.PkgPath() != api.PhpxPath {
			if n := t.ObjName(); n != "" && n[0] >= 'a' && n[0] <= 'z' {
				return true
			}
		}
		if hidden(t.Elem) || hidden(t.Key) {
			return true
		}
		for _, a := range append(append(append([]*api.Type{}, t.Args...), t.Params...), t.Results...) {
			if hidden(a) {
				return true
			}
		}
		return false
	}
	for _, p := range append(append([]*api.Type{}, sig.Params...), sig.Results...) {
		if hidden(p) {
			return true
		}
	}
	return false
}

// containsYield reports whether a function body is a generator (yields, outside nested
// functions).
func containsYield(body []ast.Vertex) bool {
	found := false
	walkAll(body, func(n ast.Vertex) bool {
		switch n.(type) {
		case *ast.ExprClosure, *ast.ExprArrowFunction, *ast.StmtClass, *ast.StmtFunction:
			return false
		case *ast.ExprYield, *ast.ExprYieldFrom:
			found = true
		}
		return !found
	})
	return found
}

// addImplicitCtor gives c a constructor that passes its arguments to its server parent's.
func (cv *converter) addImplicitCtor(c *class) {
	pkg := cv.idx.Packages[c.ExtEmbed.PkgPath()]
	if pkg == nil {
		return
	}
	n := cv.selfCtorParams(c.ExtEmbed)
	if n < 0 {
		for _, name := range cv.extCtorCands(c.ExtParentPH, c.ExtEmbed) {
			fn, ok := pkg.Funcs[name]
			if !ok || len(fn.Results) == 0 || fn.TypeParams > 0 {
				continue
			}
			if !fn.Variadic {
				n = len(fn.Params)
			}
			break
		}
	}
	if n <= 0 {
		return
	}
	{
		var params, args []string
		for i := 0; i < n; i++ {
			params = append(params, fmt.Sprintf("$phar2goArg%d = null", i))
			args = append(args, fmt.Sprintf("$phar2goArg%d", i))
		}
		src := fmt.Sprintf("<?php\nclass Phar2goImplicit{\n\tpublic function __construct(%s){\n\t\tparent::__construct(%s);\n\t}\n}\n",
			strings.Join(params, ", "), strings.Join(args, ", "))
		f, err := parsePHP(c.File.Path, []byte(src))
		if err != nil {
			return
		}
		walk(f.Root, func(n ast.Vertex) bool {
			if m, ok := n.(*ast.StmtClassMethod); ok {
				cv.addMember(c, f, m)
				return false
			}
			return true
		})
		return
	}
}
