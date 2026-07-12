# ADR-0035 — Safe, honest lenient truthiness for WithCoerce

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 012 (docs/specs/012-evaluation-correctness-and-explainability.md) / Plan 012, deeper-audit finding 5.

## Context

Predicates compiled with the opt-in `WithCoerce` option use lenient truthiness
(`truthy` in `expr/predicate.go`) instead of the strict bool-only path. The
audit found two footguns in `truthy`:

- **`NaN` coerced to `true`.** The float branch returned `f != 0`, and `NaN != 0`
  is `true` — so a rule whose input degraded to `NaN` (an invalid computation)
  would *fire* rather than read as false/invalid. `±Inf` similarly coerced to
  `true`.
- **Unhandled types silently coerced to `false`.** The `default` branch returned
  `false` for any kind it did not recognize (struct, `time.Time`, non-nil
  pointer, chan, func). A mistyped predicate — one whose expression yields the
  wrong type — read as a benign `false` instead of failing, the opposite of the
  strict path, which returns an `ErrNotBool` `*EvalError`. Both behaviors were
  untested.

## Decision

Make `truthy` safe and honest, and give it an error return
(`truthy(v any) (bool, error)`):

- **Floats:** `true` iff non-zero **and finite** — `NaN` and `±Inf` are `false`.
- **Unhandled kinds:** return a wrapped `ErrNotBool` error rather than a silent
  `false`. `Test`'s coerce branch propagates it as
  `&EvalError{Expression: p.expression, Cause: err}`, so a mistyped coerced
  predicate fails loudly, consistent with the strict path.
- **String rules documented precisely** (unchanged behavior, now stated): a
  string is parsed via `strconv.ParseBool` when it names a bool
  (`1/t/T/TRUE/true/True`, `0/f/F/FALSE/false/False`), otherwise `true` iff
  non-empty after trimming. Nil→false; bool→itself; any int/uint kind→non-zero;
  slice/array/map→non-empty.

Strict (non-coerce) predicate behavior is unchanged — it was already correct.

## Consequences

- **Breaking (SemVer):** under `WithCoerce`, a `NaN` result now coerces to
  `false` (was `true`), and an unhandled result type is now an `*EvalError` (was
  a silent `false`). Acceptable pre-`v0.1.0`; the commit subject is flagged
  breaking (`fix(expr)!`), and the full coercion matrix (including the changed
  cases) is now covered by tests.
- A degraded/invalid numeric input (`NaN`/`±Inf`) can no longer make a coerced
  rule fire — the safer reading for business rules.
- A mistyped coerced predicate surfaces a typed, debuggable error instead of a
  plausible `false`, matching the project's debuggability mandate and the strict
  path's contract.
- The `ParseBool`-subset string behavior is a documented, deliberate rule rather
  than an accidental one; callers who need strict boolean strings can use a
  non-coerce predicate.
