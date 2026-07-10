# rlng

A **pure-Go rule engine library**, built on top of [expr-lang/expr](https://github.com/expr-lang/expr).

`rlng` compiles declarative rule / expression definitions (YAML or JSON) and evaluates
them against runtime input. It is the *engine* only — not a Business Rules Management
System: no authoring UI, governance, or persistence, just fast, embeddable evaluation.

> **Status: early development.** The repository is bootstrapped but the Go module and
> source are not written yet. There is nothing to `go get` at this time. This README and
> [`CLAUDE.md`](./CLAUDE.md) describe the intended design; APIs below are illustrative and
> subject to change until the first tagged release.

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
