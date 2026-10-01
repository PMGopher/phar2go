package conv

import (
	"sort"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

type classKind int

const (
	kindClass classKind = iota
	kindInterface
	kindTrait
	kindEnum
)

// class is a PHP class, interface, trait or enum of the plugin.
type class struct {
	FQCN     string
	Short    string
	Kind     classKind
	Abstract bool
	File     *phpFile
	Node     ast.Vertex
	Doc      string

	GoName string

	ParentName string   // FQCN of the parent class
	IfaceNames []string // FQCNs of implemented (or, for interfaces, extended) interfaces
	TraitNames []string

	Parent       *class    // local parent
	ExtParent    *api.Type // external parent (named type of the server), or nil
	ExtParentPH  string    // PHP name of the external parent
	ExtEmbed     *api.Type // the type embedded for ExtParent (a struct)
	Ifaces       []*class
	ExtIfaces    []*api.Type
	ExtTraits    []*api.Type // server structs embedded for `use SomeTrait` (CancellableTrait)
	isEvent      bool
	Subclasses   []*class
	Implementors []*class

	Consts      []*constDecl
	Props       []*prop
	Methods     []*method
	methodByKey map[string]*method
	propByName  map[string]*prop

	EnumCases   []*enumCase
	EnumBacking string // "", "int" or "string"

	// Exception is set when the class is a Throwable (extends Exception/Error).
	Exception bool
	// Poly is set when the class is used polymorphically: it has subclasses or is abstract. Its
	// values are then held as the interface type IfaceName.
	Poly      bool
	IfaceName string

	// Recv is the receiver name used in its methods.
	Recv string

	listener bool
	noInit   bool
	// adapters are methods generated to satisfy interfaces (Go name -> how to implement it).
	adapters []*adapter
	anonID   int
}

type constDecl struct {
	Name    string
	GoName  string
	Expr    ast.Vertex
	Class   *class
	Type    *api.Type
	isConst bool
}

type prop struct {
	Name     string
	GoName   string
	Static   bool
	Private  bool
	TypeNode ast.Vertex
	Doc      string
	Default  ast.Vertex
	Type     *api.Type
	Class    *class
	Promoted bool
	Readonly bool
	File     *phpFile
	// Base is the parent's property this one redeclares (the Go field is the parent's).
	Base *prop
}

type param struct {
	Name     string
	TypeNode ast.Vertex
	Default  ast.Vertex
	ByRef    bool
	Variadic bool
	Type     *api.Type // PHP-side type of the parameter variable
	GoType   *api.Type // type in the Go signature
	Promote  *prop
}

type method struct {
	Name       string
	Key        string // lower-case name
	GoName     string
	Static     bool
	Abstract   bool
	Private    bool
	Protected  bool
	Params     []*param
	ReturnNode ast.Vertex
	ByRefRet   bool
	Doc        string
	Body       []ast.Vertex
	HasBody    bool
	Class      *class
	Node       ast.Vertex

	// Return is the PHP-side return type (Void for void).
	Return *api.Type
	// Sig is the Go signature.
	Sig *api.Func
	// Adopted is set when the Go signature comes from a server type the method implements.
	Adopted bool
	// FromTrait is the trait the method was copied from.
	FromTrait *class
}

type enumCase struct {
	Name   string
	GoName string
	Value  ast.Vertex
}

// adapter is a method generated so that a class implements an interface method it gets from
// the server type it extends under another name, or doesn't have at all.
type adapter struct {
	GoName string
	Sig    *api.Func
	// Target is the Go method that implements it ("" when there is none).
	Target    string
	TargetSig *api.Func
	PHPName   string
}

// function is a plugin function declared outside a class.
type function struct {
	Name   string
	GoName string
	m      *method
}

func (c *class) method(name string) *method {
	return c.methodByKey[strings.ToLower(name)]
}

// findMethod looks up a method in the class and its local ancestors and traits.
func (c *class) findMethod(name string) *method {
	for k := c; k != nil; k = k.Parent {
		if m := k.method(name); m != nil {
			return m
		}
	}
	return nil
}

// findMethodIfaces also searches the implemented interfaces.
func (c *class) findMethodIfaces(name string) *method {
	if m := c.findMethod(name); m != nil {
		return m
	}
	seen := map[*class]bool{}
	var walk func(k *class) *method
	walk = func(k *class) *method {
		if k == nil || seen[k] {
			return nil
		}
		seen[k] = true
		if m := k.method(name); m != nil {
			return m
		}
		for _, i := range k.Ifaces {
			if m := walk(i); m != nil {
				return m
			}
		}
		if k.Parent != nil {
			return walk(k.Parent)
		}
		return nil
	}
	return walk(c)
}

func (c *class) findProp(name string) *prop {
	for k := c; k != nil; k = k.Parent {
		if p := k.propByName[name]; p != nil {
			return p
		}
	}
	return nil
}

func (c *class) findConst(name string) *constDecl {
	seen := map[*class]bool{}
	var walk func(k *class) *constDecl
	walk = func(k *class) *constDecl {
		if k == nil || seen[k] {
			return nil
		}
		seen[k] = true
		for _, cd := range k.Consts {
			if cd.Name == name {
				return cd
			}
		}
		if r := walk(k.Parent); r != nil {
			return r
		}
		for _, i := range k.Ifaces {
			if r := walk(i); r != nil {
				return r
			}
		}
		return nil
	}
	return walk(c)
}

// ancestors returns the class and its local ancestors, the class first.
func (c *class) ancestors() []*class {
	var out []*class
	for k := c; k != nil; k = k.Parent {
		out = append(out, k)
	}
	return out
}

// isSubclassOf reports whether c is (or extends/implements) o.
func (c *class) isSubclassOf(o *class) bool {
	seen := map[*class]bool{}
	var walk func(k *class) bool
	walk = func(k *class) bool {
		if k == nil || seen[k] {
			return false
		}
		if k == o {
			return true
		}
		seen[k] = true
		for _, i := range k.Ifaces {
			if walk(i) {
				return true
			}
		}
		return walk(k.Parent)
	}
	return walk(c)
}

// rootExt returns the external parent of the class or of its nearest ancestor that has one.
func (c *class) rootExt() (*api.Type, string) {
	for k := c; k != nil; k = k.Parent {
		if k.ExtParent != nil {
			return k.ExtParent, k.ExtParentPH
		}
	}
	return nil, ""
}

// allExtIfaces returns the external interfaces implemented by the class and its ancestors.
func (c *class) allExtIfaces() []*api.Type {
	var out []*api.Type
	seen := map[*class]bool{}
	var walk func(k *class)
	walk = func(k *class) {
		if k == nil || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, k.ExtIfaces...)
		for _, i := range k.Ifaces {
			walk(i)
		}
		walk(k.Parent)
	}
	walk(c)
	return out
}

