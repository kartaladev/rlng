# ADR-0054 — Unify numeric coercion onto one overflow-checked kernel

- **Status:** Accepted
- **Date:** 2026-07-14
- **Prompted by:** Spec 029 / Plan 029, Task 1, backlog item **R1** (`docs/BACKLOG.md`, "Post-audit refactor
  items") — the increment-029 whole-codebase audit's headline finding.

## Context

`pipe/get.go` (`coerceToInt64`/`coerceToFloat64`, ADR-0044) and `pipe/table.go`
(`toInt64`/`toFloat64`/`asDecimal`, ADR-0038/0039) each carry their own reflect-based numeric conversion: an
integer/unsigned-integer/float `reflect.Value` widened to `int64`, `float64`, or `decimal.Decimal`. The two
switches were written independently and diverged — `get.go`'s uint64→int64 conversion overflow-checks against
`math.MaxInt64`, but `table.go`'s `toInt64` cast straight through `int64(rv.Uint())`, silently wrapping a
`uint64 > math.MaxInt64` sum to a negative or truncated value. That divergence *was* pipe bug #3, fixed in the
increment-029 audit by adding the missing check to `table.go` in isolation — which papered over the symptom
but left two copies of the same logic free to diverge again the next time either file changes.

The two callers are not interchangeable, though: `get.go` is **fail-loud and text-accepting** (it also
converts `string` and `json.Number`, erroring on overflow/non-integral/non-finite input), while `table.go` is
**trusting and text-rejecting** (`classify` has already rejected non-numeric/text values as
`kindNonNumeric` and promoted an out-of-range `uint64` to `kindDecimal`, so `toInt64`/`toFloat64`/`asDecimal`
only ever see a value already guaranteed to convert). A naive "merge the two files" refactor would either
weaken `get.go`'s fail-loud text handling or force `table.go` to re-implement type gates it doesn't need —
either way changing observable behavior, which this batch (Spec 029) forbids.

## Decision

Extract only the **shared reflect kernel** — not the callers' outer policy — into one new unexported file,
`pipe/numeric.go`:

- `int64FromNumeric(rv reflect.Value) (int64, error)` — int/uint kinds → `int64`, overflow-checked
  (`uint64 > math.MaxInt64` is an error), `errNotNumericKind` otherwise.
- `float64FromNumeric(rv reflect.Value) (float64, error)` — int/uint/float kinds → `float64`,
  `errNotNumericKind` otherwise.
- `decimalFromNumeric(rv reflect.Value) (decimal.Decimal, bool)` — int/uint (full `uint64` range via
  `big.Int`, never through `int64`) / finite-float kinds → `decimal.Decimal`; `ok=false` for a non-finite float
  or a non-numeric kind.
- `errNotNumericKind` — the shared sentinel a caller translates into its own contextual failure.

Both callers keep their own outer type-set policy and delegate only the reflect-conversion tail to the
kernel:

- `get.go`'s `coerceToInt64`/`coerceToFloat64` keep their `json.Number`/`string` heads and the float
  finite/integral/overflow checks (these are *not* reflect-kind conversions — a raw float has no distinct
  reflect kind to gate on beyond `Float32`/`Float64`, and the finite/integral rules are specific to the
  fail-loud coercing-getter contract, ADR-0044), then fall through to `int64FromNumeric`/`float64FromNumeric`
  for the int/uint reflect cases, translating `errNotNumericKind` into their existing `"%T is not numeric"`
  message.
- `table.go`'s `classify` (kind ranking, decimal/uint64 promotion) is **unchanged** — it is the outer policy,
  not the kernel. `toInt64`/`toFloat64`/`asDecimal` become thin adapters over the kernel, keeping their
  "caller guarantees the classification, return zero/false on the unreachable unexpected kind" contract.

This deletes the literal duplicated reflect switch — the divergence surface behind bug #3 — while every
accepted type-set and every error stays exactly as it is today. The kernel is **unexported** (internal to
package `pipe`): no public-API change, confirmed by `gorelease`/`apidiff` reporting no exported-surface delta
against the last tag.

*Alternative considered:* merge `get.go` and `table.go`'s numeric handling into one shared function including
the outer policy (text acceptance, classification). Rejected — the two callers have genuinely different
accepted type-sets and failure contracts (fail-loud-and-text-accepting vs. trusting-and-text-rejecting); a
merged function would have to either grow a mode flag (defeating the simplification) or change one caller's
observable behavior, which this batch (Spec 029) explicitly forbids.

## Consequences

- **Single place to change numeric-reflection semantics.** A future overflow rule or new numeric kind is
  fixed once in `pipe/numeric.go` and both callers pick it up; the bug-#3 failure mode (fix one copy, leave the
  other stale) cannot recur for this specific logic.
- **No public-API change.** `int64FromNumeric`/`float64FromNumeric`/`decimalFromNumeric`/`errNotNumericKind`
  are unexported; `gorelease`/`apidiff` reports no exported-surface delta.
- **No behavior change.** Every existing `pipe` test stays byte-green; new characterization tests
  (`pipe/get_test.go` `TestGetIntCoerceKernelBranches`, a new row in `pipe/table_test.go`
  `TestDecisionTableCollectAggregationFidelity`) pin the exact current semantics — including the bug-#3
  regression (a `uint64 > math.MaxInt64` overflow-errors through `GetInt64Coerce` and promotes to an exact
  `decimal.Decimal` through `AggregateSum`) — across the refactor.
- **The fold path (`table.go`) observes no new error.** `classify` already excludes every input that would
  make `int64FromNumeric` fail (a `uint64 > MaxInt64` is pre-promoted to `kindDecimal`, never reaching
  `toInt64`), so the kernel's overflow check is provably unreachable from that caller — `toInt64`/`toFloat64`
  keep returning `0` on the (unreachable) unexpected-kind branch rather than surfacing a new error type.
- **`get.go` and `table.go` keep diverging on outer policy, deliberately.** A reviewer seeing `coerceToInt64`
  accept `string`/`json.Number` while `toInt64` does not should read this as intentional (different callers,
  different contracts), not as unfinished unification — this ADR is the record of that boundary.

## Traceability

Spec: 029 (`docs/specs/029-post-audit-refactor-batch.md`)
Plan: 029 (`docs/plans/029-post-audit-refactor-batch.md`), Task 1
Backlog: R1 (`docs/BACKLOG.md`)
Related: ADR-0044 (coercing Scope getters — `get.go`'s outer policy), ADR-0038 (value serde consistency —
canonical int64 kind), ADR-0039 (exact-decimal value type — `asDecimal`'s decimal side).
