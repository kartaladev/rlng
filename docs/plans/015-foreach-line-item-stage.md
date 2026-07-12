# `foreach` line-item stage — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `foreach` stage that adjudicates a collection: for each element it runs an inner sub-pipeline against a per-element scope, writes structured per-element output, optionally rolls it up to header values, and records per-element firing — so line-item decisioning (per line, per collateral, per coverage) is expressible and explainable.

**Architecture:** A new `pipe.ForEach` stage (implements `pipe.Stage`) owns an inner `*pipe.Pipeline`. Its `Execute` resolves a `[]any` collection from the Scope by path, then for each element builds a fresh per-element `Scope` (seeded from the outer `Snapshot()` plus the element bound as `item`), runs the inner pipeline against it, and appends the element's result `Snapshot()` to a `<stage>.items` list. Optional `rollups` reduce a per-element key across elements via the 014-hardened `aggregate`/`foldNumeric`. Per-element firing is recorded into the outer scope under `<stage>[i]`. Config adds a `foreach` `StageDef` shape (nested `StageDef`s → the inner pipeline) dispatched in `build`.

**Tech Stack:** Go 1.25, `github.com/expr-lang/expr`, `github.com/shopspring/decimal` (via 014 aggregation), `gopkg.in/yaml.v3`, stdlib.

## Global Constraints

- Go 1.25+; pure Go, no cgo. **No new dependencies** (reuses existing). Library must not panic/os.Exit/log.Fatal on caller input — return typed errors; no global logger.
- Blackbox tests only (`package pipe_test` / `config_test` / `rlng_test`), mandatory `table-test` assert-closure form for ≥2 same-SUT cases, `t.Context()` over `context.Background()`. Export any sentinel a test must `errors.Is`.
- Every exported symbol has a godoc comment. Target ≥85% coverage on changed packages; **every hot-path and typed-error branch covered** — here the hot path is `ForEach.Execute` (iteration, per-element scope, inner Run, items write, roll-up) and every typed-error branch (non-list collection, missing collection, per-element error naming the index, ctx-cancel, nested-foreach build error).
- Additive & backward-compatible: no change to existing stage types or their wire form → normal `feat`, no `!`.
- **This increment is GATED: do NOT push. Merge/push awaits explicit user approval** (per the 2026-07-13 AFK directive). Per-task commits during SDD execution are pre-authorized (commit only).
- Traceability: every `feat` commit carries `Spec: 015`, `Plan: 015`, and (Task 1) `ADR: 0040` trailers, ending with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Implements Spec 015 (decisions D1–D9).

## Reused machinery (do not reinvent)

- `pipe.Stage` interface (`Name`/`Type`/`DependsOn`/`Execute`), `StageError`, `TypeForEach` constant convention — see `pipe/stage.go`.
- `Scope`: `NewScope(seed, opts...)`, `Snapshot()`, `Get(path)`, `Set(path, v)`, provenance/firing — see `pipe/scope.go`. `recordFirings` and the firing map — see `pipe/firing.go`.
- `Pipeline`: `NewPipeline(stages...)`, `Run(ctx, sc)` — see `pipe/pipeline.go`. The inner unit is one of these.
- `aggregate(agg CollectAggregation, vals []any)` and `CollectAggregation` (Sum/Min/Max/Count/List) — `pipe/table.go` (014-hardened: int64/decimal-faithful). `foreach.go` is in `package pipe`, so it calls these directly.
- Config `StageDef.build`/dispatch and per-type builders — `config/build.go` (the foreach builder recurses into nested `StageDef`s).

## File structure

- `pipe/foreach.go` (NEW) — `TypeForEach`, `ForEach`, `Rollup`, `NewForEach` + options, `Execute`; sentinels `ErrForEachNotList`, `ErrNestedForEach` (if enforced here) etc.
- `pipe/foreach_test.go` (NEW) — blackbox `package pipe_test`.
- `config/def.go` (MODIFY) — `StageDef` gains foreach fields (`Collection`, `As`, `Stages []StageDef`, `Output`, `Rollups []RollupDef`).
- `config/build.go` (MODIFY) — `build` dispatches `case pipe.TypeForEach: sd.buildForEach(...)`; recurse building the inner pipeline; reject a nested foreach.
- `config/foreach_test.go` (NEW) — config-level tests.
- `examples/foreach_lineitem_test.go` (NEW) — runnable line-item adjudication Example.
- `docs/adrs/0040-foreach-stage.md` (NEW).

