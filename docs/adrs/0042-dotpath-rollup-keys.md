# ADR-0042 — Dot-path `foreach` roll-up keys

- **Status:** Accepted
- **Date:** 2026-07-13
- **Spec:** 017 (`docs/specs/017-foreach-rollup-dotpath-keys.md`)
- **Plan:** 017 (`docs/plans/017-foreach-rollup-dotpath-keys.md`)
- **Backlog:** B1 (`docs/BACKLOG.md`)
- **Updates:** ADR-0040 (lifts the flat-key roll-up limitation it recorded as deferred).

## Context

The `foreach` stage (ADR-0040) rolls a per-element output up to `<stage>.<As>` via `Rollup.Key`.
`applyRollup` resolved `Key` as a **flat top-level key** in each element's inner-pipeline `Snapshot()`
(`m[r.Key]`). But an inner **decision-table** writes its output namespaced as `<table>.<key>`, so rolling
that value up required authoring a companion `single-expr` stage inside the loop whose only job was to copy
the value to a top-level key. ADR-0040 recorded this as a deferred backlog item (B1).

## Decision

Resolve `Rollup.Key` as a **dot-separated path** into each element's result map, walking `map[string]any`
levels — the same traversal `Scope.Get` performs. Concretely:

- Extract the nested-path walk shared by `Scope.Get` into one unexported helper
  `lookupPath(m map[string]any, path string) (any, bool)` (the read counterpart of the existing
  `setPath`); `Scope.Get` delegates to it, and `applyRollup` calls it in place of `m[r.Key]`.
- A **dot-free** key resolves to a single top-level lookup — byte-for-byte the prior behavior.
- A **nested** key (`"grade.score"`) reads the value an inner stage namespaced under its own name.
- An element whose path segment is missing, or whose intermediate node is not a `map[string]any`, is
  **skipped** — identical to the prior missing-key semantics; the empty-fold rules (Count→0, List→[],
  Sum/Min/Max→absent) are unchanged.

`Rollup.Key` remains an existing `string` field: dot-awareness is resolution-only, so the config surface,
the `RollupDef` struct, and `PipelineDef.Hash()` are all unchanged.

## Consequences

- **No companion stage.** A decision-table (or any nested) output can be rolled up directly.
- **Fully backward-compatible.** Every existing roll-up config/test keeps its exact semantics; not a
  breaking change (no exported-symbol change, no SemVer bump).
- **No replay-stability risk.** Because `Hash()` is unaffected (no schema/struct change), pre-017 rulesets
  keep their fingerprint — unlike 015's field additions, this needed no `omitempty` guard.
- **DRY/altitude.** One nested-path read-traversal (`lookupPath`) is now shared by `Scope.Get` and roll-up
  resolution instead of two inlined copies.
- **Scope limited to nested maps.** Array/index path segments (`items[0].x`) remain unsupported, matching
  `Scope.Get`; a non-goal (Spec 017).

## Traceability

Backlog: B1. Spec: 017. Plan: 017. Updates: ADR-0040. Related: ADR-0025 (integer-preserving aggregation,
which the roll-up reuses).
