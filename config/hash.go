package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
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
//
// Decimal literals are normalized (both the parsed source form {"$dec":"1.50"}
// and the Build-hydrated decimal.Decimal collapse to a single canonical
// {"$dec":"<normalized>"} shape) so the fingerprint is invariant to whether the
// definition has been hydrated by Build — otherwise Build would silently change
// the hash of a $dec ruleset and break MatchesRuleset on reload.
func (d *PipelineDef) canonicalJSON() ([]byte, error) {
	// Hash a copy with Version cleared and decimals normalized. The copy shares
	// the definition's non-decimal maps/slices and does not mutate the original.
	canonical := *d
	canonical.Version = ""
	canonical.Constants = canonicalizeMap(d.Constants)
	canonical.Schema = canonicalizeMap(d.Schema)
	canonical.Stages = canonicalizeStages(d.Stages)
	return json.Marshal(canonical)
}

// canonicalizeValue normalizes decimal literals to one canonical form so the
// content hash does not depend on hydration state: a hydrated decimal.Decimal
// and a parsed {"$dec":"<s>"} literal both collapse to {"$dec":"<d.String()>"},
// with the string re-normalized through decimal so "1.50" and "1.5" agree.
// Every other value is returned unchanged, recursing through maps and slices.
func canonicalizeValue(v any) any {
	switch x := v.(type) {
	case decimal.Decimal:
		return map[string]any{decLiteralKey: x.String()}
	case map[string]any:
		if raw, ok := x[decLiteralKey]; ok && len(x) == 1 {
			if s, ok := raw.(string); ok {
				if d, err := decimal.NewFromString(s); err == nil {
					return map[string]any{decLiteralKey: d.String()}
				}
			}
			return x // malformed literal: leave verbatim (Build rejects it anyway)
		}
		return canonicalizeMap(x)
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = canonicalizeValue(val)
		}
		return out
	default:
		return v
	}
}

// canonicalizeMap returns a copy of m with every value canonicalized, or nil for
// a nil map (preserving omitempty so a decimal-free definition hashes exactly as
// before).
func canonicalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = canonicalizeValue(v)
	}
	return out
}

// canonicalizeStages returns a copy of stages with every ExprDef.Globals map
// canonicalized (recursing into nested foreach sub-stages). Nil slices/maps are
// preserved so a decimal-free definition marshals byte-identically to before.
func canonicalizeStages(stages []StageDef) []StageDef {
	if stages == nil {
		return nil
	}
	out := make([]StageDef, len(stages))
	for i, sd := range stages {
		sd.Expr = canonicalizeExprDefPtr(sd.Expr)
		sd.Condition = canonicalizeExprDefPtr(sd.Condition)
		if sd.Exprs != nil {
			exprs := make([]NamedExprDef, len(sd.Exprs))
			for j, ne := range sd.Exprs {
				ne.Expr = canonicalizeExprDef(ne.Expr)
				exprs[j] = ne
			}
			sd.Exprs = exprs
		}
		if sd.Rules != nil {
			rules := make([]RuleDef, len(sd.Rules))
			for j, r := range sd.Rules {
				r.Condition = canonicalizeExprDef(r.Condition)
				r.Decisions = canonicalizeDecisions(r.Decisions)
				rules[j] = r
			}
			sd.Rules = rules
		}
		sd.Default = canonicalizeDecisions(sd.Default)
		sd.Stages = canonicalizeStages(sd.Stages)
		out[i] = sd
	}
	return out
}

func canonicalizeDecisions(m map[string]ExprDef) map[string]ExprDef {
	if m == nil {
		return nil
	}
	out := make(map[string]ExprDef, len(m))
	for k, ed := range m {
		out[k] = canonicalizeExprDef(ed)
	}
	return out
}

func canonicalizeExprDef(ed ExprDef) ExprDef {
	ed.Globals = canonicalizeMap(ed.Globals)
	return ed
}

func canonicalizeExprDefPtr(ed *ExprDef) *ExprDef {
	if ed == nil {
		return nil
	}
	c := *ed
	c.Globals = canonicalizeMap(c.Globals)
	return &c
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
	if d.hashMemo != nil {
		return *d.hashMemo
	}
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
