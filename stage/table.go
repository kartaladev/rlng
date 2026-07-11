package stage

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kartaladev/rlng/expr"
)

// HitPolicy selects how a DecisionTable resolves matching rules.
type HitPolicy int

const (
	// HitPolicySingle applies the first matching rule's decisions and stops.
	HitPolicySingle HitPolicy = iota
	// HitPolicyCollect applies every matching rule; each output key accumulates
	// a []any with one entry per matched rule, in rule order.
	HitPolicyCollect
)

// Rule is one row of a DecisionTable: a boolean condition and a set of
// output-key -> value-expression decisions.
type Rule struct {
	Condition        string
	Decisions        map[string]string
	ConditionOptions []expr.Option
	DecisionOptions  []expr.Option
}

// DecisionTable evaluates ordered rules against a Scope snapshot, writing
// decision outputs under name.<outputKey>.
type DecisionTable struct {
	name      string
	deps      []string
	hitPolicy HitPolicy
	rules     []compiledRule
}

type compiledRule struct {
	cond      *expr.Predicate
	decisions []compiledDecision
}

type compiledDecision struct {
	key string
	fn  *expr.Function
}

// NewDecisionTable compiles a DecisionTable stage. Every condition and decision
// is compiled up front. Within a rule, decisions are independent (evaluated
// against the same pre-rule snapshot), so their order is not significant; they
// are compiled in sorted-key order for deterministic collect output.
func NewDecisionTable(name string, rules []Rule, opts ...Option) (*DecisionTable, error) {
	if name == "" {
		return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: errEmptyStageName}
	}
	if len(rules) == 0 {
		return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: errors.New("decision-table requires at least one rule")}
	}
	cfg := newStageConfig(opts)

	compiled := make([]compiledRule, 0, len(rules))
	for i, r := range rules {
		if len(r.Decisions) == 0 {
			return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d has no decisions", i)}
		}
		cond, err := expr.NewPredicate(r.Condition, r.ConditionOptions...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d condition: %w", i, err)}
		}

		decisions := make([]compiledDecision, 0, len(r.Decisions))
		for _, key := range sortedKeys(r.Decisions) {
			if key == "" {
				return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d has an empty output key", i)}
			}
			fn, err := expr.NewFunction(key, r.Decisions[key], r.DecisionOptions...)
			if err != nil {
				return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d decision %q: %w", i, key, err)}
			}
			decisions = append(decisions, compiledDecision{key: key, fn: fn})
		}
		compiled = append(compiled, compiledRule{cond: cond, decisions: decisions})
	}
	return &DecisionTable{name: name, deps: cfg.deps, hitPolicy: cfg.hitPolicy, rules: compiled}, nil
}

// Name returns the stage's name; decision outputs are written under name.<key>.
func (d *DecisionTable) Name() string { return d.name }

// Type returns TypeDecisionTable.
func (d *DecisionTable) Type() string { return TypeDecisionTable }

// DependsOn returns the names of the stages this stage depends on.
func (d *DecisionTable) DependsOn() []string { return d.deps }

// Execute evaluates the rules against a Scope snapshot per the hit policy.
func (d *DecisionTable) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
	}

	env := sc.Snapshot()
	if d.hitPolicy == HitPolicyCollect {
		return d.executeCollect(env, sc)
	}
	return d.executeSingle(env, sc)
}

func (d *DecisionTable) executeSingle(env map[string]any, sc *Scope) error {
	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
		if !ok {
			continue
		}
		for _, dec := range r.decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
			if err := sc.Set(d.name+"."+dec.key, v); err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
		}
		return nil // first match wins
	}
	return nil
}

func (d *DecisionTable) executeCollect(env map[string]any, sc *Scope) error {
	collected := make(map[string][]any)
	order := make([]string, 0)
	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
		if !ok {
			continue
		}
		for _, dec := range r.decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
			if _, seen := collected[dec.key]; !seen {
				order = append(order, dec.key)
			}
			collected[dec.key] = append(collected[dec.key], v)
		}
	}
	for _, key := range order {
		if err := sc.Set(d.name+"."+key, collected[key]); err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
