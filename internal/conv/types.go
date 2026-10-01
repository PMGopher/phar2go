package conv

import (
	"sort"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

const phpxArray = "phpx.Array"

var (
	throwableT = &api.Type{K: api.KNamed, Name: "phpx.Throwable", Iface: true}
	exceptionT = api.Ptr(api.Named("phpx.Exception"))
	entryT     = api.Named("phpx.Entry")
	generatorT = api.Ptr(api.Named("phpx.Generator"))
)

// arrayT is a PHP array (*phpx.Array). k and v are the key and value types when known.
func arrayT(k, v *api.Type) *api.Type {
	n := &api.Type{K: api.KNamed, Name: phpxArray}
	if v != nil {
		if k == nil {
			k = api.Any
		}
		n.Args = []*api.Type{k, v}
	}
	return api.Ptr(n)
}

func isArrayT(t *api.Type) bool {
	return t != nil && t.K == api.KPointer && t.Elem.K == api.KNamed && t.Elem.Name == phpxArray
}

// arrayElem returns the key and value types of a PHP array type (any when unknown).
func arrayElem(t *api.Type) (*api.Type, *api.Type) {
	if isArrayT(t) && len(t.Elem.Args) == 2 {
		return t.Elem.Args[0], t.Elem.Args[1]
	}
	return api.Any, api.Any
}

func isAny(t *api.Type) bool { return t == nil || t.IsAny() }

// isGoCollection reports whether t is a Go slice or map.
func isGoCollection(t *api.Type) bool {
	return t != nil && (t.K == api.KSlice || t.K == api.KMap || t.K == api.KArray)
}

// typeInfo returns the declaration of a named type (of the server, phpx or the plugin).
func (cv *converter) typeInfo(t *api.Type) *api.TypeInfo {
	if t == nil || t.K != api.KNamed {
		return nil
	}
	if !strings.Contains(t.Name, ".") {
		return nil
	}
	return cv.idx.Type(t.Name)
}

// localClassOf returns the plugin class a type refers to.
func (cv *converter) localClassOf(t *api.Type) *class {
	if t == nil {
		return nil
	}
	if t.K == api.KPointer {
		t = t.Elem
	}
	if t.K != api.KNamed || strings.Contains(t.Name, ".") {
		return nil
	}
	return cv.byGoType[t.Name]
}

// classType is the Go type of values of a plugin class.
func (cv *converter) classType(c *class) *api.Type {
	switch {
	case c.Kind == kindInterface:
		return &api.Type{K: api.KNamed, Name: c.GoName, Iface: true}
	case c.Kind == kindTrait:
		return api.Any
	case c.Poly:
		return &api.Type{K: api.KNamed, Name: c.IfaceName, Iface: true}
	}
	return api.Ptr(api.Named(c.GoName))
}

// classOverrides fixes PHP classes whose port has a different shape in Go.
var classOverrides = map[string]string{
	`pocketmine\world\position`:        "pocketmine/entity.Location",
	`pocketmine\entity\location`:       "pocketmine/entity.Location",
	`pocketmine\command\commandsender`: "pocketmine/command.Sender",
	`pocketmine\plugin\plugin`:         "pocketmine/plugin.Plugin",
	`pocketmine\server`:                "pocketmine/server.Server",
	`pocketmine\player\player`:         "pocketmine/player.Player",
	`pocketmine\math\vector3`:          "pocketmine/math.Vector3",
	`pocketmine\world\world`:           "pocketmine/world.World",
	`pocketmine\item\item`:             "pocketmine/item.Item",
	`pocketmine\block\block`:           "pocketmine/block.Behavior",
	`pocketmine\entity\entity`:         "pocketmine/entity.Entity",
	`pocketmine\entity\living`:         "pocketmine/entity.Living",
	`pocketmine\event\listener`:        "pocketmine/event.Listener",
	`pocketmine\scheduler\task`:        "pocketmine/scheduler.Task",
	`pocketmine\scheduler\asynctask`:   "pocketmine/scheduler.AsyncTask",
	`pocketmine\form\form`:             "pocketmine/form.Form",
	`pocketmine\utils\config`:          "pocketmine/utils.Config",
}

// extClassType maps a PHP class of the server to its Go type.
func (cv *converter) extClassType(php string) *api.Type {
	key := strings.ToLower(strings.TrimPrefix(php, "\\"))
	if t, ok := cv.extCache[key]; ok {
		return t
	}
	var name string
	if o, ok := classOverrides[key]; ok {
		name = cv.idx.Module + "/" + o
		if cv.idx.Type(name) == nil {
			name = ""
		}
	}
	if name == "" {
		name = cv.idx.PHPClasses[key]
	}
	if name == "" && strings.HasPrefix(key, "pocketmine\\") {
		// pocketmine\foo\Bar -> pocketmine-go/pocketmine/foo.Bar
		i := strings.LastIndexByte(key, '\\')
		pkg := cv.idx.Module + "/" + strings.ReplaceAll(key[:i], "\\", "/")
		if p := cv.idx.Packages[pkg]; p != nil {
			short := php[strings.LastIndexByte(php, '\\')+1:]
			for n := range p.Types {
				if strings.EqualFold(n, short) {
					name = pkg + "." + n
				}
			}
		}
	}
	var t *api.Type
	if name != "" {
		ti := cv.idx.Type(name)
		switch {
		case ti == nil:
		case ti.Interface:
			t = &api.Type{K: api.KNamed, Name: name, Iface: true}
		case ti.Underlying.K == api.KStruct:
			t = api.Ptr(api.Named(name))
		default:
			t = api.Named(name)
		}
		if t != nil && len(ti.TypeParams) > 0 {
			args := make([]*api.Type, len(ti.TypeParams))
			for i := range args {
				args[i] = api.Any
			}
			t.Deref().Args = args
		}
	}
	cv.extCache[key] = t
	return t
}

// extPackage returns the Go package a PHP namespace of the server maps to.
func (cv *converter) extPackage(phpClass string) *api.Package {
	key := strings.ToLower(strings.TrimPrefix(phpClass, "\\"))
	if t := cv.extClassType(key); t != nil {
		return cv.idx.Packages[t.Deref().PkgPath()]
	}
	i := strings.LastIndexByte(key, '\\')
	if i < 0 {
		return nil
	}
	return cv.idx.Packages[cv.idx.Module+"/"+strings.ReplaceAll(key[:i], "\\", "/")]
}

// classRef is the Go type for a PHP class name used as a type.
func (cv *converter) classRef(fqcn string, ctx *class) *api.Type {
	key := strings.ToLower(strings.TrimPrefix(fqcn, "\\"))
	switch key {
	case "self", "static", "$this":
		if ctx != nil {
			return cv.classType(ctx)
		}
		return api.Any
	case "parent":
		if ctx != nil && ctx.Parent != nil {
			return cv.classType(ctx.Parent)
		}
		if ctx != nil && ctx.ExtParent != nil {
			return ctx.ExtParent
		}
		return api.Any
	case "generator":
		return generatorT
	case "closure", "callable", "iterator", "traversable", "iteratoraggregate", "stdclass", "object", "mixed":
		return api.Any
	case "throwable", "exception", "error":
		return throwableT
	case "jsonserializable":
		return &api.Type{K: api.KNamed, Name: "phpx.JsonSerializable", Iface: true}
	case `pocketmine\plugin\pluginbase`:
		// A PluginBase value is a plugin's main object: the plugin.Plugin interface keeps it
		// (a *plugin.PluginBase would be only the base embedded in it).
		if t := cv.extClassType(`pocketmine\plugin\Plugin`); t != nil {
			return t
		}
	}
	if c := cv.classes[key]; c != nil {
		return cv.classType(c)
	}
	if isThrowableClass(key) {
		return throwableT
	}
	if t := cv.extClassType(key); t != nil {
		return t
	}
	if !strings.Contains(key, "\\") {
		// Built-in classes of PHP that have no Go counterpart.
		return api.Any
	}
	cv.external[fqcn] = true
	return api.Any
}

// phpType converts a PHP type declaration (and, when it has none or only says "array", the
// doc-comment type) to a Go type. It returns nil when there is neither.
func (cv *converter) phpType(f *phpFile, n ast.Vertex, doc string, ctx *class) *api.Type {
	t := cv.phpTypeNode(f, n, ctx)
	if doc != "" && (t == nil || isArrayT(t) || isAny(t)) {
		if dt := cv.docTypeString(doc, ctx, f); dt != nil {
			if t == nil || isArrayT(dt) || isAny(t) && !isAny(dt) {
				if t != nil && isArrayT(t) && !isArrayT(dt) {
					return t
				}
				return dt
			}
		}
	}
	return t
}

func (cv *converter) phpTypeNode(f *phpFile, n ast.Vertex, ctx *class) *api.Type {
	switch x := n.(type) {
	case nil:
		return nil
	case *ast.Nullable:
		t := cv.phpTypeNode(f, x.Expr, ctx)
		if t == nil || t.Nilable() || isArrayT(t) {
			return t
		}
		return api.Any
	case *ast.Union:
		var types []*api.Type
		nullable := false
		for _, e := range x.Types {
			if strings.EqualFold(identValue(e), "null") {
				nullable = true
				continue
			}
			if t := cv.phpTypeNode(f, e, ctx); t != nil {
				types = append(types, t)
			}
		}
		if len(types) == 2 && (types[0].IsInt() && types[1].IsFloat() || types[0].IsFloat() && types[1].IsInt()) && !nullable {
			return api.Float
		}
		if len(types) == 1 {
			if !nullable || types[0].Nilable() {
				return types[0]
			}
		}
		return api.Any
	case *ast.Intersection:
		if len(x.Types) > 0 {
			return cv.phpTypeNode(f, x.Types[0], ctx)
		}
		return api.Any
	case *ast.Identifier, *ast.Name, *ast.NameFullyQualified, *ast.NameRelative:
		name := identValue(x)
		switch strings.ToLower(name) {
		case "int":
			return api.Int
		case "float":
			return api.Float
		case "string":
			return api.String
		case "bool", "false", "true":
			return api.Bool
		case "void", "never":
			return api.Void
		case "array":
			return arrayT(nil, nil)
		case "iterable":
			// An array or a generator.
			return api.Any
		case "mixed", "object", "callable", "null":
			return api.Any
		case "self", "static", "parent":
			return cv.classRef(strings.ToLower(name), ctx)
		}
		return cv.classRef(cv.resolveName(f, x), ctx)
	}
	return api.Any
}

// docTypeString parses a doc-comment type such as "Player[]", "array<string, int>" or "?Foo".
func (cv *converter) docTypeString(s string, ctx *class, f *phpFile) *api.Type {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "?") {
		t := cv.docTypeString(s[1:], ctx, f)
		if t == nil || t.Nilable() {
			return t
		}
		return api.Any
	}
	// Unions at the top level.
	if parts := splitTop(s, '|'); len(parts) > 1 {
		var types []*api.Type
		nullable := false
		for _, p := range parts {
			if strings.EqualFold(strings.TrimSpace(p), "null") {
				nullable = true
				continue
			}
			if t := cv.docTypeString(p, ctx, f); t != nil {
				types = append(types, t)
			}
		}
		if len(types) == 1 && (!nullable || types[0].Nilable()) {
			return types[0]
		}
		allArrays := len(types) > 0
		for _, t := range types {
			if !isArrayT(t) {
				allArrays = false
			}
		}
		if allArrays {
			return types[0]
		}
		return api.Any
	}
	if strings.HasSuffix(s, "[]") {
		v := cv.docTypeString(s[:len(s)-2], ctx, f)
		if v == nil {
			v = api.Any
		}
		return arrayT(api.Any, v)
	}
	if i := strings.IndexByte(s, '<'); i > 0 && strings.HasSuffix(s, ">") {
		base := strings.ToLower(s[:i])
		args := splitTop(s[i+1:len(s)-1], ',')
		switch base {
		case "\\generator", "generator":
			return generatorT
		case "iterable", "\\traversable", "traversable":
			return api.Any
		case "array", "list", "non-empty-array", "non-empty-list":
			var k, v *api.Type
			if len(args) == 1 {
				k, v = api.Int, cv.docTypeString(args[0], ctx, f)
			} else if len(args) >= 2 {
				k, v = cv.docTypeString(args[0], ctx, f), cv.docTypeString(args[1], ctx, f)
			}
			if v == nil {
				v = api.Any
			}
			if k == nil || !(k.IsInt() || k.IsString()) {
				k = api.Any
			}
			return arrayT(k, v)
		case "class-string":
			return api.String
		}
		return cv.docTypeString(s[:i], ctx, f)
	}
	if strings.HasPrefix(s, "array{") || strings.HasPrefix(s, "list{") {
		return arrayT(nil, nil)
	}
	if strings.HasPrefix(s, "(") || strings.HasPrefix(s, "\\Closure(") || strings.HasPrefix(s, "Closure(") || strings.HasPrefix(s, "callable(") {
		return api.Any
	}
	switch strings.ToLower(s) {
	case "int", "integer", "positive-int", "negative-int", "non-negative-int", "int-mask":
		return api.Int
	case "float", "double":
		return api.Float
	case "string", "non-empty-string", "class-string", "numeric-string", "lowercase-string", "literal-string":
		return api.String
	case "bool", "boolean", "true", "false":
		return api.Bool
	case "array", "list", "non-empty-array", "non-empty-list":
		return arrayT(nil, nil)
	case "iterable":
		return api.Any
	case "void", "never":
		return api.Void
	case "mixed", "object", "callable", "null", "resource", "scalar", "numeric", "array-key", "\\closure", "closure":
		return api.Any
	case "self", "static", "$this":
		return cv.classRef("self", ctx)
	}
	if !isNameLike(s) {
		return nil
	}
	// A class name, resolved like the file's `use` statements would.
	return cv.classRef(cv.resolveDocName(s, f, ctx), ctx)
}

