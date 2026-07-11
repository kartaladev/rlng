package stage

import (
	"fmt"
	"sort"
	"strings"
)

// seedStageType is the StageType of a Derivation for a seed (input) value.
const seedStageType = "seed"

const opSeed = "seed"

// Derivation records how one value in a Scope was produced. It is populated only
// when the Scope was created WithProvenance.
type Derivation struct {
	Path       string         // scope dot-path written
	Stage      string         // producing stage name ("" for a seed input)
	StageType  string         // TypeSingleExpr / TypeMultiExpr / TypeDecisionTable, or "seed"
	Operation  string         // "seed", "eval", "expr:<name>", "decision:<key>", "collect:<key>"
	Expression string         // source expression ("" for a seed)
	Inputs     map[string]any // referenced identifier -> value at eval time (nil for a seed)
	Value      any            // the derived value
}

// WithProvenance makes the Scope record a Derivation for every value (seed inputs
// and each stage write), enabling Derivation, Lineage, and Explain. It is off by
// default; when off, no derivation is stored and the write path is exactly Set.
func WithProvenance() ScopeOption { return func(s *Scope) { s.provenance = true } }

// TracksProvenance reports whether provenance recording is enabled. It is
// lock-free: the flag is set at construction and never mutated, so stages branch
// on it without taking the mutex.
func (s *Scope) TracksProvenance() bool { return s.provenance }

// Derive stores v at path and, when provenance is enabled, records d as the
// derivation of that value (filling d.Path and d.Value). When provenance is
// disabled it is exactly Set(path, v).
func (s *Scope) Derive(path string, v any, d Derivation) error {
	if err := s.Set(path, v); err != nil {
		return err
	}
	if !s.provenance {
		return nil
	}
	d.Path = path
	d.Value = v
	s.mu.Lock()
	s.derivations[path] = d
	s.mu.Unlock()
	return nil
}

// Derivation returns the recorded derivation of the value at path, or false if
// provenance is disabled or no value was recorded there.
func (s *Scope) Derivation(path string) (Derivation, bool) {
	if !s.provenance {
		return Derivation{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.derivations[path]
	return d, ok
}

// Derivations returns a copy of all recorded derivations (empty when provenance
// is disabled).
func (s *Scope) Derivations() map[string]Derivation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Derivation, len(s.derivations))
	for k, v := range s.derivations {
		out[k] = v
	}
	return out
}

// Lineage returns the derivation of the value at path plus, transitively, the
// derivations of its inputs — ordered seeds-first (a value appears after every
// input it depends on). It is empty when provenance is disabled or path has no
// derivation.
func (s *Scope) Lineage(path string) []Derivation {
	if !s.provenance {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Derivation
	s.collectLineage(path, map[string]struct{}{}, &out)
	return out
}

func (s *Scope) collectLineage(key string, visited map[string]struct{}, out *[]Derivation) {
	for _, d := range s.derivationsFor(key) {
		if _, seen := visited[d.Path]; seen {
			continue
		}
		visited[d.Path] = struct{}{}
		for _, id := range sortedInputs(d.Inputs) {
			s.collectLineage(id, visited, out)
		}
		*out = append(*out, d)
	}
}

// Explain renders the derivation of path as an indented ASCII tree, tracing each
// input back to the seed values. It returns "" when provenance is disabled or
// path has no derivation.
func (s *Scope) Explain(path string) string {
	if !s.provenance {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	s.explain(path, 0, map[string]struct{}{}, &b)
	return b.String()
}

func (s *Scope) explain(key string, depth int, visited map[string]struct{}, b *strings.Builder) {
	for _, d := range s.derivationsFor(key) {
		indent := strings.Repeat("  ", depth)
		if d.StageType == seedStageType {
			fmt.Fprintf(b, "%s%s = %v [seed]\n", indent, d.Path, d.Value)
		} else {
			fmt.Fprintf(b, "%s%s = %v [%s %s] expr: %s\n", indent, d.Path, d.Value, d.Stage, d.StageType, d.Expression)
		}
		if _, seen := visited[d.Path]; seen {
			continue
		}
		visited[d.Path] = struct{}{}
		for _, id := range sortedInputs(d.Inputs) {
			s.explain(id, depth+1, visited, b)
		}
	}
}

// derivationsFor returns the derivation recorded exactly at key, or — when there
// is none — every derivation whose path is under the key namespace (key + "."),
// sorted by path. This links an input identifier (a top-level name like "tiers")
// to the namespaced values a stage wrote under it ("tiers.tag").
func (s *Scope) derivationsFor(key string) []Derivation {
	if d, ok := s.derivations[key]; ok {
		return []Derivation{d}
	}
	var out []Derivation
	prefix := key + "."
	for p, d := range s.derivations {
		if strings.HasPrefix(p, prefix) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func sortedInputs(inputs map[string]any) []string {
	if len(inputs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(inputs))
	for id := range inputs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// snapshotRefs returns the subset of env named by refs (the identifiers an
// expression reads), as the Inputs of a Derivation. It returns nil when refs is
// empty so a no-input derivation carries a nil (not empty) Inputs map.
func snapshotRefs(env map[string]any, refs []string) map[string]any {
	if len(refs) == 0 {
		return nil
	}
	out := make(map[string]any, len(refs))
	for _, r := range refs {
		if v, ok := env[r]; ok {
			out[r] = v
		}
	}
	return out
}
