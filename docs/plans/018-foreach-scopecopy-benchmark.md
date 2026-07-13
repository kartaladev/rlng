# Plan 018 — `foreach` per-element scope-copy benchmark (B2)

- **Implements:** Spec 018 (`docs/specs/018-foreach-scopecopy-benchmark.md`).
- **Records:** ADR-0043 (rides in the single task's commit); closes ADR-0040's per-element scope-copy
  watch-item.
- **Backlog:** graduates B2 (`docs/BACKLOG.md`).

One task. This is a **measurement** increment: it adds a benchmark and records a data-backed decision, with
no production-code change (Spec 018 D6 / Non-goals). "Green" here means the benchmark compiles, runs all 12
cells cleanly, and the full library gate stays green; there are no new production branches to cover. The
ADR + plan + backlog move ride in the **same commit** as the benchmark (CLAUDE.md: couple ADRs with code).
Standing program authorization: commit the green task, run the whole-branch gate, auto merge+push.

## Task 1 — Add the scope-copy benchmark, capture results, record ADR-0043

- [x] **Step 1 — write the benchmark.** New file `pipe/foreach_bench_test.go`, `package pipe_test`, driving
  only the exported API. `BenchmarkForEachScopeCopy` with a `scope × elements` sub-benchmark grid
  (Spec 018 D2): `scope ∈ {flat8, flat64, nested}`, `elements ∈ {1, 10, 100, 1000}`, named
  `scope=<shape>/elements=<n>`.
  - Fixture (built **before** `b.Loop()`, excluded from timing — D3): an outer `*Scope` via `NewScope` with
    the shape's seed map (`flat8`/`flat64` = N flat scalar keys; `nested` = a few keys, one a depth-~3
    nested `map[string]any`); a collection `[]any` of `elements` small fixed maps (`{"amount": i}`, D4); a
    `*ForEach` via `NewForEach(name, collectionPath, NewPipeline())` — **empty inner pipeline** (D1),
    **no rollups** (Non-goals). Seed the collection into the outer scope under `collectionPath`.
  - Loop: `b.ReportAllocs()`, then `for b.Loop() { if err := f.Execute(ctx, sc); err != nil { b.Fatal(err) } }`
    — the `b.Fatal` guard makes a mis-built fixture fail loud rather than silently benchmarking nothing.
  - Use `t.Context()`-equivalent for benchmarks: `ctx := b.Context()` (Go 1.24+) so the bench respects the
    test binary's lifecycle, consistent with the project's `t.Context()` rule.
- [x] **Step 2 — run & capture.** `go test -bench=BenchmarkForEachScopeCopy -benchmem -count=10 ./pipe/…`
  Record the full output including the `goos/goarch/cpu` header (golang-benchmark skill). Sanity-check the
  scaling: ~linear in `elements` (10× elements ≈ 10× ns/op at fixed scope) and increasing with scope
  size/nesting.
- [x] **Step 3 — author ADR-0043** (Nygard): context = ADR-0040's flagged O(elements × outer-scope size)
  per-element copy; decision = the measured numbers, the confirmed scaling, and **acceptable → no
  optimization now** (watch-item closed) *or* **escalate → optimization follow-up increment** if the data
  shows a real problem; consequences either way. Cite Spec 018 / Plan 018 / ADR-0040. Update ADR-0040's
  watch-item bullet to point at ADR-0043 as its resolution. Move **B2** to `docs/BACKLOG.md`'s Resolved
  section (closing increment 018 / ADR-0043).
- [x] **Step 4 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) / `go mod
  verify`. No coverage delta expected (benchmark-only, no production branches added).
- [x] **Step 5 — commit:** `test(pipe): add foreach per-element scope-copy benchmark (B2)`, ADR-0043 + this
  plan + the BACKLOG move riding in the same commit. Trailers `Backlog: B2 / Spec: 018 / Plan: 018 /
  ADR: 0043`.

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item (B3).
