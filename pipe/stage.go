package pipe

import (
	"context"
	"errors"
	"fmt"
)

// Stage types.
const (
	TypeSingleExpr    = "single-expr"
	TypeMultiExpr     = "multi-expr"
	TypeDecisionTable = "decision-table"
	TypeForEach       = "foreach"
)

// Stage is a unit of rule/calculation logic that reads from and writes to a
// Scope. Stages compile their expressions at construction; Execute only
// evaluates. Implementations declare their dependencies via DependsOn for the
// DAG orchestrator (increment 3); this layer does not order stages.
type Stage interface {
	Name() string
	Type() string
	DependsOn() []string
	Execute(ctx context.Context, sc *Scope) error
}

// StageError reports a failure constructing or executing a stage. It names the
// stage and its type and unwraps to the underlying cause (typically an
// *expr.CompileError or *expr.EvalError).
type StageError struct {
	Stage string
	Type  string
	Cause error
}

// Error renders `stage "name" (type): <cause>`, or `stage "name" (type)` when
// Cause is nil.
func (e *StageError) Error() string {
	prefix := fmt.Sprintf("stage %q (%s)", e.Stage, e.Type)
	if e.Cause == nil {
		return prefix
	}
	return prefix + ": " + e.Cause.Error()
}

// Unwrap returns the underlying cause, so errors.Is/As reach the wrapped
// expr.CompileError/EvalError or scope error.
func (e *StageError) Unwrap() error { return e.Cause }

// ErrEmptyStageName is the Cause of a StageError returned by New* constructors
// when given an empty stage name, before any compilation is attempted.
var ErrEmptyStageName = errors.New("stage name must not be empty")
