package expr_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithEnvStrict(t *testing.T) {
	t.Parallel()

	env := map[string]any{"score": 0, "region": ""}

	type testCase struct {
		name    string
		expr    string
		opts    []expr.Option
		wantErr bool
	}

	cases := []testCase{
		{
			name:    "unknown identifier is a compile error under strict env",
			expr:    "scoer >= 650", // typo: scoer
			opts:    []expr.Option{expr.WithEnv(env)},
			wantErr: true,
		},
		{
			name:    "declared identifier compiles",
			expr:    "score >= 650",
			opts:    []expr.Option{expr.WithEnv(env)},
			wantErr: false,
		},
		{
			name:    "declared global is visible to strict compilation",
			expr:    "score >= threshold",
			opts:    []expr.Option{expr.WithEnv(env), expr.WithGlobals(map[string]any{"threshold": 650})},
			wantErr: false,
		},
		{
			name:    "registered function is visible to strict compilation",
			expr:    "bump(score)",
			opts:    []expr.Option{expr.WithEnv(env), expr.WithFunction("bump", func(a ...any) (any, error) { return 1, nil })},
			wantErr: false,
		},
		{
			name:    "unknown identifier is allowed without strict env (lenient default)",
			expr:    "scoer >= 650",
			opts:    nil,
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := expr.NewFunction("x", tc.expr, tc.opts...)
			if tc.wantErr {
				require.Error(t, err)
				var compileErr *expr.CompileError
				require.ErrorAs(t, err, &compileErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWithEnvStrictEvaluates(t *testing.T) {
	t.Parallel()

	fn, err := expr.NewFunction("x", "score * 2", expr.WithEnv(map[string]any{"score": 0}))
	require.NoError(t, err)
	got, err := fn.Apply(map[string]any{"score": 21})
	require.NoError(t, err)
	assert.Equal(t, 42, got)
}
