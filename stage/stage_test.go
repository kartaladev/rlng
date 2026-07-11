package stage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageError(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")

	type testCase struct {
		name   string
		err    *StageError
		assert func(t *testing.T, err *StageError)
	}

	cases := []testCase{
		{
			name: "names stage and type and unwraps",
			err:  &StageError{Stage: "discount", Type: TypeSingleExpr, Cause: inner},
			assert: func(t *testing.T, err *StageError) {
				assert.Equal(t, `stage "discount" (single-expr): boom`, err.Error())
				require.ErrorIs(t, err, inner)
			},
		},
		{
			name: "nil cause does not panic",
			err:  &StageError{Stage: "discount", Type: TypeSingleExpr},
			assert: func(t *testing.T, err *StageError) {
				assert.Equal(t, `stage "discount" (single-expr)`, err.Error())
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
