package expr_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/kartaladev/rlng/expr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedBool and namedString are defined (non-native) types whose Kind is
// Bool/String; truthy must coerce them by kind, not only by exact type.
type namedBool bool

type namedString string

func TestNewPredicate(t *testing.T) {
	t.Parallel()

	t.Run("empty expression is a compile error", func(t *testing.T) {
		t.Parallel()

		_, err := expr.NewPredicate("   ")
		require.ErrorIs(t, err, expr.ErrEmptyExpression)
	})
}

func TestPredicate_Test(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		expr   string
		opts   []expr.Option
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
			opts: []expr.Option{expr.WithGlobals(map[string]any{"threshold": 100})},
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
				require.ErrorIs(t, err, expr.ErrNotBool)
			},
		},
		{
			name: "coerce: non-empty string is true",
			expr: "name",
			opts: []expr.Option{expr.WithCoerce()},
			env:  map[string]any{"name": "x"},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.True(t, got)
			},
		},
		{
			name: "coerce: zero number is false",
			expr: "count",
			opts: []expr.Option{expr.WithCoerce()},
			env:  map[string]any{"count": 0},
			assert: func(t *testing.T, got bool, err error) {
				require.NoError(t, err)
				assert.False(t, got)
			},
		},
		{
			name: "coerce: unhandled result type is a typed EvalError",
			expr: "v",
			opts: []expr.Option{expr.WithCoerce()},
			env:  map[string]any{"v": time.Now()},
			assert: func(t *testing.T, got bool, err error) {
				require.Error(t, err, "an unhandled result type must be a typed error, not a silent false")
				var ee *expr.EvalError
				require.ErrorAs(t, err, &ee)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := expr.NewPredicate(tc.expr, tc.opts...)
			require.NoError(t, err)

			got, err := p.Test(tc.env)
			tc.assert(t, got, err)
		})
	}
}

// TestPredicate_Test_CoerceMatrix exercises the full expr.WithCoerce truthiness
// coercion surface. Each case evaluates the identifier `v`, whose value is
// supplied directly through the env so a native (non-`any`-typed) Go value
// reaches truthy() unchanged.
func TestPredicate_Test_CoerceMatrix(t *testing.T) {
	t.Parallel()

	isTrue := func(t *testing.T, got bool) { assert.True(t, got) }
	isFalse := func(t *testing.T, got bool) { assert.False(t, got) }

	type testCase struct {
		name   string
		val    any
		assert func(t *testing.T, got bool)
	}

	cases := []testCase{
		{name: "nil is false", val: nil, assert: isFalse},
		{name: "non-zero int is true", val: 3, assert: isTrue},
		{name: "zero int is false", val: 0, assert: isFalse},
		{name: "non-zero uint is true", val: uint(3), assert: isTrue},
		{name: "zero uint is false", val: uint(0), assert: isFalse},
		{name: "non-zero float is true", val: 1.5, assert: isTrue},
		{name: "zero float is false", val: 0.0, assert: isFalse},
		{name: "NaN is false", val: math.NaN(), assert: isFalse},
		{name: "+Inf is false", val: math.Inf(1), assert: isFalse},
		{name: "-Inf is false", val: math.Inf(-1), assert: isFalse},
		{name: "bool true is true", val: true, assert: isTrue},
		{name: "bool false is false", val: false, assert: isFalse},
		{name: `string "true" parses true`, val: "true", assert: isTrue},
		{name: `string "false" parses false`, val: "false", assert: isFalse},
		{name: "non-empty non-bool string is true", val: "hello", assert: isTrue},
		{name: "empty string is false", val: "", assert: isFalse},
		{name: "native non-empty []string is true", val: []string{"a"}, assert: isTrue},
		{name: "native empty []string is false", val: []string{}, assert: isFalse},
		{name: "native non-empty map[string]int is true", val: map[string]int{"a": 1}, assert: isTrue},
		{name: "native empty map[string]int is false", val: map[string]int{}, assert: isFalse},
		// Named (non-native) types whose Kind is Bool/String must coerce by kind.
		{name: "named bool true is true", val: namedBool(true), assert: isTrue},
		{name: "named bool false is false", val: namedBool(false), assert: isFalse},
		{name: "named non-empty string is true", val: namedString("hello"), assert: isTrue},
		{name: "named empty string is false", val: namedString(""), assert: isFalse},
		{name: `json.Number "1" parses true`, val: json.Number("1"), assert: isTrue},
		{name: `json.Number "0" parses false`, val: json.Number("0"), assert: isFalse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := expr.NewPredicate("v", expr.WithCoerce())
			require.NoError(t, err)

			got, err := p.Test(map[string]any{"v": tc.val})
			require.NoError(t, err)
			tc.assert(t, got)
		})
	}
}
