package pipe

import (
	"fmt"
	"sort"
	"strings"
)

// seedStageType is the StageType of a Derivation for a seed (input) value.
const seedStageType = "seed"

const opSeed = "seed"

// MaxLineageDepth bounds Lineage/Explain recursion against a maliciously deep
// restored derivation graph. A derivation chain deeper than this is truncated;
// Explain marks the truncation point in its output.
const MaxLineageDepth = 1000

// Derivation records how one value in a Scope was produced. It is populated only
// when the Scope was created WithProvenance.
type Derivation struct {
	Path       string `json:"path"`                 // scope dot-path written
	Stage      string `json:"stage,omitempty"`      // producing stage name ("" for a seed input)
	StageType  string `json:"stage_type"`           // TypeSingleExpr / TypeMultiExpr / TypeDecisionTable, or "seed"
	Operation  string `json:"operation"`            // "seed", "eval", "expr:<name>", "decision:<key>", "collect:<key>"
	Expression string `json:"expression,omitempty"` // source expression ("" for a seed)
	// Inputs maps each referenced identifier to its value at eval time (nil for
	// a seed). For a collect-mode decision that matched multiple rules, Inputs
	// holds each identifier's value from the last matching rule (the Value slice
	// is the authoritative per-rule record); single/multi stages record one value.
	Inputs map[string]any `json:"inputs,omitempty"`
	Value  any            `json:"value"` // the derived value
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
// derivation. Traversal is bounded to a fixed maximum depth; a derivation chain
// deeper than that is truncated (a guard for graphs restored from untrusted JSON).
func (s *Scope) Lineage(path string) []Derivation {
	if !s.provenance {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Derivation
	s.collectLineage(path, 0, map[string]struct{}{}, s.lineageIndex(), &out)
	return out
}

func (s *Scope) collectLineage(key string, depth int, visited map[string]struct{}, idx map[string][]Derivation, out *[]Derivation) {
	if depth >= MaxLineageDepth {
		return
	}
	for _, d := range derivationsFor(s.derivations, idx, key) {
		if _, seen := visited[d.Path]; seen {
			continue
		}
		visited[d.Path] = struct{}{}
		for _, id := range sortedInputs(d.Inputs) {
			s.collectLineage(id, depth+1, visited, idx, out)
		}
		*out = append(*out, d)
	}
}

// Explain renders the derivation of path as an indented ASCII tree, tracing each
// input back to the seed values. It returns "" when provenance is disabled or
// path has no derivation. Traversal is bounded to a fixed maximum depth; a
// derivation chain deeper than that is truncated (a guard for graphs restored
// from untrusted JSON).
func (s *Scope) Explain(path string) string {
	if !s.provenance {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	s.explain(path, 0, map[string]struct{}{}, s.lineageIndex(), &b)
	return b.String()
}

func (s *Scope) explain(key string, depth int, visited map[string]struct{}, idx map[string][]Derivation, b *strings.Builder) {
	if depth >= MaxLineageDepth {
		fmt.Fprintf(b, "%s… (truncated: max lineage depth %d reached)\n", strings.Repeat("  ", depth), MaxLineageDepth)
		return
	}
	for _, d := range derivationsFor(s.derivations, idx, key) {
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
			s.explain(id, depth+1, visited, idx, b)
		}
	}
}

// lineageIndex groups derivations by every ancestor namespace of their path
// ("a.b.c" is indexed under "a" and "a.b"), each list sorted by path. Built once
// per Lineage/Explain call so derivationsFor is an O(1) lookup instead of an O(N)
// prefix scan at every recursion step (a guard against restored graphs with many
// derivations). Caller must hold s.mu.
func (s *Scope) lineageIndex() map[string][]Derivation {
	idx := make(map[string][]Derivation, len(s.derivations))
	for p, d := range s.derivations {
		for i := 0; i < len(p); i++ {
			if p[i] == '.' {
				idx[p[:i]] = append(idx[p[:i]], d)
			}
		}
	}
	for k := range idx {
		list := idx[k]
		sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	}
	return idx
}

// derivationsFor returns the derivation recorded exactly at key, or — when there
// is none — every derivation whose path is under the key namespace (key + "."),
// taken from the precomputed index. This links an input identifier (a top-level
// name like "tiers") to the namespaced values a stage wrote under it ("tiers.tag").
func derivationsFor(derivations map[string]Derivation, idx map[string][]Derivation, key string) []Derivation {
	if d, ok := derivations[key]; ok {
		return []Derivation{d}
	}
	return idx[key]
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
