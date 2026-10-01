package conv

import (
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"

	"github.com/PMGopher/phar2go/internal/api"
)

// phpxFuncName is the name of the phpx function for a PHP function: str_replace -> StrReplace.
func phpxFuncName(name string) string {
	parts := strings.Split(strings.ToLower(name), "_")
	var sb strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return sb.String()
}

var phpxRenames = map[string]string{
	"pow":       "PowFn",
	"is_null":   "IsNullV",
	"array_key_exists": "ArrayKeyExists",
}

// funcCall converts a call to a function.
func (f *fctx) funcCall(x *ast.ExprFunctionCall, want *api.Type) value {
	fname := ""
	switch x.Function.(type) {
	case *ast.Name, *ast.NameFullyQualified, *ast.NameRelative:
		fname = identValue(x.Function)
	}
	if fname == "" {
		// $callable(...)
		fn := f.expr(x.Function, nil)
		if fn.t != nil && fn.t.K == api.KFunc {
			sig := &api.Func{Params: fn.t.Params, Results: fn.t.Results, Variadic: fn.t.Variadic}
			return f.callResults(paren(fn, 7)+"("+f.callArgs(sig, x.Args, nil)+")", fn.t.Results)
		}
		return callv(f.phpx("Invoke")+"("+fn.code+f.anyArgs(x.Args)+")", api.Any)
	}
	name := strings.ToLower(lastSeg(fname))
	if fn, ok := f.cv.funcs[name]; ok {
		return f.methodResult(fn.GoName+"("+f.callArgs(fn.m.Sig, x.Args, fn.m)+")", fn.m)
	}
	args := phpArgs(x.Args)
	arg := func(i int, want *api.Type) value {
		if i < len(args) {
			return f.expr(args[i].expr, want)
		}
		return prim("nil", api.UntypedNil)
	}
	switch name {
	case "count", "sizeof":
		if len(args) == 1 {
			v := arg(0, nil)
			switch {
			case isArrayT(v.t):
				return callv(paren(v, 7)+".Len()", api.Int)
			case v.t != nil && (v.t.K == api.KSlice || v.t.K == api.KMap || v.t.IsString()):
				return callv("len("+v.code+")", api.Int)
			}
			return callv(f.phpx("Len")+"("+v.code+")", api.Int)
		}
	case "strlen":
		v := arg(0, api.String)
		return callv("len("+f.coerce(v, api.String)+")", api.Int)
	case "is_null":
		v := arg(0, nil)
		return callv(f.phpx("IsNull")+"("+v.code+")", api.Bool)
	case "array_key_exists", "key_exists":
		if len(args) == 2 {
			arr := arg(1, nil)
			if isArrayT(arr.t) {
				return callv(paren(arr, 7)+".Has("+arg(0, nil).code+")", api.Bool)
			}
		}
	case "isset":
		var vs []ast.Vertex
		for _, a := range args {
			vs = append(vs, a.expr)
		}
		return f.isset(vs)
	case "intval":
		if len(args) == 1 {
			return f.cast(args[0].expr, api.Int)
		}
	case "floatval", "doubleval":
		return f.cast(args[0].expr, api.Float)
	case "strval":
		return f.cast(args[0].expr, api.String)
	case "boolval":
		return f.cast(args[0].expr, api.Bool)
	case "call_user_func":
		if len(args) > 0 {
			fn := arg(0, nil)
			return callv(f.phpx("Invoke")+"("+fn.code+f.anyArgs(x.Args[1:])+")", api.Any)
		}
	case "call_user_func_array":
		if len(args) == 2 {
			fn := arg(0, nil)
			a := arg(1, nil)
			return callv(f.phpx("Invoke")+"("+fn.code+", "+f.phpx("ToArray")+"("+a.code+").Values()...)", api.Any)
		}
	case "microtime":
		if len(args) == 1 {
			if v := arg(0, api.Bool); v.code == "true" {
				return callv(f.phpx("MicrotimeFloat")+"()", api.Float)
			}
		}
	case "hrtime":
		if len(args) == 1 {
			if v := arg(0, api.Bool); v.code == "true" {
				return callv("int("+f.phpx("Hrtime")+"(true).(int))", api.Int)
			}
		}
	case "define":
		if len(args) == 2 {
			if s, ok := args[0].expr.(*ast.ScalarString); ok {
				g := f.cv.defines[strings.ToLower(unquotePHP(s))]
				v := arg(1, nil)
				return value{code: g + " = " + f.arrayElemCode(v), t: api.Void, prec: 1, call: true}
			}
		}
	case "defined":
		if len(args) == 1 {
			if s, ok := args[0].expr.(*ast.ScalarString); ok {
				_, ok := f.cv.defines[strings.ToLower(unquotePHP(s))]
				if ok {
					return prim("true", api.Bool)
				}
			}
			return prim("false", api.Bool)
		}
	case "func_get_args":
		return callv(f.todo(x, "func_get_args() isn't supported")+f.phpx("NewArray")+"()", arrayT(nil, nil))
	case "compact":
		var kv []string
		for _, a := range args {
			if s, ok := a.expr.(*ast.ScalarString); ok {
				vn := unquotePHP(s)
				v := f.variable(&ast.ExprVariable{Name: &ast.Identifier{Value: []byte("$" + vn)}})
				kv = append(kv, quote(vn), f.arrayElemCode(v))
			}
		}
		return callv(f.phpx("Map")+"("+strings.Join(kv, ", ")+")", arrayT(nil, nil))
	case "extract", "get_defined_vars", "debug_backtrace", "debug_print_backtrace", "eval", "create_function", "set_error_handler", "set_exception_handler", "register_shutdown_function", "spl_autoload_register":
		return callv(f.todo(x, "%s() isn't supported", name)+f.phpx("Unsupported")+"("+quote(name)+f.anyArgs(x.Args)+")", api.Any)
	case "max", "min":
		// Typed fast path for numbers.
		if len(args) >= 2 {
			var vals []value
			allInt, allNum := true, true
			for i := range args {
				v := arg(i, nil)
				vals = append(vals, v)
				if !v.t.IsInt() {
					allInt = false
				}
				if !v.t.IsNumber() {
					allNum = false
				}
			}
			if allNum {
				fn, t := "MaxFloat", api.Float
				if name == "min" {
					fn = "MinFloat"
				}
				if allInt {
					fn, t = "MaxInt", api.Int
					if name == "min" {
						fn = "MinInt"
					}
				}
				var codes []string
				for _, v := range vals {
					codes = append(codes, f.coerce(v, t))
				}
				return callv(f.phpx(fn)+"("+strings.Join(codes, ", ")+")", t)
			}
		}
	case "is_a", "is_subclass_of":
		f.anyArgs(x.Args)
		return callv(f.todo(x, "%s() isn't supported; use instanceof", name)+"false", api.Bool)
	case "preg_match", "preg_match_all":
		if len(args) >= 3 {
			p := arg(0, api.String)
			s := arg(1, api.String)
			ref := f.refArg(args[2].expr, api.Ptr(arrayT(nil, nil)))
			rest := ""
			if name == "preg_match_all" && len(args) > 3 {
				rest = ", " + arg(3, nil).code
			}
			return callv(f.phpx(phpxFuncName(name))+"("+f.coerce(p, api.String)+", "+f.coerce(s, api.String)+", "+ref+rest+")", api.Int)
		}
	case "str_replace":
		if len(args) == 3 {
			subj := arg(2, nil)
			if subj.t.IsString() {
				return callv(f.phpx("StrReplaceString")+"("+arg(0, nil).code+", "+arg(1, nil).code+", "+subj.code+")", api.String)
			}
		}
	case "json_encode":
		return callv(f.phpx("JsonEncodeString")+"("+f.callArgsRaw(args)+")", api.String)
	case "array_push":
		if len(args) >= 1 {
			a := arg(0, nil)
			if a.t != nil && a.t.K == api.KSlice && a.lvalue {
				var vals []string
				for i := 1; i < len(args); i++ {
					vals = append(vals, f.coerce(arg(i, a.t.Elem), a.t.Elem))
				}
				return value{code: a.code + " = append(" + a.code + ", " + strings.Join(vals, ", ") + ")", t: api.Void, prec: 1, call: true}
			}
		}
	case "var_dump", "print_r", "var_export", "printf", "echo":
	}
	// The phpx implementation.
	goName := phpxFuncName(name)
	if r, ok := phpxRenames[name]; ok {
		goName = r
	}
	if fn, ok := f.cv.idx.Packages[api.PhpxPath].Funcs[goName]; ok && fn.TypeParams == 0 {
		return f.callResults(f.phpx(goName)+"("+f.callArgs(fn, x.Args, nil)+")", fn.Results)
	}
	return callv(f.todo(x, "function %s() isn't supported", fname)+f.phpx("Unsupported")+"("+quote(fname)+f.anyArgs(x.Args)+")", api.Any)
}

// callArgsRaw converts arguments as plain values.
func (f *fctx) callArgsRaw(args []phpArg) string {
	var out []string
	for i, a := range args {
		v := f.expr(a.expr, nil)
		if i > 0 {
			out = append(out, f.coerce(v, api.Int))
		} else {
			out = append(out, v.code)
		}
	}
	return strings.Join(out, ", ")
}