// constructor returns the nearest __construct.
func (c *class) constructor() *method {
	return c.findMethod("__construct")
}

// collectClasses walks a file's AST and records its classes and functions.
func (cv *converter) collectClasses(f *phpFile) {
	var walk func(stmts []ast.Vertex)
	walk = func(stmts []ast.Vertex) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.StmtNamespace:
				walk(n.Stmts)
			case *ast.StmtClass:
				cv.addClass(f, n, kindClass)
			case *ast.StmtInterface:
				cv.addClass(f, n, kindInterface)
			case *ast.StmtTrait:
				cv.addClass(f, n, kindTrait)
			case *ast.StmtEnum:
				cv.addClass(f, n, kindEnum)
			case *ast.StmtFunction:
				cv.addFunction(f, n)
			case *ast.StmtExpression, *ast.StmtIf, *ast.StmtEcho:
				cv.topLevelCode = append(cv.topLevelCode, f.Path)
			}
		}
	}
	if r, ok := f.Root.(*ast.Root); ok {
		walk(r.Stmts)
	}
}

func (cv *converter) addClass(f *phpFile, n ast.Vertex, kind classKind) *class {
	c := &class{File: f, Node: n, Kind: kind, methodByKey: map[string]*method{}, propByName: map[string]*prop{}}
	var stmts []ast.Vertex
	switch x := n.(type) {
	case *ast.StmtClass:
		if x.Name != nil {
			c.FQCN = f.Names[x]
			if c.FQCN == "" {
				c.FQCN = identValue(x.Name)
			}
		} else {
			cv.anonCount++
			c.anonID = cv.anonCount
			c.FQCN = cv.anonName(f, x)
		}
		for _, m := range x.Modifiers {
			if strings.EqualFold(identValue(m), "abstract") {
				c.Abstract = true
			}
		}
		if x.Extends != nil {
			c.ParentName = cv.resolveName(f, x.Extends)
		}
		for _, i := range x.Implements {
			c.IfaceNames = append(c.IfaceNames, cv.resolveName(f, i))
		}
		stmts = x.Stmts
	case *ast.StmtInterface:
		c.FQCN = f.Names[x]
		for _, i := range x.Extends {
			c.IfaceNames = append(c.IfaceNames, cv.resolveName(f, i))
		}
		stmts = x.Stmts
	case *ast.StmtTrait:
		c.FQCN = f.Names[x]
		stmts = x.Stmts
	case *ast.StmtEnum:
		c.FQCN = f.Names[x]
		if c.FQCN == "" {
			c.FQCN = identValue(x.Name)
		}
		if x.Type != nil {
			c.EnumBacking = strings.ToLower(identValue(x.Type))
		}
		for _, i := range x.Implements {
			c.IfaceNames = append(c.IfaceNames, cv.resolveName(f, i))
		}
		stmts = x.Stmts
	}
	c.FQCN = strings.TrimPrefix(c.FQCN, "\\")
	c.Short = c.FQCN[strings.LastIndexByte(c.FQCN, '\\')+1:]
	c.Doc = docBefore(f.Src, pos(n))
	for _, s := range stmts {
		cv.addMember(c, f, s)
	}
	key := strings.ToLower(c.FQCN)
	if old, dup := cv.classes[key]; dup {
		cv.warnf("", "class %s is declared twice (%s and %s); the second one is ignored", c.FQCN, old.File.Path, f.Path)
		return old
	}
	cv.classes[key] = c
	cv.classList = append(cv.classList, c)
	return c
}

