package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/kartaladev/rlng/pipe"
)

// Hash returns a deterministic content fingerprint of the ruleset: the hex
// SHA-256 of the canonical JSON encoding of the parsed definition. Because it
// hashes the parsed structure (not the source bytes), the same logical ruleset
// hashes identically regardless of YAML vs JSON, formatting, comments, or map-key
// order (encoding/json sorts map keys); a changed rule or expression changes the
// hash. The author Version label is excluded, so re-labelling a release does not
// change what the hash proves. This is a plain fingerprint, not a tamper-proof
// signature (see ADR-0037).
func (d *PipelineDef) Hash() string {
	// Hash a copy with Version cleared so the label never affects the content
	// fingerprint. The copy shares the definition's maps/slices but does not
	// mutate them (marshal is read-only).
	canonical := *d
	canonical.Version = ""
	b, err := json.Marshal(canonical)
	if err != nil {
		// PipelineDef is composed of JSON-marshalable types, so marshal cannot
		// realistically fail; fall back to a stable hash of empty content rather
		// than panic on caller input.
		b = []byte("{}")
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// MatchesRuleset reports whether id's content hash equals this definition's
// Hash() — the replay-safety check: reload a candidate ruleset and compare it to
// a persisted decision's stamped identity so a replay against the wrong ruleset
// is detected rather than silent. It compares content (Hash) only; the Version
// label is not part of the match.
func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool {
	return d.Hash() == id.Hash
}
