package pipe

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// DuplicateStageError reports two stages sharing a Name within a Pipeline.
type DuplicateStageError struct{ Name string }

// Error renders `pipeline: duplicate stage "name"`.
func (e *DuplicateStageError) Error() string {
	return fmt.Sprintf("pipeline: duplicate stage %q", e.Name)
}

// UnknownDependencyError reports a DependsOn target that names no stage in the
// pipeline's set.
type UnknownDependencyError struct {
	Stage      string
	Dependency string
}

// Error renders `pipeline: stage "x" depends on unknown stage "y"`.
func (e *UnknownDependencyError) Error() string {
	return fmt.Sprintf("pipeline: stage %q depends on unknown stage %q", e.Stage, e.Dependency)
}

// CycleError reports a dependency cycle among a Pipeline's stages. Cycle is the
// loop path, closing on the repeated stage (e.g. ["a", "b", "a"]).
type CycleError struct{ Cycle []string }

// Error renders `pipeline: dependency cycle: a -> b -> a`.
func (e *CycleError) Error() string {
	return "pipeline: dependency cycle: " + strings.Join(e.Cycle, " -> ")
}

// Pipeline runs a set of Stages in dependency order. NewPipeline validates the
// set and computes the execution order once; Run only evaluates. Execution is
// sequential and deterministic by default (ADR-0006); WithConcurrency /
// WithMaxParallel opt into deterministic parallel execution (ADR-0052).
type Pipeline struct {
	ordered     []Stage
	levels      [][]Stage // ordered grouped by dependency depth; used by the wave runner
	ruleset     RulesetIdentity
	maxParallel int // 0 = sequential; -1 = unbounded; n>0 = bounded at n
}

// PipelineOption configures a Pipeline at construction. Options are applied in
// order; where two set the same knob, the last wins.
type PipelineOption func(*pipelineConfig)

// concurrencyMode selects the execution strategy. The default zero value is
// sequential, so an unconfigured pipeline runs exactly as before (ADR-0006).
type concurrencyMode int8

const (
	concSequential concurrencyMode = iota
	concUnbounded
	concBounded
)

type pipelineConfig struct {
	ruleset RulesetIdentity
	mode    concurrencyMode
	boundN  int // requested cap when mode == concBounded; validated in NewPipeline
}

// WithConcurrency runs independent stages of each dependency level concurrently,
// unbounded. Execution stays fully deterministic: the final Scope, the surfaced
// error, and the reported stage order are identical to sequential execution
// (ADR-0052). The default (no concurrency option) is sequential.
func WithConcurrency() PipelineOption {
	return func(c *pipelineConfig) { c.mode = concUnbounded }
}

// WithMaxParallel is like WithConcurrency but caps the number of stages running
// at once to n. NewPipeline returns an *InvalidMaxParallelError if n < 1.
func WithMaxParallel(n int) PipelineOption {
	return func(c *pipelineConfig) { c.mode = concBounded; c.boundN = n }
}

// InvalidMaxParallelError reports a WithMaxParallel bound below 1.
type InvalidMaxParallelError struct{ N int }

// Error renders `pipeline: max parallel must be >= 1, got N`.
func (e *InvalidMaxParallelError) Error() string {
	return fmt.Sprintf("pipeline: max parallel must be >= 1, got %d", e.N)
}

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
	if cfg.mode == concBounded && cfg.boundN < 1 {
		return nil, &InvalidMaxParallelError{N: cfg.boundN}
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

	maxParallel := 0
	switch cfg.mode {
	case concUnbounded:
		maxParallel = -1
	case concBounded:
		maxParallel = cfg.boundN
	}

	p := &Pipeline{ordered: ordered, ruleset: cfg.ruleset, maxParallel: maxParallel}
	if maxParallel != 0 {
		p.levels = computeLevels(ordered)
	}
	return p, nil
}

// computeLevels groups ordered stages by dependency depth (level = 1 + max dep
// level). Because ordered is a topological sort, every dependency precedes its
// dependents, so the levels concatenated equal ordered — i.e. the reported and
// topo order. Each level's stages are mutually independent.
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
	sc.markStarted()
	defer sc.markFinished()
	sc.stampRuleset(p.ruleset)
	if p.maxParallel == 0 {
		return p.runSequential(ctx, sc)
	}
	return p.runWaves(ctx, sc)
}

// runSequential walks stages one at a time in topological order — the default,
// unchanged execution path (ADR-0006).
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

// runWaves executes each dependency level concurrently, with a barrier between
// levels. A stage runs only when all its dependencies (in earlier levels)
// succeeded, so a dep-failed subtree is pruned exactly as in sequential
// execution. If a level has any failure, later levels do not launch and the
// topo-earliest failure is returned — the same error a sequential Run would
// surface (ADR-0052).
//
// Cancellation matches the sequential semantics: ctx is checked before each
// level (as sequential checks before each stage), so a caller cancellation
// between levels short-circuits with ctx.Err(); a cancellation observed while a
// level runs surfaces through a stage's own ctx.Canceled return, which
// topoMinError then selects — no separate ctx re-check is needed (and re-adding
// one would wrongly mask a real stage error, since sequential does not re-check
// ctx after running a stage). The reported stage order is reconstructed to topo
// order, never completion order.
func (p *Pipeline) runWaves(ctx context.Context, sc *Scope) error {
	defer sc.reorderStages(p.orderedNames())
	for _, level := range p.levels {
		if err := ctx.Err(); err != nil {
			return err
		}
		if errs := p.runLevel(ctx, sc, level); len(errs) > 0 {
			return topoMinError(p.ordered, errs)
		}
	}
	return nil
}

// runLevel executes a level's (mutually independent) stages concurrently,
// bounded by a semaphore when maxParallel > 0, and returns the errors keyed by
// stage name. Every stage in the level runs to completion even if a sibling
// fails (no internal fail-fast), so the topo-earliest failure can be selected
// deterministically.
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
// the error a sequential Run would surface.
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
