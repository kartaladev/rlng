package rlng

import (
	"context"

	"github.com/kartaladev/rlng/stage"
)

// BareEngine runs a compiled pipeline against arbitrary input and returns the
// accumulated map[string]any — no result mapping (cf. Engine[I, R]). It is safe
// for concurrent use after construction: each call builds a fresh Scope.
type BareEngine struct {
	pipeline  *stage.Pipeline
	scopeOpts []stage.ScopeOption
}

// NewBareEngine constructs a BareEngine from a compiled pipeline. Options
// configure the per-Evaluate Scope (e.g. WithScopeOptions(stage.WithProvenance())).
func NewBareEngine(pipeline *stage.Pipeline, opts ...Option) *BareEngine {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return &BareEngine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}
}

// EvaluateScope seeds a Scope from input, runs the pipeline, and returns the
// full Scope — exposing timing, JSON serialization, and provenance. A
// map[string]any input seeds directly; any other value is flattened via
// mapstructure. Pipeline/stage errors pass through unwrapped. An input that
// cannot be flattened into a seed map is returned as a wrapped error.
func (e *BareEngine) EvaluateScope(ctx context.Context, input any) (*stage.Scope, error) {
	seed, err := flatten(input)
	if err != nil {
		return nil, err
	}
	sc := stage.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return nil, err
	}
	return sc, nil
}

// Evaluate runs the pipeline and returns the accumulated map[string]any (the
// Scope snapshot).
func (e *BareEngine) Evaluate(ctx context.Context, input any) (map[string]any, error) {
	sc, err := e.EvaluateScope(ctx, input)
	if err != nil {
		return nil, err
	}
	return sc.Snapshot(), nil
}
