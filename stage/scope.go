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
	mu          sync.RWMutex
	data        map[string]any
	strict      bool
	provenance  bool
	derivations map[string]Derivation // non-nil only when provenance is enabled
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
	if s.provenance {
		s.derivations = make(map[string]Derivation, len(data))
		for k, v := range data {
			s.derivations[k] = Derivation{Path: k, StageType: seedStageType, Operation: opSeed, Value: v}
		}
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
