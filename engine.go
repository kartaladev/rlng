package rlng

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
)

// Engine runs a compiled pipeline against arbitrary input and returns the
// accumulated map[string]any — no result mapping (cf. TypedEngine[I, R]). It is
// safe for concurrent use after construction: each call builds a fresh Scope.
type Engine struct {
	pipeline  *pipe.Pipeline
	scopeOpts []pipe.ScopeOption
}

// concurrencyMode records which concurrency Option (if any) was set, so a
// WithMaxParallel(0) request is distinguishable from "unset" (both would be 0 as
// a bare int) and can be rejected by the config path.
type concurrencyMode int8

const (
	concUnset concurrencyMode = iota
	concUnbounded
	concBounded
)

type engineConfig struct {
	scopeOpts []pipe.ScopeOption
	concMode  concurrencyMode
	concN     int // requested cap when concMode == concBounded
}

// Option configures an Engine or TypedEngine.
type Option func(*engineConfig)

// WithScopeOptions passes pipe.ScopeOption values (e.g. pipe.WithStrict) to the
// Scope seeded for each Evaluate.
func WithScopeOptions(opts ...pipe.ScopeOption) Option {
	return func(c *engineConfig) { c.scopeOpts = append(c.scopeOpts, opts...) }
}

// WithConcurrency, when passed to NewFromYAML/NewFromProvider (or their typed
// forms), builds the pipeline to run independent stages concurrently, unbounded.
// Passing it to New/NewTypedEngine, which wrap an already-built pipeline, returns
// ErrConcurrencyRequiresConfig — set concurrency on the pipeline via
// pipe.WithConcurrency instead.
func WithConcurrency() Option {
	return func(c *engineConfig) { c.concMode = concUnbounded }
}

// WithMaxParallel is like WithConcurrency but caps concurrency at n. An n < 1
// surfaces as a wrapped *pipe.InvalidMaxParallelError from the config
// constructor.
func WithMaxParallel(n int) Option {
	return func(c *engineConfig) { c.concMode = concBounded; c.concN = n }
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
	if cfg.concMode != concUnset {
		return nil, ErrConcurrencyRequiresConfig
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
		return m, nil // map seed preserves decimals untouched
	}
	rv := reflect.ValueOf(input)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return nil, ErrNilInput
	}
	var m map[string]any
	if err := mapstructure.Decode(input, &m); err != nil {
		return nil, fmt.Errorf("rlng: seed input: %w", err)
	}
	restoreDecimals(rv, m)
	return m, nil
}

// decimalType is decimal.Decimal's reflect.Type, used to detect struct fields
// that restoreDecimals must rewrite back into the flattened seed map.
var decimalType = reflect.TypeOf(decimal.Decimal{})

// restoreDecimals rewrites decimal.Decimal struct fields back into m, which
// mapstructure.Decode decomposes into empty maps: decimal.Decimal has no
// exported fields, and DecodeHooks do not fire for the struct->map (seed)
// direction, only for map->struct decoding (see the Mapper's decimalNarrowHook
// in mapper.go). Recurses into nested struct fields and their corresponding
// nested maps in m — including a *pointer*-typed nested struct field: when
// the pointer is non-nil, mapstructure.Decode dereferences it and decomposes
// the pointee into a nested map[string]any (as long as the pointee struct has
// at least one mapstructure-tagged field), and this function's own leading
// pointer-dereference loop then walks it exactly like a plain nested struct
// field. Only a nil pointer field is left un-decomposed (mapstructure keeps
// the raw nil pointer in m rather than a map), so there is nothing to
// recurse into for that field — not a limitation, since a nil pointer has no
// decimal to restore.
func restoreDecimals(rv reflect.Value, m map[string]any) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		key := mapstructureKey(f)
		if f.Type == decimalType {
			m[key] = rv.Field(i).Interface()
			continue
		}
		if child, ok := m[key].(map[string]any); ok {
			restoreDecimals(rv.Field(i), child)
		}
	}
}

// mapstructureKey returns the map key mapstructure uses for field f: the
// first comma-separated segment of the `mapstructure` tag if present, else
// the field name.
func mapstructureKey(f reflect.StructField) string {
	tag := f.Tag.Get("mapstructure")
	if tag == "" {
		return f.Name
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	if tag == "" {
		return f.Name
	}
	return tag
}
