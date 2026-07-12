package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRulesetReturnsReceiver(t *testing.T) {
	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := pipe.NewPipeline(base)
	require.NoError(t, err)

	id := pipe.RulesetIdentity{Hash: "abc123", Version: "v1.2.0"}
	require.Same(t, p, p.WithRuleset(id), "WithRuleset returns the same pipeline for chaining")
}

func TestScopeRuleset(t *testing.T) {
	type testCase struct {
		name        string
		withRuleset bool
		assert      func(t *testing.T, got pipe.RulesetIdentity, ok bool)
	}

	id := pipe.RulesetIdentity{Hash: "abc123", Version: "v1.2.0"}

	cases := []testCase{
		{
			name:        "pipeline stamped with ruleset marks the scope",
			withRuleset: true,
			assert: func(t *testing.T, got pipe.RulesetIdentity, ok bool) {
				require.True(t, ok)
				assert.Equal(t, id, got)
			},
		},
		{
			name:        "pipeline without ruleset leaves the scope unstamped",
			withRuleset: false,
			assert: func(t *testing.T, _ pipe.RulesetIdentity, ok bool) {
				assert.False(t, ok, "an un-stamped Scope reports no ruleset identity")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, err := pipe.NewSingleExpr("base", "1 + 1")
			require.NoError(t, err)
			p, err := pipe.NewPipeline(base)
			require.NoError(t, err)
			if tc.withRuleset {
				p.WithRuleset(id)
			}

			sc := pipe.NewScope(nil)
			require.NoError(t, p.Run(t.Context(), sc))

			got, ok := sc.Ruleset()
			tc.assert(t, got, ok)
		})
	}
}
