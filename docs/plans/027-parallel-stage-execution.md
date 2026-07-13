# Parallel Stage Execution (B11) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add opt-in parallel execution of independent DAG stages to `pipe.Pipeline` as a pure speedup that preserves every observable of sequential execution (data, surfaced error, reported stage order).

**Architecture:** Consolidate `NewPipeline` onto a `(stages []Stage, opts ...PipelineOption)` signature; move `WithRuleset` from a fluent method to an option; add `WithConcurrency()` / `WithMaxParallel(n)`. `Run` branches: sequential (default, unchanged) or wave-based level-barrier parallel. Errors are selected topo-min (identical to sequential); `ctx` cancellation takes precedence; reported `stageOrder` is reconstructed to topo order. Concurrency threads to the config path via `config.BuildOption`s and to the convenience constructors via `rlng.Option`s. No `Scope` structural change (existing `s.mu` + Spec-002 namespace isolation make it race-free).

**Tech Stack:** Go 1.25+, `sync` (WaitGroup/Mutex, bounded via a semaphore channel), `context`. No new external dependency.

**Governing artifacts (read first):** `docs/specs/027-parallel-stage-execution.md`, `docs/adrs/0052-parallel-stage-execution.md` (draft, supersedes ADR-0006), `docs/adrs/0006-sequential-execution.md`, `docs/adrs/0005-pipeline-orchestration.md`, `CLAUDE.md`.

## Global Constraints

- Module `github.com/kartaladev/rlng`; Go 1.25+; pure Go, `CGO_ENABLED=0` must build.
- Tests: blackbox external `_test` package only; `table-test` assert-closure form (no `want`/`wantErr` fields); `t.Context()` not `context.Background()`; fold ≥2 same-call cases into a table.
- Every hot-path branch and every typed-error branch has a covering test; target ≥85% coverage on changed packages; `go test ./... -race` green.
- Never `panic`/`log.Fatal`/`os.Exit` on caller input — return typed, wrapping errors.
- Every exported symbol has a godoc comment.
- Conventional Commits with `Spec: 027` / `Plan: 027` / `ADR: 0052` trailers on `feat` commits. Plan + ADR ride in feat commits (no standalone plan/ADR commit). Per-task commits are pre-authorized once execution begins (green unit each).

---

### Task 1: Consolidate `NewPipeline` signature and move `WithRuleset` to an option

**Files:**
- Modify: `pipe/pipeline.go` (constructor signature, add `PipelineOption`/`pipelineConfig`)
- Modify: `pipe/ruleset.go` (replace fluent `(*Pipeline).WithRuleset` method with a `WithRuleset` option)
- Modify: `config/build.go:68`, `config/build.go:81-85`, `config/build.go:319` (call sites + reorder hash before construct)
- Modify: all `*_test.go` call sites of `pipe.NewPipeline(...)` (~69) and `.WithRuleset(...)`
- Test: existing `pipe/pipeline_test.go`, `pipe/ruleset_test.go`, `config/build_test.go` (migrated, must stay green)

**Interfaces:**
- Produces:
  ```go
  func NewPipeline(stages []Stage, opts ...PipelineOption) (*Pipeline, error)
  type PipelineOption func(*pipelineConfig)
  func WithRuleset(id RulesetIdentity) PipelineOption
  ```
  (The fluent `(*Pipeline).WithRuleset(id) *Pipeline` method is REMOVED.)

- [ ] **Step 1: Add the option type and config struct, change the constructor signature (`pipe/pipeline.go`).**

Add near the top (after the error types):

```go
// PipelineOption configures a Pipeline at construction. Options are applied in
// order; where two set the same knob, the last wins.
type PipelineOption func(*pipelineConfig)

type pipelineConfig struct {
	ruleset RulesetIdentity
}
```

Change the `Pipeline` struct comment/field set to keep `ordered` and `ruleset` (unchanged), and rewrite `NewPipeline`:

```go
// NewPipeline validates stages and computes their execution order. Stage names
// must be unique; every DependsOn target must name a stage in the set; and the
// dependency graph must be acyclic. It returns a *DuplicateStageError,
// *UnknownDependencyError, or *CycleError otherwise. An empty (or nil) set is
// valid; its Run is a no-op. Options (e.g. WithRuleset) configure the pipeline.
func NewPipeline(stages []Stage, opts ...PipelineOption) (*Pipeline, error) {
	cfg := &pipelineConfig{}
	for _, o := range opts {
		o(cfg)
	}

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
	return &Pipeline{ordered: ordered, ruleset: cfg.ruleset}, nil
}
```

- [ ] **Step 2: Replace the fluent `WithRuleset` method with an option (`pipe/ruleset.go`).**

Delete the `func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline { ... }` method and replace with:

```go
// WithRuleset records which ruleset the Pipeline evaluates, so Run stamps the
// identity onto each Scope. The zero identity leaves Ruleset reporting absent.
func WithRuleset(id RulesetIdentity) PipelineOption {
	return func(c *pipelineConfig) { c.ruleset = id }
}
```

Leave `stampRuleset`, `Ruleset`, and `RulesetIdentity` unchanged. Update `pipe/doc.go` line ~14 and `pipe/json.go` line ~47 prose references from "WithRuleset(...)" method phrasing to "the WithRuleset option" (doc-only).

