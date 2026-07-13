package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/kartaladev/rlng/pipe"
)

// ErrUnhashableDef is the (wrapped) Cause of the ConfigError returned by Build
// when a definition cannot be canonically hashed because a field holds a
// non-JSON-marshalable value (e.g. a chan or func) in one of its any-typed
// fields (Constants, Schema, or an ExprDef.Globals). Only a hand-built
// PipelineDef can carry such a value — every Provider parse path decodes only
// JSON/YAML-native values — so this is reachable only from Go-constructed defs.
// Build rejects such a def rather than stamp it with a meaningless placeholder
// identity; see ADR-0045.
var ErrUnhashableDef = errors.New("config: definition is not canonically hashable (contains a non-JSON-marshalable value)")

// canonicalJSON returns the canonical JSON used for the content hash: the
// definition marshaled with Version cleared, so the release label never affects
// the fingerprint. A non-nil error is a json.Marshal failure, which only a
// hand-built definition carrying a non-marshalable value can trigger.
func (d *PipelineDef) canonicalJSON() ([]byte, error) {
	// Hash a copy with Version cleared. The copy shares the definition's
	// maps/slices but does not mutate them (marshal is read-only).
	canonical := *d
	canonical.Version = ""
	return json.Marshal(canonical)
}

// hashCanonical computes the hex SHA-256 of the canonical JSON, returning an
// error if the definition is not canonically marshalable. Build uses it to fail
// loud (ErrUnhashableDef); Hash falls back rather than surface the error.
func (d *PipelineDef) hashCanonical() (string, error) {
	b, err := d.canonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Hash returns a deterministic content fingerprint of the ruleset: the hex
// SHA-256 of the canonical JSON encoding of the parsed definition. Because it
// hashes the parsed structure (not the source bytes), the same logical ruleset
// hashes identically regardless of YAML vs JSON, formatting, comments, or map-key
// order (encoding/json sorts map keys); a changed rule or expression changes the
// hash. The author Version label is excluded, so re-labelling a release does not
// change what the hash proves. This is a plain fingerprint, not a tamper-proof
// signature (see ADR-0037).
//
// Hash assumes every field value is JSON-marshalable. A hand-built PipelineDef
// carrying a non-marshalable value (e.g. a chan or func) falls back here to a
// stable placeholder hash — Hash cannot return an error or panic on caller
// input. The fail-loud check lives at Build, which rejects such a def with
// ErrUnhashableDef (see ADR-0045); a caller who hand-hashes without Build owns
// supplying a marshalable def. Parse (any Provider) can never produce such a
// value, so this affects only hand-built definitions.
func (d *PipelineDef) Hash() string {
	h, err := d.hashCanonical()
	if err != nil {
		// Stable hash of empty content rather than a panic on caller input.
		sum := sha256.Sum256([]byte("{}"))
		return hex.EncodeToString(sum[:])
	}
	return h
}

// MatchesRuleset reports whether id's content hash equals this definition's
// Hash() — the replay-safety check: reload a candidate ruleset and compare it to
// a persisted decision's stamped identity so a replay against the wrong ruleset
// is detected rather than silent. It compares content (Hash) only; the Version
// label is not part of the match.
func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool {
	return d.Hash() == id.Hash
}
