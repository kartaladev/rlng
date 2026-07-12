package pipe_test

import (
	"testing"
	"time"

	"github.com/kartaladev/rlng/expr"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNowFunc shows the clock-backed now() host function makes temporal rules
// deterministic: the expression reads the injected clock, not the wall clock.
func TestNowFunc(t *testing.T) {
	t.Parallel()

	clk := fixedClock{t: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)}

	fn, err := expr.NewFunction("x", "now().Year()", expr.WithFunction("now", pipe.NowFunc(clk)))
	require.NoError(t, err)

	got, err := fn.Apply(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 2026, got)
}
