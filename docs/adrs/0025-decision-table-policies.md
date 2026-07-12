# ADR-0025 — Decision-table hit policies, default, and collect aggregation

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P0-#2.

## Context

The decision table had two behaviors: `HitPolicySingle` (first match wins) and
`HitPolicyCollect` (accumulate matches into a slice). Two gaps make it unsafe or
insufficient for real rulesets:

1. On **no match**, `single` wrote nothing — a downstream mapper then reads a
   missing key as a zero/nil, so "no rule applied" silently becomes a valid-looking
   decision. In adjudication that is a dangerous default.
2. `collect` could only produce a list. Real pricing/eligibility needs "sum of
   applicable fees", "max discount", "number of triggered flags".
3. There was no way to assert a table is **non-overlapping** (exactly one rule
   should apply) or that multiple applicable rules **agree**.

DMN defines a richer hit-policy set; we adopt the subset that carries its weight.

## Decision

- **Default (else) decisions** via `WithDefault(map[string]string, opts...)`,
  applied by every policy when no rule matches. "No match" becomes explicit.
- **`HitPolicyUnique`** — at most one rule may match; two or more is
  `ErrMultipleMatches`. Guards tables authored to be mutually exclusive.
- **`HitPolicyAny`** — several rules may match but must agree on every shared
  output key; a disagreement is `ErrConflictingMatches`.
- **Collect aggregation** via `WithCollectAggregation(CollectAggregation)`:
  `AggregateList` (default, unchanged), `AggregateSum`, `AggregateMin`,
  `AggregateMax`, `AggregateCount`. Sum/min/max operate on numeric values
  (integers stay integers; a float in the set promotes the result to float); a
  non-numeric value is `ErrNonNumericAggregate`. Count returns the match count.

All new error branches are typed (`errors.Is`-able) and wrapped in the existing
`StageError`, preserving the debuggability contract. Provenance is retained: the
existing `collect:<key>` label is unchanged for the default list aggregation; a
reducer is named (`collect:sum:<key>`) for aggregated collects, and agreed values
are labeled `any:<key>`.

Numeric aggregation deliberately does **not** span non-numeric types (no string
min/max) in this pass — scope-limited to the common financial case; extendable
later without an API break.

## Consequences

- "No match" is a first-class, testable outcome instead of a silent gap.
- Overlap/agreement invariants are enforced at evaluation, catching authoring
  bugs a lint pass (ADR-0029) can only warn about statically.
- Aggregations cover the common pricing reductions natively rather than pushing
  callers to post-process a list.
- `float64` semantics still apply to numeric aggregation; exact decimal money is
  deferred (ADR-0030).
