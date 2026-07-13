package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/require"
)

// TestBuildStrictSchema exercises (*PipelineDef).Build under the schema/strict
// combinations: a declared schema catches a field typo at build time, accepts
// a correctly-named field, absence of a schema stays lenient (unchanged prior
// behavior), and WithStrict without any schema is a build-time *ConfigError.
func TestBuildStrictSchema(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		parse     func(t *testing.T) *config.PipelineDef
		buildOpts []config.BuildOption
		assert    func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "declared schema rejects a field typo at build",
			parse: func(t *testing.T) *config.PipelineDef {
				t.Helper()
				doc := []byte(`
schema:
  score: 0
stages:
  - name: gate
    type: single-expr
    expr: "scoer >= 650"
`)
				d, err := config.Parse(t.Context(), config.FromYAMLBytes(doc))
				require.NoError(t, err)
				return d
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err, "typo 'scoer' must fail at build under a declared schema")
				require.Contains(t, err.Error(), "scoer")
			},
		},
		{
			name: "declared schema accepts the declared field",
			parse: func(t *testing.T) *config.PipelineDef {
				t.Helper()
				doc := []byte(`
schema:
  score: 0
stages:
  - name: gate
    type: single-expr
    expr: "score >= 650"
`)
				d, err := config.Parse(t.Context(), config.FromYAMLBytes(doc))
				require.NoError(t, err)
				return d
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "no schema stays lenient",
			parse: func(t *testing.T) *config.PipelineDef {
				t.Helper()
				doc := []byte(`{"stages":[{"name":"gate","type":"single-expr","expr":"scoer >= 650"}]}`)
				d, err := config.Parse(t.Context(), config.FromJSONBytes(doc))
				require.NoError(t, err)
				return d
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err) // lenient: undefined var tolerated, builds fine
			},
		},
		{
			name: "WithStrict without any schema errors",
			parse: func(t *testing.T) *config.PipelineDef {
				t.Helper()
				doc := []byte(`{"stages":[{"name":"gate","type":"single-expr","expr":"score >= 650"}]}`)
				d, err := config.Parse(t.Context(), config.FromJSONBytes(doc))
				require.NoError(t, err)
				return d
			},
			buildOpts: []config.BuildOption{config.WithStrict()},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "WithSchema supplies the check env programmatically, no doc schema needed",
			parse: func(t *testing.T) *config.PipelineDef {
				t.Helper()
				doc := []byte(`{"stages":[{"name":"gate","type":"single-expr","expr":"scoer >= 650"}]}`)
				d, err := config.Parse(t.Context(), config.FromJSONBytes(doc))
				require.NoError(t, err)
				return d
			},
			buildOpts: []config.BuildOption{config.WithSchema(map[string]any{"score": 0})},
			assert: func(t *testing.T, err error) {
				require.Error(t, err, "typo 'scoer' must fail at build under a WithSchema-supplied env")
				require.Contains(t, err.Error(), "scoer")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := tc.parse(t)
			_, err := d.Build(tc.buildOpts...)
			tc.assert(t, err)
		})
	}
}
