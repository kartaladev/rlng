package stage

import (
	"context"

	"github.com/kartaladev/rlng/expr"
)

// SingleExpr is a stage that evaluates one value expression, optionally gated by
// a condition predicate, writing the result to an output path in the Scope.
type SingleExpr struct {
	name   string
	output string
	deps   []string
	cond   *expr.Predicate
	fn     *expr.Function
}

// NewSingleExpr compiles a SingleExpr stage. Compilation of the value
// expression and any condition happens now; Execute only evaluates.
func NewSingleExpr(name, expression string, opts ...Option) (*SingleExpr, error) {
	if name == "" {
		return nil, &StageError{Stage: name, Type: TypeSingleExpr, Cause: errEmptyStageName}
	}

	cfg := newStageConfig(opts)
	output := name
	if cfg.hasOutput {
		output = cfg.output
	}

	fn, err := expr.NewFunction(name, expression, cfg.exprOpts...)
	if err != nil {
		return nil, &StageError{Stage: name, Type: TypeSingleExpr, Cause: err}
	}

	s := &SingleExpr{name: name, output: output, deps: cfg.deps, fn: fn}

	if cfg.condition != "" {
		cond, err := expr.NewPredicate(cfg.condition, cfg.condOpts...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeSingleExpr, Cause: err}
		}
		s.cond = cond
	}
	return s, nil
}

func (s *SingleExpr) Name() string        { return s.name }
func (s *SingleExpr) Type() string        { return TypeSingleExpr }
func (s *SingleExpr) DependsOn() []string { return s.deps }

// Execute evaluates the stage against sc. A configured condition that tests
// false makes the stage a no-op.
func (s *SingleExpr) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}

	env := sc.Snapshot()

	if s.cond != nil {
		ok, err := s.cond.Test(env)
		if err != nil {
			return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
		}
		if !ok {
			return nil
		}
	}

	v, err := s.fn.Apply(env)
	if err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}
	if err := sc.Set(s.output, v); err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}
	return nil
}
