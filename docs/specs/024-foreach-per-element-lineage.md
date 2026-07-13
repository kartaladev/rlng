# Spec 024 — per-element foreach lineage

- **Status:** Draft
- **Backlog item:** B8 (`docs/BACKLOG.md`) — graduates the "per-element lineage beyond firing is discarded"
  deferral recorded in ADR-0040 (Known limitations, D5) / Spec 015 D5.
- **Design approval:** approved by the user at the B8 design checkpoint (2026-07-13) — merge each element's
  derivation graph onto the outer scope under a `<name>[i].` path prefix, **always-on when the outer scope
  tracks provenance** (no new option).
- **Realized by:** Plan 024; ADR-0049.

## Problem

A `ForEach` stage runs an inner pipeline once per element against a fresh per-element `Scope` (`esc`). When
the outer scope tracks provenance, `esc` is created `WithProvenance` (`pipe/foreach.go`), so the inner
pipeline builds `esc`'s **full per-element derivation graph**. But `Execute` keeps only `esc.Snapshot()`
(the element's data) in the `items` list — `esc.derivations` is discarded when `esc` goes out of scope.
Per-element **firing** is already surfaced on the outer scope under `<name>[i]` (queryable via
`FiringRulesFor`), so "line *i* denied by rule X" is answerable; but the deeper question "how was element
*i*'s `decision.tier` derived, back to its seeds?" is not — the lineage graph that would answer it is
thrown away (ADR-0040 D5).

## Goal

Surface each element's derivation graph on the outer scope so `Explain`/`Lineage`/`Derivations` answer
per-element lineage, by copying the per-element derivations onto the outer scope under a `<name>[i].` path
prefix (mirroring per-element firing), reusing B6's exact/ancestor reconciliation. Zero cost when provenance
is off.

## Decisions

- **D1 — Merge each element's derivation graph, path-prefixed (`<name>[i]`).** After an element's inner
  pipeline runs, when the outer scope tracks provenance, every derivation in the per-element scope is copied
  into the outer scope's derivation map with:
  - the derivation `Path` rewritten `<name>[i].` + original (`decision.tier` → `orders[3].decision.tier`);
  - each `Inputs` **key** rewritten with the same prefix (`item.amt` → `orders[3].item.amt`).
  The uniform prefix keeps the element's subgraph internally consistent, so `derivationsFor` (exact →
  descendants → ancestor, from B6) links `orders[3].decision.tier` → `orders[3].item.amt` → its seed within
  the element. The prefix `<name>[i]` matches the existing per-element firing key exactly.
- **D2 — Full graph, including the re-seeded outer values.** The per-element scope is seeded from the outer
  `Snapshot()` plus the element bound under `as`, so `esc.derivations` records the element (`item`) and the
  outer values as **seeds**. All of them are merged (prefixed), so `Explain` can render each element leaf as
  `[seed]`. The merged size is N × (per-element graph); it is bounded by the collection the caller chose to
  trace and is only paid when provenance is on — consistent with the accepted per-element isolation cost
  (ADR-0043 / B2). No trimming to "referenced-only" seeds (kept simple and complete).
- **D3 — Implementation via a new unexported `Scope` merge helper.** Add
  `(*Scope).recordElementDerivations(prefix string, src map[string]Derivation)`: a no-op when the scope does
  not track provenance or `src` is empty; otherwise, under the scope's write lock, it inserts a
  prefix-rewritten copy of each `src` derivation (path + input keys). `ForEach.Execute` calls it per element
  immediately after the existing firing recording, passing `fmt.Sprintf("%s[%d]", f.name, i)` and
  `esc.Derivations()` (the already-locked copy). No change to `Lineage`, `Explain`, `derivationsFor`, or the
  `Derivation` shape.
- **D4 — No conflicts, no data change, zero-cost-off.** Merged derivations live in `sc.derivations`,
  separate from `sc.data` (the `items`/rollup outputs) and `sc.firing` (the per-element firing). The
  bracketed `<name>[i].…` keys never collide with the dot-only data paths `<name>.items` / `<name>.<As>`,
  nor with firing's own `<name>[i]` key (a different map). The `items` list and rollups are unchanged (still
  written via `Set`, no derivation). When provenance is off, `esc` builds no graph and the merge is a no-op.

## Non-goals

- **A derivation for the foreach's own header outputs** (`<name>.items`, rollup `<name>.<As>`) — those are
  still `Set` without a derivation; surfacing header-output lineage is a separate concern from per-element
  lineage (D5) and out of scope for B8.
- **Trimming the merged graph** to only element-reachable seeds — kept complete for correct `Explain`
  rendering; a size optimization is a possible future refinement, not B8.
- **Parallel per-element evaluation** (still sequential, deterministic — separate deferral).
- No `Hash()`, config-schema, or evaluation-semantics change; no change to `WithProvenance` or the
  zero-cost-when-off guarantee.

## Success criteria / hot-path branches to cover

1. With outer provenance on, after a foreach whose inner pipeline has a decision table, `sc.Explain("<name>
   [i].<table>.<key>")` renders the element's output, its inputs (prefixed, e.g. `<name>[i].item.<field>`),
   and traces to the element seed; `sc.Lineage(...)` returns the element's derivations seeds-first.
2. Two elements are independent: `<name>[0].…` and `<name>[1].…` derivations coexist and reconcile within
   their own prefix (no cross-element bleed).
3. Per-element firing under `<name>[i]` is still recorded (regression) and now co-exists with the merged
   lineage.
4. With provenance **off**, no per-element derivations are recorded (`sc.Derivations()` has none for the
   foreach), and firing is unaffected (regression) — the merge is a no-op.
5. An input that is a member path within the element (`item.amt`) reconciles to the element seed via B6's
   ancestor fallback under the prefix (`<name>[i].item`).

## Traceability

Backlog: B8. Plan: 024. ADR: 0049 (records surfacing per-element lineage via prefixed derivation merge;
resolves ADR-0040 D5 / Spec 015 D5). Related: ADR-0040 (foreach stage & the D5 deferral being closed),
ADR-0047 (B6 member-path refs + nearest-ancestor reconciliation, reused for the element subgraph),
ADR-0011 (opt-in provenance, `Lineage`/`Explain`).
