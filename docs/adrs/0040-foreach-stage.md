# ADR-0040 — `foreach` stage: inner sub-pipeline & per-element scoping

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 015 (docs/specs/015-foreach-line-item-stage.md) / Plan
  015, realizing the `foreach` deferral recorded in ADR-0030, motivated by the
  post-010 audit's line-item-adjudication gap (I4).

## Context

Business rules routinely adjudicate **collections**: per line item (pricing
discounts), per collateral (lending), per coverage (insurance). With `foreach`
deferred (ADR-0030), the only tool was the single-expression `map`/`filter`/
`reduce` `expr` builtins — enough for "sum the line totals," but a hard wall
for real per-element decisioning: no way to run a **decision table** per
element, no per-element **provenance/firing**, and no structured way to
**collect per-element outputs** back into the Scope. Header-level decisions
worked; line-level adjudication did not. Spec 015 (D1–D9) resolves the design;
this ADR records the resulting architecture for Task 1 — the `ForEach` stage
core (iteration, per-element scope, `items` output, safety). Roll-ups and
per-element firing (Spec 015 G2/G3, D4/D5) are deferred to a later task in the
same increment.

## Decision

- **Inner unit is a sub-`Pipeline` (Spec 015 D1, reuse-over-special-case
  altitude).** A `ForEach` stage owns an inner `*pipe.Pipeline` of ordinary
  stages (`SingleExpr`/`MultiExpr`/`DecisionTable`), built by the caller (or,
  in a later task, from nested `StageDef`s). Per element it runs that inner
  pipeline against a **fresh per-element `Scope`**, so all existing stage
  machinery — DAG ordering, decision tables and hit policies, provenance,
  timing — works per element with zero new evaluation code. The alternative, a
  bespoke "decision-table-or-exprs" inner unit, was rejected: more code, less
  reuse, and a second evaluation path to keep debuggable.
