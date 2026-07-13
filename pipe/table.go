package pipe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/kartaladev/rlng/expr"
	"github.com/shopspring/decimal"
)

// HitPolicy selects how a DecisionTable resolves matching rules.
type HitPolicy int

const (
	// HitPolicySingle applies the first matching rule's decisions and stops.
	HitPolicySingle HitPolicy = iota
	// HitPolicyCollect applies every matching rule; each output key accumulates
	// the matched values (reduced per WithCollectAggregation, default a slice).
	HitPolicyCollect
	// HitPolicyUnique requires at most one rule to match; two or more matching
	// rules is ErrMultipleMatches. It guards tables meant to be non-overlapping.
	HitPolicyUnique
	// HitPolicyAny allows several rules to match but requires their outputs to
	// agree for every shared key; a disagreement is ErrConflictingMatches.
	HitPolicyAny
)

// CollectAggregation reduces the matched values of a HitPolicyCollect table's
// output key into a single value.
type CollectAggregation int

const (
	// AggregateList keeps every matched value as a []any in rule order (default).
	AggregateList CollectAggregation = iota
	// AggregateSum sums the matched numeric values: int64 when every matched
	// value is an integer (exact, checked for overflow), decimal.Decimal when
	// any matched value is a decimal (exact), otherwise float64. An int64 sum
	// that would exceed the range of int64 is ErrAggregateOverflow rather than
	// silently wrapping or degrading to float64.
	AggregateSum
	// AggregateMin returns the smallest matched numeric value.
	AggregateMin
	// AggregateMax returns the largest matched numeric value.
	AggregateMax
	// AggregateCount returns the number of matched values as an int.
	AggregateCount
)

// ErrMultipleMatches is the Cause of a StageError from a HitPolicyUnique table
// when more than one rule matches.
var ErrMultipleMatches = errors.New("decision-table: multiple rules matched under unique hit policy")

// ErrConflictingMatches is the Cause of a StageError from a HitPolicyAny table
// when matching rules produce differing values for the same output key.
var ErrConflictingMatches = errors.New("decision-table: matching rules disagree under any hit policy")

// ErrNonNumericAggregate is the Cause of a StageError when a sum/min/max
// aggregation encounters a non-numeric matched value.
var ErrNonNumericAggregate = errors.New("decision-table: non-numeric value in numeric aggregation")

// ErrAggregateOverflow is the Cause of a StageError when an integer sum
// aggregation exceeds the range of int64. The engine errors rather than
// silently wrapping or degrading to float64.
var ErrAggregateOverflow = errors.New("decision-table: integer aggregation overflows int64")

// ErrNonFiniteAggregate is the Cause of a StageError when a NaN or ±Inf float
// would participate in exact-decimal aggregation. A non-finite float has no
// decimal representation (decimal.NewFromFloat panics on it), so the engine
// fails loud rather than crash.
var ErrNonFiniteAggregate = errors.New("decision-table: non-finite float in decimal aggregation")

// Decision is one output of a decision-table Rule: its value expression and the
// compile options (fallback, globals, coerce) that apply to that output alone. A
// bare output with no options is Decision{Expr: "..."}.
type Decision struct {
	Expr    string
	Options []expr.Option
}

// Rule is one row of a DecisionTable: a boolean condition and a set of
// output-key -> Decision entries. Each decision carries its own options, so one
// output can declare a fallback/globals/coerce that a sibling output does not.
// Optional ID and Message make a firing rule identifiable for explainable
// decisions.
type Rule struct {
	ID               string
	Message          string
	Condition        string
	Decisions        map[string]Decision
	ConditionOptions []expr.Option
}

// DecisionTable evaluates ordered rules against a Scope snapshot, writing
// decision outputs under name.<outputKey>.
type DecisionTable struct {
	name        string
	deps        []string
	hitPolicy   HitPolicy
	aggregation CollectAggregation
	rules       []compiledRule
	defaults    []compiledDecision
}

type compiledRule struct {
	id        string
	message   string
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
// are compiled in sorted-key order for deterministic output.
func NewDecisionTable(name string, rules []Rule, opts ...Option) (*DecisionTable, error) {
	if name == "" {
		return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: ErrEmptyStageName}
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
		decisions, err := compileDecisions(name, fmt.Sprintf("rule %d", i), r.Decisions)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiledRule{id: r.ID, message: r.Message, cond: cond, decisions: decisions})
	}

	var defaults []compiledDecision
	if len(cfg.defaults) > 0 {
		var err error
		defaults, err = compileDecisions(name, "default", cfg.defaults)
		if err != nil {
			return nil, err
		}
	}

	return &DecisionTable{
		name:        name,
		deps:        cfg.deps,
		hitPolicy:   cfg.hitPolicy,
		aggregation: cfg.aggregation,
		rules:       compiled,
		defaults:    defaults,
	}, nil
}

