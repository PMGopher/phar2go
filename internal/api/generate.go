package api

import (
	"fmt"
	"go/ast"
	"go/types"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	portOfType   = regexp.MustCompile(`(?:port of (?:the )?|(?:is|mirrors|mirroring) )(pocketmine\\[A-Za-z0-9_\\]+)`)
	portOfMember = regexp.MustCompile(`^(?:[A-Za-z0-9_]+) (?:is|returns|implements) (?:a port of |the port of |)?\(?([A-Za-z0-9_\\]+)::([A-Za-z0-9_]+)`)
)

// PhpxPath is the package path the phpx runtime has in the index; converted plugins import it
// from their own module.
const PhpxPath = "phpx"

// Generate builds the index from a pocketmine-go checkout in dir, and the phpx runtime package
// in phpxDir.
func Generate(dir, phpxDir string) (*Index, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule | packages.NeedImports | packages.NeedDeps,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, "./pocketmine/...")
	if err != nil {
		return nil, err
	}
	idx := &Index{
		Packages:   map[string]*Package{},
		PHPClasses: map[string]string{},
		PHPMembers: map[string]string{},
	}
	if out, err := exec.Command("git", "-C", dir, "describe", "--always", "--dirty").Output(); err == nil {
		idx.Version = strings.TrimSpace(string(out))
	}
	var errs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
		if p.Module != nil && idx.Module == "" {
			idx.Module = p.Module.Path
		}
		if strings.HasSuffix(p.Name, "_test") || p.Types == nil {
			continue
		}
		genPackage(idx, p)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("loading pocketmine-go failed:\n%s", strings.Join(errs, "\n"))
	}
	// Packages of other modules that the API exposes (uuid.UUID, ...).
	all := map[string]*packages.Package{}
	packages.Visit(pkgs, nil, func(p *packages.Package) { all[p.PkgPath] = p })
	for round := 0; round < 3; round++ {
		for path := range referencedPackages(idx) {
			if _, done := idx.Packages[path]; done {
				continue
			}
			if p, ok := all[path]; ok && p.Types != nil {
				genPackage(idx, p)
			}
		}
	}
	rt, err := packages.Load(&packages.Config{Mode: cfg.Mode, Dir: phpxDir}, ".")
	if err != nil {
		return nil, err
	}
	if len(rt) != 1 || len(rt[0].Errors) > 0 {
		return nil, fmt.Errorf("loading the phpx runtime failed: %v", rt[0].Errors)
	}
	genPackage(idx, rt[0])
	p := idx.Packages[rt[0].PkgPath]
	delete(idx.Packages, rt[0].PkgPath)
	p.Path = PhpxPath
	idx.Packages[PhpxPath] = p
	// Types of the runtime refer to it by its real path.
	renamePkg(idx, rt[0].PkgPath, PhpxPath)
	return idx, nil
}

