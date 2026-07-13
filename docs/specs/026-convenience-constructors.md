# Spec 026 — convenience constructors (build an engine from a config source)

- **Status:** Draft
- **Backlog item:** B10 (`docs/BACKLOG.md`) — graduates the `rlng.NewFromYAML` convenience deferred as YAGNI
  in ADR-0009. **Scoped to the convenience constructors only;** the second half of the B10 line
  (`Pipeline` implementing `Stage` / nested pipelines) is **re-deferred** here with rationale (see Non-goals).
- **Design approval:** B10 is a non-checkpoint item under the standing backlog-execution program — designed
  and implemented autonomously, surfaced via this committed spec (no live design-approval pause). The one
  scope fork (whether to also build Pipeline-as-Stage) was put to the user, who chose **constructors only**
  (2026-07-13).
- **Realized by:** Plan 026; ADR-0051.

## Problem

Building an engine from a declarative config today is three explicit steps:

```go
def, err := config.Parse(ctx, config.FromYAMLString(yaml)) // 1. parse
pipeline, err := def.Build()                               // 2. compile
eng, err := rlng.New(pipeline)                             // 3. wrap
```

The declarative and programmatic paths deliberately converge at `*pipe.Pipeline` (ADR-0009), and `rlng` does
not import `config` — so a consumer whose rules live in YAML/JSON must wire the three layers by hand every
time. ADR-0009 explicitly anticipated a `rlng.NewFromYAML` convenience "added additively if desired
(deliberately deferred, YAGNI)"; B10 is that follow-through.

## Goal

Add one-call constructors to the root `rlng` package that compose `config.Parse → PipelineDef.Build →
rlng.New` (and the typed `NewTypedEngine` equivalent), so the common "engine from a YAML/JSON source" path
is a single call, while the explicit three-step path remains for anything needing build-time options.

## Decisions

- **D1 — Four constructors in the root `rlng` package.** Add (new file `fromconfig.go`):
  ```go
  func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error)
  func NewFromYAML(ctx context.Context, yaml string, opts ...Option) (*Engine, error)
  func NewTypedFromProvider[I, R any](ctx context.Context, p config.Provider, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error)
  func NewTypedFromYAML[I, R any](ctx context.Context, yaml string, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error)
  ```
  Each `*Provider` form runs `config.Parse(ctx, p)` then `def.Build()` then `New`/`NewTypedEngine`,
  threading `opts` (engine `Option`s) through and returning the first error unwrapped. Each `*YAML` form is
  sugar that delegates to its `*Provider` sibling with `config.FromYAMLString(yaml)`. `NewFromProvider`
  accepts **any** `config.Provider` (`FromJSONString`, `FromYAMLFile`, `FromJSONURL`, …), so a bare
  `NewFromJSON`/`NewFromYAMLFile`/etc. family is unnecessary (YAGNI) — `NewFromYAML` is the single shorthand
  for the most common in-memory case.

- **D2 — `rlng` imports `config` (additive, in-module, ADR-0009-sanctioned).** These constructors introduce
  a `rlng → config` package import, which ADR-0009 explicitly permitted as the additive convenience.
  `config` is a package of the **same module** (`github.com/kartaladev/rlng`) and already depends on
  `pipe`/`expr` plus `yaml.v3`/`mimetype`; those are already in `go.mod`, so **no new external dependency**
  is added (`go mod tidy` stays a no-op) and Go dead-code elimination drops `config` from a consumer's
  binary if the constructors are never called. The import is acyclic: `config` does not import `rlng`
  (verified). Recorded in ADR-0051, extending ADR-0009's convergence decision.

- **D3 — `ctx`-first on every constructor.** All four take `ctx context.Context` as the first parameter,
  matching `config.Parse` and honoring the I/O of file/URL providers (`FromYAMLFile`, `FromJSONURL`) — a
  provider may block and must be cancellable. The in-memory `NewFromYAML` also takes `ctx` for a consistent,
  honest signature (it threads to `Parse`) rather than hiding a `context.Background()`.

- **D4 — Default `Build()`; the explicit path is the escape hatch for build options.** The constructors call
  `def.Build()` with **no** `BuildOption`s. A caller needing strict-schema, lint-as-error, or a version
  override (`config.WithSchema`/`WithLintErrors`/`WithVersion`) uses the explicit `Parse → Build → New`
  path. Mixing `config.BuildOption` and `rlng.Option` in one variadic list would be a type-muddled surface;
  the convenience targets the common default-build case, and the three-step path loses nothing. Documented
  on each constructor.

