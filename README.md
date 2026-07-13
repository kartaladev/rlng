# rlng

A **pure-Go rule engine library**, built on top of [expr-lang/expr](https://github.com/expr-lang/expr).

`rlng` compiles declarative rule / expression definitions (YAML or JSON) and evaluates
them against runtime input. It is the *engine* only — not a Business Rules Management
System: no authoring UI, governance, or persistence, just fast, embeddable evaluation.

> **Status: unreleased (pre-`v0.0.1`).** All five build increments are implemented and
> merged — the `expr`, `pipe`, and `config` packages and the root `rlng` facade are
> complete, tested (`-race`), and lint-clean. The exported API is stable but **not yet
> tagged**; pin to a commit until `v0.0.1` is cut. APIs may still change before the first
> tag. See [`CLAUDE.md`](./CLAUDE.md) for the architecture and contributor workflow.

## Why another rule engine?

The main alternative, [gorules/zen-go](https://github.com/gorules/zen-go), binds to a Rust
engine via **cgo**, which makes it hard to debug. `rlng`'s guiding constraint is the
opposite: **pure Go, no cgo** — so you can set a breakpoint, step through evaluation, read a
plain Go stack trace, and get typed errors that name the offending field and expression.
It also stays trivially cross-compilable (`CGO_ENABLED=0`).

## Design at a glance

Rules are declared as config and compiled once, then evaluated on the hot path:

- **Declarative config** — YAML/JSON rule definitions loaded via pluggable sources, with
  pipeline-level `constants` and an output `mapping` block so a whole decision service is
  one document. Opt-in strict mode via a `schema` block catches field typos and type errors
  at build time; `WithStrict()` / `WithSchema()` enforce or supply schema programmatically;
  `WithLintErrors()` promotes static checks (missing defaults, unreachable rules) to build errors.
- **Staged evaluation** — stages (single-expression, multi-expression, and decision-table)
  are ordered by their declared dependencies (a topologically sorted DAG).
- **Decision tables** — ordered `condition → decisions` rules with hit policies
  **single / unique / any / collect**, a per-table **default (else)** branch, and collect
  **aggregation** (`sum`/`min`/`max`/`count`/list).
- **Exact-decimal money & value fidelity** — an in-expression `decimal` type (built on
  [shopspring/decimal](https://github.com/shopspring/decimal)) with arithmetic operator
  overloads and `round`/`roundBank` (banker's) rounding, so `$250,000 × 7.25%` is exactly
  `$18,125.00`, not the `18124.999…` a `float64` produces. A value keeps its **type and
  precision at every serde boundary**: declarable as a config constant (`{"$dec": "0.0725"}`),
  preserved through the struct/map seed, numerically-exact in aggregation (integer sums stay
  `int64` with checked overflow; no float round-trip), and — via a **canonically type-tagged
  Scope JSON** (`"v":2`, backward-compatible reads) — reloaded as the *same kind* so a
  persisted decision replays losslessly.
- **Explainable decisions** — optional rule `id`/`message`, a recorded firing trail per
  stage (`FiringRule` for the first/only match, `FiringRulesFor` for every rule a
  **collect**/**any** table matched), value **provenance/lineage**, and **per-stage timing**.
- **Ruleset identity & replay** — a deterministic content `Hash()` plus an author
  `version` label stamped onto every `Scope` (`Scope.Ruleset()`), and a `MatchesRuleset`
  replay-safety check, so a persisted decision round-trips as a self-describing,
  replayable record (which ruleset produced it, and whether a candidate ruleset matches).
- **Strict typed evaluation** — opt-in `expr.WithEnv` rejects field typos and type errors
  at compile time instead of silently evaluating to nil.
- **Extensible** — register host functions (`expr.WithFunction`), including a clock-backed
  `now()` for deterministic temporal rules.
- **Ruleset lint** — static checks for unreachable rules and missing-default coverage gaps.
- **Typed result mapping** — the evaluated context is projected into a caller-supplied Go
  type via a mapping template.

See [`CLAUDE.md`](./CLAUDE.md) for the full architecture blueprint and contributor workflow,
and [`examples/`](./examples) for runnable end-to-end examples.

## Usage

`rlng` exposes a typed facade (root `rlng` package) plus the building blocks
(`expr`, `pipe`, `config`) used directly. Each subsection below mirrors one
stop on the runnable, numbered tour in [`examples/`](./examples) — start
simple (a single expression) and build up to a full typed engine. The
snippets are illustrative distillations, not verbatim copies; run
`go test ./examples/ -v` for the real, working code.

### 1. One-off expressions — `expr`

Compile a predicate or value expression once and evaluate it against any env — no pipeline needed for a single rule:

```go
gate, _ := expr.NewPredicate("age >= 18 && verified")
ok, _ := gate.Test(map[string]any{"age": 34, "verified": true}) // true

total, _ := expr.NewFunction("total", "price * qty")
v, _ := total.Apply(map[string]any{"price": 10, "qty": 3}) // 30
```

Quirk: a predicate that evaluates to a non-bool value fails loudly (`*expr.EvalError` wrapping
`expr.ErrNotBool`) instead of silently coercing; opt into lenient truthiness with `expr.WithCoerce()`.
See [`examples/01_expr_predicates_test.go`](./examples/01_expr_predicates_test.go) and
[`examples/02_expr_functions_test.go`](./examples/02_expr_functions_test.go).

### 2. Variable defaults & strict typing

`WithGlobals`/`WithLocals` compile in `x ?? <default>` fallbacks that runtime input always
overrides; `WithEnv` turns on strict type-checking so a field typo fails at compile time:

```go
gate, _ := expr.NewPredicate("score >= minScore", expr.WithGlobals(map[string]any{"minScore": 650}))
ok, _ := gate.Test(map[string]any{"score": 680}) // true — no override needed

_, err := expr.NewPredicate("scoer >= 650", expr.WithEnv(map[string]any{"score": 0}))
// err is a *expr.CompileError: "scoer" isn't in the declared env
```

Quirk: locals win over globals; without `WithEnv`, an undefined name is tolerated as always-nil forever.
See [`examples/03_expr_variables_env_test.go`](./examples/03_expr_variables_env_test.go).

### 3. Exact-decimal money

Every compiled expression has a `decimal(x)` constructor, decimal-aware arithmetic operators, and
`round`/`roundBank` builtins always available, so `$250,000 × 7.25%` is exactly `$18,125.00`:

```go
fee, _ := expr.NewFunction("fee", "roundBank(decimal(principal) * decimal(rate), 2)")
v, _ := fee.Apply(map[string]any{"principal": 250_000, "rate": "0.0725"})
fmt.Println(v) // 18125.00 — not 18124.999999999996
```

Quirk: `decimal(...)`-wrapped operands are exact in any mode; a *bare* `decimal.Decimal` variable
(`principal * rate` with no wrapper) needs `expr.WithEnv` so the compiler can see it's decimal.
See [`examples/04_expr_decimal_test.go`](./examples/04_expr_decimal_test.go).

### 4. Pipe stages & scope

A `Scope` is the `map[string]any` accumulator threaded through stage evaluation; `SingleExpr`/
`MultiExpr` stages read and write it by dot path, and typed getters come in strict and coercing flavors:

```go
loyalty, _ := pipe.NewSingleExpr("loyaltyDiscount", "0.10",
	pipe.WithCondition(`tier == "gold"`), pipe.WithOutput("pricing.discountRate"))
pipeline, _ := pipe.NewPipeline([]pipe.Stage{loyalty})

sc := pipe.NewScope(map[string]any{"tier": "gold"})
_ = pipeline.Run(context.Background(), sc)
rate, _ := sc.GetFloat64("pricing.discountRate") // 0.1
```

Quirk: a false `WithCondition` is a no-op — the output path is left absent, not written as `false`/`0`.
See [`examples/05_pipe_scope_getters_test.go`](./examples/05_pipe_scope_getters_test.go) and
[`examples/06_pipe_stages_test.go`](./examples/06_pipe_stages_test.go).

### 5. Decision tables

An ordered set of `condition → decisions` rules: `HitPolicySingle` (default) takes the first
match, `HitPolicyCollect` accumulates every match with an aggregation, and `WithDefault` makes
"no match" an explicit outcome:

```go
grade, _ := pipe.NewDecisionTable("grade", []pipe.Rule{
	{ID: "PRIME", Condition: "score >= 750", Decisions: map[string]pipe.Decision{
		"tier": {Expr: `"prime"`}, "limit": {Expr: "score * 100"},
	}},
}, pipe.WithDefault(map[string]pipe.Decision{"tier": {Expr: `"declined"`}, "limit": {Expr: "0"}}))
p, _ := pipe.NewPipeline([]pipe.Stage{grade})

sc := pipe.NewScope(map[string]any{"score": 780})
_ = p.Run(context.Background(), sc)
tier, _ := sc.GetString("grade.tier") // prime
fr, _ := sc.FiringRule("grade")       // fr.RuleID == "PRIME"
```

Quirk: `HitPolicyUnique`/`HitPolicyAny` guard against silently picking a winner — overlapping
matches surface as `ErrMultipleMatches`/`ErrConflictingMatches` instead of the first match winning.
See [`examples/07_pipe_decision_table_test.go`](./examples/07_pipe_decision_table_test.go).

### 6. Line-item adjudication (`foreach`)

A `foreach` stage runs an inner pipeline once per collection element, then optionally rolls a
per-element field up into a header value via `rollups:`:

```yaml
stages:
  - name: lines
    type: foreach
    collection: collateral
    as: item
    stages:
      - name: check
        type: decision-table
        rules:
          - id: LTV_MAX_80
            condition: item.ltv > 80
            decisions: {status: '"denied"'}
        default: {status: '"approved"'}
    rollups:
      - {key: approved, agg: sum, as: totalApproved}
```

```go
firings := sc.FiringRulesFor("lines[2].check") // which rule decided line 3
```

Quirk: rolling up an empty collection leaves a `sum`/`min`/`max` rollup key absent (no defined
answer), while `count` still writes an explicit `0`. Nesting composes (`foreach` inside `foreach`).
See [`examples/09_pipe_foreach_test.go`](./examples/09_pipe_foreach_test.go).

### 7. Provenance / explainability

`pipe.WithProvenance()` makes a `Scope` record a `Derivation` for every seed input and stage
write, so `Explain`/`Lineage` can trace a result back to the inputs that produced it:

```go
sc := pipe.NewScope(map[string]any{"applicant": map[string]any{"score": 700}}, pipe.WithProvenance())
_ = pipeline.Run(context.Background(), sc)
fmt.Print(sc.Explain("grade.limit"))
// grade.limit = 35000 [grade decision-table] expr: applicant.score * 50
//   applicant = map[score:700] [seed]
```

Quirk: `WithClock` + `pipe.NowFunc` inject a fixed clock as an expr `now()` host function, so a
temporal rule and `Scope.Duration`/`StageTimings` stay deterministic under test.
See [`examples/10_pipe_provenance_clock_test.go`](./examples/10_pipe_provenance_clock_test.go).

### 8. Declarative config rulesets

Everything above can be authored as one YAML or JSON document and turned into a `*pipe.Pipeline`
via `config.Parse` + `PipelineDef.Build` — the shape a rule actually ships in:

```go
def, _ := config.Parse(context.Background(), config.FromYAMLString(`
schema:
  principal: 0
constants:
  aprRate: {$dec: "0.0725"}
stages:
  - name: interest
    type: single-expr
    expr: decimal(principal) * decimal(aprRate)
`))
pipeline, _ := def.Build()
```

Quirk: a top-level `schema:` block auto-enables strict compilation for every stage (a field typo
becomes a `*config.ConfigError` at `Build`); `Lint()`/`WithLintErrors()` catch missing-default and
unreachable-rule smells.
See [`examples/11_config_rulesets_test.go`](./examples/11_config_rulesets_test.go).

### 9. Ruleset identity & replay

`PipelineDef.Hash()` is a deterministic content fingerprint that `Build` stamps onto every
`Scope` it runs, so a persisted decision carries proof of exactly which ruleset produced it:

```go
sc := pipe.NewScope(map[string]any{"claimsScore": 85})
_ = pipeline.Run(context.Background(), sc)
b, _ := json.Marshal(sc) // persist; json.Unmarshal restores it losslessly

reloaded := &pipe.Scope{}
_ = json.Unmarshal(b, reloaded)
id, _ := reloaded.Ruleset()
def.MatchesRuleset(id) // true iff def is still the ruleset that produced this decision
```

Quirk: `Hash()` deliberately excludes the author's `version:` label — relabeling a release with
no rule change leaves the hash unchanged.
See [`examples/12_config_replay_test.go`](./examples/12_config_replay_test.go).

### 10. Untyped engine — `Engine`

`rlng.Engine` wraps a built `*pipe.Pipeline` and owns the per-call boilerplate — seed a `Scope`,
`Run`, hand back a result:

```go
engine, _ := rlng.New(pipeline, rlng.WithScopeOptions(pipe.WithProvenance()))
out, _ := engine.Evaluate(context.Background(), map[string]any{"score": 720, "balance": 1200, "limit": 5000})
// out["offer"].(map[string]any)["apr"]

engine2, _ := rlng.NewFromYAML(context.Background(), yamlDoc) // parse + build + New, one call
```

Quirk: `Evaluate` returns the plain map; `EvaluateScope` returns the live `*pipe.Scope` for when
you need timing, JSON persistence, or `Explain`.
See [`examples/13_engine_untyped_test.go`](./examples/13_engine_untyped_test.go).

### 11. Typed engine → typed result

`rlng.TypedEngine[I, R]` pairs an `Engine` with a `Mapper[R]`, so `Evaluate` takes a typed struct
in and returns a typed struct out — a `decimal.Decimal` field survives both boundaries exactly:

```go
mapper, _ := rlng.NewMapper[LoanQuote](rlng.MappingTemplate{"fee": "fee"})
engine, _ := rlng.NewTypedEngine[LoanApplication, LoanQuote](pipeline, mapper)

quote, _ := engine.Evaluate(context.Background(), LoanApplication{
	Principal: decimal.NewFromInt(1_000),
	Rate:      decimal.RequireFromString("0.0725"),
})
fmt.Println(quote.Fee.StringFixed(2)) // 72.50
```

Quirk: mapping a fractional decimal into an `int` result field is refused, not truncated
(`*rlng.MappingError` wrapping `rlng.ErrLossyResultNarrowing`); `rlng.WithConcurrency()` only
works via the config-driven constructors (`NewFromYAML`/`NewFromProvider`) since concurrency is
baked in at `Build` time, not at `rlng.New`.
See [`examples/14_engine_typed_test.go`](./examples/14_engine_typed_test.go).

Runnable versions of every snippet live in [`examples/`](./examples) (start at
[`examples/doc.go`](./examples/doc.go) for the full index) and the package `Example` tests —
run them with `go test ./...`.

## Installation

Not yet released. Once the module is published and a `vX.Y.Z` tag is cut:

```bash
go get github.com/kartaladev/rlng@latest
```

Requires **Go 1.25+**.

## Development

Contributor conventions — TDD, the skill-driven workflow, documentation/ADR requirements,
commit discipline, and the library quality gates — are documented in
[`CLAUDE.md`](./CLAUDE.md). Common commands:

```bash
go build ./...
go test ./... -race
go vet ./...
golangci-lint run ./...
govulncheck ./...
```

## Releases

Releases are **tag-driven and SemVer'd**. Pushing an annotated tag `vX.Y.Z` (e.g. `v0.0.1`)
triggers the [`release`](./.github/workflows/release.yml) GitHub Action, which publishes a
**GitHub Release** with auto-generated notes. As a library there are no binaries to
build — the tag itself is the distribution. Consumers pin a version with
`go get github.com/kartaladev/rlng@vX.Y.Z`. Breaking changes to the exported API require a
major-version bump and a recorded ADR.

## License

Licensed under the [Apache License 2.0](./LICENSE).