- [ ] **Step 3: Migrate the config call sites (`config/build.go`).**

At `build.go:60-85`, compute the hash BEFORE constructing the pipeline and pass `pipe.WithRuleset` as an option:

```go
	stages := make([]pipe.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build(d.Constants, schema, strict)
		if err != nil {
			return nil, err
		}
		stages = append(stages, st)
	}
	version := cfg.version
	if version == "" {
		version = d.Version
	}
	// A hand-built def carrying a non-marshalable value cannot produce a stable
	// content hash, so reject it here rather than stamp a placeholder (ADR-0045).
	hash, err := d.hashCanonical()
	if err != nil {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %v", ErrUnhashableDef, err)}
	}
	p, err := pipe.NewPipeline(stages, pipe.WithRuleset(pipe.RulesetIdentity{Hash: hash, Version: version}))
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return p, nil
```

At `build.go:319`, change `pipe.NewPipeline(inner...)` → `pipe.NewPipeline(inner)`.

- [ ] **Step 4: Migrate all test call sites.** Apply mechanically across `*_test.go`:
  - `pipe.NewPipeline()` → `pipe.NewPipeline(nil)`
  - `pipe.NewPipeline(a, b, …)` (stage literals) → `pipe.NewPipeline([]pipe.Stage{a, b, …})`
  - `pipe.NewPipeline(xs...)` (already a slice, unpacked) → `pipe.NewPipeline(xs)`
  - `NewPipeline(...)` inside blackbox `pipe_test` uses the `pipe.` qualifier already; keep it.
  - `p.WithRuleset(id)` (fluent) → build with the option: `pipe.NewPipeline(stages, pipe.WithRuleset(id))`; if a test mutated an existing `p`, reconstruct it with the option instead.

- [ ] **Step 5: Run the full suite to verify green (behavior identical).**

Run: `go build ./... && go test ./... -race`
Expected: PASS across all 5 packages; no behavior change (this task is a pure API consolidation).

- [ ] **Step 6: Commit.**

```bash
git add pipe/pipeline.go pipe/ruleset.go pipe/doc.go pipe/json.go config/build.go docs/plans/027-parallel-stage-execution.md docs/adrs/0052-parallel-stage-execution.md
git add -A  # migrated *_test.go files
git commit -m "$(printf 'refactor(pipe)!: consolidate NewPipeline onto (stages, opts) and make WithRuleset an option (B11)\n\nFrees Go'\''s single variadic slot for pipeline-level options; removes the\nfluent (*Pipeline).WithRuleset method in favor of the WithRuleset\nPipelineOption. Behavior unchanged; prepares for WithConcurrency.\nPlan 027 + draft ADR-0052 (supersedes ADR-0006) added.\n\nSpec: 027\nPlan: 027\nADR: 0052')"
```

---

### Task 2: Wave-based parallel execution (`WithConcurrency` / `WithMaxParallel`)

**Files:**
- Modify: `pipe/pipeline.go` (options, `maxParallel`/`levels` on `Pipeline`, `computeLevels`, `Run` branch, `runSequential`, `runWaves`, `runLevel`, `topoMinError`, `InvalidMaxParallelError`)
- Modify: `pipe/timing.go` (add `reorderStages` Scope helper)
- Create: `pipe/concurrency_test.go` (execution tests)
- Create: `pipe/concurrency_probe_test.go` (deterministic test doubles: gated stages, cyclic barrier)

**Interfaces:**
- Consumes (Task 1): `NewPipeline(stages []Stage, opts ...PipelineOption)`, `pipelineConfig`.
- Produces:
  ```go
  func WithConcurrency() PipelineOption            // parallel, unbounded  (maxParallel = -1)
  func WithMaxParallel(n int) PipelineOption       // parallel, capped at n (maxParallel = n; n>=1)
  type InvalidMaxParallelError struct{ N int }     // *InvalidMaxParallelError from NewPipeline when n<1
  ```

- [ ] **Step 1: Write the failing tests (`pipe/concurrency_probe_test.go` — test doubles).**

