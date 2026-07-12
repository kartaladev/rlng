# ADR-0016 — expr env recursion bound and fallback semantics

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 008 (docs/specs/008-production-hardening.md), audit findings B2, M1, M9.

## Context

Three defects in `expr`:

- **B2** — `toEnv`'s `convertValue`/`structToMap` recurse through
  pointer/struct/slice/map fields with no depth bound. A caller-supplied struct
  env with a back-reference (ORM entities, tree/parent-child domain objects,
  linked lists) recurses until `fatal error: stack overflow` — unrecoverable by
  `recover()`, crashing the process. This violates the "never crash on caller
  input" rule and is a DoS vector via the public `Predicate.Test` /
  `Function.Apply`.
- **M1** — a `Function`'s fallback program is compiled with the base options, not
  the `mainOpts` that carry `WithReturnKind`'s `AsKind` coercion, so the fallback
  path returns the wrong Go type.
- **M9** — when the main expression errors and a fallback runs, `Apply` returns
  the fallback result (or the fallback's own error) and discards the original
  error, losing the root cause for a debuggability-first library.

## Decision

- **Bound the reflection (B2).** Thread a depth counter through
  `convertValue`/`structToMap` (limit `maxEnvDepth = 1000`, mirroring
  `provenance.maxLineageDepth`). Exceeding it returns an error surfaced from
  `toEnv` as an `*EvalError`, not a crash.
- **Coerce the fallback (M1).** Compile the fallback program with the same option
  set as the main program (including `AsKind` when `WithReturnKind` is set) so
  both paths honor the declared return kind.
- **Preserve the trigger (M9).** When the fallback runs because the main
  expression errored, join the main error into the returned error's cause via
  `errors.Join`, so both the fallback failure (if any) and the original reason
  survive `errors.Is`/`As`.

## Consequences

- The library no longer crashes on a cyclic/deep struct env; instead the caller
  gets a typed error naming the expression.
- `WithReturnKind` now applies uniformly; a fallback that previously leaked an
  uncoerced type is a behavior fix.
- The joined error changes the exact error string on the main-error→fallback
  path; `errors.Is` on the underlying cause continues to work.
