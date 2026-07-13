package rlng

import (
	"context"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// parseAndBuild parses p and builds the pipeline with cfg's build options. It is
// the shared body of NewFromProvider/NewTypedFromProvider. Parse and build errors
// are returned unwrapped (a *config.ConfigError).
func parseAndBuild(ctx context.Context, p config.Provider, cfg *engineConfig) (*pipe.Pipeline, error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	return def.Build(cfg.buildOpts...)
}

// NewFromProvider builds an Engine directly from a config source: it parses the
// provider, compiles the definition with the default build options, and wraps
// the resulting pipeline. It is the one-call form of
// config.Parse -> PipelineDef.Build -> New. opts configure the per-Evaluate
// Scope (e.g. WithScopeOptions(pipe.WithProvenance())). A parse or build failure
// is returned unwrapped (a *config.ConfigError). For build-time options
// (strict schema, lint-as-error, version override) use the explicit
// config.Parse -> Build -> New path.
func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error) {
	cfg := newEngineConfig(opts)
	pipeline, err := parseAndBuild(ctx, p, cfg)
	if err != nil {
		return nil, err
	}
	// Construct directly (not via New): the concurrency Option was consumed into
	// the build above, so passing it to New would trip ErrConcurrencyRequiresConfig.
	return &Engine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}, nil
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
	if mapper == nil {
		return nil, ErrNilMapper
	}
	cfg := newEngineConfig(opts)
	pipeline, err := parseAndBuild(ctx, p, cfg)
	if err != nil {
		return nil, err
	}
	// Construct directly (not via NewTypedEngine): concurrency was consumed into
	// the build, so it would otherwise trip ErrConcurrencyRequiresConfig.
	return &TypedEngine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}, nil
}

// NewTypedFromYAML builds a TypedEngine[I, R] from an in-memory YAML ruleset. It
// is shorthand for NewTypedFromProvider(ctx, config.FromYAMLString(yaml), mapper,
// opts...); for JSON, a file, or a URL, call NewTypedFromProvider with the
// matching config.From* provider.
func NewTypedFromYAML[I, R any](ctx context.Context, yaml string, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	return NewTypedFromProvider[I, R](ctx, config.FromYAMLString(yaml), mapper, opts...)
}
