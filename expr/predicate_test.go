package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPredicate(t *testing.T) {
	t.Parallel()

	t.Run("empty expression is a compile error", func(t *testing.T) {
		t.Parallel()

		_, err := NewPredicate("   ")
		require.ErrorIs(t, err, errEmptyExpression)
	})
}

func TestPredicate_Test(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		expr   string
		opts   []Option
		env    any
		assert func(t *testing.T, got bool, err error)
	}

	cases := []testCase{
		{
			name: "true condition",
			expr: "amount > 100",
			env:  map[string]any{"amount": 150},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name: "false condition",
			expr: "amount > 100",
			env:  map[string]any{"amount": 50},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
		{
			name: "struct env",
			expr: "Amount > 100",
			env:  struct{ Amount int }{Amount: 150},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name: "global default used",
			expr: "amount > threshold",
			opts: []Option{WithGlobals(map[string]any{"threshold": 100})},
			env:  map[string]any{"amount": 150},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name: "strict: non-bool result errors",
			expr: "amount + 1",
			env:  map[string]any{"amount": 1},
			assert: func(t *testing.T, got bool, err error) {
				require.ErrorIs(t, err, ErrNotBool)
			},
		},
		{
			name: "coerce: non-empty string is true",
			expr: "name",
			opts: []Option{WithCoerce()},
			env:  map[string]any{"name": "x"},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name: "coerce: zero number is false",
			expr: "count",
			opts: []Option{WithCoerce()},
			env:  map[string]any{"count": 0},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := NewPredicate(tc.expr, tc.opts...)
			require.NoError(t, err)

			got, err := p.Test(tc.env)
			tc.assert(t, got, err)
		})
	}
}
