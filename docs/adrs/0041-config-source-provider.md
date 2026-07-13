# ADR-0041 — `config.Parse(ctx, Provider)` source abstraction

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 016 (docs/specs/016-config-source-provider.md) / Plan
  016, realizing the reference blueprint's `ConfigLoader[T]` pluggable-source
  idea (CLAUDE.md "Architecture blueprint") that `config` had left
  unrealized.

## Context

Config was parsed through three fixed, independently-hardcoded entry points:
`ParseYAML([]byte)`, `ParseJSON([]byte)`, and `LoadFile(path)`
(extension-dispatched to one of the first two). Each is wired to exactly one
input shape, so a new source — a string, an already-open `io.Reader`, a
remote URL — meant either a new top-level function or caller-side glue
duplicating the strict-decode logic. The reference blueprint always intended
a pluggable source abstraction (`ConfigLoader[T]`: static / parse-YAML /
parse-JSON / filesystem); this increment realizes an equivalent for `rlng`'s
`config` package as a small `Provider`/`Source` pair funnelled through a
single `Parse` entry point, and — in a later task of the same plan — adds a
hardened remote-URL source. This is a **breaking change**: the library is
pre-1.0, and the three existing functions are removed rather than deprecated
(Spec 016 D10), with all call sites migrated to `Parse`.

This ADR is written alongside Plan 016 Task 1, which lands the core
(`Provider`/`Source`, `SourceKind`, `Parse`, the reader-based decoders, and
the preloaded bytes/string/reader providers) while **keeping**
`ParseYAML`/`ParseJSON`/`LoadFile` working so the build stays green; the
breaking removal is Task 4 of the same plan.

## Decision

- **`Parse(ctx, Provider)` as the single entry point (Spec 016 D1).**
  `Provider.Source(ctx)` threads `ctx` through so a deferred fetch (a URL, in
  a later task) is cancellable/deadline-aware, consistent with
  `Pipeline.Run(ctx, sc)`. Preloaded providers (bytes/string) simply ignore
  it. The alternative — a ctx-free `Parse(Provider)` relying only on a
  client-side timeout — was rejected: it loses caller-driven cancellation and
  deadline propagation, which the rest of the library already treats as a
  first-class concern.

