package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildHydratesDecimalLiterals drives config decimal constants entirely
// through the public Build path: a {"$dec": "..."} object form in
// PipelineDef.Constants (and in a per-expression ExprDef.Globals map) must be
// replaced in place with a decimal.Decimal before compilation, a malformed
// literal must fail Build with a *ConfigError wrapping ErrDecimalLiteral, and
// a {"$dec": ...} map carrying any OTHER key must be left untouched (it is
// not a literal).
func TestBuildHydratesDecimalLiterals(t *testing.T) {
	t.Parallel()

	trivialStage := config.StageDef{Name: "noop", Type: "single-expr", Expr: &config.ExprDef{Expr: "1"}}

	type testCase struct {
		name   string
		def    *config.PipelineDef
		assert func(t *testing.T, def *config.PipelineDef, err error)
	}

	cases := []testCase{
		{
			name: "valid $dec literal hydrates the pipeline constant and Build succeeds",
			def: &config.PipelineDef{
				Constants: map[string]any{"rate": map[string]any{"$dec": "0.0725"}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.NoError(t, err)
				d, ok := def.Constants["rate"].(decimal.Decimal)
				require.True(t, ok, "constant must be hydrated to decimal.Decimal, got %T", def.Constants["rate"])
				assert.True(t, decimal.RequireFromString("0.0725").Equal(d))
			},
		},
		{
			name: "nested $dec literal inside a constants map is hydrated",
			def: &config.PipelineDef{
				Constants: map[string]any{"tiers": map[string]any{"low": map[string]any{"$dec": "1.5"}}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.NoError(t, err)
				tiers, ok := def.Constants["tiers"].(map[string]any)
				require.True(t, ok)
				d, ok := tiers["low"].(decimal.Decimal)
				require.True(t, ok, "nested constant must be hydrated to decimal.Decimal, got %T", tiers["low"])
				assert.True(t, decimal.RequireFromString("1.5").Equal(d))
			},
		},
		{
			name: "bad $dec literal in constants fails Build with ErrDecimalLiteral attributed to constants",
			def: &config.PipelineDef{
				Constants: map[string]any{"rate": map[string]any{"$dec": "nope"}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "constants", ce.Field)
			},
		},
		{
			name: "a map with $dec plus another key is not a literal and is left untouched",
			def: &config.PipelineDef{
				Constants: map[string]any{"x": map[string]any{"$dec": "1", "other": "y"}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.NoError(t, err)
				x, ok := def.Constants["x"].(map[string]any)
				require.True(t, ok, "non-literal map must be left as map[string]any, got %T", def.Constants["x"])
				assert.Equal(t, "1", x["$dec"])
				assert.Equal(t, "y", x["other"])
			},
		},
		{
			name: "valid $dec literal in a per-expression global hydrates and Build succeeds",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "single-expr", Expr: &config.ExprDef{
						Expr:    "1",
						Globals: map[string]any{"threshold": map[string]any{"$dec": "2.5"}},
					}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.NoError(t, err)
				d, ok := def.Stages[0].Expr.Globals["threshold"].(decimal.Decimal)
				require.True(t, ok, "per-expression global must be hydrated to decimal.Decimal, got %T", def.Stages[0].Expr.Globals["threshold"])
				assert.True(t, decimal.RequireFromString("2.5").Equal(d))
			},
		},
		{
			name: "bad $dec literal in a per-expression global fails Build attributed to the expr field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "single-expr", Expr: &config.ExprDef{
						Expr:    "1",
						Globals: map[string]any{"threshold": map[string]any{"$dec": "nope"}},
					}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "expr.globals", ce.Field)
			},
		},
		{
			name: "bad $dec literal in a condition global fails Build attributed to the condition field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "single-expr", Expr: &config.ExprDef{Expr: "1"},
						Condition: &config.ExprDef{Expr: "true", Globals: map[string]any{"x": map[string]any{"$dec": "nope"}}}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "condition.globals", ce.Field)
			},
		},
		{
			name: "bad $dec literal in a multi-expr entry global fails Build attributed to exprs[i].expr field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "multi-expr", Exprs: []config.NamedExprDef{
						{Name: "a", Expr: config.ExprDef{Expr: "1", Globals: map[string]any{"x": map[string]any{"$dec": "nope"}}}},
					}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "exprs[0].expr.globals", ce.Field)
			},
		},
		{
			name: "bad $dec literal in a decision-table rule condition global fails Build attributed to rules[i].condition field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "decision-table", Rules: []config.RuleDef{
						{
							Condition: config.ExprDef{Expr: "true", Globals: map[string]any{"x": map[string]any{"$dec": "nope"}}},
							Decisions: map[string]config.ExprDef{"k": {Expr: "1"}},
						},
					}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "rules[0].condition.globals", ce.Field)
			},
		},
		{
			name: "bad $dec literal in a decision-table rule decision global fails Build attributed to rules[i].decisions.key field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "decision-table", Rules: []config.RuleDef{
						{
							Condition: config.ExprDef{Expr: "true"},
							Decisions: map[string]config.ExprDef{"k": {Expr: "1", Globals: map[string]any{"x": map[string]any{"$dec": "nope"}}}},
						},
					}},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "rules[0].decisions.k.globals", ce.Field)
			},
		},
		{
			name: "bad $dec literal in a decision-table default global fails Build attributed to default.key field",
			def: &config.PipelineDef{
				Stages: []config.StageDef{
					{Name: "s", Type: "decision-table",
						Rules: []config.RuleDef{
							{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"k": {Expr: "1"}}},
						},
						Default: map[string]config.ExprDef{"k": {Expr: "1", Globals: map[string]any{"x": map[string]any{"$dec": "nope"}}}},
					},
				},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "default.k.globals", ce.Field)
			},
		},
		{
			name: "$dec literal inside a list within constants is hydrated",
			def: &config.PipelineDef{
				Constants: map[string]any{"list": []any{map[string]any{"$dec": "2.5"}, "keep-me"}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.NoError(t, err)
				list, ok := def.Constants["list"].([]any)
				require.True(t, ok)
				require.Len(t, list, 2)
				d, ok := list[0].(decimal.Decimal)
				require.True(t, ok, "list element must be hydrated to decimal.Decimal, got %T", list[0])
				assert.True(t, decimal.RequireFromString("2.5").Equal(d))
				assert.Equal(t, "keep-me", list[1])
			},
		},
		{
			name: "bad $dec literal inside a list within constants fails Build",
			def: &config.PipelineDef{
				Constants: map[string]any{"list": []any{map[string]any{"$dec": "nope"}}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "constants", ce.Field)
			},
		},
		{
			name: "bad $dec literal doubly nested inside constants fails Build",
			def: &config.PipelineDef{
				Constants: map[string]any{"a": map[string]any{"b": map[string]any{"$dec": "nope"}}},
				Stages:    []config.StageDef{trivialStage},
			},
			assert: func(t *testing.T, def *config.PipelineDef, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "constants", ce.Field)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.def.Build()
			tc.assert(t, tc.def, err)
		})
	}
}

// TestBuildDecimalConstantUsableInExpression drives a hydrated {"$dec": "…"}
// pipeline constant end-to-end: not just that Build hydrates it (covered
// above), but that the compiled stage expression can actually reference the
// constant by bare identifier and resolve it to the decimal value at
// evaluation time — exercising expr's variable-patcher decimal branch through
// the public config.Build path.
func TestBuildDecimalConstantUsableInExpression(t *testing.T) {
	t.Parallel()

	def := &config.PipelineDef{
		Constants: map[string]any{"rate": map[string]any{"$dec": "0.0725"}},
		Stages: []config.StageDef{
			{Name: "s", Type: "single-expr", Expr: &config.ExprDef{Expr: "rate"}},
		},
	}
	p, err := def.Build()
	require.NoError(t, err)

	sc := pipe.NewScope(map[string]any{})
	require.NoError(t, p.Run(t.Context(), sc))

	v, ok := sc.Get("s")
	require.True(t, ok)
	d, ok := v.(decimal.Decimal)
	require.True(t, ok, "expected decimal.Decimal, got %T", v)
	assert.True(t, decimal.RequireFromString("0.0725").Equal(d))
}
