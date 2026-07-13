package config_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenBodyRoundTripper returns a 200 response whose body errors on Read,
// so a test can exercise the io.ReadAll failure branch in urlProvider.Source
// without relying on a flaky real connection drop.
type brokenBodyRoundTripper struct{}

func (brokenBodyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&erroringReader{}),
		Header:     make(http.Header),
	}, nil
}

// erroringReader always fails the first Read with a fixed error.
type erroringReader struct{}

var errBrokenBody = errors.New("simulated body read failure")

func (*erroringReader) Read([]byte) (int, error) { return 0, errBrokenBody }

// TestURLProviders covers FromYAMLURL/FromJSONURL and their hardening: scheme
// allowlist (checked before dialing), non-2xx status, size cap, ctx
// cancellation, and the injectable HTTP client.
func TestURLProviders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		newProvider func(t *testing.T) config.Provider
		ctx         func(ctx context.Context) context.Context // nil means identity
		assert      func(t *testing.T, d *config.PipelineDef, err error)
	}{
		{
			name: "2xx YAML body decodes",
			newProvider: func(t *testing.T) config.Provider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(oneStageYAML))
				}))
				t.Cleanup(srv.Close)
				return config.FromYAMLURL(srv.URL, config.WithHTTPClient(srv.Client()))
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name: "2xx JSON body decodes",
			newProvider: func(t *testing.T) config.Provider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(oneStageJSON))
				}))
				t.Cleanup(srv.Close)
				return config.FromJSONURL(srv.URL, config.WithHTTPClient(srv.Client()))
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "base", d.Stages[0].Name)
			},
		},
		{
			name: "non-2xx is ErrUnexpectedStatus",
			newProvider: func(t *testing.T) config.Provider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
				t.Cleanup(srv.Close)
				return config.FromYAMLURL(srv.URL, config.WithHTTPClient(srv.Client()))
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, config.ErrUnexpectedStatus)
			},
		},
		{
			name: "unsupported scheme is ErrUnsupportedScheme without dialing",
			newProvider: func(t *testing.T) config.Provider {
				return config.FromYAMLURL("ftp://example.invalid/x")
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, config.ErrUnsupportedScheme)
			},
		},
		{
			name: "oversize body is ErrMaxBytesExceeded",
			newProvider: func(t *testing.T) config.Provider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(oneStageYAML)) // well over 16 bytes
				}))
				t.Cleanup(srv.Close)
				return config.FromYAMLURL(srv.URL, config.WithHTTPClient(srv.Client()), config.WithMaxBytes(16))
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, config.ErrMaxBytesExceeded)
			},
		},
		{
			name: "malformed URL is a ConfigError wrapping the parse error",
			newProvider: func(t *testing.T) config.Provider {
				// A raw ASCII control character makes net/url.Parse itself
				// fail (before any dial), distinct from the scheme check.
				return config.FromYAMLURL("http://example.invalid/\x7f")
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "body read failure is a ConfigError",
			newProvider: func(t *testing.T) config.Provider {
				client := &http.Client{Transport: brokenBodyRoundTripper{}}
				return config.FromYAMLURL("http://example.invalid/x", config.WithHTTPClient(client))
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, errBrokenBody)
			},
		},
		{
			name: "cancelled ctx aborts the fetch",
			newProvider: func(t *testing.T) config.Provider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(oneStageYAML))
				}))
				t.Cleanup(srv.Close)
				return config.FromYAMLURL(srv.URL, config.WithHTTPClient(srv.Client()))
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, d *config.PipelineDef, err error) {
				require.Nil(t, d)
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}

			d, err := config.Parse(ctx, tc.newProvider(t))
			tc.assert(t, d, err)
		})
	}
}
