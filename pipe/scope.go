package pipe

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrPathConflict is returned by Set, in strict mode, when a leaf path already
// holds a value.
var ErrPathConflict = errors.New("scope: path already set")

// ErrPathNotMap is returned when an intermediate dot-path segment exists but is
// not a map[string]any, so the path cannot be traversed.
var ErrPathNotMap = errors.New("scope: intermediate path is not a map")

// ErrEmptyPath is returned when Set is given an empty path.
var ErrEmptyPath = errors.New("scope: path must not be empty")

// ErrEmptyPathSegment is returned by Set when a dot-path contains an empty
// segment (e.g. "a..b", ".x", "a."), which would create an unreachable "" key.
var ErrEmptyPathSegment = errors.New("scope: path segment must not be empty")

// Scope is a concurrency-safe map[string]any accumulator threaded through stage
// evaluation. Values are addressed by dot-separated paths; the accumulated map
// is the environment against which expressions are evaluated.
type Scope struct {
	mu          sync.RWMutex
	data        map[string]any
	strict      bool
	provenance  bool
	derivations map[string]Derivation   // non-nil only when provenance is enabled
	firing      map[string][]FiringRule // stage name -> the ordered rules that fired
	ruleset     RulesetIdentity         // which ruleset produced this decision (stamped by Pipeline.Run)
	startedAt   time.Time
	duration    time.Duration
	stageTimes  map[string]time.Duration
	stageOrder  []string
	clock       Clock
	concurrent  bool // set by Pipeline.Run when a level runs stages concurrently; makes Snapshot deep-copy for per-stage read isolation
}

// markConcurrent tells the Scope its stages may run concurrently, so Snapshot
// must return a deep-isolated eval environment (no live references into mutable
// Scope state). Pipeline.Run calls it once before launching any stage goroutine,
// so the flag is published before any concurrent Snapshot reads it.
func (s *Scope) markConcurrent() {
	s.mu.Lock()
	s.concurrent = true
	s.mu.Unlock()
}

// ScopeOption configures a Scope.
type ScopeOption func(*Scope)

// WithStrict makes Set return ErrPathConflict when a leaf path already holds a
// value, guarding against accidental cross-stage output collisions.
func WithStrict() ScopeOption { return func(s *Scope) { s.strict = true } }

// NewScope returns a Scope owning a deep copy of the nested map[string]any spine
// of seed (nil is treated as empty), so a Scope never aliases caller-supplied
// maps: caller mutation of seed is not observed, and the pipeline never writes
// into the caller's data. Slices, structs, and scalars in seed are shared
// (referenced), since Set only ever writes into the map spine.
//
// Seed keys are used verbatim as top-level keys. A key containing "." is stored
// literally (not split into a nested path), so like the expr environment it is
// only addressable if a later dot-path Set nests it; flatten produces dot-free
// keys, so this matters only for callers seeding raw dotted keys.
func NewScope(seed map[string]any, opts ...ScopeOption) *Scope {
	s := &Scope{data: make(map[string]any, len(seed)), clock: realClock{}}
	for _, opt := range opts {
		opt(s)
	}
	for k, v := range seed {
		s.data[k] = cloneValue(v, 0)
	}
	if s.provenance {
		s.derivations = make(map[string]Derivation, len(s.data))
		for k, v := range s.data {
			s.derivations[k] = Derivation{Path: k, StageType: seedStageType, Operation: opSeed, Value: v}
		}
	}
	return s
}

// maxCloneDepth bounds cloneValue's recursion so a pathologically deep seed map
// (e.g. one decoded from deeply-nested untrusted JSON) cannot overflow the stack.
// Beyond it, deeper maps are shared rather than copied — the isolation guarantee
// lapses only at a depth no realistic stage write (a dot-path) could reach.
const maxCloneDepth = 1000

// cloneValue deep-copies the map[string]any spine of v (recursively through
// maps), returning any other kind unchanged. This isolates the writable surface
// of a Scope: Set only creates/traverses map[string]any nodes, so copying maps
// is sufficient to prevent aliasing of caller data.
func cloneValue(v any, depth int) any {
	m, ok := v.(map[string]any)
	if !ok || depth >= maxCloneDepth {
		return v
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = cloneValue(val, depth+1)
	}
	return out
}

// setPath stores v at the dot-separated path within root, creating intermediate
// maps as needed. An empty segment is ErrEmptyPathSegment; an intermediate
// non-map is ErrPathNotMap; in strict mode an existing leaf is ErrPathConflict.
func setPath(root map[string]any, path string, v any, strict bool) error {
	keys := strings.Split(path, ".")
	m := root
	for _, k := range keys[:len(keys)-1] {
		if k == "" {
			return ErrEmptyPathSegment
		}
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
	if leaf == "" {
		return ErrEmptyPathSegment
	}
	if strict {
		if _, exists := m[leaf]; exists {
			return ErrPathConflict
		}
	}
	m[leaf] = v
	return nil
}

// Set stores v at the dot-separated path, creating intermediate maps as needed.
// A zero-value Scope is usable: the backing map is allocated on first Set.
func (s *Scope) Set(path string, v any) error {
	if path == "" {
		return ErrEmptyPath
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string]any)
	}
	return setPath(s.data, path, v, s.strict)
}

// lookupPath resolves a dot-separated path by walking map[string]any levels of
// m, returning the value and true, or (nil, false) if the path is empty, a
// segment is missing, or an intermediate node is not a map[string]any. It is
// the shared read-traversal behind Scope.Get and foreach roll-up key
// resolution (the read counterpart of setPath).
func lookupPath(m map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	// Fast path: a single-segment key (the common case for both Scope.Get and
	// dot-free roll-up keys) is a direct map lookup, avoiding a strings.Split
	// allocation on this hot path.
	if !strings.Contains(path, ".") {
		v, ok := m[path]
		return v, ok
	}
	var cur any = m
	for _, k := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Get returns the value at the dot-separated path and whether it was present.
//
// For map or slice values the returned value is a live reference into the
// Scope's internal state; callers must not mutate it, nor read it
// concurrently with Set without external synchronization. Use Snapshot for a
// decoupled evaluation environment.
func (s *Scope) Get(path string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return lookupPath(s.data, path)
}

// Snapshot returns a copy of the accumulated data, suitable as an expr
// evaluation environment.
//
// By default the copy is shallow: nested map or slice values in the returned
// snapshot are live references into the Scope's internal state; callers must
// not mutate them, nor read them concurrently with Set without external
// synchronization. Under concurrent execution (Pipeline.WithConcurrency /
// WithMaxParallel), the Scope is marked concurrent and Snapshot instead
// deep-copies the map spine so each stage evaluates against an isolated
// environment — a sibling stage's Set can never mutate a map this stage is
// reading. Slices (and maps nested inside them) are shared either way: Set only
// ever traverses map[string]any by key and can never reach into a slice
// element, so those values are effectively immutable and safe to share.
func (s *Scope) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]any, len(s.data))
	if s.concurrent {
		for k, v := range s.data {
			out[k] = cloneValue(v, 0)
		}
		return out
	}
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
