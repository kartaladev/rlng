# ADR-0043 — `foreach` per-element scope-copy cost: measured, accepted

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 018 / Plan 018, graduating backlog item **B2** — the per-element
  `Snapshot()`+`NewScope` cost flagged as a watch-item in ADR-0040's whole-branch gate outcome
  ("Each element deep-copies the outer scope's map spine (O(elements × outer-scope size)); acceptable for
  typical line-item counts, a candidate for a benchmark before large collections").

## Context

`ForEach.Execute` (`pipe/foreach.go`) runs an inner sub-pipeline once per collection element against a
**fresh per-element `Scope`**, to keep the outer scope untouched and prevent state leaking between elements
(ADR-0040 D2). Per element it (1) `Snapshot()`s the outer scope's top level, (2) binds the element, and
(3) builds a new `Scope` whose constructor **deep-copies the `map[string]any` spine** via `cloneValue`
(`pipe/scope.go`). That is O(elements × outer-scope size). ADR-0040 accepted this for typical line-item
counts but asked for a benchmark before relying on it for large collections. This ADR records that
measurement and the resulting decision.

## Measurement

Blackbox benchmark `BenchmarkForEachScopeCopy` (`pipe/foreach_bench_test.go`) drives `ForEach.Execute` over
a `scope × elements` grid, with an **empty inner pipeline** and **no roll-ups** so it isolates exactly the
scope-copy machinery (Snapshot + element bind + `NewScope`/`cloneValue` + per-element Snapshot + append +
the final Set), provenance off. `scope` shapes: `flat8`/`flat64` = 8/64 flat top-level scalar keys;
`nested` = a depth-~3 nested-map spine (exercises `cloneValue` recursion). Command:
`go test -bench=BenchmarkForEachScopeCopy -benchmem -count=10 ./pipe/`, summarized with `benchstat`.

```
goos: darwin / goarch: arm64 / cpu: Apple M4 Pro
                                    │  sec/op    │ allocs/op │
ForEachScopeCopy/scope=flat8/elements=1      1.914µ ± 2%     21.00 ± 0%
ForEachScopeCopy/scope=flat8/elements=10     21.43µ ± 6%     173.0 ± 1%
ForEachScopeCopy/scope=flat8/elements=100    171.2µ ± 5%    1.703k ± 0%
ForEachScopeCopy/scope=flat8/elements=1000   1.487m ± 3%    17.00k ± 0%
ForEachScopeCopy/scope=flat64/elements=1     6.811µ ± 3%     21.00 ± 0%
ForEachScopeCopy/scope=flat64/elements=10    67.38µ ± 1%     173.0 ± 0%
ForEachScopeCopy/scope=flat64/elements=100   548.4µ ± 3%    1.703k ± 0%
ForEachScopeCopy/scope=flat64/elements=1000  5.156m ± 1%    17.00k ± 0%
ForEachScopeCopy/scope=nested/elements=1     2.144µ ± 1%     27.00 ± 0%
ForEachScopeCopy/scope=nested/elements=10    19.89µ ± 2%     233.0 ± 0%
ForEachScopeCopy/scope=nested/elements=100   196.6µ ± 2%    2.303k ± 0%
ForEachScopeCopy/scope=nested/elements=1000  1.830m ± 2%    23.00k ± 0%
```

(B/op, omitted above for width, tracks allocs: flat8 2.86 KiB → 2.72 MiB, flat64 15.4 KiB → 14.99 MiB,
nested 3.87 KiB → 3.71 MiB across elements 1 → 1000.)

**Findings.**

- **Linear in `elements`** at every scope shape — each ×10 in element count is ≈ ×10 in time, allocs, and
  bytes (the slight sublinearity at low counts is the fixed per-`Execute` overhead). Amortized per-element
  cost: ≈ 1.5 µs (`flat8`), ≈ 5.1 µs (`flat64`), ≈ 1.8 µs (`nested`).
- **Linear in outer-scope size.** `allocs/op` is identical for `flat8` and `flat64` (same allocation
  *sites*) while `B/op` grows ~5.5× for the 8× wider header — the deep copy allocates the same number of
  maps but larger ones. `nested` adds exactly **+6 allocs/element** over the flat baseline — one heap map
  per nested `map[string]any` node `cloneValue` recurses into (customer/tier/limits/region/tax/order),
  directly confirming the recursion is the deep-copy cost.
- **Absolute magnitudes.** Typical line-item adjudication (≤ 100 elements, a modest header) is
  **sub-millisecond** (≤ 550 µs even at 64 header keys). The extreme corner measured (1000 elements × 64
  keys) is ~5 ms.

## Decision

**Accept the current per-element copy; no optimization now. The ADR-0040 watch-item is closed by this
measurement.** Rationale:

- The cost is **linear and predictable** (O(elements × outer-scope size), as ADR-0040 stated), not
  accidentally super-linear.
- Magnitudes are sub-millisecond for the counts `foreach` was designed for (line items, collaterals,
  coverages), and only low-single-digit-milliseconds at a deliberately extreme corner — acceptable for a
  decisioning call (a `foreach` `Execute` is not a per-request inner loop run millions of times per
  second).
- The copy **is the mechanism** of the per-element isolation invariant (ADR-0040 D2: outer scope untouched,
  no cross-element state leak). Removing or weakening it to save the copy would trade away that
  invariant and the debuggability it buys; that is not a trade worth making on speculation.

**If a future need arises** (a caller iterating very large collections — 10k+ elements — inside a
latency-sensitive path), the measured shape points at the optimization direction: avoid the eager full-spine
deep copy per element — e.g. a copy-on-write / lazily-seeded per-element scope that shares the outer spine
until first write, or reusing one per-element `Scope` cleared between elements. That would be its own
increment with its own ADR, justified by a real workload, and must preserve the isolation invariant. It is
**not scheduled**; this ADR records the finding and the direction, not a commitment.

## Consequences

- **No production-code change.** This increment adds only `pipe/foreach_bench_test.go` (a benchmark) and
  documentation; behavior, the exported API, and `Hash()` are all unchanged. Coverage is unaffected (no new
  production branches).
- **The watch-item is now data-backed.** ADR-0040's per-element scope-copy bullet is resolved by this ADR;
  a future reader sees measured numbers and a recorded rationale rather than an open "candidate for a
  benchmark."
- **A regression tripwire exists.** The benchmark stays in the tree; a future change that makes per-element
  copying materially more expensive is visible by re-running `BenchmarkForEachScopeCopy` (and comparing with
  `benchstat`).

## Traceability

Spec: 018 (docs/specs/018-foreach-scopecopy-benchmark.md)
Plan: 018 (docs/plans/018-foreach-scopecopy-benchmark.md)
Backlog: B2 (docs/BACKLOG.md → Resolved)
Related: ADR-0040 (`foreach` stage; this ADR closes its per-element scope-copy watch-item), ADR-0006
(deterministic sequential execution).