func isNameLike(s string) bool {
	for _, r := range s {
		if !(r == '\\' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return s != ""
}

// resolveDocName resolves a class name written in a doc comment using the file's imports.
func (cv *converter) resolveDocName(s string, f *phpFile, ctx *class) string {
	if strings.HasPrefix(s, "\\") {
		return s[1:]
	}
	if f == nil {
		return s
	}
	first := s
	rest := ""
	if i := strings.IndexByte(s, '\\'); i >= 0 {
		first, rest = s[:i], s[i:]
	}
	if imp, ok := cv.fileUses(f)[strings.ToLower(first)]; ok {
		return imp + rest
	}
	ns := ""
	if ctx != nil {
		if i := strings.LastIndexByte(ctx.FQCN, '\\'); i >= 0 {
			ns = ctx.FQCN[:i+1]
		}
	}
	if cv.classes[strings.ToLower(ns+s)] != nil {
		return ns + s
	}
	return ns + s
}

// fileUses returns the class imports (use statements) of a file: lower-case alias -> FQCN.
func (cv *converter) fileUses(f *phpFile) map[string]string {
	if u, ok := cv.uses[f]; ok {
		return u
	}
	u := map[string]string{}
	walk(f.Root, func(n ast.Vertex) bool {
		switch x := n.(type) {
		case *ast.StmtUseList:
			if x.Type != nil && identValue(x.Type) != "" {
				return false
			}
			for _, us := range x.Uses {
				if s, ok := us.(*ast.StmtUse); ok {
					full := identValue(s.Use)
					alias := full[strings.LastIndexByte(full, '\\')+1:]
					if s.Alias != nil {
						alias = identValue(s.Alias)
					}
					u[strings.ToLower(alias)] = strings.TrimPrefix(full, "\\")
				}
			}
			return false
		case *ast.StmtGroupUseList:
			prefix := identValue(x.Prefix)
			for _, us := range x.Uses {
				if s, ok := us.(*ast.StmtUse); ok {
					full := prefix + "\\" + identValue(s.Use)
					alias := full[strings.LastIndexByte(full, '\\')+1:]
					if s.Alias != nil {
						alias = identValue(s.Alias)
					}
					u[strings.ToLower(alias)] = strings.TrimPrefix(full, "\\")
				}
			}
			return false
		case *ast.StmtClass, *ast.StmtInterface, *ast.StmtTrait, *ast.StmtFunction:
			return false
		}
		return true
	})
	cv.uses[f] = u
	return u
}

func splitTop(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<', '{', '(', '[':
			depth++
		case '>', '}', ')', ']':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// methodSet returns the methods callable on a value of type t (Go names).
func (cv *converter) methodSet(t *api.Type) map[string]*api.Func {
	if t == nil {
		return nil
	}
	switch t.K {
	case api.KInterface:
		return t.Methods
	case api.KPointer:
		if t.Elem.K == api.KNamed {
			return cv.namedMethods(t.Elem)
		}
	case api.KNamed:
		return cv.namedMethods(t)
	}
	return nil
}

func (cv *converter) namedMethods(t *api.Type) map[string]*api.Func {
	if c := cv.localClassOf(t); c != nil {
		return cv.localMethods[t.Name]
	}
	if t.Name == "error" {
		return map[string]*api.Func{"Error": {Results: []*api.Type{api.String}}}
	}
	ti := cv.typeInfo(t)
	if ti == nil {
		return nil
	}
	if len(ti.TypeParams) > 0 && len(t.Args) == len(ti.TypeParams) {
		args := map[string]*api.Type{}
		for i, n := range ti.TypeParams {
			args[n] = t.Args[i]
		}
		out := make(map[string]*api.Func, len(ti.Methods))
		for n, f := range ti.Methods {
			out[n] = api.SubstFunc(f, args)
		}
		return out
	}
	return ti.Methods
}

// isInterface reports whether t is an interface type.
func (cv *converter) isInterface(t *api.Type) bool {
	if t == nil {
		return false
	}
	if t.K == api.KInterface {
		return true
	}
	if t.K == api.KNamed {
		if t.Name == "error" {
			return true
		}
		if c := cv.localClassOf(t); c != nil {
			return c.Kind == kindInterface || c.Poly && t.Name == c.IfaceName
		}
		if ti := cv.typeInfo(t); ti != nil {
			return ti.Interface
		}
	}
	return false
}

// interfaceMethods returns the methods an interface type requires.
func (cv *converter) interfaceMethods(t *api.Type) (map[string]*api.Func, bool) {
	if t.K == api.KInterface {
		return t.Methods, false
	}
	if t.Name == "error" {
		return map[string]*api.Func{"Error": {Results: []*api.Type{api.String}}}, false
	}
	if c := cv.localClassOf(t); c != nil {
		return cv.localMethods[t.Name], false
	}
	ti := cv.typeInfo(t)
	if ti == nil {
		return nil, false
	}
	return ti.Methods, ti.Unexported
}

// implements reports whether values of type t satisfy interface type iface.
func (cv *converter) implements(t, iface *api.Type) bool {
	want, unexported := cv.interfaceMethods(iface)
	if unexported {
		// Only types that embed the package's base type can implement it.
		if !cv.embeds(t, iface) {
			return false
		}
	}
	have := cv.methodSet(t)
	for n, f := range want {
		h, ok := have[n]
		if !ok {
			return false
		}
		if !api.SameSig(h, f) {
			return false
		}
	}
	return true
}

// embeds reports whether t is (or embeds) a struct that implements iface's unexported methods,
// i.e. the "<Iface>Base" type of iface's package.
func (cv *converter) embeds(t, iface *api.Type) bool {
	if c := cv.localClassOf(t); c != nil {
		for k := c; k != nil; k = k.Parent {
			if k.ExtEmbed != nil && cv.implementsByName(k.ExtEmbed, iface) {
				return true
			}
		}
		return false
	}
	return cv.implementsByName(t, iface)
}

func (cv *converter) implementsByName(t, iface *api.Type) bool {
	ti := cv.typeInfo(t.Deref())
	if ti == nil {
		return false
	}
	pkg := iface.PkgPath()
	if t.Deref().PkgPath() != pkg {
		for _, e := range ti.Embedded {
			if cv.implementsByName(e, iface) {
				return true
			}
		}
		return false
	}
	return true
}

// assignable reports whether a value of type from can be used as a to without conversion.
func (cv *converter) assignable(from, to *api.Type) bool {
	if from == nil || to == nil {
		return false
	}
	if api.Identical(from, to) {
		return true
	}
	if from.IsNil() {
		return to.Nilable() || isArrayT(to)
	}
	if to.IsAny() {
		return !from.IsVoid()
	}
	if cv.isInterface(to) {
		return cv.implements(from, to)
	}
	return false
}

// concreteFor finds the concrete server type behind an interface that has a method the
// interface lacks (e.g. plugin.Server -> *server.Server for GetPluginManager).
func (cv *converter) concreteFor(iface *api.Type, goNames []string) (*api.Type, string) {
	want, _ := cv.interfaceMethods(iface)
	key := iface.Name + "|" + strings.Join(goNames, ",")
	if r, ok := cv.concreteCache[key]; ok {
		return r.t, r.name
	}
	type cand struct {
		t     *api.Type
		name  string
		count int
		pref  int
	}
	var cands []cand
	ifaceShort := strings.ToLower(iface.ObjName())
	for _, path := range cv.idx.PackagePaths() {
		p := cv.idx.Packages[path]
		for tn, ti := range p.Types {
			if ti.Interface || ti.Underlying == nil || ti.Underlying.K != api.KStruct {
				continue
			}
			found := ""
			for _, gn := range goNames {
				if _, ok := ti.Methods[gn]; ok {
					found = gn
					break
				}
			}
			if found == "" {
				continue
			}
			ok := true
			for n := range want {
				if _, has := ti.Methods[n]; !has {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			pref := 0
			if strings.ToLower(tn) == ifaceShort {
				pref = 2
			}
			if strings.Contains(path, "/test") || strings.HasPrefix(tn, "Fake") || strings.HasPrefix(tn, "Mock") || strings.HasPrefix(tn, "Dummy") {
				pref = -5
			}
			cands = append(cands, cand{api.Ptr(api.Named(path + "." + tn)), found, len(ti.Methods), pref})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].pref != cands[j].pref {
			return cands[i].pref > cands[j].pref
		}
		if cands[i].count != cands[j].count {
			return cands[i].count < cands[j].count
		}
		return cands[i].t.Elem.Name < cands[j].t.Elem.Name
	})
	var r concreteResult
	if len(cands) > 0 {
		r = concreteResult{cands[0].t, cands[0].name}
	}
	cv.concreteCache[key] = r
	return r.t, r.name
}

type concreteResult struct {
	t    *api.Type
	name string
}

// goMethodNames returns the Go names a PHP method of the server may have.
func goMethodNames(php string) []string {
	out := []string{pascal(php)}
	l := strings.ToLower(php)
	if strings.HasPrefix(l, "get") && len(php) > 3 {
		out = append(out, pascal(php[3:]))
	}
	switch l {
	case "__tostring", "tostring":
		out = append(out, "String")
	}
	return out
}

// findMethodName finds the Go method of methods matching a PHP method name.
func findMethodName(methods map[string]*api.Func, php string, extra ...string) string {
	for _, n := range append(extra, goMethodNames(php)...) {
		if _, ok := methods[n]; ok {
			return n
		}
	}
	norm := normName(php)
	normNoGet := strings.TrimPrefix(norm, "get")
	var best string
	for n := range methods {
		ln := normName(n)
		if ln == norm || ln == normNoGet && normNoGet != "" {
			if best == "" || n < best {
				best = n
			}
		}
	}
	return best
}
