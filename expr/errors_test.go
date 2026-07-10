package expr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")

	type testCase struct {
		name   string
		err    error
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "compile error names field and expression",
			err:  &CompileError{Name: "discount", Expression: "x >", Cause: inner},
			assert: func(t *testing.T, err error) {
				assert.Equal(t, `compile "discount" (x >): boom`, err.Error())
				require.ErrorIs(t, err, inner)
			},
		},
		{
			name: "eval error names field and expression",
			err:  &EvalError{Name: "discount", Expression: "x + y", Cause: inner},
			assert: func(t *testing.T, err error) {
				assert.Equal(t, `eval "discount" (x + y): boom`, err.Error())
				require.ErrorIs(t, err, inner)
			},
		},
		{
			name: "eval error wraps ErrNotBool",
			err:  &EvalError{Expression: "x + 1", Cause: ErrNotBool},
			assert: func(t *testing.T, err error) {
				assert.Equal(t, `eval (x + 1): expression did not evaluate to bool`, err.Error())
				require.ErrorIs(t, err, ErrNotBool)
			},
		},
		{
			name: "compile error with nil Cause does not panic and omits the cause",
			err:  &CompileError{Name: "discount", Expression: "x >"},
			assert: func(t *testing.T, err error) {
				var got string
				assert.NotPanics(t, func() { got = err.Error() })
				assert.Equal(t, `compile "discount" (x >)`, got)
				assert.Nil(t, errors.Unwrap(err))
			},
		},
		{
			name: "eval error with nil Cause does not panic and omits the cause",
			err:  &EvalError{Expression: "x + 1"},
			assert: func(t *testing.T, err error) {
				var got string
				assert.NotPanics(t, func() { got = err.Error() })
				assert.Equal(t, `eval (x + 1)`, got)
				assert.Nil(t, errors.Unwrap(err))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.assert(t, tc.err)
		})
	}
}
