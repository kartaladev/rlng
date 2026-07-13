# ADR-0006 — Sequential deterministic pipeline execution (concurrency deferred)

- **Status:** Superseded by ADR-0052
- **Date:** 2026-07-11
- **Prompted by:** Spec 003 (docs/specs/003-dag-orchestration.md)

## Context

A dependency DAG of stages admits parallelism: stages with no path between them
are independent and could run concurrently. Spec 002 deliberately documented a
concurrency invariant (each stage writes only within its own name-namespace and
reads only the read-only seed plus already-complete dependencies, so a shallow
`Scope.Snapshot` is race-free) *to enable* a future parallel runner. Increment 3
must decide whether `Pipeline.Run` executes stages sequentially in topological
order or concurrently across independent stages.

## Decision

**`Pipeline.Run` executes stages sequentially, one at a time, in the
deterministic topological order computed at construction.** Parallel execution
of independent stages is **deferred**.

- **Debuggability is the project's first-class criterion** (no cgo; a developer
  can set a breakpoint and read a plain Go stack trace). Sequential execution
  means a single, reproducible order: stepping through `Run` visits stages in a
  fixed sequence, a failing stage's stack trace is unambiguous, and there is no
  scheduler nondeterminism to reason about. Concurrency would trade this away
  for a throughput win that no current consumer has asked for — the engine
  evaluates small rule sets, where per-stage `expr` VM calls are fast and
  synchronous and the coordination overhead of a concurrent runner would
  likely dominate.
- **Cancellation** is honored between stages: `Run` checks `ctx.Err()` before
  each stage and returns it unwrapped if canceled, so any `Stage` implementation
  (not only the built-ins, which also self-check) respects cancellation.
- **First error stops** the walk; the failing stage's own `*StageError`
  (already naming it) is returned without re-wrapping.
- The deterministic input-order-preserving sort (ADR-0005) means the order is a
  defined property, not an artifact — so if a concurrent variant is ever added,
  its *default* observable ordering can match this one.

## Consequences

- Simple, reproducible, debuggable execution now; YAGNI honored (no concurrency
  machinery, no goroutine/`errgroup` lifecycle to get right, no partial-failure
  semantics to define).
- If profiling on a real workload later shows independent-stage parallelism is
  worth it, it arrives as an **additive** change — e.g. a `NewConcurrentPipeline`
  / `WithConcurrency` sibling to `NewPipeline` (ADR-0005 kept the constructor
  open for this) — and **this ADR is superseded** by the one that introduces it,
  rather than edited. The Spec 002 concurrency invariant is already in place to
  make that safe.
- Sequential `Run` on a shared `Scope` needs no additional synchronization
  beyond `Scope`'s existing mutex; the mutex remains (it guards the deferred
  concurrent future and same-value concurrent `Execute` on different Scopes).
