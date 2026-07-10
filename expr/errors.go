package expr

import (
	"errors"
	"fmt"
)

// ErrNotBool is returned (wrapped in an EvalError) when a strict Predicate's
// expression evaluates to a non-boolean value.
var ErrNotBool = errors.New("expression did not evaluate to bool")

// errEmptyExpression is returned (wrapped in a CompileError) when an empty or
// whitespace-only expression is supplied to NewPredicate/NewFunction.
var errEmptyExpression = errors.New("expression must not be empty")

// CompileError reports a failure to compile an expression. It names the field
// (if any) and the offending expression, and unwraps to the underlying cause.
type CompileError struct {
	Name       string
	Expression string
	Cause      error
}

func (e *CompileError) Error() string {
	prefix := "compile " + label(e.Name, e.Expression)
	if e.Cause == nil {
		return prefix
	}
	return prefix + ": " + e.Cause.Error()
}

func (e *CompileError) Unwrap() error { return e.Cause }

// EvalError reports a failure while evaluating a compiled expression. It names
// the field (if any) and the expression, and unwraps to the underlying cause.
type EvalError struct {
	Name       string
	Expression string
	Cause      error
}

func (e *EvalError) Error() string {
	prefix := "eval " + label(e.Name, e.Expression)
	if e.Cause == nil {
		return prefix
	}
	return prefix + ": " + e.Cause.Error()
}

func (e *EvalError) Unwrap() error { return e.Cause }

// label renders `"name" (expression)` when a name is present, else `(expression)`.
func label(name, expression string) string {
	if name != "" {
		return fmt.Sprintf("%q (%s)", name, expression)
	}
	return "(" + expression + ")"
}
