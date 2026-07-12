package pipe

import (
	"bytes"
	"encoding/json"
	"time"
)

// scopeJSON is the on-wire envelope for a Scope: the accumulated result data,
// evaluation timing, and (when provenance is enabled) the derivations.
//
// Version is the envelope schema marker (Spec 014 / ADR-0038 D3): omitted (0)
// for a legacy, pre-014 blob whose Data values are bare JSON
// scalars/objects/arrays; 2 for the canonical type-tagged encoding where every
// Data value (recursively) is a {"$k":<kind>,"v":<payload>} taggedValue. Only
// Data is tagged — Derivations, Timing, Ruleset, and Firing keep their
// existing (Spec 013 and earlier) representations unchanged.
type scopeJSON struct {
	Version     int                     `json:"v,omitempty"`
	Data        map[string]any          `json:"data"`
	Timing      *scopeTimingJSON        `json:"timing,omitempty"`
	Derivations map[string]Derivation   `json:"derivations,omitempty"`
	Ruleset     *RulesetIdentity        `json:"ruleset,omitempty"`
	Firing      map[string][]FiringRule `json:"firing,omitempty"`
}

// scopeJSONVersion is the current envelope schema version written by
// MarshalJSON (ADR-0038 D3). A reader branches on scopeJSON.Version to decide
// whether Data values are type-tagged (>= 2) or legacy bare values (0).
const scopeJSONVersion = 2

type scopeTimingJSON struct {
	StartedAt  time.Time `json:"started_at"`
	DurationNS int64     `json:"duration_ns"`
}

// MarshalJSON serializes the Scope as a round-trippable envelope
// {v, data, timing?, derivations?, ruleset?, firing?} suitable for a jsonb
// column. `v` is the schema version (Spec 014 / ADR-0038 D3) and is always 2
// for a freshly-marshaled Scope: every `data` value (recursively, including
// inside nested maps/lists) is written type-tagged, so its Go kind — bool,
// string, int64, float64, decimal.Decimal, time.Time, list, map — survives
// the round-trip exactly, not just its printed text. `timing` appears after a
// run; `derivations` only when provenance is enabled (derivation values keep
// their pre-existing bare-JSON-number representation, unaffected by this
// task's data tagging); `ruleset` only when the Scope was stamped by a
// Pipeline.WithRuleset pipeline's Run; `firing` only when a decision-table
// stage recorded a firing rule. The ruleset stamp and firing rules round-trip
// through Unmarshal/MarshalJSON, so a reloaded Scope's Ruleset() and
// FiringRule(s)/FiringRulesFor report the same values as before persisting.
// For just the result data (e.g. a web response) marshal Snapshot() instead.
func (s *Scope) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	taggedData := make(map[string]any, len(s.data))
	for k, v := range s.data {
		tagged, err := encodeValue(v)
		if err != nil {
			return nil, err
		}
		taggedData[k] = tagged
	}

	env := scopeJSON{Version: scopeJSONVersion, Data: taggedData}
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
// for inspection, not re-execution.
//
// A v2 envelope (Spec 014 / ADR-0038 D3, the current MarshalJSON output)
// carries type-tagged `data` values, so a reloaded value has the same Go kind
// it had before persisting: an int64 reloads as int64, a decimal.Decimal as
// itself, a time.Time as itself, and so on — read it directly or with the
// typed getters. A legacy (pre-014, untagged) blob still loads via the
// pre-existing bare-value path: numbers restore as json.Number (exact decimal
// text, no float64 rounding, so integers of any magnitude round-trip
// losslessly) and the int/float distinction is not preserved — read those
// with the typed getters (GetInt/GetInt64/GetFloat64). Derivation values are
// always restored via the legacy bare-value path (as json.Number, etc.)
// regardless of envelope version — only `data` is type-tagged by this task.
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

	if env.Version >= 2 {
		untagged := make(map[string]any, len(env.Data))
		for k, v := range env.Data {
			decoded, err := decodeValue(v)
			if err != nil {
				return err
			}
			untagged[k] = decoded
		}
		env.Data = untagged
	}

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
