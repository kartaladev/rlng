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

// ErrForEachAsCollision is the Cause of the ConfigError returned when a foreach
// nesting chain reuses an `as` element-binding name (e.g. an outer and inner
// foreach both defaulting to "item"). A reused name would silently shadow the
// enclosing element in the inner per-element scope, so it is rejected at build
// time. Sibling foreach stages on different root-to-leaf chains may reuse a
// name (they run in independent scopes).
var ErrForEachAsCollision = errors.New("foreach: `as` element binding reused by a nested foreach")

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
	if err := d.hydrateConstants(); err != nil {
		return nil, err
	}
	if err := validateForEachAsChains(d.Stages, nil); err != nil {
		return nil, err
	}
	if cfg.lintErrors {
		if findings := d.Lint(); len(findings) > 0 {
			return nil, &LintError{Findings: findings}
		}
	}
	schema := cfg.schema
	if schema == nil {
		schema = d.Schema
	}
	strict := cfg.strict || len(schema) > 0
	if cfg.strict && len(schema) == 0 {
		return nil, &ConfigError{Field: "schema", Cause: errors.New("strict build requires a schema")}
	}
	stages := make([]pipe.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build(d.Constants, schema, strict)
		if err != nil {
			return nil, err
		}
		stages = append(stages, st)
	}
	p, err := pipe.NewPipeline(stages...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	version := cfg.version
	if version == "" {
		version = d.Version
	}
	// Stamp the ruleset identity. A hand-built def carrying a non-marshalable
	// value in an any-typed field cannot produce a stable content hash, so reject
	// it here rather than stamp a meaningless placeholder identity (ADR-0045).
	// The offending value can live in any any-typed field, so the error is not
	// field-scoped; the wrapped marshal error names the type.
	hash, err := d.hashCanonical()
	if err != nil {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %v", ErrUnhashableDef, err)}
	}
	return p.WithRuleset(pipe.RulesetIdentity{Hash: hash, Version: version}), nil
}

func (sd StageDef) build(constants, schema map[string]any, strict bool) (pipe.Stage, error) {
	var base []pipe.Option
	if len(sd.DependsOn) > 0 {
		base = append(base, pipe.WithDependsOn(sd.DependsOn...))
	}
	switch sd.Type {
	case pipe.TypeSingleExpr:
		return sd.buildSingle(base, constants, schema, strict)
	case pipe.TypeMultiExpr:
		return sd.buildMulti(base, constants, schema, strict)
	case pipe.TypeDecisionTable:
		return sd.buildTable(base, constants, schema, strict)
	case pipe.TypeForEach:
		return sd.buildForEach(base, constants, schema, strict)
	default:
		return nil, &ConfigError{Stage: sd.Name, Field: "type", Cause: fmt.Errorf("unknown stage type %q", sd.Type)}
	}
}

// hydrateConstants replaces every {"$dec": "…"} config literal in the
// pipeline's Constants and in every per-expression ExprDef.Globals map with a
// decimal.Decimal, in place, before either feeds expr.WithGlobals. A
// malformed literal is a *ConfigError wrapping ErrDecimalLiteral, attributed
// to the field ("constants" for the pipeline-level map, "<sub>.globals" for a
// per-expression override) where it was declared.
func (d *PipelineDef) hydrateConstants() error {
	if err := hydrateDecimals(d.Constants); err != nil {
		return &ConfigError{Field: "constants", Cause: err}
	}
	for _, sd := range d.Stages {
		if err := sd.hydrateGlobals(); err != nil {
			return err
		}
	}
	return nil
}