// compileDecisions compiles a key->Decision set in sorted-key order, compiling
// each decision's expression with that decision's own options, wrapping failures
// in a StageError attributed to where (e.g. "rule 2").
func compileDecisions(stage, where string, decisions map[string]Decision) ([]compiledDecision, error) {
	out := make([]compiledDecision, 0, len(decisions))
	for _, key := range sortedKeys(decisions) {
		if key == "" {
			return nil, &StageError{Stage: stage, Type: TypeDecisionTable, Cause: fmt.Errorf("%s has an empty output key", where)}
		}
		dec := decisions[key]
		fn, err := expr.NewFunction(key, dec.Expr, dec.Options...)
		if err != nil {
			return nil, &StageError{Stage: stage, Type: TypeDecisionTable, Cause: fmt.Errorf("%s decision %q: %w", where, key, err)}
		}
		out = append(out, compiledDecision{key: key, fn: fn})
	}
	return out, nil
}

// Name returns the stage's name; decision outputs are written under name.<key>.
func (d *DecisionTable) Name() string { return d.name }

// Type returns TypeDecisionTable.
func (d *DecisionTable) Type() string { return TypeDecisionTable }

// DependsOn returns the names of the stages this stage depends on.
func (d *DecisionTable) DependsOn() []string { return d.deps }

// stageErr wraps cause in a StageError naming this decision table.
func (d *DecisionTable) stageErr(cause error) error {
	return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: cause}
}

// Execute evaluates the rules against a Scope snapshot per the hit policy.
func (d *DecisionTable) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return d.stageErr(err)
	}

	env := sc.Snapshot()
	switch d.hitPolicy {
	case HitPolicyCollect:
		return d.executeCollect(env, sc)
	case HitPolicyUnique:
		return d.executeUnique(env, sc)
	case HitPolicyAny:
		return d.executeAny(env, sc)
	default:
		return d.executeSingle(env, sc)
	}
}

// matches evaluates every rule's condition and returns the indices of the rules
// that match, in rule order.
func (d *DecisionTable) matches(env map[string]any) ([]int, error) {
	var matched []int
	for i, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, i)
		}
	}
	return matched, nil
}

// writeDecision writes one decision value under name.<key>, recording provenance
// when the Scope tracks it.
func (d *DecisionTable) writeDecision(env map[string]any, sc *Scope, dec compiledDecision, v any, op string) error {
	if sc.TracksProvenance() {
		return sc.Derive(d.name+"."+dec.key, v, Derivation{
			Stage:      d.name,
			StageType:  TypeDecisionTable,
			Operation:  op,
			Expression: dec.fn.Source(),
			Inputs:     snapshotRefs(env, dec.fn.References()),
		})
	}
	return sc.Set(d.name+"."+dec.key, v)
}

// applyRule evaluates and writes every decision of a single rule, recording it
// as the firing rule.
func (d *DecisionTable) applyRule(env map[string]any, sc *Scope, r compiledRule) error {
	sc.recordFiring(d.name, r.id, r.message, false)
	for _, dec := range r.decisions {
		v, err := dec.fn.Apply(env)
		if err != nil {
			return d.stageErr(err)
		}
		if err := d.writeDecision(env, sc, dec, v, "decision:"+dec.key); err != nil {
			return d.stageErr(err)
		}
	}
	return nil
}

// applyDefaults writes the default decisions (no-op when none configured),
// recording the default as the firing rule when it fires.
func (d *DecisionTable) applyDefaults(env map[string]any, sc *Scope) error {
	if len(d.defaults) == 0 {
		return nil
	}
	sc.recordFiring(d.name, "", "", true)
	for _, dec := range d.defaults {
		v, err := dec.fn.Apply(env)
		if err != nil {
			return d.stageErr(err)
		}
		if err := d.writeDecision(env, sc, dec, v, "default:"+dec.key); err != nil {
			return d.stageErr(err)
		}
	}
	return nil
}

// executeSingle applies the first matching rule (first match wins), or the
// default decisions when no rule matches.
func (d *DecisionTable) executeSingle(env map[string]any, sc *Scope) error {
	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return d.stageErr(err)
		}
		if !ok {
			continue
		}
		return d.applyRule(env, sc, r)
	}
	return d.applyDefaults(env, sc)
}

