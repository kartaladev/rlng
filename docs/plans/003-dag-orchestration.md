# Stage DAG Orchestration (`Pipeline`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Pipeline` to the `stage` package that validates a set of `Stage`s, orders them by their declared `DependsOn()` (deterministic topological sort with cycle detection), and runs them sequentially against a shared `Scope`.

**Architecture:** A single new file `stage/pipeline.go` holds the `Pipeline` type, its constructor `NewPipeline` (which indexes stages, validates duplicate names and unknown dependencies, then computes an input-order-preserving topological order, detecting cycles and reporting a concrete loop path), and `Run` (a sequential ordered walk that honors `ctx` cancellation and stops at the first stage error). Three typed construction errors — `DuplicateStageError`, `UnknownDependencyError`, `CycleError` — keep the project's typed-error debuggability discipline. Execution is sequential and deterministic (ADR-0006); concurrency is deferred.

**Tech Stack:** Go 1.25+, standard library only (`context`, `fmt`, `strings`) plus the existing `stage` types. Tests use `github.com/stretchr/testify` (assert/require). No new dependency.

## Global Constraints

- Module path `github.com/kartaladev/rlng`; package `stage`. (Spec 003 §Design)
- **Pure Go, no cgo; no new third-party dependency** — `pipeline.go` imports only the standard library and the existing `stage` types. (Spec 003 §Dependencies)
- **No `os.Exit`/`log.Fatal`/`panic` on caller input** — return typed errors. (CLAUDE.md §What this is)
- **Typed, `errors.As`-reachable errors** naming the offending element (debuggability is the core criterion). (Spec 003 §Error model)
- **Sequential deterministic execution**; parallelism deferred. (ADR-0006)
- **Tests follow the `table-test` skill:** `assert` closure form (no `want`/`wantErr` fields); for `Run` (context-sensitive) use the `ctx` modifier with `t.Context()` and include a canceled-context case; `t.Parallel()` on the outer test and each subtest. (CLAUDE.md §Go conventions; table-test skill)
- **Test-coverage gate:** target ≥ 85% on `stage`; **hard requirement — every hot-path logic branch and every typed-error branch listed under each task must have a covering test case.** (CLAUDE.md §Test-coverage gate)
- Quality gates before the increment is delivered: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...`, `go test ./... -race`, `go mod tidy` a no-op. (CLAUDE.md §Library quality gates)

---

### Task 1: `Pipeline` — typed errors, validation, deterministic topo-order, and `Run`

**Files:**
- Create: `stage/pipeline.go`
- Test: `stage/pipeline_test.go`
- Also stage in this task's commit (ride-with-code, per CLAUDE.md commit discipline): `docs/adrs/0005-pipeline-orchestration.md`, `docs/adrs/0006-sequential-execution.md`, `docs/plans/003-dag-orchestration.md`

**Interfaces:**
- Consumes (from the existing `stage` package): `Stage` interface (`Name() string`, `Type() string`, `DependsOn() []string`, `Execute(ctx context.Context, sc *Scope) error`); `*Scope` with `NewScope(map[string]any, ...ScopeOption) *Scope`, `Set(path string, v any) error`, `Get(path string) (any, bool)`; `*StageError`; the `New*` stage constructors and `WithDependsOn(...string) Option` for building test fixtures.
- Produces (relied on by Task 2 and Increment 5):
  - `func NewPipeline(stages ...Stage) (*Pipeline, error)`
  - `func (p *Pipeline) Run(ctx context.Context, sc *Scope) error`
  - `type DuplicateStageError struct{ Name string }`
  - `type UnknownDependencyError struct{ Stage, Dependency string }`
  - `type CycleError struct{ Cycle []string }`

**Hot-path branches this task must cover with tests** (coverage gate):
- `NewPipeline`: duplicate name → `*DuplicateStageError`; unknown dependency → `*UnknownDependencyError`; cycle → `*CycleError` (self-loop, 2-node, and ≥3-node); valid acyclic set → `*Pipeline`; empty set → valid pipeline.
- `topoSort`: a pass emits ready stages; a pass emits nothing while stages remain → cycle branch; input-order preserved among independent stages; dependency ordering overrides declaration order.
- `findCycle`: following non-emitted dependencies until a stage repeats builds the concrete path (self-loop `["a","a"]`, 2-node `["a","b","a"]`, 3-node `["a","c","b","a"]`).
- `Run`: ctx already canceled before a stage → returns `ctx.Err()` and runs no stage; a stage's `Execute` errors → returns that error, later stages skipped; all succeed → nil (dependent reads dependency's Scope output); empty pipeline → nil.

- [ ] **Step 1: Write the failing tests**

Create `stage/pipeline_test.go`:

```go
package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordStage is a minimal Stage that appends its name to *order when executed
// and writes a marker into the Scope, so tests can observe execution order and
// dependency satisfaction. If execErr is non-nil, Execute returns it.
type recordStage struct {
	name    string
	deps    []string
	order   *[]string
	execErr error
}

