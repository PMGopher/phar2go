package phpx

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
)

// Throwable is PHP's Throwable: anything that can be thrown and caught.
type Throwable interface {
	error
	GetMessage() string
	GetCode() int
	GetPrevious() Throwable
	GetTraceAsString() string
	ToString() string
	// PhpClasses lists the lower-case PHP class names the exception is an instance of, its own
	// class first.
	PhpClasses() []string
	PhpClass() string
}

// Exception is PHP's Exception and Error, and the base of the plugin's own exception classes.
type Exception struct {
	Message  string
	Code     int
	Previous Throwable
	classes  []string
	trace    string
}

// builtinParents is the parent of each built-in exception class.
var builtinParents = map[string]string{
	"exception":                   "throwable",
	"error":                       "throwable",
	"errorexception":              "exception",
	"typeerror":                   "error",
	"valueerror":                  "error",
	"arithmeticerror":             "error",
	"divisionbyzeroerror":         "arithmeticerror",
	"argumentcounterror":          "typeerror",
	"assertionerror":              "error",
	"unhandledmatcherror":         "error",
	"runtimeexception":            "exception",
	"logicexception":              "exception",
	"invalidargumentexception":    "logicexception",
	"domainexception":             "logicexception",
	"lengthexception":             "logicexception",
	"outofrangeexception":         "logicexception",
	"badfunctioncallexception":    "logicexception",
	"badmethodcallexception":      "badfunctioncallexception",
	"outofboundsexception":        "runtimeexception",
	"overflowexception":           "runtimeexception",
	"underflowexception":          "runtimeexception",
	"rangeexception":              "runtimeexception",
	"unexpectedvalueexception":    "runtimeexception",
	"jsonexception":               "exception",
	"pocketmine\\utils\\assumptionfailederror": "error",
}

// RegisterExceptionClass declares the parent of an exception class (lower-case names), for
// classes that aren't built into PHP.
func RegisterExceptionClass(class, parent string) {
	class, parent = strings.ToLower(strings.TrimPrefix(class, "\\")), strings.ToLower(strings.TrimPrefix(parent, "\\"))
	if _, ok := builtinParents[class]; !ok {
		builtinParents[class] = parent
	}
}

func classChain(class string) []string {
	class = strings.ToLower(strings.TrimPrefix(class, "\\"))
	var out []string
	seen := map[string]bool{}
	for class != "" && !seen[class] {
		seen[class] = true
		out = append(out, class)
		p, ok := builtinParents[class]
		if !ok {
			if class != "throwable" {
				out = append(out, "exception", "throwable")
			}
			break
		}
		class = p
	}
	return out
}

// NewException creates an exception of the given PHP class (new $class($message)).
func NewException(class string, message string, extra ...any) *Exception {
	e := &Exception{Message: message, classes: classChain(class), trace: string(debug.Stack())}
	e.applyExtra(extra)
	return e
}

// NewError is NewException for Error classes.
func NewError(class string, message string) *Exception { return NewException(class, message) }

func (e *Exception) applyExtra(extra []any) {
	if len(extra) > 0 {
		e.Code = ToInt(extra[0])
	}
	if len(extra) > 1 {
		if p, ok := extra[1].(Throwable); ok && !IsNull(p) {
			e.Previous = p
		}
	}
}

// InitException is `parent::__construct($message, $code, $previous)` in a plugin's exception
// class.
func (e *Exception) InitException(class string, message any, extra ...any) {
	e.Message = ToString(message)
	if len(e.classes) == 0 {
		e.classes = classChain(class)
	}
	e.trace = string(debug.Stack())
	e.applyExtra(extra)
}

// SetClasses sets the class chain of a plugin exception class.
func (e *Exception) SetClasses(classes ...string) {
	for i, c := range classes {
		classes[i] = strings.ToLower(strings.TrimPrefix(c, "\\"))
	}
	if len(classes) > 0 {
		classes = append(classes[:len(classes)-1], classChain(classes[len(classes)-1])...)
	}
	e.classes = classes
}

