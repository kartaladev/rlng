# Spec 022 — precise member-path references in provenance

- **Status:** Draft
- **Backlog item:** B6 (`docs/BACKLOG.md`) — graduates the "precise member-path references are a future
  refinement" non-goal recorded in ADR-0011 (Consequences) / Spec 006.
- **Design approval:** approved by the user at the B6 design checkpoint (2026-07-13) — redefine
  `References()` to member paths (Option A), reconciliation order exact → descendants → nearest-ancestor,
  deepest-static-path dedup.
- **Realized by:** Plan 022; ADR-0047.

## Problem

Provenance (`WithProvenance`) records, per derived value, the inputs the expression read, as
`Derivation.Inputs`. Those inputs come from `expr.Function/Predicate.References()`, which today returns only
**top-level identifiers** (`grade` for `grade.tier`; `applicant` for `applicant.score`) — ADR-0011 point 4.
Two costs:

1. **Over-broad lineage.** A downstream expression that reads only `grade.tier` records the top-level input
   `grade`. `Lineage`/`Explain` reconcile `grade` via the prefix index to **every** derivation under
   `grade.` (`grade.tier`, `grade.limit`, …), so `Explain` shows outputs the value never read. The trace is
   correct-but-imprecise, which undercuts the debuggability goal.
2. **Coarse input values.** `Inputs["grade"]` is the whole `grade` sub-map, not the `grade.tier` scalar the
   expression actually consumed.

`References()` is consumed **only** by provenance recording (`snapshotRefs` at the four stage sites:
`pipe/single.go`, `pipe/multi.go`, `pipe/table.go` ×2) — it has no functional or validation role — so
refining its granularity has no blast radius beyond provenance output.

## Goal

Record each input at the **deepest statically-known member path** it was read at (`grade.tier`,
`applicant.score`), with its precise value, and reconcile lineage to the exact derivation (or the nearest
recorded ancestor — e.g. the top-level seed that contains it). Bare-identifier references and dynamic/index
access degrade gracefully to the current top-level behavior.

## Decisions

- **D1 — `references()` returns deepest static member paths.** Extraction walks the compiled expr AST
  (`ast.Walk`) and, for each reference, yields the longest statically-resolvable dot-path:
  - bare identifier unchanged: `price * qty` → `["price","qty"]`;
  - static member chain: `grade.tier + base` → `["base","grade.tier"]`; `a.b.c` → `["a.b.c"]` (the
    intermediate `a`, `a.b` are dropped — deepest wins);
  - a `MemberNode` contributes a path only when its object chain bottoms out at an `IdentifierNode` and
    every `Property` along it is a **string** (`.bar` or `["bar"]` — both decode to `ast.StringNode`).
    A non-string property (`a[0]`, `a[i]`) ends the static chain: `a[i].c` → `["a"]` plus the index
    sub-expression's own refs (`["i"]`); `a[0]` → `["a"]`. This mirrors `lookupPath`, which traverses only
    `map[string]any`, so a path it could never resolve is never recorded.
  - **Method calls are not data paths.** A `MemberNode` with `Method == true` (e.g. `foo.bar()`) and a
    `CallNode` callee are excluded from paths, exactly as call callees are excluded today; the receiver
    (`foo`) is still recorded as a bare identifier. Builtins (`len`, …) remain `BuiltinNode`, not
    identifiers.
  - **Maximal-path dedup:** collect every identifier and every `MemberNode` static path, then drop any path
    that is a strict prefix (proper ancestor) of another collected path. Result is sorted + de-duplicated,
    as today.
- **D2 — `References()` contract change (BREAKING semantics, provenance-only consumer).** The exported
  `expr.Function.References()` / `expr.Predicate.References()` godoc changes from "top-level identifiers" to
  "referenced paths — the deepest statically-known member path per reference, or the bare identifier when
  the chain is not statically resolvable." Signature (`[]string`) is unchanged; only the values differ for
  member access. Recorded in ADR-0047.
