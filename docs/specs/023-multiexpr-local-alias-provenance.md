# Spec 023 — intra-stage MultiExpr local-alias provenance

- **Status:** Draft
- **Backlog item:** B7 (`docs/BACKLOG.md`) — graduates the "intra-stage `MultiExpr` references are not
  traced" known limitation recorded in ADR-0011 ("Known limitations").
- **Design approval:** approved by the user at the B7 design checkpoint (2026-07-13) — qualify local-alias
  refs to their `stage.<ref>` scope path when recording `MultiExpr` inputs (first-segment granularity),
  leveraging B6's reconciliation.
- **Realized by:** Plan 023; ADR-0048.

## Problem

A `MultiExpr` stage evaluates several named expressions in priority order, each visible to later ones by
its **bare name**. `Execute` writes each result into the eval env under that bare name
(`env[e.name] = v`, `pipe/multi.go`) so a later expression can reference it, and persists the value to the
Scope at the namespaced path `stage.<name>`. When a later expression reads an earlier one
(`taxed = "base * 1.1"`), its `References()` yields the bare local name (`base`), so
`snapshotRefs` records `Inputs["base"]`. But no derivation is keyed by `base` — the value lives at
`calc.base` — so `derivationsFor("base")` finds nothing (no exact `base`, no `base.*` descendants, no
ancestor), and `Lineage`/`Explain` silently omit the intra-stage subtree. Cross-stage references (a member
path like `other.x`, reconciled via B6) and seeds already trace correctly; only **same-stage local
aliases** are orphaned. (Confirmed by `pipe/multi_test.go`, which asserts `taxed.Inputs ==
map[string]any{"base": 20.0}` — the bare-name key.)

## Goal

Make an intra-stage local reference reconcile to the earlier expression that produced it, so `Lineage` /
`Explain` trace `calc.taxed` → `calc.base` → its seeds, by keying such an input under its **scope path**
(`calc.base`) instead of the bare local name — reusing B6's exact/ancestor reconciliation. Seed and
cross-stage references are unchanged.

## Decisions

- **D1 — Qualify local-alias references to their scope path (`pipe/multi.go` only).** When recording a
  `MultiExpr` expression's `Inputs`, a reference whose **first path segment** matches an *earlier*
  expression's name in this stage is a local alias; its `Inputs` key becomes `stage + "." + ref` instead of
  the bare `ref`. The **value** is still looked up by the bare `ref` (it lives in `env` under the bare
  name); only the recorded key changes.
  - `base` → `calc.base` — `derivationsFor("calc.base")` exact-matches the `base` derivation.
  - `base.x` (an earlier expr `base` that produced a map, read as a B6 member path) → `calc.base.x` —
    B6's nearest-ancestor fallback links it up to `calc.base`.
  - Seeds (`price`) and cross-stage paths (`grade.tier`) — first segment is not a local name — are recorded
    unchanged.
- **D2 — `locals` is the set of expression names written before the current one.** Built as `Execute`
  iterates (a name is added to the set **after** its expression's inputs are recorded), which mirrors eval
  visibility and shadowing exactly: the first expression named `x` that reads a seed `x` is **not**
  qualified (at that point `env["x"]` is still the seed — the local has not been written yet), while a later
  expression reading `x` after a local `x` was written **is** qualified (the local shadows the seed in
  `env`, so it read the local). Qualification therefore tracks the value actually read.
- **D3 — Implementation via a keyed `snapshotRefs`.** Generalize the recorder into
  `snapshotRefsKeyed(env, refs, keyOf func(string) string)`; `snapshotRefs` delegates with `keyOf == nil`
  (unchanged for `single`/`table`, which have no locals). `MultiExpr.Execute` passes a `keyOf` that
  qualifies local aliases per D1/D2. No change to `single.go`, `table.go`, or `snapshotRefs`' existing
  callers' behavior.
- **D4 — Contract change (ADR-0048).** ADR-0011 documented "`Inputs` is keyed by the referenced
  identifier." After B7, a `MultiExpr`'s **intra-stage local** references are keyed by their **scope path**
  (`calc.base`), not the bare local name. This affects only `MultiExpr` `Inputs` for local refs; seed and
  cross-stage keys are unchanged, and `single`/`table` are unaffected. No `Hash()` or config-schema change
  (provenance recording only).
- **D5 — Migrate/prove.** Update `pipe/multi_test.go`'s provenance case (`taxed.Inputs` →
  `map[string]any{"calc.base": 20.0}`) and add a `Lineage`/`Explain` test proving `calc.taxed` now traces
  through `calc.base` to the `price`/`qty` seeds. Cover the D2 shadowing branches (first `x` reads seed →
  unqualified; later `x` reads local → qualified).

## Non-goals

- **Single-expr / decision-table stages** have no intra-stage locals — unchanged.
- **B8 (per-element `foreach` lineage)** and **B6 (member-path granularity, done)** are separate; B7 only
  fixes the bare-local-alias reconciliation gap. A cross-stage reference is already handled by B6.
- No change to `MultiExpr` **evaluation** semantics (bare-name visibility is preserved — the feature), to
  the `Derivation` shape, to the `WithProvenance` opt-in, or to the zero-cost-when-off guarantee.

## Success criteria / hot-path branches to cover

1. A later expression reading an earlier one by bare name (`taxed = "base * 1.1"`) records
   `Inputs["calc.base"]` (not `Inputs["base"]`), and `Lineage("calc.taxed")` / `Explain` include the
   `calc.base` derivation and, transitively, its `price`/`qty` seeds.
2. A local-alias **member** read (`t = "base.k"` with an earlier map-valued `base`) records
   `Inputs["calc.base.k"]` and reconciles to `calc.base` via the ancestor fallback.
3. D2 shadowing: an expression named `x` reading seed `x` (no earlier local `x`) records the **seed** key
   `x` (unqualified); a later expression reading `x` after the local `x` records `calc.x` (qualified).
4. A seed reference (`base = "price * qty"`) and a cross-stage reference are recorded unchanged (regression).
5. `single`/`table` provenance inputs are unchanged (`snapshotRefs` default path).

## Traceability

Backlog: B7. Plan: 023. ADR: 0048 (records the `MultiExpr` local-alias qualification + the `Inputs`-keying
contract change; resolves ADR-0011's known limitation). Related: ADR-0011 (opt-in provenance & the known
limitation being fixed), ADR-0047 (B6 member-path refs + nearest-ancestor reconciliation, the lever this
reuses).
