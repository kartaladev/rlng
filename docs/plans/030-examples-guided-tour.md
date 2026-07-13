# Examples Guided Tour — Implementation Plan

> **For agentic workers:** execute task-by-task; each task ends with a green `go test ./examples/ -race` and
> a commit. Because example `// Output:` blocks must match real runtime output, each task AUTHORS the
> examples then RUNS them to obtain/verify the Output — the code steps are detailed briefs, not transcription.

**Goal:** Realize Spec 030 — restructure `examples/` into a 14-file numbered, pedagogical guided tour
(simplest `expr` primitive → full typed engine), then rewrite the README `Usage` section to mirror it.

**Architecture:** `examples/` is `package examples` (blackbox, imports only the public API of `expr`, `pipe`,
`config`, `rlng`). Each numbered file is a self-contained topic with 2–3 real-world `Example` functions, each
with a teaching doc comment (what / why / the quirk) and a deterministic `// Output:` block. Files depend only
on concepts from lower-numbered files. Per-package godoc `*_example_test.go` are NOT touched.

**Tech Stack:** Go 1.25+, `expr-lang/expr`, `shopspring/decimal`, `gopkg.in/yaml.v3`, `go-viper/mapstructure/v2`.

## Global Constraints

- **Docs/tests only — no library code change.** Do not edit any non-test `.go` file under `expr/`, `pipe/`,
  `config/`, or the root package. Examples import only the exported API. No exported-API change.
- **Every `Example` teaches:** a doc comment stating what the feature is, why you'd reach for it, and the
  specific quirk/edge it demonstrates (quirks are enumerated per file below and in Spec 030 / the 030
  inventory). The file also carries a `// <NN> — <topic>` file-level doc comment framing the topic.
- **Real-world, prettified, deterministic:** deliberately-designed domain scenarios (lending, pricing,
  invoicing, orders, eligibility, adverse-action — not foo/bar); meaningful names; readable aligned inputs and
  YAML; a `// Output:` block that matches exactly. Prefer `fmt.Println`/`fmt.Printf` with stable formatting.
  Avoid map-iteration-order-dependent output (sort keys, or print specific fields).
- **Naming:** files `NN_<layer>_<topic>_test.go`; `Example` function suffixes are lowercase-first and
  descriptive (NOT digit-leading — `Example_creditTierPrime`, not `Example_07_1`). Blackbox `package examples`.
- **Reuse existing scenarios** where they map (the pre-reset files: `config_test.go`, `decimal_money_test.go`,
  `eligibility_test.go`, `foreach_lineitem_test.go`, `lending_test.go`, `nested_foreach_test.go`,
  `pricing_test.go`, `strict_typing_test.go`) — read them via `git show 4659b54:examples/<file>` if already
  removed — enrich their comments and fold them into the numbered topic. Do not lose a good scenario.
- **Green gate per task:** `go build ./... && go vet ./... && gofmt -l .` (empty) and `go test ./examples/ -race`
  pass before each commit. The task commit is pre-authorized (plan-execution exception); `git commit` only —
  no push/merge/branch.
- **Module path:** github.com/kartaladev/rlng. Commit trailers `Spec: 030`, `Plan: 030` on every commit.
- Base commit before Task 1: `4659b54` (spec 030). Plan file rides with Task 1's commit.

## API reference (verified names — use these, don't invent)