- **`Provider`/`Source` two-interface split (Spec 016 D2).**
  `Provider interface { Source(ctx context.Context) (Source, error) }` and
  `Source interface { Reader() io.Reader; Kind() SourceKind }`. `Source(ctx)`
  is the deferral point: a preloaded provider returns immediately with no
  error possible; a file/URL provider (later tasks) performs I/O here and may
  fail. The single-method alternative — `Provider.Open(ctx) (io.ReadCloser,
  SourceKind, error)` — was rejected: fewer types, but it collapses the
  nameable `Source` value and its clean `Reader()`/`Kind()` split into one
  three-way return, which is harder to test and to extend (e.g. a future
  `Source` capability without breaking `Open`'s signature).

- **`SourceKind` is an int enum with a guard zero value (Spec 016 D3).**
  `KindUnspecified` (iota 0, invalid) → `KindYAML` → `KindJSON`, with a
  `String()` method rendering `"yaml"`/`"json"`/`"unspecified"` (the default
  case also covers any future out-of-range value). A hand-built `Source` that
  forgets to set a kind therefore fails loud in `Parse`
  (`ErrUnknownSourceKind`, wrapped in a `*ConfigError`) rather than silently
  defaulting to a format. Raw `"yaml"`/`"json"` strings were rejected as not
  type-safe and without a natural zero-value guard.

- **Resource lifecycle via an `io.Closer` type assertion (Spec 016 D4).**
  `Source.Reader()` returns a bare `io.Reader`; `Parse` does
  `if c, ok := r.(io.Closer); ok { defer c.Close() }`. A preloaded
  `bytes.Reader` needs no close and the assertion is a no-op; a deferred
  provider's `*os.File` or HTTP response body (later tasks) is released
  automatically. `FromReader` (this task) deliberately hides the caller's
  reader behind a `nopReader{ io.Reader }` wrapper so this same assertion
  never observes a caller-owned `Close` — the caller keeps lifecycle
  ownership of a stream it opened itself. The `Source` interface stays
  two-method; no `Close()` was added to it, keeping the lifecycle decision in
  one place (`Parse`) rather than duplicated across every `Source`
  implementation.

- **Provider set, split preloaded vs. deferred at the constructor (Spec 016
  D5/D6).** This task ships the preloaded half: `FromYAMLBytes([]byte)`,
  `FromJSONBytes([]byte)`, `FromYAMLString(string)`, `FromJSONString(string)`
  (data held in hand, no I/O) and `FromReader(io.Reader, SourceKind)` (a
  caller-owned stream, kind explicit since Task 1 does no content sniffing).
  `FromFile`/`FromYAMLFile`/`FromJSONFile` (Task 2) and
  `FromYAMLURL`/`FromJSONURL` (Task 3) are deferred providers that perform
  I/O inside `Source(ctx)` instead. There is no `Preload(Provider)` decorator
  to eagerly read a deferred provider — rejected as YAGNI until a concrete
  need for eager file/URL loading appears; the preload/deferred property is
  simply which constructor a caller chose.

- **Breaking removal of `ParseYAML`/`ParseJSON`/`LoadFile` (Spec 016 D10),
  landing in this increment's Plan 016 Task 4.** Not deprecated shims: they
  are deleted outright and every call site (`_test.go`, `examples/`)
  migrates to `Parse(ctx, From…)`. Tasks 1–3 keep them as thin delegators to
  the new `decodeYAML`/`decodeJSON` so each task stays a green, working unit;
  Task 4 is the atomic cut once every migration is in place. A pre-1.0
  library was judged better served by a smaller, single-entry-point surface
  than by carrying permanent back-compat shims — the user's explicit call
  per Spec 016 D10.

## Consequences

- **One entry point.** All config parsing funnels through
  `Parse(ctx, Provider)`; the decode internals (`decodeYAML`/`decodeJSON`)
  are written once, against an `io.Reader`, and reused by every provider —
  no duplicated strict-decode logic between the old three functions and the
  new path.
- **Breaking exported-API change**, deliberate and recorded here because
  `rlng` is pre-1.0 (no major-version bump required, but the change is
  architecturally significant enough to warrant this ADR per the Library
  quality gates in CLAUDE.md). Every caller of `ParseYAML`/`ParseJSON`/
  `LoadFile` must migrate; Plan 016 Task 4 enumerates and performs that
  migration in one atomic commit.
- **`net/http` becomes reachable from `config`** (Task 3, URL providers) but
  it is standard library — no new module dependency, preserving the
  "dependency set minimal" quality gate.
- **The URL trust boundary is caller policy, not library policy (Spec 016
  D8).** The eventual `FromYAMLURL`/`FromJSONURL` fetch exactly the URL
  given and do not confine the host (no SSRF allowlist baked in); a caller
  wanting host confinement injects a restricted `*http.Client` via
  `WithHTTPClient`. This mirrors the existing `LoadFile` trust note — config
  is treated as a trusted-authoring surface (Spec 011), and the library
  documents the boundary rather than enforcing one policy for all callers.
- **Debuggability preserved.** Every failure surfaces as a `*ConfigError`
  wrapping an exported sentinel (`ErrUnknownSourceKind` in this task; further
  sentinels in Tasks 2–3), reusing `asConfigError`'s Field-preservation
  verbatim so a nested decode error's stage/field attribution is never
  shadowed by an outer wrap.

## Traceability

Spec: 016 (docs/specs/016-config-source-provider.md)
Plan: 016 (docs/plans/016-config-source-provider.md)
Related: the reference blueprint's `ConfigLoader[T]` (CLAUDE.md "Architecture
blueprint"), Spec 011 (config path-safety parity / `LoadFile` trust
boundary).