func genPackage(idx *Index, p *packages.Package) {
	pkg := &Package{
		Path: p.PkgPath, Name: p.Name,
		Types: map[string]*TypeInfo{}, Funcs: map[string]*Func{}, Consts: map[string]*Const{}, Vars: map[string]*Type{},
	}
	idx.Packages[p.PkgPath] = pkg
	scope := p.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			// Unexported interfaces appear in exported signatures; a plugin can still satisfy
			// them with a literal interface type.
			if tn, ok := obj.(*types.TypeName); !ok || tn.IsAlias() || !types.IsInterface(tn.Type()) {
				continue
			}
		}
		switch o := obj.(type) {
		case *types.TypeName:
			if o.IsAlias() {
				// Aliases resolve to their target.
				if n, ok := o.Type().(*types.Named); ok && n.Obj().Pkg() != nil {
					if ti := typeInfo(n); ti != nil {
						pkg.Types[name] = ti
					}
				}
				continue
			}
			named, ok := o.Type().(*types.Named)
			if !ok {
				continue
			}
			pkg.Types[name] = typeInfo(named)
		case *types.Func:
			pkg.Funcs[name] = convSig(o.Type().(*types.Signature))
		case *types.Const:
			c := &Const{Type: convType(o.Type())}
			if o.Val() != nil {
				c.Value = o.Val().ExactString()
			}
			pkg.Consts[name] = c
		case *types.Var:
			pkg.Vars[name] = convType(o.Type())
		}
	}

	// PHP names from doc comments.
	shortClasses := map[string]string{} // short PHP class name -> Go type
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				doc := ts.Doc
				if doc == nil && len(gd.Specs) == 1 {
					doc = gd.Doc
				}
				if doc == nil {
					continue
				}
				m := portOfType.FindStringSubmatch(doc.Text())
				if m == nil {
					continue
				}
				php := strings.TrimRight(m[1], `\.:`)
				goName := p.PkgPath + "." + ts.Name.Name
				if ti := pkg.Types[ts.Name.Name]; ti != nil && ti.PHP == "" {
					ti.PHP = php
				}
				key := strings.ToLower(php)
				if old, dup := idx.PHPClasses[key]; !dup || classScore(php, goName, ts) > classScore(php, old, nil) {
					idx.PHPClasses[key] = goName
				}
				shortClasses[shortName(php)] = php
			}
		}
	}
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Doc == nil || !fd.Name.IsExported() {
				continue
			}
			m := portOfMember.FindStringSubmatch(fd.Doc.Text())
			if m == nil {
				continue
			}
			class := m[1]
			if !strings.Contains(class, `\`) {
				full, ok := shortClasses[class]
				if !ok {
					// The receiver's PHP class.
					if fd.Recv != nil {
						if ti := pkg.Types[recvName(fd)]; ti != nil && ti.PHP != "" && shortName(ti.PHP) == class {
							full = ti.PHP
						}
					}
					if full == "" {
						continue
					}
				}
				class = full
			}
			target := p.PkgPath + "." + fd.Name.Name
			if fd.Recv != nil {
				target = p.PkgPath + "." + recvName(fd) + "." + fd.Name.Name
			}
			key := strings.ToLower(class + "::" + m[2])
			if _, dup := idx.PHPMembers[key]; !dup {
				idx.PHPMembers[key] = target
			}
		}
	}
}

// classScore ranks Go types that claim to port the same PHP class: the one in the package
// matching the PHP namespace wins, and type aliases lose.
func classScore(php, goName string, ts *ast.TypeSpec) int {
	score := 0
	pkg := goName[:strings.LastIndexByte(goName, '.')]
	ns := strings.ToLower(strings.ReplaceAll(php[:max(strings.LastIndexByte(php, '\\'), 0)], "\\", "/"))
	if strings.HasSuffix(strings.ToLower(pkg), ns) {
		score += 2
	}
	if strings.EqualFold(goName[strings.LastIndexByte(goName, '.')+1:], shortName(php)) {
		score++
	}
	if ts != nil && ts.Assign.IsValid() {
		score -= 3
	}
	return score
}

func shortName(php string) string {
	if i := strings.LastIndexByte(php, '\\'); i >= 0 {
		return php[i+1:]
	}
	return php
}

func recvName(fd *ast.FuncDecl) string {
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch x := t.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func typeInfo(named *types.Named) *TypeInfo {
	ti := &TypeInfo{Underlying: convType(named.Underlying())}
	if tp := named.TypeParams(); tp != nil {
		for i := 0; i < tp.Len(); i++ {
			ti.TypeParams = append(ti.TypeParams, tp.At(i).Obj().Name())
		}
	}
	ti.Methods = map[string]*Func{}
	if iface, ok := named.Underlying().(*types.Interface); ok {
		ti.Interface = true
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			if !m.Exported() {
				ti.Unexported = true
				continue
			}
			ti.Methods[m.Name()] = convSig(m.Type().(*types.Signature))
		}
		return ti
	}
	ms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i).Obj()
		if !m.Exported() {
			continue
		}
		ti.Methods[m.Name()] = convSig(m.Type().(*types.Signature))
	}
	vs := types.NewMethodSet(named)
	for i := 0; i < vs.Len(); i++ {
		if m := vs.At(i).Obj(); m.Exported() {
			ti.ValueMethods = append(ti.ValueMethods, m.Name())
		}
	}
	sort.Strings(ti.ValueMethods)
	if st, ok := named.Underlying().(*types.Struct); ok {
		ti.Fields = map[string]*Type{}
		collectFields(st, ti.Fields, 0)
		for i := 0; i < st.NumFields(); i++ {
			if f := st.Field(i); f.Embedded() {
				ti.Embedded = append(ti.Embedded, convType(f.Type()))
			}
		}
	}
	return ti
}

func collectFields(st *types.Struct, out map[string]*Type, depth int) {
	if depth > 4 {
		return
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Exported() {
			if _, ok := out[f.Name()]; !ok {
				out[f.Name()] = convType(f.Type())
			}
		}
		if f.Embedded() {
			t := f.Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if s, ok := t.Underlying().(*types.Struct); ok {
				collectFields(s, out, depth+1)
			}
		}
	}
}

func convSig(s *types.Signature) *Func {
	f := &Func{Variadic: s.Variadic()}
	for i := 0; i < s.Params().Len(); i++ {
		v := s.Params().At(i)
		f.Params = append(f.Params, convType(v.Type()))
		f.ParamNames = append(f.ParamNames, v.Name())
	}
	for i := 0; i < s.Results().Len(); i++ {
		f.Results = append(f.Results, convType(s.Results().At(i).Type()))
	}
	if s.TypeParams() != nil {
		f.TypeParams = s.TypeParams().Len()
	}
	return f
}

func convType(t types.Type) *Type {
	switch x := t.(type) {
	case *types.Basic:
		if x.Kind() == types.UnsafePointer {
			return &Type{K: KBasic, Name: "unsafe.Pointer"}
		}
		switch x.Kind() {
		case types.UntypedString:
			return &Type{K: KBasic, Name: "string"}
		case types.UntypedInt:
			return &Type{K: KBasic, Name: "int"}
		case types.UntypedFloat:
			return &Type{K: KBasic, Name: "float64"}
		case types.UntypedBool:
			return &Type{K: KBasic, Name: "bool"}
		case types.UntypedRune:
			return &Type{K: KBasic, Name: "int32"}
		}
		return &Type{K: KBasic, Name: x.Name()}
	case *types.Alias:
		return convType(types.Unalias(x))
	case *types.Named:
		name := x.Obj().Name()
		if x.Obj().Pkg() != nil {
			name = x.Obj().Pkg().Path() + "." + name
		}
		out := &Type{K: KNamed, Name: name}
		if _, ok := x.Underlying().(*types.Interface); ok {
			out.Iface = true
		}
		if ta := x.TypeArgs(); ta != nil {
			for i := 0; i < ta.Len(); i++ {
				out.Args = append(out.Args, convType(ta.At(i)))
			}
		}
		return out
	case *types.Pointer:
		return &Type{K: KPointer, Elem: convType(x.Elem())}
	case *types.Slice:
		return &Type{K: KSlice, Elem: convType(x.Elem())}
	case *types.Array:
		return &Type{K: KArray, Elem: convType(x.Elem()), Len: x.Len()}
	case *types.Map:
		return &Type{K: KMap, Key: convType(x.Key()), Elem: convType(x.Elem())}
	case *types.Chan:
		return &Type{K: KChan, Elem: convType(x.Elem())}
	case *types.Signature:
		f := convSig(x)
		return &Type{K: KFunc, Params: f.Params, Results: f.Results, Variadic: f.Variadic}
	case *types.Interface:
		out := &Type{K: KInterface}
		if x.NumMethods() > 0 {
			out.Methods = map[string]*Func{}
			for i := 0; i < x.NumMethods(); i++ {
				m := x.Method(i)
				out.Methods[m.Name()] = convSig(m.Type().(*types.Signature))
			}
		}
		if !x.IsMethodSet() {
			// A constraint (e.g. comparable); treat as any.
			return &Type{K: KInterface}
		}
		return out
	case *types.Struct:
		return &Type{K: KStruct}
	case *types.TypeParam:
		return &Type{K: KTypeParam, Name: x.Obj().Name()}
	case *types.Tuple:
		out := &Type{K: KTuple}
		for i := 0; i < x.Len(); i++ {
			out.Results = append(out.Results, convType(x.At(i).Type()))
		}
		return out
	}
	return &Type{K: KInvalid}
}

// renamePkg rewrites references to package from as to.
func renamePkg(idx *Index, from, to string) {
	var fix func(t *Type)
	fix = func(t *Type) {
		if t == nil {
			return
		}
		if t.K == KNamed && strings.HasPrefix(t.Name, from+".") {
			t.Name = to + t.Name[len(from):]
		}
		for _, a := range t.Args {
			fix(a)
		}
		fix(t.Elem)
		fix(t.Key)
		for _, p := range t.Params {
			fix(p)
		}
		for _, r := range t.Results {
			fix(r)
		}
		for _, m := range t.Methods {
			fixFunc(m, fix)
		}
	}
	p := idx.Packages[to]
	for _, ti := range p.Types {
		fix(ti.Underlying)
		for _, m := range ti.Methods {
			fixFunc(m, fix)
		}
		for _, f := range ti.Fields {
			fix(f)
		}
		for _, e := range ti.Embedded {
			fix(e)
		}
	}
	for _, f := range p.Funcs {
		fixFunc(f, fix)
	}
	for _, c := range p.Consts {
		fix(c.Type)
	}
	for _, v := range p.Vars {
		fix(v)
	}
}

func fixFunc(f *Func, fix func(*Type)) {
	for _, p := range f.Params {
		fix(p)
	}
	for _, r := range f.Results {
		fix(r)
	}
}

// referencedPackages returns the packages named types of the index come from.
func referencedPackages(idx *Index) map[string]bool {
	out := map[string]bool{}
	var visit func(t *Type)
	visit = func(t *Type) {
		if t == nil {
			return
		}
		if t.K == KNamed {
			if p := t.PkgPath(); p != "" {
				out[p] = true
			}
		}
		for _, a := range t.Args {
			visit(a)
		}
		visit(t.Elem)
		visit(t.Key)
		for _, p := range t.Params {
			visit(p)
		}
		for _, r := range t.Results {
			visit(r)
		}
	}
	visitFunc := func(f *Func) {
		for _, p := range f.Params {
			visit(p)
		}
		for _, r := range f.Results {
			visit(r)
		}
	}
	for _, p := range idx.Packages {
		for _, ti := range p.Types {
			visit(ti.Underlying)
			for _, m := range ti.Methods {
				visitFunc(m)
			}
			for _, f := range ti.Fields {
				visit(f)
			}
		}
		for _, f := range p.Funcs {
			visitFunc(f)
		}
	}
	return out
}