- **D3 — `snapshotRefs` resolves member paths via `lookupPath`.** For each ref it uses
  `lookupPath(env, ref)` (the shared read-traversal behind `Scope.Get`) instead of a direct `env[ref]`, so
  `Inputs["grade.tier"]` is the `grade.tier` value. A ref that does not resolve (undefined path) is omitted
  from `Inputs`, exactly as today. `Inputs` map keys are now paths.
- **D4 — `derivationsFor` gains a nearest-ancestor fallback.** Reconciliation of an input key becomes, in
  order: (1) **exact** — a derivation recorded at the key (the precise B6 win, e.g. `grade.tier`);
  (2) **descendants** — the existing prefix index (`key.*`), for a bare reference to a namespace a stage
  wrote piecewise; (3) **nearest ancestor** — walk the key up (`a.b.c` → `a.b` → `a`) and return the first
  key with an exact derivation. Step 3 lets a member-path input like `applicant.score` link to the
  top-level **seed** `applicant` (seeds are recorded per top-level key in `NewScope`, holding the whole
  nested value). The three cases are realistically disjoint; the order resolves any pathological overlap
  toward the more-specific derivations.
- **D5 — Precise `Explain`/`Lineage`.** With D1+D3+D4, a value that read `grade.tier` records input
  `grade.tier`, and `Explain` renders only the `grade.tier` subtree — not the whole `grade.*` fan-out.
  Values that read a scalar seed (`price`) or a stage output written at exactly the read path are unchanged
  (exact match, as today).
- **D6 — Migrate in-repo callers/tests.** `expr/refs_test.go` member-access expectations become member
  paths (`tiers.tag + base` → `["base","tiers.tag"]`). Provenance/`Explain`/`Lineage` tests that asserted
  the over-broad top-level fan-out are tightened to the now-precise output. New TDD cases target each new
  branch (D1 member chain, D1 dynamic-index fallback + method-call exclusion, D4 ancestor link to a seed
  member).

## Non-goals

- **B7 (intra-stage `MultiExpr` local-alias provenance)** stays separate — that concerns a later expression
  reading an earlier one by its **bare local name**, a different reconciliation gap (ADR-0011 known
  limitations).
- **Simultaneous read of both an ancestor and its descendant** (`a.b + a.b.c` in one expression) degrades
  to recording only the deepest path (`a.b.c`). No seed is lost from the trace (both resolve to the same
  seed lineage); only the "also read `a.b` directly" granularity is dropped. Documented, accepted.
- No change to what provenance records structurally (`Derivation` shape unchanged), to the `WithProvenance`
  opt-in, to the zero-cost-when-off guarantee, or to any non-provenance behavior. No `Hash()` or
  config-schema change (this touches `expr`/`pipe` provenance only, not the parsed def).

## Success criteria / hot-path branches to cover

1. `References()` on a static member chain returns the deepest path (`grade.tier`), and on a bare
   identifier returns the identifier (`price`) — deepest-path dedup drops intermediates.
2. `References()` on dynamic/index access returns the static prefix + the index's refs (`a[i].c` →
   `["a","i"]`), and excludes a method-call member (`foo.bar()` → `["foo"]`).
3. `snapshotRefs` records the precise value at a member path (`Inputs["grade.tier"]` is the scalar), and
   omits an unresolvable path.
4. `Explain`/`Lineage` of a value that read `grade.tier` links to **only** the `grade.tier` derivation, not
   the sibling `grade.limit` (the over-broad fan-out is gone).
5. A member-path input whose value comes from a seed (`applicant.score`) reconciles to the top-level seed
   derivation `applicant` (nearest-ancestor fallback).
6. Bare-identifier and exact-path references behave exactly as before (regression): a scalar seed `price`,
   a whole-namespace read `grade` (descendants), a single-expr output read at its own path.

## Traceability

Backlog: B6. Plan: 022. ADR: 0047 (records the `References()` member-path contract change + the
nearest-ancestor reconciliation; refines ADR-0011 point 4 & Consequences). Related: ADR-0011 (opt-in
provenance & prefix reconciliation — refined here), ADR-0042 (`lookupPath` shared read-traversal, reused by
`snapshotRefs`).