- **D5 — Errors pass through unwrapped; no new sentinels.** A `config.Parse` failure (`*ConfigError`) or
  `Build` failure returns as-is; the typed constructors return `ErrNilMapper` from `NewTypedEngine` when
  `mapper` is nil. On `Build` success the pipeline is non-nil, so `ErrNilPipeline` cannot arise from this
  path. No new error type is introduced — the constructors only thread existing typed errors, preserving
  `errors.Is`/`errors.As` for callers.

## Non-goals

- **`Pipeline` implementing `Stage` / nested pipelines (the other half of the B10 backlog line) —
  re-deferred with rationale.** ADR-0005 deliberately made `Pipeline` **not** a `Stage` ("revisit with a
  superseding ADR if composition is needed"). Reversing it has marginal value: `foreach` already owns a
  per-element inner `*Pipeline` (the real sub-pipeline use case), and a *flat* nested pipeline is
  behaviourally ≈ inlining its stages into the parent (same shared `Scope`, same DAG) — it would add naming
  (Pipeline is currently nameless), shared-scope bookkeeping (avoid double `markStarted`/`stampRuleset`),
  and name-collision semantics for no concrete demand. Kept as a documented backlog note (still open), not
  built here. ADR-0051 records this as a deliberate non-goal.
- **Config-declared result mapping** (loading a `Mapper` from YAML) — a separate backlog concern (ADR-0028
  scope); the typed constructors still take a programmatic `*Mapper[R]`.
- **A `NewFromJSON`/`NewFromYAMLFile`/`NewFromURL` family** — subsumed by `NewFromProvider` + the existing
  `config.From*` providers (D1); adding them would be redundant surface.
- No change to `New`, `NewTypedEngine`, `Evaluate`, `config.Parse`, or `Build`; no `Hash()`/eval/config-schema
  change. Purely additive public surface.

## Success criteria / hot-path branches to cover

1. **`NewFromYAML` happy path:** a valid YAML ruleset builds an `*Engine` whose `Evaluate` produces the
   expected accumulated map (end-to-end through the composed three layers).
2. **`NewFromProvider` with a non-YAML provider:** e.g. `config.FromJSONString(...)` builds and evaluates,
   proving the general constructor is provider-agnostic.
3. **Parse-error passthrough:** malformed YAML → the `*config.ConfigError` from `Parse` is returned
   unwrapped (`errors.As` reaches it), no engine created.
4. **Build-error passthrough:** valid parse but invalid build (e.g. an unknown stage `type`, or a nested
   `as`-collision) → the `*config.ConfigError` from `Build` is returned unwrapped.
5. **`NewTypedFromYAML`/`NewTypedFromProvider` happy path:** maps the final scope into the typed `R`.
6. **Typed nil-mapper:** `NewTypedFromProvider(..., nil, ...)` returns `ErrNilMapper` (passed through from
   `NewTypedEngine`) — note the invalid config is never parsed twice; a nil mapper still fails fast after a
   successful parse/build (acceptable — mirrors `NewTypedEngine`).
7. **`opts` threaded:** an engine built via `NewFromProvider(..., rlng.WithScopeOptions(pipe.WithProvenance()))`
   exposes provenance on its evaluated scope (proves `opts` reach `New`).
8. **Runnable `Example_newFromYAML`** demonstrating the one-call construction + `Evaluate`, with a
   deterministic `// Output:` block (doubles as godoc).

All tests use the `table-test` assert-closure form, blackbox `rlng_test` package, and `t.Context()`.

## Traceability

Backlog: B10 (constructors half). Plan: 026. ADR: 0051 (records the four convenience constructors + the
`rlng → config` import as the additive follow-through ADR-0009 anticipated; and the deliberate re-deferral of
Pipeline-as-Stage). Related: ADR-0009 (facade placement, `rlng`-does-not-depend-on-config + the anticipated
`NewFromYAML` convenience being realized here), ADR-0019 (`Engine[I,R]` → `TypedEngine[I,R]` naming),
ADR-0005 (Pipeline-not-a-Stage, whose reversal is the re-deferred non-goal), increment 016 (`config.Provider`
abstraction these constructors consume).
