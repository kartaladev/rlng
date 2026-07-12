# Spec 009 — Public API refinement (pre-`v0.0.1`)

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Pre-release API polish** (no tag cut yet, so breaking the exported surface is free now and expensive later).
- **Revises the naming settled by:** Spec 005 (`Engine`/`Mapper`), Spec 007 (`BareEngine`), Spec 002 (`stage` package). Realized by ADR-0019 (naming; supersedes the naming in ADR-0009/0014, revises ADR-0001), ADR-0020 (fail-fast constructors), and ADR-0021 (exported error sentinels, `Clock` interface, all-blackbox tests).

## Context

A pre-release review of the exported surface found three names that undersell or
mis-rank the API:

- `BareEngine` (map-in/map-out) is the simpler, more fundamental entry point, yet
  it carries an awkward qualifier while the generic typed facade holds the
  unqualified `Engine`.
- The generic `Engine[I, R]` is a convenience layer over the map engine; naming
  it `TypedEngine[I, R]` states what it adds.
- The `stage` package holds `Stage`, `Scope`, and `Pipeline`; its central
  interface reference `stage.Stage` stutters.

## Goals

1. **`BareEngine` → `Engine`**, constructor `NewBareEngine` → **`New`**. The
   map-in/map-out engine becomes the unqualified primary type; `rlng.New(pipeline)`
   is the canonical entry point.
2. **`Engine[I, R]` → `TypedEngine[I, R]`**, constructor `New[I, R]` →
   **`NewTypedEngine[I, R]`**.
3. **package `stage` → `pipe`.** Exported symbols keep their names
   (`pipe.Stage`, `pipe.Scope`, `pipe.Pipeline`, `pipe.SingleExpr`, …); only the
   qualifier changes, removing the `stage.Stage` stutter. `pipe` was chosen over
   `pipeline` (stutters on `pipeline.Pipeline`) and `graph` (over-emphasizes the
   DAG, reads oddly with `Scope`/expr types).
4. **File layout** reflects the new names: `engine.go` holds `Engine` + `New` +
   the shared `Option`/`flatten` helpers; `typed_engine.go` holds
   `TypedEngine` + `NewTypedEngine`. Test files and example function names follow.
5. **Fail-fast constructors.** `New` and `NewTypedEngine` validate their required
   arguments and return an error instead of a bare value: `New` returns
   `ErrNilPipeline` for a nil pipeline; `NewTypedEngine` returns `ErrNilPipeline`
   or `ErrNilMapper`. This turns a deferred nil-pointer deref on the first
   `Evaluate` into a clear construction-time error, consistent with the
   debuggability goal. See ADR-0020.
6. **Exported error sentinels.** The construction/evaluation sentinels that
   callers (and tests) assert on are exported so they are `errors.Is`-comparable
   across package boundaries: `rlng.ErrNilPipeline`/`ErrNilMapper`/`ErrNilInput`/
   `ErrEmptyMappingKey`, `expr.ErrEmptyExpression`, `pipe.ErrEmptyStageName`/
   `ErrEmptyPath`, `config.ErrNoStages` (joining the already-exported
   `pipe.ErrPathConflict`, `expr.ErrNotBool`, …). See ADR-0021.
7. **`Clock` interface.** `pipe.WithClock` takes a `pipe.Clock`
   (`interface{ Now() time.Time }`) instead of a `func() time.Time`, so a
   time-faking library such as `jonboulle/clockwork` can be injected directly
   (its clocks have `Now() time.Time`) without this module depending on it. The
   default is an internal real clock. See ADR-0021.
8. **All-blackbox tests.** Every test file is an external (`package X_test`) test
   exercising only the exported API; internal-only tests were rewritten through
   the public surface, and every table-driven test uses the `assert`-closure
   form. See ADR-0021.

## Non-goals

- No behavior change. This is a pure rename/move; every test carries over
  unchanged in intent.
- The module path (`github.com/kartaladev/rlng`, ADR-0001) is unchanged.
- No change to `expr`, `config`, `Mapper`, `MappingTemplate`, or error types.

## Verification

`go build ./...`, `go test ./... -race`, `go vet`, `gofmt`, `golangci-lint`, and
`go mod tidy` all clean; coverage unchanged from before the rename (behavior is
identical). Every importer (root, config, examples), the README, and package
docs reference the new names; no `BareEngine` / `rlng/stage` / `stage.` selector
remains in code.
