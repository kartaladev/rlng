# ADR-0030 — Decimal money and foreach stage: deferred

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit findings P0-#5 (decimal) and P1-#8 (iteration).

## Context

The business-rule audit raised two features that real rulesets eventually need:

- **Exact decimal arithmetic.** `expr` evaluates numbers as `float64`, so money
  math (tax, interest, pricing) accumulates rounding error.
- **Iteration over collections.** Adjudication often runs *per line item / per
  claim* — apply a decision table to each element of a slice.

Both are worth doing, but neither is a small change, and rushing either into this
hardening pass risks a poor API baked in before `v0.0.1`.

## Decision

Defer both, deliberately, and record why:

- **Decimal** is a cross-cutting design: a custom `expr` type with operator
  handling, a compile/return-kind story, config representation, and mapping/JSON
  round-trip. It deserves its own spec. Until then the `float64` semantics are
  documented as a caveat on aggregation and pricing paths, and callers needing
  exactness pre-scale to integer minor units (cents) — which the integer-
  preserving aggregation (ADR-0025) supports today.
- **`foreach` stage** is a new stage type (a sub-pipeline or table applied per
  element, result collected back), warranting its own spec covering scoping,
  provenance, and error semantics per element. The `expr` builtins
  (`map`/`filter`/`reduce`) already cover the scalar-in-one-expression case, so
  there is a usable interim path.

## Consequences

- No half-designed API is shipped under time pressure; each feature gets a proper
  spec when prioritized.
- The interim guidance (integer minor units; `map`/`filter`/`reduce`) is
  documented, so the gaps are navigable, not blocking.
- This ADR is the tracking record; a future spec supersedes the relevant half
  when the feature lands.
