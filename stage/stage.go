package stage

import (
	"context"
	"fmt"
)

// Stage types.
const (
	TypeSingleExpr    = "single-expr"
	TypeMultiExpr     = "multi-expr"
	TypeDecisionTable = "decision-table"
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

func (e *StageError) Error() string {
	return fmt.Sprintf("stage %q (%s): %s", e.Stage, e.Type, e.Cause.Error())
}

func (e *StageError) Unwrap() error { return e.Cause }
