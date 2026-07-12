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
//
// Build is advisory-only by default: a lint smell (e.g. a first-match table
// with no default and no catch-all) does not fail construction. Pass
// WithLintErrors to promote Lint findings to a *LintError instead.
func (d *PipelineDef) Build(opts ...BuildOption) (*pipe.Pipeline, error) {
	cfg := &buildConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if len(d.Stages) == 0 {
		return nil, &ConfigError{Cause: ErrNoStages}
	}
	if cfg.lintErrors {
		if findings := d.Lint(); len(findings) > 0 {
			return nil, &LintError{Findings: findings}
		}
	}
	stages := make([]pipe.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build(d.Constants)
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

func (sd StageDef) build(constants map[string]any) (pipe.Stage, error) {
	var base []pipe.Option
	if len(sd.DependsOn) > 0 {
		base = append(base, pipe.WithDependsOn(sd.DependsOn...))
	}
	switch sd.Type {
	case pipe.TypeSingleExpr:
		return sd.buildSingle(base, constants)
	case pipe.TypeMultiExpr:
		return sd.buildMulti(base, constants)
	case pipe.TypeDecisionTable:
		return sd.buildTable(base, constants)
	default:
		return nil, &ConfigError{Stage: sd.Name, Field: "type", Cause: fmt.Errorf("unknown stage type %q", sd.Type)}
	}
}

// withConstants prepends a WithGlobals option carrying the pipeline constants to
// opts (a no-op when there are none), so every compiled expression sees the
// constants as overridable compile-time defaults. Constants are prepended so a
// stage's own WithGlobals (if any) merges on top and wins on a key conflict —
// the more specific value takes precedence (expr.WithGlobals merges, last wins).
func withConstants(constants map[string]any, opts []expr.Option) []expr.Option {
	if len(constants) == 0 {
		return opts
	}
	return append([]expr.Option{expr.WithGlobals(constants)}, opts...)
}

func (sd StageDef) buildSingle(base []pipe.Option, constants map[string]any) (pipe.Stage, error) {
	if sd.Expr == nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: errors.New("single-expr requires an expr")}
	}
	condOpts := withConstants(constants, sd.condOptions())
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithExprOptions(withConstants(constants, sd.Expr.options())...))
	if sd.Condition != nil {
		opts = append(opts, pipe.WithCondition(sd.Condition.Expr, condOpts...))
	}
	if sd.Output != "" {
		opts = append(opts, pipe.WithOutput(sd.Output))
	}
	s, err := pipe.NewSingleExpr(sd.Name, sd.Expr.Expr, opts...)
	if err != nil {
		// Attribute to the field that actually failed to compile. The value
		// expression is compiled first, so a value error is the true first
		// failure; only a value expression that compiles cleanly leaves the
		// condition as the culprit.
		if _, verr := expr.NewFunction(sd.Name, sd.Expr.Expr, withConstants(constants, sd.Expr.options())...); verr != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: err}
		}
		if sd.Condition != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "condition", Cause: err}
		}
		return nil, &ConfigError{Cause: err}
	}
	return s, nil
}

// condOptions returns the condition sub-expression's options, or nil when there
// is no condition.
func (sd StageDef) condOptions() []expr.Option {
	if sd.Condition == nil {
		return nil
	}
	return sd.Condition.options()
}

func (sd StageDef) buildMulti(base []pipe.Option, constants map[string]any) (pipe.Stage, error) {
	if len(sd.Exprs) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "exprs", Cause: errors.New("multi-expr requires at least one expr")}
	}
	named := make([]pipe.NamedExpr, 0, len(sd.Exprs))
	for _, e := range sd.Exprs {
		named = append(named, pipe.NamedExpr{
			Name:       e.Name,
			Expression: e.Expr.Expr,
			Priority:   e.Priority,
			Options:    withConstants(constants, e.Expr.options()),
		})
	}
	s, err := pipe.NewMultiExpr(sd.Name, named, base...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

func (sd StageDef) buildTable(base []pipe.Option, constants map[string]any) (pipe.Stage, error) {
	if len(sd.Rules) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "rules", Cause: errors.New("decision-table requires at least one rule")}
	}
	hp, err := parseHitPolicy(sd.HitPolicy)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "hit_policy", Cause: err}
	}
	agg, err := parseAggregation(sd.Aggregation)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "aggregation", Cause: err}
	}
	rules := make([]pipe.Rule, 0, len(sd.Rules))
	for i, r := range sd.Rules {
		decisions, err := bareDecisions(sd.Name, fmt.Sprintf("rules[%d].decisions", i), r.Decisions)
		if err != nil {
			return nil, err
		}
		rules = append(rules, pipe.Rule{
			ID:               r.ID,
			Message:          r.Message,
			Condition:        r.Condition.Expr,
			ConditionOptions: withConstants(constants, r.Condition.options()),
			DecisionOptions:  withConstants(constants, nil),
			Decisions:        decisions,
		})
	}
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithHitPolicy(hp), pipe.WithCollectAggregation(agg))
	if len(sd.Default) > 0 {
		defaults, err := bareDecisions(sd.Name, "default", sd.Default)
		if err != nil {
			return nil, err
		}
		opts = append(opts, pipe.WithDefault(defaults, withConstants(constants, nil)...))
	}
	s, err := pipe.NewDecisionTable(sd.Name, rules, opts...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

// bareDecisions converts a key->ExprDef decision set to key->expression,
// rejecting per-decision options (only bare expressions are supported here).
func bareDecisions(stage, field string, in map[string]ExprDef) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for key, ed := range in {
		if ed.hasOptions() {
			return nil, &ConfigError{
				Stage: stage,
				Field: fmt.Sprintf("%s.%s", field, key),
				Cause: errors.New("per-decision options are not supported; use a bare expression"),
			}
		}
		out[key] = ed.Expr
	}
	return out, nil
}

func parseAggregation(s string) (pipe.CollectAggregation, error) {
	switch s {
	case "", "list":
		return pipe.AggregateList, nil
	case "sum":
		return pipe.AggregateSum, nil
	case "min":
		return pipe.AggregateMin, nil
	case "max":
		return pipe.AggregateMax, nil
	case "count":
		return pipe.AggregateCount, nil
	default:
		return 0, fmt.Errorf("unknown aggregation %q", s)
	}
}

func parseHitPolicy(s string) (pipe.HitPolicy, error) {
	switch s {
	case "", "single":
		return pipe.HitPolicySingle, nil
	case "collect":
		return pipe.HitPolicyCollect, nil
	case "unique":
		return pipe.HitPolicyUnique, nil
	case "any":
		return pipe.HitPolicyAny, nil
	default:
		return 0, fmt.Errorf("unknown hit policy %q", s)
	}
}
