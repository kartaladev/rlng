# ADR-0026 — Rule identity and firing-rule provenance

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P1-#6.

## Context

A decision-table `Rule` had only a condition and decisions. Provenance
(`Explain`) traced the *expression* that produced a value, but nothing recorded
*which rule* decided a stage. A compliance-grade decision needs "Denied by rule
`CREDIT_MIN_650`" — the rule's identity, not just the arithmetic.

## Decision

- `Rule` gains optional `ID` and `Message` fields.
- `Scope` records the firing rule per decision-table stage via a `FiringRule`
  `{Stage, RuleID, Message, IsDefault}`, exposed by `Scope.FiringRule(stage)` and
  `Scope.FiringRules()` (sorted by stage — a compact audit trail).
- Recording is **independent of provenance**: it is a single cheap map write, so
  the firing rule is available even when full lineage tracking is off.
- Firing is recorded for the matched rule under `single`, `unique`, and `any`
  policies, and for the default with `IsDefault=true` when it fires. `collect`
  records no single firing rule (it is inherently multi-rule); its per-value
  provenance already lists every contributing expression.

## Consequences

- Decisions are explainable by rule identity, closing the audit gap for
  eligibility/adjudication use cases.
- `FiringRule`/`FiringRules` are new exported surface on `Scope`; additive, no
  break.
- `collect` deliberately has no firing rule; callers wanting per-rule attribution
  under collect use provenance lineage instead.
