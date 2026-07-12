package config

import (
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/expr"
	"github.com/kartaladev/rlng/pipe"
)

// ErrNoStages is the Cause of the ConfigError returned when a definition has no
// stages (e.g. an empty or truncated config document).
var ErrNoStages = errors.New("pipeline has no stages")

// Build compiles the definition into a *pipe.Pipeline, mapping each StageDef to
// the matching stage constructor in list order. Expression and name validation
// is delegated to the stage/expr constructors; Build adds config-shape checks
// and wraps failures in a ConfigError. A definition with no stages is a
// ConfigError (ErrNoStages), so an empty/truncated document fails consistently
// across YAML and JSON rather than building a silent no-op pipeline.
func (d *PipelineDef) Build() (*pipe.Pipeline, error) {
	if len(d.Stages) == 0 {
		return nil, &ConfigError{Cause: ErrNoStages}
	}
	stages := make([]pipe.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build()
		if err != nil {
			return nil, err
		}
		stages = append(stages, st)
	}
	p, err := pipe.NewPipeline(stages...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return p, nil
}

func (sd StageDef) build() (pipe.Stage, error) {
	var base []pipe.Option
	if len(sd.DependsOn) > 0 {
		base = append(base, pipe.WithDependsOn(sd.DependsOn...))
	}
	switch sd.Type {
	case pipe.TypeSingleExpr:
		return sd.buildSingle(base)
	case pipe.TypeMultiExpr:
		return sd.buildMulti(base)
	case pipe.TypeDecisionTable:
		return sd.buildTable(base)
	default:
		return nil, &ConfigError{Stage: sd.Name, Field: "type", Cause: fmt.Errorf("unknown stage type %q", sd.Type)}
	}
}

func (sd StageDef) buildSingle(base []pipe.Option) (pipe.Stage, error) {
	if sd.Expr == nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: errors.New("single-expr requires an expr")}
	}
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithExprOptions(sd.Expr.options()...))
	if sd.Condition != nil {
		opts = append(opts, pipe.WithCondition(sd.Condition.Expr, sd.Condition.options()...))
	}
	if sd.Output != "" {
		opts = append(opts, pipe.WithOutput(sd.Output))
	}
	s, err := pipe.NewSingleExpr(sd.Name, sd.Expr.Expr, opts...)
	if err != nil {
		// The stage error already names the stage; don't re-prefix it. If the
		// culprit is the condition sub-expression, attribute it to that field.
		if sd.Condition != nil {
			if _, cerr := expr.NewPredicate(sd.Condition.Expr, sd.Condition.options()...); cerr != nil {
				return nil, &ConfigError{Stage: sd.Name, Field: "condition", Cause: cerr}
			}
		}
		return nil, &ConfigError{Cause: err}
	}
	return s, nil
}

func (sd StageDef) buildMulti(base []pipe.Option) (pipe.Stage, error) {
	if len(sd.Exprs) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "exprs", Cause: errors.New("multi-expr requires at least one expr")}
	}
	named := make([]pipe.NamedExpr, 0, len(sd.Exprs))
	for _, e := range sd.Exprs {
		named = append(named, pipe.NamedExpr{
			Name:       e.Name,
			Expression: e.Expr.Expr,
			Priority:   e.Priority,
			Options:    e.Expr.options(),
		})
	}
	s, err := pipe.NewMultiExpr(sd.Name, named, base...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

func (sd StageDef) buildTable(base []pipe.Option) (pipe.Stage, error) {
	if len(sd.Rules) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "rules", Cause: errors.New("decision-table requires at least one rule")}
	}
	hp, err := parseHitPolicy(sd.HitPolicy)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "hit_policy", Cause: err}
	}
	rules := make([]pipe.Rule, 0, len(sd.Rules))
	for i, r := range sd.Rules {
		decisions := make(map[string]string, len(r.Decisions))
		for key, ed := range r.Decisions {
			if ed.hasOptions() {
				return nil, &ConfigError{
					Stage: sd.Name,
					Field: fmt.Sprintf("rules[%d].decisions.%s", i, key),
					Cause: errors.New("per-decision options are not supported; use a bare expression"),
				}
			}
			decisions[key] = ed.Expr
		}
		rules = append(rules, pipe.Rule{
			Condition:        r.Condition.Expr,
			ConditionOptions: r.Condition.options(),
			Decisions:        decisions,
		})
	}
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithHitPolicy(hp))
	s, err := pipe.NewDecisionTable(sd.Name, rules, opts...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

func parseHitPolicy(s string) (pipe.HitPolicy, error) {
	switch s {
	case "", "single":
		return pipe.HitPolicySingle, nil
	case "collect":
		return pipe.HitPolicyCollect, nil
	default:
		return 0, fmt.Errorf("unknown hit policy %q", s)
	}
}
