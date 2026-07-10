# Scope + Stages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `rlng/stage` — the `Scope` accumulator and the three stage types (`SingleExpr`, `MultiExpr`, `DecisionTable`) that compose the increment-1 `expr` evaluators into reusable rule/calculation units.

**Architecture:** A concurrency-safe `map[string]any` accumulator (`Scope`) with dot-path `Set`/`Get`/`Snapshot` is threaded through stages. Each stage compiles its `expr.Predicate`/`expr.Function` expressions once at construction and, on `Execute(ctx, *Scope)`, reads a `Snapshot`, evaluates, and writes results back under a dot-path namespace keyed by the stage name. Stages only *declare* `DependsOn()`; ordering them into a DAG is Increment 3.

**Tech Stack:** Go 1.25+, `github.com/kartaladev/rlng/expr` (in-repo, increment 1), standard library (`context`, `sync`, `sort`, `strings`, `errors`, `fmt`). Tests use `github.com/stretchr/testify` (test-only, already present).

**Traceability:** Implements **Spec 002** (`docs/specs/002-scope-and-stages.md`), which builds on Spec 001. Records **ADR-0002** (stage execution model + `Scope` naming) in Task 2 and **ADR-0003** (decision-table hit policies) in Task 5. Every implementation commit carries a `Spec: 002` trailer (and an `ADR:` trailer on the tasks that record one).

## Global Constraints

- Module path `github.com/kartaladev/rlng`; Go **1.25+**.
- **Pure Go, no cgo:** `CGO_ENABLED=0 go build ./...` must pass.
- **No new runtime dependency.** `stage` imports only in-repo `expr` + the standard library. Do not add anything to `go.mod`'s `require` block; `go mod tidy` must stay a no-op.
- **Test-only dependency:** `github.com/stretchr/testify` (already present, test-scoped).
- Library code must not `os.Exit`/`log.Fatal`/`panic` on caller input, and must not log to a global logger — return typed errors.
- **Package name:** `stage`. Import the increment-1 package as `"github.com/kartaladev/rlng/expr"`.
- **Accumulator is named `Scope`** (not `Context`) — see ADR-0002. `Execute` takes `context.Context` as its first argument.
- **Test convention (mandatory):** follow `.claude/skills/table-test/SKILL.md` — a per-case `assert` closure (NOT `want`/`wantErr` fields), `testify` `require`/`assert`, `t.Parallel()` on the test and each subtest. **`Execute` is context-sensitive**, so its tables carry a `ctx func(context.Context) context.Context` modifier and include a canceled-context case, using `t.Context()` (never `context.Background()`). Constructor-only tables (`NewScope`, compile-error cases) need no `ctx` modifier. Add runnable `Example…` tests as godoc.
- **Pre-commit gate (CLAUDE.md §Development workflow):** before the *final* commit (Task 6), run `/code-review` over the whole-branch diff (`main..HEAD`) and `/security-review`, resolve/triage every finding, then `go test ./... -race`. Per-task commits at minimum run `go test ./... -race`.

## File Structure

```
docs/plans/002-scope-and-stages.md   # this plan (committed with Task 1's feat)
docs/adrs/0002-stage-execution-model.md   # Task 2
docs/adrs/0003-decision-table-hit-policies.md  # Task 5
stage/
  scope.go                # Scope + ScopeOption + WithStrict + path sentinels
  scope_test.go
  scope_example_test.go
  stage.go                # Stage interface, Type* constants, StageError
  stage_test.go
  single.go               # SingleExpr + options
  single_test.go
  single_example_test.go
  multi.go                # MultiExpr + NamedExpr + options
  multi_test.go
  multi_example_test.go
  table.go                # DecisionTable + Rule + HitPolicy + options
  table_test.go
  table_example_test.go
  doc.go                  # package doc
```

**Refinement vs Spec 002's file list:** the spec sketched a separate `errors.go`. This plan folds the scope path sentinels into `scope.go` (they are scope-specific) and `StageError` into `stage.go` (it lives with the interface it serves), avoiding a thin `errors.go`. Behavior and exported names are unchanged.

---

### Task 1: Scope accumulator

**Files:**
- Create: `stage/scope.go`, `stage/scope_test.go`, `stage/scope_example_test.go`
- Also commit: `docs/plans/002-scope-and-stages.md` (the plan rides with the first feat, per commit discipline).

**Interfaces:**
- Produces:
  - `type Scope` with `NewScope(seed map[string]any, opts ...ScopeOption) *Scope`, `(*Scope) Set(path string, v any) error`, `(*Scope) Get(path string) (any, bool)`, `(*Scope) Snapshot() map[string]any`.
  - `type ScopeOption func(*Scope)`; `WithStrict() ScopeOption`.
  - Sentinels `ErrPathConflict`, `ErrPathNotMap` (exported), `errEmptyPath` (unexported).

- [ ] **Step 1: Write the failing test** — `stage/scope_test.go`

