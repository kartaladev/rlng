# Spec 003 — Stage DAG orchestration (`Pipeline`)

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-11
- **Increment:** 3 of 5 (see [Roadmap](#roadmap-position))
- **Builds on:** Spec 002 (`docs/specs/002-scope-and-stages.md`) — the `stage` package (`Stage` interface, `Scope`, `StageError`, the three stage types, and each stage's already-declared `DependsOn()`).
- **Realized by plans:** `docs/plans/003-dag-orchestration.md`
- **Related ADRs:** ADR-0005 (Pipeline placement, name, construction & validation model), ADR-0006 (sequential deterministic execution; concurrency deferred) — recorded with the implementation commits of this increment.

## Context

`rlng` is a pure-Go rule + calculation engine on [`expr-lang/expr`](https://github.com/expr-lang/expr), built for debuggability (no cgo, plain stack traces, typed errors that name the failing field and expression). Increment 2 delivered the three stage types and the `Scope` accumulator, with each stage *declaring* its dependencies via `DependsOn()` but **nothing consuming them** — a stage runs only in isolation via `Execute`. This increment builds the layer that consumes `DependsOn()`: a **`Pipeline`** that holds a set of stages, validates and dependency-orders them once at construction (topological sort with cycle detection), and runs them in that order against a shared `Scope`.

This is the DAG the architecture blueprint describes ("Stages declare `depends_on` and are topologically sorted (Kahn's algorithm) with cycle detection before execution"). It is the last piece before declarative config (Increment 4) and the `Engine[I, R]` facade (Increment 5), which will construct a `Pipeline` from loaded config and wrap its `Run` with typed input/output mapping.

### Roadmap position

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | **Done** (merged) |
| 2 | Scope + stages (single-expr, multi-expr, decision-table) | **Done** (merged) |
| **3** | **Stage DAG orchestration (`depends_on` topo-sort + cycle detection)** *(this spec)* | **This increment** |
| 4 | Declarative config (YAML/JSON loaders) | next |
| 5 | Result mapper + `Engine[I, R]` facade | later |

**Scope boundary:** this increment ships the **orchestrator only** — a `Pipeline` that validates, orders, and runs already-constructed `Stage` values against a caller-provided `Scope`. It does **not** construct stages from config (Increment 4) and does **not** create the `Scope` from a typed input struct or map a result out (Increment 5). Execution is **sequential and deterministic**; parallel execution of independent stages is explicitly deferred (ADR-0006).

## Goals

1. A **`Pipeline`** type (in the `stage` package) constructed from a set of `Stage` values via `NewPipeline(stages ...Stage) (*Pipeline, error)`.
2. **Construction-time validation** (all failures typed, each naming the offending element):
   - **duplicate stage names** → `*DuplicateStageError`;
   - a `DependsOn` target that names **no stage in the set** → `*UnknownDependencyError`;
   - a **dependency cycle** (including a self-dependency) → `*CycleError` carrying a concrete cycle path.
3. **Deterministic topological ordering** computed once at construction: among stages that are ready (all dependencies already ordered), emit in **constructor input order** — so the execution order is stable and intuitive, not hash-map dependent.
4. **`Run(ctx context.Context, sc *Scope) error`** — execute the stages in the computed order against `sc`, stopping at the first stage that errors and returning its (already stage-named) error. Honor `ctx` cancellation between stages.
5. Preserve the increment's debuggability discipline: readable, typed errors (a cycle error prints the actual loop, e.g. `a -> b -> a`, in ASCII for greppable, encoding-safe messages).

## Non-goals (deferred)

- **Concurrent execution** of independent stages — sequential deterministic ordering ships now; parallelism is a future additive change (ADR-0006, own ADR so it can be cleanly superseded).
- **Constructing stages from YAML/JSON config** — the `Pipeline` takes already-built `Stage` values this increment (Increment 4 builds them from config).
- **Typed input seeding / result mapping** — `Run` takes a caller-provided `*Scope`; flattening a typed input into it and projecting a typed result out is the `Engine[I, R]` facade's job (Increment 5).
- **`Pipeline` implementing `Stage`** (nested pipelines as sub-stages) — speculative composability, not needed now (YAGNI). `Pipeline` is a top-level orchestrator.
- **Per-pipeline options** (e.g. a future `WithConcurrency`) — none exist this increment; the variadic-stages constructor keeps the door open for an additive constructor later.

## Design

### Package layout

```
github.com/kartaladev/rlng/
  stage/                        # extended this increment
    pipeline.go                 # Pipeline: NewPipeline (validate + topo-sort), Run; typed construction errors
    pipeline_test.go            # table-driven tests
    pipeline_example_test.go    # runnable Example… tests (godoc)
```

`Pipeline` lives in `stage` because it operates purely on `Stage` and `Scope`; the root `rlng` package stays empty until the Increment-5 `Engine` facade (ADR-0005). No new third-party dependency — only `context`, `fmt`/`strings`, and the existing `stage` types.

### `Pipeline` (`pipeline.go`)

```go
// Pipeline runs a set of Stages in dependency order. It validates the set and
// computes the execution order once, at construction; Run only evaluates.
type Pipeline struct { /* ordered []Stage (unexported) */ }

// NewPipeline validates stages and computes their execution order. Stage names
// must be unique; every DependsOn target must name a stage in the set; the
// dependency graph must be acyclic. Returns *DuplicateStageError,
// *UnknownDependencyError, or *CycleError otherwise. An empty set is valid
// (Run is then a no-op).
func NewPipeline(stages ...Stage) (*Pipeline, error)

// Run executes the stages in dependency order against sc, stopping at and
// returning the first stage error. It checks ctx before each stage and returns
// ctx.Err() (unwrapped) if canceled.
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error
```

- **Construction is the expensive step** (validate + sort, once); `Run` is a straight ordered walk. This mirrors the "compile at construction, evaluate on the hot path" discipline the stage types already follow.
- **Empty pipeline** (`NewPipeline()`) is valid and `Run` is a no-op returning nil — composable when Increment 4 builds a pipeline from a possibly-empty config.
- **Determinism.** The stored order is fixed at construction, so repeated `Run`s (and the eventual concurrent variant's *default* ordering) are reproducible — a debuggability property, not an accident.

### Validation & topological sort

Performed inside `NewPipeline`, in this order:

1. **Duplicate names.** Build a `name → Stage` index over the input; the first repeated `Name()` yields `&DuplicateStageError{Name: n}`.
2. **Unknown dependencies.** For each stage, every `DependsOn()` entry must be a key in the index; the first miss yields `&UnknownDependencyError{Stage: s, Dependency: d}`. (A self-dependency passes this check — it names a real stage — and is caught as a cycle next.)
3. **Order (input-order-preserving Kahn).** Repeatedly emit the **first not-yet-emitted stage, in constructor input order, whose dependencies are all already emitted.** If a full pass emits nothing while stages remain, the remainder contains a cycle → go to step 4. `n` (stage count) is small for a rule engine, so the simple O(n²) form is chosen for clarity/debuggability over an ordered-queue Kahn; the resulting order is identical.
4. **Cycle path.** On an unresolvable remainder, run a depth-first search over the remainder (visiting each stage's `DependsOn` in declared order) to extract one **concrete cycle** and return `&CycleError{Cycle: [...]}` where `Cycle` lists the stages around the loop, closing on the repeated node (e.g. `["a", "b", "a"]`). This is the debuggability payoff: the error shows the actual loop, not just "a cycle exists".

### Execution (`Run`) — sequential, deterministic (ADR-0006)

`Run` walks the precomputed order and calls each stage's `Execute(ctx, sc)`:

- **Cancellation.** Before each stage, `Run` checks `ctx.Err()`; if non-nil it returns that error **unwrapped** (idiomatic `errors.Is(err, context.Canceled)` / `context.DeadlineExceeded`), guaranteeing cancellation is honored for any `Stage` implementation, not only the built-ins (which also self-check). No further stages run.
- **First error stops.** If a stage's `Execute` returns an error, `Run` returns it immediately without running later stages. The built-in stages already return a `*StageError` naming themselves, so the failing stage is identified; `Run` does not re-wrap (no double-wrapping).
- **Shared `Scope`.** All stages read and write the same `sc`; because they run one at a time in dependency order, each stage sees its dependencies' outputs already written. This is the sequential realization of Spec 002's concurrency invariant (stages write disjoint name-namespaces), which a future concurrent variant would exploit.

### Error model

```go
// DuplicateStageError reports two stages sharing a Name within a Pipeline.
type DuplicateStageError struct{ Name string }

// UnknownDependencyError reports a DependsOn target naming no stage in the set.
type UnknownDependencyError struct{ Stage, Dependency string }

// CycleError reports a dependency cycle; Cycle is the loop path, closing on the
// repeated stage (e.g. ["a","b","a"]).
type CycleError struct{ Cycle []string }
```

Each implements `error` with a readable message (`pipeline: duplicate stage "x"`; `pipeline: stage "a" depends on unknown stage "b"`; `pipeline: dependency cycle: a -> b -> a`). They are distinct types (not one struct with a kind enum) because each carries different identifying data and `errors.As` reaches the exact failure — the same typed-error discipline `expr` and `stage` already follow. These are construction errors only; `Run` surfaces stage `*StageError`s (unchanged) and bare `context` errors.

## Testing strategy

TDD, red → green → refactor from the first commit.

- **Table-driven** tests via the project `table-test` skill: the `assert` closure form; the **`ctx` modifier with `t.Context()`** (not `context.Background()`) for `Run`, including a **canceled-context** case; constructor tables that take no context stay context-free.
- **Runnable `Example…` tests** doubling as godoc: a small linear pipeline (dependency-ordered) and a cycle-error example. Diamond (`a → {b, c} → d`) dependency ordering is covered by the unit tests rather than a separate example.
- Coverage of:
  - **Ordering** — linear chain; diamond; independent stages preserve **input order**; a dependency declared before its dependent still runs after it (order is by dependency, not declaration).
  - **Validation** — duplicate name (`*DuplicateStageError`); unknown dependency (`*UnknownDependencyError`); direct cycle `a → a`, two-node cycle `a ↔ b`, longer cycle, each yielding a `*CycleError` with the concrete loop path; empty pipeline is valid and `Run` no-ops.
  - **Run** — dependent stage reads a dependency's Scope output; first-error stops and returns the failing stage's `*StageError` (use modulo-by-zero to force a genuine eval error — `expr-lang` does float division, so `1/0 → +Inf`); **canceled context** short-circuits with `ctx.Err()` and runs no stage.
  - **`errors.As`** reaches each typed construction error.
- **Library quality gates:** `go test ./... -race`, `go vet ./...`, `gofmt`/`gofumpt`, `golangci-lint run ./...`, and `govulncheck ./...` (if installed) all clean; `go mod tidy` a no-op.

## Dependencies

- **No new dependency.** `pipeline.go` uses only the standard library (`context`, `fmt`, `strings`) plus the existing `stage` types. `expr-lang/expr` remains the only consumer-visible dependency; `testify` stays test-only. Target Go 1.25+.

## Traceability

- **Spec:** 003 (this document). Builds on Spec 002.
- **Plan:** `docs/plans/003-dag-orchestration.md`.
- **ADRs:** ADR-0005 (Pipeline placement, name, construction & validation model), ADR-0006 (sequential deterministic execution; concurrency deferred).
- Implementation commits reference this spec via a `Spec: 003` trailer (and `ADR:` trailers where they record a decision).