// hydrateGlobals hydrates every ExprDef.Globals map reachable from sd: the
// value expr, the condition, each multi-expr entry, each rule's condition and
// decisions, and the table default decisions.
func (sd StageDef) hydrateGlobals() error {
	if err := hydrateExprGlobals(sd.Name, "expr", sd.Expr); err != nil {
		return err
	}
	if err := hydrateExprGlobals(sd.Name, "condition", sd.Condition); err != nil {
		return err
	}
	for i, e := range sd.Exprs {
		if err := hydrateExprGlobals(sd.Name, fmt.Sprintf("exprs[%d].expr", i), &e.Expr); err != nil {
			return err
		}
	}
	for i, r := range sd.Rules {
		if err := hydrateExprGlobals(sd.Name, fmt.Sprintf("rules[%d].condition", i), &r.Condition); err != nil {
			return err
		}
		for key, ed := range r.Decisions {
			if err := hydrateExprGlobals(sd.Name, fmt.Sprintf("rules[%d].decisions.%s", i, key), &ed); err != nil {
				return err
			}
		}
	}
	for key, ed := range sd.Default {
		if err := hydrateExprGlobals(sd.Name, fmt.Sprintf("default.%s", key), &ed); err != nil {
			return err
		}
	}
	for _, isd := range sd.Stages {
		if err := isd.hydrateGlobals(); err != nil {
			return err
		}
	}
	return nil
}

// hydrateExprGlobals hydrates ed's Globals map in place, wrapping a decimal
// literal failure in a *ConfigError naming stage and field+".globals". A nil
// ed (an absent optional sub-expression, e.g. StageDef.Condition) is a no-op.
func hydrateExprGlobals(stage, field string, ed *ExprDef) error {
	if ed == nil {
		return nil
	}
	if err := hydrateDecimals(ed.Globals); err != nil {
		return &ConfigError{Stage: stage, Field: field + ".globals", Cause: err}
	}
	return nil
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

// withStrictEnv appends expr.WithEnv(schema) to opts when strict, so the
// expression type-checks against the declared input shape. Declared
// globals/locals and registered functions are merged into the check env by
// expr.buildExprOpts, so they remain usable.
func withStrictEnv(strict bool, schema map[string]any, opts []expr.Option) []expr.Option {
	if !strict || len(schema) == 0 {
		return opts
	}
	return append(opts, expr.WithEnv(schema))
}

func (sd StageDef) buildSingle(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error) {
	if sd.Expr == nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: errors.New("single-expr requires an expr")}
	}
	condOpts := withStrictEnv(strict, schema, withConstants(constants, sd.condOptions()))
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithExprOptions(withStrictEnv(strict, schema, withConstants(constants, sd.Expr.options()))...))
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
		// failure; only when the value expression compiles cleanly do we check
		// whether the condition genuinely failed before blaming it — otherwise
		// the failure is stage-level (e.g. an empty stage name) and must not be
		// misattributed to a sub-expression field.
		if _, verr := expr.NewFunction(sd.Name, sd.Expr.Expr, withStrictEnv(strict, schema, withConstants(constants, sd.Expr.options()))...); verr != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: err}
		}
		if sd.Condition != nil {
			if _, cerr := expr.NewPredicate(sd.Condition.Expr, condOpts...); cerr != nil {
				return nil, &ConfigError{Stage: sd.Name, Field: "condition", Cause: err}
			}
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

func (sd StageDef) buildMulti(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error) {
	if len(sd.Exprs) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "exprs", Cause: errors.New("multi-expr requires at least one expr")}
	}
	named := make([]pipe.NamedExpr, 0, len(sd.Exprs))
	for _, e := range sd.Exprs {
		named = append(named, pipe.NamedExpr{
			Name:       e.Name,
			Expression: e.Expr.Expr,
			Priority:   e.Priority,
			Options:    withStrictEnv(strict, schema, withConstants(constants, e.Expr.options())),
		})
	}
	s, err := pipe.NewMultiExpr(sd.Name, named, base...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

func (sd StageDef) buildTable(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error) {
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
	for _, r := range sd.Rules {
		rules = append(rules, pipe.Rule{
			ID:               r.ID,
			Message:          r.Message,
			Condition:        r.Condition.Expr,
			ConditionOptions: withStrictEnv(strict, schema, withConstants(constants, r.Condition.options())),
			Decisions:        decisionsFrom(constants, schema, strict, r.Decisions),
		})
	}
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithHitPolicy(hp), pipe.WithCollectAggregation(agg))
	if len(sd.Default) > 0 {
		opts = append(opts, pipe.WithDefault(decisionsFrom(constants, schema, strict, sd.Default)))
	}
	s, err := pipe.NewDecisionTable(sd.Name, rules, opts...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

// buildForEach builds a foreach stage: each inner StageDef is built via the
// same StageDef.build path used for top-level stages (so an inner stage sees
// the same constants/schema/strict-env), assembled into a *pipe.Pipeline, and
// wrapped by pipe.NewForEach. An inner stage may itself be a foreach — the
// isd.build dispatch recurses back into buildForEach, so nesting composes
// without any special-casing here; the as-chain collision guard runs once,
// up front, in Build (validateForEachAsChains). depends-on comes from
// sd.DependsOn via WithForEachDependsOn rather than base, since base carries
// a pipe.Option (WithDependsOn), not a pipe.ForEachOption.
func (sd StageDef) buildForEach(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error) { //nolint:unparam // base is part of the shared build-dispatch signature; see doc comment.
	if len(sd.Stages) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "stages", Cause: errors.New("foreach requires at least one inner stage")}
	}

	inner := make([]pipe.Stage, 0, len(sd.Stages))
	for _, isd := range sd.Stages {
		st, err := isd.build(constants, schema, strict)
		if err != nil {
			return nil, err // already a *ConfigError naming the inner stage
		}
		inner = append(inner, st)
	}

	innerPipe, err := pipe.NewPipeline(inner...)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Cause: err}
	}

	rollups := make([]pipe.Rollup, 0, len(sd.Rollups))
	for _, rd := range sd.Rollups {
		agg, err := parseAggregation(rd.Agg)
		if err != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "rollups", Cause: err}
		}
		rollups = append(rollups, pipe.Rollup{Key: rd.Key, Agg: agg, As: rd.As})
	}

	opts := []pipe.ForEachOption{
		pipe.WithRollups(rollups...),
		pipe.WithForEachDependsOn(sd.DependsOn...),
	}
	if sd.As != "" {
		opts = append(opts, pipe.WithForEachAs(sd.As))
	}
	if sd.Output != "" {
		opts = append(opts, pipe.WithForEachOutput(sd.Output))
	}

	s, err := pipe.NewForEach(sd.Name, sd.Collection, innerPipe, opts...)
	if err != nil {
		return nil, &ConfigError{Cause: err} // stage error already names the stage
	}
	return s, nil
}

