# ADR-0024 — Host functions in expressions

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P1-#7.

## Context

Real-world rules need domain helpers the built-in expr language cannot express:
`businessDaysBetween(a, b)`, a currency conversion, a table lookup, or a
deterministic `now()` for temporal rules ("expires within 30 days"). Before this
change there was no way to inject a host function into the evaluation
environment; a rule author was limited to expr builtins plus scalar
globals/locals.

## Decision

Add `expr.WithFunction(name string, fn func(...any) (any, error)) Option`. Each
registered function is appended as an `expr.Function(name, fn)` compile option in
`buildExprOpts`, so it is visible to **both** the compiler (it type-checks,
including under `WithEnv` strict mode — ADR-0023) and the VM. Registering the
same name twice keeps the last registration (last-write-wins), matching Go option
semantics.

The signature deliberately mirrors expr's own `func(...any) (any, error)` rather
than inventing a new abstraction: `expr` is the project's declared foundation and
is already exposed in the public API (`expr.Option`), so there is no vendor-lock
concern to hide behind an indirection (unlike watermill/casbin in the sibling
`wrkflw` project).

A clock-backed `now()` is not hardwired here; it is composed at the `pipe`/engine
layer from an injected `Clock` (ADR-0021) via `WithFunction`, keeping `expr`
free of a time dependency.

## Consequences

- Rules can call arbitrary host logic; the function author owns its safety. A
  panicking function is contained (ADR-0022) and returns an `EvalError`.
- Strict-mode compilation still type-checks calls, so an arity/type mistake in a
  call site is caught at construction.
- Determinism for temporal rules is achieved by injecting a clock-backed `now()`,
  never by reading the wall clock inside `expr`.
