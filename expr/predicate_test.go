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

// TestPredicate_Test_CoerceMatrix exercises the full WithCoerce truthiness
// coercion surface. Each case evaluates the identifier `v`, whose value is
// supplied directly through the env so a native (non-`any`-typed) Go value
// reaches truthy() unchanged.
func TestPredicate_Test_CoerceMatrix(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		val  any
		want bool
	}

	cases := []testCase{
		{name: "nil is false", val: nil, want: false},
		{name: "non-zero int is true", val: 3, want: true},
		{name: "zero int is false", val: 0, want: false},
		{name: "non-zero uint is true", val: uint(3), want: true},
		{name: "zero uint is false", val: uint(0), want: false},
		{name: "non-zero float is true", val: 1.5, want: true},
		{name: "zero float is false", val: 0.0, want: false},
		{name: "bool true is true", val: true, want: true},
		{name: "bool false is false", val: false, want: false},
		{name: `string "true" parses true`, val: "true", want: true},
		{name: `string "false" parses false`, val: "false", want: false},
		{name: "non-empty non-bool string is true", val: "hello", want: true},
		{name: "empty string is false", val: "", want: false},
		{name: "native non-empty []string is true", val: []string{"a"}, want: true},
		{name: "native empty []string is false", val: []string{}, want: false},
		{name: "native non-empty map[string]int is true", val: map[string]int{"a": 1}, want: true},
		{name: "native empty map[string]int is false", val: map[string]int{}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := NewPredicate("v", WithCoerce())
			require.NoError(t, err)

			got, err := p.Test(map[string]any{"v": tc.val})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