```go
package pipe_test

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/kartaladev/rlng/pipe"
)

// cyclicBarrier releases waiters in batches of size n; it re-arms after each
// batch, so a level of >n gated stages passing a size-c semaphore releases in
// c-sized waves without deadlock.
type cyclicBarrier struct {
	mu    sync.Mutex
	size  int
	count int
	gen   chan struct{}
}

func newCyclicBarrier(size int) *cyclicBarrier {
	return &cyclicBarrier{size: size, gen: make(chan struct{})}
}

func (b *cyclicBarrier) wait() {
	b.mu.Lock()
	ch := b.gen
	b.count++
	if b.count == b.size {
		b.count = 0
		b.gen = make(chan struct{})
		b.mu.Unlock()
		close(ch)
		return
	}
	b.mu.Unlock()
	<-ch
}

// probe tracks the peak number of stages executing concurrently.
type probe struct {
	active  atomic.Int32
	peak    atomic.Int32
	barrier *cyclicBarrier
}

func (p *probe) recordPeak(n int32) {
	for {
		old := p.peak.Load()
		if n <= old || p.peak.CompareAndSwap(old, n) {
			return
		}
	}
}

// gatedStage: on Execute, marks itself active, records the peak, waits on the
// barrier so a whole batch overlaps, sets its output, then returns.
type gatedStage struct {
	name  string
	deps  []string
	pr    *probe
}

func (s *gatedStage) Name() string        { return s.name }
func (s *gatedStage) Type() string        { return "test-gated" }
func (s *gatedStage) DependsOn() []string { return s.deps }
func (s *gatedStage) Execute(ctx context.Context, sc *pipe.Scope) error {
	n := s.pr.active.Add(1)
	s.pr.recordPeak(n)
	s.pr.barrier.wait()
	s.pr.active.Add(-1)
	return sc.Set(s.name, true)
}

// erroringStage returns a fixed error; setStage records that it ran.
type erroringStage struct {
	name string
	deps []string
	err  error
}

func (s *erroringStage) Name() string        { return s.name }
func (s *erroringStage) Type() string        { return "test-err" }
func (s *erroringStage) DependsOn() []string { return s.deps }
func (s *erroringStage) Execute(ctx context.Context, sc *pipe.Scope) error { return s.err }

type setStage struct {
	name string
	deps []string
}

func (s *setStage) Name() string        { return s.name }
func (s *setStage) Type() string        { return "test-set" }
func (s *setStage) DependsOn() []string { return s.deps }
func (s *setStage) Execute(ctx context.Context, sc *pipe.Scope) error { return sc.Set(s.name, true) }
```

- [ ] **Step 2: Write the failing execution tests (`pipe/concurrency_test.go`).**

```go
package pipe_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineConcurrency(t *testing.T) {
	errA := errors.New("A failed")
	errB := errors.New("B failed")

	tests := []struct {
		name   string
		build  func() (*pipe.Pipeline, error)
		assert func(t *testing.T, sc *pipe.Scope, runErr error, buildErr error)
	}{
		{
			name: "unbounded runs a whole level concurrently",
			build: func() (*pipe.Pipeline, error) {
				pr := &probe{barrier: newCyclicBarrier(3)}
				return pipe.NewPipeline([]pipe.Stage{
					&gatedStage{name: "a", pr: pr},
					&gatedStage{name: "b", pr: pr},
					&gatedStage{name: "c", pr: pr},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				require.NoError(t, runErr)
				// If all 3 did not overlap, the size-3 barrier would deadlock,
				// so reaching here already proves concurrency; data is complete.
				for _, k := range []string{"a", "b", "c"} {
					v, ok := sc.Get(k)
					assert.True(t, ok)
					assert.Equal(t, true, v)
				}
			},
		},
		{
			name: "bounded caps concurrency at n",
			build: func() (*pipe.Pipeline, error) {
				pr := &probe{barrier: newCyclicBarrier(2)} // batch size == cap
				return pipe.NewPipeline([]pipe.Stage{
					&gatedStage{name: "a", pr: pr},
					&gatedStage{name: "b", pr: pr},
					&gatedStage{name: "c", pr: pr},
					&gatedStage{name: "d", pr: pr},
				}, pipe.WithMaxParallel(2))
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				require.NoError(t, runErr)
			},
		},
		{
			name: "n<1 is an InvalidMaxParallelError",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{&setStage{name: "a"}}, pipe.WithMaxParallel(0))
			},
			assert: func(t *testing.T, _ *pipe.Scope, _, buildErr error) {
				var e *pipe.InvalidMaxParallelError
				require.ErrorAs(t, buildErr, &e)
				assert.Equal(t, 0, e.N)
			},
		},
		{
			name: "topo-min error wins when independent stages both fail",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&erroringStage{name: "a", err: errA},
					&erroringStage{name: "b", err: errB},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, _ *pipe.Scope, runErr, _ error) {
				assert.ErrorIs(t, runErr, errA) // "a" is topo-first
			},
		},
		{
			name: "dependency-failed stage never runs",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&erroringStage{name: "a", err: errA},
					&setStage{name: "b", deps: []string{"a"}},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				assert.ErrorIs(t, runErr, errA)
				_, ok := sc.Get("b")
				assert.False(t, ok) // b depends on failed a → pruned
			},
		},
		{
			name: "reported stage order is topo order, not completion order",
			build: func() (*pipe.Pipeline, error) {
				return pipe.NewPipeline([]pipe.Stage{
					&setStage{name: "a"},
					&setStage{name: "b", deps: []string{"a"}},
					&setStage{name: "c", deps: []string{"a"}},
				}, pipe.WithConcurrency())
			},
			assert: func(t *testing.T, sc *pipe.Scope, runErr, _ error) {
				require.NoError(t, runErr)
				got := make([]string, 0, 3)
				for _, tm := range sc.StageTimings() {
					got = append(got, tm.Name)
				}
				assert.Equal(t, []string{"a", "b", "c"}, got)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, buildErr := tc.build()
			if buildErr != nil {
				tc.assert(t, nil, nil, buildErr)
				return
			}
			sc := pipe.NewScope(nil)
			runErr := p.Run(t.Context(), sc)
			tc.assert(t, sc, runErr, nil)
		})
	}
}

func TestPipelineConcurrencyContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancelled

	p, err := pipe.NewPipeline([]pipe.Stage{&setStage{name: "a"}}, pipe.WithConcurrency())
	require.NoError(t, err)

	sc := pipe.NewScope(nil)
	runErr := p.Run(ctx, sc)
	assert.ErrorIs(t, runErr, context.Canceled)
	_, ok := sc.Get("a")
	assert.False(t, ok) // no wave launched
}
```

