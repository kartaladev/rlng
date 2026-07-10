# Spec 005 — Result mapper + `Engine[I, R]` facade

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-11
- **Increment:** 5 of 5 — **final** (see [Roadmap](#roadmap-position))
- **Builds on:** Spec 001 (`expr` — `Function`, `Option`, typed errors), Spec 002 (`stage.Scope`, stage types), Spec 003 (`stage.Pipeline`, `NewPipeline`, `Run`), Spec 004 (`config` — parse/Build a `*stage.Pipeline`).
- **Realized by plans:** `docs/plans/005-engine-facade.md`
- **Related ADRs:** ADR-0001 (ratifies the `Engine`/`Evaluate` naming — accepted in Increment 1, realized here), ADR-0009 (Engine/Mapper design + root-package placement), ADR-0010 (mapstructure dependency) — recorded with the implementation commits.

## Context

`rlng` is a pure-Go rule + calculation engine on [`expr-lang/expr`](https://github.com/expr-lang/expr), built for debuggability. Increments 1–4 delivered the evaluation stack: atomic evaluators (`expr`), stages + `Scope` (`stage`), the `Pipeline` DAG orchestrator (`stage`), and declarative config loading (`config`). What remains is the **public facade** that a consumer actually imports: a generic **`Engine[I, R]`** that takes a typed input, runs a pipeline, and returns a typed result — plus the **result `Mapper[R]`** that projects the accumulated `Scope` into `R`. This is the product's front door and the last increment; after it the library is feature-complete for a first `v0.0.x` tag.

### Roadmap position

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | **Done** (merged) |
| 2 | Scope + stages | **Done** (merged) |
| 3 | Stage DAG orchestration (`Pipeline`) | **Done** (merged) |
| 4 | Declarative config (`config/`) | **Done** (merged) |
| **5** | **Result mapper + `Engine[I, R]` facade** *(this spec)* | **This increment — final** |

**Scope boundary:** this increment ships the **root `rlng` facade**: `Engine[I, R]`, `Mapper[R]`, `MappingTemplate`, and typed input seeding + result mapping. The `Engine` is composed from an already-built `*stage.Pipeline` (which the caller obtains programmatically or via `config`) and a `Mapper[R]`. It does **not** couple the root package to `config` (they stay siblings over `stage`), and it does **not** add config-declared output mapping or `VariablePatcher` defaults (backlog).

## Goals

1. **`Engine[I, R]`** (generic over input `I` and result `R`): construct once from a `*stage.Pipeline` + a `*Mapper[R]`; `Evaluate(ctx, I) (R, error)` seeds a `Scope` from the input, runs the pipeline, and maps the final `Scope` into `R`.
2. **Input seeding** of the `Scope` from `I`: a `map[string]any` is used directly; a struct is flattened to `map[string]any` (type-faithfully) via mapstructure.
3. **`Mapper[R]` + `MappingTemplate`**: a `MappingTemplate` is `map[string]string` mapping an **output dot-path** to a **leaf expression** evaluated against the final `Scope`. `Map` evaluates each field, assembles a nested `map[string]any`, and decodes it into `R` (a struct via mapstructure tags, or `map[string]any` directly).
4. **Typed `MappingError`** naming the offending output field and unwrapping to the underlying `expr`/decode error — extending the debuggability chain to the mapping layer.
5. Ratify the **`Engine`/`Evaluate`** naming (ADR-0001) by implementing it; keep a small, stable exported surface ("accept interfaces / concrete pipelines, return structs").

## Non-goals (deferred)

- **Config-declared output mapping** — the `MappingTemplate` is programmatic this increment; loading it from YAML/JSON is a backlog item. `config` builds the pipeline; the mapping is Go.
- **Coupling `rlng` → `config`** — the root package composes a caller-supplied `*stage.Pipeline`; the consumer wires `config.ParseYAML(...).Build()` themselves.
- **`VariablePatcher` defaults** (`x ?? <literal>` injection) — still deferred; `expr`/`stage` globals cover declared variables.
- **Concurrency** — `Evaluate` runs the pipeline sequentially (ADR-0006); `Engine` holds no mutable state post-construction, so distinct `Evaluate` calls are safe to run concurrently.

## Design

### Package layout

```
github.com/kartaladev/rlng/         # root package "rlng" — this increment
  engine.go                         # Engine[I, R], New, Evaluate, input seeding
  mapper.go                         # Mapper[R], MappingTemplate, NewMapper, Map
  errors.go                         # MappingError
  doc.go
  *_test.go, example_test.go
```

Root `rlng` imports `stage`, `expr`, and `github.com/go-viper/mapstructure/v2`. It does **not** import `config`.

### `Engine[I, R]` (`engine.go`)

```go
// Engine evaluates a typed input I against a compiled pipeline and maps the
// result into a typed R. It is safe for concurrent use after construction.
type Engine[I any, R any] struct {
	pipeline  *stage.Pipeline
	mapper    *Mapper[R]
	scopeOpts []stage.ScopeOption
}

// New constructs an Engine from a compiled pipeline and a result mapper.
func New[I any, R any](pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option) *Engine[I, R]

// Option configures an Engine (e.g. WithScopeOptions to pass stage.WithStrict).
type Option func(*engineConfig)
func WithScopeOptions(opts ...stage.ScopeOption) Option

// Evaluate seeds a Scope from input, runs the pipeline, and maps the final
// Scope into R. A nil pipeline error is required before mapping; the pipeline's
// typed errors (StageError, etc.) pass through unwrapped.
func (e *Engine[I, R]) Evaluate(ctx context.Context, input I) (R, error)
```

`Evaluate` steps:
1. `seed, err := flatten(input)` — if `I` is already `map[string]any`, use it directly; otherwise mapstructure-decode the struct into a `map[string]any` (type-faithful: an `int` field stays an `int`, which matters for `expr` integer semantics like `%`). On failure return the zero `R` and the wrapped error.
2. `sc := stage.NewScope(seed, e.scopeOpts...)`.
3. `if err := e.pipeline.Run(ctx, sc); err != nil` → return zero `R`, err (unwrapped pipeline/stage error).
4. `return e.mapper.Map(sc.Snapshot())`.

### `Mapper[R]` + `MappingTemplate` (`mapper.go`)

```go
// MappingTemplate maps an output dot-path to a leaf expression evaluated against
// the final Scope. Example: {"total": "line.net + line.tax", "info.tag": "tiers.tag"}.
type MappingTemplate map[string]string

// Mapper projects a Scope into a typed R by evaluating each template field and
// decoding the assembled nested map into R.
type Mapper[R any] struct {
	fields []mappedField // compiled: dot-path + *expr.Function
}

// NewMapper compiles each template field's expression up front. A compile error
// is a *MappingError naming the field.
func NewMapper[R any](tmpl MappingTemplate) (*Mapper[R], error)

// Map evaluates each field against scope, assembles a nested map[string]any by
// dot-path, and decodes it into R. Eval and decode errors are *MappingError.
func (m *Mapper[R]) Map(scope map[string]any) (R, error)
```

- **Compile once.** `NewMapper` compiles each field expression to an `*expr.Function` (reusing Increment 1). Fields are stored sorted by dot-path for deterministic evaluation/assembly order.
- **`Map`** evaluates each field's `Function.Apply(scope)`, then `setNested(out, dotPath, value)` builds a nested `map[string]any` (a small local dot-path setter, mirroring `Scope.Set`'s descent). It then decodes `out` into `R` with mapstructure. When `R` is `map[string]any`, the decode is a map→map copy; when `R` is a struct, mapstructure maps by its `mapstructure` tag (or field name).
- **Empty template** is valid: `Map` returns the zero/empty `R`.

### Input seeding & result decoding — mapstructure (ADR-0010)

Both directions use `github.com/go-viper/mapstructure/v2`:
- **Input:** `flatten(input I)` — `mapstructure.Decode(input, &m)` where `m` is `map[string]any` flattens a struct into the Scope seed, preserving field types. A `map[string]any` input bypasses mapstructure (used directly).
- **Result:** `Mapper.Map` — `mapstructure.Decode(out, &r)` maps the assembled nested map into `R` (struct or map). Nested dot-paths become nested maps, decoded into nested struct fields.

Type fidelity (ints stay ints, not float64) is the reason for mapstructure over an `encoding/json` round-trip; robustness/tags is the reason over hand-rolled reflection (ADR-0010).

### Error model (`errors.go`)

```go
// MappingError reports a failure compiling or evaluating a result-mapping field,
// or decoding the assembled result. Field is the output dot-path ("" for the
// final decode). It unwraps to the underlying expr or mapstructure error.
type MappingError struct {
	Field string
	Cause error
}
func (e *MappingError) Error() string
func (e *MappingError) Unwrap() error
```

`Evaluate`'s error sources: input flatten (wrapped with `%w`), pipeline `Run` (typed `*stage.StageError`/pipeline errors, passed through), and mapping (`*MappingError`). `errors.As` reaches each. `MappingError.Error()` uses `%v` on `Cause` (nil-safe).

## Testing strategy

TDD, red → green → refactor.

- **Table-driven** tests via `table-test` (assert-closure form). `Evaluate` takes a `context.Context`, so its table uses the **`ctx` modifier + `t.Context()`** with a **canceled-context** case (the pipeline short-circuits).
- **Runnable `Example…` tests** doubling as godoc: an end-to-end `Engine` with a struct input and struct result over a small pipeline + mapping; a `map[string]any` in/out variant.
- Coverage of (**every hot-path + typed-error branch**, per the coverage gate):
  - `flatten` — `map[string]any` passthrough; struct→map; a non-struct/non-map (e.g. `int`) input that mapstructure rejects → error.
  - `NewMapper` — compiles fields; a bad field expression → `*MappingError` naming the field; empty template valid.
  - `Map` — single and nested (dot-path) output fields; a field eval error → `*MappingError`; decode into a struct (tags) and into `map[string]any`; a decode type-mismatch → `*MappingError`.
  - `Engine.Evaluate` — happy path struct→struct; the dependent pipeline output reaches the result; a pipeline stage error surfaces (unwrapped `*stage.StageError`); a canceled context short-circuits; input flatten error surfaces.
  - `MappingError.Error()`/`Unwrap` for field and final-decode cases.
- **Library quality gates:** `go test ./... -race`, `go vet ./...`, `gofmt`/`gofumpt`, `golangci-lint run ./...`, `govulncheck ./...` (if installed) clean; `go mod tidy` updates `go.mod`/`go.sum` once (adds mapstructure), then a no-op; `go mod verify` passes.

## Dependencies

- **One new consumer-visible dependency:** `github.com/go-viper/mapstructure/v2` (struct↔map decoding, both directions). Consumer-visible deps after this increment: `expr-lang/expr`, `gopkg.in/yaml.v3` (config only), `go-viper/mapstructure/v2`. `testify` stays test-only. Target Go 1.25+.

## Traceability

- **Spec:** 005 (this document). Builds on Specs 001–004. Completes the roadmap.
- **Plan:** `docs/plans/005-engine-facade.md`.
- **ADRs:** ADR-0001 (Engine/Evaluate naming — realized here), ADR-0009 (Engine/Mapper design + root placement), ADR-0010 (mapstructure dependency).
- Implementation commits reference this spec via a `Spec: 005` trailer (and `ADR:` trailers).