```go
package stage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeSetGet(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		assert func(t *testing.T, s *Scope)
	}

	cases := []testCase{
		{
			name:  "single-segment set and get",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("amount", 100))
				got, ok := s.Get("amount")
				require.True(t, ok)
				assert.Equal(t, 100, got)
			},
		},
		{
			name:  "dot-path creates nested maps",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("discount.rate", 0.1))
				got, ok := s.Get("discount.rate")
				require.True(t, ok)
				assert.Equal(t, 0.1, got)
			},
		},
		{
			name:  "seed is read via get",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				got, ok := s.Get("a")
				require.True(t, ok)
				assert.Equal(t, 1, got)
			},
		},
		{
			name:  "missing path returns false",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Get("nope")
				assert.False(t, ok)
			},
		},
		{
			name:  "empty path is an error",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("", 1), errEmptyPath)
			},
		},
		{
			name:  "descend through scalar errors",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("a.b", 2), ErrPathNotMap)
			},
		},
		{
			name:  "lenient overwrite (default) wins last",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("a", 2))
				got, _ := s.Get("a")
				assert.Equal(t, 2, got)
			},
		},
		{
			name:  "strict overwrite conflicts",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}, WithStrict()) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("a", 2), ErrPathConflict)
			},
		},
		{
			name:  "snapshot is decoupled from later writes",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				snap := s.Snapshot()
				require.NoError(t, s.Set("b", 2))
				_, ok := snap["b"]
				assert.False(t, ok, "snapshot must not see writes made after it was taken")
			},
		},
		{
			name:  "seed copy protects against caller mutation",
			build: func() *Scope { return nil }, // built inline below
			assert: func(t *testing.T, _ *Scope) {
				seed := map[string]any{"a": 1}
				s := NewScope(seed)
				seed["a"] = 999
				got, _ := s.Get("a")
				assert.Equal(t, 1, got, "scope must not observe caller's post-construction seed mutation")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.build())
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run TestScopeSetGet -v`
Expected: FAIL — `undefined: NewScope` (and the other symbols).

- [ ] **Step 3: Write minimal implementation** — `stage/scope.go`

```go
// Package stage provides rlng's Scope accumulator and the stage types that
// compose the expr evaluators into reusable rule/calculation units.
package stage

import (
	"errors"
	"strings"
	"sync"
)

// ErrPathConflict is returned by Set, in strict mode, when a leaf path already
// holds a value.
var ErrPathConflict = errors.New("scope: path already set")

// ErrPathNotMap is returned when an intermediate dot-path segment exists but is
// not a map[string]any, so the path cannot be traversed.
var ErrPathNotMap = errors.New("scope: intermediate path is not a map")

// errEmptyPath is returned when Set is given an empty path.
var errEmptyPath = errors.New("scope: path must not be empty")

// Scope is a concurrency-safe map[string]any accumulator threaded through stage
// evaluation. Values are addressed by dot-separated paths; the accumulated map
// is the environment against which expressions are evaluated.
type Scope struct {
	mu     sync.RWMutex
	data   map[string]any
	strict bool
}

// ScopeOption configures a Scope.
type ScopeOption func(*Scope)

// WithStrict makes Set return ErrPathConflict when a leaf path already holds a
// value, guarding against accidental cross-stage output collisions.
func WithStrict() ScopeOption { return func(s *Scope) { s.strict = true } }

// NewScope returns a Scope seeded with a shallow copy of seed (nil is treated as
// empty). Nested structures inside seed are referenced, not cloned.
func NewScope(seed map[string]any, opts ...ScopeOption) *Scope {
	data := make(map[string]any, len(seed))
	for k, v := range seed {
		data[k] = v
	}
	s := &Scope{data: data}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Set stores v at the dot-separated path, creating intermediate maps as needed.
func (s *Scope) Set(path string, v any) error {
	if path == "" {
		return errEmptyPath
	}
	keys := strings.Split(path, ".")

	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.data
	for _, k := range keys[:len(keys)-1] {
		next, ok := m[k]
		if !ok {
			child := make(map[string]any)
			m[k] = child
			m = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return ErrPathNotMap
		}
		m = child
	}

	leaf := keys[len(keys)-1]
	if s.strict {
		if _, exists := m[leaf]; exists {
			return ErrPathConflict
		}
	}
	m[leaf] = v
	return nil
}

// Get returns the value at the dot-separated path and whether it was present.
func (s *Scope) Get(path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	keys := strings.Split(path, ".")

	s.mu.RLock()
	defer s.mu.RUnlock()

	var cur any = s.data
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Snapshot returns a shallow top-level copy of the accumulated data, suitable as
// an expr evaluation environment.
func (s *Scope) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./stage/ -run TestScopeSetGet -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Add a runnable example** — `stage/scope_example_test.go`

```go
package stage_test

