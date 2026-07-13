# Spec 025 — nested foreach support

- **Status:** Draft
- **Backlog item:** B9 (`docs/BACKLOG.md`) — graduates the "nested `foreach` deferred" non-goal recorded in
  ADR-0040 (D7 deferral) / Spec 015 D7, enforced today by `config.ErrNestedForEach` (`config/build.go`).
- **Design approval:** approved by the user at the B9 design checkpoint (2026-07-13) — **full nested
  provenance**: lift the config-layer gate and make firing/lineage correct under nesting. Four decisions
  settled at the checkpoint: (1) firing keys compose by **hierarchical re-prefix** (mirroring derivations);
  (2) reject duplicate `as` along a nesting chain at build time; (3) **no** static nesting-depth cap; (4)
  remove the now-dead `ErrNestedForEach` sentinel.
- **Realized by:** Plan 025; ADR-0050.

## Problem

Business rules nest collections: line items each carrying tax lines, loans each carrying collateral each
carrying valuations. A `ForEach` stage runs an inner `*Pipeline` once per element against a fresh
per-element `Scope` (ADR-0040), and nothing in the `pipe` layer forbids that inner pipeline from containing
another `ForEach` — nested `foreach` **already runs** programmatically. But the **config layer rejects it**
at build time (`ErrNestedForEach`, `config/build.go`), the D7 deferral made concrete, so it is unreachable
from declarative YAML/JSON. Two things also block "runs" from meaning "works well":

1. **`as` collision.** Outer and inner `foreach` both default to binding the element under `"item"`, so an
   inner `foreach` silently shadows the outer element in its per-element scope — a debuggability footgun.
2. **Firing loses the nesting index.** The outer records element *i*'s inner firings via `esc.FiringRules()`,
   which **flattens** the per-element scope's firing map into one list under the single key `<name>[i]`,
   discarding the inner element index. So "line *i*, tax line *j* charged by rule VAT_RED" is unanswerable.
   Derivations, by contrast, already compose: `ForEach.Execute` calls `recordElementDerivations` per element,
   and because a nested `ForEach` runs *inside* the per-element scope, its derivations are already keyed
   `<inner>[j].…`; the outer re-prefix yields the full nested path `<name>[i].<inner>[j].…` with no new code.

## Goal

Make nested `foreach` a first-class, declarative feature with correct per-element provenance at any depth:
lift the config gate; fix firing to compose hierarchically like derivations already do; reject `as`
collisions along a nesting chain at build time. Preserve the `pipe` layer's nesting-agnostic altitude
(ADR-0040) — the fix is uniform for single-level and nested, not nesting-detection logic inside `ForEach`.

## Decisions

- **D1 — Firing composes by hierarchical re-prefix (the one `pipe`-layer change).** Replace `Execute`'s
  firing **flatten** (`esc.FiringRules()` → one `<name>[i]` key) with a firing **map re-prefix** mirroring
  `recordElementDerivations`. Add two unexported `Scope` methods:
  - `firingMap() map[string][]FiringRule` — an `RLock`'d copy of the raw firing map.
  - `recordElementFirings(prefix string, src map[string][]FiringRule)` — under the write lock, for each key
    `k` in `src`, sets `s.firing[prefix+"."+k] = <copy>`; a no-op when `src` is empty.

  `Execute` calls `sc.recordElementFirings(fmt.Sprintf("%s[%d]", f.name, i), esc.firingMap())` in place of
  the old `recordFirings(...)`. Firing keys always end at the decision-table stage that fired:
  `lines[0].ltv` (a decision table directly in the inner pipeline) and, for a nested `foreach` `taxes`
  whose inner pipeline holds a table `vat`, `lines[0].taxes[1].vat` — the innermost stage name is the leaf,
  exactly as a single-level key is `lines[0].ltv`. This composes to any depth because a nested `ForEach`
  already produced the `taxes[1].vat` key in the per-element scope; the outer re-prefix prepends
  `lines[0].`. It works identically for single-level and nested; **no nesting-detection logic enters
  `ForEach`.** `FiringRule.Stage` stays the bare decision-table name (its meaning is unchanged); only the
  map **key** carries the element path, exactly as the derivation `Path` does.

  *Breaking (pre-1.0):* the single-level firing query changes from `FiringRulesFor("<name>[i]")` (flat
  aggregate) to `FiringRulesFor("<name>[i].<inner stage>")`. Recorded in ADR-0050 as a superseding note to
  Spec 015 D5 / ADR-0049.

- **D2 — Lineage needs no new code; it already composes.** Because `ForEach.Execute` calls
  `recordElementDerivations` per element and a nested `ForEach` runs inside the per-element scope, the inner
  derivations are already keyed `<inner>[j].…` (with prefixed `Inputs`) in the outer element's scope; the
  outer re-prefix yields `<name>[i].<inner>[j].<path>`, and B6's exact→descendants→ancestor reconciliation
  (`derivationsFor`) links each nested output back to the element seed within its own prefix. The spec
  asserts this composition with tests; it introduces no derivation-layer change.

- **D3 — Lift the config gate; nesting builds by existing recursion.** Delete the `ErrNestedForEach`
  rejection in `buildForEach` (`config/build.go`). `buildForEach` already builds each inner `StageDef` via
  `isd.build`, which dispatches a `type: foreach` inner stage straight back into `buildForEach` — so nested
  pipelines assemble with **no new wiring**. Inner stages already inherit the same constants/schema/strict
  env recursively.

