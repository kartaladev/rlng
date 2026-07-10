# ADR-0002 — Stage execution model and `Scope` naming

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 002 (docs/specs/002-scope-and-stages.md)

## Context

Increment 2 introduces the layer above the atomic `expr` evaluators: an
accumulator threaded through evaluation, and stage types that read from and
write to it. Two decisions needed recording. First, what the stage abstraction
is and how a stage runs in isolation, given that dependency-ordered execution
(the DAG) is deferred to Increment 3. Second, what to call the accumulator: the
design brief called it "Context", but `Execute` takes a `context.Context`, so a
`*Context` second argument would put two different "Context" types in one
signature.

## Decision

- A `Stage` is `Name() / Type() / DependsOn() / Execute(ctx context.Context, sc *Scope) error`.
  Stages compile all expressions in their constructor (errors surface early) and
  only evaluate in `Execute`. `Execute` reads a `Scope` snapshot, evaluates, and
  writes results back under a dot-path namespace keyed by the stage name.
- Stages **declare** `DependsOn()` but do not act on it; ordering stages into a
  dependency DAG (topo-sort + cycle detection) is Increment 3. Each stage is
  independently constructible and executable this increment.
- The accumulator is named **`Scope`**, not `Context`, to avoid the
  double-`Context` signature and to respect Go's convention that `Context` means
  `context.Context`. `Scope` names "the variables in scope during evaluation,
  growing as each stage contributes". This realizes the brief's "Context
  accumulator" under a clearer name.
- Failures are a typed `StageError` that names the stage and unwraps to the
  underlying `expr` error, preserving the field+expression debuggability chain.

## Consequences

- Stages are testable in isolation now; the DAG runner in Increment 3 consumes
  the already-declared `DependsOn()` without changing the stage contract.
- CLAUDE.md's architecture blueprint (which uses "Context") is refreshed to say
  `Scope` as part of completing this increment (plan 002, Task 6), so the docs
  do not drift.
- Supersede this ADR rather than editing it if the stage contract changes.