import (
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleScope() {
	s := stage.NewScope(map[string]any{"amount": 150})
	_ = s.Set("discount.rate", 0.1)

	rate, _ := s.Get("discount.rate")
	fmt.Println(rate)
	// Output: 0.1
}
```

- [ ] **Step 6: Run the example**

Run: `go test ./stage/ -run ExampleScope -v`
Expected: PASS.

- [ ] **Step 7: Verify build + module hygiene**

Run: `CGO_ENABLED=0 go build ./... && go vet ./stage/ && go mod tidy`
Expected: builds clean; `go mod tidy` is a no-op (no new dependency — `scope.go` imports only the standard library).

- [ ] **Step 8: Commit**

```bash
git add stage/scope.go stage/scope_test.go stage/scope_example_test.go docs/plans/002-scope-and-stages.md
git commit -m "feat(stage): Scope accumulator with dot-path set/get" \
  -m "Concurrency-safe map[string]any accumulator with dot-path Set/Get, a decoupled Snapshot eval env, lenient-by-default overwrite plus WithStrict, and typed path sentinels. Carries the increment-2 plan." \
  -m "Spec: 002" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Stage interface + StageError + ADR-0002

**Files:**
- Create: `stage/stage.go`, `stage/stage_test.go`, `docs/adrs/0002-stage-execution-model.md`

**Interfaces:**
- Produces:
  - `type Stage interface { Name() string; Type() string; DependsOn() []string; Execute(ctx context.Context, sc *Scope) error }`.
  - Constants `TypeSingleExpr = "single-expr"`, `TypeMultiExpr = "multi-expr"`, `TypeDecisionTable = "decision-table"`.
  - `type StageError struct { Stage, Type string; Cause error }` implementing `error` + `Unwrap`.

- [ ] **Step 1: Write the failing test** — `stage/stage_test.go`

```go
package stage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageError(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")

	type testCase struct {
		name   string
		err    *StageError
		assert func(t *testing.T, err *StageError)
	}

	cases := []testCase{
		{
			name: "names stage and type and unwraps",
			err:  &StageError{Stage: "discount", Type: TypeSingleExpr, Cause: inner},
			assert: func(t *testing.T, err *StageError) {
				assert.Equal(t, `stage "discount" (single-expr): boom`, err.Error())
				require.ErrorIs(t, err, inner)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.err)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run TestStageError -v`
Expected: FAIL — `undefined: StageError` / `undefined: TypeSingleExpr`.

- [ ] **Step 3: Write minimal implementation** — `stage/stage.go`

```go
package stage

import (
	"context"
	"fmt"
)

// Stage types.
const (
	TypeSingleExpr    = "single-expr"
	TypeMultiExpr     = "multi-expr"
	TypeDecisionTable = "decision-table"
)

// Stage is a unit of rule/calculation logic that reads from and writes to a
// Scope. Stages compile their expressions at construction; Execute only
// evaluates. Implementations declare their dependencies via DependsOn for the
// DAG orchestrator (increment 3); this layer does not order stages.
type Stage interface {
	Name() string
	Type() string
	DependsOn() []string
	Execute(ctx context.Context, sc *Scope) error
}

// StageError reports a failure constructing or executing a stage. It names the
// stage and its type and unwraps to the underlying cause (typically an
// *expr.CompileError or *expr.EvalError).
type StageError struct {
	Stage string
	Type  string
	Cause error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("stage %q (%s): %s", e.Stage, e.Type, e.Cause.Error())
}

func (e *StageError) Unwrap() error { return e.Cause }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./stage/ -run TestStageError -v`
Expected: PASS.

- [ ] **Step 5: Write ADR-0002** — `docs/adrs/0002-stage-execution-model.md`

```markdown
# ADR-0002 — Stage execution model and `Scope` naming

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 002 (docs/specs/002-scope-and-stages.md)

## Context

Increment 2 introduces the layer above the atomic `expr` evaluators: an
accumulator threaded through evaluation, and stage types that read from and
write to it. Two decisions needed recording. First, what the stage abstraction
is and how a stage runs in isolation, given that dependency-ordered execution
(the DAG) is deferred to Increment 3. Second, what to call the accumulator: the
design brief called it "Context", but `Execute` takes a `context.Context`, so a
`*Context` second argument would put two different "Context" types in one
signature.

## Decision

- A `Stage` is `Name() / Type() / DependsOn() / Execute(ctx context.Context, sc *Scope) error`.
  Stages compile all expressions in their constructor (errors surface early) and
  only evaluate in `Execute`. `Execute` reads a `Scope` snapshot, evaluates, and
  writes results back under a dot-path namespace keyed by the stage name.
- Stages **declare** `DependsOn()` but do not act on it; ordering stages into a
  dependency DAG (topo-sort + cycle detection) is Increment 3. Each stage is
  independently constructible and executable this increment.
- The accumulator is named **`Scope`**, not `Context`, to avoid the
  double-`Context` signature and to respect Go's convention that `Context` means
  `context.Context`. `Scope` names "the variables in scope during evaluation,
  growing as each stage contributes". This realizes the brief's "Context
  accumulator" under a clearer name.
- Failures are a typed `StageError` that names the stage and unwraps to the
  underlying `expr` error, preserving the field+expression debuggability chain.

## Consequences

- Stages are testable in isolation now; the DAG runner in Increment 3 consumes
  the already-declared `DependsOn()` without changing the stage contract.
- CLAUDE.md's architecture blueprint (which uses "Context") is refreshed to say
  `Scope` so the docs do not drift.
- Supersede this ADR rather than editing it if the stage contract changes.
```

- [ ] **Step 6: Run the suite with the race detector**

Run: `go test ./... -race`
Expected: PASS, no data races.

- [ ] **Step 7: Commit**

```bash
git add stage/stage.go stage/stage_test.go docs/adrs/0002-stage-execution-model.md
git commit -m "feat(stage): Stage interface and typed StageError" \
  -m "Defines the Name/Type/DependsOn/Execute contract, the Type* constants, and StageError (unwraps to the expr cause). Records ADR-0002 (stage execution model + Scope naming)." \
  -m "Spec: 002" -m "ADR: 0002" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: SingleExpr stage

**Files:**
- Create: `stage/single.go`, `stage/single_test.go`, `stage/single_example_test.go`

**Interfaces:**
- Consumes: `Scope`, `Stage`, `StageError`, `TypeSingleExpr`, and `expr.NewFunction`/`expr.NewPredicate`/`expr.Option`.
- Produces:
  - `type SingleExpr` implementing `Stage`; `NewSingleExpr(name, expression string, opts ...Option) (*SingleExpr, error)`.
  - `type Option`; `WithOutput(path string)`, `WithCondition(condition string, opts ...expr.Option)`, `WithExprOptions(opts ...expr.Option)`, `WithDependsOn(deps ...string)`.

- [ ] **Step 1: Write the failing test** — `stage/single_test.go`

```go
package stage

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleExprExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*SingleExpr, *Scope)
		ctx    func(ctx context.Context) context.Context // nil = identity
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "computes and writes to stage name by default",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("total")
				require.True(t, ok)
				assert.Equal(t, 30, got)
			},
		},
		{
			name: "custom output path",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty", WithOutput("order.total"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("order.total")
				require.True(t, ok)
				assert.Equal(t, 30, got)
			},
		},
		{
			name: "condition false skips write",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("bonus", "100", WithCondition("vip"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"vip": false})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("bonus")
				assert.False(t, ok, "skipped stage must write nothing")
			},
		},
		{
			name: "condition true writes",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("bonus", "100", WithCondition("vip"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"vip": true})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("bonus")
				require.True(t, ok)
				assert.Equal(t, 100, got)
			},
		},
		{
			name: "fallback on eval error",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("ratio", "a / b",
					WithExprOptions(expr.WithFallback("0.0")))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("ratio")
				require.True(t, ok)
				assert.Equal(t, 0.0, got)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("ratio", "a / b")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "ratio", se.Stage)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
				_, ok := sc.Get("total")
				assert.False(t, ok)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := s.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewSingleExprCompileError(t *testing.T) {
	t.Parallel()

	_, err := NewSingleExpr("bad", "x +")
	var se *StageError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, TypeSingleExpr, se.Type)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run 'TestSingleExpr|TestNewSingleExpr' -v`
Expected: FAIL — `undefined: NewSingleExpr`.

- [ ] **Step 3: Write minimal implementation** — `stage/single.go`

```go
package stage

