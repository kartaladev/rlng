package rlng_test

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type order struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type quote struct {
	Total float64 `mapstructure:"total"`
}

// buildTypedEngine wires a two-stage pipeline (base = price*qty, taxed = base*1.1)
// and a mapper projecting total = taxed, with any Options applied. It accepts a
// testing.TB so both tests and benchmarks can share it.
func buildTypedEngine(tb testing.TB, opts ...rlng.Option) *rlng.TypedEngine[order, quote] {
	tb.Helper()
	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := pipe.NewPipeline([]pipe.Stage{base, taxed})
	require.NoError(tb, err)
	m, err := rlng.NewMapper[quote](rlng.MappingTemplate{"total": "taxed"})
	require.NoError(tb, err)
	e, err := rlng.NewTypedEngine[order, quote](p, m, opts...)
	require.NoError(tb, err)
	return e
}

func TestTypedEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *rlng.TypedEngine[order, quote]
		input  order
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, q quote, err error)
	}

	cases := []testCase{
		{
			name:   "happy path struct in struct out",
			engine: func(tb testing.TB) *rlng.TypedEngine[order, quote] { return buildTypedEngine(tb) },
			input:  order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 22.0, q.Total, 1e-9)
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *rlng.TypedEngine[order, quote] {
				// boom uses modulo by zero on a seeded int, failing at eval.
				boom, err := pipe.NewSingleExpr("taxed", "qty % 0")
				require.NoError(tb, err)
				p, err := pipe.NewPipeline([]pipe.Stage{boom})
				require.NoError(tb, err)
				m, err := rlng.NewMapper[quote](rlng.MappingTemplate{"total": "taxed"})
				require.NoError(tb, err)
				e, err := rlng.NewTypedEngine[order, quote](p, m)
				require.NoError(tb, err)
				return e
			},
			input: order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "taxed", se.Stage)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: func(tb testing.TB) *rlng.TypedEngine[order, quote] { return buildTypedEngine(tb) },
			input:  order{Price: 10, Qty: 2},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, q quote, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := tc.engine(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			q, err := e.Evaluate(ctx, tc.input)
			tc.assert(t, q, err)
		})
	}
}

func TestTypedEngineWithScopeOptions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		opts   []rlng.Option
		assert func(t *testing.T, err error)
	}

	// The pipeline's stage writes to "base"; seeding "base" makes a strict Scope
	// conflict on that write, while the default (lenient) Scope overwrites.
	cases := []testCase{
		{
			name: "strict scope surfaces a conflict",
			opts: []rlng.Option{rlng.WithScopeOptions(pipe.WithStrict())},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				require.ErrorIs(t, err, pipe.ErrPathConflict)
			},
		},
		{
			name: "lenient scope overwrites",
			opts: nil,
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, err := pipe.NewSingleExpr("base", "1")
			require.NoError(t, err)
			p, err := pipe.NewPipeline([]pipe.Stage{base})
			require.NoError(t, err)
			m, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{})
			require.NoError(t, err)
			e, err := rlng.NewTypedEngine[map[string]any, map[string]any](p, m, tc.opts...)
			require.NoError(t, err)

			_, err = e.Evaluate(t.Context(), map[string]any{"base": 99})
			tc.assert(t, err)
		})
	}
}

// TestEngineEvaluateMapInput covers a map[string]any input (I) that bypasses
// mapstructure flattening — a structurally different I, so a separate test.
func TestTypedEngineEvaluateMapInput(t *testing.T) {
	t.Parallel()

	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := pipe.NewPipeline([]pipe.Stage{base})
	require.NoError(t, err)
	m, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{"out": "base"})
	require.NoError(t, err)
	e, err := rlng.NewTypedEngine[map[string]any, map[string]any](p, m)
	require.NoError(t, err)

	out, err := e.Evaluate(t.Context(), map[string]any{"price": 10.0, "qty": 2.0})
	require.NoError(t, err)
	assert.InDelta(t, 20.0, out["out"], 1e-9)
}

// TestEngineEvaluateFlattenError covers an input that mapstructure cannot flatten.
func TestTypedEngineEvaluateFlattenError(t *testing.T) {
	t.Parallel()

	p, err := pipe.NewPipeline(nil)
	require.NoError(t, err)
	m, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{})
	require.NoError(t, err)
	e, err := rlng.NewTypedEngine[int, map[string]any](p, m)
	require.NoError(t, err)

	_, err = e.Evaluate(t.Context(), 42)
	require.Error(t, err)
}

// TestEngineEvaluateNilInput covers a nil pointer input, which must be a typed
// error rather than a bogus zero result over an empty scope.
func TestTypedEngineEvaluateNilInput(t *testing.T) {
	t.Parallel()

	base, err := pipe.NewSingleExpr("base", "1")
	require.NoError(t, err)
	p, err := pipe.NewPipeline([]pipe.Stage{base})
	require.NoError(t, err)
	m, err := rlng.NewMapper[quote](rlng.MappingTemplate{"total": "base"})
	require.NoError(t, err)
	e, err := rlng.NewTypedEngine[*order, quote](p, m)
	require.NoError(t, err)

	_, err = e.Evaluate(t.Context(), nil)
	require.ErrorIs(t, err, rlng.ErrNilInput)
}

// TestNewTypedEngineRejectsNilArgs covers the fail-fast guards for the two
// required arguments: a nil pipeline or a nil mapper is a construction-time
// error rather than a deferred nil deref.
func TestNewTypedEngineRejectsNilArgs(t *testing.T) {
	t.Parallel()

	p, err := pipe.NewPipeline(nil)
	require.NoError(t, err)
	m, err := rlng.NewMapper[quote](rlng.MappingTemplate{})
	require.NoError(t, err)

	type testCase struct {
		name     string
		pipeline *pipe.Pipeline
		mapper   *rlng.Mapper[quote]
		assert   func(t *testing.T, e *rlng.TypedEngine[order, quote], err error)
	}

	cases := []testCase{
		{
			name:     "nil pipeline",
			pipeline: nil,
			mapper:   m,
			assert: func(t *testing.T, e *rlng.TypedEngine[order, quote], err error) {
				assert.Nil(t, e)
				require.ErrorIs(t, err, rlng.ErrNilPipeline)
			},
		},
		{
			name:     "nil mapper",
			pipeline: p,
			mapper:   nil,
			assert: func(t *testing.T, e *rlng.TypedEngine[order, quote], err error) {
				assert.Nil(t, e)
				require.ErrorIs(t, err, rlng.ErrNilMapper)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := rlng.NewTypedEngine[order, quote](tc.pipeline, tc.mapper)
			tc.assert(t, e, err)
		})
	}
}
