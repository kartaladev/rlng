package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionTableExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*DecisionTable, *Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "single mode: first match wins",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
					{Condition: "amount >= 100", Decisions: map[string]string{"level": `"silver"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				level, ok := sc.Get("tier.level")
				require.True(t, ok)
				assert.Equal(t, "gold", level)
			},
		},
		{
			name: "single mode: no match writes nothing",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("tier.level")
				assert.False(t, ok)
			},
		},
		{
			name: "collect mode: accumulates matches in rule order",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": `"big"`}},
					{Condition: "amount >= 1000", Decisions: map[string]string{"tag": `"huge"`}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "huge"}, tags)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("t", []Rule{
					{Condition: "a % b > 0", Decisions: map[string]string{"x": "1"}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "t", se.Stage)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "true", Decisions: map[string]string{"x": "1"}},
				})
				require.NoError(t, err)
				return d, NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := d.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewDecisionTableValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		stageName string
		rules     []Rule
		assert    func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:      "empty rule set is rejected",
			stageName: "t",
			rules:     nil,
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "empty stage name is rejected",
			stageName: "",
			rules:     []Rule{{Condition: "true", Decisions: map[string]string{"y": "1"}}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, errEmptyStageName)
			},
		},
		{
			name:      "rule without decisions is rejected",
			stageName: "t",
			rules:     []Rule{{Condition: "true", Decisions: nil}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "bad condition is a compile error",
			stageName: "t",
			rules:     []Rule{{Condition: "x +", Decisions: map[string]string{"y": "1"}}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewDecisionTable(tc.stageName, tc.rules)
			tc.assert(t, err)
		})
	}
}
