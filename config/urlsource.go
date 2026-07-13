package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// defaultURLMaxBytes is the default response body cap (WithMaxBytes) for a
// URL provider: 5 MiB.
const defaultURLMaxBytes int64 = 5 << 20

// defaultURLTimeout is the default http.Client timeout (WithHTTPClient) for a
// URL provider.
const defaultURLTimeout = 10 * time.Second

// ErrUnsupportedScheme is the Cause of the ConfigError a URL provider returns
// when the URL scheme is neither "http" nor "https". Checked before any
// request is made.
var ErrUnsupportedScheme = errors.New("config: unsupported URL scheme")

// ErrUnexpectedStatus is the Cause of the ConfigError a URL provider returns
// when the HTTP response status is not 2xx.
var ErrUnexpectedStatus = errors.New("config: unexpected HTTP status")

// ErrMaxBytesExceeded is the Cause of the ConfigError a URL provider returns
// when the response body exceeds the configured maximum (default 5 MiB; see
// WithMaxBytes).
var ErrMaxBytesExceeded = errors.New("config: source exceeds max bytes")

// urlProvider is a deferred provider that fetches a config document over
// HTTP(S) at Parse time.
type urlProvider struct {
	rawURL   string
	kind     SourceKind
	client   *http.Client
	maxBytes int64
}

// URLOption configures a URL provider built by FromYAMLURL/FromJSONURL.
type URLOption func(*urlProvider)

// WithHTTPClient sets the http.Client used to fetch the config (default:
// &http.Client{Timeout: 10s}). A nil client is ignored.
//
// Inject a client with a restricted Transport/DialContext to confine which
// hosts may be fetched — this is the mechanism for SSRF confinement; the
// provider itself does not validate the host.
func WithHTTPClient(c *http.Client) URLOption {
	return func(p *urlProvider) {
		if c != nil {
			p.client = c
		}
	}
}

// WithMaxBytes caps the response body size (default 5 MiB). A non-positive
// value is ignored.
func WithMaxBytes(n int64) URLOption {
	return func(p *urlProvider) {
		if n > 0 {
			p.maxBytes = n
		}
	}
}

// newURLProvider builds a urlProvider with defaults applied, then opts, then
// a default client if none was supplied.
func newURLProvider(rawURL string, kind SourceKind, opts []URLOption) Provider {
	p := &urlProvider{rawURL: rawURL, kind: kind, maxBytes: defaultURLMaxBytes}
	for _, o := range opts {
		o(p)
	}
	if p.client == nil {
		p.client = &http.Client{Timeout: defaultURLTimeout}
	}
	return p
}

// FromYAMLURL returns a Provider that fetches a YAML config over HTTP(S) at
// Parse time. Only http/https URLs are allowed (ErrUnsupportedScheme
// otherwise, checked before dialing); the response must be 2xx
// (ErrUnexpectedStatus otherwise) and within the size cap (WithMaxBytes;
// ErrMaxBytesExceeded otherwise). The fetch honors the ctx passed to Parse
// and the response body is read fully into memory and closed before Source
// returns, so Parse has nothing left to close.
//
// Trust boundary: the provider fetches exactly the URL given and does not
// validate the host against internal/link-local ranges. SSRF confinement is
// the caller's responsibility via WithHTTPClient (inject a client with a
// restricted transport/dialer). Config is a trusted-authoring surface; do not
// build rawURL from untrusted input.
func FromYAMLURL(rawURL string, opts ...URLOption) Provider {
	return newURLProvider(rawURL, KindYAML, opts)
}

// FromJSONURL is FromYAMLURL for a JSON config. See FromYAMLURL for the
// hardening behavior and trust boundary.
func FromJSONURL(rawURL string, opts ...URLOption) Provider {
	return newURLProvider(rawURL, KindJSON, opts)
}

// Source fetches the URL and returns an in-memory bytesSource. See
// FromYAMLURL for the hardening behavior this implements.
func (p *urlProvider) Source(ctx context.Context) (Source, error) {
	u, err := url.Parse(p.rawURL)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %q", ErrUnsupportedScheme, u.Scheme)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.rawURL, nil)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, p.maxBytes+1))
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	if int64(len(data)) > p.maxBytes {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %d", ErrMaxBytesExceeded, p.maxBytes)}
	}

	return bytesSource{data, p.kind}, nil // in-memory; body already closed above
}