Confirm `StageTiming` has a `Name` field (it does — `sc.StageTimings()` returns `[]StageTiming{Name, Duration}`; verify in `pipe/timing.go` and adjust the field name if different).

- [ ] **Step 3: Run the tests to verify they fail.**

Run: `go test ./pipe/ -run 'TestPipelineConcurrency' -race`
Expected: FAIL — `WithConcurrency`, `WithMaxParallel`, `InvalidMaxParallelError` undefined.

- [ ] **Step 4: Add the options, error type, and `maxParallel`/`levels` (`pipe/pipeline.go`).**

Extend `pipelineConfig` and `Pipeline`:

```go
type pipelineConfig struct {
	ruleset     RulesetIdentity
	maxParallel int // 0 = sequential; -1 = unbounded; n>0 = bounded at n
}
```

```go
type Pipeline struct {
	ordered     []Stage
	levels      [][]Stage // dependency levels (ordered grouped by longest-chain depth)
	ruleset     RulesetIdentity
	maxParallel int
}
```

Add the options and error:

```go
// WithConcurrency runs independent stages of each dependency level concurrently,
// unbounded. Execution stays fully deterministic: the final Scope, the surfaced
// error, and the reported stage order are identical to sequential execution
// (ADR-0052). The default (no concurrency option) is sequential.
func WithConcurrency() PipelineOption {
	return func(c *pipelineConfig) { c.maxParallel = -1 }
}

// WithMaxParallel is like WithConcurrency but caps the number of stages running
// at once to n. NewPipeline returns an *InvalidMaxParallelError if n < 1.
func WithMaxParallel(n int) PipelineOption {
	return func(c *pipelineConfig) { c.maxParallel = n }
}

// InvalidMaxParallelError reports a WithMaxParallel bound below 1.
type InvalidMaxParallelError struct{ N int }

// Error renders `pipeline: max parallel must be >= 1, got N`.
func (e *InvalidMaxParallelError) Error() string {
	return fmt.Sprintf("pipeline: max parallel must be >= 1, got %d", e.N)
}
```

In `NewPipeline`, after applying options and before `topoSort`, validate the bound; after `topoSort`, compute levels:

```go
	if cfg.maxParallel == 0 || cfg.maxParallel >= 1 {
		// valid: 0 (sequential) or a positive bound
	} else if cfg.maxParallel != -1 {
		return nil, &InvalidMaxParallelError{N: cfg.maxParallel}
	}
	// ... existing index/dep/topoSort ...
	return &Pipeline{
		ordered:     ordered,
		levels:      computeLevels(ordered),
		ruleset:     cfg.ruleset,
		maxParallel: cfg.maxParallel,
	}, nil
```

Add `computeLevels` (relies on `ordered` being topo-sorted so every dep precedes its dependents):

```go
// computeLevels groups ordered stages by dependency depth (level = 1 + max dep
// level). Because ordered is a topological sort, every dependency precedes its
// dependents, so levels concatenated equal ordered — the reported/topo order.
func computeLevels(ordered []Stage) [][]Stage {
	lvlOf := make(map[string]int, len(ordered))
	var levels [][]Stage
	for _, s := range ordered {
		lvl := 0
		for _, dep := range s.DependsOn() {
			if d := lvlOf[dep] + 1; d > lvl {
				lvl = d
			}
		}
		lvlOf[s.Name()] = lvl
		for len(levels) <= lvl {
			levels = append(levels, nil)
		}
		levels[lvl] = append(levels[lvl], s)
	}
	return levels
}
```

- [ ] **Step 5: Branch `Run` into sequential vs wave (`pipe/pipeline.go`).**

Replace `Run` and add the helpers:

