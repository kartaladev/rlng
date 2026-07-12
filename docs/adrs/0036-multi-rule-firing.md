# ADR-0036 — Multi-rule firing for collect and any policies

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 012 (docs/specs/012-evaluation-correctness-and-explainability.md) / Plan 012, deeper-audit findings 1 and 2.

## Context

`Scope` records, per decision-table stage, which rule "fired" — a provenance
trail (`FiringRule`/`FiringRules`) that answers *which rule decided this stage*
for adverse-action reasoning. ADR-0026 introduced this as a single firing rule
per stage. The audit found the model was too narrow for the multi-match
policies:

- **`HitPolicyCollect` recorded no firing on a match.** `executeCollect` called
  `recordFiring` only in the no-match/default branch — never when rules matched.
  So a collect table that fired wrote its outputs correctly, yet
  `FiringRule(stage)` returned `(_, false)` and `FiringRules()` omitted the
  stage. This blanked the explainability trail for exactly the multi-reason
  "denied for reasons A, B, C" shape that collect exists for.
- **`HitPolicyAny` firing was lossy.** When several rules agreed, only the first
  matched rule's identity was recorded — a multi-rule agreement could not be
  fully explained.

The single-rule-per-stage store (`map[string]FiringRule`) could not represent
several contributing rules, so the store shape itself had to change.

## Decision

Extend the firing store from one rule per stage to an **ordered slice** per
stage (`map[string][]FiringRule`), and record every contributing rule:

- `recordFirings(stage, []FiringRule)` (new) stores an ordered multi-rule set,
  replacing any prior record for the stage. `recordFiring` (single) is preserved
  and delegates to it with a one-element slice.
- `executeCollect` records one `FiringRule` per matched rule (in rule order),
  before aggregation; a no-match run still records the default via
  `applyDefaults`.
- `executeAny` records every agreeing rule (in rule order) after the agreement
  loop succeeds — a conflict still errors **without** recording a stray firing.
- Accessors: `FiringRule(stage)` returns the **first** rule for the stage
  (preserved); `FiringRules()` flattens all stages' rules sorted by stage then
  firing order (preserved signature); `FiringRulesFor(stage)` (new) returns the
  full ordered slice for one stage (a defensive copy; nil if none).

## Consequences

- Collect and any decisions are now fully explainable: every contributing rule
  ID/message is captured for adverse-action reasoning, closing a hot-path branch
  that previously had zero test coverage.
- **Backward compatible:** `FiringRule` and `FiringRules` keep their signatures
  and their single-rule behavior; existing callers see no change. Only the
  unexported store type changed, plus the additive `FiringRulesFor`.
- Firing is recorded independent of provenance (a cheap audit) and under the
  `Scope` mutex, preserving concurrency safety.
- **Known gap (out of scope here):** the `Scope` JSON codec (`pipe/json.go`)
  does not yet serialize firing; a replayable decision record that round-trips
  firing + ruleset identity is owned by Spec 013. Recorded here so it is not
  lost.
