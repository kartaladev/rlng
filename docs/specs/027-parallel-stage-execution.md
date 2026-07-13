# Spec 027 — parallel execution of independent DAG stages

- **Status:** Accepted
- **Backlog item:** B11 (`docs/BACKLOG.md`) — graduates the parallel-execution feature deferred as YAGNI in
  ADR-0006 ("Sequential deterministic pipeline execution (concurrency deferred)"). B11 is a **design-checkpoint
  item and a deliberate non-goal reversal**: it requires a **superseding ADR to ADR-0006** and a **live
  design-approval pause before any implementation** (per the standing backlog-execution program, see
  `docs/HANDOVER.md`).
- **Design approval:** design decisions D1–D6 below were put to the user and approved during brainstorming
  (2026-07-13). Implementation is gated on a **final live sign-off after this spec + the draft ADR-0052 are
  written** — no code before that sign-off.
- **Realized by:** Plan 027; ADR-0052 (supersedes ADR-0006).

## Problem

`Pipeline.Run` executes stages strictly sequentially, one at a time, in the deterministic topological order
computed at construction (ADR-0006). A dependency DAG, however, admits parallelism: stages with no path
between them are independent and could run concurrently. ADR-0006 deferred this as YAGNI ("stage counts are
small; no consumer has asked; coordination overhead would likely dominate") but **explicitly anticipated the
reversal** — parallel execution "arrives as an additive change … and this ADR is superseded by the one that
introduces it." The Spec 002 concurrency invariant (each stage writes only within its own name-namespace and
reads only the read-only seed plus already-complete dependencies) was put in place precisely to make that
safe, and `Scope` already carries an `RWMutex` guarding this future path.

B11 is that follow-through: an **opt-in** parallel runner for the independent stages of a wide DAG with heavy
per-stage `expr` evaluation, without giving up the debuggability that motivated ADR-0006.

## Goal

Let a caller opt a `Pipeline` into concurrent execution of independent stages, as a **pure speedup** — the
final `Scope` data, the surfaced error, and the reported stage order are **byte-for-byte identical to
sequential topo-order execution**. Sequential remains the default. Concurrency changes *when* stages run,
never *what* is observed.

## Decisions

