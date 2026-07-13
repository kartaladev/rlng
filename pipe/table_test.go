package pipe_test

import (
	"context"
	"math"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionTableExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}

	cases := []testCase{
		{
			name: "single mode: first match wins",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"level": {Expr: `"silver"`}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				level, ok := sc.Get("tier.level")
				require.True(t, ok)
				assert.Equal(t, "gold", level)
			},
		},
		{
			name: "single mode: no match writes nothing",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("tier.level")
				assert.False(t, ok)
			},
		},
		{
			name: "collect mode: accumulates matches in rule order",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"tag": {Expr: `"big"`}}},
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"tag": {Expr: `"huge"`}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "huge"}, tags)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("t", []pipe.Rule{
					{Condition: "a % b > 0", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "t", se.Stage)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
		{
			name: "provenance on: single mode records the winning rule's derivation",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"level": {Expr: `"silver"`}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				d, ok := sc.Derivation("tier.level")
				require.True(t, ok)
				assert.Equal(t, "tier", d.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, d.StageType)
				assert.Equal(t, "decision:level", d.Operation)
				assert.Equal(t, `"gold"`, d.Expression)
				assert.Equal(t, "gold", d.Value)
			},
		},
		{
			name: "provenance on: collect mode joins expressions and unions inputs across matched rules",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"tag": {Expr: "label1"}}},
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"tag": {Expr: "label2"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000, "label1": "big", "label2": "huge"}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				tags, ok := sc.Get("tags.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "huge"}, tags)

				d, ok := sc.Derivation("tags.tag")
				require.True(t, ok)
				assert.Equal(t, "tags", d.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, d.StageType)
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
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				// The scalar seed "tier" collides with the stage namespace, so
				// Derive's Set("tier.level", …) fails with ErrPathNotMap.
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"tier": 1, "amount": 5000}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "provenance on: collect mode write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				// The scalar seed "tags" collides with the stage namespace, so
				// Derive's Set("tags.tag", …) fails with ErrPathNotMap.
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"tag": {Expr: "label1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"tags": 1, "amount": 5000, "label1": "big"}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "single mode: decision eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"level": {Expr: "a % b"}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000, "a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
			},
		},
		{
			name: "single mode: write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				// Scalar seed "tier" collides with the stage namespace, so
				// Set("tier.level", …) fails with ErrPathNotMap (provenance off).
				d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
				})
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"tier": 1, "amount": 5000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tier", se.Stage)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "collect mode: condition eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "a % b == 0", Decisions: map[string]pipe.Decision{"tag": {Expr: `"x"`}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
			},
		},
		{
			name: "collect mode: decision eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"tag": {Expr: "a % b"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000, "a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
			},
		},
		{
			name: "collect mode: write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				// Scalar seed "tags" collides with the stage namespace, so
				// Set("tags.tag", …) fails with ErrPathNotMap (provenance off).
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"tag": {Expr: `"big"`}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"tags": 1, "amount": 5000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "tags", se.Stage)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "collect mode: non-matching rule is skipped",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("tags", []pipe.Rule{
					{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"tag": {Expr: `"big"`}}},
					{Condition: "amount >= 100000", Decisions: map[string]pipe.Decision{"tag": {Expr: `"huge"`}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"amount": 5000})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
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
		rules     []pipe.Rule
		assert    func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:      "empty rule set is rejected",
			stageName: "t",
			rules:     nil,
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "empty stage name is rejected",
			stageName: "",
			rules:     []pipe.Rule{{Condition: "true", Decisions: map[string]pipe.Decision{"y": {Expr: "1"}}}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
				assert.ErrorIs(t, se, pipe.ErrEmptyStageName)
			},
		},
		{
			name:      "rule without decisions is rejected",
			stageName: "t",
			rules:     []pipe.Rule{{Condition: "true", Decisions: nil}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "bad condition is a compile error",
			stageName: "t",
			rules:     []pipe.Rule{{Condition: "x +", Decisions: map[string]pipe.Decision{"y": {Expr: "1"}}}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "empty output key is rejected",
			stageName: "t",
			rules:     []pipe.Rule{{Condition: "true", Decisions: map[string]pipe.Decision{"": {Expr: "1"}}}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
			},
		},
		{
			name:      "bad decision expression is a compile error",
			stageName: "t",
			rules:     []pipe.Rule{{Condition: "true", Decisions: map[string]pipe.Decision{"y": {Expr: "x +"}}}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeDecisionTable, se.Type)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := pipe.NewDecisionTable(tc.stageName, tc.rules)
			tc.assert(t, err)
		})
	}
}

