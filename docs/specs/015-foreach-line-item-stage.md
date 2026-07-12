# Spec 015 — `foreach` line-item stage

- **Status:** Accepted (design brainstormed autonomously & approved-pending 2026-07-13; see Resolved decisions)
- **Date:** 2026-07-12 (decisions resolved 2026-07-13)
- **Post-010 audit remediation.** Realizes the `foreach` deferral recorded in
  ADR-0030; motivated by the audit's line-item-adjudication gap (I4).
- **Builds on:** Spec 002 (`pipe` Scope & stages), Spec 003 (`Pipeline` DAG),
  Spec 006 (provenance & firing), Spec 010 (decision tables), ADR-0030.
- **Realized by:** Plan 015.
- **Anticipated ADRs:** ADR-0040 (foreach stage & per-element scoping).
- **Relates to:** Spec 012 (per-element firing reuses the collect/any firing
  model) and Spec 014 (per-element numeric outputs honor the value contract).

## Context

Business rules routinely adjudicate **collections**: per line item (pricing
discounts), per collateral (lending), per coverage (insurance). The audit found
that with `foreach` deferred (ADR-0030), the only tool is the single-expression
`map`/`filter`/`reduce` expr builtins. Those cover "sum the line totals" but hit a
hard wall for real per-element decisioning: you **cannot run a decision *table*
per element**, cannot produce **per-element provenance/firing rules**, and cannot
**collect per-element outputs back into the scope** in a structured, explainable
way. Header-level decisions work today; line-level adjudication does not.

## Goals

1. **G1 — A `foreach` stage type (ADR-0040).** A new stage that iterates a
   collection resolved from the Scope by path (a `[]any` / list of maps) and, for
   each element, evaluates an inner unit — a decision table and/or a set of
   expressions — against a **per-element scope** where the element is addressable
   (e.g. bound as `item`, with the outer scope still readable for shared
   constants/thresholds). Fits the existing `Stage` contract (`Name`/`Type`/
   `DependsOn`/`Execute`) and the DAG so it orders like any other stage.
2. **G2 — Structured per-element output.** Per-element results are written back
   under an indexed/namespaced path (e.g. `stage.items[i].<key>`, or a list of
   per-element result maps under `stage.items`) so downstream stages and the
   mapper can consume them, and an optional roll-up (reuse Spec 010 collect
   aggregation: sum/min/max/count/list) reduces per-element outputs to a
   header-level value in the same stage or a following one.
3. **G3 — Per-element explainability.** Provenance and firing are recorded **per
   element**, not just per stage: element *i*'s firing rule(s) and lineage are
   retrievable, so "line 3 was denied by rule LTV_MAX_80" is answerable. This
   reuses the multi-firing model from Spec 012 (G1) keyed by element index.
4. **G4 — Config surface.** Express a `foreach` stage in YAML/JSON (the collection
   path, the element binding name, the inner table/expressions, the optional
   roll-up), consistent with Spec 011's strict-decoding and (opt-in) strict-env
   guarantees, so line-item rules are authorable in the same document.
5. **G5 — Safety & determinism.** Iteration is deterministic (in collection
   order); an empty collection is a defined no-op (writes an empty result, not a
   silent gap); a non-list at the collection path is a typed stage error, not a
   panic; per-element evaluation stays panic-safe and context-cancellable at
   element boundaries (bounding a large collection).

## Non-goals

- Nested `foreach` beyond one level in the first cut (decide in ADR-0040 whether
  to allow it or defer; if deferred, error clearly on nesting).
- Parallel per-element evaluation (iteration is sequential and deterministic;
  concurrency is a later performance concern, not a semantic one).
- Cross-element rules ("this line relative to the previous line") beyond what an
  explicit roll-up + a following stage can express.

## Hot-path branches (test targets)

- Iteration: each element evaluated against its per-element scope with the element
  bound; the outer scope (constants/thresholds) still readable; results written
  under the indexed/namespaced path in collection order.
- Roll-up: per-element outputs reduced by each aggregation (sum/min/max/count/
  list); roll-up over an empty collection is the defined identity/empty result.
- Explainability: element *i*'s firing rule(s) and lineage retrievable and
  correctly attributed to element *i*.
