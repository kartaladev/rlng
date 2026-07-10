# Spec 002 — Scope + Stages

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-10
- **Increment:** 2 of 5 (see [Roadmap](#roadmap-position))
- **Builds on:** Spec 001 (`docs/specs/001-expression-core.md`) — the `expr` package (`Predicate`, `Function`, `expr.Option`, typed errors).
- **Realized by plans:** `docs/plans/002-scope-and-stages.md`
- **Related ADRs:** ADR-0002 (stage execution model + `Scope` naming), ADR-0003 (decision-table hit policies), ADR-0004 (shared stage `Option` convention + hit-policy naming + empty-name validation) — recorded with the implementation commits of this increment.

## Context

`rlng` is a pure-Go rule + calculation engine on [`expr-lang/expr`](https://github.com/expr-lang/expr), built for debuggability (no cgo, plain stack traces, typed errors that name the failing field and expression). Increment 1 delivered the atomic evaluators in `expr/`. This increment builds the next layer up: a **shared evaluation accumulator** and the **three stage types** that compose those evaluators into reusable units of rule/calculation logic.

### Roadmap position

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | **Done** (merged) |
| **2** | **Scope + stages (single-expr, multi-expr, decision-table)** *(this spec)* | **This increment** |
| 3 | Stage DAG orchestration (`depends_on` topo-sort + cycle detection) | next |
| 4 | Declarative config (YAML/JSON loaders) | later |
| 5 | Result mapper + `Engine[I, R]` facade | later |

**Scope boundary (decided during brainstorming):** this increment ships the **building blocks only** — the `Scope` accumulator and the three stage types, each independently constructible and executable via `Execute`, and each *declaring* its `DependsOn()`. There is **no multi-stage runner**: composing stages into a dependency-ordered DAG (consuming `DependsOn()`, topo-sorting, detecting cycles) is Increment 3. No wrkflw-specific `map`→`map` adapter is built in `rlng` at all — the `wrkflw` engine imports `rlng` and adapts on its own side.

## Goals

1. **`Scope`** — a concurrency-safe `map[string]any` accumulator threaded through evaluation, with **dot-path** `Set`/`Get` and a `Snapshot` view that serves as the `expr` evaluation environment.
2. A common **`Stage`** interface — `Name() / Type() / DependsOn() / Execute(ctx, *Scope)` — with three implementations:
   - **`SingleExpr`** — one value expression, optional **condition gate**, optional fallback (via `expr.WithFallback`).
   - **`MultiExpr`** — several named expressions evaluated in **priority order**, each visible to later ones within the stage.
   - **`DecisionTable`** — ordered rules (condition + decisions) with **`single`** (first-match-wins) and **`collect`** (accumulate) hit policies.
3. Compile once at construction (errors surface from `New*`); `Execute` only evaluates.
4. Typed, `Unwrap`-able **`StageError`** that wraps the underlying `expr.CompileError`/`EvalError`, preserving field+expression naming.
5. Threading `context.Context` through `Execute` so cancellation is available at this layer (per spec 001's deferral).

## Non-goals (deferred)

- **DAG orchestration** — `depends_on` topo-sort, cycle detection, and any multi-stage runner (Increment 3). Stages only *declare* dependencies here.
- **YAML/JSON config loading** — stages are constructed programmatically from Go strings this increment (Increment 4).
- **Result mapper + `Engine[I, R]` facade** (Increment 5).
- **wrkflw `map`→`map` adapter** — lives in the `wrkflw` repo, not here.
- **Struct→map seeding of `Scope`** — `Scope` is seeded from `map[string]any` this increment; flattening a typed input struct is the `Engine`'s job (Increment 5), reusing the `expr` env logic.

## Design

### Package layout

```
github.com/kartaladev/rlng/
  stage/                # this increment
    scope.go            # Scope accumulator (dot-path Set/Get/Snapshot, mutex-guarded)
    scope_test.go
    stage.go            # Stage interface + Type constants + StageError
    errors.go           # StageError, sentinels (ErrPathConflict, ErrPathNotMap)
    errors_test.go
    single.go           # SingleExpr stage
    single_test.go
    single_example_test.go
    multi.go            # MultiExpr stage
    multi_test.go
    multi_example_test.go
    table.go            # DecisionTable stage (single/collect)
    table_test.go
    table_example_test.go
    doc.go
```

The root `rlng` package stays empty until the Increment-5 `Engine` facade. `stage` imports `github.com/kartaladev/rlng/expr` and the standard library; no new third-party dependency is introduced.

### Naming (ADR-0002)

The brief/handover call the accumulator "Context". This increment names it **`Scope`** instead: `Execute(ctx context.Context, sc *Scope)` would otherwise carry two different "Context" types in one signature — a permanent readability cost, and Go convention reserves `Context` for `context.Context`. `Scope` accurately names "the set of variables in scope during evaluation, growing as each stage contributes." ADR-0002 records this rename (realizing the brief's "Context accumulator") alongside the stage-execution model; CLAUDE.md's blueprint wording is refreshed in the same change to avoid drift.

### `Scope` (`scope.go`)

```go
type Scope struct { /* sync.RWMutex + data map[string]any + strict bool (unexported) */ }

func NewScope(seed map[string]any, opts ...ScopeOption) *Scope
func WithStrict() ScopeOption

func (s *Scope) Set(path string, v any) error   // dot-path; creates intermediate maps
func (s *Scope) Get(path string) (any, bool)     // dot-path lookup
func (s *Scope) Snapshot() map[string]any        // shallow top-level copy under RLock — the expr eval env
```

- **Dot paths.** `Set("discount.rate", 0.1)` builds `{"discount": {"rate": 0.1}}`; `Get("discount.rate")` traverses it. A single segment (no dot) sets/gets a top-level key. An empty path is an error.
- **Seed.** `NewScope` defensively **shallow-copies** the seed's top-level map so callers can't mutate it underneath; nested structures are referenced, not cloned (documented).
- **Overwrite policy.** Default **lenient** (last-write-wins). `WithStrict()` makes `Set` return `ErrPathConflict` when the leaf path already holds a value — a debuggability guard against accidental cross-stage output collisions.
- **Descent errors.** If an intermediate segment exists but is not a `map[string]any`, `Set`/`Get` fail with `ErrPathNotMap` (you cannot descend through a scalar).
- **Concurrency.** All methods take the `RWMutex`. Stages read via `Snapshot()` (never the live map) so evaluation cannot race a concurrent writer. `Snapshot` is a shallow top-level copy; the concurrency contract that makes this race-free under Increment 3's parallel DAG is stated below.

#### Concurrency model (the invariant Increment 3 upholds)

Each stage writes **only within its own name-namespace** (top-level key = the stage name) and reads only its declared inputs (the read-only seed) plus the outputs of stages it `DependsOn` (already complete before it runs). Therefore two stages that Increment 3 runs concurrently are, by definition, independent — they write **disjoint top-level keys**, hence disjoint nested maps — and neither reads a namespace the other is still writing. A shallow `Snapshot` is thus safe: the nested maps a reader shares are either the immutable seed or a finished dependency's output. Within this increment stages execute one at a time, so the property is trivially satisfied and the `-race` suite is clean; Increment 3's DAG is what preserves it under parallelism.

### `Stage` interface (`stage.go`)

```go
type Stage interface {
	Name() string
	Type() string                                    // one of the Type* constants
	DependsOn() []string                             // declared now; consumed by the DAG in Increment 3
	Execute(ctx context.Context, sc *Scope) error    // reads Snapshot, writes results via Set
}

const (
	TypeSingleExpr    = "single-expr"
	TypeMultiExpr     = "multi-expr"
	TypeDecisionTable = "decision-table"
)
```

`Execute` honors `ctx` cancellation at the natural boundary (checked before evaluating; the underlying `expr` VM calls are fast and synchronous). All three stages are safe for concurrent `Execute` on the *same* stage value against *different* `Scope`s (they hold no mutable state post-construction).

### Stage options (`options.go`)

A single shared `Option` type configures all three stage constructors —
mirroring `expr.Option`'s pattern from Spec 001: names are unprefixed, and an
option that does not apply to a given stage type is silently ignored
(documented per option). See ADR-0004.

```go
type Option func(*stageConfig)

func WithDependsOn(deps ...string) Option                        // all stage types
func WithOutput(path string) Option                              // SingleExpr only; default output path = the stage name
func WithCondition(condition string, opts ...expr.Option) Option // SingleExpr only
func WithExprOptions(opts ...expr.Option) Option                 // SingleExpr only; applied to the main Function (WithFallback, WithGlobals, …)
func WithHitPolicy(h HitPolicy) Option                            // DecisionTable only; default HitPolicySingle
```

### `SingleExpr` (`single.go`)

```go
func NewSingleExpr(name, expression string, opts ...Option) (*SingleExpr, error)
```

`NewSingleExpr` returns a `*StageError` wrapping `errEmptyStageName` if `name == ""`, before compiling anything.

`Execute`: take `sc.Snapshot()`; if a condition predicate is configured and tests **false**, the stage is a **no-op** (writes nothing) and returns nil; otherwise `Apply` the main `expr.Function` and `Set` the result at the output path (default: the stage name). A `nil` result (with an `expr.WithFallback` configured) is handled by `Function.Apply` per spec 001. Compilation of both the condition and the main function happens in `NewSingleExpr`.

### `MultiExpr` (`multi.go`)

```go
type NamedExpr struct {
	Name       string
	Expression string
	Priority   int            // lower value = evaluated earlier
	Options    []expr.Option  // per-expression (WithFallback, WithGlobals, …)
}

func NewMultiExpr(name string, exprs []NamedExpr, opts ...Option) (*MultiExpr, error)
```

Constructor validates a non-empty stage `name` (a `*StageError` wrapping `errEmptyStageName` otherwise), non-empty `Name`s and unique names within the stage, compiles each expression, and orders them by ascending `Priority` (stable for ties). `Execute` evaluates them in that order against a **working env** seeded from `sc.Snapshot()`: each result is written back into the working env under its `Name` **and** persisted to the scope at `name.<exprName>`, so a later expression can read an earlier one's output within the same stage.

### `DecisionTable` (`table.go`)

```go
type Rule struct {
	Condition       string
	Decisions       map[string]string  // output key -> value expression
	ConditionOptions []expr.Option
	DecisionOptions  []expr.Option
}

type HitPolicy int
const ( HitPolicySingle HitPolicy = iota; HitPolicyCollect )   // HitPolicySingle is the default

func NewDecisionTable(name string, rules []Rule, opts ...Option) (*DecisionTable, error)
```

Rules are evaluated **in declaration order** against a single `sc.Snapshot()`. Within a rule, the decisions are **independent** — each is evaluated against the same pre-rule env, so decision order does not matter (this is why `Decisions` may be an unordered map). Outputs are written under `name.<outputKey>`.

- **`HitPolicySingle`** (default, first-match-wins): the first rule whose condition tests true has each of its decisions applied and written; evaluation stops. No matching rule ⇒ no writes.
- **`HitPolicyCollect`**: every rule whose condition tests true contributes; each output key accumulates a **`[]any`** with one entry per matched rule, **in rule order** (DMN COLLECT semantics). Keys are written once after all rules are evaluated. No matches ⇒ no writes.

The constructor validates a non-empty stage `name` (a `*StageError` wrapping `errEmptyStageName` otherwise), a non-empty rule set, non-empty output keys, and compiles every condition (as an `expr.Predicate`) and every decision (as an `expr.Function`) up front.

### Error model (`errors.go`)

```go
type StageError struct {
	Stage string   // the stage's Name
	Type  string   // the stage's Type
	Cause error    // wraps an *expr.CompileError / *expr.EvalError / scope error
}
func (e *StageError) Error() string  // `stage "name" (type): <cause>`
func (e *StageError) Unwrap() error

// scope sentinels
var ErrPathConflict = errors.New("scope: path already set")     // strict-mode overwrite
var ErrPathNotMap   = errors.New("scope: intermediate path is not a map")

// stage sentinel (unexported; surfaced via StageError.Cause)
var errEmptyStageName = errors.New("stage name must not be empty")
```

Construction errors from `New*` wrap `expr.CompileError` in a `StageError` (naming the stage). Evaluation errors from `Execute` wrap `expr.EvalError` (or a scope error) likewise. An empty `name` given to any `New*` constructor is rejected up front — before any compilation — as a `StageError` wrapping `errEmptyStageName` (ADR-0004), so callers never get a silent `""`-namespace write. Because the wrapped `expr` errors already name the offending expression and field, `errors.As` reaches the exact failure — preserving the debuggability chain end to end.

## Testing strategy

TDD, red → green → refactor from the first commit.

- **Table-driven** tests via the project `table-test` skill: the `assert` closure form, testify `require`/`assert`, and — because `Execute` takes a `context.Context` — the **`ctx` modifier with `t.Context()`** (not `context.Background()`). `NewScope`/`New*` constructor tables that take no context stay context-free.
- **Runnable `Example…` tests** doubling as godoc for `Scope` dot-paths and each stage (including a decision-table `single` and a `collect` example).
- Coverage of: dot-path `Set`/`Get` (nested create, single-segment, empty-path error, `ErrPathNotMap`, lenient vs `WithStrict` overwrite); `SingleExpr` condition-skip / fallback / custom output; `MultiExpr` priority ordering and intra-stage visibility; `DecisionTable` single first-match vs collect accumulation and no-match; empty stage `name` rejection (`errEmptyStageName`) across all three `New*` constructors; `StageError` wrapping/`Unwrap` to the underlying `expr` error; `ctx` cancellation short-circuit.
- **Library quality gates:** `go test ./... -race`, `go vet ./...`, `gofmt`/`gofumpt`, `golangci-lint run ./...`, and `govulncheck ./...` (if installed) all clean; `go mod tidy` a no-op.

## Dependencies

- **No new runtime dependency.** `stage` imports `github.com/kartaladev/rlng/expr` (already present) and the standard library (`context`, `sync`, `sort`, `strings`, `errors`, `fmt`). `expr-lang/expr` remains the only consumer-visible dependency.
- **Test-only:** `github.com/stretchr/testify` (already present, test-scoped).
- Target Go 1.25+.

## Traceability

- **Spec:** 002 (this document). Builds on Spec 001.
- **Plan:** `docs/plans/002-scope-and-stages.md`.
- **ADRs:** ADR-0002 (stage execution model + `Scope` naming), ADR-0003 (decision-table hit policies), ADR-0004 (shared stage `Option` convention + hit-policy naming + empty-name validation).
- Implementation commits reference this spec via a `Spec: 002` trailer (and `ADR:` trailers where they record a decision).
