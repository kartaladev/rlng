package expr_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithGlobalsMerge verifies multiple WithGlobals/WithLocals options merge
// (last value wins per key) rather than the last call overwriting all earlier
// keys — so a pipeline-level constant and a per-expression global can coexist.
func TestWithGlobalsMerge(t *testing.T) {
	t.Parallel()

	t.Run("globals merge across calls", func(t *testing.T) {
		t.Parallel()
		// a from the first call, b from the second: both must be visible.
		fn, err := expr.NewFunction("x", "a + b",
			expr.WithGlobals(map[string]any{"a": 1}),
			expr.WithGlobals(map[string]any{"b": 2}),
		)
		require.NoError(t, err)
		got, err := fn.Apply(map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 3, got)
	})

	t.Run("later call wins on key conflict", func(t *testing.T) {
		t.Parallel()
		fn, err := expr.NewFunction("x", "a",
			expr.WithGlobals(map[string]any{"a": 1}),
			expr.WithGlobals(map[string]any{"a": 9}),
		)
		require.NoError(t, err)
		got, err := fn.Apply(map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 9, got)
	})
}
