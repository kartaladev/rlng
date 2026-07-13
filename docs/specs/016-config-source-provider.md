# Spec 016 — `config.Parse(ctx, Provider)` source abstraction

- **Status:** Draft (design brainstormed & approved 2026-07-13; decisions D1–D10 below)
- **Date:** 2026-07-13
- **Builds on:** Spec 011 (config path-safety parity, `LoadFile` trust boundary),
  the config parse/decode layer (`config/parse.go`), and the reference blueprint's
  `ConfigLoader[T]` pluggable-source idea (CLAUDE.md "Architecture blueprint").
- **Realized by:** Plan 016.
- **Anticipated ADRs:** ADR-0041 (source Provider abstraction; breaking replacement
  of `ParseYAML`/`ParseJSON`/`LoadFile`).

## Context

Config today is parsed through three fixed entry points — `ParseYAML([]byte)`,
`ParseJSON([]byte)`, and `LoadFile(path)` (extension-dispatched). Each is a
separate exported function hard-wired to one input shape, so a new source (a
string, an already-open reader, a remote URL) means a new top-level function or
caller-side glue. The reference blueprint always intended a **pluggable source
abstraction** (`ConfigLoader[T]`: static / parse-YAML / parse-JSON / filesystem);
this increment realizes it as a small `Provider`/`Source` pair funnelled through
a single `Parse` entry point, and adds a hardened remote-URL source. It is a
**breaking change**: the three existing functions are removed and all call sites
migrate to `Parse`.

## Goals

1. **G1 — One entry point.** `Parse(ctx, Provider) (*PipelineDef, error)` replaces
   `ParseYAML`/`ParseJSON`/`LoadFile`. A `Provider` yields a `Source` (a reader +
   a declared `SourceKind`); `Parse` decodes it with the existing strict decoders,
   preserving every current behaviour (strict unknown-field rejection, empty-doc →
   empty def, `*ConfigError` Field attribution).
2. **G2 — A provider set covering the real input shapes.** Bytes, string, an
   arbitrary reader, a file (kind inferred by extension, plus explicit-kind
   variants), and a remote URL — each a small constructor returning a `Provider`.
3. **G3 — Preload vs deferred is a first-class property.** Bytes/string providers
   hold their data (I/O, if any, done at construction); file/URL providers defer
   I/O to `Source(ctx)`. `Parse` releases resources it is handed (closes a
   deferred `*os.File` / HTTP body) without the caller managing lifecycle.
4. **G4 — Hardened remote source.** `FromYAMLURL`/`FromJSONURL` fetch config over
   HTTP with a default timeout, a response-size cap, an `http`/`https` scheme
   allowlist, and an injectable `*http.Client` — cancellable via the `ctx`.
5. **G5 — Debuggability & typed errors.** Every failure (I/O, unknown kind,
   unsupported extension, bad scheme, non-2xx, oversize) is a `*ConfigError`
   unwrapping to an exported sentinel a test can `errors.Is`; no panics on caller
   input; no global logger; pure Go, no new module dependency (`net/http` is
   stdlib).

## Non-goals

- Content **sniffing** of a raw reader's bytes (no `mimetype` dependency). Kind is
  explicit per constructor; only `FromFile` infers, and only by file extension.
- SSRF host confinement inside the library (validating the URL host against
  internal/link-local ranges). That is caller policy, enabled via
  `WithHTTPClient` (custom transport/dialer). The library documents the boundary,
  mirroring the existing `LoadFile` trust note — config is a trusted-authoring
  surface.
- Retries, caching, auth headers, or streaming/incremental decode of remote
  config (a config document is small and read once).
- A generic `ConfigLoader[T]` generic type; `Parse` returns `*PipelineDef` only.

## Hot-path / typed-error branches (test targets)

- `Parse`: each `SourceKind` (YAML, JSON) decodes correctly; an unspecified/
  unknown kind → `ErrUnknownSourceKind`; a deferred source's reader is closed
  (no leak); strict unknown-field rejection still fires through the new path;
  empty document → empty `PipelineDef` (not an error); a nested `*ConfigError`'s
  Field attribution is preserved (not shadowed).
- Providers: bytes/string/reader → correct kind + decoded def; `FromFile` infers
  YAML vs JSON by extension and errors (`ErrUnsupportedExtension`) otherwise;
  `FromYAMLFile`/`FromJSONFile` force kind; a missing file → the `os.Open` error
  (wrapped).
- URL: a 2xx body decodes; non-2xx → `ErrUnexpectedStatus`; a non-`http`/`https`
  scheme → `ErrUnsupportedScheme`; a body over the cap → `ErrMaxBytesExceeded`;
  ctx cancellation/timeout aborts the fetch; an injected client is used.

## Resolved decisions (brainstormed & approved 2026-07-13)

