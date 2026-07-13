package pipe

import (
	"context"
	"errors"
	"fmt"
)

// ErrForEachNotList is the Cause of a StageError when the collection path
// holds a value that is not a []any.
var ErrForEachNotList = errors.New("foreach: collection is not a list")

// ErrForEachNoCollection is the Cause of a StageError when the collection
// path is not present in the Scope.
var ErrForEachNoCollection = errors.New("foreach: collection path not found")

// ErrForEachEmptyRollup is the Cause of a StageError returned by NewForEach
// when a Rollup has an empty Key or As. Both are required: an empty As has no
// output path to write to, and an empty Key matches no per-element value — both
// are fail-loud at construction rather than a confusing runtime path error or a
// silently-empty roll-up.
var ErrForEachEmptyRollup = errors.New("foreach: rollup Key and As must not be empty")

// ForEach is a stage that runs an inner *Pipeline once per element of a Scope
// collection, against a fresh per-element Scope, collecting each element's
// resulting data back as an ordered list. See ADR-0040.
type ForEach struct {
	name       string
	collection string
	as         string
	output     string
	deps       []string
	inner      *Pipeline
	rollups    []Rollup
}

// Rollup reduces a per-element output key across all elements into a single
// header value, written to the outer Scope at "<stage name>.<As>". Key is a
// dot-separated path resolved in each element's result map (produced by the
// inner pipeline): a dot-free key is a top-level lookup, while a nested key
// such as "grade.score" reads a value an inner stage namespaced under its own
// name — e.g. a decision-table output "<table>.<key>" — without a companion
// single-expr to surface it. An element whose path is missing, or whose
// intermediate segment is not a map, is skipped. Agg is the same
// CollectAggregation used by a HitPolicyCollect DecisionTable, so a Rollup
// carries the same int64/decimal.Decimal/float64 fidelity as decision-table
// aggregation.
type Rollup struct {
	Key string
	Agg CollectAggregation
	As  string
}

// forEachConfig accumulates ForEachOption settings before defaults are
// applied by NewForEach.
type forEachConfig struct {
	as      string
	output  string
	deps    []string
	rollups []Rollup
}

// ForEachOption configures a ForEach stage built by NewForEach.
type ForEachOption func(*forEachConfig)

// WithForEachAs sets the name each element is bound under in its per-element
// Scope (default "item").
func WithForEachAs(name string) ForEachOption {
	return func(c *forEachConfig) { c.as = name }
}

// WithForEachOutput sets the key, namespaced under the stage's own name, that
// the per-element results list is written to (default "items", written at
// "<stage name>.items").
func WithForEachOutput(key string) ForEachOption {
	return func(c *forEachConfig) { c.output = key }
}

// WithForEachDependsOn declares the stages this stage depends on.
func WithForEachDependsOn(deps ...string) ForEachOption {
	return func(c *forEachConfig) { c.deps = deps }
}

// WithRollups configures roll-up aggregations that reduce a per-element
// output key across all elements into a header value at "<stage name>.<As>".
func WithRollups(r ...Rollup) ForEachOption {
	return func(c *forEachConfig) { c.rollups = r }
}

// NewForEach compiles a ForEach stage. collection is the Scope path to a
// []any; inner is the sub-pipeline run once per element. name must be
// non-empty (ErrEmptyStageName) and inner and collection must be provided;
// opts set the element binding (default "item"), output key (default
// "items"), and depends-on.
func NewForEach(name, collection string, inner *Pipeline, opts ...ForEachOption) (*ForEach, error) {
	if name == "" {
		return nil, &StageError{Stage: name, Type: TypeForEach, Cause: ErrEmptyStageName}
	}
	if inner == nil {
		return nil, &StageError{Stage: name, Type: TypeForEach, Cause: errors.New("foreach: inner pipeline must not be nil")}
	}
	if collection == "" {
		return nil, &StageError{Stage: name, Type: TypeForEach, Cause: errors.New("foreach: collection path must not be empty")}
	}

	cfg := &forEachConfig{as: "item", output: "items"}
	for _, o := range opts {
		o(cfg)
	}
	for _, r := range cfg.rollups {
		if r.Key == "" || r.As == "" {
			return nil, &StageError{Stage: name, Type: TypeForEach, Cause: ErrForEachEmptyRollup}
		}
	}

	return &ForEach{
		name:       name,
		collection: collection,
		as:         cfg.as,
		output:     cfg.output,
		deps:       cfg.deps,
		inner:      inner,
		rollups:    cfg.rollups,
	}, nil
}

// Name returns the stage's name; the per-element results list is written
// under "<name>.<output>".
func (f *ForEach) Name() string { return f.name }

// Type returns TypeForEach.
func (f *ForEach) Type() string { return TypeForEach }

// DependsOn returns the names of the stages this stage depends on.
func (f *ForEach) DependsOn() []string { return f.deps }