func (cv *converter) anonName(f *phpFile, n *ast.StmtClass) string {
	base := "Anonymous"
	if n.Extends != nil {
		base = identValue(n.Extends)
		if i := strings.LastIndexByte(base, '\\'); i >= 0 {
			base = base[i+1:]
		}
	}
	ns := ""
	for k, v := range f.Names {
		if _, ok := k.(*ast.StmtClass); ok && strings.Contains(v, "\\") {
			ns = v[:strings.LastIndexByte(v, '\\')+1]
			break
		}
	}
	return ns + "anon" + base + itoa(cv.anonCount)
}

func (cv *converter) addMember(c *class, f *phpFile, s ast.Vertex) {
	switch m := s.(type) {
	case *ast.StmtClassConstList:
		for _, cn := range m.Consts {
			if k, ok := cn.(*ast.StmtConstant); ok {
				c.Consts = append(c.Consts, &constDecl{Name: identValue(k.Name), Expr: k.Expr, Class: c})
			}
		}
	case *ast.StmtPropertyList:
		static, private, readonly := false, false, false
		for _, mod := range m.Modifiers {
			switch strings.ToLower(identValue(mod)) {
			case "static":
				static = true
			case "private", "protected":
				private = true
			case "readonly":
				readonly = true
			}
		}
		doc := docBefore(f.Src, pos(m))
		for _, pv := range m.Props {
			p, ok := pv.(*ast.StmtProperty)
			if !ok {
				continue
			}
			pr := &prop{Name: varName(p.Var), Static: static, Private: private, TypeNode: m.Type, Doc: doc, Default: p.Expr, Class: c, Readonly: readonly, File: f}
			c.Props = append(c.Props, pr)
			c.propByName[pr.Name] = pr
		}
	case *ast.StmtClassMethod:
		me := &method{Name: identValue(m.Name), Class: c, Node: m, ReturnNode: m.ReturnType, ByRefRet: m.AmpersandTkn != nil}
		me.Key = strings.ToLower(me.Name)
		me.Doc = docBefore(f.Src, pos(m))
		for _, mod := range m.Modifiers {
			switch strings.ToLower(identValue(mod)) {
			case "static":
				me.Static = true
			case "abstract":
				me.Abstract = true
			case "private":
				me.Private = true
			case "protected":
				me.Protected = true
			}
		}
		if c.Kind == kindInterface {
			me.Abstract = true
		}
		if body, ok := m.Stmt.(*ast.StmtStmtList); ok {
			me.Body = body.Stmts
			me.HasBody = true
		}
		for _, pv := range m.Params {
			p := pv.(*ast.Parameter)
			pa := &param{Name: varName(p.Var), TypeNode: p.Type, Default: p.DefaultValue, ByRef: p.AmpersandTkn != nil, Variadic: p.VariadicTkn != nil}
			if len(p.Modifiers) > 0 && me.Key == "__construct" {
				// Constructor promotion.
				pr := &prop{Name: pa.Name, TypeNode: p.Type, Doc: me.Doc, Class: c, Promoted: true, File: f}
				for _, mod := range p.Modifiers {
					switch strings.ToLower(identValue(mod)) {
					case "private", "protected":
						pr.Private = true
					case "readonly":
						pr.Readonly = true
					}
				}
				c.Props = append(c.Props, pr)
				c.propByName[pr.Name] = pr
				pa.Promote = pr
			}
			me.Params = append(me.Params, pa)
		}
		c.Methods = append(c.Methods, me)
		c.methodByKey[me.Key] = me
	case *ast.StmtTraitUse:
		for _, t := range m.Traits {
			c.TraitNames = append(c.TraitNames, cv.resolveName(f, t))
		}
		if len(m.Adaptations) > 0 {
			cv.warnf(f.Path, "trait adaptations (insteadof/as) in %s are ignored", c.FQCN)
		}
	case *ast.EnumCase:
		c.EnumCases = append(c.EnumCases, &enumCase{Name: identValue(m.Name), Value: m.Expr})
	}
}

