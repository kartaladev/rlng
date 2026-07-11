package rlng

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bareInput struct {
	Price int `mapstructure:"price"`
	Qty   int `mapstructure:"qty"`
}

func buildBareEngine(tb testing.TB, opts ...Option) *BareEngine {
	tb.Helper()
	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := stage.NewPipeline(base, taxed)
	require.NoError(tb, err)
	return NewBareEngine(p, opts...)
}

func TestBareEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *BareEngine
		input  any
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, out map[string]any, err error)
	}

	cases := []testCase{
		{
			name:   "struct input returns accumulated map",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  bareInput{Price: 10, Qty: 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 20, out["base"])
				assert.InDelta(t, 22.0, out["taxed"], 1e-9)
			},
		},
		{
			name:   "map input passes through",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 3},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, out["base"])
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *BareEngine {
				boom, err := stage.NewSingleExpr("x", "qty % 0")
				require.NoError(tb, err)
				p, err := stage.NewPipeline(boom)
				require.NoError(tb, err)
				return NewBareEngine(p)
			},
			input: map[string]any{"qty": 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 2},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, out map[string]any, err error) {
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
			out, err := e.Evaluate(ctx, tc.input)
			tc.assert(t, out, err)
		})
	}
}

func TestBareEngineEvaluateScope(t *testing.T) {
	t.Parallel()

	e := buildBareEngine(t)
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)

	_, ok := sc.Duration()
	assert.True(t, ok, "EvaluateScope exposes timing")
	v, err := sc.GetFloat64("taxed")
	require.NoError(t, err)
	assert.InDelta(t, 22.0, v, 1e-9)
}
