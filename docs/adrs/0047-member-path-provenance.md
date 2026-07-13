# ADR-0047 — precise member-path references in provenance

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 022 / Plan 022, graduating backlog item **B6** — the "precise member-path
  references are a future refinement" non-goal recorded in ADR-0011 (point 4 & Consequences) / Spec 006.

## Context

Provenance (`WithProvenance`) records, per derived value, the inputs the expression read, as
`Derivation.Inputs`, sourced from `expr.Function/Predicate.References()`. ADR-0011 point 4 chose
**top-level identifiers** (`grade` for `grade.tier`) to keep AST handling simple, and reconciled them to
namespaced derivations via a prefix index (an input `grade` links to every `grade.*` a stage wrote). Two
costs to debuggability: a downstream expression that reads only `grade.tier` records the coarse input
`grade`, so `Explain`/`Lineage` fan out to sibling outputs (`grade.limit`) the value never read; and
`Inputs["grade"]` is the whole sub-map, not the scalar consumed. `References()` is consumed **only** by
provenance recording (`snapshotRefs` at the four stage sites) — it has no functional or validation role —
so refining its granularity has no blast radius beyond provenance output. The user approved refining it
(B6 design checkpoint, 2026-07-13).

## Decision

**Record each input at its deepest statically-known member path, and reconcile lineage to the exact
derivation or the nearest recorded ancestor.**

- **`References()` returns deepest static member paths (Spec 022 D1/D2).** `expr.references()` walks the
  compiled AST and emits, per reference, the longest fully-static dot-path: a bare identifier unchanged
  (`price`), a static member chain collapsed to its deepest path (`grade.tier`, `a.b.c` — intermediates
  dropped), `a["b"]` treated the same as `a.b` (both decode to `ast.StringNode`). A non-string property
  (`a[i]`, `a[0]`) ends the static chain and yields the base identifier plus the index's own refs; a
  method-call member (`foo.bar()`, `MemberNode.Method`) and call callees are excluded (only the receiver /
  data identifiers survive). The exported `References()` **signature is unchanged** (`[]string`); only its
  documented semantics change — recorded here as a deliberate (provenance-only) contract change, no SemVer
  signature break.
- **`snapshotRefs` resolves paths via `lookupPath` (Spec 022 D3).** Each ref is resolved through the shared
  read-traversal behind `Scope.Get` (ADR-0042's `lookupPath`), so `Inputs["grade.tier"]` is the precise
  scalar; an unresolvable path is omitted, exactly as before. A single-segment ref stays a direct map
  lookup (`lookupPath` fast path), so top-level references are byte-for-byte unchanged.
- **`derivationsFor` gains a nearest-ancestor fallback (Spec 022 D4).** Reconciliation is now, in order of
  precision: **exact** (a derivation at the key — the precise win for `grade.tier`); else **descendants**
  (the existing prefix index `key.*` — a bare namespace reference); else **nearest ancestor** (walk
  `a.b.c` → `a.b` → `a`). The ancestor step links a member-path input like `applicant.score` to the
  top-level **seed** `applicant`, which `NewScope` records per top-level key holding the whole nested value.
  The three cases are realistically disjoint; the order resolves any pathological overlap toward the
  more-specific derivations.

## Consequences

- **Precise lineage.** `Explain`/`Lineage` of a value that read `grade.tier` link to only the `grade.tier`
  subtree — the over-broad `grade.*` fan-out is gone — and `Inputs` carry the exact values read. Directly
  advances the debuggability-first criterion.
- **`References()` semantics change, no signature/behaviour break elsewhere.** Values differ only for member
  access; bare identifiers and call-callee exclusion are unchanged. The only consumer is provenance, so no
  functional path is affected. No `Hash()` or config-schema change (this touches `expr`/`pipe` provenance,
  not the parsed def), and the `WithProvenance` opt-in and the zero-cost-when-off guarantee are untouched.
- **Behavior-preserving prep isolated.** The `pipe`-side path-awareness (`lookupPath` resolution +
  ancestor fallback) landed as its own green commit ahead of the `expr` change: for today's top-level refs
  it is a no-op (single-segment lookup, ancestor branch unreached), and the ancestor branch is covered via
  the public `Derive`+`Explain` surface.
- **Documented degradation.** An expression that reads both an ancestor and its descendant in one place
  (`a.b + a.b.c`) records only the deepest path (`a.b.c`); no seed is lost from the trace (both resolve to
  the same seed lineage), only the "also read `a.b` directly" granularity. Accepted as a simplification of
  the deepest-path dedup, which avoids a fragile parent-aware AST walk across every expr node type.
- **B7 still separate.** Intra-stage `MultiExpr` **local-alias** provenance (a later expr reading an earlier
  one by bare local name) is a different reconciliation gap (ADR-0011 known limitations) and remains open.
- **Small surface, no new dependency.** `expr/refs.go` rewrite (`staticPath`, `isStrictPrefixOfAny`) reuses
  the already-imported `expr-lang/expr/ast` + `/parser`; two `pipe/provenance.go` helpers widened.

## Traceability

Spec: 022 (docs/specs/022-member-path-provenance.md)
Plan: 022 (docs/plans/022-member-path-provenance.md)
Backlog: B6 (docs/BACKLOG.md → Resolved)
Refines: ADR-0011 point 4 & Consequences (top-level-identifier granularity + prefix reconciliation).
Related: ADR-0042 (`lookupPath` shared read-traversal, reused by `snapshotRefs`).
