package rlng

import (
	"context"
	"sync"
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
// and a mapper projecting total = taxed, with any Options applied. It accepts a
// testing.TB so both tests and benchmarks can share it.
func buildEngine(tb testing.TB, opts ...Option) *Engine[order, quote] {
	tb.Helper()
	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := stage.NewPipeline(base, taxed)
	require.NoError(tb, err)
	m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
	require.NoError(tb, err)
	return New[order, quote](p, m, opts...)
}

func TestEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *Engine[order, quote]
		input  order
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, q quote, err error)
	}

	cases := []testCase{
		{
			name:   "happy path struct in struct out",
			engine: func(tb testing.TB) *Engine[order, quote] { return buildEngine(tb) },
			input:  order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 22.0, q.Total, 1e-9)
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *Engine[order, quote] {
				// boom uses modulo by zero on a seeded int, failing at eval.
				boom, err := stage.NewSingleExpr("taxed", "qty % 0")
				require.NoError(tb, err)
				p, err := stage.NewPipeline(boom)
				require.NoError(tb, err)
				m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
				require.NoError(tb, err)
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
			engine: func(tb testing.TB) *Engine[order, quote] { return buildEngine(tb) },
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

// TestEngineEvaluateNilInput covers a nil pointer input, which must be a typed
// error rather than a bogus zero result over an empty scope.
func TestEngineEvaluateNilInput(t *testing.T) {
	t.Parallel()

	base, err := stage.NewSingleExpr("base", "1")
	require.NoError(t, err)
	p, err := stage.NewPipeline(base)
	require.NoError(t, err)
	m, err := NewMapper[quote](MappingTemplate{"total": "base"})
	require.NoError(t, err)
	e := New[*order, quote](p, m)

	_, err = e.Evaluate(t.Context(), nil)
	require.ErrorIs(t, err, errNilInput)
}

// TestEngineConcurrentEvaluateMapInputIsolation reproduces the aliasing/data-race
// (a stage writing a nested path of a shared map[string]any input) and confirms
// the deep-copy seed fix: concurrent Evaluate is race-free (under -race) and the
// caller's nested input map is never mutated.
func TestEngineConcurrentEvaluateMapInputIsolation(t *testing.T) {
	t.Parallel()

	s, err := stage.NewSingleExpr("rate", "price * 2", stage.WithOutput("cfg.rate"))
	require.NoError(t, err)
	p, err := stage.NewPipeline(s)
	require.NoError(t, err)
	eng := NewBareEngine(p)

	input := map[string]any{"price": 10, "cfg": map[string]any{"existing": 1}}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := eng.Evaluate(context.Background(), input)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	cfg := input["cfg"].(map[string]any)
	_, rateWritten := cfg["rate"]
	assert.False(t, rateWritten, "engine must not mutate the caller's nested input map")
}
