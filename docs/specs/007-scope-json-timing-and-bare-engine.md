# Spec 007 — Scope JSON codec + evaluation timing, and a mapper-less `BareEngine`

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-11
- **Post-roadmap feature** (the 5-increment roadmap is complete; this is additive, release-relevant capability toward `v0.0.1`).
- **Builds on:** Spec 002 (`stage.Scope`, the three stages), Spec 003 (`Pipeline.Run`), Spec 005 (`Engine`/`Mapper`), Spec 006 (`Scope` provenance/`Derivation`, typed getters).
- **Related ADRs:** ADR-0013 (Scope JSON envelope + always-on timing), ADR-0014 (`BareEngine`). Realized by Plan 007.
- **Supersedes** Spec 006's non-goal "Serializing the lineage (JSON export of the derivation graph)" — this spec provides that serialization as part of the Scope codec.

## Context

A `Scope` is the accumulator threaded through evaluation and the natural carrier of a
computation's **result**. Consumers need to move that result across process boundaries:
return it in a **web response**, or persist it in a **`jsonb` database column** and read it
back later (a database codec). Today the `Scope` has no serialization, so callers hand-roll
it and lose the provenance and any notion of *when* the calculation ran.

Two capabilities close that gap:

1. **A round-trippable JSON codec on `Scope`** — `MarshalJSON`/`UnmarshalJSON` — so a
   `Scope` persists to and restores from a `jsonb` column losslessly, carrying its result
   data, its evaluation timing, and (when enabled) its provenance derivations.
2. **Evaluation timing** — *when* the calculation happened and *how long* it took —
   recorded on every run and serialized with the Scope.

Separately, not every consumer wants the typed-`Mapper` projection that `Engine[I, R]`
performs. A **`BareEngine`** runs a compiled `Pipeline` against arbitrary input and returns
the raw accumulated `map[string]any` (the Scope snapshot), skipping the mapper entirely.

Finally, the library's capabilities are spread across packages (`expr`, `stage`, `config`,
root). A dedicated **`examples/` package** of runnable `Example…` tests demonstrates a
realistic end-to-end usage of every capability, and the README points to it.

## Goals

1. **Evaluation timing (always-on).** Every pipeline run records when it started and how
   long it took, on the `Scope`. Recorded once in `Pipeline.Run` (the single choke point
   all engines flow through). Cost is two clock reads per run — too cheap to gate, so it is
   **not** opt-in. A `stage.WithClock` option injects a deterministic clock for tests.
2. **Round-trippable `Scope` JSON codec.** `Scope.MarshalJSON` / `Scope.UnmarshalJSON`
   serialize and restore an **envelope** of `{data, timing, derivations?}`. `data` is
   always present; `timing` is present after a run; `derivations` appear only when
   provenance is enabled. Restoring rebuilds `data`, `timing`, and — when derivations are
   present — the provenance state.
3. **`rlng.BareEngine`.** A mapper-less engine: `Evaluate(ctx, input any) (map[string]any,
   error)` returns the Scope snapshot; `EvaluateScope(ctx, input any) (*stage.Scope,
   error)` returns the full Scope (for timing/JSON/provenance). Reuses the existing input
   `flatten` and `WithScopeOptions`.
4. **`examples/` package.** Runnable `Example…` tests covering every capability:
   single/multi-expr, decision-table (single + collect), typed getters, provenance
   `Explain`, timing, Scope JSON round-trip, `BareEngine`, and config-driven (JSON/YAML)
   pipeline construction.
5. **README + docs.** A concise Examples section pointing at `examples/` (one line per
   case), plus godoc on every new exported symbol. ADRs and plan are traceable.

## Non-goals (deferred)

- **Opt-in timing.** Timing is always-on (ADR-0013); it is too cheap to gate. A future
  toggle would be additive.
- **A separate web-response envelope.** Raw result data for a web response is already
  `json.Marshal(sc.Snapshot())` (just the `data` map). `MarshalJSON` intentionally emits the
  richer **envelope** for persistence/round-trip; the two are distinct on purpose.
- **Round-tripping compiled expressions.** The JSON codec serializes a Scope's *values*
  (data/timing/provenance records), not the `Pipeline`/stage *definitions* — those already
  serialize via the `config` definition types (Spec 004).
- **Restoring provenance to a fully re-runnable state.** `UnmarshalJSON` restores the
  recorded derivations and marks the Scope provenance-enabled for *inspection*
  (`Derivation`/`Lineage`/`Explain`); it does not re-attach compiled expressions.
- **Wall-clock determinism in production.** `WithClock` exists for test determinism; the
  default is `time.Now`.

## Design

### `stage.Scope` — evaluation timing (`stage/scope.go`, `stage/timing.go`)

New unexported fields on `Scope`: `startedAt time.Time`, `duration time.Duration`,
`clock func() time.Time`. `NewScope` defaults `clock` to `time.Now`.

```go
func WithClock(clock func() time.Time) ScopeOption  // inject a deterministic clock (tests)

func (s *Scope) StartedAt() (time.Time, bool)     // false until a run stamps it
func (s *Scope) Duration()  (time.Duration, bool) // false until a run stamps it
```

`Pipeline.Run` stamps timing around stage execution:

```go
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error {
    sc.markStarted()          // startedAt = clock()
    defer sc.markFinished()   // duration  = clock() - startedAt (set even on error)
    // … existing topo-ordered stage execution …
}
```

`markStarted`/`markFinished` take the write lock. Timing is stamped even when a stage
errors (a partial run still took time). Accessors take the read lock and return `false`
when unset (`startedAt.IsZero()`).

### `stage.Scope` — JSON codec (`stage/json.go`)