func (s *recordStage) Name() string        { return s.name }
func (s *recordStage) Type() string        { return "record" }
func (s *recordStage) DependsOn() []string { return s.deps }
func (s *recordStage) Execute(ctx context.Context, sc *Scope) error {
	if s.execErr != nil {
		return s.execErr
	}
	*s.order = append(*s.order, s.name)
	return sc.Set(s.name, true)
}

func TestNewPipelineValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() []Stage
		assert func(t *testing.T, p *Pipeline, err error)
	}

	var order []string
	rs := func(name string, deps ...string) *recordStage {
		return &recordStage{name: name, deps: deps, order: &order}
	}

	cases := []testCase{
		{
			name:  "empty set is valid",
			build: func() []Stage { return nil },
			assert: func(t *testing.T, p *Pipeline, err error) {
				require.NoError(t, err)
				require.NotNil(t, p)
			},
		},
		{
			name:  "valid acyclic set constructs",
			build: func() []Stage { return []Stage{rs("a"), rs("b", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				require.NoError(t, err)
				require.NotNil(t, p)
			},
		},
		{
			name:  "duplicate name is rejected",
			build: func() []Stage { return []Stage{rs("a"), rs("a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var de *DuplicateStageError
				require.ErrorAs(t, err, &de)
				assert.Equal(t, "a", de.Name)
				assert.Equal(t, `pipeline: duplicate stage "a"`, de.Error())
			},
		},
		{
			name:  "unknown dependency is rejected",
			build: func() []Stage { return []Stage{rs("a", "ghost")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ue *UnknownDependencyError
				require.ErrorAs(t, err, &ue)
				assert.Equal(t, "a", ue.Stage)
				assert.Equal(t, "ghost", ue.Dependency)
				assert.Equal(t, `pipeline: stage "a" depends on unknown stage "ghost"`, ue.Error())
			},
		},
		{
			name:  "self dependency is a cycle",
			build: func() []Stage { return []Stage{rs("a", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, []string{"a", "a"}, ce.Cycle)
			},
		},
		{
			name:  "two node cycle reports concrete path",
			build: func() []Stage { return []Stage{rs("a", "b"), rs("b", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, []string{"a", "b", "a"}, ce.Cycle)
				assert.Equal(t, "pipeline: dependency cycle: a -> b -> a", ce.Error())
			},
		},
		{
			name:  "three node cycle is detected",
			build: func() []Stage { return []Stage{rs("a", "c"), rs("b", "a"), rs("c", "b")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				// The cycle closes on the repeated node.
				assert.Equal(t, ce.Cycle[0], ce.Cycle[len(ce.Cycle)-1])
				assert.Len(t, ce.Cycle, 4)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := NewPipeline(tc.build()...)
			tc.assert(t, p, err)
		})
	}
}

func TestPipelineRun(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		seed   map[string]any
		build  func(order *[]string) []Stage
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, order []string, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "runs in dependency order overriding declaration order",
			build: func(order *[]string) []Stage {
				// Declared b-before-a, but b depends on a, so a must run first.
				return []Stage{
					&recordStage{name: "b", deps: []string{"a"}, order: order},
					&recordStage{name: "a", order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"a", "b"}, order)
			},
		},
		{
			name: "independent stages preserve input order",
			build: func(order *[]string) []Stage {
				return []Stage{
					&recordStage{name: "x", order: order},
					&recordStage{name: "y", order: order},
					&recordStage{name: "z", order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"x", "y", "z"}, order)
			},
		},
		{
			name: "diamond runs dependencies before dependents",
			build: func(order *[]string) []Stage {
				// a -> {b, c} -> d
				return []Stage{
					&recordStage{name: "a", order: order},
					&recordStage{name: "b", deps: []string{"a"}, order: order},
					&recordStage{name: "c", deps: []string{"a"}, order: order},
					&recordStage{name: "d", deps: []string{"b", "c"}, order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"a", "b", "c", "d"}, order)
			},
		},
		{
			name: "dependent reads dependency output from scope",
			build: func(order *[]string) []Stage {
				a, err := NewSingleExpr("a", "21")
				require.NoError(t, err)
				b, err := NewSingleExpr("b", "a * 2", WithDependsOn("a"))
				require.NoError(t, err)
				return []Stage{b, a} // declared out of order on purpose
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				v, ok := sc.Get("b")
				require.True(t, ok)
				assert.Equal(t, 42, v)
			},
		},
		{
			name: "empty pipeline run is a no-op",
			build: func(order *[]string) []Stage {
				return nil
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Empty(t, order)
			},
		},
		{
			name: "first stage error stops the run",
			// x is a runtime value, so `x % 0` is not constant-folded at compile;
			// it fails at eval with an integer divide by zero (expr does float
			// division, so `/` would yield +Inf — modulo forces a real error).
			seed: map[string]any{"x": 1},
			build: func(order *[]string) []Stage {
				boom, err := NewSingleExpr("boom", "x % 0")
				require.NoError(t, err)
				return []Stage{
					boom,
					&recordStage{name: "after", deps: []string{"boom"}, order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "boom", se.Stage)
				assert.Empty(t, order) // "after" never ran
			},
		},
		{
			name: "canceled context short-circuits before any stage",
			build: func(order *[]string) []Stage {
				return []Stage{&recordStage{name: "a", order: order}}
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
				assert.Empty(t, order)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var order []string
			p, err := NewPipeline(tc.build(&order)...)
			require.NoError(t, err)

			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			sc := NewScope(tc.seed)
			runErr := p.Run(ctx, sc)
			tc.assert(t, order, sc, runErr)
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./stage/ -run 'TestNewPipelineValidation|TestPipelineRun'`
Expected: FAIL — compile error, `NewPipeline`/`Pipeline`/error types undefined.

- [ ] **Step 3: Write the implementation**

Create `stage/pipeline.go`:

```go
package stage

import (
	"context"
	"fmt"
	"strings"
)

// DuplicateStageError reports two stages sharing a Name within a Pipeline.
type DuplicateStageError struct{ Name string }

func (e *DuplicateStageError) Error() string {
	return fmt.Sprintf("pipeline: duplicate stage %q", e.Name)
}

// UnknownDependencyError reports a DependsOn target that names no stage in the
// pipeline's set.
type UnknownDependencyError struct {
	Stage      string
	Dependency string
}

func (e *UnknownDependencyError) Error() string {
	return fmt.Sprintf("pipeline: stage %q depends on unknown stage %q", e.Stage, e.Dependency)
}

// CycleError reports a dependency cycle among a Pipeline's stages. Cycle is the
// loop path, closing on the repeated stage (e.g. ["a", "b", "a"]).
type CycleError struct{ Cycle []string }

func (e *CycleError) Error() string {
	return "pipeline: dependency cycle: " + strings.Join(e.Cycle, " -> ")
}

// Pipeline runs a set of Stages in dependency order. NewPipeline validates the
// set and computes the execution order once; Run only evaluates. Execution is
// sequential and deterministic (see ADR-0006).
type Pipeline struct {
	ordered []Stage
}

// NewPipeline validates stages and computes their execution order. Stage names
// must be unique; every DependsOn target must name a stage in the set; and the
// dependency graph must be acyclic. It returns a *DuplicateStageError,
// *UnknownDependencyError, or *CycleError otherwise. An empty set is valid; its
// Run is a no-op.
func NewPipeline(stages ...Stage) (*Pipeline, error) {
	index := make(map[string]Stage, len(stages))
	for _, s := range stages {
		name := s.Name()
		if _, dup := index[name]; dup {
			return nil, &DuplicateStageError{Name: name}
		}
		index[name] = s
	}

	for _, s := range stages {
		for _, dep := range s.DependsOn() {
			if _, ok := index[dep]; !ok {
				return nil, &UnknownDependencyError{Stage: s.Name(), Dependency: dep}
			}
		}
	}

	ordered, err := topoSort(stages, index)
	if err != nil {
		return nil, err
	}
	return &Pipeline{ordered: ordered}, nil
}

// topoSort returns stages in dependency order, preserving input order among
// stages that become ready together (input-order-preserving Kahn). It assumes
// every DependsOn target exists in index. On a cycle it returns a *CycleError
// carrying a concrete loop path.
func topoSort(stages []Stage, index map[string]Stage) ([]Stage, error) {
	emitted := make(map[string]bool, len(stages))
	ordered := make([]Stage, 0, len(stages))

	for len(ordered) < len(stages) {
		progressed := false
		for _, s := range stages {
			if emitted[s.Name()] || !depsSatisfied(s, emitted) {
				continue
			}
			emitted[s.Name()] = true
			ordered = append(ordered, s)
			progressed = true
		}
		if !progressed {
			return nil, &CycleError{Cycle: findCycle(stages, index, emitted)}
		}
	}
	return ordered, nil
}

func depsSatisfied(s Stage, emitted map[string]bool) bool {
	for _, dep := range s.DependsOn() {
		if !emitted[dep] {
			return false
		}
	}
	return true
}

// findCycle returns one concrete cycle among the not-yet-emitted stages, as a
// path closing on the repeated stage (e.g. ["a", "b", "a"]). It is called only
// when topoSort stalls, so a cycle is guaranteed among the non-emitted stages.
//
// A non-emitted stage always has at least one non-emitted dependency (that is
// precisely why it could not be emitted). Following any such dependency stays
// within the non-emitted set, which is finite, so a stage must eventually
// repeat — and the path from that stage's first occurrence to its repeat is a
// cycle. Any leading tail (nodes that feed into but are not part of the cycle)
// is dropped by starting the returned slice at the first occurrence.
func findCycle(stages []Stage, index map[string]Stage, emitted map[string]bool) []string {
	var cur string
	for _, s := range stages {
		if !emitted[s.Name()] {
			cur = s.Name()
			break
		}
	}

	posOf := make(map[string]int, len(stages))
	var path []string
	for {
		if i, seen := posOf[cur]; seen {
			cycle := make([]string, 0, len(path)-i+1)
			cycle = append(cycle, path[i:]...)
			cycle = append(cycle, cur)
			return cycle
		}
		posOf[cur] = len(path)
		path = append(path, cur)

		// Follow the first non-emitted dependency; one is guaranteed to exist.
		for _, dep := range index[cur].DependsOn() {
			if !emitted[dep] {
				cur = dep
				break
			}
		}
	}
}

// Run executes the pipeline's stages in dependency order against sc, stopping at
// and returning the first stage error. It checks ctx before each stage and
// returns ctx.Err() (unwrapped) if the context is canceled; no further stages
// run. Built-in stages return a *StageError naming themselves, so the failing
// stage is identified without Run re-wrapping.
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error {
	for _, s := range p.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Execute(ctx, sc); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./stage/ -run 'TestNewPipelineValidation|TestPipelineRun' -race -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Verify branch coverage and gates**

Run:
```bash
go test ./stage/ -race -cover
go vet ./stage/
gofmt -l stage/pipeline.go stage/pipeline_test.go   # expect no output
```
Expected: tests PASS, coverage ≥ 85% on `stage`, `vet` clean, `gofmt` prints nothing. Confirm every hot-path branch listed for this task is exercised by a named subtest (self-loop, 2-node, 3-node cycle; duplicate; unknown dep; empty; dep-order; input-order; diamond; scope read; first-error-stops; canceled ctx).

- [ ] **Step 6: Commit**

```bash
git add stage/pipeline.go stage/pipeline_test.go \
  docs/adrs/0005-pipeline-orchestration.md docs/adrs/0006-sequential-execution.md \
  docs/plans/003-dag-orchestration.md docs/specs/003-dag-orchestration.md
git commit -m "$(cat <<'MSG'
feat(stage): Pipeline DAG orchestration with topo-sort and cycle detection

NewPipeline validates a set of Stages (duplicate names, unknown
dependencies) and computes a deterministic, input-order-preserving
topological order once at construction; a cycle yields a *CycleError with
a concrete loop path (DFS). Run walks that order sequentially, honoring
ctx cancellation between stages and stopping at the first stage error.
Sequential deterministic execution per ADR-0006; concurrency deferred.

Spec: 003
Plan: 003
ADR: 0005
ADR: 0006
MSG
)"
```

---

### Task 2: Runnable `Example…` tests (godoc)

**Files:**
- Test: `stage/pipeline_example_test.go`

**Interfaces:**
- Consumes: `NewPipeline`, `(*Pipeline).Run`, `CycleError` (from Task 1); `NewSingleExpr`, `WithDependsOn`, `NewScope`, `(*Scope).Get` (existing).
- Produces: nothing (documentation tests only).

**Hot-path branches this task must cover:** the examples double as documentation and exercise the success path (linear + diamond ordering) and the cycle-error reporting path end to end via `Example` output assertions.

- [ ] **Step 1: Write the failing example tests**

Create `stage/pipeline_example_test.go`. Use `context.Background()` (never a nil context — `Run` calls `ctx.Err()`, which panics on nil):

```go
package stage_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

// ExamplePipeline shows a linear pipeline where a later stage reads an earlier
// stage's output from the shared Scope; the pipeline orders by dependency, not
// declaration.
func ExamplePipeline() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	taxed, _ := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))

	p, _ := stage.NewPipeline(taxed, base) // declared out of order; ordered by deps
	sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	v, _ := sc.Get("taxed")
	fmt.Printf("%.1f\n", v)
	// Output: 22.0
}

// ExamplePipeline_cycle shows that a dependency cycle is reported at
// construction with the concrete loop path.
func ExamplePipeline_cycle() {
	a, _ := stage.NewSingleExpr("a", "b", stage.WithDependsOn("b"))
	b, _ := stage.NewSingleExpr("b", "a", stage.WithDependsOn("a"))

	_, err := stage.NewPipeline(a, b)
	var ce *stage.CycleError
	if errors.As(err, &ce) {
		fmt.Println(err)
	}
	// Output: pipeline: dependency cycle: a -> b -> a
}
```

- [ ] **Step 2: Run the examples to verify they fail then pass**

Run: `go test ./stage/ -run 'ExamplePipeline'`
Expected: FAIL first if Task 1 is not yet built; once Task 1 is in, PASS with output matching each `// Output:` block.

- [ ] **Step 3: Run the examples to verify they pass**

Run: `go test ./stage/ -run 'ExamplePipeline' -v`
Expected: PASS — both `ExamplePipeline` and `ExamplePipeline_cycle`, output matches `// Output:`.

- [ ] **Step 4: Full gate**

Run:
```bash
go test ./... -race
go vet ./...
gofmt -l .          # expect no output
```
Expected: all PASS/clean.

- [ ] **Step 5: Commit**

```bash
git add stage/pipeline_example_test.go
git commit -m "$(cat <<'MSG'
docs(stage): runnable Pipeline examples for godoc

Linear-ordering and cycle-error examples that double as package
documentation for the Pipeline orchestrator.

Spec: 003
Plan: 003
MSG
)"
```

---

## Post-implementation (increment delivery — outside the task loop)

Per CLAUDE.md §Development workflow and the HANDOVER per-increment recipe:

1. **Whole-branch gate:** `/code-review` over `main..HEAD` (dispatch a final reviewer), then `/security-review` on the branch diff. Resolve or explicitly triage every finding. Confirm the coverage gate (≥85% on `stage`; every hot-path branch tested).
2. **Re-run** `go test ./... -race` (green), `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` (clean if installed), `go mod tidy` (no-op), `govulncheck ./...` (if installed).
3. **Update** `docs/HANDOVER.md` at the increment boundary (state, next = Increment 4).
4. **Merge** `feat/dag-orchestration` → `main` (fast-forward, linear history), **push**, and **delete** the branch.
```
