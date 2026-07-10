# Spec 004 — Declarative config (YAML/JSON loaders)

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-11
- **Increment:** 4 of 5 (see [Roadmap](#roadmap-position))
- **Builds on:** Spec 002 (the `stage` package — `SingleExpr`/`MultiExpr`/`DecisionTable`, `Option`, `HitPolicy`), Spec 003 (`stage.Pipeline`, `NewPipeline`), and Spec 001 (the `expr` package — `expr.Option`: `WithFallback`, `WithGlobals`, `WithLocals`, `WithCoerce`, `WithReturnKind`).
- **Realized by plans:** `docs/plans/004-declarative-config.md`
- **Related ADRs:** ADR-0007 (config package, two-phase parse/Build, list schema, `ExprDef` scalar shorthand), ADR-0008 (dependency choices — `yaml.v3` only; no `mimetype`/`validator`) — recorded with the implementation commits of this increment.

## Context

`rlng` is a pure-Go rule + calculation engine on [`expr-lang/expr`](https://github.com/expr-lang/expr), built for debuggability (no cgo, plain stack traces, typed errors that name the failing field and expression). Increments 1–3 delivered the programmatic API: atomic evaluators (`expr`), the three stage types + `Scope` (`stage`), and the `Pipeline` DAG orchestrator (`stage`). Everything so far is constructed from Go. This increment adds the **declarative front door**: parse a stage/pipeline definition from **YAML or JSON** and build a `*stage.Pipeline` from it, so consumers can author rules as config rather than Go code.

### Roadmap position

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | **Done** (merged) |
| 2 | Scope + stages | **Done** (merged) |
| 3 | Stage DAG orchestration (`Pipeline`) | **Done** (merged) |
| **4** | **Declarative config (YAML/JSON loaders)** *(this spec)* | **This increment** |
| 5 | Result mapper + `Engine[I, R]` facade | later |

**Scope boundary:** this increment ships the **config → `*stage.Pipeline`** path only. It parses declarative definitions and builds the already-existing stage/pipeline types. It does **not** add the typed-input `Scope` seeding or typed-result mapping (the `Engine[I, R]` facade, Increment 5), and does **not** implement config-declared variable *defaults* injection (the `VariablePatcher` `x ?? <literal>` mechanism) — `ExprDef` exposes `globals`/`locals` via the existing `expr` options, which covers the immediate need.

## Goals

1. **Definition types** modeling a pipeline of stages declaratively:
   - `PipelineDef` — an ordered **list** of `StageDef`.
   - `StageDef` — a flat union with a `type` discriminator (`single-expr` / `multi-expr` / `decision-table`), `name`, `depends_on`, and type-specific fields.
   - `ExprDef` — an expression definition supporting a **scalar shorthand** (a bare string = the expression) or a full object (`expr`, `fallback`, `globals`, `coerce`), decoded by custom `UnmarshalYAML`/`UnmarshalJSON`. Reused for single-expr values/conditions, multi-expr entries, and decision-table conditions/decisions.
2. **Parsers**: `ParseYAML([]byte)`, `ParseJSON([]byte)`, and `LoadFile(path)` (dispatch by file extension), each returning a `*PipelineDef`.
3. **Builder**: `(*PipelineDef).Build() (*stage.Pipeline, error)` — map each `StageDef` to the matching `stage.New*` constructor and assemble via `stage.NewPipeline`. Compilation/validation is delegated to the existing constructors; the builder adds only config-shape validation.
4. **Typed `ConfigError`** naming the offending stage and field, unwrapping to the underlying `stage`/`expr` error — preserving the debuggability chain from config text to compiled expression.
5. **Minimal dependency footprint**: exactly one new consumer-visible dependency, `gopkg.in/yaml.v3`; JSON via the standard library.

## Non-goals (deferred)

- **`Engine[I, R]` facade** — typed-input seeding and typed-result mapping (Increment 5). `Build` returns a `*stage.Pipeline`; the caller supplies the `*stage.Scope`.
- **`VariablePatcher`** (config-declared defaults injected as `x ?? <literal>` at compile time) — not needed now; `ExprDef.globals`/`locals` cover declared variables.
- **A generic `ConfigLoader[T]` abstraction** and content-type **sniffing** via `mimetype` — concrete parse functions + extension dispatch instead (ADR-0008).
- **`go-playground/validator`** — inline validation with typed, field-located errors instead (ADR-0008).
- **Round-tripping / serialization** (writing config back out) — read-only load.

## Design

### Package layout

```
github.com/kartaladev/rlng/
  config/                     # this increment (new package)
    def.go                    # PipelineDef, StageDef, ExprDef (+ NamedExprDef, RuleDef) types
    expr_def.go               # ExprDef scalar-shorthand UnmarshalYAML/UnmarshalJSON + toOptions
    parse.go                  # ParseYAML, ParseJSON, LoadFile (extension dispatch)
    build.go                  # (*PipelineDef).Build -> *stage.Pipeline; per-type stage builders
    errors.go                 # ConfigError
    <...>_test.go, <...>_example_test.go
```

`config` imports `github.com/kartaladev/rlng/stage`, `github.com/kartaladev/rlng/expr`, `gopkg.in/yaml.v3`, and the standard library (`encoding/json`, `os`, `path/filepath`, `fmt`, `errors`, `strings`). The root `rlng` package stays empty until the Increment-5 `Engine`. Keeping YAML out of `stage`/`expr` preserves their zero-extra-dep footprint.

### Definition types (`def.go`)

```go
// PipelineDef is a declarative pipeline: an ordered list of stages. List order
// is authoring order, which Build preserves into NewPipeline so the pipeline's
// deterministic tie-break ordering is reproducible.
type PipelineDef struct {
	Stages []StageDef `yaml:"stages" json:"stages"`
}

// StageDef is a flat union over the three stage types, selected by Type.
// Fields not relevant to Type are ignored by Build.
type StageDef struct {
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"` // single-expr | multi-expr | decision-table
	DependsOn []string `yaml:"depends_on" json:"depends_on"`

	// single-expr
	Expr      *ExprDef `yaml:"expr" json:"expr"`
	Condition *ExprDef `yaml:"condition" json:"condition"`
	Output    string   `yaml:"output" json:"output"`

	// multi-expr
	Exprs []NamedExprDef `yaml:"exprs" json:"exprs"`

	// decision-table
	HitPolicy string    `yaml:"hit_policy" json:"hit_policy"` // single (default) | collect
	Rules     []RuleDef `yaml:"rules" json:"rules"`
}

type NamedExprDef struct {
	Name     string   `yaml:"name" json:"name"`
	Priority int      `yaml:"priority" json:"priority"`
	Expr     ExprDef  `yaml:"expr" json:"expr"`
}

type RuleDef struct {
	Condition ExprDef            `yaml:"condition" json:"condition"`
	Decisions map[string]ExprDef `yaml:"decisions" json:"decisions"`
}
```

### `ExprDef` scalar shorthand (`expr_def.go`)

```go
// ExprDef is an expression with optional compile options. It decodes from
// either a scalar string (shorthand: the string is Expr) or a mapping.
type ExprDef struct {
	Expr     string         `yaml:"expr" json:"expr"`
	Fallback string         `yaml:"fallback" json:"fallback"`
	Globals  map[string]any `yaml:"globals" json:"globals"`
	Coerce   bool           `yaml:"coerce" json:"coerce"`
}

func (e *ExprDef) UnmarshalYAML(value *yaml.Node) error // scalar -> Expr; mapping -> fields
func (e *ExprDef) UnmarshalJSON(data []byte) error      // JSON string -> Expr; object -> fields

// options maps the ExprDef to expr.Option values (WithFallback, WithGlobals,
// WithCoerce), used when compiling via the stage constructors.
func (e ExprDef) options() []expr.Option
```

- **YAML shorthand:** if the node kind is a scalar, set `Expr` to its value; if a mapping, decode into the struct (using an alias type to avoid infinite recursion). Any other kind is a `ConfigError`.
- **JSON shorthand:** if `data` unmarshals as a JSON string, set `Expr`; if it starts with `{`, decode the object; otherwise error.
- `options()` returns `WithFallback` when `Fallback != ""`, `WithGlobals` when `Globals` is non-empty, and `WithCoerce` when `Coerce` is true.

### Parsers (`parse.go`)

```go
func ParseYAML(data []byte) (*PipelineDef, error) // yaml.Unmarshal with KnownFields where practical
func ParseJSON(data []byte) (*PipelineDef, error) // encoding/json
func LoadFile(path string) (*PipelineDef, error)  // os.ReadFile + dispatch by extension
```

`LoadFile` dispatches on the lower-cased extension: `.yaml`/`.yml` → `ParseYAML`, `.json` → `ParseJSON`; any other extension is a `ConfigError` naming the path. No content sniffing (ADR-0008). Parse errors from `yaml.v3`/`encoding/json` are wrapped in a `ConfigError` (no stage/field context available yet at the decode stage).

### Builder (`build.go`)

```go
func (d *PipelineDef) Build() (*stage.Pipeline, error)
```

For each `StageDef`, in order:

- Assemble the type-agnostic options: `stage.WithDependsOn(sd.DependsOn...)`.
- Switch on `Type`:
  - **`single-expr`**: require `Expr != nil`; options = `WithExprOptions(sd.Expr.options()...)`, plus `WithCondition(sd.Condition.Expr, sd.Condition.options()...)` if `Condition != nil`, plus `WithOutput(sd.Output)` if `Output != ""`; call `stage.NewSingleExpr(sd.Name, sd.Expr.Expr, opts...)`.
  - **`multi-expr`**: require non-empty `Exprs`; map each `NamedExprDef` to `stage.NamedExpr{Name, Expression: e.Expr.Expr, Priority, Options: e.Expr.options()}`; call `stage.NewMultiExpr(sd.Name, named, opts...)`.
  - **`decision-table`**: require non-empty `Rules`; parse `HitPolicy` (`""`/`single` → `HitPolicySingle`, `collect` → `HitPolicyCollect`, else `ConfigError`); map each `RuleDef` to `stage.Rule{Condition: r.Condition.Expr, ConditionOptions: r.Condition.options(), Decisions: <key->e.Expr.Expr>, DecisionOptions: <merged, see below>}`; call `stage.NewDecisionTable(sd.Name, rules, WithHitPolicy(hp), opts...)`.
  - **unknown / empty `Type`**: `ConfigError` naming the stage.
- Collect the built `stage.Stage` values in list order and call `stage.NewPipeline(stages...)`; surface its `*DuplicateStageError`/`*UnknownDependencyError`/`*CycleError` (wrapped in a `ConfigError` for a config-level message, still `errors.As`-reachable to the pipeline error).

**Decision-table per-decision options caveat:** `stage.Rule` carries one `DecisionOptions []expr.Option` shared across all decisions in the rule, but each `ExprDef` decision can declare its own `fallback`/`globals`. Since the shared slot cannot hold per-decision options, this increment applies **per-decision options is a known limitation**: `RuleDef.Decisions` values use only their `Expr` string; a rule-level `decision_options` is not exposed. Recorded as a backlog item; the common case (bare-string decisions) is unaffected. (Alternatively, decisions with options are rejected with a clear `ConfigError` rather than silently dropped — the plan chooses rejection to avoid surprise.)

### Error model (`errors.go`)

```go
// ConfigError reports a failure loading or building a pipeline definition. It
// names the stage and field where known and unwraps to the underlying cause
// (a decode error, a *stage.StageError, or a pipeline construction error).
type ConfigError struct {
	Stage string // "" when not stage-scoped (e.g. a decode error)
	Field string // "" when not field-scoped
	Cause error
}
func (e *ConfigError) Error() string
func (e *ConfigError) Unwrap() error
```

Because `Cause` unwraps to the existing `stage`/`expr`/pipeline errors (which already name the expression and field), `errors.As` reaches the exact failure — config text → stage → compiled expression — end to end.

## Testing strategy

TDD, red → green → refactor from the first commit.

- **Table-driven** tests via the `table-test` skill (assert-closure form). Parsers/Build take no `context.Context`, so those tables are context-free; there is no lifecycle path to cancel in this increment.
- **Runnable `Example…` tests** doubling as godoc: a full YAML pipeline (single-expr + decision-table) parsed and built, then run against a `Scope`; the JSON equivalent; and the scalar-shorthand form.
- Coverage of (**every hot-path + typed-error branch**, per the coverage gate):
  - `ExprDef` decode — YAML scalar shorthand vs mapping vs invalid kind; JSON string vs object vs invalid; `options()` each branch (fallback/globals/coerce present and absent).
  - `ParseYAML`/`ParseJSON` — valid, and malformed input → `ConfigError`.
  - `LoadFile` — `.yaml`, `.yml`, `.json` dispatch; unknown extension → `ConfigError`; unreadable path → `ConfigError`.
  - `Build` — each stage type built and runnable; unknown `type` → `ConfigError`; missing required field per type (`single-expr` no `expr`, `multi-expr` no `exprs`, `decision-table` no `rules`) → `ConfigError`; invalid `hit_policy` → `ConfigError`; a compile error from a bad expression surfaces as a `ConfigError` unwrapping to `*stage.StageError`; a duplicate name / unknown dependency / cycle surfaces the pipeline error.
- **Library quality gates:** `go test ./... -race`, `go vet ./...`, `gofmt`/`gofumpt`, `golangci-lint run ./...`, and `govulncheck ./...` (if installed) all clean; **`go mod tidy`** now legitimately updates `go.mod`/`go.sum` (adding `yaml.v3`) — that is expected for this increment, after which it is a no-op.

## Dependencies

- **One new consumer-visible dependency:** `gopkg.in/yaml.v3` (YAML decode). JSON uses the standard library. No `mimetype`, no `go-playground/validator` (ADR-0008). `expr-lang/expr` and `yaml.v3` are then the two consumer-visible deps; `testify` stays test-only. Target Go 1.25+.

## Traceability

- **Spec:** 004 (this document). Builds on Specs 001–003.
- **Plan:** `docs/plans/004-declarative-config.md`.
- **ADRs:** ADR-0007 (config package, two-phase parse/Build, list schema, `ExprDef` scalar shorthand), ADR-0008 (dependency choices).
- Implementation commits reference this spec via a `Spec: 004` trailer (and `ADR:` trailers where they record a decision).