```go
// Run executes the pipeline's stages in dependency order against sc, returning
// the first stage error (topo-earliest). It checks ctx before each stage/wave
// and returns ctx.Err() (unwrapped) if canceled; no further stages run.
// Sequential by default; WithConcurrency/WithMaxParallel run independent stages
// of each level concurrently with identical observable results (ADR-0052).
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error {
	sc.markStarted()
	defer sc.markFinished()
	sc.stampRuleset(p.ruleset)
	if p.maxParallel == 0 {
		return p.runSequential(ctx, sc)
	}
	return p.runWaves(ctx, sc)
}

func (p *Pipeline) runSequential(ctx context.Context, sc *Scope) error {
	for _, st := range p.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sc.timeStage(st.Name(), func() error { return st.Execute(ctx, sc) }); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) runWaves(ctx context.Context, sc *Scope) error {
	defer sc.reorderStages(p.orderedNames())
	for _, level := range p.levels {
		if err := ctx.Err(); err != nil {
			return err
		}
		errs := p.runLevel(ctx, sc, level)
		if len(errs) > 0 {
			if err := ctx.Err(); err != nil {
				return err // caller cancellation takes precedence
			}
			return topoMinError(p.ordered, errs)
		}
	}
	return nil
}

// runLevel executes a level's stages concurrently (bounded by a semaphore when
// maxParallel > 0) and returns the errors keyed by stage name. All stages in a
// level run to completion even if a sibling fails (no internal fail-fast), so
// the topo-earliest failure can be selected deterministically.
func (p *Pipeline) runLevel(ctx context.Context, sc *Scope, level []Stage) map[string]error {
	var (
		mu   sync.Mutex
		errs = make(map[string]error)
		wg   sync.WaitGroup
		sem  chan struct{}
	)
	if p.maxParallel > 0 {
		sem = make(chan struct{}, p.maxParallel)
	}
	for _, st := range level {
		if sem != nil {
			sem <- struct{}{}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			if err := sc.timeStage(st.Name(), func() error { return st.Execute(ctx, sc) }); err != nil {
				mu.Lock()
				errs[st.Name()] = err
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errs
}

// topoMinError returns the error of the topo-earliest failing stage — exactly
// the error sequential Run would surface.
func topoMinError(ordered []Stage, errs map[string]error) error {
	for _, s := range ordered {
		if e, ok := errs[s.Name()]; ok {
			return e
		}
	}
	return nil
}

func (p *Pipeline) orderedNames() []string {
	names := make([]string, len(p.ordered))
	for i, s := range p.ordered {
		names[i] = s.Name()
	}
	return names
}
```

Add `import "sync"` to `pipe/pipeline.go`.

- [ ] **Step 6: Add `reorderStages` to the Scope (`pipe/timing.go`).**

```go
// reorderStages rewrites the reported stage order to topo order (intersected
// with stages that actually executed), so concurrent completion order does not
// leak into StageTimings. Called by the wave runner after execution.
func (s *Scope) reorderStages(topo []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.stageTimes) == 0 {
		return
	}
	out := make([]string, 0, len(s.stageOrder))
	for _, name := range topo {
		if _, ran := s.stageTimes[name]; ran {
			out = append(out, name)
		}
	}
	s.stageOrder = out
}
```

- [ ] **Step 7: Run the tests to verify they pass.**

Run: `go test ./pipe/ -run 'TestPipelineConcurrency' -race -count=20`
Expected: PASS (the `-count=20` re-run guards determinism of error selection and stage order).

- [ ] **Step 8: Run the full suite.**

Run: `go build ./... && go test ./... -race`
Expected: PASS (sequential default path unchanged).

- [ ] **Step 9: Commit.**

```bash
git add pipe/pipeline.go pipe/timing.go pipe/concurrency_test.go pipe/concurrency_probe_test.go
git commit -m "$(printf 'feat(pipe): wave-based parallel execution of independent stages (B11)\n\nWithConcurrency/WithMaxParallel run each dependency level concurrently\n(unbounded or n-bounded). Fully deterministic: topo-min error selection,\nctx-cancel precedence, topo-order stage reporting. Sequential default\nunchanged; Scope race-free by its existing mutex + Spec-002 isolation.\n\nSpec: 027\nPlan: 027\nADR: 0052')"
```

---

### Task 3: Thread concurrency through the config `Build`

**Files:**
- Modify: `config/build_options.go` (add `WithConcurrency`/`WithMaxParallel` BuildOptions + config fields)
- Modify: `config/build.go` (pass matching `pipe.PipelineOption`s into `NewPipeline`)
- Create: `config/build_concurrency_test.go`

**Interfaces:**
- Consumes (Task 2): `pipe.WithConcurrency()`, `pipe.WithMaxParallel(n)`.
- Produces:
  ```go
  func WithConcurrency() BuildOption
  func WithMaxParallel(n int) BuildOption
  ```

- [ ] **Step 1: Write the failing test (`config/build_concurrency_test.go`).**

```go
package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildConcurrency(t *testing.T) {
	yaml := `
stages:
  - name: a
    type: single-expr
    expr: "1 + 1"
  - name: b
    type: single-expr
    expr: "2 + 2"
`
	tests := []struct {
		name   string
		opts   []config.BuildOption
		assert func(t *testing.T, out map[string]any, err error)
	}{
		{
			name: "concurrent build still evaluates correctly",
			opts: []config.BuildOption{config.WithConcurrency()},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
				assert.EqualValues(t, 4, out["b"])
			},
		},
		{
			name: "bounded build still evaluates correctly",
			opts: []config.BuildOption{config.WithMaxParallel(1)},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "invalid bound surfaces from Build",
			opts: []config.BuildOption{config.WithMaxParallel(0)},
			assert: func(t *testing.T, _ map[string]any, err error) {
				require.Error(t, err) // wraps *pipe.InvalidMaxParallelError
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, err := config.Parse(t.Context(), config.FromYAMLString(yaml))
			require.NoError(t, err)
			p, err := def.Build(tc.opts...)
			if err != nil {
				tc.assert(t, nil, err)
				return
			}
			sc := runPipeline(t, p) // existing helper; else inline: pipe.NewScope + p.Run + snapshot
			tc.assert(t, sc, nil)
		})
	}
}
```

