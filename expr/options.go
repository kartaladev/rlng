package expr

import (
	"reflect"

	exprlang "github.com/expr-lang/expr"
)

// config holds the settings shared by Predicate and Function, assembled from
// functional Options passed to their constructors.
type config struct {
	globals          map[string]any
	locals           map[string]any
	coerce           bool
	fallback         string
	returnKind       reflect.Kind
	hasReturnKind    bool
	functions        []hostFunction
	env              map[string]any
	hasEnv           bool
	fallbackOnNil    bool
	fallbackObserver func(name, expression string, cause error)
}

// hostFunction is a named host function registered into the compile/eval
// environment via WithFunction.
type hostFunction struct {
	name string
	fn   func(...any) (any, error)
}

// Option configures a Predicate or Function. Options that do not apply to a
// given evaluator are ignored: WithCoerce applies only to Predicate;
// WithFallback and WithReturnKind only to Function; WithGlobals/WithLocals to
// both.
type Option func(*config)

func newConfig(opts []Option) *config {
	c := &config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithGlobals adds engine-wide default variables, injected as `??` defaults.
// Multiple calls merge (last value wins per key), so a pipeline-level constant
// and a per-expression global can coexist rather than the later call discarding
// the earlier keys.
func WithGlobals(vars map[string]any) Option {
	return func(c *config) { c.globals = mergeInto(c.globals, vars) }
}

// WithLocals adds per-evaluator default variables; they take precedence over
// globals. Multiple calls merge (last value wins per key), as WithGlobals.
func WithLocals(vars map[string]any) Option {
	return func(c *config) { c.locals = mergeInto(c.locals, vars) }
}

// mergeInto returns dst with every entry of src copied in (last wins),
// allocating dst if nil. It never mutates src.
func mergeInto(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// WithCoerce makes a Predicate use lenient truthiness instead of the default
// strict (bool-only) mode.
func WithCoerce() Option { return func(c *config) { c.coerce = true } }

// WithFallback sets a Function's fallback expression, evaluated (over an empty
// env) when the main expression errors (or, with WithFallbackOnNil, also when
// it yields nil).
func WithFallback(expression string) Option { return func(c *config) { c.fallback = expression } }

// WithFallbackOnNil makes a Function's fallback also fire when the main
// expression evaluates to nil (not only when it errors). By default nil is a
// first-class result and the fallback fires only on an error.
func WithFallbackOnNil() Option { return func(c *config) { c.fallbackOnNil = true } }

// WithFallbackObserver registers a callback invoked when a Function's fallback
// fires because the main expression ERRORED, receiving the function name, the
// main expression, and the triggering cause — so the masked error is observable
// rather than silently discarded. It is not called for a nil-triggered fallback.
func WithFallbackObserver(fn func(name, expression string, cause error)) Option {
	return func(c *config) { c.fallbackObserver = fn }
}

// WithReturnKind compiles a Function to coerce its result to the given kind.
func WithReturnKind(k reflect.Kind) Option {
	return func(c *config) { c.returnKind = k; c.hasReturnKind = true }
}

// WithEnv enables strict compilation against a declared environment: the
// expression is type-checked against env (a map of field name -> a
// representative value giving its type), and undefined-variable tolerance is
// dropped. A field typo such as `scoer` then fails at compile time instead of
// silently evaluating to nil. Declared globals/locals (WithGlobals/WithLocals)
// and registered functions (WithFunction) are merged into the type-check
// environment so they remain usable. Without WithEnv the default is lenient
// (undefined variables allowed), preserving prior behavior.
func WithEnv(env map[string]any) Option {
	return func(c *config) { c.env = env; c.hasEnv = true }
}

// WithFunction registers a host function callable from the expression by name,
// e.g. WithFunction("now", ...) or a domain helper like businessDaysBetween.
// The function is visible to both the compiler (so it type-checks, including in
// WithEnv strict mode) and the VM. Registering the same name twice keeps the
// last registration.
func WithFunction(name string, fn func(...any) (any, error)) Option {
	return func(c *config) { c.functions = append(c.functions, hostFunction{name: name, fn: fn}) }
}

// buildExprOpts assembles the base expr compile options shared by both
// evaluators: undefined-variables allowed, plus the variable-default patcher
// when any variables are declared, plus the exact-decimal value type (see
// decimalExprOptions) which is always available regardless of other options.
func buildExprOpts(cfg *config) []exprlang.Option {
	var opts []exprlang.Option
	if cfg.hasEnv {
		// Strict: type-check against the declared env (plus declared
		// globals/locals so patched defaults resolve) and reject unknown names.
		opts = append(opts, exprlang.Env(strictEnv(cfg)))
	} else {
		opts = append(opts, exprlang.AllowUndefinedVariables())
	}
	if p := newPatcher(cfg.globals, cfg.locals); p != nil {
		opts = append(opts, exprlang.Patch(p))
	}
	for _, f := range cfg.functions {
		opts = append(opts, exprlang.Function(f.name, f.fn))
	}
	opts = append(opts, decimalExprOptions()...) // exact-decimal type, always available
	return opts
}

// strictEnv builds the type-check environment for WithEnv: the declared env,
// overlaid with declared globals then locals (locals take precedence), so an
// identifier resolved by a `??` default patch still type-checks. Registered
// functions are supplied separately via exprlang.Function and need not appear
// here.
func strictEnv(cfg *config) map[string]any {
	out := make(map[string]any, len(cfg.env)+len(cfg.globals)+len(cfg.locals))
	for k, v := range cfg.env {
		out[k] = v
	}
	for k, v := range cfg.globals {
		out[k] = v
	}
	for k, v := range cfg.locals {
		out[k] = v
	}
	return out
}
