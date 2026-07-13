# `config.Parse(ctx, Provider)` source abstraction — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `config.ParseYAML`/`ParseJSON`/`LoadFile` with a single `Parse(ctx, Provider)` entry point over a `Provider`/`Source` abstraction, and add bytes/string/reader/file/URL providers (URL hardened).

**Architecture:** A `Provider` yields a `Source` (an `io.Reader` + a `SourceKind`); `Parse` closes the reader if it is an `io.Closer`, then decodes via the existing strict decoders (refactored to take an `io.Reader`). Preloaded providers (bytes/string) hold their data; deferred providers (file/URL) do I/O in `Source(ctx)`. Realizes Spec 016 (D1–D10); records ADR-0041.

**Tech Stack:** Go 1.25, stdlib (`io`, `bytes`, `os`, `context`, `net/http`, `net/url`, `time`, `path/filepath`), `gopkg.in/yaml.v3`. **No new module dependency.**

## Global Constraints

- Go 1.25+; pure Go, no cgo. **No new module dependency** (`net/http` etc. are stdlib). Library must not panic/os.Exit/log.Fatal on caller input — return typed errors; no global logger.
- Blackbox tests only (`package config_test`); mandatory `table-test` assert-closure form for ≥2 same-SUT cases (NO `want`/`wantErr` fields); `t.Context()` over `context.Background()`. Export any sentinel a test must `errors.Is`.
- Every exported symbol has a godoc comment. Target ≥85% coverage on `config`; **every hot-path and typed-error branch covered** — here the hot path is `Parse` (each kind, unknown-kind, closer-close) and every provider's `Source` error branch (unsupported extension, bad scheme, non-2xx, oversize, os.Open error).
- **Breaking change:** `ParseYAML`/`ParseJSON`/`LoadFile` are removed in Task 4 → ADR-0041 (exported-API change = architectural decision; library is pre-1.0). Tasks 1–3 keep them working so each task is a green unit; Task 4 is the atomic cut.
- Errors are `*config.ConfigError` unwrapping to an exported sentinel; the decode path reuses the existing `asConfigError` Field-preservation verbatim.
- URL providers: injectable `*http.Client` (default `&http.Client{Timeout: 10*time.Second}`), default **5 MiB** size cap, `http`/`https` scheme only, ctx-cancellable.
- Traceability: every `feat` commit carries `Spec: 016`, `Plan: 016`, `ADR: 0041`, ending `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Implements Spec 016 (D1–D10).

## Reused machinery (do not reinvent)

- `ConfigError{Stage, Field string, Cause error}` + `Unwrap` — `config/errors.go`. Wrap every failure.
- `asConfigError(err) *ConfigError` — `config/parse.go` — returns the first `*ConfigError` in a chain (preserves nested Field attribution). Reused by `Parse` and by the refactored decoders.
- The strict-decode bodies currently in `ParseYAML`/`ParseJSON` (`yaml.NewDecoder(...).KnownFields(true)`, `json.NewDecoder(...).DisallowUnknownFields()`, EOF→empty) — refactored to take an `io.Reader`.
- `PipelineDef` (`config/def.go`) — the decode target, unchanged.

## File structure

- `config/source.go` (NEW) — `SourceKind` + `String()`, `Provider`/`Source` interfaces, the exported error sentinels.
- `config/parse.go` (REWRITE) — `Parse(ctx, Provider)`, unexported `decodeYAML(io.Reader)`/`decodeJSON(io.Reader)`. Tasks 1–3: `ParseYAML`/`ParseJSON`/`LoadFile` kept as thin delegators. Task 4: removed.
- `config/providers.go` (NEW) — preloaded (bytes/string/reader) + file providers.
- `config/urlsource.go` (NEW) — `FromYAMLURL`/`FromJSONURL` + `URLOption` (`WithHTTPClient`/`WithMaxBytes`).
- `config/source_test.go`, `config/providers_test.go`, `config/urlsource_test.go` (NEW) — blackbox tests.
- `config/doc.go`, `README.md` (MODIFY, Task 4) — package doc + usage snippets.
- `docs/adrs/0041-config-source-provider.md` (NEW, Task 1).

---

## Task 1: `Provider`/`Source` core, `Parse`, decode refactor, preloaded providers + ADR-0041

The foundation: the interfaces, `SourceKind`, sentinels, `Parse`, the reader-based decoders, and the bytes/string/reader providers. `ParseYAML`/`ParseJSON` are refactored to delegate to the new decoders (kept working); `LoadFile` untouched.

**Files:** Create `config/source.go`, `config/providers.go`, `config/source_test.go`, `config/providers_test.go`, `docs/adrs/0041-config-source-provider.md`; Modify `config/parse.go`.

**Interfaces (Produces):**
```go
type SourceKind int
const ( KindUnspecified SourceKind = iota; KindYAML; KindJSON )
func (k SourceKind) String() string

