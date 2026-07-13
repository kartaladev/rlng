package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProvider is a test-only Provider returning a caller-controlled
// (Source, error) pair. No in-repo provider yields a raw (non-*ConfigError)
// error or a nil Source with nil error, so it is the only way to exercise
// Parse's raw-error-wrap fallback and its nil-Source guard.
type fakeProvider struct {
	src config.Source
	err error
}

func (p fakeProvider) Source(context.Context) (config.Source, error) { return p.src, p.err }

// TestParseProviderContract covers Parse's two Provider-contract-violation
// branches: a Source() failure that is not already a *ConfigError (wrapped,
// cause preserved) and a (nil, nil) return (ErrNilSource, not a panic). The
// *ConfigError-preserving branch is already covered by the FromFile-missing-path
// case in TestProviders.
func TestParseProviderContract(t *testing.T) {
	t.Parallel()

	rawErr := errors.New("provider exploded")

	cases := []struct {
		name     string
		provider config.Provider
		assert   func(t *testing.T, d *config.PipelineDef, err error)
	}{
		{
			name:     "raw error is wrapped in ConfigError, cause preserved",
			provider: fakeProvider{err: rawErr},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				assert.Nil(t, d)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce, "a raw provider error must be wrapped in *ConfigError")
				assert.ErrorIs(t, err, rawErr, "the original cause must be preserved for unwrapping")
			},
		},
		{
			name:     "nil Source with nil error is ErrNilSource, not a panic",
			provider: fakeProvider{}, // src == nil, err == nil
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				assert.Nil(t, d)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, config.ErrNilSource)
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
