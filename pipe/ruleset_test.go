package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			var opts []pipe.PipelineOption
			if tc.withRuleset {
				opts = append(opts, pipe.WithRuleset(id))
			}
			p, err := pipe.NewPipeline([]pipe.Stage{base}, opts...)
			require.NoError(t, err)

			sc := pipe.NewScope(nil)
			require.NoError(t, p.Run(t.Context(), sc))

			got, ok := sc.Ruleset()
			tc.assert(t, got, ok)
		})
	}
}
