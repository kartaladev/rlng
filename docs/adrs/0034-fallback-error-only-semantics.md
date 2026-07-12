# ADR-0034 — Fallback fires on error only; nil stays first-class

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 012 (docs/specs/012-evaluation-correctness-and-explainability.md) / Plan 012, deeper-audit findings 3 and 4.

## Context

A `Function` configured with `WithFallback` evaluated its fallback whenever the
main expression either **errored** or evaluated to **nil** (`expr/function.go`
`Apply`). The audit found two defects in that behavior:

1. **Genuine errors were silently swallowed.** When the main expression errored
   (divide-by-zero, a typo'd field, a broken host function) and a fallback was
   configured, `Apply` returned `(fallbackResult, nil)` — the triggering error
   was discarded entirely, surviving only if the fallback *also* failed. A
   business user saw a plausible fallback number and never learned the rule was
   broken. This directly contradicts the project's first-class debuggability
   mandate. `function_test.go` asserted `require.NoError` on exactly this,
   locking in the masking.
2. **`nil` was conflated with failure.** Because the fallback also fired on a
   `nil` main result, a Function with a fallback could **never** return `nil`. A
   legitimate "no discount" (`nil`) was silently overwritten by the fallback,
   indistinguishable from a genuine value.

## Decision

Change `Apply` so the fallback fires on **error only** by default, and `nil`
stays a first-class result:

- **Error path:** unchanged in that the fallback still runs, but the triggering
  cause is now made **observable**. `WithFallbackObserver(fn func(name,
  expression string, cause error))` registers a callback invoked with the
  function name, main expression, and the original error before the fallback
  runs. Default nil (no-op). When no fallback is configured, the error is
  propagated as an `*EvalError` as before (never dropped).
- **Nil path:** by default a `nil` main result is returned as-is; the fallback
  does **not** fire. `WithFallbackOnNil()` opts back into the old behavior
  (fallback fires on `nil`). The observer is **never** called for a
  nil-triggered fallback — it exists to surface masked *errors*, and a `nil`
  result is not an error.

`runFallback` is unchanged: if the fallback itself fails, the main error (when
present) is joined into the returned `*EvalError` so no cause is lost.

## Consequences

- **Breaking (SemVer):** the default fallback semantics change — a `nil` main
  result with a configured fallback now returns `nil` instead of the fallback,
  and callers relying on the old nil→fallback behavior must add
  `WithFallbackOnNil()`. This is acceptable pre-`v0.1.0`; the existing
  `function_test.go` case that asserted the old behavior was updated in the same
  commit, and the commit subject is flagged breaking (`feat(expr)!`).
- An error-triggered fallback is no longer a silent trap: with an observer wired
  (e.g. to a logger or metric), operators see every masked failure while callers
  still get the resilient fallback value.
- `nil` becomes a usable "no result" signal for Functions, distinct from both a
  fallback value and a genuine zero.
- The two new options are additive; a Function with neither behaves exactly as
  the new default (error-only, nil-first-class).
