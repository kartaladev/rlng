package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hashYAML = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
`

// The same logical ruleset as equivalent JSON (object-form expr, reordered keys).
const hashJSON = `{"stages":[{"expr":{"expr":"price * qty"},"type":"single-expr","name":"base"}]}`

// pre015HashYAML is a non-foreach ruleset whose Hash() is pinned to the value
// produced before the foreach schema fields were added to StageDef. The foreach
// fields must not perturb the fingerprint of a ruleset that uses none of them
// (they carry `omitempty`), so a decision persisted under an earlier release
// still MatchesRuleset after upgrading — replay-safety across the boundary.
const pre015HashYAML = `
constants:
  primeMin: 750
stages:
  - name: grade
    type: decision-table
    rules:
      - id: PRIME
        condition: score >= primeMin
        decisions:
          tier: '"prime"'
    default:
      tier: '"declined"'
`

// pre015Golden is the Hash() of pre015HashYAML as computed by the pre-foreach
// StageDef (verified against the `main` branch).
const pre015Golden = "35050a41fa676ae392d72d91e6a091ac2b6a72519c396d86c2b216fddb837de2"

func TestPipelineDefHash(t *testing.T) {
	tests := []struct {
		name   string
		build  func(t *testing.T) (string, string) // returns two hashes to compare
		assert func(t *testing.T, a, b string)
	}{
		{
			name: "YAML and equivalent JSON hash identically",
			build: func(t *testing.T) (string, string) {
				y, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML))
				require.NoError(t, err)
				j, err := config.Parse(t.Context(), config.FromJSONString(hashJSON))
				require.NoError(t, err)
				return y.Hash(), j.Hash()
			},
			assert: func(t *testing.T, a, b string) {
				assert.Equal(t, a, b)
				assert.Len(t, a, 64, "hex sha256 is 64 chars")
			},
		},
		{
			name: "version does not affect the content hash",
			build: func(t *testing.T) (string, string) {
				d1, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML))
				require.NoError(t, err)
				d2, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML+"version: v9.9.9\n"))
				require.NoError(t, err)
				return d1.Hash(), d2.Hash()
			},
			assert: func(t *testing.T, a, b string) { assert.Equal(t, a, b) },
		},
		{
			name: "foreach schema fields do not perturb a pre-015 ruleset's hash (cross-version replay stability)",
			build: func(t *testing.T) (string, string) {
				d, err := config.Parse(t.Context(), config.FromYAMLString(pre015HashYAML))
				require.NoError(t, err)
				return d.Hash(), pre015Golden
			},
			assert: func(t *testing.T, a, b string) { assert.Equal(t, b, a) },
		},
		{
			name: "a changed expression changes the hash",
			build: func(t *testing.T) (string, string) {
				d1, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML))
				require.NoError(t, err)
				d2, err := config.Parse(t.Context(), config.FromYAMLString("stages:\n  - name: base\n    type: single-expr\n    expr: price * qty * 2\n"))
				require.NoError(t, err)
				return d1.Hash(), d2.Hash()
			},
			assert: func(t *testing.T, a, b string) { assert.NotEqual(t, a, b) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := tt.build(t)
			tt.assert(t, a, b)
		})
	}
}

// unhashableDef hand-builds a PipelineDef carrying a non-JSON-marshalable value
// (a func) in Constants, with a stage whose expression references no constant so
// the func is unreferenced and Build reaches the ruleset-identity stamp. The
// parse path can never produce such a value, so this is a Go-hand-built def.
func unhashableDef() *config.PipelineDef {
	return &config.PipelineDef{
		Constants: map[string]any{"bad": func() {}},
		Stages: []config.StageDef{
			{Name: "s", Type: "single-expr", Expr: &config.ExprDef{Expr: "1 + 1"}},
		},
	}
}

// TestBuildRejectsUnhashableDef pins B4: a hand-built def whose canonical JSON
// cannot be produced is rejected at Build with ErrUnhashableDef instead of being
// silently stamped with the placeholder hash.
func TestBuildRejectsUnhashableDef(t *testing.T) {
	_, err := unhashableDef().Build()

	require.Error(t, err)
	require.ErrorIs(t, err, config.ErrUnhashableDef)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
}

// TestHashPlaceholderForUnmarshalableDef documents the retained fallback: a
// direct Hash() on an unmarshalable def does not panic and returns a stable
// 64-char placeholder (the fail-loud check lives at Build, not Hash).
func TestHashPlaceholderForUnmarshalableDef(t *testing.T) {
	d := unhashableDef()

	h1 := d.Hash()
	h2 := d.Hash()

	assert.Len(t, h1, 64)
	assert.Equal(t, h1, h2)
}

func TestPipelineDefMatchesRuleset(t *testing.T) {
	d, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML))
	require.NoError(t, err)

	tests := []struct {
		name   string
		id     pipe.RulesetIdentity
		assert func(t *testing.T, matches bool)
	}{
		{
			name: "matching hash",
			id:   pipe.RulesetIdentity{Hash: d.Hash()},
			assert: func(t *testing.T, matches bool) {
				assert.True(t, matches)
			},
		},
		{
			name: "mismatched hash",
			id:   pipe.RulesetIdentity{Hash: "nope"},
			assert: func(t *testing.T, matches bool) {
				assert.False(t, matches)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, d.MatchesRuleset(tt.id))
		})
	}
}