If no `runPipeline` helper exists in `config` tests, inline: build a `pipe.NewScope(nil)`, call `p.Run(t.Context(), sc)`, then `sc.Snapshot()`; import `pipe`.

- [ ] **Step 2: Run to verify it fails.**

Run: `go test ./config/ -run TestBuildConcurrency`
Expected: FAIL — `config.WithConcurrency`/`WithMaxParallel` undefined.

- [ ] **Step 3: Add the BuildOptions (`config/build_options.go`).**

Add to the build config struct (find the struct configured by `BuildOption`, currently holding `strict`, `schema`, `version`, `lintErrors`):

```go
	maxParallel int // 0 = sequential; -1 = unbounded; n>0 = bounded
```

Add:

```go
// WithConcurrency builds a Pipeline that runs independent stages concurrently
// (unbounded). See pipe.WithConcurrency.
func WithConcurrency() BuildOption {
	return func(c *buildConfig) { c.maxParallel = -1 }
}

// WithMaxParallel builds a Pipeline that runs independent stages concurrently,
// capped at n. Build fails if n < 1. See pipe.WithMaxParallel.
func WithMaxParallel(n int) BuildOption {
	return func(c *buildConfig) { c.maxParallel = n }
}
```

(Use the actual config-struct type name from the file — likely `buildConfig`.)

- [ ] **Step 4: Pass the option into `NewPipeline` (`config/build.go`).**

Build the pipeline options list before constructing:

```go
	pipeOpts := []pipe.PipelineOption{pipe.WithRuleset(pipe.RulesetIdentity{Hash: hash, Version: version})}
	switch {
	case cfg.maxParallel == -1:
		pipeOpts = append(pipeOpts, pipe.WithConcurrency())
	case cfg.maxParallel > 0:
		pipeOpts = append(pipeOpts, pipe.WithMaxParallel(cfg.maxParallel))
	case cfg.maxParallel != 0:
		// negative-but-not-(-1) is invalid; let NewPipeline surface it
		pipeOpts = append(pipeOpts, pipe.WithMaxParallel(cfg.maxParallel))
	}
	p, err := pipe.NewPipeline(stages, pipeOpts...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return p, nil
```

- [ ] **Step 5: Run the tests to verify they pass, then the full suite.**

Run: `go test ./config/ -run TestBuildConcurrency -race && go test ./... -race`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add config/build_options.go config/build.go config/build_concurrency_test.go
git commit -m "$(printf 'feat(config): thread concurrency into Build (WithConcurrency/WithMaxParallel) (B11)\n\nBuildOptions translate to the matching pipe.PipelineOption so a config\nsource can opt into parallel stage execution.\n\nSpec: 027\nPlan: 027\nADR: 0052')"
```

---

### Task 4: Root-package engine options + convenience-constructor threading

**Files:**
- Modify: `engine.go` (add `maxParallel` to `engineConfig`; `WithConcurrency`/`WithMaxParallel` Options; fail-loud in `New` when set on a pre-built pipeline)
- Modify: `typed_engine.go` (same fail-loud guard in `NewTypedEngine`)
- Modify: `errors.go` (new sentinel `ErrConcurrencyRequiresConfig`)
- Modify: `fromconfig.go` (translate engine concurrency Option → `config.BuildOption`)
- Create: `fromconfig_concurrency_test.go`
- Modify: `engine_test.go` (guard test) — or add to the new file

**Interfaces:**
- Consumes (Task 3): `config.WithConcurrency()`, `config.WithMaxParallel(n)`.
- Produces:
  ```go
  func WithConcurrency() Option
  func WithMaxParallel(n int) Option
  var ErrConcurrencyRequiresConfig error
  ```

- [ ] **Step 1: Write the failing tests (`fromconfig_concurrency_test.go`).**

```go
package rlng_test

