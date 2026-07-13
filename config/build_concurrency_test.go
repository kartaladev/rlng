package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildConcurrency(t *testing.T) {
	yaml := `
stages:
  - name: a
    type: single-expr
    expr: "1 + 1"
  - name: b
    type: single-expr
    expr: "2 + 2"
`
	tests := []struct {
		name   string
		opts   []config.BuildOption
		assert func(t *testing.T, out map[string]any, buildErr error)
	}{
		{
			name: "concurrent build still evaluates correctly",
			opts: []config.BuildOption{config.WithConcurrency()},
			assert: func(t *testing.T, out map[string]any, buildErr error) {
				require.NoError(t, buildErr)
				assert.EqualValues(t, 2, out["a"])
				assert.EqualValues(t, 4, out["b"])
			},
		},
		{
			name: "bounded build still evaluates correctly",
			opts: []config.BuildOption{config.WithMaxParallel(1)},
			assert: func(t *testing.T, out map[string]any, buildErr error) {
				require.NoError(t, buildErr)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "invalid bound surfaces from Build",
			opts: []config.BuildOption{config.WithMaxParallel(0)},
			assert: func(t *testing.T, _ map[string]any, buildErr error) {
				var e *pipe.InvalidMaxParallelError
				require.ErrorAs(t, buildErr, &e)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, err := config.Parse(t.Context(), config.FromYAMLString(yaml))
			require.NoError(t, err)
			p, err := def.Build(tc.opts...)
			if err != nil {
				tc.assert(t, nil, err)
				return
			}
			sc := pipe.NewScope(nil)
			require.NoError(t, p.Run(t.Context(), sc))
			tc.assert(t, sc.Snapshot(), nil)
		})
	}
}
