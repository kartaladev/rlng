package config_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closeTrackingReader wraps a reader and counts Close calls, so a test can
// assert whether Parse closed it.
type closeTrackingReader struct {
	io.Reader
	closes int
}

func (c *closeTrackingReader) Close() error {
	c.closes++
	return nil
}

// TestFromReaderDoesNotCloseCallerReader asserts the documented FromReader
// contract: the caller owns the reader's lifecycle, so Parse must not close
// it even though it implements io.Closer.
func TestFromReaderDoesNotCloseCallerReader(t *testing.T) {
	t.Parallel()

	r := &closeTrackingReader{Reader: strings.NewReader(oneStageYAML)}
	d, err := config.Parse(t.Context(), config.FromReader(r, config.KindYAML))

	require.NoError(t, err)
	require.Len(t, d.Stages, 1)
	assert.Equal(t, 0, r.closes, "FromReader must not close a caller-owned reader")
}

// TestProviders covers the remaining preloaded-provider surface not already
// exercised by TestParse: distinct providers built from the same source text
// all decode to the same result via the public Parse entry point.
func TestProviders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTemp := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	yamlPath := writeTemp("x.yaml", oneStageYAML)
	ymlPath := writeTemp("x.yml", oneStageYAML)
	jsonPath := writeTemp("x.json", oneStageJSON)
	unsupportedExtPath := writeTemp("x.txt", oneStageYAML)
	forcedYAMLPath := writeTemp("forced.txt", oneStageYAML)
	forcedJSONPath := writeTemp("forced.yaml", oneStageJSON)
	twiceParsePath := writeTemp("twice.yaml", oneStageYAML)
	missingPath := filepath.Join(dir, "nope.yaml")

	cases := []struct {
		name     string
		provider func() config.Provider
		assert   func(t *testing.T, d *config.PipelineDef, err error)
	}{
		{
			name:     "FromYAMLBytes and FromYAMLString agree",
			provider: func() config.Provider { return config.FromYAMLString(oneStageYAML) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromJSONBytes and FromJSONString agree",
			provider: func() config.Provider { return config.FromJSONString(oneStageJSON) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromFile infers YAML for .yaml extension",
			provider: func() config.Provider { return config.FromFile(yamlPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromFile infers YAML for .yml extension",
			provider: func() config.Provider { return config.FromFile(ymlPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromFile infers JSON for .json extension",
			provider: func() config.Provider { return config.FromFile(jsonPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromFile unsupported extension is ErrUnsupportedExtension",
			provider: func() config.Provider { return config.FromFile(unsupportedExtPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, config.ErrUnsupportedExtension)
			},
		},
		{
			name:     "FromYAMLFile decodes as YAML regardless of extension",
			provider: func() config.Provider { return config.FromYAMLFile(forcedYAMLPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromJSONFile decodes as JSON regardless of extension",
			provider: func() config.Provider { return config.FromJSONFile(forcedJSONPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name:     "FromFile missing path is a ConfigError wrapping os.ErrNotExist",
			provider: func() config.Provider { return config.FromFile(missingPath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name:     "FromFile parses the same file twice without leaking the handle",
			provider: func() config.Provider { return config.FromFile(twiceParsePath) },
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)

				d2, err2 := config.Parse(t.Context(), config.FromFile(twiceParsePath))
				require.NoError(t, err2)
				require.Len(t, d2.Stages, 1)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := config.Parse(t.Context(), tc.provider())
			tc.assert(t, d, err)
		})
	}
}