import (
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromYAMLConcurrency(t *testing.T) {
	yaml := `
stages:
  - name: a
    type: single-expr
    expr: "1 + 1"
`
	tests := []struct {
		name   string
		opts   []rlng.Option
		assert func(t *testing.T, out map[string]any, err error)
	}{
		{
			name: "NewFromYAML with WithConcurrency builds and evaluates",
			opts: []rlng.Option{rlng.WithConcurrency()},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "NewFromYAML with WithMaxParallel(1) builds and evaluates",
			opts: []rlng.Option{rlng.WithMaxParallel(1)},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "invalid bound surfaces from NewFromYAML",
			opts: []rlng.Option{rlng.WithMaxParallel(0)},
			assert: func(t *testing.T, _ map[string]any, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := rlng.NewFromYAML(t.Context(), yaml, tc.opts...)
			if err != nil {
				tc.assert(t, nil, err)
				return
			}
			out, err := eng.Evaluate(t.Context(), map[string]any{})
			tc.assert(t, out, err)
		})
	}
}

func TestNewRejectsConcurrencyOnPrebuiltPipeline(t *testing.T) {
	p, err := pipe.NewPipeline([]pipe.Stage{})
	require.NoError(t, err)

	_, err = rlng.New(p, rlng.WithConcurrency())
	assert.ErrorIs(t, err, rlng.ErrConcurrencyRequiresConfig)
}
```

- [ ] **Step 2: Run to verify failure.**

Run: `go test . -run 'Concurrency|Prebuilt'`
Expected: FAIL — undefined `rlng.WithConcurrency`, `rlng.WithMaxParallel`, `rlng.ErrConcurrencyRequiresConfig`.

- [ ] **Step 3: Add the sentinel (`errors.go`).**

```go
// ErrConcurrencyRequiresConfig is returned by New/NewTypedEngine when a
// concurrency Option (WithConcurrency/WithMaxParallel) is passed to a
// constructor that wraps an already-built Pipeline. Concurrency is a property
// of pipeline construction: set it on the pipeline via pipe.WithConcurrency, or
// use NewFromYAML/NewFromProvider which build the pipeline for you.
var ErrConcurrencyRequiresConfig = errors.New("rlng: concurrency options apply only to the config constructors (NewFromYAML/NewFromProvider); set concurrency on the pipeline via pipe.WithConcurrency")
```

Confirm `errors.go` imports `errors`.

- [ ] **Step 4: Add the Options and the guard (`engine.go`).**

Extend `engineConfig` and add options:

```go
type engineConfig struct {
	scopeOpts   []pipe.ScopeOption
	maxParallel int // 0 = unset; -1 = unbounded; n>0 = bounded (config path only)
}

// WithConcurrency, when passed to NewFromYAML/NewFromProvider, builds the
// pipeline to run independent stages concurrently (unbounded). Passing it to
// New (which wraps an already-built pipeline) returns ErrConcurrencyRequiresConfig.
func WithConcurrency() Option {
	return func(c *engineConfig) { c.maxParallel = -1 }
}

// WithMaxParallel is like WithConcurrency but caps concurrency at n.
func WithMaxParallel(n int) Option {
	return func(c *engineConfig) { c.maxParallel = n }
}
```

In `New`, after applying options:

```go
	if cfg.maxParallel != 0 {
		return nil, ErrConcurrencyRequiresConfig
	}
```

- [ ] **Step 5: Add the same guard to `NewTypedEngine` (`typed_engine.go`).**

After the option loop:

```go
	if cfg.maxParallel != 0 {
		return nil, ErrConcurrencyRequiresConfig
	}
```

- [ ] **Step 6: Thread through the convenience constructors (`fromconfig.go`).**

`NewFromProvider` builds the def, then applies concurrency as a `config.BuildOption`, and must NOT pass the concurrency Option down to `New` (which would trip the guard). Read the current `fromconfig.go` and refactor `NewFromProvider`:

```go
func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error) {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	pipeline, err := def.Build(buildOptsFor(cfg)...)
	if err != nil {
		return nil, err
	}
	return &Engine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}, nil
}

// buildOptsFor maps engine-level concurrency config to config.BuildOptions.
func buildOptsFor(cfg *engineConfig) []config.BuildOption {
	switch {
	case cfg.maxParallel == -1:
		return []config.BuildOption{config.WithConcurrency()}
	case cfg.maxParallel != 0:
		return []config.BuildOption{config.WithMaxParallel(cfg.maxParallel)}
	default:
		return nil
	}
}
```

Do the equivalent for `NewTypedFromProvider` (build with `buildOptsFor(cfg)`, then construct the `TypedEngine` directly from `pipeline`, `mapper`, `cfg.scopeOpts` — mirroring `NewTypedEngine`'s body but skipping the guard, since concurrency was consumed into the build). Keep `NewFromYAML`/`NewTypedFromYAML` delegating to their `*Provider` siblings with `config.FromYAMLString(yaml)` unchanged.

Note: constructing `&Engine{...}` / `&TypedEngine{...}` directly (rather than via `New`) is required so the concurrency Option does not re-trip the guard. Add a short comment saying so.

- [ ] **Step 7: Run the tests, then the full suite.**

Run: `go test . -run 'Concurrency|Prebuilt' -race && go test ./... -race`
Expected: PASS.

- [ ] **Step 8: Commit.**

```bash
git add engine.go typed_engine.go errors.go fromconfig.go fromconfig_concurrency_test.go
git commit -m "$(printf 'feat(rlng): concurrency options on the config constructors (B11)\n\nWithConcurrency/WithMaxParallel thread through NewFromYAML/NewFromProvider\nto the built pipeline; New/NewTypedEngine reject them with\nErrConcurrencyRequiresConfig (a pre-built pipeline carries its own\nconcurrency).\n\nSpec: 027\nPlan: 027\nADR: 0052')"
```

---

### Task 5: Runnable example, docs, and artifact status close-out

**Files:**
- Create: `pipe/concurrency_example_test.go` (runnable `Example` with deterministic `// Output:`)
- Modify: `pipe/doc.go` (document the concurrency feature + option-based config)
- Modify: `docs/adrs/0006-sequential-execution.md` (Status → `Superseded by ADR-0052`)
- Modify: `docs/adrs/0052-parallel-stage-execution.md` (Status → `Accepted`)
- Modify: `docs/specs/027-parallel-stage-execution.md` (Status → `Accepted`)
- Modify: `docs/BACKLOG.md` (B11 → Done row + Resolved section)
- Modify: `README.md` if it enumerates pipeline options (grep first)