func (cv *converter) addFunction(f *phpFile, n *ast.StmtFunction) {
	fn := &function{Name: identValue(n.Name)}
	m := &method{Name: fn.Name, Key: strings.ToLower(fn.Name), Static: true, ReturnNode: n.ReturnType, Node: n, Body: n.Stmts, HasBody: true}
	m.Doc = docBefore(f.Src, pos(n))
	for _, pv := range n.Params {
		p := pv.(*ast.Parameter)
		m.Params = append(m.Params, &param{Name: varName(p.Var), TypeNode: p.Type, Default: p.DefaultValue, ByRef: p.AmpersandTkn != nil, Variadic: p.VariadicTkn != nil})
	}
	fn.m = m
	cv.funcs[strings.ToLower(fn.Name)] = fn
	cv.funcFile[fn] = f
}

// resolveName returns the fully qualified name of a class name node.
func (cv *converter) resolveName(f *phpFile, n ast.Vertex) string {
	if s, ok := f.Names[n]; ok {
		return strings.TrimPrefix(s, "\\")
	}
	return strings.TrimPrefix(identValue(n), "\\")
}

// mergeTraits copies the members of used traits into the classes.
func (cv *converter) mergeTraits() {
	for _, c := range cv.classList {
		seen := map[*class]bool{}
		var use func(names []string)
		use = func(names []string) {
			for _, tn := range names {
				t := cv.classes[strings.ToLower(tn)]
				if t == nil || t.Kind != kindTrait {
					if t == nil {
						if et := cv.extClassType(tn); et != nil && et.K == api.KPointer {
							c.ExtTraits = append(c.ExtTraits, et.Elem)
							continue
						}
						cv.external[tn] = true
						cv.warnf(c.File.Path, "%s uses trait %s, which isn't part of the plugin; its methods are missing", c.FQCN, tn)
					}
					continue
				}
				if seen[t] {
					continue
				}
				seen[t] = true
				use(t.TraitNames)
				for _, m := range t.Methods {
					if c.method(m.Name) != nil {
						continue
					}
					cp := *m
					cp.Class = c
					cp.FromTrait = t
					cp.Params = cloneParams(m.Params, c)
					c.Methods = append(c.Methods, &cp)
					c.methodByKey[cp.Key] = &cp
				}
				for _, p := range t.Props {
					if c.propByName[p.Name] != nil {
						continue
					}
					cp := *p
					cp.Class = c
					c.Props = append(c.Props, &cp)
					c.propByName[cp.Name] = &cp
				}
				for _, k := range t.Consts {
					cp := *k
					cp.Class = c
					c.Consts = append(c.Consts, &cp)
				}
			}
		}
		use(c.TraitNames)
	}
}

