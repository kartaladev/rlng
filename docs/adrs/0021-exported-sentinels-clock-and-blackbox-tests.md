# ADR-0021 — Exported error sentinels, Clock interface, and blackbox tests

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 009 (docs/specs/009-api-naming-refinement.md)

## Context

A pre-`v0.0.1` pass converted the whole test suite to blackbox (external
`package X_test`) tests, which exercise only the exported API. Two things
followed from that goal, plus one independent request:

- Several tests asserted on **unexported sentinel errors** via `errors.Is`; an
  external test cannot name them. The library already exported some error
  sentinels (`pipe.ErrPathConflict`, `pipe.ErrPathNotMap`,
  `pipe.ErrEmptyPathSegment`, `pipe.ErrPathNotFound`, `expr.ErrNotBool`), so the
  remaining ones were the inconsistency.
- A couple of tests reached genuinely-internal helpers (`toEnv`, the variable
  patcher, `markStarted`/`markFinished`); these had to be rewritten through the
  public surface.
- `pipe.WithClock` took a `func() time.Time`. A `Clock` **interface** integrates
  more naturally with time-faking libraries (notably `jonboulle/clockwork`).

## Decision

- **Export the asserted sentinels.** `rlng.ErrNilPipeline`, `rlng.ErrNilMapper`,
  `rlng.ErrNilInput`, `rlng.ErrEmptyMappingKey`, `expr.ErrEmptyExpression`,
  `pipe.ErrEmptyStageName`, `pipe.ErrEmptyPath`, and `config.ErrNoStages` become
  exported, `errors.Is`-comparable values — a deliberate public error contract,
  consistent with the sentinels already exported.
- **`Clock` interface.** `pipe.Clock` is `interface{ Now() time.Time }`;
  `pipe.WithClock(Clock)` replaces the func form. The default is an unexported
  `realClock` delegating to `time.Now`. A `clockwork` clock (real or fake)
  satisfies `Clock` directly, so tests can inject deterministic time without the
  library depending on clockwork.
- **All-blackbox tests.** Every `_test.go` file is `package X_test`. Tests that
  formerly reached internals were rewritten to observe the same behavior through
  the exported API (e.g. struct-env conversion via `Function.Apply`; variable
  defaults via `NewFunction`+`WithGlobals`; timing via an empty `Pipeline.Run`;
  `ExprDef` option wiring via `Build`+run). No production symbol was exported
  solely to satisfy a test beyond the sentinels above. Every table-driven test
  uses the `assert`-closure form (per the project `table-test` skill).

## Consequences

- The public error and option surface grows by the exported sentinels and the
  `Clock` type — a pre-release, tag-free break, done deliberately now.
- Tests now guard the exported contract, catching accidental reliance on
  internals; coverage was preserved or improved (expr rose from 92.3% to 93.6%;
  root/config 100%, pipe 99.8%).
- Callers gain `errors.Is`-based error handling and clockwork-compatible time
  injection.
