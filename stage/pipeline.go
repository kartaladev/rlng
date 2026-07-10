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