import (
	"context"

	"github.com/kartaladev/rlng/expr"
)

// SingleExpr is a stage that evaluates one value expression, optionally gated by
// a condition predicate, writing the result to an output path in the Scope.
type SingleExpr struct {
	name   string
	output string
	deps   []string
	cond   *expr.Predicate
	fn     *expr.Function
}

type singleExprConfig struct {
	output    string
	deps      []string
	condition string
	condOpts  []expr.Option
	exprOpts  []expr.Option
}

// Option configures a SingleExpr.
type Option func(*singleExprConfig)

// WithOutput sets the Scope path the result is written to (default: stage name).
func WithOutput(path string) Option {
	return func(c *singleExprConfig) { c.output = path }
}

// WithCondition gates the stage on a boolean predicate; when it tests false the
// stage writes nothing.
func WithCondition(condition string, opts ...expr.Option) Option {
	return func(c *singleExprConfig) { c.condition = condition; c.condOpts = opts }
}

// WithExprOptions passes options to the main value expression (e.g.
// expr.WithFallback, expr.WithGlobals).
func WithExprOptions(opts ...expr.Option) Option {
	return func(c *singleExprConfig) { c.exprOpts = opts }
}

// WithDependsOn declares the stages this stage depends on (consumed by the DAG
// in increment 3).
func WithDependsOn(deps ...string) Option {
	return func(c *singleExprConfig) { c.deps = deps }
}

// NewSingleExpr compiles a SingleExpr stage. Compilation of the value
// expression and any condition happens now; Execute only evaluates.
func NewSingleExpr(name, expression string, opts ...Option) (*SingleExpr, error) {
	cfg := &singleExprConfig{output: name}
	for _, opt := range opts {
		opt(cfg)
	}

	fn, err := expr.NewFunction(name, expression, cfg.exprOpts...)
	if err != nil {
		return nil, &StageError{Stage: name, Type: TypeSingleExpr, Cause: err}
	}

	s := &SingleExpr{name: name, output: cfg.output, deps: cfg.deps, fn: fn}

	if cfg.condition != "" {
		cond, err := expr.NewPredicate(cfg.condition, cfg.condOpts...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeSingleExpr, Cause: err}
		}
		s.cond = cond
	}
	return s, nil
}

func (s *SingleExpr) Name() string        { return s.name }
func (s *SingleExpr) Type() string        { return TypeSingleExpr }
func (s *SingleExpr) DependsOn() []string { return s.deps }

// Execute evaluates the stage against sc. A configured condition that tests
// false makes the stage a no-op.
func (s *SingleExpr) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}

	env := sc.Snapshot()

	if s.cond != nil {
		ok, err := s.cond.Test(env)
		if err != nil {
			return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
		}
		if !ok {
			return nil
		}
	}

	v, err := s.fn.Apply(env)
	if err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}
	if err := sc.Set(s.output, v); err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./stage/ -run 'TestSingleExpr|TestNewSingleExpr' -v`
Expected: PASS.

- [ ] **Step 5: Add a runnable example** — `stage/single_example_test.go`

```go
package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleSingleExpr() {
	s, err := stage.NewSingleExpr("total", "price * qty")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"price": 10, "qty": 3})
	if err := s.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	total, _ := sc.Get("total")
	fmt.Println(total)
	// Output: 30
}
```

Note: `Example…` tests use `context.TODO()` for self-containment — the `t.Context()` rule applies to `Test…` functions, not examples.

- [ ] **Step 6: Run the example**

Run: `go test ./stage/ -run ExampleSingleExpr -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add stage/single.go stage/single_test.go stage/single_example_test.go
git commit -m "feat(stage): SingleExpr stage with condition gate" \
  -m "One value expression with an optional condition predicate (false = no-op), pass-through expr options (fallback/globals), and a configurable output path defaulting to the stage name." \
  -m "Spec: 002" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: MultiExpr stage

**Files:**
- Create: `stage/multi.go`, `stage/multi_test.go`, `stage/multi_example_test.go`