func cloneParams(ps []*param, c *class) []*param {
	out := make([]*param, len(ps))
	for i, p := range ps {
		cp := *p
		if p.Promote != nil {
			pr := c.propByName[p.Promote.Name]
			cp.Promote = pr
		}
		out[i] = &cp
	}
	return out
}

// linkClasses resolves parents and interfaces, local or external.
func (cv *converter) linkClasses() {
	for _, c := range cv.classList {
		if c.ParentName != "" {
			if p := cv.classes[strings.ToLower(c.ParentName)]; p != nil {
				c.Parent = p
				p.Subclasses = append(p.Subclasses, c)
			} else {
				c.ExtParentPH = c.ParentName
				if strings.EqualFold(c.ParentName, `pocketmine\event\Event`) {
					c.isEvent = true
				} else if isThrowableClass(c.ParentName) {
					c.Exception = true
				} else if t := cv.extClassType(c.ParentName); t != nil {
					c.ExtParent = t
				} else {
					cv.external[c.ParentName] = true
					cv.warnf(c.File.Path, "%s extends %s, which pocketmine-go doesn't have; the inherited members are missing", c.FQCN, c.ParentName)
				}
			}
		}
		for _, in := range c.IfaceNames {
			if i := cv.classes[strings.ToLower(in)]; i != nil {
				c.Ifaces = append(c.Ifaces, i)
				i.Implementors = append(i.Implementors, c)
			} else if t := cv.extClassType(in); t != nil {
				c.ExtIfaces = append(c.ExtIfaces, t)
				if strings.EqualFold(in, `pocketmine\event\Listener`) {
					c.listener = true
				}
			} else if strings.EqualFold(in, "JsonSerializable") || strings.EqualFold(in, "Stringable") || strings.EqualFold(in, "Countable") || strings.EqualFold(in, "IteratorAggregate") || strings.EqualFold(in, "ArrayAccess") || strings.EqualFold(in, "Throwable") {
				// Built-in interfaces handled where they matter.
			} else if !strings.EqualFold(in, `pocketmine\event\Listener`) {
				cv.external[in] = true
			} else {
				c.listener = true
			}
		}
	}
	// Exceptions and listeners are inherited.
	for _, c := range cv.classList {
		for k := c; k != nil; k = k.Parent {
			if k.Exception {
				c.Exception = true
			}
			if k.listener {
				c.listener = true
			}
			if k.isEvent || k.ExtParent != nil && strings.Contains(k.ExtParent.Deref().Name, "/pocketmine/event/") {
				c.isEvent = true
			}
		}
	}
	// Sort so parents come before children.
	sort.SliceStable(cv.classList, func(i, j int) bool {
		return depth(cv.classList[i]) < depth(cv.classList[j])
	})
}

func depth(c *class) int {
	d := 0
	seen := map[*class]bool{}
	for k := c.Parent; k != nil && !seen[k]; k = k.Parent {
		seen[k] = true
		d++
	}
	if c.Kind == kindInterface {
		return -1
	}
	return d
}

var throwableClasses = map[string]bool{
	"exception": true, "error": true, "throwable": true, "runtimeexception": true, "logicexception": true,
	"invalidargumentexception": true, "domainexception": true, "lengthexception": true, "outofrangeexception": true,
	"outofboundsexception": true, "overflowexception": true, "underflowexception": true, "rangeexception": true,
	"unexpectedvalueexception": true, "badfunctioncallexception": true, "badmethodcallexception": true,
	"typeerror": true, "valueerror": true, "arithmeticerror": true, "divisionbyzeroerror": true,
	"errorexception": true, "jsonexception": true, "assertionerror": true, "argumentcounterror": true,
}

func isThrowableClass(name string) bool {
	n := strings.ToLower(strings.TrimPrefix(name, "\\"))
	if throwableClasses[n] {
		return true
	}
	if strings.HasPrefix(n, "pocketmine\\") && (strings.HasSuffix(n, "exception") || strings.HasSuffix(n, "error")) {
		return true
	}
	return false
}
