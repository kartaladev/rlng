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
(`expr`, `pipe`, `config`) used directly. The snippets below are distilled from
the runnable [`examples/`](./examples) and the package `Example` tests.

### Declarative pipeline → typed result

Declare a pipeline as config, build it, and evaluate a typed input into a typed result:

```go
package main

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
)

type Input struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type Quote struct {
	Total float64 `mapstructure:"total"`
}

const rules = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`

func main() {
	def, _ := config.Parse(context.Background(), config.FromYAMLString(rules)) // parse declarative rules
	pipeline, _ := def.Build()                                                 // compile + topo-sort (cycle-checked)

	mapper, _ := rlng.NewMapper[Quote](rlng.MappingTemplate{"total": "taxed"})
	engine, _ := rlng.NewTypedEngine[Input, Quote](pipeline, mapper)

	q, err := engine.Evaluate(context.Background(), Input{Price: 10, Qty: 2})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%.1f\n", q.Total) // 22.0
}
```

### Build a pipeline in Go (no config)

Skip config and wire stages directly; `depends_on` still orders the DAG:

```go
base, _ := pipe.NewSingleExpr("base", "price * qty")
taxed, _ := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))
pipeline, _ := pipe.NewPipeline([]pipe.Stage{base, taxed})

mapper, _ := rlng.NewMapper[Quote](rlng.MappingTemplate{"total": "taxed"})
engine, _ := rlng.NewTypedEngine[Input, Quote](pipeline, mapper)

q, _ := engine.Evaluate(context.Background(), Input{Price: 10, Qty: 2})
fmt.Printf("%.1f\n", q.Total) // 22.0
```

### Raw `map[string]any` output — `Engine`

When you don't need a typed result, `Engine` returns the accumulated map:

```go
pipeline, _ := def.Build() // from config, or built in Go
engine, _ := rlng.New(pipeline)

out, _ := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"]) // base=20 taxed=22.0
```

### Multi-expression stage, typed getters, timing & JSON

A `multi-expr` stage computes several named results. The `Scope` offers typed
getters, evaluation timing, and a round-trippable JSON codec (e.g. a `jsonb` column):

```go
base, _ := pipe.NewSingleExpr("base", "price * qty")
calc, _ := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
	{Name: "taxed", Expression: "base * 1.1"},
	{Name: "discounted", Expression: "base * 0.9"},
}, pipe.WithDependsOn("base"))
p, _ := pipe.NewPipeline([]pipe.Stage{base, calc})

sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2})
_ = p.Run(context.Background(), sc)

taxed, _ := sc.GetFloat64("calc.taxed") // 22.0
dur, _ := sc.Duration()                 // evaluation duration
blob, _ := json.Marshal(sc)             // persist; json.Unmarshal restores it losslessly
```

The JSON codec is **kind-preserving** (`"v":2` type tagging): an `int64`, `decimal`, `time`,
`float64`, etc. reloads as the same Go type, so a persisted decision reproduces the same
result on replay. Legacy (pre-tagging) blobs still load.

### Exact-decimal money

Money and rate math is exact end to end. Declare the rate once as a decimal constant, seed the
principal as a decimal, keep the arithmetic decimal via `decimal(...)`, and round deterministically
with `roundBank` — the fee survives a JSON round-trip and maps into a typed result unchanged:

```go
const rules = `
constants:
  rate: {$dec: "0.0725"}
stages:
  - name: loan
    type: single-expr
    expr: roundBank(decimal(principal) * decimal(rate), 2)
`
def, _ := config.Parse(context.Background(), config.FromYAMLString(rules))
pipeline, _ := def.Build()
engine, _ := rlng.New(pipeline)

