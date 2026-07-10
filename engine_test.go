package rlng

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/stage"
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

// buildEngine wires a two-stage pipeline (base = price*qty, taxed = base*1.1)
// and a mapper projecting total = taxed.
func buildEngine(t *testing.T) *Engine[order, quote] {
	t.Helper()
	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	taxed, err := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	require.NoError(t, err)
	p, err := stage.NewPipeline(base, taxed)
	require.NoError(t, err)
	m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
	require.NoError(t, err)
	return New[order, quote](p, m)
}

func TestEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(t *testing.T) *Engine[order, quote]
		input  order
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, q quote, err error)
	}

	cases := []testCase{
		{
			name:   "happy path struct in struct out",
			engine: buildEngine,
			input:  order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 22.0, q.Total, 1e-9)
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(t *testing.T) *Engine[order, quote] {
				// boom uses modulo by zero on a seeded int, failing at eval.
				boom, err := stage.NewSingleExpr("taxed", "qty % 0")
				require.NoError(t, err)
				p, err := stage.NewPipeline(boom)
				require.NoError(t, err)
				m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
				require.NoError(t, err)
				return New[order, quote](p, m)
			},
			input: order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "taxed", se.Stage)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: buildEngine,
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

func TestEngineWithScopeOptions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		opts   []Option
		assert func(t *testing.T, err error)
	}

	// The pipeline's stage writes to "base"; seeding "base" makes a strict Scope
	// conflict on that write, while the default (lenient) Scope overwrites.
	cases := []testCase{
		{
			name: "strict scope surfaces a conflict",
			opts: []Option{WithScopeOptions(stage.WithStrict())},
			assert: func(t *testing.T, err error) {
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
				require.ErrorIs(t, err, stage.ErrPathConflict)
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
			base, err := stage.NewSingleExpr("base", "1")
			require.NoError(t, err)
			p, err := stage.NewPipeline(base)
			require.NoError(t, err)
			m, err := NewMapper[map[string]any](MappingTemplate{})
			require.NoError(t, err)
			e := New[map[string]any, map[string]any](p, m, tc.opts...)

			_, err = e.Evaluate(t.Context(), map[string]any{"base": 99})
			tc.assert(t, err)
		})
	}
}

// TestEngineEvaluateMapInput covers a map[string]any input (I) that bypasses
// mapstructure flattening — a structurally different I, so a separate test.
func TestEngineEvaluateMapInput(t *testing.T) {
	t.Parallel()

	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := stage.NewPipeline(base)
	require.NoError(t, err)
	m, err := NewMapper[map[string]any](MappingTemplate{"out": "base"})
	require.NoError(t, err)
	e := New[map[string]any, map[string]any](p, m)

	out, err := e.Evaluate(t.Context(), map[string]any{"price": 10.0, "qty": 2.0})
	require.NoError(t, err)
	assert.InDelta(t, 20.0, out["out"], 1e-9)
}

// TestEngineEvaluateFlattenError covers an input that mapstructure cannot flatten.
func TestEngineEvaluateFlattenError(t *testing.T) {
	t.Parallel()

	p, err := stage.NewPipeline()
	require.NoError(t, err)
	m, err := NewMapper[map[string]any](MappingTemplate{})
	require.NoError(t, err)
	e := New[int, map[string]any](p, m)

	_, err = e.Evaluate(t.Context(), 42)
	require.Error(t, err)
}