- [ ] **Step 1: Write the runnable example (`pipe/concurrency_example_test.go`).**

```go
package pipe_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// ExamplePipeline_withConcurrency runs two independent stages concurrently; the
// result is deterministic despite the parallelism.
func ExamplePipeline_withConcurrency() {
	a, _ := pipe.NewSingleExpr("a", "1 + 1")
	b, _ := pipe.NewSingleExpr("b", "10 * 2")

	p, _ := pipe.NewPipeline([]pipe.Stage{a, b}, pipe.WithConcurrency())

	sc := pipe.NewScope(nil)
	_ = p.Run(context.Background(), sc)

	fmt.Println(sc.Snapshot()["a"], sc.Snapshot()["b"])
	// Output: 2 20
}
```

Verify the `NewSingleExpr` constructor name/signature against `pipe/single.go` and adjust (it may take options). If a single-expr stage's output key differs from its name, set an explicit output or read the correct key.

- [ ] **Step 2: Run the example.**

Run: `go test ./pipe/ -run ExamplePipeline_withConcurrency`
Expected: PASS (output matches).

- [ ] **Step 3: Update `pipe/doc.go`** — add a sentence documenting that a Pipeline is sequential by default and `WithConcurrency`/`WithMaxParallel` opt into deterministic parallel execution of independent stages.

- [ ] **Step 4: Flip the ADR/spec statuses.**
  - `docs/adrs/0006-sequential-execution.md`: change `- **Status:** Accepted` → `- **Status:** Superseded by ADR-0052`.
  - `docs/adrs/0052-parallel-stage-execution.md`: change `- **Status:** Draft (proposed) — supersedes ADR-0006` → `- **Status:** Accepted — supersedes ADR-0006`.
  - `docs/specs/027-parallel-stage-execution.md`: change `- **Status:** Draft` → `- **Status:** Accepted`.

- [ ] **Step 5: Close out `docs/BACKLOG.md`.** Change the B11 table row to `✅ **Done** (incr 027, ADR-0052)`, strike the title, and add a Resolved-section row `Parallel stage execution (B11; ADR-0006) | Increment 027 / ADR-0052`, mirroring the B10 entry.

- [ ] **Step 6: Run the full suite + format/vet.**

Run: `go build ./... && go test ./... -race && go vet ./... && gofmt -l .`
Expected: build+tests PASS; `gofmt -l .` prints nothing.

- [ ] **Step 7: Commit.**

```bash
git add pipe/concurrency_example_test.go pipe/doc.go docs/adrs/0006-sequential-execution.md docs/adrs/0052-parallel-stage-execution.md docs/specs/027-parallel-stage-execution.md docs/BACKLOG.md README.md
git commit -m "$(printf 'docs(rlng): runnable concurrency example; close B11; supersede ADR-0006 (B11)\n\nSpec: 027\nPlan: 027\nADR: 0052')"
```

---

### Whole-branch delivery gate (after Task 5)

- [ ] Run `/code-review high main..HEAD`; resolve or triage every finding; re-run affected review.
- [ ] Run `/security-review` on the branch diff; resolve anything flagged.
- [ ] `go test ./... -race -count=5` green (determinism guard); `CGO_ENABLED=0 go build ./...`; `go mod tidy` no-op; `go test ./... -cover` (changed packages ≥85%, every new hot-path branch covered).
- [ ] Update `docs/HANDOVER.md` to reflect B11 done / B12 next.
- [ ] Merge to `main` (fast-forward), push, delete branch — per the standing AUTO-merge authorization (no per-increment approval; release tags still need explicit approval).

## Self-Review

**Spec coverage:** D1 (determinism) → Task 2 (topo-min, reorderStages) + example. D2 (API consolidation) → Tasks 1, 2. D3 (bound) → Task 2. D4 (wave scheduling) → Task 2 (`computeLevels`/`runWaves`). D5 (error path) → Task 2 (`topoMinError`, dep-failed pruning via stop-after-failing-wave). D6 (cancellation + reported order) → Task 2 (`runWaves` ctx checks, `reorderStages`). Concurrency-safety (no Scope change) → Task 2 relies on existing locks; `-race` gate. Success criteria 1–12 → criteria 1,10 (Task 1 migration green); 2–9 (Task 2 tests); 11 (Task 4); 3–4 config (Task 3); 12 example (Task 5). Threading (D2 engine options) → Tasks 3, 4.

**Placeholder scan:** none — every code step shows concrete code. Two verification notes flagged inline (StageTiming.Name field name; NewSingleExpr signature) are check-and-adjust instructions, not placeholders.

**Type consistency:** `maxParallel` sentinel (0/-1/n) consistent across `pipe.pipelineConfig`, `config.buildConfig`, `rlng.engineConfig`. `PipelineOption`/`BuildOption`/`Option` used in their own packages. `InvalidMaxParallelError.N`, `ErrConcurrencyRequiresConfig`, `topoMinError(ordered, errs)`, `computeLevels(ordered)`, `reorderStages(topo)`, `orderedNames()` names consistent between definition and use.
