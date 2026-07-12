# ADR-0038 — Value serde consistency: the value model & fidelity contract

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 014 (`docs/specs/014-value-serde-consistency.md`), Plan 014
  (`docs/plans/014-value-serde-consistency.md`) — post-010 audit remediation of
  the float64-money blocker (B1), generalized into a cross-cutting value serde
  consistency requirement.

## Context

The engine has no consistency guarantee for typed values as they cross its
serde boundaries — a value can silently change type or lose precision at each
hop. Spec 014 enumerates six such boundaries:

1. **Config decode** — a YAML/JSON literal → Go value (the scalar shorthand,
   integer vs float representation).
2. **Input seed** — a caller struct/map → `Scope` via `mapstructure`
   (`flatten` in `engine.go`).
3. **Expr env** — `Scope.Snapshot()` → the map handed to `expr.Run`; expr's own
   numeric model (int literals vs `float64`) governs arithmetic.
4. **Aggregation** — `foldNumeric` accumulates sums and folds across
   decision-table rows; naive `float64` accumulation loses precision above
   2^53 and mixed int/float representations can falsely disagree under
   `HitPolicyAny`.
5. **Scope persistence round-trip** — `pipe/json.go` Scope → JSON → Scope for
   audit/replay; a bare JSON number reloads as `float64` regardless of its
   original Go kind, so a persisted decision may not reproduce the same result
   on replay (ties to ADR-0037's replay guarantee).
6. **Result mapping** — `Scope` → typed `R` via `mapstructure`.

The sharpest symptom is money: `principal * 0.0725` on a $250k loan lands at
`18124.999999999996` in plain `float64` arithmetic, breaking reconciliation.
The ADR-0030 "integer minor units" interim only covers pure add/subtract of
pre-scaled amounts and breaks the instant a rate appears inside an expression
— the common case. But money is only the sharpest symptom; the root cause —
no end-to-end fidelity contract for values — affects every boundary above.

## Decision

**D1 — The canonical value kinds.** The engine guarantees fidelity for eight
value kinds across all six boundaries: `bool`, `string`, `int64` (sized
integer), `float64`, `decimal` (`decimal.Decimal`, see ADR-0039), `time`
(`time.Time`), `list` (`[]any`), and `map` (`map[string]any`). These are the
kinds a value is classified into at any serde boundary; every conversion at a
boundary is expected to land on one of these eight, not on an unbounded set of
Go types.

**D2 — The fidelity contract per boundary.** A value must mean the same
thing, and keep the same kind and precision, at every boundary it crosses:

- Config decode and struct/map seed: an integer literal or field is not
  silently widened to `float64`; a decimal literal (object form, see ADR-0039)
  decodes to `decimal.Decimal`, not a plain string.
- Expr env: native `int`/`float64` arithmetic is untouched (Spec 014
  non-goal — this ADR does not rewrite expr's evaluation model); a `decimal`
  value stays a `decimal.Decimal` through evaluation.
- Aggregation: an all-integer fold stays `int64`-exact (checked overflow, not
  a lossy `float64` round-trip); a fold touching a `decimal` value promotes to
  `decimal`; equality-based hit policies compare numeric values by magnitude,
  not by matching Go representation, so `10` (`int64`) and `10.0` (`float64`)
  are not a false conflict.
- Scope JSON persistence: every `data` scalar round-trips its **kind**, not
  just its printed text — an `int64` decision value reloads as `int64`, a
  `decimal` reloads as `decimal.Decimal`, a `time.Time` reloads as
  `time.Time` (RFC3339), not as a bare `json.Number`/string/float.
- Result mapping: the mapper does not lossily narrow a value into `R` without
  surfacing that narrowing as an error.

Where an exact representation is required (money, identifiers, large
integers), the engine offers a type that preserves it end-to-end — `decimal`
— rather than silently degrading to `float64`. Where a conversion is
genuinely lossy (e.g. narrowing a `decimal` into a Go `float32` result field),
the contract requires an error, not a silent truncation — consistent with the
project's debuggability principle of typed, surfaced failures over opaque
ones.

**D3 — Full canonical type tagging in Scope JSON, with a version marker and
legacy-read fallback.** To satisfy the persistence-boundary half of the
contract (D2), the Scope JSON envelope changes its `data` encoding from a
bare value to a canonically type-tagged form: every scalar encodes as
`{"$k":<kind>,"v":<payload>}`, where `<kind>` is one of the D1 kinds. The
envelope itself carries a schema-version marker (`"v":2` at the envelope
level, orthogonal to the per-value `"v"` payload key) so a reader can detect
which format it is looking at. The decoder rehydrates a `v2` blob by tag; a
**legacy (untagged, pre-014) blob still loads** via the existing bare-value
path (Spec 007 semantics: JSON numbers as `json.Number`, etc.), so reading old
decisions remains backward-compatible. Only the *write* path changes; nothing
that could already be read stops being readable.

## Consequences

- **A v2 blob is not readable by a pre-014 reader.** Because the tagged
  encoding (`{"$k":...,"v":...}`) is a different shape than a bare scalar, an
  older version of this library attempting to decode a v2-written blob into
  its expected bare-value shape will not recover the same value (the tag
  object, not the payload, is what it sees). This is a deliberate, one-way
  format evolution: new writers produce a format only new-enough readers fully
  understand. The envelope version marker (`"v":2`) makes this detectable at
  read time rather than silently misinterpreted — a reader can branch on the
  marker instead of guessing.
- **Numeric-aware aggregation can change a previously-lossy result to the
  correct one.** A decision that previously aggregated `10` and `10.0` as
  conflicting under `HitPolicyAny` (false conflict from `reflect.DeepEqual`
  comparing different Go kinds) will now agree; a sum previously silently
  truncated through `float64` above 2^53 will now either compute the correct
  `int64` sum or return a typed overflow error. Any consumer relying on the
  old, lossy behavior (including a previously-tolerated false-conflict error)
  will observe a changed decision outcome on the same input after upgrading —
  this is intentional, since the old behavior was incorrect per the fidelity
  contract this ADR establishes, but it is a behavior change worth calling out
  for anyone diffing decisions across a library upgrade.
- The eight-kind model (D1) is deliberately closed rather than open-ended —
  it is easier to extend to a ninth kind later (a new ADR) than to walk back an
  unbounded "any Go type is a value kind" contract.
- This ADR is the umbrella; ADR-0039 records the specific decision to depend
  on `github.com/shopspring/decimal` for the `decimal` kind introduced here.
