package rlng_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type engineInput struct {
	Price int `mapstructure:"price"`
	Qty   int `mapstructure:"qty"`
}

func buildEngine(tb testing.TB, opts ...rlng.Option) *rlng.Engine {
	tb.Helper()
	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := pipe.NewPipeline(base, taxed)
	require.NoError(tb, err)
	e, err := rlng.New(p, opts...)
	require.NoError(tb, err)
	return e
}

func TestEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *rlng.Engine
		input  any
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, out map[string]any, err error)
	}

	cases := []testCase{
		{
			name:   "struct input returns accumulated map",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  engineInput{Price: 10, Qty: 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 20, out["base"])
				assert.InDelta(t, 22.0, out["taxed"], 1e-9)
			},
		},
		{
			name:   "map input passes through",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 3},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, out["base"])
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *rlng.Engine {
				boom, err := pipe.NewSingleExpr("x", "qty % 0")
				require.NoError(tb, err)
				p, err := pipe.NewPipeline(boom)
				require.NoError(tb, err)
				e, err := rlng.New(p)
				require.NoError(tb, err)
				return e
			},
			input: map[string]any{"qty": 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
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
		{
			name:   "non-flattenable input is an error",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  42, // a bare int cannot be decoded into a map[string]any seed
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
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

func TestEngineEvaluateScope(t *testing.T) {
	t.Parallel()

	e := buildEngine(t)
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)

	_, ok := sc.Duration()
	assert.True(t, ok, "EvaluateScope exposes timing")
	v, err := sc.GetFloat64("taxed")
	require.NoError(t, err)
	assert.InDelta(t, 22.0, v, 1e-9)
}

func TestEngineScopeOptions(t *testing.T) {
	t.Parallel()

	// WithScopeOptions must flow through New to the per-Evaluate Scope.
	e := buildEngine(t, rlng.WithScopeOptions(pipe.WithProvenance()))
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)
	assert.True(t, sc.TracksProvenance())
}

// TestEngineConcurrentEvaluateMapInputIsolation reproduces the aliasing/data-race
// (a stage writing a nested path of a shared map[string]any input) and confirms
// the deep-copy seed fix: concurrent Evaluate is race-free (under -race) and the
// caller's nested input map is never mutated.
func TestEngineConcurrentEvaluateMapInputIsolation(t *testing.T) {
	t.Parallel()

	s, err := pipe.NewSingleExpr("rate", "price * 2", pipe.WithOutput("cfg.rate"))
	require.NoError(t, err)
	p, err := pipe.NewPipeline(s)
	require.NoError(t, err)
	eng, err := rlng.New(p)
	require.NoError(t, err)

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

// TestNewRejectsNilPipeline covers the fail-fast guard: a nil pipeline is a
// construction-time error, not a deferred nil deref on the first Evaluate.
func TestNewRejectsNilPipeline(t *testing.T) {
	t.Parallel()

	e, err := rlng.New(nil)
	assert.Nil(t, e)
	require.ErrorIs(t, err, rlng.ErrNilPipeline)
}
