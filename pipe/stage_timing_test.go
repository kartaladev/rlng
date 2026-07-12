package pipe_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// steppingClock advances by step on every Now() call, so each timed span across
// two consecutive reads is exactly step — deterministic per-stage timing.
type steppingClock struct {
	mu   sync.Mutex
	base time.Time
	step time.Duration
	n    int
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.base.Add(time.Duration(c.n) * c.step)
	c.n++
	return t
}

func TestStageTimings(t *testing.T) {
	t.Parallel()

	a, err := pipe.NewSingleExpr("a", "1 + 1")
	require.NoError(t, err)
	b, err := pipe.NewSingleExpr("b", "a + 1", pipe.WithDependsOn("a"))
	require.NoError(t, err)
	p, err := pipe.NewPipeline(a, b)
	require.NoError(t, err)

	clk := &steppingClock{base: time.Unix(0, 0), step: time.Millisecond}
	sc := pipe.NewScope(map[string]any{}, pipe.WithClock(clk))
	require.NoError(t, p.Run(context.Background(), sc))

	timings := sc.StageTimings()
	require.Len(t, timings, 2)
	assert.Equal(t, "a", timings[0].Stage)
	assert.Equal(t, "b", timings[1].Stage)
	assert.Equal(t, time.Millisecond, timings[0].Duration)
	assert.Equal(t, time.Millisecond, timings[1].Duration)

	da, ok := sc.StageDuration("a")
	require.True(t, ok)
	assert.Equal(t, time.Millisecond, da)

	_, ok = sc.StageDuration("missing")
	assert.False(t, ok)
}
