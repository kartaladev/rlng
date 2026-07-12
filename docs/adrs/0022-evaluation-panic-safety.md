# ADR-0022 — Evaluation panic safety

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P0-#3.

## Context

A rule engine embedded in a consumer's process must not let one bad rule crash
the host. The audit flagged the risk that a panic — from a host function
(ADR-0024), a method invoked on caller-supplied env data (`account.Risk()`), or
the expression VM itself — could propagate as an unrecovered Go panic out of
`Function.Apply` / `Predicate.Test`.

Empirically, the underlying `expr-lang/expr` VM installs a top-level `recover`
in `expr.Run`: panics from host functions, env-value methods, and VM internals
are all converted into ordinary returned errors (verified against v1.17.8 for
each of those paths). So the host is already protected on the evaluation path.

## Decision

Rather than wrap `Apply`/`Test` in a second, redundant `recover` — which would
add a branch that no public-API input can reach (the VM never lets a panic
escape), leaving it untestable under the blackbox-only + coverage gates — we
**rely on the VM's recovery and lock the guarantee with a regression test**.
`TestEvalPanicIsError` drives a panicking host function through both `Apply` and
`Test` and asserts a typed `EvalError` is returned, not a crash.

If a future expr version, or a new non-VM evaluation path we add, can let a panic
escape, the test fails and we add an explicit `recover` at that point.

## Consequences

- No dead defensive code; the safety property is asserted, not assumed.
- The guarantee is documented on `Apply`/`Test`: evaluation panics surface as
  `EvalError`.
- Coupled to expr's recovery behavior — pinned by the regression test, so a
  dependency bump that regresses it is caught in CI.
