# ADR-0049 — per-element foreach lineage

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 024 / Plan 024, graduating backlog item **B8** — the "per-element lineage beyond
  firing is discarded" deferral recorded in ADR-0040 (Known limitations, D5) / Spec 015 D5.

## Context

A `ForEach` stage runs an inner pipeline once per element against a fresh per-element `Scope` (`esc`). When
the outer scope tracks provenance, `esc` is created `WithProvenance` (`pipe/foreach.go`), so the inner
pipeline builds `esc`'s **full per-element derivation graph**. But `Execute` kept only `esc.Snapshot()` (the
element's data) in the `items` list — `esc.derivations` was discarded when `esc` went out of scope.
Per-element **firing** was already surfaced on the outer scope under `<name>[i]` (queryable via
`FiringRulesFor`), so "line *i* denied by rule X" was answerable, but "how was element *i*'s output
derived, back to its seeds?" was not — the graph that would answer it was thrown away. ADR-0040 D5 recorded
this as deferred. The user approved surfacing it at the B8 design checkpoint (2026-07-13), **always-on when
the outer scope tracks provenance** (no new option).

## Decision

**Merge each element's derivation graph onto the outer scope under a `<name>[i].` path prefix, reusing B6's
reconciliation.**

- **Prefix-merge the per-element derivations (Spec 024 D1).** After an element's inner pipeline runs, when
  the outer scope tracks provenance, every derivation in `esc` is copied into the outer scope's derivation
  map with its `Path` and each of its `Inputs` **keys** rewritten `<name>[i].` + original
  (`check.score` → `lines[0].check.score`; input `item.ltv` → `lines[0].item.ltv`). The uniform prefix keeps
  the element's subgraph internally consistent, so `derivationsFor` (exact → descendants → ancestor, from
  B6) links `lines[0].check.score` → `lines[0].item.ltv` → the element seed `lines[0].item`. The prefix
  matches the existing per-element firing key exactly.
- **Full graph, always-on when provenance is on (Spec 024 D2).** The per-element scope is seeded from the
  outer `Snapshot()` plus the element, so its derivations include the element (`item`) and the outer values
  as seeds; all are merged (prefixed) so `Explain` renders each element leaf as `[seed]`. Recording is
  automatic whenever the outer scope tracks provenance — no new option, consistent with per-element firing
  (already always recorded) and the debuggability-first ethos. The merged size is N × (per-element graph),
  bounded by the collection the caller chose to trace and paid only under provenance — consistent with the
  accepted per-element isolation cost (ADR-0043 / B2).
- **New unexported `Scope` merge helper (Spec 024 D3).** `(*Scope).recordElementDerivations(prefix, src)`:
  a no-op when the scope does not track provenance or `src` is empty; otherwise it inserts a prefix-rewritten
  copy of each `src` derivation under the scope's write lock. `ForEach.Execute` calls it per element right
  after firing recording, passing `esc.Derivations()` (the already-locked copy). No change to `Lineage`,
  `Explain`, `derivationsFor`, or the `Derivation` shape.

## Consequences

- **Per-element lineage is answerable.** `sc.Explain("lines[3].check.score")` /
  `sc.Lineage("lines[3].check.score")` / `sc.Derivations()` now trace an element's output through its inputs
  to its seed, alongside the per-element firing — closing ADR-0040 D5. Observed rendering:
  `lines[0].check.score = 180 [check decision-table] expr: item.ltv * 2` → `lines[0].item = map[ltv:90]
  [seed]`.
- **No data / signature / eval change.** Merged derivations live in `sc.derivations`, separate from
  `sc.data` (the `items`/rollup outputs) and `sc.firing`; the bracketed `<name>[i].…` keys never collide
  with the dot-only `<name>.items` / `<name>.<As>` data paths, nor with firing's `<name>[i]` key (a
  different map). The `items` list and rollups are unchanged (still `Set`, no derivation). No exported
  signature, `Hash()`, config-schema, or evaluation change; zero cost when provenance is off (`esc` builds
  no graph and the merge is a no-op).
- **Bounded memory, debug-only.** Full per-element graphs (including re-seeded outer values) are kept — not
  trimmed to element-reachable seeds — for correct `Explain` rendering. This is O(N × graph) additional
  derivations, only under provenance; a size optimization (trim to reachable) is a possible future
  refinement, out of scope for B8.
- **Header-output lineage still out of scope.** The foreach's own `<name>.items` / rollup `<name>.<As>`
  outputs are still `Set` without a derivation; surfacing header-output lineage is a separate concern.
- **Small surface.** One unexported `Scope` helper + one `ForEach.Execute` call; no new dependency. The
  merge branch and the provenance-off no-op are both covered.

## Traceability

Spec: 024 (docs/specs/024-foreach-per-element-lineage.md)
Plan: 024 (docs/plans/024-foreach-per-element-lineage.md)
Backlog: B8 (docs/BACKLOG.md → Resolved)
Resolves: ADR-0040 D5 / Spec 015 D5 (per-element lineage beyond firing).
Related: ADR-0047 / Spec 022 (B6 member-path refs + nearest-ancestor reconciliation, reused for the element
subgraph), ADR-0011 (opt-in provenance, `Lineage`/`Explain`), ADR-0043 / B2 (accepted per-element isolation
cost).