scope, _ := engine.EvaluateScope(context.Background(), map[string]any{
	"principal": decimal.NewFromInt(250000),
})
fee, _ := pipe.GetAs[decimal.Decimal](scope, "loan")
fmt.Println(fee.StringFixed(2)) // 18125.00 — exact, not 18124.999999999996
```

Bare-variable decimal arithmetic (`principal * rate` without `decimal(...)`) resolves only under
strict-env mode (`expr.WithEnv`), where the type-checker knows the operands are decimals; the
`decimal(...)`-wrapped form above works everywhere. See
[`examples/decimal_money_test.go`](./examples/decimal_money_test.go) for the full seed → evaluate →
JSON round-trip → map flow.

### Decision tables & provenance

Ordered `condition → decisions` rules, first match wins. With `WithProvenance`,
`Explain` traces a result back through the expressions to its seed inputs:

```go
grade, _ := pipe.NewDecisionTable("grade", []pipe.Rule{
	{Condition: "score >= 750", Decisions: map[string]pipe.Decision{"tier": {Expr: `"prime"`}, "limit": {Expr: "score * 100"}}},
	{Condition: "score >= 650", Decisions: map[string]pipe.Decision{"tier": {Expr: `"near_prime"`}, "limit": {Expr: "score * 50"}}},
	{Condition: "true", Decisions: map[string]pipe.Decision{"tier": {Expr: `"subprime"`}, "limit": {Expr: "score * 10"}}},
})
p, _ := pipe.NewPipeline([]pipe.Stage{grade})

sc := pipe.NewScope(map[string]any{"score": 700}, pipe.WithProvenance())
_ = p.Run(context.Background(), sc)

tier, _ := sc.GetString("grade.tier") // near_prime
fmt.Print(sc.Explain("grade.limit"))
// grade.limit = 35000 [grade decision-table] expr: score * 50
//   score = 700 [seed]
```

Each `Decision` carries its own options, so one output can declare a `fallback`
(or `globals`/`coerce`) that a sibling output does not — e.g.
`{"limit": {Expr: "score * f", Options: []expr.Option{expr.WithFallback("0")}}}`
recovers `limit` to `0` on a value error while a fallback-less `tier` still fails.

`WithHitPolicy(pipe.HitPolicyCollect)` accumulates every matching rule's decisions into a slice:

```go
checks, _ := pipe.NewDecisionTable("checks", []pipe.Rule{
	{Condition: "score < 650", Decisions: map[string]pipe.Decision{"flag": {Expr: `"low_score"`}}},
	{Condition: "dti > 0.4", Decisions: map[string]pipe.Decision{"flag": {Expr: `"high_dti"`}}},
}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
// ...run over {"score": 600, "dti": 0.5}...
flags, _ := sc.GetSlice("checks.flag") // [low_score high_dti]
```

### Line-item adjudication (`foreach`)

A `foreach` stage runs an inner pipeline once per element of a collection,
each against its own per-element scope (the element bound under `as`, default
`item`). It writes structured per-element output to `<stage>.items`,
optionally rolls a per-element key up into a header value (`rollups:`, same
decimal/int64-faithful aggregations as decision-table `collect`), and records
each element's firing rules under `<stage>[i]` — so "line 3 was denied by rule
LTV_MAX_80" is answerable directly:

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
      - key: approved
        agg: sum
        as: totalApproved
```

```go
firings := sc.FiringRulesFor("lines[2]") // rule that decided line 3
```

Nested `foreach` (a `foreach` inside a `foreach`'s inner `stages:`) is
rejected at build time — see `examples/foreach_lineitem_test.go` for a
full runnable adjudication.

### One-off expressions — `expr`

Compile and evaluate a single value expression or boolean predicate directly:

```go
f, _ := expr.NewFunction("total", "price * qty")
v, _ := f.Apply(map[string]any{"price": 10, "qty": 3}) // 30

p, _ := expr.NewPredicate("amount > threshold", expr.WithGlobals(map[string]any{"threshold": 100}))
ok, _ := p.Test(map[string]any{"amount": 150}) // true
```

Runnable versions of every snippet live in [`examples/`](./examples) and the
package `Example` tests — run them with `go test ./...`.

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
