package expr_test

import (
	"github.com/kartaladev/rlng/expr"
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
		opts   []expr.Option
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:   "empty expression is a compile error",
			fnName: "x",
			expr:   "   ",
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, expr.ErrEmptyExpression)
			},
		},
		{
			name:   "invalid expression is a compile error",
			fnName: "x",
			expr:   "1 +",
			assert: func(t *testing.T, err error) {
				require.Error(t, err)

				var compileErr *expr.CompileError
				require.ErrorAs(t, err, &compileErr)
			},
		},
		{
			name:   "invalid fallback expression surfaces at construction",
			fnName: "x",
			expr:   "1 + 1",
			opts:   []expr.Option{expr.WithFallback("(")},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)

				var compileErr *expr.CompileError
				require.ErrorAs(t, err, &compileErr)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := expr.NewFunction(tc.fnName, tc.expr, tc.opts...)
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
		opts   []expr.Option
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
			opts:   []expr.Option{expr.WithFallback("-1")},
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

				var evalErr *expr.EvalError
				require.ErrorAs(t, err, &evalErr)
			},
		},
		{
			name:   "fallback error is keyed to the fallback source, not the main expression",
			fnName: "ratio",
			expr:   "a % b", // main errors: modulo by zero
			// fallback runs over an EMPTY env, so `z` is nil and `1 % z`
			// errors at runtime (it is not constant-folded at compile time).
			opts: []expr.Option{expr.WithFallback("1 % z")},
			env:  map[string]any{"a": 1, "b": 0},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)

				var evalErr *expr.EvalError
				require.ErrorAs(t, err, &evalErr)
				assert.Equal(t, "1 % z", evalErr.Expression)
				assert.NotEqual(t, "a % b", evalErr.Expression)
			},
		},
		{
			name:   "fallback honors expr.WithReturnKind coercion",
			fnName: "x",
			expr:   "a % b", // main errors: modulo by zero -> fallback runs
			opts:   []expr.Option{expr.WithFallback("0"), expr.WithReturnKind(reflect.Float64)},
			env:    map[string]any{"a": 1, "b": 0},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0.0, got)
				_, isFloat := got.(float64)
				assert.True(t, isFloat, "fallback result should be coerced to float64, got %T", got)
			},
		},
		{
			name:   "fallback failure preserves the triggering main error",
			fnName: "ratio",
			expr:   "a % b",                                   // main errors: modulo by zero
			opts:   []expr.Option{expr.WithFallback("1 % z")}, // fallback also errors over empty env
			env:    map[string]any{"a": 1, "b": 0},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
				var evalErr *expr.EvalError
				require.ErrorAs(t, err, &evalErr)
				joined, ok := evalErr.Cause.(interface{ Unwrap() []error })
				require.True(t, ok, "a fallback failure should join the main error into the cause")
				assert.Len(t, joined.Unwrap(), 2, "both the main and fallback errors should survive")
			},
		},
		func() testCase {
			var seen error
			return testCase{
				name:   "error-triggered fallback observer sees the masked cause",
				fnName: "ratio",
				expr:   "a % b", // modulo by zero -> runtime error
				opts: []expr.Option{
					expr.WithFallback("-1"),
					expr.WithFallbackObserver(func(_, _ string, cause error) { seen = cause }),
				},
				env: map[string]any{"a": 1, "b": 0},
				assert: func(t *testing.T, got any, err error) {
					require.NoError(t, err)
					assert.Equal(t, -1, got)
					require.Error(t, seen, "the masked division error must be surfaced to the observer")
				},
			}
		}(),
		func() testCase {
			var seen error
			return testCase{
				name:   "nil-triggered fallback does not invoke the observer",
				fnName: "x",
				expr:   "nil",
				opts: []expr.Option{
					expr.WithFallback("42"),
					expr.WithFallbackOnNil(),
					expr.WithFallbackObserver(func(_, _ string, cause error) { seen = cause }),
				},
				env: nil,
				assert: func(t *testing.T, got any, err error) {
					require.NoError(t, err)
					assert.Equal(t, 42, got)
					assert.NoError(t, seen, "observer must only be called for an error-triggered fallback")
				},
			}
		}(),
		{
			name:   "nil is first-class: fallback not used on nil result by default",
			fnName: "x",
			expr:   "nil",
			opts:   []expr.Option{expr.WithFallback("42")},
			env:    nil,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Nil(t, got, "fallback must not fire on a nil main result unless WithFallbackOnNil is set")
			},
		},
		{
			name:   "fallback used on nil result when WithFallbackOnNil is set",
			fnName: "x",
			expr:   "nil",
			opts:   []expr.Option{expr.WithFallback("42"), expr.WithFallbackOnNil()},
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
			name:   "expr.WithReturnKind coerces the result",
			fnName: "x",
			expr:   "1 + 1",
			opts:   []expr.Option{expr.WithReturnKind(reflect.Float64)},
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
			opts:   []expr.Option{expr.WithGlobals(map[string]any{"qty": 2})},
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

			f, err := expr.NewFunction(tc.fnName, tc.expr, tc.opts...)
			require.NoError(t, err)

			got, err := f.Apply(tc.env)
			tc.assert(t, got, err)
		})
	}
}