- **D1 — `Parse(ctx, Provider)`.** Single entry point. `ctx` is threaded through
  `Provider.Source(ctx)` so a URL fetch is cancellable/deadline-aware, consistent
  with `Pipeline.Run(ctx, sc)`; bytes/file providers ignore it. *Alt rejected:*
  ctx-free `Parse(Provider)` relying only on the client timeout — loses
  caller-driven cancellation and deadline propagation.
- **D2 — `Provider` / `Source` two-interface split.**
  `Provider interface { Source(ctx context.Context) (Source, error) }` and
  `Source interface { Reader() io.Reader; Kind() SourceKind }`. `Source(ctx)` is
  the deferral point: a preloaded provider returns immediately; a file/URL
  provider performs I/O here and may error. *Alt rejected:* a single
  `Provider.Open(ctx) (io.ReadCloser, SourceKind, error)` — fewer types but loses
  the nameable `Source` value and the clean `Reader()`/`Kind()` split.
- **D3 — `SourceKind` is an int enum with a guard zero value.**
  `KindUnspecified` (iota 0, invalid) → `KindYAML` → `KindJSON`, with a
  `String()` method. A hand-built `Source` that forgets to set kind fails loud in
  `Parse` (`ErrUnknownSourceKind`) rather than silently defaulting. *Alt
  rejected:* raw `"yaml"`/`"json"` strings — not type-safe, no zero-value guard.
- **D4 — Resource lifecycle via `io.Closer` assertion.** `Source.Reader()` returns
  a bare `io.Reader` (as sketched). `Parse` does
  `if c, ok := r.(io.Closer); ok { defer c.Close() }`, so a preloaded
  `bytes.Reader` needs no close while a deferred `*os.File`/HTTP body is released.
  The `Source` interface stays two-method; no `Close()` on it.
- **D5 — Provider set (idiomatic all-caps initialisms).**
  `FromYAMLBytes([]byte)`, `FromYAMLString(string)`, `FromJSONBytes([]byte)`,
  `FromJSONString(string)` (preloaded); `FromReader(io.Reader, SourceKind)`
  (caller-owned stream); `FromFile(path)` (deferred, kind inferred by extension —
  subsumes `LoadFile`); `FromYAMLFile(path)`, `FromJSONFile(path)` (deferred,
  explicit kind); `FromYAMLURL(url, ...URLOption)`, `FromJSONURL(url,
  ...URLOption)` (deferred, hardened).
- **D6 — Preload vs deferred is the constructor split (G3).** No decorator: bytes/
  string providers are "preloaded" (data in hand); file/URL are "deferred" (I/O in
  `Source`). `FromReader` covers a caller who has already opened a stream. *Alt
  rejected:* a `Preload(Provider)` decorator that eagerly reads any provider — YAGNI
  until a concrete need for eager file/URL loading appears.
- **D7 — URL hardening (G4).** `URLOption`: `WithHTTPClient(*http.Client)` (default
  `&http.Client{Timeout: 10 * time.Second}`) and `WithMaxBytes(int64)` (default
  **5 MiB**). `Source(ctx)`: build the request with `ctx`; reject a non-`http`/
  `https` scheme (`ErrUnsupportedScheme`) before dialing; a non-2xx response →
  `ErrUnexpectedStatus` (carrying the code); read through
  `io.LimitReader(body, max+1)` and error `ErrMaxBytesExceeded` if the cap is
  exceeded. The body is fully read into memory (config is small) so the returned
  `Source` wraps a `bytes.Reader` and the response body is closed inside
  `Source`.
- **D8 — Trust boundary (documented, not enforced).** The URL provider fetches
  whatever URL the caller passes and does not confine the host; SSRF policy is the
  caller's via `WithHTTPClient`. A trust-boundary doc comment mirrors the existing
  `LoadFile` note. Config remains a trusted-authoring surface (Spec 011).
- **D9 — Exported error sentinels.** `ErrUnknownSourceKind`,
  `ErrUnsupportedExtension` (promoting `LoadFile`'s current inline error),
  `ErrUnsupportedScheme`, `ErrUnexpectedStatus`, `ErrMaxBytesExceeded` — each the
  `Cause` of a `*ConfigError`, so `errors.Is` reaches it and the `ConfigError`
  message stays consistent. `Parse`'s decode internals reuse the existing
  `asConfigError` Field-preservation logic verbatim.
- **D10 — Breaking replacement + migration.** `ParseYAML`, `ParseJSON`, and
  `LoadFile` are **removed** (not deprecated shims). All ~46 call sites across
  `_test.go` and `examples/` migrate to `Parse(ctx, From…)`. ADR-0041 records the
  break (exported-API change → this is a deliberate architectural decision;
  library is pre-1.0). *Alt rejected:* keep deprecated shims — a smaller final
  surface was preferred over back-compat for a pre-1.0 library, per the user's
  decision.
