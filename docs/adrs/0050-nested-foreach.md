# ADR-0050 — nested foreach: hierarchical firing keys, config gate lift, `as`-chain guard

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 025 / Plan 025, graduating backlog B9 — the nested-foreach deferral recorded in
  ADR-0040 D7 (enforced by config.ErrNestedForEach).

## Context

Nested `foreach` already ran programmatically (ADR-0040 kept the `pipe` layer nesting-agnostic), but the
config layer rejected it (ErrNestedForEach), and "runs" was not "works well": the outer recorded an
element's inner firings via a flatten (`esc.FiringRules()`) that discarded the inner element index, and
outer/inner both defaulting `as: item` silently shadowed the outer element. Derivations, by contrast,
already composed via the per-element `recordElementDerivations` call (a nested ForEach runs inside the
per-element scope, so its derivations were already keyed `<inner>[j].…`).

## Decision

- **Firing composes by hierarchical re-prefix, mirroring derivations.** `ForEach.Execute` now copies the
  per-element scope's firing *map* under a `<stage>[i].` prefix (`recordElementFirings`) instead of
  flattening it into one key. Keys end at the decision-table stage (`lines[0].check`;
  `lines[0].taxes[1].vat`), composing to any depth. `FiringRule.Stage` keeps its meaning (the bare DT name);
  nesting lives only in the map key. No nesting-detection logic enters `ForEach` — the change is uniform for
  single-level and nested.
- **Config gate lifted; nesting builds by existing recursion.** `buildForEach` already builds inner stages
  via `isd.build`, which dispatches a `type: foreach` inner stage back into `buildForEach`; deleting the
  ErrNestedForEach rejection is sufficient.
- **`as`-chain collision rejected at build time.** `validateForEachAsChains` (called from `Build`) rejects
  any root-to-leaf nesting chain that reuses an effective `as` name (`config.ErrForEachAsCollision`, naming
  both stages). Siblings on different chains may reuse a name. The check lives at the config layer, the
  altitude where ADR-0040 placed nesting-aware validation.
- **No static nesting-depth cap.** Nesting depth is author-controlled static config, and `ctx.Err()` is
  already checked per element at every level, so runaway iteration is cancellable. Multiplicative fan-out is
  documented, not capped.

## Consequences

- **Two breaking pre-1.0 API changes:** (1) the single-level firing query shape moves from
  `FiringRulesFor("<name>[i]")` (flat aggregate) to `FiringRulesFor("<name>[i].<inner>")` — a superseding
  note to ADR-0049 / Spec 015 D5; (2) `config.ErrNestedForEach` is removed (a never-firing sentinel is
  misleading surface).
- **Nested lineage works with zero new derivation code** — the per-element merge composes to full nested
  paths (`lines[0].taxes[1].vat.amount`), reconciled by B6's exact/ancestor logic.
- **Debuggability preserved:** typed `*StageError` at every level (nested inner errors name each index),
  typed `*ConfigError` wrapping `ErrForEachAsCollision`, no panics on caller input.

## Traceability

Spec: 025. Plan: 025. Supersedes: ADR-0040 D7 (nested deferral). Superseding note: ADR-0049 / Spec 015 D5
(firing-query shape). Related: ADR-0047 (B6 reconciliation, reused for nested lineage), ADR-0006
(deterministic sequential execution, preserved at every level).
