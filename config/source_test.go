package config_test

import (
	"strings"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oneStageYAML = "stages:\n  - name: base\n    type: single-expr\n    expr: 1+1\n"
	oneStageJSON = `{"stages":[{"name":"base","type":"single-expr","expr":{"expr":"1+1"}}]}`
)

// TestParse covers Parse over every preloaded provider kind, the
// KindUnspecified failure path, strict unknown-field rejection, and the
// empty-document case — all driven through the public Provider API.
func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider config.Provider
		assert   func(t *testing.T, d *config.PipelineDef, err error)
	}{
		{
			name:     "FromYAMLBytes decodes",
			provider: config.FromYAMLBytes([]byte(oneStageYAML)),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromJSONBytes decodes",
			provider: config.FromJSONBytes([]byte(oneStageJSON)),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromYAMLString decodes",
			provider: config.FromYAMLString(oneStageYAML),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromJSONString decodes",
			provider: config.FromJSONString(oneStageJSON),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromReader decodes YAML",
			provider: config.FromReader(strings.NewReader(oneStageYAML), config.KindYAML),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromReader decodes JSON",
			provider: config.FromReader(strings.NewReader(oneStageJSON), config.KindJSON),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "unspecified kind is ErrUnknownSourceKind wrapped in ConfigError",
			provider: config.FromReader(strings.NewReader(oneStageYAML), config.KindUnspecified),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				assert.ErrorIs(t, err, config.ErrUnknownSourceKind)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name:     "unknown field is rejected (strict decode preserved)",
			provider: config.FromYAMLBytes([]byte("stages:\n  - name: x\n    type: single-expr\n    expr: \"1\"\n    hitpolicy: collect\n")),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce, "a misspelled key must be a clear error, not silently dropped")
			},
		},
		{
			name:     "empty document is an empty def, no error",
			provider: config.FromYAMLBytes([]byte("")),
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.NotNil(t, d)
				assert.Empty(t, d.Stages)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := config.Parse(t.Context(), tc.provider)
			tc.assert(t, d, err)
		})
	}
}

// TestSourceKindString covers every SourceKind.String() branch, including the
// zero-value guard.
func TestSourceKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind config.SourceKind
		want string
	}{
		{name: "YAML", kind: config.KindYAML, want: "yaml"},
		{name: "JSON", kind: config.KindJSON, want: "json"},
		{name: "Unspecified", kind: config.KindUnspecified, want: "unspecified"},
		{name: "unrecognized value falls back to unspecified", kind: config.SourceKind(99), want: "unspecified"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.kind.String())
		})
	}
}
