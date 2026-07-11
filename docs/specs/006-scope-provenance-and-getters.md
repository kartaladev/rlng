# Spec 006 — Scope value provenance/lineage + typed getters

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-11
- **Post-roadmap feature** (the 5-increment roadmap is complete; this is additive, release-relevant hardening toward `v0.0.1`).
- **Builds on:** Spec 001 (`expr` — `Function`/`Predicate`, compiled programs), Spec 002 (`stage.Scope`, the three stages), Spec 003 (`Pipeline`), Spec 005 (`Engine`).
- **Related ADRs:** ADR-0011 (opt-in provenance/lineage model), ADR-0012 (strict typed getters + generic `GetAs`).

## Context

`rlng`'s first-class criterion is **debuggability**. Today a `Scope` holds the accumulated values but not *how* each was produced. This spec makes the derivation of every value inspectable — for a given path, which **stage** produced it, by what **operation**, from which **expression**, reading which **input values** — and lets a caller trace that chain recursively back to the seed inputs. It also adds **strict typed getters** so callers extract `int`/`string`/… from the `map[string]any` scope without hand-rolled type assertions.

**Performance is a hard requirement of this change.** Provenance is **opt-in**; with it off, evaluation must be indistinguishable from today (no added allocations on the hot path). This is proven with benchmarks (`-benchmem`) comparing provenance-off against the current baseline, and measuring the provenance-on and getter paths.

## Goals

1. **Provenance (opt-in via `stage.WithProvenance()`):** each value in the `Scope` carries a `Derivation` — `Path`, `Stage`, `StageType`, `Operation`, `Expression`, `Inputs` (referenced identifier → value at eval time), and `Value`. Seed inputs are recorded as `Operation: "seed"`.
2. **Recursive lineage:** `Scope.Lineage(path)` returns the derivation chain (the value's derivation plus, transitively, its inputs' derivations back to seeds); `Scope.Explain(path)` renders that chain as a human-readable indented tree.
3. **Referenced-identifier extraction (`expr`):** `Function.References()` / `Predicate.References()` return the top-level identifiers an expression reads, computed **once at compile** (parse + AST walk), so stages can snapshot exactly the inputs that fed each value with **zero eval-time cost** and no per-eval parsing.
4. **Strict typed getters:** `Scope.GetInt/GetInt64/GetFloat64/GetString/GetBool/GetSlice/GetMap` (+ generic `GetAs[T]`), each returning `ErrPathNotFound` for a missing path and a typed `*ScopeTypeError` for a type mismatch. **Strict** — no numeric coercion (a `float64` does not satisfy `GetInt`).
5. **Benchmarks + optimization:** the provenance-off path adds **zero** allocations vs. baseline; benchmarks cover `Set` vs `Derive`, `Engine.Evaluate` off vs on, the getters, and `Explain`.

## Non-goals (deferred)

- **Always-on provenance** — opt-in only (ADR-0011); the cost is paid only when requested.
- **Full member-path references** (`a.b.c` as one input) — references are **top-level identifiers** (`a`); `Lineage` reconciles namespaced stage outputs by dot-path prefix (below). Precise member-path lineage is a possible future refinement.
- **Numeric-coercing getters** — strict only (ADR-0012); a coercing variant can be added later without breaking the strict API.
- **Serializing the lineage** (JSON export of the derivation graph) — `Derivations()` exposes the raw records; formatting/serialization is the caller's.

## Design

### `expr` — referenced identifiers (`expr/function.go`, `expr/predicate.go`, `expr/refs.go`)

```go
func (f *Function) References() []string   // sorted, unique top-level identifiers read
func (p *Predicate) References() []string
```

Computed in the constructor (after a successful compile) by `parser.Parse(src)` + `ast.Walk` collecting `*ast.IdentifierNode` values, deduped and sorted, then **stored on the struct**. `References()` returns the cached slice — no work at `Apply`/`Test` time. Extraction failures (parse of an already-compiled source) yield an empty slice, never an error. A shared helper `references(src string) []string` lives in `expr/refs.go`.

### `stage.Scope` — provenance (`stage/scope.go`, `stage/provenance.go`)

```go
// Derivation records how one value in the Scope was produced.
type Derivation struct {
	Path       string         // scope dot-path written
	Stage      string         // producing stage name ("" for a seed input)
	StageType  string         // TypeSingleExpr / TypeMultiExpr / TypeDecisionTable, or "seed"
	Operation  string         // "seed", "eval", "expr:<name>", "decision:<key>", "collect:<key>"
	Expression string         // source expression ("" for a seed)
	Inputs     map[string]any // referenced identifier -> value at eval time (nil for a seed)
	Value      any            // the derived value
}

func WithProvenance() ScopeOption          // opt in; off by default
func (s *Scope) TracksProvenance() bool     // lock-free (set at construction, immutable)

// Derive stores v at path and, when provenance is enabled, records d (filling
// Path and Value). When disabled it is exactly Set(path, v).
func (s *Scope) Derive(path string, v any, d Derivation) error

func (s *Scope) Derivation(path string) (Derivation, bool)
func (s *Scope) Derivations() map[string]Derivation      // copy
func (s *Scope) Lineage(path string) []Derivation         // value + transitive inputs, root(seed)-first
func (s *Scope) Explain(path string) string               // indented derivation tree
```

