package expr

import (
	"fmt"
	"math"
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

// References returns the sorted, unique paths this Predicate reads: the deepest
// statically-known member path per reference (e.g. "grade.tier"), or the bare
// identifier when the chain is not statically resolvable (dynamic/index access,
// method calls). Computed once at compile; used to record provenance inputs. The
// returned slice must not be mutated.
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
		b, err := truthy(result)
		if err != nil {
			return false, &EvalError{Expression: p.expression, Cause: err}
		}
		return b, nil
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

// truthy implements lenient truthiness for WithCoerce predicates: nil is false;
// bool is itself; a string is parsed via strconv.ParseBool when it names one
// (1/t/T/TRUE/true/True and 0/f/F/FALSE/false/False), else true iff non-empty
// after trimming; any integer/uint kind is true iff non-zero; a float is true
// iff non-zero AND finite (NaN and ±Inf are false); a slice/array/map is true
// iff non-empty. Any other kind (struct, pointer, chan, func, time.Time, ...) is
// an error, so a mistyped predicate fails loudly instead of silently as false.
func truthy(v any) (bool, error) {
	if v == nil {
		return false, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		// Native or named bool type (e.g. `type Flag bool`).
		return rv.Bool(), nil
	case reflect.String:
		// Native or named string type, and json.Number for String: parse a bool
		// literal, else non-empty is true.
		s := strings.TrimSpace(rv.String())
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		}
		return s != "", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0, nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		return f != 0 && !math.IsNaN(f) && !math.IsInf(f, 0), nil
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0, nil
	default:
		return false, fmt.Errorf("%w: cannot coerce %T to bool", ErrNotBool, v)
	}
}
