// Package expr provides rlng's atomic expression evaluators, built on
// github.com/expr-lang/expr.
//
// Predicate compiles a boolean expression; by default it is strict (the result
// must be a bool) and returns an EvalError wrapping ErrNotBool otherwise. Pass
// WithCoerce for lenient truthiness.
//
// Function compiles a value-producing expression with an optional WithFallback
// expression, evaluated when the main expression errors or yields nil.
//
// Both accept an environment that is either a map[string]any or a struct
// (converted field-by-field), and both support WithGlobals/WithLocals default
// variables injected as `x ?? <default>` at compile time. All failures are
// *CompileError or *EvalError, which name the field and expression and unwrap
// to the underlying cause.
package expr
