package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errProvider is a test-only Provider whose Source fails with a plain error
// (not a *ConfigError). No in-repo provider does this — bytes/file/URL all
// pre-wrap their failures in *ConfigError — so it is the only way to exercise
// Parse's raw-error-wrap fallback branch.
type errProvider struct{ err error }

func (p errProvider) Source(context.Context) (config.Source, error) { return nil, p.err }

// TestParseProviderRawError covers Parse's fallback for a Provider.Source that
// returns a non-*ConfigError: Parse must wrap it in a *ConfigError while
// preserving the original cause. (The *ConfigError-preserving branch is already
// covered by the FromFile-missing-path case in TestProviders.)
func TestParseProviderRawError(t *testing.T) {
	t.Parallel()

	raw := errors.New("provider exploded")
	d, err := config.Parse(t.Context(), errProvider{err: raw})

	assert.Nil(t, d)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce, "a raw provider error must be wrapped in *ConfigError")
	assert.ErrorIs(t, err, raw, "the original cause must be preserved for unwrapping")
}
