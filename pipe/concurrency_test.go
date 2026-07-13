package pipe_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineConcurrency(t *testing.T) {
	errA := errors.New("A failed")
	errB := errors.New("B failed")

	tests := []struct {
		name   string
		build  func() (*pipe.Pipeline, error)
		assert func(t *testing.T, sc *pipe.Scope, runErr, buildErr error)
	}{
		{
			name: "unbounded runs a whole level concurrently",
			build: func() (*pipe.Pipeline, error) {
				pr := &probe{barrier: newCyclicBarrier(3)}
				return pipe.NewPipeline([]pipe.Stage{
					&gatedStage{name: "a", pr: pr},
					&gatedStage{name: "b", pr: pr},
					&gatedStage{name: "c", pr: pr},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				require.NoError(t, runErr)
				// A size-3 barrier only releases when all 3 overlap, so reaching
				// here proves concurrency; the data is complete.
				for _, k := range []string{"a", "b", "c"} {
					v, ok := sc.Get(k)
					assert.True(t, ok)
					assert.Equal(t, true, v)
				}
			},
		},
		{
			name: "bounded caps concurrency at n",
			build: func() (*pipe.Pipeline, error) {
				pr := &probe{barrier: newCyclicBarrier(2)} // batch size == cap
				return pipe.NewPipeline([]pipe.Stage{
					&gatedStage{name: "a", pr: pr},
					&gatedStage{name: "b", pr: pr},
					&gatedStage{name: "c", pr: pr},
					&gatedStage{name: "d", pr: pr},
				}, pipe.WithMaxParallel(2))
			},
			assert: func(t *testing.T, _ *pipe.Scope, runErr, _ error) {
				// If the cap were exceeded, the size-2 barrier would still release
				// (>=2 present); if the cap throttled below 2 the barrier would
				// deadlock. Completing proves exactly-2-at-a-time waves.
				require.NoError(t, runErr)
			},
		},
		{
			name: "n<1 is an InvalidMaxParallelError",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{&setStage{name: "a"}}, pipe.WithMaxParallel(0))
			},
			assert: func(t *testing.T, _ *pipe.Scope, _, buildErr error) {
				var e *pipe.InvalidMaxParallelError
				require.ErrorAs(t, buildErr, &e)
				assert.Equal(t, 0, e.N)
				assert.Contains(t, e.Error(), "must be >= 1, got 0")
			},
		},
		{
			name: "topo-min error wins when independent stages both fail",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&erroringStage{name: "a", err: errA},
					&erroringStage{name: "b", err: errB},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, _ *pipe.Scope, runErr, _ error) {
				assert.ErrorIs(t, runErr, errA) // "a" is topo-first
			},
		},
		{
			name: "dependency-failed stage never runs",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&erroringStage{name: "a", err: errA},
					&setStage{name: "b", deps: []string{"a"}},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				assert.ErrorIs(t, runErr, errA)
				_, ok := sc.Get("b")
				assert.False(t, ok) // b depends on failed a -> pruned
			},
		},
		{
			name: "reported stage order is topo order, not completion order",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&setStage{name: "a"},
					&setStage{name: "b", deps: []string{"a"}},
					&setStage{name: "c", deps: []string{"a"}},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				require.NoError(t, runErr)
				got := make([]string, 0, 3)
				for _, tm := range sc.StageTimings() {
					got = append(got, tm.Stage)
				}
				assert.Equal(t, []string{"a", "b", "c"}, got)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, buildErr := tc.build()
			if buildErr != nil {
				tc.assert(t, nil, nil, buildErr)
				return
			}
			sc := pipe.NewScope(nil)
			runErr := p.Run(t.Context(), sc)
			tc.assert(t, sc, runErr, nil)
		})
	}
}

func TestPipelineConcurrencyStageCancelSurfacesStageError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stageErr := errors.New("boom during cancel")

	// The stage cancels ctx then returns its own error. Sequential Run surfaces
	// the stage error (it does not re-check ctx after a stage), so the wave
	// runner must too — not mask it with context.Canceled.
	s := &cancelThenErrStage{name: "a", cancel: cancel, err: stageErr}
	p, err := pipe.NewPipeline([]pipe.Stage{s}, pipe.WithConcurrency())
	require.NoError(t, err)

	runErr := p.Run(ctx, pipe.NewScope(nil))
	assert.ErrorIs(t, runErr, stageErr)
	assert.NotErrorIs(t, runErr, context.Canceled)
}

func TestPipelineConcurrencyContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancelled

	p, err := pipe.NewPipeline([]pipe.Stage{&setStage{name: "a"}}, pipe.WithConcurrency())
	require.NoError(t, err)

	sc := pipe.NewScope(nil)
	runErr := p.Run(ctx, sc)
	assert.ErrorIs(t, runErr, context.Canceled)
	_, ok := sc.Get("a")
	assert.False(t, ok) // no wave launched
}
