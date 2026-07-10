# Spec 001 — Expression Core

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-10
- **Increment:** 1 of 5 (see [Roadmap](#roadmap-position))
- **Realized by plans:** _`docs/plans/001-*` (pending — created next via writing-plans)_
- **Related ADRs:** ADR-0001 (module path + rule-vs-`calc` naming) — to be recorded with the first implementation commit

## Context

`rlng` is a pure-Go, general-purpose **rule + calculation engine** built on
[`expr-lang/expr`](https://github.com/expr-lang/expr). Its guiding constraint is
debuggability: no cgo, plain Go stack traces, and typed errors that name the failing
field and expression (contrast with `gorules/zen-go`, which binds a Rust engine via cgo).

The library serves **two tracks on a shared core**:

- **Rule track** → decision tables (backs a BPMN `BusinessRuleTask`; `map[string]any` →
  `map[string]any`). First external consumer: the `wrkflw` workflow engine, which plugs a
  rule engine in through its generic `action.Action` port
  (`Do(ctx, map[string]any) (map[string]any, error)`).
- **Calculation track** → staged expression pipelines projected into a typed result
  (the general-purpose capability seeded by the `pkg/calc` reference in `bbn-crm-backend`).

Both tracks are built from the same atomic evaluators. **This spec defines that atom.**

### Roadmap position

The full engine is delivered as an ordered sequence of spec → plan → implementation cycles:

| # | Increment | Serves |
|---|-----------|--------|
| **1** | **Expression core** *(this spec)* | both tracks |
| 2 | Context + stages (single-expr, multi-expr, decision-table) | both; early wrkflw value |
| 3 | Stage DAG orchestration (`depends_on` topo-sort + cycle detection) | calculation pipelines |
| 4 | Declarative config (YAML/JSON loaders) | both |
| 5 | Result mapper + `Engine[I, R]` facade + `map`→`map` action adapter | full pipeline reached |

## Goals

1. Provide the two atomic evaluators every higher layer composes from:
   - **`Predicate`** — a compiled boolean expression: `Test(env) (bool, error)`.
   - **`Function`** — a compiled value expression with optional fallback: `Apply(env) (any, error)`.
2. Compile once, evaluate many; compilation errors surface at construction.
3. Support config-declared **variable defaults** (globals + locals) injected as `??`
   fallbacks at compile time.
4. Fail with **typed, `Unwrap`-able errors** that name the field and the offending expression.
5. Evaluate over `map[string]any`, and accept a `struct`/`*struct` by converting it to that env.

## Non-goals (deferred to later increments)

- `Context` accumulator, stages, decision tables, DAG orchestration (increments 2–3).
- YAML/JSON config loading — this layer is constructed programmatically from Go strings (increment 4).
- Result mapping and the public `Engine` facade (increment 5).
- `context.Context` / cancellation at this layer — the primitives are pure, synchronous,
  and fast; cancellation is introduced at the stage/engine layer where it is meaningful.

## Design

### Package layout

```
github.com/kartaladev/rlng/
  expr/                 # this increment
    predicate.go        # Predicate, NewPredicate, Test
    function.go         # Function, NewFunction, Apply
    variables.go        # variable patcher (globals/locals -> `??` defaults)
    env.go              # struct/*struct -> map[string]any conversion
    options.go          # functional options + internal config
    errors.go           # CompileError, EvalError, sentinels
```

The `expr-lang/expr` dependency is import-aliased inside this package (e.g. `exprlang`) to
avoid the package-name clash. The root `rlng` package is left clean for the eventual
`Engine` facade.

### Naming (ADR-0001)

- Top-level facade (increment 5): neutral **`Engine` / `Evaluate`** — the library spans
  rules *and* calculations, so `Calculator`/`Calculate` is too narrow.
- This layer: **`Predicate.Test`** and **`Function.Apply`** — matches DMN/rule vocabulary
  and the reference.
- ADR-0001 records the module path (`github.com/kartaladev/rlng`, ratifying the git remote)
  and this rule-vs-`calc` naming. It rides with the first implementation commit (per the
  commit discipline: ADRs are committed with the code that realizes them).

### Public API

```go
package expr

// Predicate is a compiled boolean expression.
type Predicate struct { /* compiled program + flags (unexported) */ }

func NewPredicate(expression string, opts ...Option) (*Predicate, error)
func (p *Predicate) Test(env any) (bool, error)

// Function is a compiled value expression with an optional fallback.
type Function struct { /* compiled program + fallback (unexported) */ }

func NewFunction(name, expression string, opts ...Option) (*Function, error)
func (f *Function) Apply(env any) (any, error)
```

`env any` accepts a `map[string]any` (used directly) or a `struct`/`*struct` (converted via
`env.go`). `nil` env is treated as an empty environment.

### Options (D1: plain functional options, no external dep)

The reference used `lestrrat-go/option`; we drop it. Every direct dependency becomes a
consumer's transitive dependency (a library quality gate), and std-style functional options
give the same ergonomics with zero deps.

```go
type Option func(*config)

func WithGlobals(vars map[string]any) Option // engine-wide default variables
func WithLocals(vars map[string]any) Option  // per-evaluator default variables (take precedence)
func WithCoerce() Option                      // Predicate: opt into lenient truthiness (default is strict — see D2)
func WithFallback(expression string) Option   // Function: expression evaluated when the main errors
func WithReturnKind(k reflect.Kind) Option    // Function: compile with expr.AsKind(k)
```

Options share one `config`; `WithCoerce` applies only to `Predicate`, `WithFallback`/
`WithReturnKind` only to `Function`, and `WithGlobals`/`WithLocals` to both. An option passed
to the evaluator it does not apply to is ignored — documented per option to avoid surprise.
Strictness has a single control: **omit `WithCoerce` for strict** (default), **pass it for
lenient**. There is no separate `WithStrict`.

### Variable defaults — the `??` patcher

At **compile time**, an AST visitor rewrites each identifier `x` that matches a declared
variable into `x ?? <literal>`, so the variable acts as a default overridable by runtime
input. Lookup precedence: **locals → globals**. Only scalar kinds are patched
(bool / string / int* / uint* / float*); non-scalar values are skipped with a logged
warning (they are not representable as a literal AST node). Pointers are followed to their
pointee; nil pointers are skipped with a warning.

### Truthiness policy (D2: strict by default)

Debuggability is the primary criterion, so:

- **`Predicate` is strict by default** — compiled with `expr.AsBool()`; a non-boolean result
  is a typed `EvalError` (wrapping `ErrNotBool`), not a silent coercion.
- **`WithCoerce()`** opts into the lenient truthiness table (matching the reference):
  `nil` → false; numbers → `!= 0`; string → `strconv.ParseBool`, else non-empty; slice/map →
  non-empty; other → false.

### Function fallback semantics

`Apply` runs the main program. The fallback (if a `WithFallback` expression was provided) is
evaluated — over an empty env, and its result returned — in exactly two cases: (a) the main
program returns an error, or (b) the main program succeeds but yields a `nil` result. With no
fallback configured, `Apply` returns the main program's error (case a) or its `nil` result
(case b) unchanged. Fallback compile errors surface at `NewFunction` time, not at `Apply` time.

### Error model

```go
type CompileError struct { Name, Expression string; Cause error } // from New*, wraps expr compile error
type EvalError    struct { Name, Expression string; Cause error } // from Test/Apply

// sentinels for errors.Is
var ErrNotBool = errors.New("expression did not evaluate to bool")
```

Both error types implement `Error()` (message names `Name` + `Expression` + `Cause`) and
`Unwrap()`. For a `Predicate` the `Name` is empty unless supplied; a `Function` uses its
`name` argument.

### Env conversion (`env.go`)

A small reflection walk converts a `struct`/`*struct` to `map[string]any`: exported fields
become keys (by Go field name), nested structs become nested maps, slices/arrays and maps are
converted element-wise, nil pointers become `nil`. A `map[string]any` is passed through
unchanged. This covers wrkflw's variables bag and general-purpose typed callers. `mapstructure`
is **not** introduced here (deferred to the mapper increment).

`expr.AllowUndefinedVariables()` is enabled on every compile, so referencing a key absent from
the env yields `nil` rather than a compile/eval error — higher layers depend on this to
reference not-yet-computed fields.

## Testing strategy

TDD, red → green → refactor from the first commit (per `superpowers:test-driven-development`).

- **Table-driven** tests using the project `table-test` skill (`assert` closure form,
  `t.Context()` where a context is involved).
- **Runnable `Example…` tests** that double as godoc for `Predicate` and `Function`.
- Coverage of: strict vs `WithCoerce` truthiness matrix; patcher default-and-override with
  locals-over-globals precedence and scalar-only limits; function fallback-on-error and
  fallback-on-nil; struct→env conversion (nested, slices, nil pointers); every typed error
  path (`CompileError`, `EvalError`, `ErrNotBool`).
- `go test ./... -race`, `go vet`, `golangci-lint`, `govulncheck` clean (library quality gates).

## Dependencies

- **Direct runtime (new):** `github.com/expr-lang/expr` — the expression core.
- **Test-only:** `github.com/stretchr/testify` — required by the mandatory `table-test` convention. Test-scoped and pruned from consumers by the Go module graph, so it does not count against the minimal-runtime-deps gate.
- **Dropped vs reference:** `lestrrat-go/option` (D1). `mapstructure` deferred to increment 5.
- Net: **one** consumer-visible dependency for this increment. Target Go 1.25+.

## Traceability

- **Spec:** 001 (this document).
- **Plan:** `docs/plans/001-*` — pending, created next via `superpowers:writing-plans`; will link back here.
- **ADR:** ADR-0001 (module path + naming) — recorded with the first implementation commit.
- Implementation commits reference this spec via a `Spec: 001` trailer.
