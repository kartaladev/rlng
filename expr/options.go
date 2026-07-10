package expr

import "reflect"

// config holds the settings shared by Predicate and Function, assembled from
// functional Options passed to their constructors.
type config struct {
	globals       map[string]any
	locals        map[string]any
	coerce        bool
	fallback      string
	returnKind    reflect.Kind
	hasReturnKind bool
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

// WithGlobals sets engine-wide default variables, injected as `??` defaults.
func WithGlobals(vars map[string]any) Option { return func(c *config) { c.globals = vars } }

// WithLocals sets per-evaluator default variables; they take precedence over globals.
func WithLocals(vars map[string]any) Option { return func(c *config) { c.locals = vars } }

// WithCoerce makes a Predicate use lenient truthiness instead of the default
// strict (bool-only) mode.
func WithCoerce() Option { return func(c *config) { c.coerce = true } }

// WithFallback sets a Function's fallback expression, evaluated (over an empty
// env) when the main expression errors or yields nil.
func WithFallback(expression string) Option { return func(c *config) { c.fallback = expression } }

// WithReturnKind compiles a Function to coerce its result to the given kind.
func WithReturnKind(k reflect.Kind) Option {
	return func(c *config) { c.returnKind = k; c.hasReturnKind = true }
}
