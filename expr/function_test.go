package expr

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFunction(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		fnName string
		expr   string
		opts   []Option
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:   "empty expression is a compile error",
			fnName: "x",
			expr:   "   ",
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errEmptyExpression)
			},
		},
		{
			name:   "invalid expression is a compile error",
			fnName: "x",
			expr:   "1 +",
			assert: func(t *testing.T, err error) {
				require.Error(t, err)

				var compileErr *CompileError
				require.ErrorAs(t, err, &compileErr)
			},
		},
		{
			name:   "invalid fallback expression surfaces at construction",
			fnName: "x",
			expr:   "1 + 1",
			opts:   []Option{WithFallback("(")},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)

				var compileErr *CompileError
				require.ErrorAs(t, err, &compileErr)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewFunction(tc.fnName, tc.expr, tc.opts...)
			tc.assert(t, err)
		})
	}
}

func TestFunction_Apply(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		fnName string
		expr   string
		opts   []Option
		env    any
		assert func(t *testing.T, got any, err error)
	}

	cases := []testCase{
		{
			name:   "computes value",
			fnName: "total",
			expr:   "price * qty",
			env:    map[string]any{"price": 10, "qty": 3},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, got)
			},
		},
		{
			name:   "struct env",
			fnName: "total",
			expr:   "Price * Qty",
			env:    struct{ Price, Qty int }{Price: 10, Qty: 3},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, got)
			},
		},
		{
			name:   "fallback used on eval error",
			fnName: "ratio",
			expr:   "a % b",
			opts:   []Option{WithFallback("-1")},
			env:    map[string]any{"a": 1, "b": 0}, // modulo by zero -> runtime error
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, -1, got)
			},
		},
		{
			name:   "no fallback surfaces eval error",
			fnName: "ratio",
			expr:   "a % b",
			env:    map[string]any{"a": 1, "b": 0},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)

				var evalErr *EvalError
				require.ErrorAs(t, err, &evalErr)
			},
		},
		{
			name:   "fallback error is keyed to the fallback source, not the main expression",
			fnName: "ratio",
			expr:   "a % b", // main errors: modulo by zero
			// fallback runs over an EMPTY env, so `z` is nil and `1 % z`
			// errors at runtime (it is not constant-folded at compile time).
			opts: []Option{WithFallback("1 % z")},
			env:  map[string]any{"a": 1, "b": 0},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)

				var evalErr *EvalError
				require.ErrorAs(t, err, &evalErr)
				assert.Equal(t, "1 % z", evalErr.Expression)
				assert.NotEqual(t, "a % b", evalErr.Expression)
			},
		},
		{
			name:   "fallback used on nil result",
			fnName: "x",
			expr:   "nil",
			opts:   []Option{WithFallback("42")},
			env:    nil,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 42, got)
			},
		},
		{
			name:   "no fallback: nil result returned unchanged",
			fnName: "x",
			expr:   "nil",
			env:    nil,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Nil(t, got)
			},
		},
		{
			name:   "WithReturnKind coerces the result",
			fnName: "x",
			expr:   "1 + 1",
			opts:   []Option{WithReturnKind(reflect.Float64)},
			env:    nil,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2.0, got)
			},
		},
		{
			name:   "global default used",
			fnName: "total",
			expr:   "price * qty",
			opts:   []Option{WithGlobals(map[string]any{"qty": 2})},
			env:    map[string]any{"price": 10},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 20, got)
			},
		},
		{
			name:   "value struct with no exported fields (time.Time) survives conversion, so its methods are callable",
			fnName: "year",
			expr:   "T.Year()",
			env:    envTestWithTime{T: time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC)},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2024, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := NewFunction(tc.fnName, tc.expr, tc.opts...)
			require.NoError(t, err)

			got, err := f.Apply(tc.env)
			tc.assert(t, got, err)
		})
	}
}
