package pipe_test

import (
	"errors"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageError(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")

	type testCase struct {
		name   string
		err    *pipe.StageError
		assert func(t *testing.T, err *pipe.StageError)
	}

	cases := []testCase{
		{
			name: "names stage and type and unwraps",
			err:  &pipe.StageError{Stage: "discount", Type: pipe.TypeSingleExpr, Cause: inner},
			assert: func(t *testing.T, err *pipe.StageError) {
				assert.Equal(t, `stage "discount" (single-expr): boom`, err.Error())
				require.ErrorIs(t, err, inner)
			},
		},
		{
			name: "nil cause does not panic",
			err:  &pipe.StageError{Stage: "discount", Type: pipe.TypeSingleExpr},
			assert: func(t *testing.T, err *pipe.StageError) {
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
