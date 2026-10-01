package conv

import (
	"fmt"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// targetType is the type of an assignment target, or nil when it isn't typed.
func (f *fctx) targetType(n ast.Vertex) *api.Type {
	switch x := n.(type) {
	case *ast.ExprVariable:
		if name := varName(x); name != "" && name != "this" {
			if v := f.lookupVar(name); v != nil {
				return v.t
			}
		}
	case *ast.ExprPropertyFetch, *ast.ExprStaticPropertyFetch:
		saved := f.dry
		f.dry = true
		v := f.expr(n, nil)
		f.dry = saved
		if v.lvalue {
			return v.t
		}
	case *ast.ExprArrayDimFetch:
		saved := f.dry
		f.dry = true
		base := f.expr(x.Var, nil)
		f.dry = saved
		switch {
		case isArrayT(base.t):
			_, vt := arrayElem(base.t)
			if isAny(vt) {
				return nil
			}
			return vt
		case base.t != nil && (base.t.K == api.KMap || base.t.K == api.KSlice):
			return base.t.Elem
		}
	}
	return nil
}

// assignStmt converts `target = rhs` as a statement.
func (f *fctx) assignStmt(target, rhs ast.Vertex) string {
	want := f.targetType(target)
	v := f.expr(rhs, want)
	return f.assignValue(target, v)
}

// assignValue assigns an already converted value to a PHP target.
func (f *fctx) assignValue(target ast.Vertex, v value) string {
	if v.t != nil && v.t.IsVoid() {
		// Assigning the result of a function without one: PHP stores null.
		code := v.code
		return code + "\n" + f.assignValue(target, prim("nil", api.UntypedNil))
	}
	switch x := target.(type) {
	case *ast.ExprVariable:
		name := varName(x)
		if name == "" || name == "this" {
			return f.todo(target, "assignment to $this or a variable variable")
		}
		l := f.lookupVar(name)
		if l == nil {
			l = f.addVar(name)
		}
		t := l.t
		if t == nil {
			t = api.Any
		}
		lhs := l.goName
		if l.ptr {
			lhs = "*" + lhs
		}
		return lhs + " = " + f.storeCode(v, t)
	case *ast.ExprPropertyFetch:
		return f.assignProp(x.Var, x.Prop, v)
	case *ast.ExprStaticPropertyFetch:
		cur := f.expr(x, nil)
		if !cur.lvalue {
			return cur.code
		}
		return cur.code + " = " + f.storeCode(v, cur.t)
	case *ast.ExprArrayDimFetch:
		return f.assignDim(x, v)
	case *ast.ExprList:
		return f.assignDestructure(x.Items, v)
	case *ast.ExprArray:
		return f.assignDestructure(x.Items, v)
	}
	return f.todo(target, "unsupported assignment target %T", target)
}

// storeCode converts a value for storing in a slot of type t (arrays are copied).
func (f *fctx) storeCode(v value, t *api.Type) string {
	if isArrayT(t) && isArrayT(v.t) && v.lvalue {
		return paren(v, 7) + ".Clone()"
	}
	if isAny(t) {
		return f.arrayElemCode(v)
	}
	return f.coerce(v, t)
}

func (f *fctx) assignProp(objN, propN ast.Vertex, v value) string {
	name := identValue(propN)
	cur := f.propFetch(objN, propN, false)
	if name != "" && cur.lvalue && !strings.Contains(cur.code, f.pkgRef(api.PhpxPath)+".Prop(") {
		return cur.code + " = " + f.storeCode(v, cur.t)
	}
	obj := f.expr(objN, nil)
	if name == "" {
		pn := f.expr(propN, api.String)
		return f.phpx("SetProp") + "(" + obj.code + ", " + f.coerce(pn, api.String) + ", " + f.arrayElemCode(v) + ")"
	}
	return f.phpx("SetProp") + "(" + obj.code + ", " + quote(name) + ", " + f.arrayElemCode(v) + ")"
}

// writeContainer is the array a nested $a[x][y] = ... writes into.
func (f *fctx) writeContainer(n ast.Vertex) value {
	if d, ok := n.(*ast.ExprArrayDimFetch); ok {
		base := f.writeContainer(d.Var)
		if isArrayT(base.t) {
			if d.Dim == nil {
				tmp := "(" + f.phpx("NewArray") + "())"
				return value{code: paren(base, 7) + ".Append(" + tmp + ").(*" + f.phpx("Array") + ")", t: arrayT(nil, nil), prec: 7}
			}
			k := f.expr(d.Dim, nil)
			return value{code: paren(base, 7) + ".Sub(" + k.code + ")", t: arrayT(nil, nil), prec: 7}
		}
		if base.t != nil && base.t.K == api.KMap && d.Dim != nil {
			k := f.expr(d.Dim, base.t.Key)
			return value{code: paren(base, 7) + "[" + f.coerce(k, base.t.Key) + "]", t: base.t.Elem, prec: 7}
		}
		k := "nil"
		if d.Dim != nil {
			k = f.expr(d.Dim, nil).code
		}
		return value{code: f.phpx("Index") + "(" + base.code + ", " + k + ")", t: api.Any, prec: 7}
	}
	return f.expr(n, nil)
}

func (f *fctx) assignDim(x *ast.ExprArrayDimFetch, v value) string {
	c := f.writeContainer(x.Var)
	t := c.t
	switch {
	case isArrayT(t):
		_, vt := arrayElem(t)
		val := f.arrayElemCode(v)
		if !isAny(vt) {
			val = f.coerce(v, vt)
		}
		if x.Dim == nil {
			return paren(c, 7) + ".Append(" + val + ")"
		}
		return paren(c, 7) + ".Set(" + f.expr(x.Dim, nil).code + ", " + val + ")"
	case t != nil && t.K == api.KMap && x.Dim != nil:
		k := f.expr(x.Dim, t.Key)
		return paren(c, 7) + "[" + f.coerce(k, t.Key) + "] = " + f.coerce(v, t.Elem)
	case t != nil && t.K == api.KSlice && c.lvalue:
		if x.Dim == nil {
			return c.code + " = append(" + c.code + ", " + f.coerce(v, t.Elem) + ")"
		}
		return paren(c, 7) + "[" + f.coerce(f.expr(x.Dim, api.Int), api.Int) + "] = " + f.coerce(v, t.Elem)
	}
	k := "nil"
	if x.Dim != nil {
		k = f.expr(x.Dim, nil).code
	}
	return f.phpx("SetIndex") + "(" + c.code + ", " + k + ", " + f.arrayElemCode(v) + ")"
}

// assignDestructure converts [$a, $b] = $value and ['k' => $a] = $value.
func (f *fctx) assignDestructure(items []ast.Vertex, v value) string {
	tmp := f.newTmp("list")
	lines := []string{tmp + " := " + f.phpx("ToArray") + "(" + v.code + ")"}
	i := 0
	for _, it := range items {
		ai, ok := it.(*ast.ExprArrayItem)
		if !ok || ai.Val == nil {
			i++
			continue
		}
		key := itoa(i)
		if ai.Key != nil {
			key = f.expr(ai.Key, nil).code
		} else {
			i++
		}
		lines = append(lines, f.assignValue(ai.Val, value{code: tmp + ".Get(" + key + ")", t: api.Any, prec: 7}))
	}
	return strings.Join(lines, "\n")
}

// assignExpr is an assignment used as an expression.
func (f *fctx) assignExpr(target, rhs ast.Vertex, n ast.Vertex) value {
	want := f.targetType(target)
	v := f.expr(rhs, want)
	switch x := target.(type) {
	case *ast.ExprVariable:
		name := varName(x)
		if l := f.lookupVar(name); l != nil && name != "this" {
			t := l.t
			if t == nil {
				t = api.Any
			}
			ref := "&" + l.goName
			if l.ptr {
				ref = l.goName
			}
			return callv(f.phpx("Set")+"["+f.typeStr(t)+"]("+ref+", "+f.storeCode(v, t)+")", t)
		}
	case *ast.ExprPropertyFetch:
		cur := f.propFetch(x.Var, x.Prop, false)
		if cur.lvalue && !strings.Contains(cur.code, ".Prop(") {
			return callv(f.phpx("Set")+"["+f.typeStr(cur.t)+"](&"+cur.code+", "+f.storeCode(v, cur.t)+")", cur.t)
		}
	case *ast.ExprArrayDimFetch:
		c := f.writeContainer(x.Var)
		if isArrayT(c.t) {
			if x.Dim == nil {
				return callv(paren(c, 7)+".Append("+f.arrayElemCode(v)+")", api.Any)
			}
			return callv(paren(c, 7)+".Set("+f.expr(x.Dim, nil).code+", "+f.arrayElemCode(v)+")", api.Any)
		}
	}
	// Fall back to a function literal.
	t := v.t
	if t == nil || t.IsVoid() || t.IsNil() {
		t = api.Any
	}
	tmp := f.newTmp("v")
	return callv(fmt.Sprintf("func() %s { %s := %s; %s; return %s }()", f.typeStr(t), tmp, f.coerce(v, t),
		f.assignValue(target, value{code: tmp, t: t, prec: 7}), tmp), t)
}

// binaryFor builds the binary expression of a compound assignment ($a .= $b -> $a . $b).
func binaryFor(n ast.Vertex) (target ast.Vertex, bin ast.Vertex, coalesce bool) {
	switch x := n.(type) {
	case *ast.ExprAssignConcat:
		return x.Var, &ast.ExprBinaryConcat{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignPlus:
		return x.Var, &ast.ExprBinaryPlus{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignMinus:
		return x.Var, &ast.ExprBinaryMinus{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignMul:
		return x.Var, &ast.ExprBinaryMul{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignDiv:
		return x.Var, &ast.ExprBinaryDiv{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignMod:
		return x.Var, &ast.ExprBinaryMod{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignPow:
		return x.Var, &ast.ExprBinaryPow{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignBitwiseAnd:
		return x.Var, &ast.ExprBinaryBitwiseAnd{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignBitwiseOr:
		return x.Var, &ast.ExprBinaryBitwiseOr{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignBitwiseXor:
		return x.Var, &ast.ExprBinaryBitwiseXor{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignShiftLeft:
		return x.Var, &ast.ExprBinaryShiftLeft{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignShiftRight:
		return x.Var, &ast.ExprBinaryShiftRight{Left: x.Var, Right: x.Expr}, false
	case *ast.ExprAssignCoalesce:
		return x.Var, x.Expr, true
	}
	return nil, nil, false
}

// compoundStmt converts $a op= $b as a statement.
func (f *fctx) compoundStmt(n ast.Vertex) string {
	target, bin, coalesce := binaryFor(n)
	if coalesce {
		cond := f.isset([]ast.Vertex{target})
		return "if !(" + cond.code + ") {\n" + f.assignStmt(target, bin) + "\n}"
	}
	// Short forms for typed variables and fields.
	tt := f.targetType(target)
	if tt != nil && !isAny(tt) {
		if _, isDim := target.(*ast.ExprArrayDimFetch); !isDim {
			lhs := f.expr(target, nil)
			switch x := n.(type) {
			case *ast.ExprAssignConcat:
				if tt.IsString() {
					return lhs.code + " += " + f.str(f.expr(x.Expr, api.String))
				}
			case *ast.ExprAssignPlus, *ast.ExprAssignMinus, *ast.ExprAssignMul:
				var r ast.Vertex
				op := map[string]string{"*ast.ExprAssignPlus": "+=", "*ast.ExprAssignMinus": "-=", "*ast.ExprAssignMul": "*="}[fmt.Sprintf("%T", n)]
				switch y := x.(type) {
				case *ast.ExprAssignPlus:
					r = y.Expr
				case *ast.ExprAssignMinus:
					r = y.Expr
				case *ast.ExprAssignMul:
					r = y.Expr
				}
				rv := f.expr(r, tt)
				if tt.IsNumber() && (rv.t.IsNumber() || rv.konst) {
					return lhs.code + " " + op + " " + f.coerce(rv, tt)
				}
			}
		}
	}
	v := f.expr(bin, tt)
	return f.assignValue(target, v)
}

func (f *fctx) compoundAssignExpr(n ast.Vertex) value {
	target, bin, coalesce := binaryFor(n)
	if coalesce {
		bin = &ast.ExprBinaryCoalesce{Left: target, Right: bin}
	}
	return f.assignExpr(target, bin, n)
}

// incDecStmt converts $x++ / --$x as a statement.
func (f *fctx) incDecStmt(target ast.Vertex, op string) string {
	tt := f.targetType(target)
	if tt != nil && tt.IsNumber() {
		if _, isDim := target.(*ast.ExprArrayDimFetch); !isDim {
			lhs := f.expr(target, nil)
			return paren(lhs, 7) + op
		}
	}
	fn := "Inc"
	if op == "--" {
		fn = "Dec"
	}
	cur := f.expr(target, nil)
	return f.assignValue(target, callv(f.phpx(fn)+"("+cur.code+")", api.Any))
}

func (f *fctx) incDec(target ast.Vertex, fn, op string, n ast.Vertex) value {
	tt := f.targetType(target)
	if tt != nil && tt.IsNumber() {
		lhs := f.expr(target, nil)
		if _, isDim := target.(*ast.ExprArrayDimFetch); !isDim && lhs.lvalue {
			ref := "&" + lhs.code
			if strings.HasPrefix(lhs.code, "*") {
				ref = lhs.code[1:]
			}
			return callv(f.phpx(fn)+"("+ref+")", tt)
		}
	}
	cur := f.expr(target, nil)
	t := cur.t
	if t == nil || !t.IsNumber() {
		t = api.Any
	}
	tmp := f.newTmp("v")
	post := strings.HasPrefix(fn, "Post")
	ret := tmp
	stmt := f.incDecStmt(target, op)
	if post {
		return callv(fmt.Sprintf("func() %s { %s := %s; %s; return %s }()", f.typeStr(t), tmp, cur.code, stmt, ret), t)
	}
	return callv(fmt.Sprintf("func() %s { %s; return %s }()", f.typeStr(t), stmt, f.coerce(f.expr(target, nil), t)), t)
}