- **Storage.** When `WithProvenance()` is set, `Scope` allocates a `derivations map[string]Derivation` and records each seed key in `NewScope`. When unset, `derivations` is nil and no provenance work happens anywhere.
- **`TracksProvenance()` is lock-free** — `provenance bool` is set once at construction and never mutated, so stages can branch on it without taking the mutex.
- **Lineage reconciliation.** For an input identifier `id`, `Lineage`/`Explain` look up `Derivation(id)`; if absent, they include every derivation whose path equals `id` or begins with `id + "."` (a stage that read the `tiers` namespace links to the `tiers.tag` derivations). A `visited` set guards against re-walking shared inputs (the pipeline is a DAG).
- **`Explain` format** (ASCII, deterministic; exact strings covered by tests), e.g.:
  ```
  quote.total = 22 [taxed single-expr] expr: base * 1.1
    base = 20 [base single-expr] expr: price * qty
      price = 10 [seed]
      qty = 2 [seed]
  ```

### Stage recording (`stage/single.go`, `multi.go`, `table.go`)

Each `Execute` records a derivation **only when `sc.TracksProvenance()`** — the guard keeps the provenance-off path allocation-free and identical to today (plain `sc.Set`). When on, the stage snapshots the referenced identifiers from the env it evaluated against and calls `sc.Derive`:

```go
if sc.TracksProvenance() {
	inputs := snapshotRefs(env, fn.References())        // map of referenced id -> value
	return sc.Derive(path, v, Derivation{Stage: name, StageType: <type>,
		Operation: <op>, Expression: <src>, Inputs: inputs})
}
return sc.Set(path, v)
```

Operations: `SingleExpr` → `eval`; `MultiExpr` → `expr:<exprName>`; `DecisionTable` single → `decision:<key>`; collect → `collect:<key>` (the `Value` is the accumulated `[]any`, `Inputs` the union of matched rules' references). `snapshotRefs` and the `MultiExpr`/`DecisionTable` expression source strings are added where needed (stages currently discard the raw source; they will retain it for the `Expression` field).

### `stage.Scope` — strict typed getters (`stage/get.go`)

```go
var ErrPathNotFound = errors.New("scope: path not found")

type ScopeTypeError struct{ Path, Expected, Actual string }
func (e *ScopeTypeError) Error() string   // scope: path "x": expected int, got string

func GetAs[T any](s *Scope, path string) (T, error)   // generic, strict
func (s *Scope) GetInt(path string) (int, error)
func (s *Scope) GetInt64(path string) (int64, error)
func (s *Scope) GetFloat64(path string) (float64, error)
func (s *Scope) GetString(path string) (string, error)
func (s *Scope) GetBool(path string) (bool, error)
func (s *Scope) GetSlice(path string) ([]any, error)          // decision-table collect yields []any
func (s *Scope) GetMap(path string) (map[string]any, error)   // stage namespaces are maps
```

All strict, via one generic helper: `Get(path)` → missing ⇒ `ErrPathNotFound`; present but not a `T` ⇒ `*ScopeTypeError{Path, Expected: "%T"(zero), Actual: "%T"(value)}`. The named methods delegate to `GetAs[T]`; `GetAs` is exported for types without a named method (a `[]any`, a custom struct, …). No reflection — a single generic type assertion.

### Benchmarks (`stage/*_bench_test.go`, root `*_bench_test.go`)

- `BenchmarkScopeSet` vs `BenchmarkScopeDeriveOff` (Derive with provenance off — must match Set) vs `BenchmarkScopeDeriveOn`.
- `BenchmarkEngineEvaluate` (provenance off) vs `BenchmarkEngineEvaluateProvenance` (on) — the off case must show the **same allocs/op** as before this change.
- `BenchmarkGetInt` / `BenchmarkGetString`, `BenchmarkExplain`, `BenchmarkFunctionReferences` (compile-time, ensures caching).
- All use `b.ReportAllocs()` and `b.ResetTimer()` after setup; results are compared with `benchstat` and summarized in the plan/commit.

## Testing strategy

TDD, red → green → refactor.

- **Table-driven** tests (`table-test` skill, assert-closure). `Derive`/getters take no context (context-free tables); the stage `Execute` paths already have `ctx` tables — add provenance-on cases there.
- Coverage of (**every hot-path + typed-error branch**, per the coverage gate):
  - `expr.References` — identifiers, member access (`a.b` → `a`), builtins excluded, dedup+sort, empty for a literal-only expression.
  - `Scope` provenance — off: `Derivation` absent, `Derive` == `Set`, `TracksProvenance()==false`; on: seed derivations recorded, each stage's derivation recorded with correct `Operation`/`Expression`/`Inputs`; `Lineage`/`Explain` for a linear chain, a diamond, a namespaced (decision-table) read, and a value read directly from a seed; `Explain` of an unknown path.
  - Getters — hit for each type; missing → `ErrPathNotFound`; wrong type → `*ScopeTypeError` (assert `Expected`/`Actual`); a stored `nil`; `GetAs` for a non-scalar (`[]any`).
- **Runnable `Example`** for `Explain` (debuggability showcase) and a typed getter.
- **Benchmarks** as above; the provenance-off allocation count is asserted to be unchanged in the commit narrative (benchstat before/after).
- **Gates:** `go test ./... -race`, `go vet`, `gofmt`, `golangci-lint`, `go test -bench . -benchmem` run clean; coverage ≥ 85% on changed packages.

## Dependencies

- **No new module dependency.** `expr` gains internal imports of `github.com/expr-lang/expr/parser` and `.../ast` (sub-packages of the existing `expr-lang/expr` dependency — no new module). `stage` uses only the standard library plus `expr`. Target Go 1.25+.

## Traceability

- **Spec:** 006 (this document).
- **Plan:** `docs/plans/006-scope-provenance-and-getters.md`.
- **ADRs:** ADR-0011 (opt-in provenance/lineage), ADR-0012 (strict typed getters + generic `GetAs`).
- Commits reference this spec via a `Spec: 006` trailer (and `ADR:` trailers).