**Interfaces:**
- Consumes: `Scope`, `Stage`, `StageError`, `TypeMultiExpr`, `expr.NewFunction`/`expr.Option`.
- Produces:
  - `type NamedExpr struct { Name, Expression string; Priority int; Options []expr.Option }`.
  - `type MultiExpr` implementing `Stage`; `NewMultiExpr(name string, exprs []NamedExpr, opts ...Option) (*MultiExpr, error)`.
  - `type Option`; `WithDependsOn(deps ...string)`.

- [ ] **Step 1: Write the failing test** — `stage/multi_test.go`

```go
package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiExprExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*MultiExpr, *Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "writes each result under stage.name",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
					{Name: "taxed", Expression: "base * 1.1", Priority: 1},
				})
				require.NoError(t, err)
				return m, NewScope(map[string]any{"price": 10.0, "qty": 2.0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				base, ok := sc.Get("calc.base")
				require.True(t, ok)
				assert.Equal(t, 20.0, base)
				taxed, ok := sc.Get("calc.taxed")
				require.True(t, ok)
				assert.InDelta(t, 22.0, taxed, 1e-9)
			},
		},
		{
			name: "priority controls visibility order",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				// 'taxed' references 'base'; declaring it first but with a higher
				// priority number must still evaluate 'base' first.
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "taxed", Expression: "base * 2", Priority: 10},
					{Name: "base", Expression: "5", Priority: 1},
				})
				require.NoError(t, err)
				return m, NewScope(nil)
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				taxed, ok := sc.Get("calc.taxed")
				require.True(t, ok)
				assert.Equal(t, 10, taxed)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				m, err := NewMultiExpr("calc", []NamedExpr{{Name: "x", Expression: "1"}})
				require.NoError(t, err)
				return m, NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := m.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewMultiExprValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		exprs  []NamedExpr
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:  "empty set is rejected",
			exprs: nil,
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:  "empty name is rejected",
			exprs: []NamedExpr{{Name: "", Expression: "1"}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "duplicate name is rejected",
			exprs: []NamedExpr{
				{Name: "a", Expression: "1"},
				{Name: "a", Expression: "2"},
			},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewMultiExpr("calc", tc.exprs)
			tc.assert(t, err)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run 'TestMultiExpr|TestNewMultiExpr' -v`
Expected: FAIL — `undefined: NewMultiExpr`.

- [ ] **Step 3: Write minimal implementation** — `stage/multi.go`

```go
package stage

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kartaladev/rlng/expr"
)

// NamedExpr is one entry in a MultiExpr stage.
type NamedExpr struct {
	Name       string
	Expression string
	Priority   int
	Options    []expr.Option
}

// MultiExpr evaluates several named expressions in ascending priority order,
// each visible to later ones within the stage, writing each result to
// name.<exprName> in the Scope.
type MultiExpr struct {
	name  string
	deps  []string
	exprs []compiledNamed
}

type compiledNamed struct {
	name string
	fn   *expr.Function
}

type multiExprConfig struct {
	deps []string
}

// Option configures a MultiExpr.
type Option func(*multiExprConfig)

// WithDependsOn declares the stages this stage depends on (consumed by the
// DAG in increment 3).
func WithDependsOn(deps ...string) Option {
	return func(c *multiExprConfig) { c.deps = deps }
}

// NewMultiExpr compiles a MultiExpr stage. Expression names must be non-empty
// and unique within the stage; entries are ordered by ascending Priority
// (stable for ties).
func NewMultiExpr(name string, exprs []NamedExpr, opts ...Option) (*MultiExpr, error) {
	if len(exprs) == 0 {
		return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: errors.New("multi-expr requires at least one expression")}
	}
	cfg := &multiExprConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	ordered := make([]NamedExpr, len(exprs))
	copy(ordered, exprs)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })

	seen := make(map[string]struct{}, len(ordered))
	compiled := make([]compiledNamed, 0, len(ordered))
	for _, e := range ordered {
		if e.Name == "" {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: errors.New("expression name must not be empty")}
		}
		if _, dup := seen[e.Name]; dup {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: fmt.Errorf("duplicate expression name %q", e.Name)}
		}
		seen[e.Name] = struct{}{}

		fn, err := expr.NewFunction(e.Name, e.Expression, e.Options...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: err}
		}
		compiled = append(compiled, compiledNamed{name: e.Name, fn: fn})
	}
	return &MultiExpr{name: name, deps: cfg.deps, exprs: compiled}, nil
}

func (m *MultiExpr) Name() string        { return m.name }
func (m *MultiExpr) Type() string        { return TypeMultiExpr }
func (m *MultiExpr) DependsOn() []string { return m.deps }

// Execute evaluates the expressions in priority order. Each result is visible to
// later expressions in this stage (by its bare name) and persisted to the Scope
// under name.<exprName>.
func (m *MultiExpr) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
	}

	env := sc.Snapshot()
	for _, e := range m.exprs {
		v, err := e.fn.Apply(env)
		if err != nil {
			return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
		}
		env[e.name] = v // visible to later expressions within this stage
		if err := sc.Set(m.name+"."+e.name, v); err != nil {
			return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./stage/ -run 'TestMultiExpr|TestNewMultiExpr' -v`
Expected: PASS.

- [ ] **Step 5: Add a runnable example** — `stage/multi_example_test.go`

```go
package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleMultiExpr() {
	m, err := stage.NewMultiExpr("calc", []stage.NamedExpr{
		{Name: "base", Expression: "price * qty", Priority: 0},
		{Name: "taxed", Expression: "base * 1.1", Priority: 1},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := m.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	taxed, _ := sc.Get("calc.taxed")
	fmt.Println(taxed)
	// Output: 22
}
```

- [ ] **Step 6: Run the example**

