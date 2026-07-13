package pipe_test

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecisionTablePolicyEdges covers the error and provenance branches of the
// non-single hit policies and aggregations.
func TestDecisionTablePolicyEdges(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope)
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}

	// a condition that errors at eval: integer modulo by zero.
	badCond := "a % b > 0"

	cases := []testCase{
		{
			name: "unique: condition eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("u", []pipe.Rule{
					{Condition: badCond, Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyUnique))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "any: condition eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("an", []pipe.Rule{
					{Condition: badCond, Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "any: decision eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("an", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"x": {Expr: "a % b"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "default decision eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("s", []pipe.Rule{
					{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "a % b"}}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "collect min over floats yields a float",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("m", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"r": {Expr: "0.2"}}},
					{Condition: "true", Decisions: map[string]pipe.Decision{"r": {Expr: "0.05"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateMin))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("m.r")
				assert.InDelta(t, 0.05, v, 1e-9)
			},
		},
		{
			name: "collect sum over uint seed value",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("c", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"r": {Expr: "u"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{"u": uint(7)})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("c.r")
				assert.Equal(t, int64(7), v, "an all-int sum (incl. a uint operand) stays exact in int64")
			},
		},
		{
			name: "out-of-range aggregation does not panic under provenance",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("x", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"v": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.CollectAggregation(99)))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				// Unknown aggregation falls back to the list result.
				v, ok := sc.Get("x.v")
				require.True(t, ok)
				assert.Equal(t, []any{1}, v)
			},
		},
		{
			name: "provenance: aggregated collect labels the reducer",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("fees", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"fee": {Expr: "25"}}},
					{Condition: "true", Decisions: map[string]pipe.Decision{"fee": {Expr: "15"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				d, ok := sc.Derivation("fees.fee")
				require.True(t, ok)
				assert.Equal(t, "collect:sum:fee", d.Operation)
			},
		},
		{
			name: "provenance: any policy records the agreed value",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("g", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"ok": {Expr: "true"}}},
					{Condition: "true", Decisions: map[string]pipe.Decision{"ok": {Expr: "true"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("g.ok")
				assert.Equal(t, true, v)
				d, ok := sc.Derivation("g.ok")
				require.True(t, ok)
				assert.Equal(t, "any:ok", d.Operation)
			},
		},
		{
			name: "unique: no match applies default",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("u", []pipe.Rule{
					{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyUnique), pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "9"}}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("u.x")
				assert.Equal(t, 9, v)
			},
		},
		{
			name: "any: no match applies default",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("an", []pipe.Rule{
					{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny), pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "9"}}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("an.x")
				assert.Equal(t, 9, v)
			},
		},
		{
			name: "collect: no match applies default",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("c", []pipe.Rule{
					{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "9"}}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				v, _ := sc.Get("c.x")
				assert.Equal(t, 9, v)
			},
		},
		{
			name: "default decision path conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				// keys "x" (scalar) then "x.y" collide: x is written first
				// (sorted), then x.y cannot traverse a scalar.
				d, err := pipe.NewDecisionTable("s", []pipe.Rule{
					{Condition: "false", Decisions: map[string]pipe.Decision{"z": {Expr: "1"}}},
				}, pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "1"}, "x.y": {Expr: "2"}}))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "any decision path conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.DecisionTable, *pipe.Scope) {
				d, err := pipe.NewDecisionTable("an", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}, "x.y": {Expr: "2"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicyAny))
				require.NoError(t, err)
				return d, pipe.NewScope(map[string]any{})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, sc := tc.build(t)
			err := d.Execute(context.Background(), sc)
			tc.assert(t, sc, err)
		})
	}
}

// TestCollectAggregationProvenanceLabels checks each reducer names itself in the
// provenance operation label.
func TestCollectAggregationProvenanceLabels(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		agg     pipe.CollectAggregation
		wantOp  string
		wantVal any
	}

	cases := []testCase{
		{name: "sum", agg: pipe.AggregateSum, wantOp: "collect:sum:v", wantVal: int64(30)},
		{name: "min", agg: pipe.AggregateMin, wantOp: "collect:min:v", wantVal: 10},
		{name: "max", agg: pipe.AggregateMax, wantOp: "collect:max:v", wantVal: 20},
		{name: "count", agg: pipe.AggregateCount, wantOp: "collect:count:v", wantVal: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := pipe.NewDecisionTable("x", []pipe.Rule{
				{Condition: "true", Decisions: map[string]pipe.Decision{"v": {Expr: "10"}}},
				{Condition: "true", Decisions: map[string]pipe.Decision{"v": {Expr: "20"}}},
			}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(tc.agg))
			require.NoError(t, err)
			sc := pipe.NewScope(map[string]any{}, pipe.WithProvenance())
			require.NoError(t, d.Execute(context.Background(), sc))

			v, _ := sc.Get("x.v")
			assert.Equal(t, tc.wantVal, v)
			der, ok := sc.Derivation("x.v")
			require.True(t, ok)
			assert.Equal(t, tc.wantOp, der.Operation)
		})
	}
}

// TestNewDecisionTableDefaultErrors covers construction-time validation of the
// default decision set.
func TestNewDecisionTableDefaultErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() error
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "invalid default expression is a construction error",
			build: func() error {
				_, err := pipe.NewDecisionTable("t", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithDefault(map[string]pipe.Decision{"x": {Expr: "1 +"}}))
				return err
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "empty default output key is a construction error",
			build: func() error {
				_, err := pipe.NewDecisionTable("t", []pipe.Rule{
					{Condition: "true", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}},
				}, pipe.WithDefault(map[string]pipe.Decision{"": {Expr: "1"}}))
				return err
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.build())
		})
	}
}