// executeUnique requires at most one matching rule.
func (d *DecisionTable) executeUnique(env map[string]any, sc *Scope) error {
	matched, err := d.matches(env)
	if err != nil {
		return d.stageErr(err)
	}
	switch len(matched) {
	case 0:
		return d.applyDefaults(env, sc)
	case 1:
		return d.applyRule(env, sc, d.rules[matched[0]])
	default:
		return d.stageErr(fmt.Errorf("%w: %d rules matched", ErrMultipleMatches, len(matched)))
	}
}

// executeAny allows several matches but requires agreement on shared keys.
func (d *DecisionTable) executeAny(env map[string]any, sc *Scope) error {
	matched, err := d.matches(env)
	if err != nil {
		return d.stageErr(err)
	}
	if len(matched) == 0 {
		return d.applyDefaults(env, sc)
	}

	agreed := make(map[string]any)
	order := make([]string, 0)
	for _, idx := range matched {
		for _, dec := range d.rules[idx].decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return d.stageErr(err)
			}
			prev, seen := agreed[dec.key]
			if !seen {
				agreed[dec.key] = v
				order = append(order, dec.key)
				continue
			}
			if !valuesAgree(prev, v) {
				return d.stageErr(fmt.Errorf("%w: key %q has %v and %v", ErrConflictingMatches, dec.key, prev, v))
			}
		}
	}

	firings := make([]FiringRule, 0, len(matched))
	for _, idx := range matched {
		firings = append(firings, FiringRule{Stage: d.name, RuleID: d.rules[idx].id, Message: d.rules[idx].message})
	}
	sc.recordFirings(d.name, firings)

	for _, key := range order {
		if err := d.writeAgg(sc, key, agreed[key], "any:"+key); err != nil {
			return d.stageErr(err)
		}
	}
	return nil
}

// executeCollect accumulates every matching rule's decisions per key, then
// reduces each key by the configured aggregation. When no rule matches, the
// default decisions apply.
func (d *DecisionTable) executeCollect(env map[string]any, sc *Scope) error {
	tracking := sc.TracksProvenance()
	collected := make(map[string][]any)
	order := make([]string, 0)
	var exprs map[string][]string
	var inputs map[string]map[string]any
	if tracking {
		exprs = make(map[string][]string)
		inputs = make(map[string]map[string]any)
	}
	matchedAny := false
	var firings []FiringRule

	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return d.stageErr(err)
		}
		if !ok {
			continue
		}
		matchedAny = true
		firings = append(firings, FiringRule{Stage: d.name, RuleID: r.id, Message: r.message})
		for _, dec := range r.decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return d.stageErr(err)
			}
			if _, seen := collected[dec.key]; !seen {
				order = append(order, dec.key)
			}
			collected[dec.key] = append(collected[dec.key], v)
			if tracking {
				exprs[dec.key] = append(exprs[dec.key], dec.fn.Source())
				for k, rv := range snapshotRefs(env, dec.fn.References()) {
					if inputs[dec.key] == nil {
						inputs[dec.key] = make(map[string]any)
					}
					inputs[dec.key][k] = rv
				}
			}
		}
	}

	if !matchedAny {
		return d.applyDefaults(env, sc)
	}
	sc.recordFirings(d.name, firings)

	for _, key := range order {
		reduced, err := aggregate(d.aggregation, collected[key])
		if err != nil {
			return d.stageErr(err)
		}
		// Preserve the "collect:<key>" provenance label for the default (list)
		// aggregation; name the reducer for sum/min/max/count. An unrecognized
		// aggregation keeps the plain label (and list behavior), never panics.
		op := "collect:" + key
		if lbl, ok := aggLabel(d.aggregation); ok {
			op = "collect:" + lbl + ":" + key
		}
		if err := d.writeCollected(sc, key, reduced, op, exprs[key], inputs[key]); err != nil {
			return d.stageErr(err)
		}
	}
	return nil
}

// writeCollected writes an aggregated collect value under name.<key>, recording
// the joined source expressions and unioned inputs when provenance is tracked.
func (d *DecisionTable) writeCollected(sc *Scope, key string, v any, op string, exprs []string, inputs map[string]any) error {
	if sc.TracksProvenance() {
		return sc.Derive(d.name+"."+key, v, Derivation{
			Stage:      d.name,
			StageType:  TypeDecisionTable,
			Operation:  op,
			Expression: strings.Join(exprs, "; "),
			Inputs:     inputs,
		})
	}
	return sc.Set(d.name+"."+key, v)
}

