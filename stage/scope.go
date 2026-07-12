package stage

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

// errEmptyPath is returned when Set is given an empty path.
var errEmptyPath = errors.New("scope: path must not be empty")

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
	derivations map[string]Derivation // non-nil only when provenance is enabled
	startedAt   time.Time
	duration    time.Duration
	clock       func() time.Time
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
	s := &Scope{data: make(map[string]any, len(seed)), clock: time.Now}
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
		return errEmptyPath
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string]any)
	}
	return setPath(s.data, path, v, s.strict)
}

// Get returns the value at the dot-separated path and whether it was present.
//
// For map or slice values the returned value is a live reference into the
// Scope's internal state; callers must not mutate it, nor read it
// concurrently with Set without external synchronization. Use Snapshot for a
// decoupled evaluation environment.
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
//
// The copy is only shallow: nested map or slice values in the returned
// snapshot are live references into the Scope's internal state; callers must
// not mutate them, nor read them concurrently with Set without external
// synchronization.
func (s *Scope) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