// TestDecisionTableCollectAggregationFidelity covers the numeric-fidelity
// aggregation rewrite of foldNumeric (G2): int64 sums stay exact (with checked
// overflow), a decimal present in the matched values folds in decimal, mixed
// kinds promote to the widest kind present, and min/max always return the
// actual matched element rather than a reconstructed value.
func TestDecisionTableCollectAggregationFidelity(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		agg  pipe.CollectAggregation
		// exprs is one decision expression per rule; every rule matches
		// ("true"), and each contributes one value to key "v".
		exprs []string
		// seed pre-populates the Scope so an expr can reference a Go-typed
		// value directly (e.g. a uint) rather than only expr literals.
		seed   map[string]any
		assert func(t *testing.T, got any, err error)
	}

	cases := []testCase{
		{
			name:  "int sum above 2^53 stays exact int64",
			agg:   pipe.AggregateSum,
			exprs: []string{"9007199254740993", "1"}, // 2^53+1, then +1
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, int64(9007199254740994), got)
			},
		},
		{
			name:  "int sum overflow errors, not garbage",
			agg:   pipe.AggregateSum,
			exprs: []string{"9223372036854775807", "1"}, // math.MaxInt64 + 1
			assert: func(t *testing.T, got any, err error) {
				require.ErrorIs(t, err, pipe.ErrAggregateOverflow)
			},
		},
		{
			// The two-MinInt64 case wraps to exactly 0 under the naive
			// sign-mismatch overflow check ((acc<0 && n<0 && sum>0) is
			// false when sum==0), so it must be exercised explicitly.
			name:  "int sum of two MinInt64 overflows, not silently zero",
			agg:   pipe.AggregateSum,
			exprs: []string{"a", "b"},
			seed:  map[string]any{"a": int64(math.MinInt64), "b": int64(math.MinInt64)},
			assert: func(t *testing.T, got any, err error) {
				require.ErrorIs(t, err, pipe.ErrAggregateOverflow)
			},
		},
		{
			name:  "int sum negative overflow errors (MinInt64 + -1)",
			agg:   pipe.AggregateSum,
			exprs: []string{"a", "-1"},
			seed:  map[string]any{"a": int64(math.MinInt64)},
			assert: func(t *testing.T, got any, err error) {
				require.ErrorIs(t, err, pipe.ErrAggregateOverflow)
			},
		},
		{
			name:  "int sum mixed sign to a normal negative does not overflow",
			agg:   pipe.AggregateSum,
			exprs: []string{"-5", "3"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, int64(-2), got)
			},
		},
		{
			name:  "decimal sum is exact (0.1 + 0.2 = 0.3)",
			agg:   pipe.AggregateSum,
			exprs: []string{`decimal("0.1")`, `decimal("0.2")`},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("0.3").Equal(d), "got %s", d)
			},
		},
		{
			name:  "mixed int + decimal sum promotes to decimal",
			agg:   pipe.AggregateSum,
			exprs: []string{"10", `decimal("0.5")`},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("10.5").Equal(d), "got %s", d)
			},
		},
		{
			name:  "float sum",
			agg:   pipe.AggregateSum,
			exprs: []string{"1.5", "2.25"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 3.75, got, 1e-9)
			},
		},
		{
			// A bare int alongside a float promotes to float64 (no decimal
			// present), exercising the int-to-float64 conversion branch.
			name:  "mixed int + float sum promotes to float64",
			agg:   pipe.AggregateSum,
			exprs: []string{"3", "1.25"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 4.25, got, 1e-9)
			},
		},
		{
			// A bare float alongside a decimal promotes to decimal (widest
			// kind wins), exercising the float64-to-decimal conversion branch.
			name:  "mixed float + decimal sum promotes to decimal",
			agg:   pipe.AggregateSum,
			exprs: []string{"1.25", `decimal("0.25")`},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("1.5").Equal(d), "got %s", d)
			},
		},
		{
			// A Go uint seed value alongside a float promotes to float64,
			// exercising the uint-to-float64 conversion branch.
			name:  "mixed uint + float sum promotes to float64",
			agg:   pipe.AggregateSum,
			exprs: []string{"u", "1.25"},
			seed:  map[string]any{"u": uint(2)},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 3.25, got, 1e-9)
			},
		},
		{
			// A Go uint seed value alongside a decimal promotes to decimal,
			// exercising the uint-to-decimal conversion branch.
			name:  "mixed uint + decimal sum promotes to decimal",
			agg:   pipe.AggregateSum,
			exprs: []string{"u", `decimal("0.5")`},
			seed:  map[string]any{"u": uint(3)},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("3.5").Equal(d), "got %s", d)
			},
		},
		{
			name:  "decimal min returns the exact matched element",
			agg:   pipe.AggregateMin,
			exprs: []string{`decimal("2.5")`, `decimal("1.5")`},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("1.5").Equal(d), "got %s", d)
			},
		},
		{
			name:  "decimal max returns the exact matched element",
			agg:   pipe.AggregateMax,
			exprs: []string{`decimal("1.5")`, `decimal("2.5")`},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("2.5").Equal(d), "got %s", d)
			},
		},
		{
			name:  "float max returns the exact matched element",
			agg:   pipe.AggregateMax,
			exprs: []string{"1.1", "3.3", "2.2"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.InDelta(t, 3.3, got, 1e-9)
			},
		},
		{
			name:  "large-int min returns the exact matched element",
			agg:   pipe.AggregateMin,
			exprs: []string{"9007199254740993", "5", "9007199254740992"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 5, got)
			},
		},
		{
			name:  "large-int max returns the exact matched element",
			agg:   pipe.AggregateMax,
			exprs: []string{"9007199254740992", "9007199254740993"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 9007199254740993, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rules := make([]pipe.Rule, len(tc.exprs))
			for i, e := range tc.exprs {
				rules[i] = pipe.Rule{Condition: "true", Decisions: map[string]pipe.Decision{"v": {Expr: e}}}
			}
			d, err := pipe.NewDecisionTable("agg", rules,
				pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(tc.agg))
			require.NoError(t, err)

			sc := pipe.NewScope(tc.seed)
			err = d.Execute(t.Context(), sc)
			got, _ := sc.Get("agg.v")
			tc.assert(t, got, err)
		})
	}
}

func TestDecisionTableAccessors(t *testing.T) {
	t.Parallel()

	d, err := pipe.NewDecisionTable("tier",
		[]pipe.Rule{{Condition: "true", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}}},
		pipe.WithDependsOn("amount"))
	require.NoError(t, err)
	assert.Equal(t, "tier", d.Name())
	assert.Equal(t, pipe.TypeDecisionTable, d.Type())
	assert.Equal(t, []string{"amount"}, d.DependsOn())
}
