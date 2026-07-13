package rlng_test

import (
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validYAML = `
stages:
  - name: grade
    type: single-expr
    expr: input.score * 2
`

func TestNewFromProviderAndYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		build  func(t *testing.T) (*rlng.Engine, error)
		assert func(t *testing.T, eng *rlng.Engine, err error)
	}{
		{
			name: "NewFromYAML builds an engine that evaluates",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), validYAML)
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				require.NotNil(t, eng)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(21)}})
				require.NoError(t, err)
				// expr's `*` on an int64 seed and an int literal yields a plain
				// int result (observed), not int64 — the Engine returns the raw
				// expr-computed value with no mapstructure narrowing.
				assert.Equal(t, 42, out["grade"])
			},
		},
		{
			name: "NewFromProvider accepts a non-YAML provider (JSON)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				const j = `{"stages":[{"name":"grade","type":"single-expr","expr":"input.score * 2"}]}`
				return rlng.NewFromProvider(t.Context(), config.FromJSONString(j))
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(5)}})
				require.NoError(t, err)
				assert.Equal(t, 10, out["grade"]) // see note above: expr yields plain int here
			},
		},
		{
			name: "parse error passes through unwrapped",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), "this: is: not: valid: yaml: [")
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "build error passes through unwrapped (unknown stage type)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				const bad = `
stages:
  - name: x
    type: no-such-type
    expr: input.score
`
				return rlng.NewFromYAML(t.Context(), bad)
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "engine Option is threaded (provenance visible on the scope)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), validYAML, rlng.WithScopeOptions(pipe.WithProvenance()))
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				sc, err := eng.EvaluateScope(t.Context(), map[string]any{"input": map[string]any{"score": int64(3)}})
				require.NoError(t, err)
				assert.True(t, sc.TracksProvenance(), "WithScopeOptions(WithProvenance) must reach the evaluated scope")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eng, err := tc.build(t)
			tc.assert(t, eng, err)
		})
	}
}

func TestNewTypedFromProviderAndYAML(t *testing.T) {
	t.Parallel()

	type result struct {
		Grade int64 `mapstructure:"grade"`
	}
	mapper, err := rlng.NewMapper[result](rlng.MappingTemplate{"grade": "grade"})
	require.NoError(t, err)

	cases := []struct {
		name   string
		build  func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error)
		assert func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error)
	}{
		{
			name: "NewTypedFromYAML maps into the typed result",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromYAML[map[string]any, result](t.Context(), validYAML, mapper)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.NoError(t, err)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(4)}})
				require.NoError(t, err)
				assert.Equal(t, int64(8), out.Grade)
			},
		},
		{
			name: "NewTypedFromProvider with a nil mapper returns ErrNilMapper",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromProvider[map[string]any, result](t.Context(), config.FromYAMLString(validYAML), nil)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				assert.ErrorIs(t, err, rlng.ErrNilMapper)
			},
		},
		{
			name: "typed parse error passes through unwrapped",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromYAML[map[string]any, result](t.Context(), "not: valid: [", mapper)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "typed build error passes through unwrapped (unknown stage type)",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				const bad = `
stages:
  - name: x
    type: no-such-type
    expr: input.score
`
				return rlng.NewTypedFromYAML[map[string]any, result](t.Context(), bad, mapper)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eng, err := tc.build(t)
			tc.assert(t, eng, err)
		})
	}
}