// writeAgg writes an agreed value (HitPolicyAny) under name.<key>. It is
// provenance-aware but carries no single source expression, since the value is
// shared across several matching rules.
func (d *DecisionTable) writeAgg(sc *Scope, key string, v any, op string) error {
	if sc.TracksProvenance() {
		return sc.Derive(d.name+"."+key, v, Derivation{
			Stage:     d.name,
			StageType: TypeDecisionTable,
			Operation: op,
		})
	}
	return sc.Set(d.name+"."+key, v)
}

// aggregate reduces vals per the aggregation policy.
func aggregate(a CollectAggregation, vals []any) (any, error) {
	switch a {
	case AggregateCount:
		return len(vals), nil
	case AggregateSum:
		return foldNumeric(vals, aggSum)
	case AggregateMin:
		return foldNumeric(vals, aggMin)
	case AggregateMax:
		return foldNumeric(vals, aggMax)
	default: // AggregateList
		return vals, nil
	}
}

type numericOp int

const (
	aggSum numericOp = iota
	aggMin
	aggMax
)

// numKind ranks numeric kinds so a fold promotes to the widest present kind:
// int64 < float64 < decimal.
type numKind int

const (
	kindInt numKind = iota
	kindFloat
	kindDecimal
	kindNonNumeric
)

// classify reports the numeric kind of v, or kindNonNumeric when v is not one
// of the supported numeric types. A uint above math.MaxInt64 is classified
// kindDecimal (not kindInt) because it cannot fold in int64 without wrapping —
// promoting it keeps the aggregation exact.
func classify(v any) numKind {
	switch x := v.(type) {
	case int, int8, int16, int32, int64:
		return kindInt
	case uint, uint8, uint16, uint32, uint64:
		if reflect.ValueOf(x).Uint() > math.MaxInt64 {
			return kindDecimal
		}
		return kindInt
	case float32, float64:
		return kindFloat
	case decimal.Decimal:
		return kindDecimal
	default:
		return kindNonNumeric
	}
}

// foldNumeric folds vals (non-empty) by op with kind fidelity: an all-integer
// fold stays int64 (checked overflow -> ErrAggregateOverflow); any decimal
// present folds in decimal; otherwise float64. min/max return the actual
// matched element. A non-numeric value is ErrNonNumericAggregate.
func foldNumeric(vals []any, op numericOp) (any, error) {
	widest := kindInt
	for _, v := range vals {
		k := classify(v)
		if k == kindNonNumeric {
			return nil, fmt.Errorf("%w: %v (%T)", ErrNonNumericAggregate, v, v)
		}
		if k > widest {
			widest = k
		}
	}
	switch widest {
	case kindDecimal:
		return foldDecimal(vals, op)
	case kindFloat:
		return foldFloat(vals, op)
	default:
		return foldInt(vals, op)
	}
}

// foldInt folds vals (all classified kindInt) in int64. Sum is checked for
// int64 overflow via the canonical test: adding operand n overflows iff n
// is positive and the sum is smaller than the running total, or n is
// negative and the sum is larger than the running total. This correctly
// flags acc == n == math.MinInt64, which wraps to exactly 0 and would evade
// a same-sign-operands check. min/max return the actual matched element
// (vals[bestIdx]) rather than a reconstructed int64, so the caller sees the
// original type (e.g. Go's native int for an expr literal).
func foldInt(vals []any, op numericOp) (any, error) {
	acc := toInt64(vals[0])
	best := acc // for min/max, best is the winning element's int64
	bestIdx := 0
	for i := 1; i < len(vals); i++ {
		n := toInt64(vals[i])
		switch op {
		case aggSum:
			sum := acc + n
			// Canonical signed-overflow test: adding a positive n that makes
			// the sum smaller than acc is positive overflow; adding a
			// negative n that makes the sum larger than acc is negative
			// overflow. Unlike a same-sign-operands check, this also catches
			// acc == n == math.MinInt64, where acc+n wraps to exactly 0 and
			// would otherwise slip through undetected.
			if (n > 0 && sum < acc) || (n < 0 && sum > acc) {
				return nil, ErrAggregateOverflow
			}
			acc = sum
		case aggMin:
			if n < best {
				best, bestIdx = n, i
			}
		case aggMax:
			if n > best {
				best, bestIdx = n, i
			}
		}
	}
	if op == aggSum {
		return acc, nil
	}
	return vals[bestIdx], nil // return the ACTUAL matched element
}

