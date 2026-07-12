// Package expr provides rlng's atomic expression evaluators, built on
// github.com/expr-lang/expr.
//
// Predicate compiles a boolean expression; by default it is strict (the result
// must be a bool) and returns an EvalError wrapping ErrNotBool otherwise. Pass
// WithCoerce for lenient truthiness.
//
// Function compiles a value-producing expression with an optional WithFallback
// expression. By default the fallback fires only when the main expression
// errors; a nil main result is returned as-is. WithFallbackOnNil restores the
// fallback-on-nil behavior, and WithFallbackObserver surfaces the error the
// fallback would otherwise mask.
//
// Both accept an environment that is either a map[string]any or a struct
// (converted field-by-field), and both support WithGlobals/WithLocals default
// variables injected as `x ?? <default>` at compile time. All failures are
// *CompileError or *EvalError, which name the field and expression and unwrap
// to the underlying cause.
//
// WithCoerce enables lenient truthiness on Predicate: numbers are true iff
// non-zero and finite (NaN/±Inf are false), strings follow strconv.ParseBool
// for recognized bool spellings and otherwise non-empty-after-trim, and an
// unhandled result type returns an *EvalError instead of a silent false.
package expr
