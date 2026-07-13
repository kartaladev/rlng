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
