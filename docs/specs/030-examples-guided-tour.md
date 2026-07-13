# Spec 030 — examples as a numbered, pedagogical guided tour

- **Status:** Accepted
- **Driver:** User request (2026-07-14, autonomous session): restructure the `examples/` tests into a
  numbered progression from the simplest concept (starting at the `expr` layer) to the most complex,
  **completely exposing every library capability** including its **variances/quirks**; make each example a
  **deliberately-designed real-world scenario** (2–3 per topic), with **elaborate doc comments that teach the
  reader each feature in detail**, and **prettified, readable** sample code. Then rewrite the `README.md`
  `Usage` section to explain the examples with gradually-increasing complexity, cross-referencing them
  (not too verbose).
- **Realized by:** Plan 030. No ADR (documentation/test increment — no architectural decision, no library
  behavior change, no exported-API change).

## Problem

The `examples/` package today is a good but ad-hoc set of 8 files / 12 `Example` functions in no particular
reading order, and it leaves large parts of the capability surface undocumented by example (per the 030
inventory: nested foreach is present but ruleset replay, strict-env/lint, concurrency, coercing getters,
clock injection, typed result mapping/decimal-narrowing, fallbacks/observers, and most quirks are not, or are
scattered). A reader cannot open `examples/` and learn the library front-to-back.

## Goal

Turn `examples/` into a **guided tour**: a reader starts at file `01`, the simplest expression primitive,
and walks up to file `14`, the full typed engine, learning every capability and its notable quirks along the
way through realistic scenarios. The examples remain runnable (`go test ./examples/`) and double as godoc.

## Design

### Principles

1. **Numbered files = reading order.** Files are named `NN_<layer>_<topic>_test.go` (`01`…`14`); the leading
   number gives the tour sequence in a directory listing and in `doc.go`'s index. (Go example *function*
   names keep a lowercase-first suffix — a digit-leading `Example_01_x` suffix is not recognized as a valid
   example by `go test`, so ordering lives in the filename + `doc.go` index + README, not the func name.)
2. **Simplest → most complex, bottom-up by layer.** `expr` (pure expressions) → `pipe` (scope, stages,
   tables, foreach, pipeline, provenance) → `config` (declarative YAML/JSON, replay) → `rlng` (the engine
   facade, typed mapping). Each file depends only on concepts introduced in lower-numbered files.
3. **Real-world scenarios, 2–3 per topic.** Lending/credit, pricing/tax, invoicing/money, order line-items,
   eligibility, adverse-action — deliberately designed, not `foo/bar`. Reuse the strong existing scenarios,
   enrich them, and add scenarios for the uncovered capabilities.
4. **Teach, don't just compile.** Every `Example` carries a doc comment that explains *what* the feature is,
   *why* you'd use it, and *the quirk/edge it demonstrates* (drawn from the 030 capability inventory). The
   file-level doc comment frames the topic. The code is prettified: meaningful names, aligned/commented
   inputs, readable YAML, a deterministic `// Output:` block.
5. **Expose the quirks explicitly.** Each capability's notable variance is shown or called out: lenient
   truthiness (NaN/±Inf falsy, `json.Number` via the string path), fallback-over-empty-env, `??` default
   precedence, compile-time decimal operator resolution + bare-var-needs-`WithEnv`, coercing-getter overflow
   errors, condition-false-is-a-no-op, collect widest-kind promotion + empty-fold-absent, hit-policy
   unique/any errors, `CycleError` concrete path, concurrency determinism, `ErrForEachAsCollision`,
   provenance exact/ancestor reconciliation + depth truncation, `$dec` marker + Hash normalization, strict
   schema turning a typo into a Build error, lint false-positive caveat, `Hash` excludes `Version`,
   `ErrConcurrencyRequiresConfig`, decimal→result lossy-narrowing error, `ErrNilInput` vs empty map.

### File progression (topic → capabilities & quirks)

