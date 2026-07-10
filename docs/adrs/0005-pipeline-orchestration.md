# ADR-0005 — Pipeline placement, name, and construction/validation model

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 003 (docs/specs/003-dag-orchestration.md)

## Context

Increment 3 introduces the orchestrator that consumes the `DependsOn()` each
stage declared in Increment 2: a component that holds a set of `Stage` values,
validates them, dependency-orders them, and runs them against a shared `Scope`.
Several decisions needed recording: where the type lives, what it is called,
how it is constructed, and how construction-time failures are reported. (The
execution model — sequential vs concurrent — is consequential enough to isolate
in its own record, ADR-0006, so it can be superseded independently.)

## Decision

1. **Placement: the `stage` package.** The orchestrator operates purely on
   `Stage` and `Scope`, both of which live in `stage`; putting it there keeps it
   next to what it composes and leaves the root `rlng` package empty for the
   Increment-5 `Engine[I, R]` facade (which will wrap this orchestrator with
   typed input/result mapping). No new package is introduced.

2. **Name: `Pipeline`.** Of the candidates (`Pipeline` / `Graph` / `Flow` /
   `DAG`), `Pipeline` is the most recognizable name for "a set of stages run in
   dependency order," and `Pipeline.Run(ctx, sc)` reads naturally. `Graph`/`DAG`
   describe the internal structure, not the behavior; `Flow` is vague. The set
   is a DAG internally, but consumers construct and run a *pipeline of stages*.

3. **Constructor: `NewPipeline(stages ...Stage) (*Pipeline, error)`.** Variadic
   is ergonomic both for a fixed set (`NewPipeline(a, b, c)`) and for a slice
   built dynamically (`NewPipeline(built...)`, as Increment 4 will do from
   config). No per-pipeline options exist this increment, so no `PipelineOption`
   type is introduced (it would be an empty extension point); a future option
   (e.g. concurrency, ADR-0006) can arrive as an additive sibling constructor
   without breaking this signature. An **empty set is valid** — `Run` is then a
   no-op — so a pipeline built from an empty config is not a special case.

4. **Validation, at construction, in order — all failures typed:**
   1. **Duplicate stage names** → `*DuplicateStageError{Name}`.
   2. A `DependsOn` target that names **no stage in the set** →
      `*UnknownDependencyError{Stage, Dependency}`. (A self-dependency names a
      real stage, so it passes here and is caught as a cycle next.)
   3. **Dependency cycle** → `*CycleError{Cycle}`, where `Cycle` is a **concrete
      loop path** closing on the repeated node (e.g. `["a","b","a"]`), extracted
      by a depth-first search over the unorderable remainder.

   These are **three distinct types**, not one struct with a kind enum, because
   each carries different identifying data and `errors.As` then reaches the
   exact failure — the same typed-error discipline `expr` (`CompileError`/
   `EvalError`) and `stage` (`StageError`) already follow, and the reason the
   cycle error prints the actual loop rather than a boolean "cycle exists."

5. **Ordering: input-order-preserving topological sort.** Among stages that are
   ready (all dependencies already ordered), emit in **constructor input order**,
   so the execution order is deterministic and intuitive rather than
   map-iteration dependent. The simple O(n²) Kahn form (repeatedly emit the
   first ready, not-yet-emitted stage) is chosen over an ordered-queue variant:
   stage counts in a rule engine are small, and the straightforward form is
   easier to read and debug for an identical result. The order is computed once
   at construction and stored; `Run` is a straight ordered walk.

## Consequences

- The orchestrator sits beside the stages it composes; Increment 5's `Engine`
  wraps a `Pipeline` without either package importing the other's internals.
- `Pipeline` does **not** implement `Stage` (no nested pipelines) — deliberately
  out of scope (YAGNI); revisit with a superseding ADR if composition is needed.
- Distinct typed construction errors add three small exported types to the
  surface, justified by the debuggability goal; callers pattern-match with
  `errors.As`.
- Choosing O(n²) ordering is a documented small-N trade-off; if very large
  stage sets ever appear, switching to a linear Kahn is an internal change with
  no API impact (the order is defined as input-order-preserving either way).
- The variadic constructor with no options is a minimal surface now; adding
  concurrency later (ADR-0006) is additive, not breaking.
