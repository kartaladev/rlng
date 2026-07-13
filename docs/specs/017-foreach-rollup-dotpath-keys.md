# Spec 017 — dot-path `foreach` roll-up keys

- **Status:** Draft
- **Backlog item:** B1 (`docs/BACKLOG.md`) — graduates the "dot-path roll-up keys" deferral in ADR-0040.
- **Realized by:** Plan 017; ADR-0042.

## Problem

A `foreach` stage rolls a per-element key up to `<stage>.<As>` via `Rollup.Key`. Today `applyRollup`
(`pipe/foreach.go:211`) looks the key up as a **flat top-level key** in each element's inner-pipeline
`Snapshot()` (a `map[string]any`): `v, ok := m[r.Key]`. A **decision-table** inside the loop writes its
output namespaced as `<table>.<key>`, so rolling that value up currently requires a companion `single-expr`
stage inside the loop just to copy the value to a top-level key. That extra stage is boilerplate the author
must add solely to satisfy the roll-up's flat-lookup limitation.

## Goal

Make `Rollup.Key` **dot-path-aware** so a nested value (e.g. a decision-table output `tier.score`, or any
`<stage>.<field>` an inner stage produces) can be rolled up directly, eliminating the companion stage.

## Decisions

- **D1 — Dot-path resolution.** `Rollup.Key` is split on `.` and resolved by walking `map[string]any`
  levels in each element's Snapshot, identical to how `Scope.Get` (`pipe/scope.go:158`) resolves a path.
  `"tier.score"` reads `element["tier"].(map[string]any)["score"]`.
- **D2 — Backward compatibility (hard requirement).** A **dot-free** key resolves to a single top-level
  lookup — byte-for-byte the current behavior. Every existing roll-up config and test keeps its exact
  semantics; this is purely additive resolution power.
- **D3 — Missing / type-mismatch semantics unchanged.** If any path segment is absent, or an intermediate
  value is not a `map[string]any`, the element is **skipped** (as today when `!ok`). The empty-fold rules
  are unchanged: `Count`→0, `List`→`[]`, `Sum`/`Min`/`Max`→key absent.
- **D4 — Reuse, don't duplicate (altitude).** Extract the nested-path walk shared by `Scope.Get` and
  `applyRollup` into one unexported helper `lookupPath(m map[string]any, path string) (any, bool)`;
  `Scope.Get` delegates to it. One traversal mechanism, no copy-paste.
- **D5 — No schema/struct/hash change.** `Rollup.Key` is an existing `string` field. Dot-awareness is
  resolution-only, so the config surface, the `RollupDef` struct, and `PipelineDef.Hash()` are all
  unchanged — **no ruleset-replay-stability concern** (contrast 015's field additions).
- **D6 — Docs.** Update the `Rollup.Key` godoc and ADR-0040's roll-up contract note to state the key is a
  dot-path; refresh the acceptance example that used a companion `single-expr` if it becomes unnecessary.

## Non-goals

- Array/index path segments (`items[0].x`) — only nested-map traversal, matching `Scope.Get`.
- Changing aggregation kinds or their empty-fold behavior.
- Dot-path awareness anywhere else (`As` output path stays a two-segment `<stage>.<As>` write as today).

## Success criteria / hot-path branches to cover

1. Dot-free key resolves exactly as before (regression guard).
2. Nested dot-path key resolves the inner value.
3. Missing leaf segment → element skipped.
4. Missing / non-map intermediate segment → element skipped.
5. Mixed elements (some supply the path, some don't) fold over only the present ones.
6. Each aggregation kind (`Sum`/`Min`/`Max`/`Count`/`List`) works over a dot-path key.
7. `Scope.Get` behavior is unchanged after delegating to `lookupPath` (existing scope tests stay green).

## Traceability

Backlog: B1. Plan: 017. ADR: 0042 (and updates ADR-0040's roll-up contract note). Related: ADR-0040
(foreach stage), ADR-0025 (integer-preserving aggregation).