- **D4 — Reject duplicate `as` along a nesting chain at build time.** Add
  `validateForEachAsChains(stages []StageDef, chain []string) error`, called once from `Build` over the
  top-level stages. It walks the `foreach` nesting tree; for each `foreach` it computes the **effective**
  `as` (`sd.As` or the default `"item"`); if that name is already in the current root-to-leaf `chain`, it
  returns a `*ConfigError` wrapping the new sentinel `ErrForEachAsCollision`, naming both the ancestor and
  the colliding inner stage; otherwise it recurses into `sd.Stages` with `chain + effectiveAs`. **Siblings
  on different chains may reuse a name** (they run in independent scopes). The check lives at the config
  layer (build time), the same altitude where `ErrNestedForEach` lived and where ADR-0040 places
  nesting-aware validation; the `pipe` layer stays nesting-agnostic, and programmatic callers remain
  responsible for distinct names (documented). `ErrForEachAsCollision` is a new **exported** `config`
  sentinel (the debuggability contract).

- **D5 — No static nesting-depth cap.** Nesting depth is a static, author-controlled config property (not
  untrusted runtime data), and `ctx.Err()` is already checked before the loop and before each element at
  every level, so runaway iteration is cancellable at element boundaries. Document the multiplicative
  fan-out (`O(∏ collection sizes)`) on the config surface; add **no** arbitrary `MaxForEachNestingDepth`
  ceiling. (Contrast `MaxLineageDepth`, which guards graphs restored from *untrusted* JSON — a different
  threat.)

- **D6 — Remove the dead `ErrNestedForEach` sentinel.** With nesting supported the error can never be
  returned again; a never-firing exported sentinel is misleading surface. Remove `config.ErrNestedForEach`
  (a breaking pre-1.0 API removal, ADR-0050) and update the `config/def.go` doc comment that referenced it.
  *(Alternative considered: keep it `// Deprecated:` and unreferenced — rejected as retaining misleading
  dead surface pre-1.0.)*

## Non-goals

- **Parallel per-element / nested evaluation** — iteration stays sequential and deterministic at every level
  (ADR-0006); parallelism is a separate deferral (B11).
- **A static nesting-depth limit** (D5) — deliberately omitted.
- **New rollup semantics for nesting** — an outer `Rollup.Key` is a dot-path resolved in each element's
  result map, which already contains a nested `foreach`'s outputs (`<inner>.items`, `<inner>.<As>`), so an
  outer rollup over a nested value works via the existing `lookupPath`; the spec adds a test, not code.
- **`Hash()` / config-schema change** — lifting the gate does **not** alter the parsed `PipelineDef` shape
  (the `foreach` fields already exist and are `omitempty`), so pre-025 rulesets hash byte-identically; a
  pinned golden-hash test guards this.
- **Changing `FiringRule.Stage`'s meaning** — it stays the bare inner decision-table name; nesting lives in
  the map key only.

## Success criteria / hot-path branches to cover

1. **Nested firing index preserved.** A two-level `foreach` (outer `lines` over line items, inner `taxes`
   over each line's tax lines, innermost a decision table `vat`) records
   `FiringRulesFor("lines[0].taxes[1].vat")` returning the inner element's fired rule (the leaf is the
   decision-table stage name, the general form `<name>[i].<inner>[j].<dt>`); a decision table `ltv` directly
   in the outer inner pipeline records `FiringRulesFor("lines[0].ltv")`.
2. **`recordElementFirings` branches.** Non-empty inner map re-prefixes every key (single-level key now
   `<name>[i].<inner>`); an empty inner firing map is a no-op (no `<name>[i]` key created).
3. **Nested lineage composes.** With outer provenance on, `Explain`/`Lineage` for
   `lines[0].taxes[1].<dt>.<key>` traces through the nested prefixes to the element seed
   (`lines[0].taxes[1].item.<field>`); no cross-element/cross-branch bleed.
4. **Nested output shape.** `lines.items[i]` contains `taxes.items[j]` (the inner list nests naturally in
   the per-element snapshot).
5. **Outer rollup over a nested value** resolves through `lookupPath` (e.g. an outer rollup summing each
   line's inner `taxes` total).
6. **Config gate lifted.** A nested-`foreach` config `Build`s successfully (no `ErrNestedForEach`); a
   3-level nesting builds.
7. **`as`-collision rejected.** A chain reusing `as` (outer and inner both default `"item"`; or explicit
   collision) fails `Build` with `errors.Is(err, config.ErrForEachAsCollision)`, the `*ConfigError` naming
   both stages; a chain with distinct names builds; **sibling** nested foreaches reusing a name build.
8. **Cancellation across nesting.** A `ctx` canceled during inner iteration stops with a `*StageError`
   naming the element index, at an element boundary, without writing partial output.
9. **Nested inner error** names the failing index path (outer element and inner element).
10. **`Hash()` stability.** A pre-025 (single-level) ruleset hashes byte-identically after the gate is
    lifted (pinned golden-hash test).

All tests use the `table-test` assert-closure form, blackbox `_test` packages, and `t.Context()`.

## Traceability

Backlog: B9. Plan: 025. ADR: 0050 (records nested `foreach` support: hierarchical firing re-prefix, config
gate removal, `as`-chain guard, sentinel removal; supersedes ADR-0040 D7 and adds a superseding note to
ADR-0049 / Spec 015 D5 for the firing-query shape). Related: ADR-0040 (foreach stage & the D7 deferral being
closed), ADR-0049 / Spec 024 (per-element derivation merge, reused unchanged for nested lineage), ADR-0047 /
Spec 022 (B6 member-path refs + nearest-ancestor reconciliation, reused for the nested element subgraph),
ADR-0006 (deterministic sequential execution, preserved at every nesting level).