- **D1 — Full determinism is the contract (preserves ADR-0006's debuggability rationale).** Enabling
  concurrency must not change any observable of a `Run`: (a) final `Scope` data (guaranteed by the Spec 002
  namespace-isolation invariant — independent stages write disjoint top-level subtrees, so accumulation is
  order-independent); (b) the surfaced error — the **topo-earliest** failing stage's error, exactly what a
  sequential `Run` returns (see D5); (c) the reported `stageOrder` — reconstructed to topo order, not
  completion order (see D6). Concurrency is invisible except in wall-clock time. This keeps ADR-0006's
  first-class criterion intact: a developer still reasons about a single, reproducible order and an
  unambiguous error.

- **D2 — Opt-in via variadic `PipelineOption`s; `NewPipeline` name kept, `WithRuleset` folded in.** The
  `pipe.NewPipeline` **signature is consolidated** (a deliberate, pre-1.0 breaking change) to carry
  pipeline-level options:
  ```go
  func NewPipeline(stages []Stage, opts ...PipelineOption) (*Pipeline, error)   // stages: variadic → slice
  type PipelineOption func(*pipelineConfig)
  func WithRuleset(id RulesetIdentity) PipelineOption   // replaces the fluent (*Pipeline).WithRuleset method
  func WithConcurrency() PipelineOption                 // parallel execution, unbounded
  func WithMaxParallel(n int) PipelineOption            // parallel execution, capped at n goroutines
  ```
  Go permits only one variadic parameter, so freeing the variadic slot for options requires `stages` to
  become a `[]Stage`; the constructor **name is preserved** (per the user's direction). The existing fluent
  `(*Pipeline).WithRuleset(id) *Pipeline` method is **removed** and re-expressed as the `WithRuleset`
  `PipelineOption`, consolidating all pipeline-level configuration in one variadic list.
  - Both concurrency options absent ⇒ **sequential** — today's behavior, bit-identical (default unchanged).
  - `WithConcurrency` and `WithMaxParallel` both present ⇒ **last-wins** (options applied left-to-right).
  - `WithMaxParallel(n)` with `n < 1` ⇒ `NewPipeline` returns a typed construction error
    (`*InvalidMaxParallelError{N}`), fail-loud rather than silently treating it as unbounded or sequential.

- **D3 — Bound: unbounded by default, optional cap.** `WithConcurrency()` runs **all** ready-and-independent
  stages in a level concurrently (fan-out). `WithMaxParallel(n)` runs a level through a **bounded worker pool
  of `n`** goroutines. Rule DAGs are small, so unbounded is rarely a risk; the cap is the escape hatch for a
  caller with a very wide level of side-effecting stages who wants to throttle goroutines/CPU.

- **D4 — Wave-based (level-barrier) scheduling.** Precompute dependency **levels** from the DAG (level =
  length of the longest dependency chain to a stage; derivable from the existing input-order-preserving topo
  sort — topo-index order coincides with level order, so waves are grouped topo order). `Run` executes level
  0's stages concurrently, waits (barrier), then level 1, and so on. This is the simplest, most debuggable
  structure and, for the shallow-wide DAGs typical of rule pipelines (1–3 levels), captures nearly all the
  available parallelism. It leaves *cross-level* parallelism unused (a fast level-2 stage waits for the
  slowest level-1 stage even if unrelated); **continuous / completion-driven scheduling** (a stage launching
  the instant its own deps complete) is recorded as a **future refinement**, not built now. Sequential
  execution (both options off) keeps the current straight ordered walk — no wave machinery.

- **D5 — Error path: topo-min selection, no internal fail-fast.** A stage launches only when **all** its
  dependencies **succeeded**, so a dep-failed subtree is naturally pruned — exactly as sequential never
  reaches a stage whose predecessor failed. Within a wave, all (independent) stages run to completion even if
  a sibling fails; failures are collected. If the wave had any failure, **subsequent waves do not launch**
  (they are topo-later; sequential would not reach them) and `Run` returns the **topo-earliest** collected
  failure. Because the sequential-first-failure always runs (its deps all succeeded) and every collected
  failure is topo-≥ it, topo-min selection returns exactly the sequential error. **No internal fail-fast
  cancellation:** cancelling an in-flight independent sibling on a sibling's failure could mask a topo-earlier
  failure and break D1's determinism, so it is deliberately avoided. Cost — independent branches topo-later
  than the failure still finish (wasted work on the error path only). Pruning topo-later in-flight work once
  the earliest-failure index is known is recorded as a **future optimization**.

- **D6 — Cancellation and reported order.** Caller `ctx` cancellation is honored: `Run` checks `ctx.Err()`
  before launching each wave (and each built-in stage self-checks), and `ctx.Err()` **takes precedence** over
  a stage error in the return value — matching sequential `Run`, which checks `ctx` before each stage. The
  reported `stageOrder` is **reconstructed from the topo order** (∩ the set of stages that actually executed)
  rather than appended in completion order, so `StageTimings`/`stageOrder` stay deterministic. Per-stage
  `stageTimes` durations are each individually correct (measured around that stage's own `Execute`).

## Concurrency safety (no Scope change)

`Scope` needs **no structural change**. Every mutator already holds `s.mu`: `Set` (data), `timeStage`
(`stageTimes`/`stageOrder`), `recordFirings` (`firing`), `Derive`/`recordElementDerivations` (`derivations`),
`stampRuleset`/`markStarted`/`markFinished`. Combined with the Spec 002 invariant — independent stages write
disjoint top-level subtrees, and `Snapshot`/`Get` share only read-only seed values and already-complete
dependency subtrees, none of which a concurrently-running independent sibling mutates — parallel execution is
race-free. The whole-`setPath` write is performed under the full lock, so even a (caller-induced, via
`WithOutput`) shared-parent write is serialized, never corrupting the map spine. `go test -race` over the new
parallel paths is a delivery gate.

## Non-goals

- **Continuous / completion-driven scheduling** — deferred to a future refinement (D4); wave-based first.
- **Fail-fast cancellation of independent siblings on stage error** — rejected as determinism-breaking (D5);
  pruning topo-later in-flight work once the earliest-failure index is known is a future optimization.
- **Parallelizing *within* a stage** (e.g. concurrent rules inside one decision-table, or concurrent
  `foreach` elements) — out of scope; B11 is about independent *stages*. (`foreach` per-element parallelism
  may be a separate future item.)
- **Config-declared concurrency** — concurrency is a runtime execution strategy, not part of the ruleset
  definition; it stays **programmatic** (a `PipelineOption` / engine `Option`), like `WithClock`. No
  YAML/JSON schema change, **no `Hash()` change** (concurrency does not alter the parsed `PipelineDef`, so
  existing rulesets hash byte-identically).
- **Changing the default** — sequential stays the default; parallel is strictly opt-in.

## Success criteria / hot-path branches to cover

All tests: `table-test` assert-closure form, blackbox `pipe_test` / `rlng_test` packages, `t.Context()`, and
`go test -race`.

1. **Sequential unchanged (default):** a pipeline built with no concurrency option runs bit-identically to
   today (existing tests, migrated to the new `NewPipeline([]Stage{…})` signature, stay green).
2. **Unbounded parallel correctness:** a wide DAG of independent stages under `WithConcurrency()` produces the
   same final `Scope` data as sequential.
3. **Bounded parallel correctness + cap honored:** same result under `WithMaxParallel(n)`; a test proves no
   more than `n` stages run concurrently (e.g. via a stage that records observed concurrency through an
   injected counter/gate).
4. **`WithMaxParallel(n<1)` → `*InvalidMaxParallelError`** from `NewPipeline` (`errors.As` reaches it).
5. **Determinism of data across many runs:** repeated parallel runs of a DAG with independent stages yield an
   identical accumulated map every time.
6. **Topo-min error selection:** two independent stages both fail; the surfaced error is the topo-earliest
   one, identical to what sequential `Run` returns — asserted across repeated runs (not wall-clock dependent).
7. **Dep-failed pruning:** a stage whose dependency fails never executes (its output is absent; it contributes
   no error), matching sequential.
8. **`ctx` cancellation precedence:** a cancelled context returns `ctx.Err()` (not a stage error), and no
   further waves launch.
9. **Deterministic `stageOrder`/`StageTimings`:** reported order equals topo order (∩ executed) regardless of
   completion order, across repeated parallel runs.
10. **`WithRuleset` as a `PipelineOption`:** a pipeline built with `NewPipeline(stages, WithRuleset(id))`
    stamps the ruleset onto every evaluated `Scope` (replacing the removed fluent method; the migrated
    `config.build` path and its tests stay green).
11. **Engine-level threading:** `rlng.NewFromYAML(ctx, yaml, rlng.WithConcurrency())` (and `WithMaxParallel`)
    builds an engine whose internal pipeline runs in parallel — proves the option reaches the built pipeline.
12. **Runnable `Example`** demonstrating `WithConcurrency()` on a small independent-stage pipeline with a
    deterministic `// Output:` block (doubles as godoc; determinism makes the output stable).

## Traceability

Backlog: B11. Plan: 027. ADR: 0052 (**supersedes ADR-0006**; records the opt-in parallel runner, the API
consolidation, wave-based scheduling, topo-min error selection, and the no-Scope-change safety argument).
Related / superseded: **ADR-0006** (sequential deterministic execution — superseded here), **ADR-0005**
(pipeline placement/name/construction/validation — its variadic-constructor "future option arrives additively"
note is realized, and its `NewPipeline` signature is consolidated), Spec 002 (the concurrency invariant this
relies on), Spec 003 (DAG orchestration), ADR-0009 (engine facade + the `rlng` engine `Option`s threaded).
