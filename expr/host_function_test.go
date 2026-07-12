package expr_test

import (
	"errors"
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFunction(t *testing.T) {
	t.Parallel()

	double := func(args ...any) (any, error) {
		n, ok := args[0].(int)
		if !ok {
			return nil, errors.New("double: want int")
		}
		return n * 2, nil
	}

	type testCase struct {
		name   string
		build  func() (any, error) // returns the Apply/Test result
		assert func(t *testing.T, got any, err error)
	}

	cases := []testCase{
		{
			name: "function callable from a Function expression",
			build: func() (any, error) {
				fn, err := expr.NewFunction("x", "double(score) + 1", expr.WithFunction("double", double))
				if err != nil {
					return nil, err
				}
				return fn.Apply(map[string]any{"score": 10})
			},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 21, got)
			},
		},
		{
			name: "function callable from a Predicate expression",
			build: func() (any, error) {
				p, err := expr.NewPredicate("double(score) > 15", expr.WithFunction("double", double))
				if err != nil {
					return nil, err
				}
				return p.Test(map[string]any{"score": 10})
			},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, true, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.build()
			tc.assert(t, got, err)
		})
	}
}
