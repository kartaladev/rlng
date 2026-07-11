# rlng

A **pure-Go rule engine library**, built on top of [expr-lang/expr](https://github.com/expr-lang/expr).

`rlng` compiles declarative rule / expression definitions (YAML or JSON) and evaluates
them against runtime input. It is the *engine* only — not a Business Rules Management
System: no authoring UI, governance, or persistence, just fast, embeddable evaluation.

> **Status: unreleased (pre-`v0.0.1`).** All five build increments are implemented and
> merged — the `expr`, `stage`, and `config` packages and the root `rlng` facade are
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
	engine := rlng.New[Input, Quote](pipeline, mapper)

	q, err := engine.Evaluate(context.Background(), Input{Price: 10, Qty: 2})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%.1f\n", q.Total) // 22.0
}
```

The packages are also usable independently: `expr` (compile/evaluate one expression),
`stage` (build stages and a `Pipeline` in Go), and `config` (load definitions). The root
`rlng` package is the end-to-end facade. Every exported behavior has a runnable `Example`
test that doubles as godoc.

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
