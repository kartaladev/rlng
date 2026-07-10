# Result Mapper + `Engine[I, R]` Facade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the root `rlng` package facade: `Engine[I, R]` (seed a Scope from typed input, run a pipeline, map the final Scope into typed R) plus `Mapper[R]`/`MappingTemplate`.

**Architecture:** Three files in the module-root package `rlng`: `errors.go` (`MappingError`), `mapper.go` (`Mapper[R]`, `MappingTemplate`, `NewMapper`, `Map`, `setNested`), and `engine.go` (`Engine[I, R]`, `New`, `Option`/`WithScopeOptions`, `Evaluate`, `flatten`). Result-field expressions reuse `expr.Function`; the pipeline is the Increment-3 `stage.Pipeline`; input/result struct↔map conversion uses `github.com/go-viper/mapstructure/v2`. The root package imports `stage`, `expr`, and mapstructure — not `config`.

**Tech Stack:** Go 1.25+; `github.com/go-viper/mapstructure/v2` (the one new dependency); reuses `.../stage` and `.../expr`. Tests use `github.com/stretchr/testify`.

## Global Constraints

- Module path `github.com/kartaladev/rlng`; the code is the **root package `rlng`**. (Spec 005 §Design)
- **One new consumer-visible dependency: `github.com/go-viper/mapstructure/v2`.** (ADR-0010) Root `rlng` must **not** import `config`. (ADR-0009)
- **Pure Go, no cgo; no `os.Exit`/`log.Fatal`/`panic` on caller input** — return typed errors. (CLAUDE.md)
- **Typed, `errors.As`-reachable errors**: `MappingError{Field, Cause}` for mapper compile/eval/decode; pipeline errors pass through unwrapped; input flatten wrapped with `%w`. (Spec 005 §Error model)
- **Tests follow the `table-test` skill:** `assert` closure form; `Evaluate` is context-sensitive, so its table uses the `ctx` modifier with `t.Context()` and a canceled-context case. Mapper tables that take no context stay context-free; where different `R` type instantiations force structurally different setup, split into focused test funcs (document the divergence). (table-test skill)
- **Test-coverage gate:** target ≥ 85% on the root package; every hot-path + typed-error branch listed per task has a covering test. (CLAUDE.md §Test-coverage gate)
- Quality gates before delivery: `go build ./...`, `go vet ./...`, `gofmt -l .` empty, `golangci-lint run ./...`, `go test ./... -race`. `go mod tidy` updates `go.mod`/`go.sum` **once** (adds mapstructure), then a no-op; `go mod verify` passes. (CLAUDE.md §Library quality gates)

---

### Task 1: `MappingError` + `Mapper[R]`

**Files:**
- Create: `errors.go`, `mapper.go`
- Test: `mapper_test.go`
- Also stage in this task's commit (ride-with-code): `docs/adrs/0009-engine-and-mapper.md`, `docs/adrs/0010-mapstructure-dependency.md`, `docs/plans/005-engine-facade.md`

**Interfaces:**
- Consumes: `github.com/kartaladev/rlng/expr` (`expr.NewFunction`, `*expr.Function`, `Apply`); `github.com/go-viper/mapstructure/v2`.
- Produces:
  - `type MappingError struct{ Field string; Cause error }` (with `Error()`/`Unwrap()`)
  - `type MappingTemplate map[string]string`
  - `type Mapper[R any] struct{ ... }`; `func NewMapper[R any](tmpl MappingTemplate) (*Mapper[R], error)`; `func (m *Mapper[R]) Map(scope map[string]any) (R, error)`

**Hot-path branches to cover:** `NewMapper` compiles fields; bad field expression → `*MappingError` naming the field; empty template valid. `Map` single + nested dot-path output; field eval error → `*MappingError`; decode into a struct (tags) and into `map[string]any`; decode type-mismatch → `*MappingError`. `MappingError.Error()` for field and final-decode (`Field==""`) forms + `Unwrap`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/go-viper/mapstructure/v2@latest`
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the failing tests**

Create `mapper_test.go`:

```go
package rlng

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapResult struct {
	Total int `mapstructure:"total"`
	Info  struct {
		Tag string `mapstructure:"tag"`
	} `mapstructure:"info"`
}

