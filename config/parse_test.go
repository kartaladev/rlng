package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`

func TestParseYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name: "valid preserves order and shorthand",
			yaml: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
				assert.Equal(t, "base", d.Stages[0].Name)
				require.NotNil(t, d.Stages[0].Expr)
				assert.Equal(t, "price * qty", d.Stages[0].Expr.Expr)
				assert.Equal(t, []string{"base"}, d.Stages[1].DependsOn)
			},
		},
		{
			name: "malformed yaml errors",
			yaml: "stages: [unclosed",
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := ParseYAML([]byte(tc.yaml))
			tc.assert(t, d, err)
		})
	}
}

func TestParseJSON(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		json   string
		assert func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name: "valid",
			json: `{"stages":[{"name":"base","type":"single-expr","expr":"price * qty"}]}`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				require.NotNil(t, d.Stages[0].Expr)
				assert.Equal(t, "price * qty", d.Stages[0].Expr.Expr)
			},
		},
		{
			name: "malformed",
			json: `{"stages": [`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := ParseJSON([]byte(tc.json))
			tc.assert(t, d, err)
		})
	}
}

func TestLoadFile(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		file    string // basename; contents chosen by ext
		content string
		assert  func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name:    "yaml extension",
			file:    "p.yaml",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
			},
		},
		{
			name:    "yml extension",
			file:    "p.yml",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
			},
		},
		{
			name:    "json extension",
			file:    "p.json",
			content: `{"stages":[{"name":"a","type":"single-expr","expr":"1"}]}`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
			},
		},
		{
			name:    "unknown extension",
			file:    "p.txt",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))
			d, err := LoadFile(path)
			tc.assert(t, d, err)
		})
	}
}

func TestLoadFileMissing(t *testing.T) {
	t.Parallel()
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	var ce *ConfigError
	require.ErrorAs(t, err, &ce)
}