// Execute resolves the collection at f.collection from sc and, for each
// element in order, runs the inner pipeline against a fresh per-element Scope
// seeded from sc's Snapshot plus the element bound under f.as. The outer sc is
// never mutated during iteration. Each element's resulting Snapshot is
// appended, in order, to a list written to sc at "<name>.<output>". Any rules
// the inner pipeline fires for element i are recorded on sc under the
// composite stage key "<name>[i]" (queryable via sc.FiringRulesFor), giving
// each element its own provenance without disturbing the inner stage's own
// name. When the outer sc tracks provenance, element i's full derivation graph
// is also merged onto sc under the "<name>[i]." path prefix, so sc.Lineage /
// sc.Explain answer per-element lineage (e.g. "<name>[i].<inner output>");
// nothing is recorded per-element when provenance is off. After every element
// runs, each configured Rollup reduces its Key
// across all elements' results (skipping elements missing Key) and writes the
// result to sc at "<name>.<As>"; an empty (or all-absent) collection writes 0
// for AggregateCount and an empty list for AggregateList, and leaves the
// key absent for AggregateSum/Min/Max (folding empty is undefined). ctx is
// checked before the loop and before each element; a canceled context stops
// iteration without writing the output. A non-list collection value is
// ErrForEachNotList; a missing collection path is ErrForEachNoCollection; a
// per-element inner-pipeline error is returned naming the element index. All
// errors are returned as a *StageError.
func (f *ForEach) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return f.stageErr(err)
	}

	raw, ok := sc.Get(f.collection)
	if !ok {
		return f.stageErr(ErrForEachNoCollection)
	}
	list, ok := raw.([]any)
	if !ok {
		return f.stageErr(fmt.Errorf("%w: %s (%T)", ErrForEachNotList, f.collection, raw))
	}

	var provOpts []ScopeOption
	if sc.TracksProvenance() {
		provOpts = append(provOpts, WithProvenance())
	}

	items := make([]any, 0, len(list))
	for i, el := range list {
		if err := ctx.Err(); err != nil {
			return f.stageErr(fmt.Errorf("element %d: %w", i, err))
		}

		seed := sc.Snapshot()
		seed[f.as] = el
		esc := NewScope(seed, provOpts...)

		if err := f.inner.Run(ctx, esc); err != nil {
			return f.stageErr(fmt.Errorf("element %d: %w", i, err))
		}

		if elemFirings := esc.FiringRules(); len(elemFirings) > 0 {
			sc.recordFirings(fmt.Sprintf("%s[%d]", f.name, i), elemFirings)
		}
		if sc.TracksProvenance() {
			sc.recordElementDerivations(fmt.Sprintf("%s[%d]", f.name, i), esc.Derivations())
		}
		items = append(items, esc.Snapshot())
	}

	for _, r := range f.rollups {
		if err := f.applyRollup(sc, r, items); err != nil {
			return err
		}
	}

	if err := sc.Set(f.name+"."+f.output, items); err != nil {
		return f.stageErr(err)
	}
	return nil
}

// applyRollup reduces r.Key across items (each a map[string]any produced by
// an element's inner-pipeline Snapshot) per r.Agg, writing the result to sc at
// "<name>.<As>". Elements missing r.Key are skipped. When no element supplies
// a value, AggregateCount writes 0 and AggregateList writes an empty list;
// AggregateSum/Min/Max leave the key absent, since folding an empty
// collection is undefined.
func (f *ForEach) applyRollup(sc *Scope, r Rollup, items []any) error {
	vals := make([]any, 0, len(items))
	for _, item := range items {
		m := item.(map[string]any) //nolint:errcheck // items are always esc.Snapshot() results
		if v, ok := lookupPath(m, r.Key); ok {
			vals = append(vals, v)
		}
	}

	if len(vals) == 0 {
		switch r.Agg {
		case AggregateCount:
			return f.setRollup(sc, r.As, 0)
		case AggregateList:
			return f.setRollup(sc, r.As, []any{})
		default: // AggregateSum, AggregateMin, AggregateMax: no defined result over empty
			return nil
		}
	}

	reduced, err := aggregate(r.Agg, vals)
	if err != nil {
		return f.stageErr(err)
	}
	return f.setRollup(sc, r.As, reduced)
}

// setRollup writes v to sc at "<name>.<as>", wrapping any Scope error as a
// *StageError.
func (f *ForEach) setRollup(sc *Scope, as string, v any) error {
	if err := sc.Set(f.name+"."+as, v); err != nil {
		return f.stageErr(err)
	}
	return nil
}

// stageErr wraps cause as a *StageError naming this stage.
func (f *ForEach) stageErr(cause error) error {
	return &StageError{Stage: f.name, Type: TypeForEach, Cause: cause}
}