func TestNewMapper(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		tmpl   MappingTemplate
		assert func(t *testing.T, m *Mapper[mapResult], err error)
	}

	cases := []testCase{
		{
			name: "compiles fields",
			tmpl: MappingTemplate{"total": "1 + 1"},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "empty template is valid",
			tmpl: MappingTemplate{},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "bad field expression is a MappingError",
			tmpl: MappingTemplate{"total": "1 +"},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				assert.Nil(t, m)
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := NewMapper[mapResult](tc.tmpl)
			tc.assert(t, m, err)
		})
	}
}

func TestMapperMapStruct(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		tmpl   MappingTemplate
		scope  map[string]any
		assert func(t *testing.T, r mapResult, err error)
	}

	cases := []testCase{
		{
			name:  "single and nested fields",
			tmpl:  MappingTemplate{"total": "net + tax", "info.tag": "label"},
			scope: map[string]any{"net": 10, "tax": 2, "label": "big"},
			assert: func(t *testing.T, r mapResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 12, r.Total)
				assert.Equal(t, "big", r.Info.Tag)
			},
		},
		{
			name:  "field eval error is a MappingError",
			tmpl:  MappingTemplate{"total": "a % 0"},
			scope: map[string]any{"a": 1},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
		{
			name:  "decode type mismatch is a MappingError",
			tmpl:  MappingTemplate{"total": `"not a number"`},
			scope: map[string]any{},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Empty(t, me.Field) // final decode
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := NewMapper[mapResult](tc.tmpl)
			require.NoError(t, err)
			r, err := m.Map(tc.scope)
			tc.assert(t, r, err)
		})
	}
}

// TestMapperMapToMap covers R = map[string]any (a structurally different R than
// the struct table above), so it is a separate focused test.
func TestMapperMapToMap(t *testing.T) {
	t.Parallel()

	m, err := NewMapper[map[string]any](MappingTemplate{"total": "1 + 2"})
	require.NoError(t, err)
	r, err := m.Map(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 3, r["total"])
}

func TestMappingErrorMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		err  *MappingError
		want string
	}

	cause := errors.New("boom")
	cases := []testCase{
		{name: "field", err: &MappingError{Field: "total", Cause: cause}, want: `rlng: mapping field "total": boom`},
		{name: "final decode", err: &MappingError{Cause: cause}, want: `rlng: mapping: boom`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.err.Error())
			assert.ErrorIs(t, tc.err, cause)
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test . 2>&1 | head`
Expected: FAIL — `MappingError`/`Mapper`/`NewMapper` undefined.

- [ ] **Step 4: Write `errors.go`**

```go
package rlng

import "fmt"

// MappingError reports a failure compiling or evaluating a result-mapping field,
// or decoding the assembled result. Field is the output dot-path ("" for the
// final decode). It unwraps to the underlying expr or mapstructure error.
type MappingError struct {
	Field string
	Cause error
}

func (e *MappingError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("rlng: mapping field %q: %v", e.Field, e.Cause)
	}
	return fmt.Sprintf("rlng: mapping: %v", e.Cause)
}

func (e *MappingError) Unwrap() error { return e.Cause }
```

- [ ] **Step 5: Write `mapper.go`**

```go
package rlng

import (
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/expr"
)

// MappingTemplate maps an output dot-path to a leaf expression evaluated against
// the final Scope, e.g. {"total": "line.net + line.tax", "info.tag": "tiers.tag"}.
type MappingTemplate map[string]string

// Mapper projects a Scope into a typed R by evaluating each template field and
// decoding the assembled nested map into R.
type Mapper[R any] struct {
	fields []mappedField
}

type mappedField struct {
	path string
	fn   *expr.Function
}

