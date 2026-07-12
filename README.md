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

- **Declarative config** — YAML/JSON rule definitions loaded via pluggable sources.
- **Staged evaluation** — stages (single-expression, multi-expression, and decision-table)
  are ordered by their declared dependencies (a topologically sorted DAG).
- **Decision tables** — ordered `condition → decisions` rules, in first-match or
  collect-all modes.
- **Typed result mapping** — the evaluated context is projected into a caller-supplied Go
  type via a mapping template.

See [`CLAUDE.md`](./CLAUDE.md) for the full architecture blueprint and contributor workflow.

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
	def, _ := config.ParseYAML([]byte(rules)) // parse declarative rules
	pipeline, _ := def.Build()                // compile + topo-sort (cycle-checked)

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
pipeline, _ := pipe.NewPipeline(base, taxed)

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
p, _ := pipe.NewPipeline(base, calc)

sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2})
_ = p.Run(context.Background(), sc)

taxed, _ := sc.GetFloat64("calc.taxed") // 22.0
dur, _ := sc.Duration()                 // evaluation duration
blob, _ := json.Marshal(sc)             // persist; json.Unmarshal restores it losslessly
```

### Decision tables & provenance

Ordered `condition → decisions` rules, first match wins. With `WithProvenance`,
`Explain` traces a result back through the expressions to its seed inputs:

```go
grade, _ := pipe.NewDecisionTable("grade", []pipe.Rule{
	{Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`, "limit": "score * 100"}},
	{Condition: "score >= 650", Decisions: map[string]string{"tier": `"near_prime"`, "limit": "score * 50"}},
	{Condition: "true", Decisions: map[string]string{"tier": `"subprime"`, "limit": "score * 10"}},
})
p, _ := pipe.NewPipeline(grade)

sc := pipe.NewScope(map[string]any{"score": 700}, pipe.WithProvenance())
_ = p.Run(context.Background(), sc)

tier, _ := sc.GetString("grade.tier") // near_prime
fmt.Print(sc.Explain("grade.limit"))
// grade.limit = 35000 [grade decision-table] expr: score * 50
//   score = 700 [seed]
```

`WithHitPolicy(pipe.HitPolicyCollect)` accumulates every matching rule's decisions into a slice:

```go
checks, _ := pipe.NewDecisionTable("checks", []pipe.Rule{
	{Condition: "score < 650", Decisions: map[string]string{"flag": `"low_score"`}},
	{Condition: "dti > 0.4", Decisions: map[string]string{"flag": `"high_dti"`}},
}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
// ...run over {"score": 600, "dti": 0.5}...
flags, _ := sc.GetSlice("checks.flag") // [low_score high_dti]
```

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
