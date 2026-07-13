# ADR-0048 — intra-stage MultiExpr local-alias provenance

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 023 / Plan 023, graduating backlog item **B7** — the "intra-stage `MultiExpr`
  references are not traced" known limitation recorded in ADR-0011 ("Known limitations").

## Context

A `MultiExpr` stage evaluates several named expressions in priority order, each visible to later ones by
its **bare name**: `Execute` writes each result into the eval env under the bare name (`env[e.name] = v`)
so a later expression can reference it, and persists the value to the Scope at the namespaced path
`stage.<name>`. When a later expression reads an earlier one (`taxed = "base * 1.1"`), its `References()`
yields the bare local name (`base`), so provenance recorded `Inputs["base"]`. But no derivation is keyed by
`base` — the value lives at `calc.base` — so `derivationsFor("base")` found nothing (no exact `base`, no
`base.*` descendants, and, after B6, no ancestor either), and `Lineage`/`Explain` silently omitted the
intra-stage subtree. Cross-stage references (a member path like `other.x`, reconciled by B6) and seeds
already traced; only **same-stage local aliases** were orphaned. ADR-0011 recorded the fix direction:
qualify local aliases to their `stage.<name>` path when recording inputs — deferred so it could be decided
deliberately. The user approved it at the B7 design checkpoint (2026-07-13).

## Decision

**Key an intra-stage local-alias reference by its scope path when recording `MultiExpr` inputs, reusing B6's
reconciliation.**

- **Qualify local aliases (Spec 023 D1), `pipe/multi.go` only.** When recording a `MultiExpr` expression's
  `Inputs`, a reference whose **first path segment** matches an *earlier* expression's name in this stage is
  a local alias; its `Inputs` key becomes `stage + "." + ref` instead of the bare `ref`. The **value** is
  still looked up by the bare `ref` (it lives in `env` under the bare name); only the recorded key changes.
  `base` → `calc.base` (B6 exact match); `base.x` (a map-valued local read as a B6 member path) →
  `calc.base.x` (B6 nearest-ancestor fallback to `calc.base`). Seeds and cross-stage paths — first segment
  not a local name — are recorded unchanged.
- **`locals` = names written before the current expression (Spec 023 D2).** Built as `Execute` iterates (a
  name is added **after** its expression's inputs are recorded), mirroring eval visibility and shadowing:
  the first expression named `x` reading a seed `x` is **not** qualified (at that point `env["x"]` is still
  the seed), while a later expression reading `x` after a local `x` was written **is** qualified (the local
  shadows the seed in `env`). Qualification tracks the value actually read.
- **Keyed `snapshotRefs` (Spec 023 D3).** `snapshotRefs` delegates to a new
  `snapshotRefsKeyed(env, refs, keyOf)` with `keyOf == nil`; `MultiExpr.Execute` passes a `keyOf` that
  qualifies local aliases. `single`/`table` (which have no locals) are byte-for-byte unchanged.

## Consequences

- **Intra-stage lineage now traces.** `Lineage("calc.taxed")` / `Explain` include the producing derivation
  `calc.base` and, transitively, its seeds — the known limitation is resolved. The `MultiExpr` cascade is
  fully explainable.
- **`Inputs`-keying contract change, scoped to `MultiExpr` locals.** ADR-0011 documented "`Inputs` is keyed
  by the referenced identifier." A `MultiExpr`'s intra-stage local references are now keyed by their scope
  path (`calc.base`), not the bare local name. This affects only `MultiExpr` `Inputs` for local refs; seed
  and cross-stage keys, `single`/`table` recording, the `Derivation` shape, the `WithProvenance` opt-in, and
  the zero-cost-when-off guarantee are all unchanged. No exported signature change, no `Hash()` or
  config-schema change (provenance recording only).
- **Reuses B6, no new reconciliation.** The fix is purely at the recording site: qualify the key, and B6's
  existing `derivationsFor` (exact → descendants → ancestor) does the linking. No change to
  `derivationsFor`, `Lineage`, or `Explain`.
- **Small surface.** One new unexported recorder (`snapshotRefsKeyed`) + one unexported `MultiExpr` helper
  (`qualifyLocal`); no new dependency (`strings` added to `pipe/multi.go`). Every new branch (local-alias
  qualify, D2 shadowing both directions, member-alias ancestor link) is covered.

## Traceability

Spec: 023 (docs/specs/023-multiexpr-local-alias-provenance.md)
Plan: 023 (docs/plans/023-multiexpr-local-alias-provenance.md)
Backlog: B7 (docs/BACKLOG.md → Resolved)
Resolves: ADR-0011 "Known limitations" (intra-stage MultiExpr alias tracing).
Related: ADR-0047 / Spec 022 (B6 member-path refs + nearest-ancestor reconciliation — the lever this
reuses), ADR-0011 (opt-in provenance & prefix reconciliation).