// NewMapper compiles each template field's expression up front. A compile error
// is a *MappingError naming the field. Fields are evaluated in sorted dot-path
// order for determinism.
func NewMapper[R any](tmpl MappingTemplate) (*Mapper[R], error) {
	paths := make([]string, 0, len(tmpl))
	for p := range tmpl {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fields := make([]mappedField, 0, len(paths))
	for _, p := range paths {
		fn, err := expr.NewFunction(p, tmpl[p])
		if err != nil {
			return nil, &MappingError{Field: p, Cause: err}
		}
		fields = append(fields, mappedField{path: p, fn: fn})
	}
	return &Mapper[R]{fields: fields}, nil
}

// Map evaluates each field against scope, assembles a nested map[string]any by
// dot-path, and decodes it into R. Eval and decode errors are *MappingError.
func (m *Mapper[R]) Map(scope map[string]any) (R, error) {
	var zero R
	out := make(map[string]any)
	for _, f := range m.fields {
		v, err := f.fn.Apply(scope)
		if err != nil {
			return zero, &MappingError{Field: f.path, Cause: err}
		}
		setNested(out, f.path, v)
	}

	var r R
	if err := mapstructure.Decode(out, &r); err != nil {
		return zero, &MappingError{Cause: err}
	}
	return r, nil
}

// setNested writes v at a dot-separated path in out, creating intermediate maps.
func setNested(out map[string]any, path string, v any) {
	keys := strings.Split(path, ".")
	m := out
	for _, k := range keys[:len(keys)-1] {
		child, ok := m[k].(map[string]any)
		if !ok {
			child = make(map[string]any)
			m[k] = child
		}
		m = child
	}
	m[keys[len(keys)-1]] = v
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test . -race -run 'Mapper|NewMapper|MappingError' -v 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 7: Gates + commit**

```bash
go test . -race -cover
go vet . && gofmt -l .
git add errors.go mapper.go mapper_test.go go.mod go.sum \
  docs/adrs/0009-engine-and-mapper.md docs/adrs/0010-mapstructure-dependency.md \
  docs/plans/005-engine-facade.md
git commit -m "$(cat <<'MSG'
feat(rlng): result Mapper[R] and MappingTemplate

Add the root-package result projection: a MappingTemplate maps output
dot-paths to leaf expressions evaluated against the final Scope; NewMapper
compiles each once, and Map assembles a nested map and decodes it into R
via mapstructure. Typed MappingError names the offending field. Adds
go-viper/mapstructure/v2 as the one new consumer dependency (ADR-0010).

Spec: 005
Plan: 005
ADR: 0009
ADR: 0010
MSG
)"
```

---

### Task 2: `Engine[I, R]` + `Evaluate`

**Files:**
- Create: `engine.go`
- Test: `engine_test.go`

**Interfaces:**
- Consumes: `Mapper[R]` (Task 1); `github.com/kartaladev/rlng/stage` (`Pipeline`, `NewPipeline`, `NewScope`, `ScopeOption`, `Snapshot`, `NewSingleExpr`, `WithDependsOn`, `StageError`); `github.com/go-viper/mapstructure/v2`.
- Produces:
  - `type Engine[I any, R any] struct{ ... }`
  - `type Option func(*engineConfig)`; `func WithScopeOptions(opts ...stage.ScopeOption) Option`
  - `func New[I any, R any](pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option) *Engine[I, R]`
  - `func (e *Engine[I, R]) Evaluate(ctx context.Context, input I) (R, error)`

**Hot-path branches to cover:** `flatten` map[string]any passthrough; struct→map; a non-map non-struct input (`int`) → error. `Evaluate` happy struct→struct (dependent pipeline output reaches result); pipeline stage error surfaces (unwrapped `*stage.StageError`); canceled context short-circuits; input flatten error surfaces; `WithScopeOptions` applied.

- [ ] **Step 1: Write the failing tests**

Create `engine_test.go`:

```go
package rlng

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type order struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type quote struct {
	Total float64 `mapstructure:"total"`
}

// buildEngine wires a two-stage pipeline (base = price*qty, taxed = base*1.1)
// and a mapper projecting total = taxed.
func buildEngine(t *testing.T) *Engine[order, quote] {
	t.Helper()
	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	taxed, err := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	require.NoError(t, err)
	p, err := stage.NewPipeline(base, taxed)
	require.NoError(t, err)
	m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
	require.NoError(t, err)
	return New[order, quote](p, m)
}

func TestEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(t *testing.T) *Engine[order, quote]
		input  order
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, q quote, err error)
	}

	cases := []testCase{
		{
			name:   "happy path struct in struct out",
			engine: buildEngine,
			input:  order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 22.0, q.Total, 1e-9)
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(t *testing.T) *Engine[order, quote] {
				// boom uses modulo by zero on a seeded int, failing at eval.
				boom, err := stage.NewSingleExpr("taxed", "qty % 0")
				require.NoError(t, err)
				p, err := stage.NewPipeline(boom)
				require.NoError(t, err)
				m, err := NewMapper[quote](MappingTemplate{"total": "taxed"})
				require.NoError(t, err)
				return New[order, quote](p, m)
			},
			input: order{Price: 10, Qty: 2},
			assert: func(t *testing.T, q quote, err error) {
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "taxed", se.Stage)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: buildEngine,
			input:  order{Price: 10, Qty: 2},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, q quote, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := tc.engine(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			q, err := e.Evaluate(ctx, tc.input)
			tc.assert(t, q, err)
		})
	}
}

// TestEngineEvaluateMapInput covers a map[string]any input (I) that bypasses
// mapstructure flattening — a structurally different I, so a separate test.
func TestEngineEvaluateMapInput(t *testing.T) {
	t.Parallel()

	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := stage.NewPipeline(base)
	require.NoError(t, err)
	m, err := NewMapper[map[string]any](MappingTemplate{"out": "base"})
	require.NoError(t, err)
	e := New[map[string]any, map[string]any](p, m)

	out, err := e.Evaluate(t.Context(), map[string]any{"price": 10.0, "qty": 2.0})
	require.NoError(t, err)
	assert.InDelta(t, 20.0, out["out"], 1e-9)
}

// TestEngineEvaluateFlattenError covers an input that mapstructure cannot flatten.
func TestEngineEvaluateFlattenError(t *testing.T) {
	t.Parallel()

	p, err := stage.NewPipeline()
	require.NoError(t, err)
	m, err := NewMapper[map[string]any](MappingTemplate{})
	require.NoError(t, err)
	e := New[int, map[string]any](p, m)

	_, err = e.Evaluate(t.Context(), 42)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run TestEngine`
Expected: FAIL — `Engine`/`New`/`Evaluate` undefined.

- [ ] **Step 3: Write `engine.go`**

```go
package rlng

import (
	"context"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/stage"
)

// Engine evaluates a typed input I against a compiled pipeline and maps the
// result into a typed R. It is safe for concurrent use after construction.
type Engine[I any, R any] struct {
	pipeline  *stage.Pipeline
	mapper    *Mapper[R]
	scopeOpts []stage.ScopeOption
}

type engineConfig struct {
	scopeOpts []stage.ScopeOption
}

// Option configures an Engine.
type Option func(*engineConfig)

// WithScopeOptions passes stage.ScopeOption values (e.g. stage.WithStrict) to the
// Scope seeded for each Evaluate.
func WithScopeOptions(opts ...stage.ScopeOption) Option {
	return func(c *engineConfig) { c.scopeOpts = append(c.scopeOpts, opts...) }
}

// New constructs an Engine from a compiled pipeline and a result mapper.
func New[I any, R any](pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option) *Engine[I, R] {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return &Engine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}
}

// Evaluate seeds a Scope from input, runs the pipeline, and maps the final Scope
// into R. Pipeline/stage errors pass through unwrapped; mapping errors are a
// *MappingError; an input that cannot be flattened is a wrapped error.
func (e *Engine[I, R]) Evaluate(ctx context.Context, input I) (R, error) {
	var zero R

	seed, err := flatten(input)
	if err != nil {
		return zero, err
	}

	sc := stage.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return zero, err
	}
	return e.mapper.Map(sc.Snapshot())
}

// flatten converts input into a map[string]any Scope seed. A map[string]any is
// used directly; any other value (typically a struct) is decoded via
// mapstructure, preserving field types.
func flatten[I any](input I) (map[string]any, error) {
	if m, ok := any(input).(map[string]any); ok {
		return m, nil
	}
	var m map[string]any
	if err := mapstructure.Decode(input, &m); err != nil {
		return nil, fmt.Errorf("rlng: seed input: %w", err)
	}
	return m, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -race -run TestEngine -v 2>&1 | tail -30`
Expected: PASS (all subtests).

- [ ] **Step 5: Coverage + gates + commit**

```bash
go test . -race -cover     # expect >= 85%; confirm every branch above is hit
go vet . && gofmt -l .
git add engine.go engine_test.go
git commit -m "$(cat <<'MSG'
feat(rlng): Engine[I,R] facade with typed input seeding

Engine.Evaluate flattens a typed input into a Scope (map passthrough, or
mapstructure for structs), runs the *stage.Pipeline, and maps the final
Scope into R. New composes a built pipeline with a Mapper; WithScopeOptions
threads stage.ScopeOption. Pipeline/stage errors pass through unwrapped.
Realizes the Engine/Evaluate naming (ADR-0001).

Spec: 005
Plan: 005
ADR: 0009
MSG
)"
```

---

### Task 3: Package doc + runnable end-to-end example

**Files:**
- Create: `doc.go`, `example_test.go`

**Interfaces:**
- Consumes: `New`, `Evaluate`, `NewMapper`, `MappingTemplate` (Tasks 1–2); `stage.NewSingleExpr`, `stage.WithDependsOn`, `stage.NewPipeline`.
- Produces: nothing (documentation).

**Hot-path branches to cover:** the example runs input → pipeline → mapping end to end (success path) via `// Output:`.

- [ ] **Step 1: Write `doc.go`**

```go
// Package rlng is a pure-Go rule and calculation engine built on expr-lang/expr.
//
// An Engine[I, R] seeds a Scope from a typed input I, runs a stage.Pipeline
// (build one programmatically or from the config package), and projects the
// final Scope into a typed result R with a Mapper[R]. A MappingTemplate maps
// each output dot-path to an expression evaluated against the final Scope.
// Errors are typed and unwrap to the offending expression or field.
package rlng
```

- [ ] **Step 2: Write `example_test.go`**

```go
package rlng_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/stage"
)

type input struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type result struct {
	Total float64 `mapstructure:"total"`
}

func ExampleEngine() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	taxed, _ := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	pipeline, _ := stage.NewPipeline(base, taxed)

	mapper, _ := rlng.NewMapper[result](rlng.MappingTemplate{"total": "taxed"})
	engine := rlng.New[input, result](pipeline, mapper)

	out, err := engine.Evaluate(context.Background(), input{Price: 10, Qty: 2})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.1f\n", out.Total)
	// Output: 22.0
}
```

- [ ] **Step 3: Run to verify pass**

Run: `go test . -run Example -v`
Expected: PASS (`ExampleEngine` output `22.0`).

- [ ] **Step 4: Full gate + commit**

```bash
go test ./... -race
go vet ./... && gofmt -l . && go mod tidy && git diff --exit-code go.mod go.sum && go mod verify
git add doc.go example_test.go
git commit -m "$(cat <<'MSG'
docs(rlng): package doc and runnable end-to-end Engine example

Spec: 005
Plan: 005
MSG
)"
```

---

## Post-implementation (increment delivery — outside the task loop)

1. **Whole-branch gate:** `/code-review` over `main..HEAD`, then `/security-review` on the branch diff (mapstructure decode of typed inputs/results). Resolve/triage every finding. Confirm the coverage gate (≥85% on root `rlng`; every hot-path/typed-error branch tested).
2. **Re-run** `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` (clean), `go mod tidy` (no-op after mapstructure add), `go mod verify`, `govulncheck ./...` (if installed).
3. **Update** `docs/HANDOVER.md`: roadmap **complete** (all 5 increments merged); next = triage the release backlog, then tag `v0.0.1`.
4. **Merge** `feat/engine-facade` → `main` (fast-forward), **push**, and **delete** the branch.
