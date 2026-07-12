package pipe

import (
	"bytes"
	"encoding/json"
	"time"
)

// scopeJSON is the on-wire envelope for a Scope: the accumulated result data,
// evaluation timing, and (when provenance is enabled) the derivations.
type scopeJSON struct {
	Data        map[string]any          `json:"data"`
	Timing      *scopeTimingJSON        `json:"timing,omitempty"`
	Derivations map[string]Derivation   `json:"derivations,omitempty"`
	Ruleset     *RulesetIdentity        `json:"ruleset,omitempty"`
	Firing      map[string][]FiringRule `json:"firing,omitempty"`
}

type scopeTimingJSON struct {
	StartedAt  time.Time `json:"started_at"`
	DurationNS int64     `json:"duration_ns"`
}

// MarshalJSON serializes the Scope as a round-trippable envelope
// {data, timing?, derivations?} suitable for a jsonb column. `timing` appears
// after a run; `derivations` only when provenance is enabled. For just the
// result data (e.g. a web response) marshal Snapshot() instead.
func (s *Scope) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	env := scopeJSON{Data: s.data}
	if !s.startedAt.IsZero() {
		env.Timing = &scopeTimingJSON{StartedAt: s.startedAt, DurationNS: s.duration.Nanoseconds()}
	}
	if s.provenance {
		env.Derivations = s.derivations
	}
	if s.ruleset != (RulesetIdentity{}) {
		id := s.ruleset
		env.Ruleset = &id
	}
	if len(s.firing) > 0 {
		env.Firing = s.firing
	}
	return json.Marshal(env)
}

// UnmarshalJSON restores a Scope from the envelope produced by MarshalJSON:
// result data, timing, and — when derivations are present — provenance state so
// Derivation/Lineage/Explain work on the restored Scope. A restored Scope is
// for inspection, not re-execution. Numbers in data and derivations are
// restored as json.Number (exact decimal text, no float64 rounding), so
// integers of any magnitude round-trip losslessly. The int/float type
// distinction is not preserved across JSON — read reloaded numbers with the
// typed getters (GetInt/GetInt64/GetFloat64).
//
// UnmarshalJSON must not be called on a Scope shared with a concurrent
// reader — it restores provenance state, so treat unmarshal as construction
// of a fresh Scope.
func (s *Scope) UnmarshalJSON(b []byte) error {
	var env scopeJSON
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = env.Data
	if s.data == nil {
		s.data = map[string]any{}
	}
	if s.clock == nil {
		s.clock = realClock{}
	}
	if env.Timing != nil {
		s.startedAt = env.Timing.StartedAt
		s.duration = time.Duration(env.Timing.DurationNS)
	}
	if env.Derivations != nil {
		s.provenance = true
		s.derivations = env.Derivations
	}
	if env.Ruleset != nil {
		s.ruleset = *env.Ruleset
	}
	if env.Firing != nil {
		s.firing = env.Firing
	}
	return nil
}
