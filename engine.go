package rlng

import (
	"context"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/stage"
)

// Engine evaluates a typed input I against a compiled pipeline and maps the
// result into a typed R. It is safe for concurrent use after construction.
type Engine[I any, R any] struct {
	pipeline  *stage.Pipeline
	mapper    *Mapper[R]
	scopeOpts []stage.ScopeOption
}

type engineConfig struct {
	scopeOpts []stage.ScopeOption
}

// Option configures an Engine.
type Option func(*engineConfig)

// WithScopeOptions passes stage.ScopeOption values (e.g. stage.WithStrict) to the
// Scope seeded for each Evaluate.
func WithScopeOptions(opts ...stage.ScopeOption) Option {
	return func(c *engineConfig) { c.scopeOpts = append(c.scopeOpts, opts...) }
}

// New constructs an Engine from a compiled pipeline and a result mapper.
func New[I any, R any](pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option) *Engine[I, R] {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return &Engine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}
}

// Evaluate seeds a Scope from input, runs the pipeline, and maps the final Scope
// into R. Pipeline/stage errors pass through unwrapped; mapping errors are a
// *MappingError; an input that cannot be flattened is a wrapped error.
//
// When input is a map[string]any it seeds the Scope directly: the top level is
// copied, but nested maps are referenced, not deep-copied (as in
// stage.NewScope). A struct input is flattened into fresh maps and never shares
// structure with the caller.
func (e *Engine[I, R]) Evaluate(ctx context.Context, input I) (R, error) {
	var zero R

	seed, err := flatten(input)
	if err != nil {
		return zero, err
	}

	sc := stage.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return zero, err
	}
	return e.mapper.Map(sc.Snapshot())
}

// flatten converts input into a map[string]any Scope seed. A map[string]any is
// used directly; any other value (typically a struct) is decoded via
// mapstructure, preserving field types.
func flatten[I any](input I) (map[string]any, error) {
	if m, ok := any(input).(map[string]any); ok {
		return m, nil
	}
	var m map[string]any
	if err := mapstructure.Decode(input, &m); err != nil {
		return nil, fmt.Errorf("rlng: seed input: %w", err)
	}
	return m, nil
}