- **Per-element scope, outer scope untouched (Spec 015 D2).** Each iteration
  seeds a new `Scope` from the outer scope's `Snapshot()` (so outer constants/
  thresholds stay readable) plus the element bound under a configurable name
  (`WithForEachAs`, default `"item"`). The outer `Scope` is never mutated
  during iteration — no key is set or unset on it — so provenance/replay of
  the outer decision is unaffected by iteration, and per-element evaluation
  cannot leak state between elements or into the caller's scope. Provenance
  tracking (`WithProvenance`) is threaded into the per-element scope
  precisely when the outer scope has it on, so the reuse is transparent to
  provenance too (full per-element firing capture is Task 2/3's concern).
- **Namespaced `<stage>.<output>` output (Spec 015 D3, refined during Task
  1).** Results are written as an ordered `[]any` of `map[string]any` (each
  element's per-element `Scope.Snapshot()`) under `<stage name>.<output>`
  (`WithForEachOutput`, default `"items"` — so a stage named `lines` writes
  `lines.items`). This is namespaced under the stage's own name, following the
  convention `MultiExpr` already uses for its own outputs
  (`<stage>.<exprName>`), rather than a bare top-level `items` key — avoiding
  collisions between multiple `foreach` stages in the same pipeline and
  keeping a `foreach` stage's output addressable the same way any other
  stage's output is.
- **Sequential iteration; typed, indexed errors (Spec 015 D8, G5).**
  Iteration is sequential in collection order — no concurrency, matching
  `Pipeline.Run`'s deterministic execution (ADR-0006). `ctx.Err()` is checked
  once before the loop and again before each element, so a canceled context
  stops iteration at the next element boundary rather than mid-element, and a
  large collection is cancellable. A missing collection path is
  `ErrForEachNoCollection`; a non-`[]any` value at the path is
  `ErrForEachNotList` (wrapped with the path and the value's dynamic type); a
  per-element inner-pipeline error is wrapped `"element %d: %w"` so the
  failing index is always named. Every error surfaces as a `*StageError`
  naming the `foreach` stage, per the existing `Stage` contract — no panics on
  caller input. An empty collection is a defined no-op: it writes an empty
  `[]any{}` at `<stage>.<output>`, not a silent gap and not an error.
- **Nested `foreach` deferred (Spec 015 D7, non-goal for this ADR).** Nothing
  in the `ForEach` core enforces single-level nesting yet — the inner
  `*Pipeline` is caller-constructed Go code in Task 1, so a caller could hand
  it a pipeline containing another `ForEach`. The config layer (a later task
  in Plan 015) is where nested `foreach` becomes reachable from declarative
  YAML/JSON, and that is where the build-time rejection belongs and will be
  enforced. This ADR records the deferral so it is not silently forgotten.

## Consequences

- **One new stage type, maximal reuse.** `ForEach` implements the existing
  `Stage` interface (`Name`/`Type`/`DependsOn`/`Execute`) exactly like
  `SingleExpr`/`MultiExpr`/`DecisionTable`, so it composes into a `Pipeline`,
  participates in the DAG, and is timed/ordered like any other stage — no
  changes to `Pipeline`, `Scope`, or any other existing stage type were
  needed.
- **Per-element decision tables, provenance, and firing become possible**
  simply by putting a `DecisionTable` stage inside the inner pipeline — the
  audit gap (I4) that motivated Spec 015 is closed at the mechanism level;
  Task 2/3 add roll-ups and outer-scope-visible per-element firing on top of
  this core.
- **Debuggability preserved.** Every failure path returns a typed
  `*StageError` (never a panic), and per-element errors always name the
  offending index — a developer can `errors.Is`/`errors.As` down to
  `ErrForEachNotList`/`ErrForEachNoCollection` or the inner stage's own typed
  error, and can breakpoint into `ForEach.Execute` and step through the exact
  per-element `Pipeline.Run` call that failed.
- **Nesting is an open door until the config layer closes it.** Programmatic
  callers can nest `foreach` today; only the declarative config path (a later
  task) enforces the D7 deferral. This is an accepted, temporary gap scoped to
  Task 1/2 of Plan 015, not a design flaw — closing it earlier would mean
  adding nesting-detection logic to `ForEach` itself, which contradicts the
  reuse-over-special-case altitude this ADR chose.

## Whole-branch gate outcome (increment 015 complete)

Tasks 2–4 (roll-ups + per-element firing, config surface, acceptance example)
landed on this core. The final `main..HEAD` review settled three more points:

- **Hash stability across the schema change (fixed).** `StageDef` gained four
  foreach fields (`Collection`/`As`/`Stages`/`Rollups`). Because `Hash()`
  fingerprints the canonical JSON of the parsed definition, always-emitted new
  fields would have changed the hash of *every* pre-foreach ruleset, silently
  breaking `MatchesRuleset` replay-verification across the upgrade. The four
  fields therefore carry `json:",omitempty"` so a stage using none of them
  serializes exactly as before; a pinned golden-hash test guards this.
- **Fail-loud roll-up validation (fixed).** `NewForEach` now rejects a `Rollup`
  with an empty `Key` or `As` (`ErrForEachEmptyRollup`) at construction — an
  empty `As` would otherwise be a confusing runtime path error and an empty
  `Key` a silently-empty roll-up. The config builder surfaces it as a
  `*ConfigError` naming the stage.
- **Deferred to a later increment (backlog, not blocking):**
  - *Dot-path roll-up keys.* **RESOLVED in increment 017 (ADR-0042).** `Rollup.Key`
    was originally looked up as a flat top-level key in each element's result map,
    so rolling up a **decision-table** output (namespaced `<table>.<key>`) needed a
    companion `single-expr` to surface the value top-level. `Key` is now
    dot-path-aware (backward-compatible: a dot-free key is unchanged), so the
    companion stage is no longer required.
  - *Per-element lineage (D5 beyond firing).* **RESOLVED in increment 024 (ADR-0049).** Each element's full
    derivation graph is now merged onto the outer scope under the `<stage>[i].`
    path prefix when the outer scope tracks provenance, so `Lineage`/`Explain`
    answer per-element lineage (e.g. `<stage>[i].<inner output>`) alongside the
    per-element firing.
  - *Per-element `Snapshot()`+`NewScope` cost.* **RESOLVED in increment 018 (ADR-0043).** Each element
    deep-copies the outer scope's map spine (O(elements × outer-scope size)); a benchmark
    (`BenchmarkForEachScopeCopy`) confirmed the cost is linear in both axes and sub-millisecond for typical
    line-item counts (~5 ms only at a deliberately extreme 1000-element × 64-key corner). Accepted as the
    price of the per-element isolation invariant (D2); no optimization now.

## Traceability

Spec: 015 (docs/specs/015-foreach-line-item-stage.md)
Plan: 015 (docs/plans/015-foreach-line-item-stage.md)
Related: ADR-0030 (foreach deferral), ADR-0006 (deterministic sequential
execution), ADR-0025 (integer-preserving aggregation, relevant to Task 2's
roll-ups).
