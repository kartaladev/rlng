package rlng

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/pipe"
)

// Engine runs a compiled pipeline against arbitrary input and returns the
// accumulated map[string]any — no result mapping (cf. TypedEngine[I, R]). It is
// safe for concurrent use after construction: each call builds a fresh Scope.
type Engine struct {
	pipeline  *pipe.Pipeline
	scopeOpts []pipe.ScopeOption
}

type engineConfig struct {
	scopeOpts []pipe.ScopeOption
}

// Option configures an Engine or TypedEngine.
type Option func(*engineConfig)

// WithScopeOptions passes pipe.ScopeOption values (e.g. pipe.WithStrict) to the
// Scope seeded for each Evaluate.
func WithScopeOptions(opts ...pipe.ScopeOption) Option {
	return func(c *engineConfig) { c.scopeOpts = append(c.scopeOpts, opts...) }
}

// New constructs an Engine from a compiled pipeline. Options configure the
// per-Evaluate Scope (e.g. WithScopeOptions(pipe.WithProvenance())). It returns
// ErrNilPipeline if pipeline is nil, failing fast at construction rather than
// on the first Evaluate.
func New(pipeline *pipe.Pipeline, opts ...Option) (*Engine, error) {
	if pipeline == nil {
		return nil, ErrNilPipeline
	}
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return &Engine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}, nil
}

// EvaluateScope seeds a Scope from input, runs the pipeline, and returns the
// full Scope — exposing timing, JSON serialization, and provenance. A
// map[string]any input seeds directly; any other value is flattened via
// mapstructure. Pipeline/stage errors pass through unwrapped. An input that
// cannot be flattened into a seed map is returned as a wrapped error.
func (e *Engine) EvaluateScope(ctx context.Context, input any) (*pipe.Scope, error) {
	seed, err := flatten(input)
	if err != nil {
		return nil, err
	}
	sc := pipe.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return nil, err
	}
	return sc, nil
}

// Evaluate runs the pipeline and returns the accumulated map[string]any (the
// Scope snapshot).
func (e *Engine) Evaluate(ctx context.Context, input any) (map[string]any, error) {
	sc, err := e.EvaluateScope(ctx, input)
	if err != nil {
		return nil, err
	}
	return sc.Snapshot(), nil
}

// flatten converts input into a map[string]any Scope seed. A map[string]any is
// used directly; any other value (typically a struct) is decoded via
// mapstructure, preserving field types. A nil pointer or untyped-nil input is
// ErrNilInput (it would otherwise seed an empty Scope and yield a bogus zero
// result); a non-nil empty map is a valid empty seed.
func flatten[I any](input I) (map[string]any, error) {
	if m, ok := any(input).(map[string]any); ok {
		return m, nil
	}
	rv := reflect.ValueOf(input)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return nil, ErrNilInput
	}
	var m map[string]any
	if err := mapstructure.Decode(input, &m); err != nil {
		return nil, fmt.Errorf("rlng: seed input: %w", err)
	}
	return m, nil
}