// asBinding pairs a foreach stage name with its effective `as` element binding,
// used to detect an `as` reused down a nesting chain.
type asBinding struct{ stage, as string }

// validateForEachAsChains walks the foreach nesting tree in stages and rejects
// any root-to-leaf chain that reuses an `as` element binding, which would
// silently shadow an enclosing element in the inner per-element scope. chain
// holds the (stage, effective-as) of the enclosing foreach ancestors; sibling
// foreaches on different chains may reuse a name. Non-foreach stages are
// skipped (their own inner Stages, if any, belong to a foreach handled here).
func validateForEachAsChains(stages []StageDef, chain []asBinding) error {
	for _, sd := range stages {
		if sd.Type != pipe.TypeForEach {
			continue
		}
		as := sd.As
		if as == "" {
			as = "item" // must match pipe.NewForEach's default (WithForEachAs)
		}
		for _, prior := range chain {
			if prior.as == as {
				return &ConfigError{
					Stage: sd.Name, Field: "as",
					Cause: fmt.Errorf("%w: inner foreach %q reuses element binding %q of enclosing foreach %q",
						ErrForEachAsCollision, sd.Name, as, prior.stage),
				}
			}
		}
		// Copy the chain so sibling recursions cannot alias the backing array.
		child := make([]asBinding, len(chain), len(chain)+1)
		copy(child, chain)
		child = append(child, asBinding{stage: sd.Name, as: as})
		if err := validateForEachAsChains(sd.Stages, child); err != nil {
			return err
		}
	}
	return nil
}

// decisionsFrom converts a key->ExprDef decision set to key->pipe.Decision,
// threading each decision's own options (fallback/globals/coerce) plus the
// shared constants and strict env — so a per-decision option composes with the
// pipeline env instead of being rejected.
func decisionsFrom(constants, schema map[string]any, strict bool, in map[string]ExprDef) map[string]pipe.Decision {
	out := make(map[string]pipe.Decision, len(in))
	for key, ed := range in {
		out[key] = pipe.Decision{
			Expr:    ed.Expr,
			Options: withStrictEnv(strict, schema, withConstants(constants, ed.options())),
		}
	}
	return out
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
