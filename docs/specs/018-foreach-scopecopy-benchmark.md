# Spec 018 — `foreach` per-element scope-copy benchmark

- **Status:** Draft
- **Backlog item:** B2 (`docs/BACKLOG.md`) — graduates the "per-element `Snapshot()`+`NewScope` cost"
  watch-item recorded in ADR-0040's whole-branch gate outcome.
- **Realized by:** Plan 018; ADR-0043.

## Problem

`ForEach.Execute` (`pipe/foreach.go:155`) runs an inner sub-pipeline once per collection element against a
**fresh per-element `Scope`**. Per element it:

1. `seed := sc.Snapshot()` — a shallow top-level copy of the outer scope's map (`pipe/scope.go:201`),
   O(number of top-level keys).
2. `seed[f.as] = el` — binds the element.
3. `esc := NewScope(seed, …)` — which **deep-copies the map spine** of `seed` via `cloneValue`
   (`pipe/scope.go:87`), O(total nested-`map[string]any` node count), then appends `esc.Snapshot()` to the
   results.

So the per-`Execute` cost is **O(elements × outer-scope size)**, where "size" is the top-level key count
(from `Snapshot`) plus the deep `map[string]any` spine (from `NewScope`'s `cloneValue`). ADR-0040 flagged
this as "acceptable for typical line-item counts, a candidate for a benchmark before large collections."
This increment is that benchmark: **measure first, optimize only if the data shows a real problem.**

## Goal

Add a Go benchmark that measures `ForEach.Execute`'s per-element scope-copy cost across the two axes of the
O(elements × outer-scope size) claim — collection size and outer-scope size/shape — so the scaling can be
read off (linear in each axis), and record the result as a data-backed decision (ADR-0043) that either
closes the watch-item as acceptable or spawns an optimization follow-up.

## Decisions

- **D1 — Isolate the scope-copy cost with an empty inner pipeline.** The benchmarked stage's inner
  `*Pipeline` is `NewPipeline()` with **zero stages** — a valid pipeline whose `Run` is a documented no-op
  (`pipe/pipeline.go:50`). This makes `Execute` measure exactly the foreach machinery under study —
  `Snapshot` + element bind + `NewScope`/`cloneValue` + per-element `esc.Snapshot()` + append + the final
  `sc.Set` — with no inner-stage evaluation confounding the numbers. Inner-stage cost (decision tables,
  expressions) is not what ADR-0040 flagged and is out of scope.
- **D2 — Two measured axes, as a sub-benchmark grid.**
  - **`elements`** (collection size): `1, 10, 100, 1000` — the "elements" factor. Holding scope fixed, cost
    should scale ~linearly here.
  - **`scope`** (outer-scope size/shape): the "outer-scope size" factor, three named shapes —
    - `flat8` — 8 flat top-level scalar keys (a typical header).
    - `flat64` — 64 flat top-level scalar keys (a wide header); against `flat8`, isolates the top-level
      `Snapshot`/clone scaling.
    - `nested` — a spine with nested `map[string]any` levels (a few top-level keys, one a nested map of
      depth ~3), so `cloneValue`'s **recursion** is exercised, not just the shallow top level.

  Grid = `scope × elements` = 3 × 4 = 12 sub-benchmarks, named `scope=<shape>/elements=<n>`.
- **D3 — `b.Loop()` (Go 1.24+), `-benchmem`.** Go is 1.25 (`go.mod`), so use `b.Loop()` (times only the loop
  body, keeps args/results alive) and report allocations via `b.ReportAllocs()`. All fixture construction
  (outer scope, collection, `ForEach` stage) happens **before** the loop so it is excluded from timing —
  only `Execute` is measured.
- **D4 — Realistic small map elements.** Each collection element is a small fixed `map[string]any` (a line
  item, e.g. `{"amount": <i>}`). Its per-element clone cost is a small constant independent of the `scope`
  axis, so it does not confound the scope-size scaling read.
- **D5 — Blackbox, alongside the existing bench file.** New file `pipe/foreach_bench_test.go`, external
  `package pipe_test`, driving only the exported API (`NewScope`, `NewPipeline`, `NewForEach`, `Execute`) —
  matching `pipe/scope_bench_test.go`'s form and this project's blackbox-only test rule.
- **D6 — Measure, then decide in ADR-0043.** Run `go test -bench=BenchmarkForEachScopeCopy -benchmem
  -count=10 ./pipe/…`, capture the output (with `goos/goarch/cpu`), and record in ADR-0043: the observed
  per-`Execute` cost at each cell, confirmation of the ~linear scaling in each axis, and the decision —
  **acceptable, no optimization now** (watch-item closed) if typical counts are cheap, or an **optimization
  follow-up** (its own increment + ADR) if a real regression shows. No production code changes in this
  increment unless the data demands one.

## Non-goals

- **No optimization in this increment.** This is a measurement increment; any optimization is a separate,
  ADR-backed follow-up triggered only by the data.
- **Provenance-on cost.** `NewScope(WithProvenance())` builds a per-element derivations map — a distinct
  cost dimension not named in the ADR-0040 watch-item. The grid runs provenance **off** (the common path);
  provenance-on is a documented, deferrable follow-up axis.
- **Roll-up cost.** The grid configures **no** `Rollup`s, to isolate the scope-copy machinery; roll-up
  aggregation cost is separate and out of scope here.
- **Inner-stage evaluation cost** (decision tables / expressions per element) — measured elsewhere if ever
  needed; the empty inner pipeline deliberately excludes it (D1).

## Success criteria

1. `pipe/foreach_bench_test.go` exists, `package pipe_test`, and `go test -bench=BenchmarkForEachScopeCopy
   -benchmem ./pipe/…` runs all 12 cells cleanly.
2. The benchmark drives only the exported API (no whitebox access).
3. `go test ./... -race` and the full library gate stay green (benchmarks compile and run under the normal
   `go test`; no behavior change to production code).
4. ADR-0043 records the captured numbers, the scaling confirmation, and the acceptable-vs-optimize decision,
   closing (or escalating) the ADR-0040 watch-item; `docs/BACKLOG.md` moves B2 to Resolved.

## Traceability

Backlog: B2. Plan: 018. ADR: 0043 (records the measurement + decision; closes ADR-0040's per-element
scope-copy watch-item). Related: ADR-0040 (`foreach` stage), ADR-0006 (deterministic sequential execution).
