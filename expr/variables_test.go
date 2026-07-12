package expr_test

import (
	"math"
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/require"
)

// TestVariableDefaults exercises the variable-default patcher (declared
// globals/locals injected as `identifier ?? default` at compile time, overridable
// by the runtime env) through the public NewFunction + Apply. It replaces the
// former white-box test of the internal patcher, covering the same branches:
// scalar default applied, runtime override, local-over-global precedence, the
// no-variables (nil patcher) path, and the non-scalar / nil-pointer / overflow
// skips.
func TestVariableDefaults(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name            string
		src             string
		globals, locals map[string]any
		env             map[string]any
		assert          func(t *testing.T, got any, err error)
	}

	cases := []testCase{
		{
			name:    "global default applies when env omits the key",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 0.15, got)
			},
		},
		{
			name:    "runtime env overrides the default",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			env:     map[string]any{"rate": 0.2},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 0.2, got)
			},
		},
		{
			name:    "local takes precedence over global",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			locals:  map[string]any{"rate": 0.99},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 0.99, got)
			},
		},
		{
			name: "no variables declared: nil patcher, plain evaluation",
			src:  "1 + 1",
			env:  map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 2, got)
			},
		},
		{
			name:    "non-scalar global is skipped, left as unpatched identifier",
			src:     "items",
			globals: map[string]any{"items": []int{1, 2, 3}},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Nil(t, got)
			},
		},
		{
			name:    "nil-pointer global is skipped, left as unpatched identifier",
			src:     "p",
			globals: map[string]any{"p": (*int)(nil)},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Nil(t, got)
			},
		},
		{
			name:    "non-scalar global skipped, runtime env value is read",
			src:     "items",
			globals: map[string]any{"items": []int{1, 2, 3}},
			env:     map[string]any{"items": 7},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 7, got)
			},
		},
		{
			name:    "huge uint global overflowing int is skipped, left unpatched",
			src:     "big",
			globals: map[string]any{"big": uint64(math.MaxUint64)},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Nil(t, got)
			},
		},
		{
			name:    "huge uint global skipped, runtime env value is read",
			src:     "big",
			globals: map[string]any{"big": uint64(math.MaxUint64)},
			env:     map[string]any{"big": 7},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Equal(t, 7, got)
			},
		},
		{
			name:    "uintptr global is not a value scalar, skipped and left unpatched",
			src:     "ptr",
			globals: map[string]any{"ptr": uintptr(42)},
			env:     map[string]any{},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				require.Nil(t, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var opts []expr.Option
			if tc.globals != nil {
				opts = append(opts, expr.WithGlobals(tc.globals))
			}
			if tc.locals != nil {
				opts = append(opts, expr.WithLocals(tc.locals))
			}

			f, err := expr.NewFunction("f", tc.src, opts...)
			require.NoError(t, err)

			got, err := f.Apply(tc.env)
			tc.assert(t, got, err)
		})
	}
}
