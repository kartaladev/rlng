# Spec 014 — Value serialization/deserialization consistency

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-010 audit remediation.** Generalizes the audit's "exact-decimal money"
  blocker (B1) into a cross-cutting **value serde consistency** requirement;
  exact-decimal money is the primary motivating use case, not the whole spec.
- **Builds on:** Spec 001 (`expr` evaluation), Spec 002 (`pipe` Scope),
  Spec 005 (`Engine`/`Mapper` seeding & mapping), Spec 007 (Scope JSON),
  Spec 010 (collect aggregation), ADR-0030 (decimal deferred).
- **Realized by:** Plan 014.
- **Anticipated ADRs:** ADR-0038 (value serde consistency invariants & the value
  model), ADR-0039 (exact-decimal value type).
- **Relates to:** Spec 013 (a decision is only replayable if both its ruleset
  identity *and* its values round-trip losslessly).

## Context — one problem, six boundaries

The audit surfaced a float64-money blocker: `principal * 0.0725` on a $250k loan
lands at `18124.999999999996`, breaking reconciliation; the ADR-0030 "integer
minor units" interim only holds for pure add/subtract of pre-scaled amounts and
**breaks the instant a rate appears inside an expression** (the common case), and
no `round()`/decimal builtin is exposed so authors cannot even round
deterministically in-rule.

But money is only the sharpest symptom. The root issue is that rlng has **no
consistency guarantee for typed values as they cross its serde boundaries** — a
value can silently change type or lose precision at each hop:

1. **Config decode** — a YAML/JSON literal → Go value; integer vs float
   representation, the scalar shorthand.
2. **Input seed** — a caller struct/map → Scope via `mapstructure` (`flatten` in
   `engine.go`); field types may be converted.
3. **Expr env** — `Scope.Snapshot()` → the map handed to `expr.Run`; expr's own
   numeric model (int literals vs `float64`) governs arithmetic.
4. **Aggregation** — `foldNumeric` (`pipe/table.go:454-479`) accumulates integer
   sums in `float64` and returns `int(acc)`: silent precision loss above 2^53 and
   platform-dependent overflow. `HitPolicyAny` agreement uses `reflect.DeepEqual`
   (`table.go:316`), so `10` (int) and `10.0` (float) falsely conflict.
5. **Scope persistence round-trip** — `pipe/json.go` Scope→JSON→Scope for
   audit/replay: JSON numbers reload as `float64`, so an `int64` decision value
   comes back a float, and a persisted decision may **not reproduce** the same
   result on replay (ties to Spec 013).
6. **Result mapping** — Scope → typed `R` via `mapstructure` (`mapper.go`).

## The invariant

A value must **mean the same thing, and keep the same type and precision, at every
boundary it crosses**, and a persist→reload round-trip must be **lossless and
reproduce the same decision**. Where an exact representation is required (money,
identifiers, large integers), the engine must offer a type that preserves it
end-to-end rather than silently degrading to `float64`.

## Goals

1. **G1 — Define the value model & consistency invariants (ADR-0038).** Enumerate
   the canonical value kinds the engine guarantees across all six boundaries
   (bool, string, sized integer, float, exact-decimal, time, list, map) and the
   fidelity contract for each: what is preserved, what is *documented* to convert,
   and where a lossy conversion becomes an error rather than a silent degrade.
   This is the spec's backbone; the remaining goals are its enforcement points.
2. **G2 — Numeric fidelity in aggregation.** `foldNumeric` accumulates integer-only
   sums in `int64` (falling to `float64`/decimal only when a non-integer is
   present) and errors on overflow instead of round-tripping through `float64`;
   `min`/`max` return the actual matched element without precision loss.
   `HitPolicyAny` agreement compares values numerically-aware (equal magnitude
   regardless of int/float kind) so type representation alone does not raise a
   false `ErrConflictingMatches`.
3. **G3 — Lossless, deterministic Scope JSON round-trip.** The Scope JSON
   serialization preserves value kind on reload (an integer decision reloads as an
   integer, an exact-decimal as itself) so a persisted decision reproduces the
   same result — the serde half of Spec 013's replay guarantee. Serialization is
   deterministic (already sorted-key; assert it) so the same decision serializes
   byte-identically.
4. **G4 — Exact-decimal value type (ADR-0039) — the motivating case.** Introduce
   an exact-decimal value usable inside expressions (a custom expr type with
   operator/arithmetic support, or an equivalent), so money/rate math is exact end
   to end, plus rounding builtins (`round`, and banker's/half-even rounding) so an
   author can round deterministically in-rule. The $250k-at-7.25% example is a
   driving acceptance test: the fee is exactly `$18,125.00`, survives the JSON
   round-trip, and reproduces on replay. Decide (ADR-0039) between a decimal
   library dependency vs an in-house minor-units type, honoring the minimal-deps
   and pure-Go/no-cgo constraints.
5. **G5 — Consistency across config decode / input seed / result mapping.** Ensure
   a value declared in config, seeded from a struct, and mapped into a result type
   preserves kind per the G1 contract (e.g. an integer constant is not silently
   widened to float on the way in, and the mapper does not lossily narrow on the
   way out), or errors where the contract says a conversion is lossy.

## Non-goals

- A full numeric tower or arbitrary-precision arithmetic beyond exact decimal.
- Changing expr's own evaluation model for native `int`/`float` literals (G4 adds
  a decimal type *alongside*, it does not rewrite expr arithmetic).
- Currency handling (rounding rules per ISO currency, multi-currency) — a decimal
  *value* is in scope; currency semantics are a caller/BRMS concern.

## Hot-path branches (test targets)

- Aggregation fidelity: integer-only `sum` stays `int64` and is exact above 2^53;
  overflow errors (not garbage); mixed int/float sum promotes per the contract;
  `min`/`max` return the exact matched element.
- `HitPolicyAny`: `10` (int) and `10.0` (float) agree (no false conflict); genuine
  disagreement still errors.
- JSON round-trip: integer / exact-decimal / bool / string / nested map decision
  values reload with the same kind; a round-tripped decision reproduces the same
  result; serialization is byte-deterministic.
- Decimal: `0.1 + 0.2` is exact; `principal * rate` for the driving loan example
  is exact and rounds deterministically; decimal survives seed → eval →
  aggregate → JSON → reload → map.
- Seed/mapping fidelity: an integer config constant / struct field is not silently
  widened to float; a lossy result-mapping narrowing errors per the G1 contract.