---

## Task 1: ADR-0040 + `pipe.ForEach` core (iteration, per-element scope, items output, safety)

The core stage: iterate a collection, run the inner pipeline per element against a per-element scope, write `<stage>.items`, and handle every edge (empty, non-list, per-element error, ctx-cancel). No roll-up / per-element firing yet (Task 2).

**Files:** Create `pipe/foreach.go`, `pipe/foreach_test.go`; Docs `docs/adrs/0040-foreach-stage.md`.

**Interfaces (Produces):**
```go
const TypeForEach = "foreach"

// ForEach runs an inner pipeline once per element of a Scope collection.
type ForEach struct { /* name, deps, collection, as, inner *Pipeline, output string, rollups []Rollup */ }

// NewForEach compiles a foreach stage. collection is the Scope path to a []any;
// inner is the sub-pipeline run per element; opts set the element binding (default
// "item"), output key (default "items"), rollups, and depends-on.
func NewForEach(name, collection string, inner *Pipeline, opts ...ForEachOption) (*ForEach, error)

type ForEachOption func(*forEachConfig)
func WithForEachAs(name string) ForEachOption      // element binding, default "item"
func WithForEachOutput(key string) ForEachOption   // default "items"
func WithForEachDependsOn(deps ...string) ForEachOption

// ErrForEachNotList is the Cause of a StageError when the collection path holds a non-list.
var ErrForEachNotList = errors.New("foreach: collection is not a list")
// ErrForEachNoCollection is the Cause when the collection path is absent.
var ErrForEachNoCollection = errors.New("foreach: collection path not found")
```

- [ ] **Step 1: Write failing tests** (`pipe/foreach_test.go`) — a table over `ForEach.Execute`:
  - each element evaluated against its per-element scope with the element bound as `item`; the outer scope (a constant/threshold) still readable; results written under `<stage>.items` as `[]map[string]any` in collection order (assert element values + order).
  - empty collection (`[]any{}`) → `<stage>.items` is an empty list (defined no-op), no error.
  - non-list at the path (e.g. an int) → `errors.Is(err, ErrForEachNotList)` inside a `*StageError`.
  - missing collection path → `errors.Is(err, ErrForEachNoCollection)`.
  - a per-element inner error → a `*StageError` whose message names the element index.
  - context cancelled before element k → iteration stops with a ctx error (cancel the `t.Context()`-derived ctx after the first element via an inner stage or a pre-cancelled ctx).
  Build the inner pipeline with existing stages (`pipe.NewSingleExpr`/`NewDecisionTable`) referencing `item.<field>` and an outer constant.

- [ ] **Step 2: Run to verify failure** — `go test ./pipe/ -run ForEach -v` (FAIL: undefined).

- [ ] **Step 3: Implement `pipe/foreach.go`.** `Execute`:
  1. `if err := ctx.Err(); err != nil { return f.stageErr(err) }`.
  2. `raw, ok := sc.Get(f.collection)`; `!ok` → `f.stageErr(ErrForEachNoCollection)`.
  3. `list, ok := raw.([]any)`; `!ok` → `f.stageErr(fmt.Errorf("%w: %s (%T)", ErrForEachNotList, f.collection, raw))`.
  4. `items := make([]any, 0, len(list))`; for `i, el := range list`: check `ctx.Err()` (name index on cancel); build per-element seed `seed := sc.Snapshot(); seed[f.as] = el`; `esc := NewScope(seed)` (propagate provenance if `sc.TracksProvenance()` — via a ScopeOption); `if err := f.inner.Run(ctx, esc); err != nil { return f.stageErr(fmt.Errorf("element %d: %w", i, err)) }`; `items = append(items, esc.Snapshot())`.
  5. `return sc.Set(f.output, items)`.
  Constructor validates name (`ErrEmptyStageName`), non-nil inner pipeline, non-empty collection path; defaults `as="item"`, `output="items"`. `Name/Type/DependsOn` per the interface; `Type()` returns `TypeForEach`; `stageErr` wraps in `&StageError{Stage,Type:TypeForEach,Cause}`.

