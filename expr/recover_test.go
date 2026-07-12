package expr_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/require"
)

// TestEvalPanicIsError locks the safety guarantee that a panic during
// evaluation — from a host function or an env-value method — is surfaced as a
// typed EvalError rather than crashing the host process. The underlying expr VM
// recovers such panics; this test guards against a regression (e.g. a future
// evaluation path that bypasses that protection).
func TestEvalPanicIsError(t *testing.T) {
	t.Parallel()

	boom := func(args ...any) (any, error) { panic("boom") }

	type testCase struct {
		name string
		run  func() error
	}

	cases := []testCase{
		{
			name: "Function.Apply surfaces a panicking host function as an error",
			run: func() error {
				fn, err := expr.NewFunction("x", "boom()", expr.WithFunction("boom", boom))
				require.NoError(t, err)
				_, err = fn.Apply(map[string]any{})
				return err
			},
		},
		{
			name: "Predicate.Test surfaces a panicking host function as an error",
			run: func() error {
				p, err := expr.NewPredicate("boom()", expr.WithFunction("boom", boom))
				require.NoError(t, err)
				_, err = p.Test(map[string]any{})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.run()
			require.Error(t, err)
			var evalErr *expr.EvalError
			require.ErrorAs(t, err, &evalErr)
		})
	}
}
