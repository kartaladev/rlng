package pipe_test

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFiringRule(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope)
		assert func(t *testing.T, sc *pipe.Scope)
	}

	cases := []testCase{
		{
			name: "single: firing rule id and message recorded",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("grade", []pipe.Rule{
					{ID: "PRIME", Message: "prime tier", Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`}},
					{ID: "SUB", Condition: "true", Decisions: map[string]string{"tier": `"sub"`}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800})
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				fr, ok := sc.FiringRule("grade")
				require.True(t, ok)
				assert.Equal(t, "PRIME", fr.RuleID)
				assert.Equal(t, "prime tier", fr.Message)
				assert.False(t, fr.IsDefault)
			},
		},
		{
			name: "unique: firing rule recorded",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{ID: "A", Condition: "score >= 750", Decisions: map[string]string{"x": "1"}},
					{ID: "B", Condition: "score < 600", Decisions: map[string]string{"x": "2"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyUnique))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800})
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				fr, ok := sc.FiringRule("g")
				require.True(t, ok)
				assert.Equal(t, "A", fr.RuleID)
			},
		},
		{
			name: "default fired is flagged",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{ID: "A", Condition: "false", Decisions: map[string]string{"x": "1"}},
				}, pipe.WithDefault(map[string]string{"x": "9"}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				fr, ok := sc.FiringRule("g")
				require.True(t, ok)
				assert.True(t, fr.IsDefault)
				assert.Equal(t, "", fr.RuleID)
			},
		},
		{
			name: "no match and no default records nothing",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{ID: "A", Condition: "false", Decisions: map[string]string{"x": "1"}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				_, ok := sc.FiringRule("g")
				assert.False(t, ok)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, sc := tc.build(t)
			require.NoError(t, d.Execute(context.Background(), sc))
			tc.assert(t, sc)
		})
	}
}

func TestFiringRulesAll(t *testing.T) {
	t.Parallel()

	a, err := pipe.NewDecisionTable("a", []pipe.Rule{
		{ID: "RA", Condition: "true", Decisions: map[string]string{"x": "1"}},
	})
	require.NoError(t, err)
	b, err := pipe.NewDecisionTable("b", []pipe.Rule{
		{ID: "RB", Condition: "true", Decisions: map[string]string{"y": "2"}},
	})
	require.NoError(t, err)
	p, err := pipe.NewPipeline(a, b)
	require.NoError(t, err)

	sc := pipe.NewScope(map[string]any{})
	require.NoError(t, p.Run(context.Background(), sc))

	all := sc.FiringRules()
	require.Len(t, all, 2)
	assert.Equal(t, "a", all[0].Stage)
	assert.Equal(t, "RA", all[0].RuleID)
	assert.Equal(t, "b", all[1].Stage)
	assert.Equal(t, "RB", all[1].RuleID)
}

func TestScopeFiringRulesForSingleMatch(t *testing.T) {
	t.Parallel()
	// A first-match (single) table records the matched rule; FiringRulesFor
	// returns it as a one-element slice, and FiringRule returns that first rule.
	tbl, err := pipe.NewDecisionTable("t", []pipe.Rule{
		{ID: "R1", Condition: "x > 0", Decisions: map[string]string{"tag": `"a"`}},
	}, pipe.WithHitPolicy(pipe.HitPolicySingle))
	require.NoError(t, err)
	sc := pipe.NewScope(map[string]any{"x": 2})
	require.NoError(t, tbl.Execute(context.Background(), sc))

	got := sc.FiringRulesFor("t")
	require.Len(t, got, 1)
	assert.Equal(t, "R1", got[0].RuleID)

	first, ok := sc.FiringRule("t")
	require.True(t, ok)
	assert.Equal(t, "R1", first.RuleID)

	assert.Len(t, sc.FiringRules(), 1)
}

func TestScopeFiringRulesForAbsent(t *testing.T) {
	t.Parallel()
	sc := pipe.NewScope(nil)
	assert.Nil(t, sc.FiringRulesFor("nope"))
	_, ok := sc.FiringRule("nope")
	assert.False(t, ok)
}