type Provider interface { Source(ctx context.Context) (Source, error) }
type Source   interface { Reader() io.Reader; Kind() SourceKind }

func Parse(ctx context.Context, p Provider) (*PipelineDef, error)

func FromYAMLBytes(data []byte) Provider
func FromJSONBytes(data []byte) Provider
func FromYAMLString(s string) Provider
func FromJSONString(s string) Provider
func FromReader(r io.Reader, kind SourceKind) Provider

var ErrUnknownSourceKind = errors.New("config: unknown source kind")
```

- [ ] **Step 1: Write failing tests** (`config/source_test.go` + `config/providers_test.go`), blackbox `package config_test`, assert-closure tables driven through the public API:
  - `Parse` over `FromYAMLBytes`/`FromJSONBytes`/`FromYAMLString`/`FromJSONString` decodes a valid one-stage def (assert `d.Stages[0].Name`).
  - `FromReader(r, KindYAML)` decodes; `FromReader(r, KindJSON)` decodes.
  - strict decoding still fires: an unknown field via `FromYAMLBytes` → error (decode error, not silent).
  - empty document via `FromYAMLBytes([]byte(""))` → empty `*PipelineDef`, no error.
  - a `Source` with `KindUnspecified` (build one via `FromReader(r, KindUnspecified)`) → `errors.Is(err, config.ErrUnknownSourceKind)` inside a `*ConfigError`.
  - `SourceKind.String()`: `KindYAML`→"yaml", `KindJSON`→"json", `KindUnspecified`→"unspecified".
  Example table shape:
  ```go
  func TestParse(t *testing.T) {
      const yamlDoc = "stages:\n  - name: base\n    type: single-expr\n    expr: 1+1\n"
      const jsonDoc = `{"stages":[{"name":"base","type":"single-expr","expr":{"expr":"1+1"}}]}`
      cases := []struct {
          name     string
          provider config.Provider
          assert   func(t *testing.T, d *config.PipelineDef, err error)
      }{
          {
              name:     "FromYAMLBytes decodes",
              provider: config.FromYAMLBytes([]byte(yamlDoc)),
              assert: func(t *testing.T, d *config.PipelineDef, err error) {
                  require.NoError(t, err)
                  require.Len(t, d.Stages, 1)
                  assert.Equal(t, "base", d.Stages[0].Name)
              },
          },
          {
              name:     "unspecified kind is ErrUnknownSourceKind",
              provider: config.FromReader(strings.NewReader(yamlDoc), config.KindUnspecified),
              assert: func(t *testing.T, d *config.PipelineDef, err error) {
                  assert.ErrorIs(t, err, config.ErrUnknownSourceKind)
                  var ce *config.ConfigError
                  assert.ErrorAs(t, err, &ce)
              },
          },
          // ...FromJSONBytes, FromYAMLString, unknown-field-rejected, empty-doc...
      }
      for _, tc := range cases {
          t.Run(tc.name, func(t *testing.T) {
              d, err := config.Parse(t.Context(), tc.provider)
              tc.assert(t, d, err)
          })
      }
  }
  ```

- [ ] **Step 2: Run to verify failure** — `go test ./config/ -run 'TestParse|TestSourceKind' -v` (FAIL: undefined `config.Parse`/`config.FromYAMLBytes`/…).

- [ ] **Step 3: Implement `config/source.go`:**
  ```go
  package config

  import (
      "context"
      "errors"
      "io"
  )

  // SourceKind is the wire format of a config Source. The zero value
  // (KindUnspecified) is invalid: Parse rejects it, so a Source that forgets to
  // declare a kind fails loud rather than silently defaulting to a format.
  type SourceKind int

  const (
      KindUnspecified SourceKind = iota // invalid; zero-value guard
      KindYAML
      KindJSON
  )

  // String renders the kind as "yaml"/"json", or "unspecified" for the zero value.
  func (k SourceKind) String() string {
      switch k {
      case KindYAML:
          return "yaml"
      case KindJSON:
          return "json"
      default:
          return "unspecified"
      }
  }

  // Provider yields a Source to parse. A preloaded provider (bytes/string)
  // returns immediately; a deferred provider (file/URL) performs I/O in Source and
  // may fail. ctx cancels/deadlines that deferred I/O.
  type Provider interface {
      Source(ctx context.Context) (Source, error)
  }

  // Source is one config document to decode: a reader over its bytes and the
  // declared format. If Reader returns a value that also implements io.Closer
  // (e.g. an *os.File), Parse closes it after decoding.
  type Source interface {
      Reader() io.Reader
      Kind() SourceKind
  }

  // ErrUnknownSourceKind is the Cause of the ConfigError Parse returns when a
  // Source declares a kind it cannot decode (including the KindUnspecified zero).
  var ErrUnknownSourceKind = errors.New("config: unknown source kind")
  ```
  Rewrite `config/parse.go` — add `Parse` + `decodeYAML`/`decodeJSON`, and delegate the kept `ParseYAML`/`ParseJSON` to them:
  ```go
  // Parse decodes a PipelineDef from a Provider's Source. The Source's reader is
  // closed after decoding if it implements io.Closer. Unknown fields are rejected
  // (matching the underlying YAML/JSON strict decoders); an empty document decodes
  // to an empty PipelineDef (Build then rejects the zero-stage case). Failures are
  // a *ConfigError; a Source with an unrecognized Kind is ErrUnknownSourceKind.
  func Parse(ctx context.Context, p Provider) (*PipelineDef, error) {
      src, err := p.Source(ctx)
      if err != nil {
          if ce := asConfigError(err); ce != nil {
              return nil, ce
          }
          return nil, &ConfigError{Cause: err}
      }
      r := src.Reader()
      if c, ok := r.(io.Closer); ok {
          defer c.Close()
      }
      switch src.Kind() {
      case KindYAML:
          return decodeYAML(r)
      case KindJSON:
          return decodeJSON(r)
      default:
          return nil, &ConfigError{Cause: fmt.Errorf("%w: %s", ErrUnknownSourceKind, src.Kind())}
      }
  }

  func decodeYAML(r io.Reader) (*PipelineDef, error) {
      var d PipelineDef
      dec := yaml.NewDecoder(r)
      dec.KnownFields(true)
      if err := dec.Decode(&d); err != nil {
          if errors.Is(err, io.EOF) {
              return &d, nil
          }
          if ce := asConfigError(err); ce != nil {
              return nil, ce
          }
          return nil, &ConfigError{Cause: err}
      }
      return &d, nil
  }

  func decodeJSON(r io.Reader) (*PipelineDef, error) {
      var d PipelineDef
      dec := json.NewDecoder(r)
      dec.DisallowUnknownFields()
      if err := dec.Decode(&d); err != nil {
          if errors.Is(err, io.EOF) {
              return &d, nil
          }
          if ce := asConfigError(err); ce != nil {
              return nil, ce
          }
          return nil, &ConfigError{Cause: err}
      }
      return &d, nil
  }

  // ParseYAML ... (existing godoc kept)
  func ParseYAML(data []byte) (*PipelineDef, error) { return decodeYAML(bytes.NewReader(data)) }
  // ParseJSON ... (existing godoc kept)
  func ParseJSON(data []byte) (*PipelineDef, error) { return decodeJSON(bytes.NewReader(data)) }
  ```
  (Keep `LoadFile` and `asConfigError` unchanged. Ensure imports: `bytes`, `context`, `encoding/json`, `errors`, `fmt`, `io`, `os`, `path/filepath`, `strings`, `yaml`.)
  Implement `config/providers.go` (preloaded + reader):
  ```go
  package config

  import (
      "bytes"
      "context"
      "io"
  )

  type bytesSource struct {
      data []byte
      kind SourceKind
  }

  func (s bytesSource) Reader() io.Reader { return bytes.NewReader(s.data) }
  func (s bytesSource) Kind() SourceKind  { return s.kind }

  type staticProvider struct{ src Source }

  func (p staticProvider) Source(context.Context) (Source, error) { return p.src, nil }

  // FromYAMLBytes returns a Provider for an in-memory YAML document. The bytes are
  // read on each Parse; the provider performs no I/O.
  func FromYAMLBytes(data []byte) Provider { return staticProvider{bytesSource{data, KindYAML}} }

  // FromJSONBytes returns a Provider for an in-memory JSON document.
  func FromJSONBytes(data []byte) Provider { return staticProvider{bytesSource{data, KindJSON}} }

  // FromYAMLString is FromYAMLBytes for a string.
  func FromYAMLString(s string) Provider { return FromYAMLBytes([]byte(s)) }

  // FromJSONString is FromJSONBytes for a string.
  func FromJSONString(s string) Provider { return FromJSONBytes([]byte(s)) }

  // nopReader hides any io.Closer on a caller-owned reader so Parse does not close
  // a stream the caller still owns.
  type nopReader struct{ io.Reader }

  type readerSource struct {
      r    io.Reader
      kind SourceKind
  }

  func (s readerSource) Reader() io.Reader { return s.r }
  func (s readerSource) Kind() SourceKind  { return s.kind }

  // FromReader returns a Provider that decodes from a caller-supplied reader as
  // the given kind. The caller owns the reader's lifecycle: Parse does not close
  // it even if it implements io.Closer.
  func FromReader(r io.Reader, kind SourceKind) Provider {
      return staticProvider{readerSource{nopReader{r}, kind}}
  }
  ```

- [ ] **Step 4: Run tests to pass** — `go test ./config/ -run 'TestParse|TestSourceKind|TestProviders' -race -v`, then `go test ./config/ -race`.

- [ ] **Step 5: ADR-0041** (`docs/adrs/0041-config-source-provider.md`, Nygard): Context = three fixed parse entry points, no pluggable source (blueprint's `ConfigLoader[T]` unrealized); Spec 016. Decision = `Parse(ctx, Provider)` + `Provider`/`Source` split (D1/D2), `SourceKind` int enum with guard zero (D3), `io.Closer` lifecycle (D4), the provider set (D5), preload-vs-deferred as the constructor split (D6), and the **breaking removal** of `ParseYAML`/`ParseJSON`/`LoadFile` (D10) landing in this increment (Plan 016 Task 4). Consequences = one entry point, breaking API change (pre-1.0), `net/http` reachable but stdlib (no new dep), URL trust boundary is caller policy (D8). Status: Accepted. Cite Spec 016 / Plan 016.

- [ ] **Step 6: Commit** — `feat(config): source Provider abstraction + Parse(ctx, Provider) core`; trailers `Spec: 016`, `Plan: 016`, `ADR: 0041`. Files: `config/source.go`, `config/providers.go`, `config/parse.go`, the two new test files, the ADR.

---

## Task 2: File providers (`FromFile`, `FromYAMLFile`, `FromJSONFile`)

Deferred file sources: `FromFile` infers kind by extension (subsuming `LoadFile`); the explicit-kind variants force it. `Parse` closes the `*os.File` via its `io.Closer` path.

**Files:** Modify `config/providers.go`, `config/providers_test.go`.

**Interfaces (Produces):**
```go
func FromFile(path string) Provider     // kind inferred by extension
func FromYAMLFile(path string) Provider
func FromJSONFile(path string) Provider
var ErrUnsupportedExtension = errors.New("config: unsupported file extension")
```

- [ ] **Step 1: Failing tests** — write a temp file via `t.TempDir()` + `os.WriteFile`, then:
  - `FromFile("x.yaml")` and `FromFile("x.yml")` decode as YAML; `FromFile("x.json")` decodes as JSON.
  - `FromFile("x.txt")` → `errors.Is(err, config.ErrUnsupportedExtension)` inside a `*ConfigError` (assert Parse returns it — the extension check is in `Source`).
  - `FromYAMLFile("x.txt")` decodes the file as YAML regardless of extension (write YAML content to `x.txt`).
  - a missing path (`FromFile(filepath.Join(dir,"nope.yaml"))`) → a `*ConfigError` wrapping the `os.Open` error (assert `errors.Is(err, os.ErrNotExist)`).
  - **closer is closed:** after a successful `Parse` of a file provider, the file handle is released — assert by `os.Remove`-ing the temp file on Windows-safe platforms is flaky; instead assert no error on a second `Parse` of the same path (handle not leaked/locked) OR simply that Parse succeeds (the `io.Closer` branch is covered by the file test running under `-race` with no leak warning). Keep it simple: one case that `Parse`s a file twice in a row and both succeed.
  Fold these into the existing `providers_test.go` table (assert-closure form).

- [ ] **Step 2: Run to verify failure** — `go test ./config/ -run TestProviders -v` (FAIL: undefined `FromFile`).

- [ ] **Step 3: Implement** — append to `config/providers.go` (add imports `errors`, `fmt`, `os`, `path/filepath`, `strings`):
  ```go
  // ErrUnsupportedExtension is the Cause of the ConfigError FromFile's Source
  // returns when a path's extension is neither .yaml/.yml nor .json.
  var ErrUnsupportedExtension = errors.New("config: unsupported file extension")

  type fileProvider struct {
      path string
      kind SourceKind // KindUnspecified => infer by extension
  }

  type fileSource struct {
      f    *os.File
      kind SourceKind
  }

  func (s fileSource) Reader() io.Reader { return s.f } // *os.File is an io.Closer; Parse closes it
  func (s fileSource) Kind() SourceKind  { return s.kind }

  func (p fileProvider) Source(context.Context) (Source, error) {
      kind := p.kind
      if kind == KindUnspecified {
          switch strings.ToLower(filepath.Ext(p.path)) {
          case ".yaml", ".yml":
              kind = KindYAML
          case ".json":
              kind = KindJSON
          default:
              return nil, &ConfigError{Cause: fmt.Errorf("%w: %q", ErrUnsupportedExtension, filepath.Ext(p.path))}
          }
      }
      f, err := os.Open(p.path)
      if err != nil {
          return nil, &ConfigError{Cause: err}
      }
      return fileSource{f, kind}, nil
  }

  // FromFile returns a Provider that opens path at Parse time and decodes it by
  // extension: .yaml/.yml as YAML, .json as JSON, else ErrUnsupportedExtension.
  //
  // Trust boundary: path is passed to os.Open as-is (no base-directory
  // confinement, symlink check, or size limit). Pipeline definitions are meant to
  // be developer/operator-authored (trusted); do not pass a path derived from
  // untrusted input.
  func FromFile(path string) Provider { return fileProvider{path, KindUnspecified} }

  // FromYAMLFile returns a Provider that opens path at Parse time and decodes it
  // as YAML regardless of extension. Same trust boundary as FromFile.
  func FromYAMLFile(path string) Provider { return fileProvider{path, KindYAML} }

  // FromJSONFile returns a Provider that opens path at Parse time and decodes it
  // as JSON regardless of extension. Same trust boundary as FromFile.
  func FromJSONFile(path string) Provider { return fileProvider{path, KindJSON} }
  ```

- [ ] **Step 4: Tests pass** — `go test ./config/ -run TestProviders -race`, then `go test ./config/ -race`.

- [ ] **Step 5: Commit** — `feat(config): file source providers (FromFile ext-inference + explicit-kind)`; trailers `Spec: 016`, `Plan: 016`, `ADR: 0041`.

---

## Task 3: URL providers (`FromYAMLURL`/`FromJSONURL`) + hardening

Deferred remote sources: hardened HTTP GET (injectable client, timeout, size cap, scheme allowlist), body read fully into memory and closed inside `Source`, ctx-cancellable.

**Files:** Create `config/urlsource.go`, `config/urlsource_test.go`.

**Interfaces (Produces):**
```go
func FromYAMLURL(rawURL string, opts ...URLOption) Provider
func FromJSONURL(rawURL string, opts ...URLOption) Provider
type URLOption func(*urlProvider)
func WithHTTPClient(c *http.Client) URLOption
func WithMaxBytes(n int64) URLOption
var ErrUnsupportedScheme = errors.New("config: unsupported URL scheme")
var ErrUnexpectedStatus  = errors.New("config: unexpected HTTP status")
var ErrMaxBytesExceeded  = errors.New("config: source exceeds max bytes")
```

- [ ] **Step 1: Failing tests** (`config/urlsource_test.go`) using `net/http/httptest`, assert-closure table:
  - a `httptest.Server` returning a valid YAML doc → `Parse(t.Context(), config.FromYAMLURL(srv.URL))` decodes it (assert stage name).
  - JSON variant via `FromJSONURL`.
  - server returns 404 → `errors.Is(err, config.ErrUnexpectedStatus)`.
  - `FromYAMLURL("ftp://example.com/x")` → `errors.Is(err, config.ErrUnsupportedScheme)` (no request made).
  - server returns a body larger than a small cap set via `config.WithMaxBytes(16)` → `errors.Is(err, config.ErrMaxBytesExceeded)`.
  - ctx cancellation: a server that blocks; call with a `ctx` cancelled via `context.WithCancel` (cancel before/mid) → `Parse` returns a non-nil error (assert `err != nil`; the client's request honors ctx).
  - injected client is used: pass `config.WithHTTPClient(srv.Client())` and assert success (covers the option wiring).
  Example case:
  ```go
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
          assert.ErrorIs(t, err, config.ErrUnexpectedStatus)
      },
  }
  ```

- [ ] **Step 2: Run to verify failure** — `go test ./config/ -run TestURL -v` (FAIL: undefined `FromYAMLURL`).

- [ ] **Step 3: Implement `config/urlsource.go`:**
  ```go
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

  const defaultURLMaxBytes int64 = 5 << 20 // 5 MiB

  const defaultURLTimeout = 10 * time.Second

  // ErrUnsupportedScheme is the Cause of the ConfigError a URL provider returns
  // when the URL scheme is not http or https.
  var ErrUnsupportedScheme = errors.New("config: unsupported URL scheme")

  // ErrUnexpectedStatus is the Cause when a URL provider gets a non-2xx response.
  var ErrUnexpectedStatus = errors.New("config: unexpected HTTP status")

  // ErrMaxBytesExceeded is the Cause when a source's body exceeds the configured
  // maximum (default 5 MiB, see WithMaxBytes).
  var ErrMaxBytesExceeded = errors.New("config: source exceeds max bytes")

  type urlProvider struct {
      rawURL   string
      kind     SourceKind
      client   *http.Client
      maxBytes int64
  }

  // URLOption configures a URL provider built by FromYAMLURL/FromJSONURL.
  type URLOption func(*urlProvider)

  // WithHTTPClient sets the http.Client used to fetch the config (default:
  // &http.Client{Timeout: 10s}). A nil client is ignored. Inject a client with a
  // restricted transport/dialer to confine which hosts may be fetched (SSRF).
  func WithHTTPClient(c *http.Client) URLOption {
      return func(p *urlProvider) {
          if c != nil {
              p.client = c
          }
      }
  }

  // WithMaxBytes caps the response body size (default 5 MiB). A non-positive value
  // is ignored.
  func WithMaxBytes(n int64) URLOption {
      return func(p *urlProvider) {
          if n > 0 {
              p.maxBytes = n
          }
      }
  }

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

  // FromYAMLURL returns a Provider that fetches a YAML config over HTTP at Parse
  // time. Only http/https URLs are allowed; the response must be 2xx and within
  // the size cap (WithMaxBytes). The fetch honors the Parse ctx.
  //
  // Trust boundary: the provider fetches exactly the URL given and does not
  // validate the host against internal/link-local ranges. SSRF confinement is the
  // caller's responsibility via WithHTTPClient (a restricted transport). Config is
  // a trusted-authoring surface; do not build the URL from untrusted input.
  func FromYAMLURL(rawURL string, opts ...URLOption) Provider {
      return newURLProvider(rawURL, KindYAML, opts)
  }

  // FromJSONURL is FromYAMLURL for a JSON config.
  func FromJSONURL(rawURL string, opts ...URLOption) Provider {
      return newURLProvider(rawURL, KindJSON, opts)
  }

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
      return bytesSource{data, p.kind}, nil // in-memory; body already closed
  }
  ```

- [ ] **Step 4: Tests pass** — `go test ./config/ -run TestURL -race`, then `go test ./config/ -race`.

- [ ] **Step 5: Commit** — `feat(config): hardened URL source providers (client/timeout/size cap/scheme)`; trailers `Spec: 016`, `Plan: 016`, `ADR: 0041`.

---

## Task 4: Remove `ParseYAML`/`ParseJSON`/`LoadFile`; migrate all call sites; docs

The atomic breaking cut. Remove the three functions, migrate every caller to `Parse(ctx, From…)`, and update `doc.go` + `README`. Build goes red on removal and green once every call site is migrated — done in one task so no intermediate task is broken.

**Files:** Modify `config/parse.go` (remove the three funcs); every `_test.go` and `examples/*_test.go` that called them; `config/doc.go`; `README.md`.

**Migration mapping (apply to every call site — find them with `grep -rn 'ParseYAML\|ParseJSON\|LoadFile' --include='*.go'`):**
- `config.ParseYAML(b)` → `config.Parse(ctx, config.FromYAMLBytes(b))`
- `config.ParseJSON(b)` → `config.Parse(ctx, config.FromJSONBytes(b))`
- `config.ParseYAML([]byte(s))` → `config.Parse(ctx, config.FromYAMLString(s))` (when the arg is `[]byte("literal")`)
- `config.LoadFile(path)` → `config.Parse(ctx, config.FromFile(path))`
- Within the `config` package's own tests, the calls are unqualified (`ParseYAML(...)` → `Parse(ctx, FromYAMLBytes(...))`).
- `ctx`: in `_test.go` use `t.Context()`; in `examples/*_test.go` use `context.Background()` (examples already import `context` for `pipeline.Run`).

- [ ] **Step 1: Remove the three functions** from `config/parse.go` (delete `ParseYAML`, `ParseJSON`, `LoadFile` and their now-unused imports if any — `os`/`path/filepath`/`strings` moved to `providers.go` in Task 2, so parse.go may no longer need them; run `goimports`/`gofmt` and let the compiler flag unused imports).

- [ ] **Step 2: Run the build to see every break** — `go build ./... 2>&1` and `go vet ./... 2>&1`; the compiler lists every call site to migrate. Also `grep -rn 'ParseYAML\|ParseJSON\|LoadFile' --include='*.go'` for a complete list (ignore matches inside godoc comments you are rewriting).

- [ ] **Step 3: Migrate every call site** per the mapping table above. Touch each failing `_test.go` and `examples/*_test.go`. For a table test whose cases build a def, thread `t.Context()` into the `config.Parse` call. Keep each test's assertions unchanged — only the parse construction changes.

- [ ] **Step 4: Update docs** — `config/doc.go`: replace the `ParseYAML, ParseJSON, or LoadFile` sentence with `Parse with a Provider (e.g. FromYAMLBytes, FromFile, FromYAMLURL)`. `README.md`: update the config usage snippets (the `### Declarative pipeline → typed result` section and any `LoadFile` mention) to `config.Parse(context.Background(), config.FromYAMLString(...))`. Also fix the `config/hash.go` godoc sentence referencing `ParseYAML/ParseJSON`.

- [ ] **Step 5: Verify green** — `go build ./...`, `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op — no new dep), `go mod verify`. Confirm `grep -rn 'ParseYAML\|ParseJSON\|LoadFile' --include='*.go'` returns nothing (no stragglers, no stale godoc).

- [ ] **Step 6: Commit** — `feat(config)!: remove ParseYAML/ParseJSON/LoadFile for Parse(ctx, Provider)`; body notes the breaking change and points to ADR-0041; trailers `Spec: 016`, `Plan: 016`, `ADR: 0041`.

---

## Task 5: Whole-branch gate

- [ ] **Step 1: Whole-branch gate** over `main..HEAD`: `/code-review` (high) + `/security-review` (focus the URL provider: SSRF boundary is documented+caller-injectable, scheme allowlist enforced before dial, size cap via LimitReader, ctx honored, body closed; no panic on caller input). Fix/triage every finding; re-run affected reviews.
- [ ] **Step 2: Full gate green** — `go test ./... -race`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./...`, `go mod tidy`(no-op)/`verify` all clean.
- [ ] **Step 3: Update `docs/HANDOVER.md`** and present increment 016 for the merge/push decision (do not push without explicit approval).

## Self-review (author checklist)

- **Spec coverage:** G1→Task 1 (`Parse` + decoders). G2→Tasks 1–3 (bytes/string/reader; file; URL). G3→Task 1/2 (preload vs deferred; `io.Closer` close). G4→Task 3 (client/timeout/cap/scheme/ctx). G5→all (ConfigError sentinels, no panic, no new dep). D10 breaking removal→Task 4. ✅
- **Type consistency:** `SourceKind`/`KindYAML`/`KindJSON`/`KindUnspecified`, `Provider.Source(ctx)`, `Source.Reader()`/`Kind()`, `Parse(ctx, Provider)`, `From*`/`URLOption`/`WithHTTPClient`/`WithMaxBytes`, sentinels — used identically across Tasks 1–4. ✅
- **Reuse/altitude:** decoders refactored once and shared by `Parse` (and, transitionally, the kept `ParseYAML`/`ParseJSON`); `asConfigError` Field-preservation reused verbatim; no duplicate decode logic. ✅
- **Open item settled by test:** `FromReader` hides a caller's `io.Closer` (`nopReader`) so Parse won't close a caller-owned stream — asserted implicitly by the reader-provider case (no double-close) and stated in godoc.