```go
func (s *Scope) MarshalJSON() ([]byte, error)
func (s *Scope) UnmarshalJSON(b []byte) error
```

The wire envelope (read-locked on marshal, fields conditional):

```json
{
  "data": { "base": 20, "taxed": 22, "tier": { "level": "gold" } },
  "timing": { "started_at": "2026-07-11T09:00:00.000000004Z", "duration_ns": 4200 },
  "derivations": {
    "taxed": {
      "path": "taxed", "stage": "taxed", "stage_type": "single-expr",
      "operation": "eval", "expression": "base * 1.1",
      "inputs": { "base": 20 }, "value": 22
    }
  }
}
```

- `data` — always present (the accumulated map; nested values marshal as-is).
- `timing` — present only after a run (`started_at` RFC3339Nano, `duration_ns` int64);
  omitted when unset.
- `derivations` — present only when provenance is enabled; a map keyed by path.

`Derivation` gains snake_case `json:` tags so its schema is stable and documented.

`UnmarshalJSON` on a `*Scope`:
1. decodes the envelope; sets `data` (empty map if `data` absent — never nil),
2. restores `timing` (`startedAt`, `duration`) when present,
3. if `derivations` present: sets `provenance = true` and repopulates the `derivations`
   map (so `Derivation`/`Lineage`/`Explain` work on the restored Scope),
4. initializes `clock` to `time.Now` (a restored Scope is inspected, not re-run, but the
   field must be non-nil).

It initializes the mutex-guarded maps so the restored Scope is immediately usable by the
getters and provenance accessors. Numbers decode as `float64` per `encoding/json` — this
is the documented behavior of round-tripped `data`, unchanged from any JSON boundary; the
strict typed getters (`GetInt` vs `GetFloat64`) reflect that faithfully.

**Web vs. persistence:** `json.Marshal(sc.Snapshot())` → just the data map (web response);
`json.Marshal(sc)` (or a `*Scope` field) → the envelope (persistence/round-trip). Documented
on both methods.

### `rlng.BareEngine` (`bare_engine.go`)

```go
type BareEngine struct { pipeline *stage.Pipeline; scopeOpts []stage.ScopeOption }

func NewBareEngine(pipeline *stage.Pipeline, opts ...Option) *BareEngine

// Evaluate seeds a Scope from input, runs the pipeline, and returns the accumulated
// map[string]any (the Scope snapshot). No result mapping.
func (e *BareEngine) Evaluate(ctx context.Context, input any) (map[string]any, error)

// EvaluateScope is Evaluate but returns the full *stage.Scope, exposing timing,
// JSON serialization, and provenance.
func (e *BareEngine) EvaluateScope(ctx context.Context, input any) (*stage.Scope, error)
```

`EvaluateScope` reuses the existing `flatten(input)` (map passthrough, struct via
`mapstructure`) and `WithScopeOptions`; `Evaluate` wraps it and returns `sc.Snapshot()`.
`input` is `any` (not the generic `I`) — a `BareEngine` is not parameterized on input or
result type. Safe for concurrent use after construction (each call builds a fresh Scope).

### `examples/` package (`examples/*_test.go`, `examples/doc.go`)

A dedicated package of runnable `Example…` tests, each with a `// Output:` block, using a
`WithClock` where a duration would otherwise be non-deterministic:

- **`pricing_test.go`** — order → `base = price*qty` (single-expr) → `taxed`/`discounted`
  (multi-expr); typed getters read results; Scope JSON round-trip (persist → reload →
  re-read); timing shown via injected clock.
- **`eligibility_test.go`** — loan/credit decision-table: single mode (first-match tier)
  and collect mode (accumulated flags); provenance `Explain` renders the derivation tree.
- **`config_test.go`** — a JSON (and YAML) pipeline definition loaded via `config`, built
  into a `Pipeline`, and run through a `BareEngine` returning `map[string]any`.

`doc.go` gives the package a one-paragraph overview.

### README + ADRs

- README: an **Examples** section — one line per example case and "runnable via
  `go test ./examples/`". Kept brief; the code is the documentation.
- **ADR-0013** — Scope JSON envelope shape + always-on timing (with `WithClock` for tests).
- **ADR-0014** — `BareEngine` (mapper-less, `any` in / `map[string]any` out, plus
  `EvaluateScope`).

## Testing (per CLAUDE.md gates)

- **Timing:** run stamps `StartedAt`/`Duration`; accessors return `false` before a run and
  `true` after; timing stamped even when a stage errors; `WithClock` yields a deterministic
  duration.
- **JSON codec (hot paths + typed-error branches):** marshal with/without timing,
  with/without provenance; **byte-stable round-trip** (`marshal → unmarshal → marshal`
  yields equal JSON) — asserted at the JSON layer, not on Go-typed values, because
  `encoding/json` reloads every number as `float64` (a documented consequence, reflected by
  the strict getters); `timing` restores exactly (`started_at`/`duration_ns` are lossless);
  `UnmarshalJSON` on malformed JSON returns an error; absent `data` yields an empty (not
  nil) map; a restored provenance Scope answers `Derivation`/`Explain`.
- **`BareEngine`:** map input and struct input; `Evaluate` returns the snapshot;
  `EvaluateScope` exposes timing/JSON; a pipeline/stage error surfaces unwrapped; a
  non-flattenable input is a wrapped error.
- **Examples:** every `Example…` has a stable `// Output:` and passes under `go test`.
- Target ≥ 85% on changed packages; **every hot-path and typed-error branch covered**;
  `-race` green; benchmarks unaffected on the existing off paths.

## Open questions

None outstanding. Timing is always-on (confirmed); JSON is round-trippable for a DB codec
(confirmed); examples cover all features (confirmed).
