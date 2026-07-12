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
		{
			name: "provenance on: single mode records the winning rule's derivation",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
					{Condition: "amount >= 100", Decisions: map[string]string{"level": `"silver"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)

				d, ok := sc.Derivation("tier.level")
				require.True(t, ok)
				assert.Equal(t, "tier", d.Stage)
				assert.Equal(t, TypeDecisionTable, d.StageType)
				assert.Equal(t, "decision:level", d.Operation)
				assert.Equal(t, `"gold"`, d.Expression)
				assert.Equal(t, "gold", d.Value)
			},
		},
		{
			name: "provenance on: collect mode joins expressions and unions inputs across matched rules",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": "label1"}},
					{Condition: "amount >= 1000", Decisions: map[string]string{"tag": "label2"}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000, "label1": "big", "label2": "huge"}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)

				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "huge"}, tags)

				d, ok := sc.Derivation("tags.tag")
				require.True(t, ok)
				assert.Equal(t, "tags", d.Stage)
				assert.Equal(t, TypeDecisionTable, d.StageType)
				assert.Equal(t, "collect:tag", d.Operation)
				assert.Equal(t, "label1; label2", d.Expression)
				assert.Equal(t, map[string]any{"label1": "big", "label2": "huge"}, d.Inputs)
				assert.Equal(t, []any{"big", "huge"}, d.Value)

				explained := sc.Explain("tags.tag")
				assert.Contains(t, explained, "tags.tag = [big huge] [tags decision-table] expr: label1; label2")
				assert.Contains(t, explained, "label1 = big [seed]")
				assert.Contains(t, explained, "label2 = huge [seed]")
			},
		},
		{
			name: "provenance on: single mode write conflict surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				// The scalar seed "tier" collides with the stage namespace, so
				// Derive's Set("tier.level", …) fails with ErrPathNotMap.
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"tier": 1, "amount": 5000}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.Equal(t, TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
		{
			name: "provenance on: collect mode write conflict surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				// The scalar seed "tags" collides with the stage namespace, so
				// Derive's Set("tags.tag", …) fails with ErrPathNotMap.
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": "label1"}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"tags": 1, "amount": 5000, "label1": "big"}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
		{
			name: "single mode: decision eval error surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"level": "a % b"}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000, "a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.Equal(t, TypeDecisionTable, se.Type)
			},
		},
		{
			name: "single mode: write conflict surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				// Scalar seed "tier" collides with the stage namespace, so
				// Set("tier.level", …) fails with ErrPathNotMap (provenance off).
				d, err := NewDecisionTable("tier", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"level": `"gold"`}},
				})
				require.NoError(t, err)
				return d, NewScope(map[string]any{"tier": 1, "amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
		{
			name: "collect mode: condition eval error surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "a % b == 0", Decisions: map[string]string{"tag": `"x"`}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, TypeDecisionTable, se.Type)
			},
		},
		{
			name: "collect mode: decision eval error surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": "a % b"}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000, "a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, TypeDecisionTable, se.Type)
			},
		},
		{
			name: "collect mode: write conflict surfaces as StageError",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				// Scalar seed "tags" collides with the stage namespace, so
				// Set("tags.tag", …) fails with ErrPathNotMap (provenance off).
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 100", Decisions: map[string]string{"tag": `"big"`}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"tags": 1, "amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
		{
			name: "collect mode: non-matching rule is skipped",
			build: func(t *testing.T) (*DecisionTable, *Scope) {
				d, err := NewDecisionTable("tags", []Rule{
					{Condition: "amount >= 1000", Decisions: map[string]string{"tag": `"big"`}},
					{Condition: "amount >= 100000", Decisions: map[string]string{"tag": `"huge"`}},
				}, WithHitPolicy(HitPolicyCollect))
				require.NoError(t, err)
				return d, NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big"}, tags)
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
		{
			name:      "empty output key is rejected",
			stageName: "t",
			rules:     []Rule{{Condition: "true", Decisions: map[string]string{"": "1"}}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeDecisionTable, se.Type)
			},
		},
		{
			name:      "bad decision expression is a compile error",
			stageName: "t",
			rules:     []Rule{{Condition: "true", Decisions: map[string]string{"y": "x +"}}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeDecisionTable, se.Type)
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

func TestDecisionTableAccessors(t *testing.T) {
	t.Parallel()

	d, err := NewDecisionTable("tier",
		[]Rule{{Condition: "true", Decisions: map[string]string{"level": `"gold"`}}},
		WithDependsOn("amount"))
	require.NoError(t, err)
	assert.Equal(t, "tier", d.Name())
	assert.Equal(t, TypeDecisionTable, d.Type())
	assert.Equal(t, []string{"amount"}, d.DependsOn())
}
