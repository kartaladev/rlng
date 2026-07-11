package expr

import (
	"strings"

	exprlang "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Function is a compiled value-producing expression with an optional
// fallback. It is safe for concurrent use.
type Function struct {
	name        string
	expression  string
	program     *vm.Program
	fallback    *vm.Program
	fallbackSrc string
	refs        []string
}

// NewFunction compiles expression into a named Function. When WithFallback is
// given, the fallback expression is compiled now (its compile errors surface
// from NewFunction, not from Apply) and evaluated, over an empty env,
// whenever Apply's main expression errors or yields nil. WithReturnKind
// compiles the main expression to coerce its result to the given kind.
func NewFunction(name, expression string, opts ...Option) (*Function, error) {
	src := strings.TrimSpace(expression)
	if src == "" {
		return nil, &CompileError{Name: name, Expression: expression, Cause: errEmptyExpression}
	}

	cfg := newConfig(opts)
	exprOpts := buildExprOpts(cfg)

	mainOpts := exprOpts
	if cfg.hasReturnKind {
		mainOpts = append(append([]exprlang.Option{}, exprOpts...), exprlang.AsKind(cfg.returnKind))
	}

	program, err := exprlang.Compile(src, mainOpts...)
	if err != nil {
		return nil, &CompileError{Name: name, Expression: expression, Cause: err}
	}

	fn := &Function{name: name, expression: expression, program: program, refs: references(src)}

	if fb := strings.TrimSpace(cfg.fallback); fb != "" {
		fbProgram, err := exprlang.Compile(fb, exprOpts...)
		if err != nil {
			return nil, &CompileError{Name: name, Expression: cfg.fallback, Cause: err}
		}
		fn.fallback = fbProgram
		fn.fallbackSrc = cfg.fallback
	}
	return fn, nil
}

// Apply evaluates the function against env (a map[string]any or a struct). If
// the main expression errors, or evaluates to nil, and a fallback was
// configured via WithFallback, the fallback is evaluated (over an empty env)
// and its result returned instead.
func (f *Function) Apply(env any) (any, error) {
	m, err := toEnv(env)
	if err != nil {
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}

	result, err := exprlang.Run(f.program, m)
	if err != nil {
		if f.fallback != nil {
			return f.runFallback()
		}
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}
	if result == nil && f.fallback != nil {
		return f.runFallback()
	}
	return result, nil
}

// References returns the sorted top-level identifiers this Function reads,
// computed once at compile. The returned slice must not be mutated.
func (f *Function) References() []string { return f.refs }

// Source returns the Function's original (untrimmed) expression string.
func (f *Function) Source() string { return f.expression }

func (f *Function) runFallback() (any, error) {
	result, err := exprlang.Run(f.fallback, map[string]any{})
	if err != nil {
		return nil, &EvalError{Name: f.name, Expression: f.fallbackSrc, Cause: err}
	}
	return result, nil
}