- [ ] **Step 4: Run tests to pass** — `go test ./pipe/ -run ForEach -race -v`, then `go test ./pipe/ -race`.

- [ ] **Step 5: ADR-0040** (`docs/adrs/0040-foreach-stage.md`, Nygard): Context = line-item adjudication gap (audit I4, ADR-0030 foreach deferral, Spec 015); Decision = inner sub-pipeline + per-element scope (D1/D2), `items` output (D3), reuse-over-special-case altitude, nested-foreach deferred (D7); Consequences = one new stage type, reuses all stage machinery, nesting deferred. Cite Spec 015 / Plan 015.

- [ ] **Step 6: Commit** — `feat(pipe): foreach line-item stage core (per-element sub-pipeline)`; trailers `Spec: 015`, `Plan: 015`, `ADR: 0040`. Files: `pipe/foreach.go`, `pipe/foreach_test.go`, the ADR.

---

## Task 2: Roll-up + per-element firing/provenance

Reduce per-element outputs to header values (reusing 014 aggregation) and record per-element firing under `<stage>[i]`.

**Files:** Modify `pipe/foreach.go`, `pipe/foreach_test.go`.

**Interfaces (Produces):**
```go
// Rollup reduces a per-element output key across all elements into <stage>.<As>.
type Rollup struct { Key string; Agg CollectAggregation; As string }
func WithRollups(r ...Rollup) ForEachOption
```

- [ ] **Step 1: Failing tests** — table:
  - roll-up by each aggregation (Sum/Min/Max/Count/List) over per-element numeric outputs reduces to the correct header value at `<stage>.<As>` (use decimal + int64 element values to prove 014 fidelity carries through).
  - roll-up over an empty collection → the aggregation's identity/empty result (Count→0, List→empty, Sum over no elements → defined: skip or zero — assert the chosen behavior; recommend Count=0, List=[], Sum/Min/Max over empty = the key absent or a typed no-op — pick and test).
  - per-element firing: build an inner decision-table with rule IDs; assert `sc.FiringRulesFor("<stage>[i]")` returns element *i*'s fired rule(s) and is correctly attributed to *i*.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.** After building `items` (Task 1): for each `Rollup`, collect `vals := []any{}` from `items[*][r.Key]` (skip absent), `reduced, err := aggregate(r.Agg, vals)`, `sc.Set(r.As, reduced)`. Define empty-collection roll-up behavior explicitly. For firing: after each element's inner `Run`, read the per-element scope's firing (add a `Scope` accessor if none exists — reuse `FiringRulesFor`/the firing map) and `sc.recordFirings(fmt.Sprintf("%s[%d]", f.name, i), elementFirings)`. Keep provenance propagation consistent with Task 1.

- [ ] **Step 4: Tests pass** — `go test ./pipe/ -run ForEach -race`, then `go test ./pipe/ -race`.

- [ ] **Step 5: Commit** — `feat(pipe): foreach roll-up aggregation + per-element firing`; trailers `Spec: 015`, `Plan: 015`, `ADR: 0040`.

---

## Task 3: Config surface (`type: foreach`) + nested-foreach guard

Express a foreach stage in YAML/JSON with nested inner `StageDef`s, honoring strict decoding; reject nested foreach at build.

**Files:** Modify `config/def.go`, `config/build.go`; Create `config/foreach_test.go`.

**Interfaces (Produces):** `StageDef` gains `Collection string`, `As string`, `Stages []StageDef`, `Output string`, `Rollups []RollupDef{Key, Agg, As}`. `build` dispatches `case pipe.TypeForEach: sd.buildForEach(...)`.

