package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionTablePolicies(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope)
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}

	cases := []testCase{
		{
			name: "single: no match applies default",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`}},
				}, pipe.WithDefault(map[string]string{"tier": `"declined"`}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 500})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, ok := sc.Get("tier.tier")
				require.True(t, ok)
				assert.Equal(t, "declined", v)
			},
		},
		{
			name: "unique: exactly one match writes it",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`}},
					{Condition: "score < 600", Decisions: map[string]string{"tier": `"subprime"`}},
				}, pipe.WithHitPolicy(pipe.HitPolicyUnique))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("g.tier")
				assert.Equal(t, "prime", v)
			},
		},
		{
			name: "unique: multiple matches is an error",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "score >= 750", Decisions: map[string]string{"tier": `"a"`}},
					{Condition: "score >= 700", Decisions: map[string]string{"tier": `"b"`}},
				}, pipe.WithHitPolicy(pipe.HitPolicyUnique))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipe.ErrMultipleMatches)
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "any: agreeing matches write the agreed value",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "score >= 750", Decisions: map[string]string{"ok": "true"}},
					{Condition: "income > 50000", Decisions: map[string]string{"ok": "true"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800, "income": 60000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("g.ok")
				assert.Equal(t, true, v)
			},
		},
		{
			name: "any: numeric int vs float agree without a false conflict",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "true", Decisions: map[string]string{"amt": "10"}},
					{Condition: "true", Decisions: map[string]string{"amt": "10.0"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, ok := sc.Get("g.amt")
				require.True(t, ok)
				assert.Equal(t, 10, v, "the first-seen value is kept")
			},
		},
		{
			name: "any: numeric disagreement is still a conflict",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "true", Decisions: map[string]string{"amt": "10"}},
					{Condition: "true", Decisions: map[string]string{"amt": "11"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipe.ErrConflictingMatches)
			},
		},
		{
			name: "any: conflicting matches is an error",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "score >= 750", Decisions: map[string]string{"tier": `"a"`}},
					{Condition: "income > 50000", Decisions: map[string]string{"tier": `"b"`}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 800, "income": 60000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipe.ErrConflictingMatches)
				assert.Empty(t, sc.FiringRulesFor("g"), "a conflict must not record a firing")
			},
		},
		{
			name: "any: records firing per agreeing rule",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("decide", []pipe.Rule{
					{ID: "A", Condition: "x > 0", Decisions: map[string]string{"ok": "true"}},
					{ID: "B", Condition: "x > 1", Decisions: map[string]string{"ok": "true"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"x": 2})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				fired := sc.FiringRulesFor("decide")
				require.Len(t, fired, 2)
				assert.Equal(t, "A", fired[0].RuleID)
				assert.Equal(t, "B", fired[1].RuleID)
			},
		},
		{
			name: "collect sum aggregates numeric decisions",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("fees", []pipe.Rule{
					{Condition: "wire", Decisions: map[string]string{"fee": "25"}},
					{Condition: "rush", Decisions: map[string]string{"fee": "15"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"wire": true, "rush": true})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("fees.fee")
				assert.Equal(t, int64(40), v, "an all-int sum stays exact in int64")
			},
		},
		{
			name: "collect count aggregates match count",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("flags", []pipe.Rule{
					{Condition: "score < 650", Decisions: map[string]string{"n": `"low"`}},
					{Condition: "dti > 0.4", Decisions: map[string]string{"n": `"high"`}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateCount))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"score": 600, "dti": 0.5})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("flags.n")
				assert.Equal(t, 2, v)
			},
		},
		{
			name: "collect max/min aggregate extremes",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("disc", []pipe.Rule{
					{Condition: "true", Decisions: map[string]string{"pct": "10"}},
					{Condition: "loyal", Decisions: map[string]string{"pct": "25"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateMax))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"loyal": true})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("disc.pct")
				assert.Equal(t, 25, v)
			},
		},
		{
			name: "collect: records firing per match",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("fees", []pipe.Rule{
					{ID: "BASE", Condition: "true", Decisions: map[string]string{"fee": "10"}},
					{ID: "SURCHARGE", Condition: "risky", Decisions: map[string]string{"fee": "5"}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"risky": true})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				fired := sc.FiringRulesFor("fees")
				require.Len(t, fired, 2)
				assert.Equal(t, "BASE", fired[0].RuleID)
				assert.Equal(t, "SURCHARGE", fired[1].RuleID)
			},
		},
		{
			name: "collect sum over non-numeric is an error",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("x", []pipe.Rule{
					{Condition: "true", Decisions: map[string]string{"v": `"a"`}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipe.ErrNonNumericAggregate)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, sc := tc.build(t)
			err := d.Execute(t.Context(), sc)
			tc.assert(t, sc, err)
		})
	}
}
