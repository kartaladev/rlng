package rlng

import (
	"context"

	"github.com/kartaladev/rlng/config"
)

// NewFromProvider builds an Engine directly from a config source: it parses the
// provider, compiles the definition with the default build options, and wraps
// the resulting pipeline. It is the one-call form of
// config.Parse -> PipelineDef.Build -> New. opts configure the per-Evaluate
// Scope (e.g. WithScopeOptions(pipe.WithProvenance())). A parse or build failure
// is returned unwrapped (a *config.ConfigError). For build-time options
// (strict schema, lint-as-error, version override) use the explicit
// config.Parse -> Build -> New path.
func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	pipeline, err := def.Build()
	if err != nil {
		return nil, err
	}
	return New(pipeline, opts...)
}

// NewFromYAML builds an Engine from an in-memory YAML ruleset. It is shorthand
// for NewFromProvider(ctx, config.FromYAMLString(yaml), opts...); for JSON, a
// file, or a URL, call NewFromProvider with the matching config.From* provider.
func NewFromYAML(ctx context.Context, yaml string, opts ...Option) (*Engine, error) {
	return NewFromProvider(ctx, config.FromYAMLString(yaml), opts...)
}

// NewTypedFromProvider builds a TypedEngine[I, R] directly from a config source:
// it parses the provider, compiles with the default build options, and wraps the
// pipeline with mapper. It is the one-call form of
// config.Parse -> PipelineDef.Build -> NewTypedEngine. A parse or build failure
// is returned unwrapped; a nil mapper returns ErrNilMapper. Build-time options
// require the explicit config.Parse -> Build -> NewTypedEngine path.
func NewTypedFromProvider[I, R any](ctx context.Context, p config.Provider, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	pipeline, err := def.Build()
	if err != nil {
		return nil, err
	}
	return NewTypedEngine[I, R](pipeline, mapper, opts...)
}

// NewTypedFromYAML builds a TypedEngine[I, R] from an in-memory YAML ruleset. It
// is shorthand for NewTypedFromProvider(ctx, config.FromYAMLString(yaml), mapper,
// opts...); for JSON, a file, or a URL, call NewTypedFromProvider with the
// matching config.From* provider.
func NewTypedFromYAML[I, R any](ctx context.Context, yaml string, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	return NewTypedFromProvider[I, R](ctx, config.FromYAMLString(yaml), mapper, opts...)
}
