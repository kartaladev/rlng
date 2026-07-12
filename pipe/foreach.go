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
}

// forEachConfig accumulates ForEachOption settings before defaults are
// applied by NewForEach.
type forEachConfig struct {
	as     string
	output string
	deps   []string
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

	return &ForEach{
		name:       name,
		collection: collection,
		as:         cfg.as,
		output:     cfg.output,
		deps:       cfg.deps,
		inner:      inner,
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
// appended, in order, to a list written to sc at "<name>.<output>". ctx is
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
		items = append(items, esc.Snapshot())
	}

	if err := sc.Set(f.name+"."+f.output, items); err != nil {
		return f.stageErr(err)
	}
	return nil
}

// stageErr wraps cause as a *StageError naming this stage.
func (f *ForEach) stageErr(cause error) error {
	return &StageError{Stage: f.name, Type: TypeForEach, Cause: cause}
}