// foldFloat folds vals (kindInt/kindFloat present, no decimal) in float64.
// min/max return the actual matched element.
func foldFloat(vals []any, op numericOp) (any, error) {
	acc := toFloat64(vals[0])
	best := acc
	bestIdx := 0
	for i := 1; i < len(vals); i++ {
		n := toFloat64(vals[i])
		switch op {
		case aggSum:
			acc += n
		case aggMin:
			if n < best {
				best, bestIdx = n, i
			}
		case aggMax:
			if n > best {
				best, bestIdx = n, i
			}
		}
	}
	if op == aggSum {
		return acc, nil
	}
	return vals[bestIdx], nil
}

// foldDecimal folds vals (any decimal present) in decimal.Decimal for exact
// arithmetic. min/max return the actual matched element. A non-finite float
// among vals has no decimal representation and is ErrNonFiniteAggregate rather
// than a panic.
func foldDecimal(vals []any, op numericOp) (any, error) {
	acc, ok := asDecimal(vals[0])
	if !ok {
		return nil, fmt.Errorf("%w: %v (%T)", ErrNonFiniteAggregate, vals[0], vals[0])
	}
	best := acc
	bestIdx := 0
	for i := 1; i < len(vals); i++ {
		d, ok := asDecimal(vals[i])
		if !ok {
			return nil, fmt.Errorf("%w: %v (%T)", ErrNonFiniteAggregate, vals[i], vals[i])
		}
		switch op {
		case aggSum:
			acc = acc.Add(d)
		case aggMin:
			if d.Cmp(best) < 0 {
				best, bestIdx = d, i
			}
		case aggMax:
			if d.Cmp(best) > 0 {
				best, bestIdx = d, i
			}
		}
	}
	if op == aggSum {
		return acc, nil
	}
	return vals[bestIdx], nil
}

// toInt64 converts a value already classified kindInt to int64 via the shared
// numeric kernel. Callers guarantee the classification, so an unrecognized
// kind (unreachable in practice) reports zero rather than panicking.
func toInt64(v any) int64 {
	i, err := int64FromNumeric(reflect.ValueOf(v))
	if err != nil {
		return 0 // unreachable: classify guaranteed kindInt (uint64 <= MaxInt64)
	}
	return i
}

// toFloat64 converts a value already classified kindInt or kindFloat to
// float64 via the shared numeric kernel. Callers guarantee the classification,
// so an unrecognized kind (unreachable in practice) reports zero rather than
// panicking.
func toFloat64(v any) float64 {
	f, err := float64FromNumeric(reflect.ValueOf(v))
	if err != nil {
		return 0 // unreachable: classify guaranteed kindInt or kindFloat
	}
	return f
}

// asDecimal converts a numeric value (int/uint kinds, finite float32/64, or an
// already-decimal.Decimal) to decimal.Decimal via the shared numeric kernel. ok
// is false for a non-numeric v or a non-finite float (NaN/±Inf), which has no
// decimal representation — callers must not pass such a value to
// decimal.NewFromFloat (it panics). Full uint64 range is preserved via big.Int
// (int64(rv.Uint()) would wrap).
func asDecimal(v any) (decimal.Decimal, bool) {
	if d, ok := v.(decimal.Decimal); ok {
		return d, true
	}
	return decimalFromNumeric(reflect.ValueOf(v))
}

// valuesAgree reports whether two decision values agree for HitPolicyAny.
// Numeric values agree by magnitude regardless of int/float/decimal kind (so
// 10 and 10.0 agree); non-numeric values (or a numeric/non-numeric mix) fall
// back to reflect.DeepEqual.
func valuesAgree(a, b any) bool {
	ka, kb := classify(a), classify(b)
	if ka != kindNonNumeric && kb != kindNonNumeric {
		da, oka := asDecimal(a)
		db, okb := asDecimal(b)
		if oka && okb {
			return da.Equal(db)
		}
		// A non-finite float has no decimal form; fall through to identity
		// comparison rather than panic (it will simply not agree with a decimal).
	}
	return reflect.DeepEqual(a, b)
}

// aggLabel names a reducing aggregation for provenance operation labels,
// reporting false for the list default or any unrecognized value (so callers
// keep the plain "collect:<key>" label rather than indexing out of range).
func aggLabel(a CollectAggregation) (string, bool) {
	switch a {
	case AggregateSum:
		return "sum", true
	case AggregateMin:
		return "min", true
	case AggregateMax:
		return "max", true
	case AggregateCount:
		return "count", true
	default:
		return "", false
	}
}

func sortedKeys(m map[string]Decision) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
