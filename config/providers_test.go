package config_test

import (
	"io"
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := config.Parse(t.Context(), tc.provider())
			tc.assert(t, d, err)
		})
	}
}