func (e *Exception) Error() string            { return e.Message }
func (e *Exception) GetMessage() string       { return e.Message }
func (e *Exception) GetCode() int             { return e.Code }
func (e *Exception) GetPrevious() Throwable   { return e.Previous }
func (e *Exception) GetTraceAsString() string { return e.trace }
func (e *Exception) GetFile() string          { return "" }
func (e *Exception) GetLine() int             { return 0 }
func (e *Exception) PhpClasses() []string {
	if len(e.classes) == 0 {
		e.classes = classChain("exception")
	}
	return e.classes
}
func (e *Exception) PhpClass() string { return e.PhpClasses()[0] }
func (e *Exception) ToString() string {
	return fmt.Sprintf("%s: %s", e.PhpClass(), e.Message)
}
func (e *Exception) Unwrap() error {
	if e.Previous == nil {
		return nil
	}
	return e.Previous
}

// goError is a Go error or panic seen from PHP code.
type goError struct {
	Exception
	err error
}

func (g *goError) Unwrap() error { return g.err }

// AsThrowable converts a recovered panic value or a Go error to a Throwable.
func AsThrowable(r any) Throwable {
	switch x := r.(type) {
	case Throwable:
		return x
	case error:
		var t Throwable
		if errors.As(x, &t) {
			return t
		}
		class := "exception"
		if _, ok := r.(interface{ RuntimeError() }); ok {
			class = "error"
		}
		return &goError{Exception: Exception{Message: x.Error(), classes: classChain(class), trace: string(debug.Stack())}, err: x}
	}
	return &goError{Exception: Exception{Message: fmt.Sprint(r), classes: classChain("error"), trace: string(debug.Stack())}}
}

// Throw is `throw $e`.
func Throw(e any) {
	if IsNull(e) {
		panic(NewError("Error", "Can only throw objects"))
	}
	panic(AsThrowable(e))
}

// InstanceOf reports whether a throwable is an instance of a PHP class (lower-case name).
func InstanceOf(t Throwable, class string) bool {
	class = strings.ToLower(strings.TrimPrefix(class, "\\"))
	for _, c := range t.PhpClasses() {
		if c == class {
			return true
		}
	}
	return false
}

// Catch is one catch block: the classes it catches and its body. The body returns a control
// code (0 = fall through, 1 = return, 2 = break, 3 = continue) and the returned value.
type Catch struct {
	Classes []string
	Body    func(e Throwable) (int, any)
}

// Try runs a try/catch/finally statement. body, the catch bodies and finally run in order; the
// control code and value of the block that ended the statement are returned.
func Try(body func() (int, any), finally func(), catches ...Catch) (ctl int, val any) {
	if finally != nil {
		defer finally()
	}
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			t := AsThrowable(r)
			for _, c := range catches {
				for _, class := range c.Classes {
					if InstanceOf(t, class) {
						ctl, val = c.Body(t)
						return
					}
				}
			}
			panic(r)
		}()
		ctl, val = body()
	}()
	return ctl, val
}

// Recover turns a panic into an error, for converted methods that return one (OnEnable,
// OnRun, ...). Use it as `defer phpx.Recover(&err)`.
func Recover(err *error) {
	if r := recover(); r != nil {
		t := AsThrowable(r)
		*err = t
	}
}

// RecoverLog recovers a panic and reports it through log (for methods without an error result).
func RecoverLog(log func(string)) {
	if r := recover(); r != nil {
		t := AsThrowable(r)
		log(t.ToString())
	}
}

// InstanceOfValue is `$v instanceof SomeException` for any value.
func InstanceOfValue(v any, class string) bool {
	if IsNull(v) {
		return false
	}
	t, ok := v.(Throwable)
	if !ok {
		if _, isErr := v.(error); !isErr {
			return false
		}
		t = AsThrowable(v)
	}
	return InstanceOf(t, class)
}
