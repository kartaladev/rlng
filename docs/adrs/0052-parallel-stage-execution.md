# ADR-0052 — Opt-in parallel execution of independent DAG stages

- **Status:** Draft (proposed) — supersedes ADR-0006
- **Date:** 2026-07-13
- **Prompted by:** Backlog B11 (`docs/BACKLOG.md`); Spec 027 (docs/specs/027-parallel-stage-execution.md)
- **Supersedes:** ADR-0006 (Sequential deterministic pipeline execution)

## Context

ADR-0006 made `Pipeline.Run` execute stages strictly sequentially in a fixed topological order, deferring
parallelism as YAGNI — but explicitly recorded that a parallel runner would "arrive as an additive change …
and this ADR is superseded by the one that introduces it," and that the Spec 002 concurrency invariant (each
stage writes only within its own name-namespace and reads only the read-only seed plus already-complete
dependencies) was already in place to make it safe. `Scope` carries an `RWMutex` guarding exactly this future.

A dependency DAG admits parallelism: independent stages (no path between them) can run concurrently. For a
wide DAG whose stages each do heavy `expr` evaluation, sequential execution leaves wall-clock on the table.
B11 graduates the deferral. The tension to resolve is that ADR-0006's rationale was **debuggability** — a
single reproducible order and an unambiguous error — which naive concurrency would trade away.

## Decision

**`Pipeline` gains opt-in concurrent execution of independent stages, implemented as a pure speedup that
preserves every observable of sequential execution.** Sequential remains the default.

1. **Full-determinism contract.** Enabling concurrency changes *when* stages run, never *what* a `Run`
   observes: final `Scope` data, the surfaced error, and the reported `stageOrder` are identical to a
   sequential `Run`. This is what keeps ADR-0006's debuggability criterion intact rather than discarding it.

2. **Opt-in via `PipelineOption`s; consolidated constructor.** `NewPipeline` is changed to
   `NewPipeline(stages []Stage, opts ...PipelineOption) (*Pipeline, error)` — the name is kept, but `stages`
   becomes a slice to free Go's single variadic slot for options. New options:
   `WithConcurrency()` (unbounded parallel), `WithMaxParallel(n)` (bounded to `n`; `n < 1` →
   `*InvalidMaxParallelError`), and `WithRuleset(id)` — the latter **replacing** the fluent
   `(*Pipeline).WithRuleset` method so all pipeline-level config lives in one variadic list. Both concurrency
   options absent ⇒ sequential (unchanged). Root-package `rlng.WithConcurrency()` / `rlng.WithMaxParallel(n)`
   engine `Option`s thread the choice through the `NewFrom*` convenience constructors.

3. **Wave-based (level-barrier) scheduling.** Stages are grouped into dependency levels (longest-chain depth,
   derived from the existing input-order-preserving topo sort); each level runs concurrently with a barrier
   between levels — unbounded, or through an `n`-goroutine worker pool when capped. Simplest and most
   debuggable; captures nearly all the parallelism of the shallow-wide DAGs typical of rule pipelines.
   Continuous/completion-driven scheduling is left as a future refinement.

4. **Topo-min error selection, no internal fail-fast.** A stage runs only when all deps succeeded (dep-failed
   subtrees pruned as in sequential). Independent in-flight stages run to completion even after a sibling
   fails; the returned error is the **topo-earliest** failure — provably the exact error sequential `Run`
   would return. Sibling cancellation on failure is deliberately avoided (it could mask a topo-earlier
   failure and break determinism). Caller `ctx` cancellation is honored and takes precedence over a stage
   error, matching sequential. Reported `stageOrder` is reconstructed from the topo order (∩ executed),
   never completion order.

5. **No `Scope` change.** Every `Scope` mutator already holds `s.mu`; with the Spec 002 disjoint-namespace
   invariant, parallel execution is race-free with no structural change. `go test -race` over the parallel
   paths is a gate.

## Consequences

- Independent stages of a wide DAG now overlap when a caller opts in; the default path is byte-identical to
  before, so no existing behavior or hash changes and no config-schema change (concurrency is a runtime
  strategy, not part of the ruleset definition — `Hash()` is untouched).
- **Breaking, pre-1.0 API change:** `NewPipeline`'s signature (variadic `Stage` → `[]Stage`) and the removal
  of the fluent `(*Pipeline).WithRuleset` method in favor of the `WithRuleset` option. All in-tree call sites
  (`config/build.go` and tests) migrate; the change is a deliberate consolidation, recorded here, and would
  warrant a minor bump under SemVer post-1.0.
- Debuggability is preserved: a failing parallel run still surfaces one deterministic error, and stepping is
  aided by the level structure; a developer can also simply drop the concurrency option to get the identical
  result sequentially for breakpoint work.
- Error-path wasted work: independent branches topo-later than a failure still run to completion (no
  fail-fast). Accepted as the price of determinism; a topo-index pruning optimization is left open.
- Wave scheduling leaves cross-level parallelism unused; if a real deep/uneven DAG workload shows it matters,
  a continuous scheduler is an internal change behind the same options (no API impact), recorded as a future
  refinement rather than built now.
- ADR-0006 is superseded, not edited: its Status becomes `Superseded by ADR-0052`, preserving the history of
  why sequential-only was correct for increments 3–26.