- **expr:** `NewPredicate(expr, ...Option) (*Predicate, error)`, `Predicate.Test(env any)`; `NewFunction(name, expr, ...Option)`, `Function.Apply(env any)`; options `WithCoerce`, `WithFallback`, `WithFallbackOnNil`, `WithFallbackObserver(func(name, expr string, cause error))`, `WithReturnKind(reflect.Kind)`, `WithGlobals(map[string]any)`, `WithLocals(map[string]any)`, `WithEnv(map[string]any)`, `WithFunction(name, func(...any)(any,error))`; sentinels `ErrNotBool`, `ErrEmptyExpression`; `*CompileError`, `*EvalError`. Decimal builtins in-expr: `decimal(x)`, `+ - * /`, `round(d,n)`, `roundBank(d,n)`.
- **pipe:** `NewScope(seed map[string]any, ...ScopeOption)`, `Scope.Set/Get`, `WithStrict()`, `WithProvenance()`, `WithClock(Clock)`; getters `GetInt/GetInt64/GetFloat64/GetString/GetBool/GetSlice/GetMap`, `GetIntCoerce/GetInt64Coerce/GetFloat64Coerce`, `GetAs[T](*Scope,path)`; `NewSingleExpr(name,expr,...Option)` with `WithCondition(expr,...)`, `WithOutput(path)`, `WithDependsOn(...)`, `WithExprOptions(...expr.Option)`; `NewMultiExpr(name, []NamedExpr, ...Option)`; `NewDecisionTable(name, []Rule, ...Option)` with `WithHitPolicy`, `WithCollectAggregation`, `WithDefault(map[string]Decision)`; `Rule{ID,Message,Condition,Decisions map[string]Decision,ConditionOptions}`, `Decision{Expr,Options}`; policies `HitPolicySingle/Collect/Unique/Any`; aggregations `AggregateList/Sum/Min/Max/Count`; errors `ErrMultipleMatches/ErrConflictingMatches/ErrAggregateOverflow/ErrNonNumericAggregate/ErrNonFiniteAggregate`; `NewForEach(name, collectionPath, *Pipeline, ...ForEachOption)` with `WithForEachAs`, `WithForEachOutput`, `WithRollups(...Rollup)`, `WithForEachDependsOn`; `Rollup{Key,Agg,As}`; `NewPipeline([]Stage, ...PipelineOption)` with `WithConcurrency()`, `WithMaxParallel(n)`, `WithRuleset(RulesetIdentity)`; errors `*DuplicateStageError/*UnknownDependencyError/*CycleError/*InvalidMaxParallelError`; `Scope.Explain/Lineage/Derivations/Derivation`, `Scope.FiringRule/FiringRulesFor/FiringRules`, `Scope.Duration/StartedAt/StageTimings`, `Scope.Ruleset`, `pipe.NowFunc(clock)`. (Confirm exact option/method spellings against the source before use.)
- **config:** `Parse(ctx, Provider)`, providers `FromYAMLString/FromJSONString/FromYAMLBytes/FromJSONBytes/FromFile/FromYAMLFile/FromJSONFile/FromReader/FromYAMLURL/FromJSONURL`; `PipelineDef.Build(...BuildOption)` with `WithLintErrors/WithConcurrency/WithMaxParallel/WithStrict/WithSchema/WithRulesetVersion`; `PipelineDef.Lint/Hash/MatchesRuleset`; `*ConfigError/*LintError`; `$dec` constant marker.
- **rlng:** `New(*pipe.Pipeline, ...Option)`, `NewTypedEngine[I,R](*pipe.Pipeline,*Mapper[R],...Option)`, `NewFromYAML(ctx,yaml,...Option)`, `NewFromProvider`, `NewTypedFromYAML[I,R]`, `NewTypedFromProvider`; `Engine.Evaluate/EvaluateScope`; `TypedEngine.Evaluate`; `NewMapper[R](MappingTemplate)`, `MappingTemplate map[string]string`; options `WithScopeOptions(...pipe.ScopeOption)`, `WithConcurrency`, `WithMaxParallel`; sentinels `ErrNilPipeline/ErrNilMapper/ErrNilInput/ErrConcurrencyRequiresConfig`.

---

## Task 1 — reset + `expr` layer (files 01–04)

**Files:** remove all `examples/*_test.go` (the 8 old files) — keep `doc.go`; create `01_expr_predicates_test.go`, `02_expr_functions_test.go`, `03_expr_variables_env_test.go`, `04_expr_decimal_test.go`. Stage the plan file too.

Before removing, read the old files for reusable scenarios. Then:

- **01 — predicates (2–3 examples).** (a) strict boolean eligibility gate (`NewPredicate("age >= 18 && verified")`, `Test`); show a non-bool expression returning `*EvalError`/`ErrNotBool` via `errors.Is`. (b) lenient truthiness feature flag (`WithCoerce`) driven by a `"1"`/`"true"`/`""` string and a numeric count — call out the quirks in comments: `json.Number` goes through the string-parse path, empty string is false, `NaN`/`±Inf` float is false (not an error). (c) optional: non-empty slice/map truthiness.
- **02 — functions & fallbacks (2–3).** (a) `NewFunction` line-item total. (b) `WithFallback` safe-division / missing-rate → default; comment that the fallback evaluates over an EMPTY env and is compiled eagerly at construction. (c) `WithFallbackOnNil` + `WithFallbackObserver` to observe an otherwise-masked error (show the observer firing on the error case but NOT the nil case); optionally `WithReturnKind`.
- **03 — variables, strict env, host fns (2–3).** (a) `WithGlobals`/`WithLocals` as overridable `??` defaults (a threshold defaulted, overridden by runtime input); comment precedence (locals > globals) and merge (last-wins). (b) `WithEnv` strict: a field typo (`scoer`) compiles fine lenient but is a `*CompileError` under `WithEnv` — show both. (c) `WithFunction` host function (e.g. a `discountFor(tier)` helper or clock-free `businessDaysBetween`); comment last-registration-wins.
- **04 — exact decimal (2).** (a) invoice/loan money total with `decimal(...)` arithmetic + `roundBank` (banker's, half-even) vs `round` (half-away) on a `.5` case to show they differ. (b) comment + demonstrate the quirk: `decimal(x) * y` is exact everywhere (compile-time operator resolution), while bare-variable `principal * rate` needs `WithEnv` — show the `WithEnv` form working. Reuse `decimal_money_test.go`'s scenario, enriched.

**Steps:** (1) read old files; (2) `git rm examples/{config,decimal_money,eligibility,foreach_lineitem,lending,nested_foreach,pricing,strict_typing}_test.go`; (3) author 01–04 with teaching comments; (4) `go test ./examples/ -run 'Example' -v` and paste real output into each `// Output:`; (5) `go build ./... && go vet ./... && gofmt -l . && go test ./examples/ -race`; (6) commit `docs(examples): guided tour 01–04 (expr layer)` with the plan file, trailers `Spec: 030`/`Plan: 030`.

## Task 2 — `pipe` core (files 05–08)

**Files:** create `05_pipe_scope_getters_test.go`, `06_pipe_stages_test.go`, `07_pipe_decision_table_test.go`, `08_pipe_pipeline_concurrency_test.go`.

- **05 — scope & getters (2).** (a) `NewScope`+`Set`/`Get` dot-paths; `WithStrict` → `ErrPathConflict` on a duplicate leaf. (b) strict getters (`GetInt`/`GetFloat64`/`GetString`) vs coercing (`GetIntCoerce`): show a numeric string coercing, and an overflow/non-integral float producing a `*ScopeTypeError` (quirk: never silently truncates).
- **06 — single & multi stages (2).** (a) `SingleExpr` with `WithCondition` gate — show that a false condition writes NOTHING (the output path is absent), plus `WithOutput`. (b) `MultiExpr` priority ordering with intra-stage aliasing (an earlier named expr visible to a later one).
- **07 — decision tables (3).** (a) credit-tier single-match with `WithDefault` + firing (`ID`/`Message`, `FiringRule.IsDefault`) — reuse `lending_test.go`/`eligibility_test.go`. (b) collect + `AggregateSum`/list — adverse-action flags (list) and fee aggregation (sum); comment widest-kind promotion (int+decimal→decimal) and empty-fold behavior. (c) unique vs any — `HitPolicyUnique` → `ErrMultipleMatches` on overlap; `HitPolicyAny` agreement vs `ErrConflictingMatches`.
- **08 — pipeline DAG & concurrency (2).** (a) `NewPipeline` runs in dependency order despite out-of-order declaration; a mutual dependency → `*CycleError` with its concrete `.Cycle` path (`errors.As`). (b) `WithConcurrency`/`WithMaxParallel(n)` over independent stages yields identical output to sequential (determinism); `WithMaxParallel(0)` → `*InvalidMaxParallelError`.

**Steps:** author → run → fill Output → gate → commit `docs(examples): guided tour 05–08 (pipe core)`.

## Task 3 — `pipe` advanced (files 09–10)

**Files:** create `09_pipe_foreach_test.go`, `10_pipe_provenance_clock_test.go`.

- **09 — foreach (2).** (a) line-item adjudication: `ForEach` over collateral with an inner decision-table, `Rollup` summing an approved amount via a dot-path key, and `sc.FiringRulesFor("lines[2].check")` for per-element explainability — reuse `foreach_lineitem_test.go`. Comment the empty-collection quirk (Sum/Min/Max leave the key absent; Count=0). (b) nested foreach (order → tax line) with the composite firing key `orders[i].taxes[j].<table>`; mention `ErrForEachAsCollision` (distinct `as` per nesting chain) — reuse `nested_foreach_test.go`.
- **10 — provenance & clock (2).** (a) `WithProvenance` + `Explain`/`Lineage` tracing a decision back to seed inputs (reuse the eligibility explain), plus `Derivations`. (b) `WithClock` (a fixed clock) + `pipe.NowFunc` registered as `now()` for a deterministic temporal rule (`expires.Before(now())`), and `Scope.Duration`/`StageTimings` — reuse `pricing_test.go`'s clock usage.

**Steps:** author → run → fill Output → gate → commit `docs(examples): guided tour 09–10 (pipe foreach, provenance)`.

## Task 4 — `config` layer (files 11–12)

**Files:** create `11_config_rulesets_test.go`, `12_config_replay_test.go`.

- **11 — declarative rulesets (2–3).** (a) the same pricing pipeline authored in YAML (scalar shorthand) and JSON (object form) → identical result; comment JSON-requires-quoted-string shorthand — reuse `config_test.go`. (b) `Constants` with a `{$dec: "0.0725"}` rate + a strict `schema:` block that turns a field typo into a Build-time `*ConfigError` — reuse `strict_typing_test.go`'s intent at the config layer. (c) `Lint`/`WithLintErrors` catching a missing-default first-match table (comment the syntactic catch-all caveat).
- **12 — identity & replay (1–2).** `Hash()` (stable, excludes `Version`) + `MatchesRuleset`, `Scope.Ruleset` stamping, and a Scope JSON persist→reload→replay round-trip proving the same decision reproduces — adapt `config/ruleset_example_test.go`'s scenario into the examples package.

**Steps:** author → run → fill Output → gate → commit `docs(examples): guided tour 11–12 (config)`.

## Task 5 — `rlng` engine layer (files 13–14)

**Files:** create `13_engine_untyped_test.go`, `14_engine_typed_test.go`.

- **13 — untyped engine (2).** (a) `rlng.New(pipeline)` + `Evaluate`→map and `EvaluateScope`→`*Scope`; `WithScopeOptions(pipe.WithProvenance())` then `Explain` off the returned scope. (b) `rlng.NewFromYAML(ctx, yaml)` one-call parse+build+engine — reuse `fromconfig_example_test.go`'s intent.
- **14 — typed engine & mapping (2).** (a) `TypedEngine[Input,Result]` with `NewMapper[Result](MappingTemplate{...})` projecting scope paths into a typed struct, including a decimal field that survives struct→seed→result (restore + exact narrowing); show the lossy-narrowing error when a fractional decimal maps to an int field (`errors.Is`/`*MappingError`). (b) `ErrConcurrencyRequiresConfig`: passing `rlng.WithConcurrency()` to `rlng.New` errors, but works via `NewFromYAML(..., rlng.WithConcurrency())` — a teaching point on why concurrency is a build-time property.

**Steps:** author → run → fill Output → gate → commit `docs(examples): guided tour 13–14 (engine)`.

## Task 6 — `doc.go` index + README `Usage` rewrite

**Files:** `examples/doc.go`, `README.md`.

- **doc.go:** reframe the package comment as an ordered guided tour and list the 14 topics as a one-line index (file → topic), so `go doc ./examples` reads as a curriculum. Keep it exported-nothing.
- **README `Usage`:** reorder simplest→complex to mirror the tour — expr predicate/function (one-off) → variables/strict env → decimal → pipe stages → decision tables → foreach → provenance → config rulesets → replay → untyped engine → typed engine. Each subsection: one intro sentence + one compact snippet + one quirk line + `see examples/NN_*_test.go`. Keep it concise (the examples carry depth). Fix every stale example-file reference (old `examples/decimal_money_test.go` etc. → new numbered files). Verify snippet code still reflects the current API.

**Steps:** rewrite both; `go build ./... && go test ./examples/ -race` (unchanged, green); confirm README code blocks compile-conceptually and file links resolve; `gofmt -l .` clean; commit `docs(examples): tour index + README Usage rewrite`.

## Final gate (after Task 6, before merge)

- `go build ./... && go vet ./... && gofmt -l .` (empty) and `go test ./... -race` green.
- A whole-branch `/code-review` pass focused on: example correctness (Output matches, no flaky map-order
  output), teaching-comment quality, real-world/quirk coverage vs the Spec 030 table, README accuracy and
  concision, and confirmation that NO non-test library file changed. Resolve/triage findings.
- Update `docs/HANDOVER.md`. Then merge to main + push origin/main + delete branch (authorized this session).

## Self-Review

- **Coverage vs Spec 030 table:** every one of the 14 files is produced by Tasks 1–5; every capability/quirk
  column has a home. doc.go + README in Task 6.
- **No placeholders:** each task names concrete scenarios, capabilities, quirks, API, and the reuse source.
  Output blocks are intentionally not pre-written — they are obtained by running (a hard requirement, not a gap).
- **Naming consistency:** file scheme `NN_<layer>_<topic>_test.go`; lowercase-first descriptive Example
  suffixes; `package examples`; verified API names in the reference block above.