Run: `go test ./stage/ -run ExampleMultiExpr -v`
Expected: PASS. (`22.00000000000000...` — if the float prints with trailing noise, adjust the `// Output:` line to the exact printed value observed, or format via `fmt.Printf("%.1f\n", taxed)` and set `// Output: 22.0`.)

- [ ] **Step 7: Commit**

```bash
git add stage/multi.go stage/multi_test.go stage/multi_example_test.go
git commit -m "feat(stage): MultiExpr stage with priority ordering" \
  -m "Several named expressions evaluated in ascending priority order, each visible to later ones within the stage, persisted under name.<exprName>. Rejects empty/duplicate names." \
  -m "Spec: 002" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: DecisionTable stage + ADR-0003

**Files:**
- Create: `stage/table.go`, `stage/table_test.go`, `stage/table_example_test.go`, `docs/adrs/0003-decision-table-hit-policies.md`

**Interfaces:**
- Consumes: `Scope`, `Stage`, `StageError`, `TypeDecisionTable`, `expr.NewPredicate`/`expr.NewFunction`/`expr.Option`.
- Produces:
  - `type HitPolicy int`; constants `HitPolicySingle` (iota 0, default), `HitPolicyCollect`.
  - `type Rule struct { Condition string; Decisions map[string]string; ConditionOptions, DecisionOptions []expr.Option }`.
  - `type DecisionTable` implementing `Stage`; `NewDecisionTable(name string, rules []Rule, opts ...Option) (*DecisionTable, error)`.
  - `type Option`; `WithHitPolicy(m HitPolicy)`, `WithDependsOn(deps ...string)`.

- [ ] **Step 1: Write the failing test** — `stage/table_test.go`

```go
package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionTableExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*DecisionTable, *Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "single mode: first match wins",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
					{Condition: "amount >= 100", Decisions: map[string]string{"level": `"silver"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				level, ok := sc.Get("tier.level")
				require.True(t, ok)
				assert.Equal(t, "gold", level)
			},
		},
		{
			name: "single mode: no match writes nothing",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("tier.level")
				assert.False(t, ok)
			},
		},
		{
			name: "collect mode: accumulates matches in rule order",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": `"big"`}},
					{Condition: "amount >= 1000", Decisions: map[string]string{"tag": `"huge"`}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "huge"}, tags)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("t", []Rule{
					{Condition: "a / b > 1", Decisions: map[string]string{"x": "1"}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "t", se.Stage)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "true", Decisions: map[string]string{"x": "1"}},
				})
				require.NoError(t, err)
				return d, NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := d.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewDecisionTableValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		rules  []Rule
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:  "empty rule set is rejected",
			rules: nil,
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:  "rule without decisions is rejected",
			rules: []Rule{{Condition: "true", Decisions: nil}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:  "bad condition is a compile error",
			rules: []Rule{{Condition: "x +", Decisions: map[string]string{"y": "1"}}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewDecisionTable("t", tc.rules)
			tc.assert(t, err)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run 'TestDecisionTable|TestNewDecisionTable' -v`
Expected: FAIL — `undefined: NewDecisionTable`.

- [ ] **Step 3: Write minimal implementation** — `stage/table.go`

```go
package stage

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kartaladev/rlng/expr"
)

// HitPolicy selects how a DecisionTable resolves matching rules.
type HitPolicy int

const (
	// HitPolicySingle applies the first matching rule's decisions and stops.
	HitPolicySingle HitPolicy = iota
	// HitPolicyCollect applies every matching rule; each output key accumulates a
	// []any with one entry per matched rule, in rule order.
	HitPolicyCollect
)

// Rule is one row of a DecisionTable: a boolean condition and a set of
// output-key -> value-expression decisions.
type Rule struct {
	Condition        string
	Decisions        map[string]string
	ConditionOptions []expr.Option
	DecisionOptions  []expr.Option
}

// DecisionTable evaluates ordered rules against a Scope snapshot, writing
// decision outputs under name.<outputKey>.
type DecisionTable struct {
	name  string
	deps  []string
	mode  HitPolicy
	rules []compiledRule
}

type compiledRule struct {
	cond      *expr.Predicate
	decisions []compiledDecision
}

type compiledDecision struct {
	key string
	fn  *expr.Function
}

type decisionTableConfig struct {
	mode HitPolicy
	deps []string
}

// Option configures a DecisionTable.
type Option func(*decisionTableConfig)

// WithHitPolicy sets the hit policy (default HitPolicySingle).
func WithHitPolicy(m HitPolicy) Option {
	return func(c *decisionTableConfig) { c.mode = m }
}

// WithDependsOn declares the stages this stage depends on (consumed by the
// DAG in increment 3).
func WithDependsOn(deps ...string) Option {
	return func(c *decisionTableConfig) { c.deps = deps }
}

// NewDecisionTable compiles a DecisionTable stage. Every condition and decision
// is compiled up front. Within a rule, decisions are independent (evaluated
// against the same pre-rule snapshot), so their order is not significant; they
// are compiled in sorted-key order for deterministic collect output.
func NewDecisionTable(name string, rules []Rule, opts ...Option) (*DecisionTable, error) {
	if len(rules) == 0 {
		return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: errors.New("decision-table requires at least one rule")}
	}
	cfg := &decisionTableConfig{mode: HitPolicySingle}
	for _, opt := range opts {
		opt(cfg)
	}

	compiled := make([]compiledRule, 0, len(rules))
	for i, r := range rules {
		if len(r.Decisions) == 0 {
			return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d has no decisions", i)}
		}
		cond, err := expr.NewPredicate(r.Condition, r.ConditionOptions...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d condition: %w", i, err)}
		}

		decisions := make([]compiledDecision, 0, len(r.Decisions))
		for _, key := range sortedKeys(r.Decisions) {
			if key == "" {
				return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d has an empty output key", i)}
			}
			fn, err := expr.NewFunction(key, r.Decisions[key], r.DecisionOptions...)
			if err != nil {
				return nil, &StageError{Stage: name, Type: TypeDecisionTable, Cause: fmt.Errorf("rule %d decision %q: %w", i, key, err)}
			}
			decisions = append(decisions, compiledDecision{key: key, fn: fn})
		}
		compiled = append(compiled, compiledRule{cond: cond, decisions: decisions})
	}
	return &DecisionTable{name: name, deps: cfg.deps, mode: cfg.mode, rules: compiled}, nil
}

func (d *DecisionTable) Name() string        { return d.name }
func (d *DecisionTable) Type() string        { return TypeDecisionTable }
func (d *DecisionTable) DependsOn() []string { return d.deps }

// Execute evaluates the rules against a Scope snapshot per the hit policy.
func (d *DecisionTable) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
	}

	env := sc.Snapshot()
	if d.mode == HitPolicyCollect {
		return d.executeCollect(env, sc)
	}
	return d.executeSingle(env, sc)
}

func (d *DecisionTable) executeSingle(env map[string]any, sc *Scope) error {
	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
		if !ok {
			continue
		}
		for _, dec := range r.decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
			if err := sc.Set(d.name+"."+dec.key, v); err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
		}
		return nil // first match wins
	}
	return nil
}

func (d *DecisionTable) executeCollect(env map[string]any, sc *Scope) error {
	collected := make(map[string][]any)
	order := make([]string, 0)
	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
		if !ok {
			continue
		}
		for _, dec := range r.decisions {
			v, err := dec.fn.Apply(env)
			if err != nil {
				return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
			}
			if _, seen := collected[dec.key]; !seen {
				order = append(order, dec.key)
			}
			collected[dec.key] = append(collected[dec.key], v)
		}
	}
	for _, key := range order {
		if err := sc.Set(d.name+"."+key, collected[key]); err != nil {
			return &StageError{Stage: d.name, Type: TypeDecisionTable, Cause: err}
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./stage/ -run 'TestDecisionTable|TestNewDecisionTable' -v`
Expected: PASS.

- [ ] **Step 5: Add a runnable example** — `stage/table_example_test.go`

```go
package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleDecisionTable() {
	d, err := stage.NewDecisionTable("tier", []stage.Rule{
		{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
		{Condition: "amount >= 100", Decisions: map[string]string{"level": `"silver"`}},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"amount": 5000})
	if err := d.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	level, _ := sc.Get("tier.level")
	fmt.Println(level)
	// Output: gold
}
```

- [ ] **Step 6: Run the example**

Run: `go test ./stage/ -run ExampleDecisionTable -v`
Expected: PASS.

- [ ] **Step 7: Write ADR-0003** — `docs/adrs/0003-decision-table-hit-policies.md`

```markdown
# ADR-0003 — Decision-table hit policies

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 002 (docs/specs/002-scope-and-stages.md)

## Context

The DecisionTable stage evaluates ordered rules, each a condition plus a set of
output-key -> value-expression decisions. Two semantic questions needed
recording: how multiple matching rules resolve, and whether decisions within a
single rule may depend on one another.

## Decision

- **Hit policy** is selected by `WithHitPolicy`, defaulting to `HitPolicySingle`:
  - `HitPolicySingle` — first-match-wins: the first rule whose condition tests true
    has its decisions applied under `name.<outputKey>`, and evaluation stops. No
    match writes nothing.
  - `HitPolicyCollect` — every matching rule contributes; each output key accumulates
    a `[]any` with one entry per matched rule, in rule order (DMN COLLECT
    semantics). No match writes nothing.
- **Decisions within a rule are independent**: all are evaluated against the same
  pre-rule Scope snapshot, so decision order is not significant. This is why
  `Rule.Decisions` is a plain `map[string]string`. Decisions are compiled and
  evaluated in sorted-key order purely for deterministic output.

## Consequences

- Collect output is always a list per key, even for a single match, so consumers
  read a stable shape.
- A decision cannot read another decision's output within the same rule; chain
  such dependencies across stages (or a MultiExpr) instead. Revisit with a
  superseding ADR if intra-rule decision dependencies become necessary.
```

- [ ] **Step 8: Run the suite with the race detector**

Run: `go test ./... -race`
Expected: PASS, no data races.

- [ ] **Step 9: Commit**

```bash
git add stage/table.go stage/table_test.go stage/table_example_test.go docs/adrs/0003-decision-table-hit-policies.md
git commit -m "feat(stage): DecisionTable stage with single/collect hit policies" \
  -m "Ordered condition+decisions rules; HitPolicySingle first-match-wins and HitPolicyCollect ([]any per output key, rule order). Decisions within a rule are independent. Records ADR-0003." \
  -m "Spec: 002" -m "ADR: 0003" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Package doc + docs refresh + full quality gate

**Files:**
- Create: `stage/doc.go`
- Modify: `CLAUDE.md` (blueprint wording: "Context" → "Scope"), `docs/specs/002-scope-and-stages.md` (Traceability: link the realized plan), `docs/HANDOVER.md` (mark increment 2 complete)

**Interfaces:**
- Consumes: everything above. Produces no new API.

- [ ] **Step 1: Write the package doc** — `stage/doc.go`

```go
// Package stage provides rlng's Scope accumulator and the stage types that
// compose the expr evaluators (github.com/kartaladev/rlng/expr) into reusable
// rule and calculation units.
//
// Scope is a concurrency-safe map[string]any accumulator addressed by
// dot-separated paths (Set/Get), with a decoupled Snapshot that serves as the
// expression evaluation environment.
//
// A Stage is Name/Type/DependsOn/Execute. Three implementations are provided:
// SingleExpr (one value expression with an optional condition gate), MultiExpr
// (several named expressions in priority order, each visible to later ones), and
// DecisionTable (ordered condition+decisions rules with HitPolicySingle first-match
// or HitPolicyCollect accumulation). Stages compile at construction and only evaluate
// in Execute; failures are a *StageError that unwraps to the underlying expr
// error. Stages declare DependsOn but do not order themselves — dependency-DAG
// orchestration is a later increment.
package stage
```

- [ ] **Step 2: Refresh CLAUDE.md and spec/handover cross-links**

- In `CLAUDE.md`, the "Architecture blueprint" bullet that describes the accumulator as **Context** (`context.go`): update the wording to note the increment-2 implementation names it **`Scope`** (package `stage`), so the blueprint doesn't drift from the code. Keep it a one-line clarification, not a rewrite.
- In `docs/specs/002-scope-and-stages.md`, change the **Realized by plans** / Traceability lines to reference `docs/plans/002-scope-and-stages.md` (drop "pending").
- In `docs/HANDOVER.md`, mark Increment 2 complete and point "Next" at Increment 3 (Stage DAG orchestration). (Handover is refreshed at the end of the increment, per CLAUDE.md.)

- [ ] **Step 3: Run the whole suite with the race detector**

Run: `go test ./... -race`
Expected: PASS, no data races.

- [ ] **Step 4: Run the library quality gates**

Run:
```bash
CGO_ENABLED=0 go build ./...
go vet ./...
gofmt -l .            # expect no output
go mod tidy           # expect go.mod/go.sum unchanged
```
Expected: all clean. If `golangci-lint` / `govulncheck` are installed, run `golangci-lint run ./...` and `govulncheck ./...` too and resolve anything flagged.

- [ ] **Step 5: Whole-branch pre-commit review gate (CLAUDE.md)**

Run `/code-review` over the whole-branch diff (`main..HEAD`) and address findings; then `/security-review` on the pending changes and resolve anything flagged; then re-run `go test ./... -race`. Resolve or explicitly triage every finding before proceeding.

- [ ] **Step 6: Commit**

```bash
git add stage/doc.go CLAUDE.md docs/specs/002-scope-and-stages.md docs/HANDOVER.md
git commit -m "docs(stage): package doc and increment-2 cross-links" \
  -m "Adds the stage package doc, refreshes the CLAUDE.md blueprint wording (Context -> Scope), links spec 002 to its plan, and marks increment 2 complete in the handover." \
  -m "Spec: 002" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**1. Spec coverage:**
- `Scope` (dot-path Set/Get, Snapshot, lenient/`WithStrict`, seed copy, path sentinels) → Task 1. ✓
- `Stage` interface + `Type*` constants + `StageError` (Unwrap to expr) → Task 2. ✓
- `SingleExpr` (condition gate, fallback via expr options, custom output) → Task 3. ✓
- `MultiExpr` (priority order, intra-stage visibility, name validation) → Task 4. ✓
- `DecisionTable` (HitPolicySingle/HitPolicyCollect, independent decisions, validation) → Task 5. ✓
- `context.Context` threading + cancellation cases → Tasks 3–5 (`ctx` modifier). ✓
- No new dependency; `go mod tidy` no-op → Tasks 1 & 6 hygiene steps. ✓
- ADR-0002 (model + Scope name) → Task 2; ADR-0003 (hit policies) → Task 5. ✓
- Testing: table-test `assert`-closure + `ctx` modifier + Example tests + `-race` → all tasks + Task 6. ✓
- Non-goals (no DAG/config/facade/adapter) → nothing in the plan builds them. ✓

**2. Placeholder scan:** No TBD/TODO. Every code step contains complete code. The one flagged uncertainty (the `ExampleMultiExpr` float `// Output:` line) has an explicit fallback instruction, not a placeholder.

**3. Type consistency:** `Scope`/`NewScope`/`Set`/`Get`/`Snapshot`/`WithStrict`, `Stage`/`StageError`/`Type*`, `SingleExpr`/`NewSingleExpr`/`WithOutput`/`WithCondition`/`WithExprOptions`/`WithDependsOn`, `NamedExpr`/`MultiExpr`/`NewMultiExpr`/`WithDependsOn`, `Rule`/`HitPolicy`/`HitPolicySingle`/`HitPolicyCollect`/`DecisionTable`/`NewDecisionTable`/`WithHitPolicy`/`WithDependsOn` are defined once and used consistently across tasks. The internal `sortedKeys`, `compiledRule`, `compiledDecision`, `compiledNamed`, and `*Config` types are used only within their defining task.

## Notes / deviations

- **File-layout refinement vs Spec 002:** scope sentinels live in `scope.go` and `StageError` in `stage.go` rather than a separate `errors.go` (noted in File Structure). Exported names unchanged.
- **`context.Context` in examples:** `Example…` tests use `context.TODO()` for self-containment; the `t.Context()` rule applies to `Test…` functions, not examples.
- **Cancellation granularity:** stages check `ctx.Err()` once before evaluating. The `expr` VM calls are fast and synchronous, so finer-grained mid-evaluation cancellation is unnecessary at this layer (consistent with Spec 001's deferral rationale).
