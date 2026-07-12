package expr

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	exprlang "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Predicate is a compiled boolean expression. It is safe for concurrent use.
type Predicate struct {
	expression string
	program    *vm.Program
	coerce     bool
	refs       []string
}

// NewPredicate compiles expression into a Predicate. By default the expression
// must evaluate to a bool (strict); pass WithCoerce for lenient truthiness.
func NewPredicate(expression string, opts ...Option) (*Predicate, error) {
	src := strings.TrimSpace(expression)
	if src == "" {
		return nil, &CompileError{Expression: expression, Cause: ErrEmptyExpression}
	}

	cfg := newConfig(opts)
	exprOpts := buildExprOpts(cfg)

	program, err := exprlang.Compile(src, exprOpts...)
	if err != nil {
		return nil, &CompileError{Expression: expression, Cause: err}
	}
	return &Predicate{expression: expression, program: program, coerce: cfg.coerce, refs: references(src)}, nil
}

// References returns the sorted top-level identifiers this Predicate reads,
// computed once at compile. The returned slice must not be mutated.
func (p *Predicate) References() []string { return p.refs }

// Source returns the Predicate's original (untrimmed) expression string.
func (p *Predicate) Source() string { return p.expression }

// Test evaluates the predicate against env (a map[string]any or a struct).
func (p *Predicate) Test(env any) (bool, error) {
	m, err := toEnv(env)
	if err != nil {
		return false, &EvalError{Expression: p.expression, Cause: err}
	}

	result, err := exprlang.Run(p.program, m)
	if err != nil {
		return false, &EvalError{Expression: p.expression, Cause: err}
	}

	if p.coerce {
		return truthy(result), nil
	}

	b, ok := result.(bool)
	if !ok {
		return false, &EvalError{
			Expression: p.expression,
			Cause:      fmt.Errorf("%w: got %T", ErrNotBool, result),
		}
	}
	return b, nil
}

// truthy implements lenient truthiness for WithCoerce predicates: nil is
// false; bool is itself; a string is parsed via strconv.ParseBool when
// possible, else true iff non-empty (after trimming); any numeric kind (int,
// uint, or float, of any width) is true iff non-zero; a slice or map is true
// iff non-empty; anything else is false.
func truthy(v any) bool {
	if v == nil {
		return false
	}

	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.TrimSpace(x)
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
		return s != ""
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0
	default:
		return false
	}
}
