package rlng

import (
	"context"

	"github.com/kartaladev/rlng/pipe"
)

// TypedEngine evaluates a typed input I against a compiled pipeline and maps the
// result into a typed R. It is safe for concurrent use after construction.
type TypedEngine[I any, R any] struct {
	pipeline  *pipe.Pipeline
	mapper    *Mapper[R]
	scopeOpts []pipe.ScopeOption
}

// NewTypedEngine constructs a TypedEngine from a compiled pipeline and a result
// mapper. It returns ErrNilPipeline or ErrNilMapper if either required argument
// is nil, failing fast at construction rather than on the first Evaluate.
func NewTypedEngine[I any, R any](pipeline *pipe.Pipeline, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	if pipeline == nil {
		return nil, ErrNilPipeline
	}
	if mapper == nil {
		return nil, ErrNilMapper
	}
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.concMode != concUnset {
		return nil, ErrConcurrencyRequiresConfig
	}
	return &TypedEngine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}, nil
}

// Evaluate seeds a Scope from input, runs the pipeline, and maps the final Scope
// into R. Pipeline/stage errors pass through unwrapped; mapping errors are a
// *MappingError; an input that cannot be flattened is a wrapped error.
//
// When input is a map[string]any it seeds the Scope directly: the top level is
// copied and nested maps are deep-copied (as in pipe.NewScope), so the caller's
// map is never mutated. A struct input is flattened into fresh maps.
func (e *TypedEngine[I, R]) Evaluate(ctx context.Context, input I) (R, error) {
	var zero R

	seed, err := flatten(input)
	if err != nil {
		return zero, err
	}

	sc := pipe.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return zero, err
	}
	return e.mapper.Map(sc.Snapshot())
}