- [ ] **Step 1: Failing tests** (`config/foreach_test.go`) — table over `config.ParseYAML(...).Build()`:
  - a valid foreach stage parses and builds; running it produces the expected `items` + roll-up (drive end-to-end through the public `Build` + a `pipe.Scope` run).
  - strict decoding: an unknown field on the foreach stage (or inner stage) is a decode error.
  - an invalid inner stage (e.g. bad expr) is a build-time error naming the outer stage and inner stage.
  - a **nested** foreach (an inner stage of `type: foreach`) is a build-time error (`errors.Is(err, config.ErrNestedForEach)` or a `*ConfigError` with a clear message).

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.** Add the fields to `StageDef` (json/yaml tags; mirror the existing decode-validation pre-check so unknown fields are attributed). `buildForEach`: build each inner `StageDef` via the existing `sd.build`-equivalent path into `[]pipe.Stage`, `pipe.NewPipeline(inner...)`, then `pipe.NewForEach(name, collection, innerPipeline, WithForEachAs(as), WithForEachOutput(output), WithRollups(...), WithForEachDependsOn(deps...))`. Before building an inner stage, if its `Type == pipe.TypeForEach` → return a `*ConfigError` wrapping `ErrNestedForEach` (D7). Map `RollupDef.Agg` (string) → `pipe.CollectAggregation` (reuse the existing agg-name parsing if present, else add one). Pass constants/schema/strict into the inner build so inner expressions see the same constants and strict-env.

- [ ] **Step 4: Tests pass** — `go test ./config/ -race`, then `go test ./... -race`.

- [ ] **Step 5: Commit** — `feat(config): foreach stage config surface + nested-foreach guard`; trailers `Spec: 015`, `Plan: 015`, `ADR: 0040`.

---

## Task 4: Acceptance example + docs + whole-branch gate

**Files:** Create `examples/foreach_lineitem_test.go`; Modify `README.md`; ADR/spec cross-links.

- [ ] **Step 1: Example** — a runnable `Example` adjudicating loan collateral line items: a collection of items each with an amount/LTV; a `foreach` runs an inner decision-table (e.g. `LTV_MAX_80`) per item, writes `items`, rolls up the total approved amount (decimal, exact), and shows `FiringRulesFor` naming which rule denied line 3. `// Output:` deterministic.

- [ ] **Step 2: Run it** — `go test ./examples/ -run ForEach -v`.

- [ ] **Step 3: README** — add a "Line-item adjudication (`foreach`)" subsection; note nested-foreach is deferred.

- [ ] **Step 4: Commit** — `docs(examples): foreach line-item adjudication example`; trailers `Spec: 015`, `Plan: 015`, `ADR: 0040`.

- [ ] **Step 5: Whole-branch gate** over `main..HEAD`: `/code-review` + `/security-review`; fix/triage findings; `go test ./... -race`, `go vet`, `gofmt -l`, `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. Update `docs/HANDOVER.md`. **Then STOP and present increment 015 for the user's merge/push decision — do NOT push (gated per the AFK directive).**

## Self-review (author checklist)

- **Spec coverage:** G1→Task 1 (inner sub-pipeline + per-element scope). G2→Task 1 (items) + Task 2 (roll-up). G3→Task 2 (per-element firing). G4→Task 3 (config). G5→Tasks 1–2 (empty no-op, non-list error, indexed error, ctx-cancel) + D9 fidelity carried from 014. ✅
- **Reuse/altitude:** inner unit is a full sub-`Pipeline` (reuses stages/DAG/firing/provenance); roll-up reuses `aggregate`; firing reuses `recordFirings`. No special-case duplication of stage evaluation. ✅
- **Open items for the implementer to settle with a test (not left vague):** (1) empty-collection roll-up semantics for Sum/Min/Max (recommend: key omitted / no-op, Count=0, List=[]) — pick and test in Task 2 Step 1. (2) provenance propagation into per-element scopes — mirror `sc.TracksProvenance()` via a ScopeOption. (3) whether per-element firing accessor needs a new small exported `Scope` method — add one if the firing map isn't already reachable, godoc'd + tested.