- Edge cases: empty collection → defined no-op result; a non-list / missing
  collection path → typed `StageError` (no panic); a per-element expression error
  → typed error naming the element index; context cancellation between elements
  stops iteration.
- Config: a `foreach` stage parses and builds (strict-decoding honored); an
  invalid inner unit is a build-time error naming the stage.

## Resolved decisions (brainstormed autonomously 2026-07-13; approval-pending)

> Brainstormed without live user input per the user's 2026-07-13 AFK directive.
> Plan 015 realizes these; ADR-0040 records them. **This increment is to be left
> GATED — merge/push awaits explicit user approval.** Decisions favor maximal
> reuse of existing machinery over new special-case code (altitude).

- **D1 — Inner unit is a sub-`Pipeline` (G1, reuse/altitude):** a `foreach` stage
  owns an inner `*pipe.Pipeline` of ordinary stages (single-expr/multi-expr/
  decision-table), built from nested `StageDef`s. Per element it runs that inner
  pipeline against a **fresh per-element `Scope`**, so ALL existing stage
  machinery (DAG ordering, decision tables + hit policies, firing, provenance,
  timing) works per element with no new evaluation code. *Alt rejected:* a
  bespoke "decision-table-or-exprs" inner unit — more code, less reuse.
- **D2 — Per-element scope (G1):** the per-element `Scope` is seeded from the
  outer scope's `Snapshot()` (outer constants/thresholds readable) plus the
  element bound under a configurable name, default `item` (`as:`). The outer
  scope is **not mutated** during iteration. *Alt rejected:* setting/unsetting an
  `item` key on the shared parent scope — mutation hazards for provenance/replay.
- **D3 — Structured per-element output (G2):** results are written as a list
  under `<stage>.<output>` (default `output: items`), element *i* = the inner
  pipeline's resulting data map (its `Snapshot()`), in collection order. Downstream
  stages and the mapper consume `items` as a `[]map[string]any`. *Alt rejected:*
  synthesizing `items[i].<key>` dot-paths into the scope — awkward and unbounded.
- **D4 — Roll-up reuses Spec-010 / 014 aggregation (G2):** an optional `rollups`
  list, each `{key, agg, as}`, reduces a per-element output `key` across elements
  by `agg` (sum/min/max/count/list, via the 014-hardened `foldNumeric` — int64/
  decimal-faithful) and writes the result at `<stage>.<as>`. Roll-up over an
  empty collection is the aggregation's identity/empty result.
- **D5 — Per-element firing & provenance (G3):** each element's inner-pipeline
  firing is recorded into the outer scope under the composite stage key
  `<stage>[i]` (reusing the Spec-012 multi-firing model + `recordFirings`), so
  `FiringRulesFor("<stage>[i]")` answers "line *i* denied by rule LTV_MAX_80".
  Per-element lineage is available on each element's result.
- **D6 — Config surface (G4):** `type: foreach`, `collection: <scope path>`,
  `as: item` (default), `stages: [<inner StageDef>...]` (the inner unit, nested
  `StageDef`s built into the sub-pipeline), `output: items` (default), and
  `rollups: [{key, agg, as}]`. Spec-011 strict decoding is honored (unknown
  fields rejected; an invalid inner stage is a build-time error naming the
  outer stage and the inner stage).
- **D7 — Nested `foreach` DEFERRED (non-goal):** an inner unit that itself
  contains a `foreach` stage is a **build-time error** with a clear message.
  Bounds first-cut complexity (per-element indexing/firing across nesting is
  deferred); reassess in a later increment.
- **D8 — Safety & determinism (G5):** iteration is sequential in collection
  order; an empty collection writes an empty `items` (`[]`) and identity roll-ups
  (a defined no-op, not a silent gap); a non-list / missing value at `collection`
  is a typed `StageError` (no panic); a per-element inner error is a typed
  `StageError` naming the element index; `ctx.Err()` is checked before each
  element so a large collection is cancellable at element boundaries.
- **D9 — Value fidelity (relates to 014):** per-element numeric outputs honor the
  014 value contract (decimal/int64 preserved through the per-element scope, the
  `items` list, roll-up aggregation, and any JSON round-trip).
