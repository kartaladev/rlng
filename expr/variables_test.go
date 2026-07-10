package expr

import (
	"testing"

	exprlang "github.com/expr-lang/expr"
	"github.com/stretchr/testify/require"
)

func TestVariablePatcher(t *testing.T) {
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
			name: "nil patcher when no variables declared",
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []exprlang.Option{exprlang.AllowUndefinedVariables()}
			if p := newPatcher(tc.globals, tc.locals); p != nil {
				opts = append(opts, exprlang.Patch(p))
			}

			program, err := exprlang.Compile(tc.src, opts...)
			require.NoError(t, err)

			got, err := exprlang.Run(program, tc.env)
			tc.assert(t, got, err)
		})
	}
}

func TestNewPatcher(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name            string
		globals, locals map[string]any
		assert          func(t *testing.T, p *variablePatcher)
	}

	cases := []testCase{
		{
			name: "nil when both maps are empty",
			assert: func(t *testing.T, p *variablePatcher) {
				require.Nil(t, p)
			},
		},
		{
			name:    "non-nil when globals declared",
			globals: map[string]any{"rate": 0.15},
			assert: func(t *testing.T, p *variablePatcher) {
				require.NotNil(t, p)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := newPatcher(tc.globals, tc.locals)
			tc.assert(t, p)
		})
	}
}