| # | File | Topic | Capabilities / quirks shown |
|---|------|-------|------------------------------|
| 01 | `01_expr_predicates_test.go` | `expr` boolean predicates | `NewPredicate` strict (`ErrNotBool`); `WithCoerce` lenient truthiness incl. NaN/±Inf/empty-string/`json.Number` quirks |
| 02 | `02_expr_functions_test.go` | `expr` value functions & fallbacks | `NewFunction`; `WithFallback` (empty-env, eager compile), `WithFallbackOnNil`, `WithFallbackObserver`, `WithReturnKind` |
| 03 | `03_expr_variables_env_test.go` | `expr` variable defaults, strict env, host fns | `WithGlobals`/`WithLocals` (`??` precedence/merge); `WithEnv` typo→`CompileError`; `WithFunction` (last-wins) |
| 04 | `04_expr_decimal_test.go` | `expr` exact-decimal money | `decimal()`, `+ - * /` overloads, `round`/`roundBank`; compile-time operator resolution; bare-var needs `WithEnv` |
| 05 | `05_pipe_scope_getters_test.go` | `pipe` Scope & typed getters | `NewScope`/`Set`/`Get` dot-paths, `WithStrict` conflict; strict vs coercing getters + overflow/non-integral errors |
| 06 | `06_pipe_stages_test.go` | `pipe` single & multi stages | `SingleExpr` `WithCondition` (no-op when false)/`WithOutput`; `MultiExpr` priority + intra-stage aliasing |
| 07 | `07_pipe_decision_table_test.go` | `pipe` decision tables | single+`WithDefault`+firing (`ID`/`Message`/`IsDefault`); collect+aggregations (widest-kind, empty-fold); unique/any + `ErrMultipleMatches`/`ErrConflictingMatches` |
| 08 | `08_pipe_pipeline_concurrency_test.go` | `pipe` DAG & parallel | topo order out-of-declaration; `CycleError` path; `WithConcurrency`/`WithMaxParallel` determinism; `InvalidMaxParallelError` |
| 09 | `09_pipe_foreach_test.go` | `pipe` per-element iteration | `ForEach` + `Rollup` (dot-path key), per-element firing `lines[i]`; nested foreach + composite key; `ErrForEachAsCollision`; empty-fold-absent |
| 10 | `10_pipe_provenance_clock_test.go` | `pipe` explainability & time | `WithProvenance` `Explain`/`Lineage`/`Derivations`; firing trail; `WithClock` + `now()` deterministic temporal rule |
| 11 | `11_config_rulesets_test.go` | `config` declarative rulesets | `Parse` YAML+JSON, shorthand vs object; `Constants`+`$dec`; `schema` strict (typo→Build error); `Lint`/`WithLintErrors` |
| 12 | `12_config_replay_test.go` | `config` identity & replay | `Hash` (excludes `Version`)/`MatchesRuleset`; `Scope.Ruleset` stamp; Scope JSON persist→replay |
| 13 | `13_engine_untyped_test.go` | `rlng` untyped engine | `Engine.Evaluate`→map / `EvaluateScope`→`*Scope`; `WithScopeOptions(WithProvenance)`; `NewFromYAML` one-call |
| 14 | `14_engine_typed_test.go` | `rlng` typed engine & mapping | `TypedEngine[I,R]`, `Mapper`/`MappingTemplate`, decimal struct restore + lossy-narrowing error; `NewTypedFromYAML`; `ErrConcurrencyRequiresConfig` |

### `doc.go` index

Rewrite `examples/doc.go` to frame the package as an ordered tour and list the 14 topics (a one-line index),
so `go doc ./examples` and the file listing both read as a curriculum.

### README `Usage` rewrite

Reorder the `## Usage` section to mirror the tour's gradually-increasing complexity — **start with the
simplest** (`expr` one-off predicate/function), then pipe stages → decision tables → foreach → config →
untyped engine → typed engine. Keep each subsection short (intro sentence + one compact snippet + the quirk
in a line), and end each with a cross-reference to the governing numbered example
(`see examples/NN_*_test.go`). Update the existing file references (e.g. the old
`examples/decimal_money_test.go` pointer) to the new numbered files. Not-too-verbose is a hard requirement —
the examples carry the depth; the README is the map.

## Non-goals

- **No change to the per-package godoc `Example*` tests** (`expr/*_example_test.go`, `pipe/*_example_test.go`,
  `config/*_example_test.go`, root `example_test.go`/`fromconfig_example_test.go`). Those are API-reference
  examples bound to exported symbols and are appropriate as-is; the tour lives in `examples/`.
- **No library code change.** This is documentation/tests only — no exported-API, behavior, `Hash()`, or
  schema change; the whole build stays green and race-clean.
- **No new dependency.**

## Success criteria

1. `examples/` contains the 14 numbered files; every listed capability/quirk appears in at least one example;
   each topic has 2–3 real-world `Example` functions (1–2 acceptable where a capability is genuinely singular,
   e.g. replay).
2. `go test ./examples/ -race` is green — every `// Output:` block matches actual output.
3. Every `Example` has a teaching doc comment (what/why/quirk); `doc.go` lists the ordered tour.
4. `go build ./... && go vet ./... && gofmt -l .` clean; no exported-API change (examples import only public
   API).
5. README `Usage` is reordered simplest→complex, each subsection cross-references its numbered example, all
   file references resolve, and it stays concise.

## Traceability

Driver: user request (autonomous session 2026-07-14). Plan: 030. Inventory input: the 030 capability-surface
report. No ADR. Supersedes the ad-hoc `examples/` layout (files replaced/renamed under the numbered scheme).
