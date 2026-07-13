package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildForEach drives config.Parse(...).Build() (the public API) for
// the foreach stage type end-to-end: valid configs are parsed, built, and run
// against a pipe.Scope to verify the resulting Scope state; invalid configs
// are checked for the specific error each must produce (strict decode error,
// inner-stage build error, nested-foreach guard).
func TestBuildForEach(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, d *config.PipelineDef, buildErr error)
	}

	cases := []testCase{
		{
			name: "valid foreach with inner single-expr rolls up a decimal sum end-to-end",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    as: line
    output: results
    stages:
      - name: amt
        type: single-expr
        expr: line.price
    rollups:
      - key: amt
        agg: sum
        as: total
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				p, err := d.Build()
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"price": decimal.RequireFromString("10.5")},
						map[string]any{"price": decimal.RequireFromString("5.25")},
					},
				})
				require.NoError(t, p.Run(t.Context(), sc))

				results, ok := sc.Get("lines.results")
				require.True(t, ok)
				items, ok := results.([]any)
				require.True(t, ok)
				require.Len(t, items, 2)
				el0, ok := items[0].(map[string]any)
				require.True(t, ok)
				assert.True(t, decimal.RequireFromString("10.5").Equal(el0["amt"].(decimal.Decimal)))

				total, ok := sc.Get("lines.total")
				require.True(t, ok)
				d0, ok := total.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", total)
				assert.True(t, decimal.RequireFromString("15.75").Equal(d0), "got %s", d0)
			},
		},
		{
			name: "valid foreach with inner decision-table fires per element",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: check
        type: decision-table
        rules:
          - id: HIGH
            condition: item.ltv > 80
            decisions:
              flag: '"high"'
          - id: LOW
            condition: item.ltv < 50
            decisions:
              flag: '"low"'
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				p, err := d.Build()
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"ltv": int64(90)},
						map[string]any{"ltv": int64(30)},
					},
				})
				require.NoError(t, p.Run(t.Context(), sc))

				got, ok := sc.Get("lines.items")
				require.True(t, ok)
				items, ok := got.([]any)
				require.True(t, ok)
				require.Len(t, items, 2)

				f0 := sc.FiringRulesFor("lines[0].check")
				require.Len(t, f0, 1)
				assert.Equal(t, "HIGH", f0[0].RuleID)

				f1 := sc.FiringRulesFor("lines[1].check")
				require.Len(t, f1, 1)
				assert.Equal(t, "LOW", f1[0].RuleID)
			},
		},
		{
			name: "unknown field on the foreach stage itself is a strict-decode error",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    bogus: true
    stages:
      - name: amt
        type: single-expr
        expr: line.price
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.Error(t, buildErr)
				var ce *config.ConfigError
				assert.ErrorAs(t, buildErr, &ce)
			},
		},
		{
			name: "unknown field on an inner stage is a strict-decode error",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: amt
        type: single-expr
        expr: line.price
        bogus: true
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.Error(t, buildErr)
				var ce *config.ConfigError
				assert.ErrorAs(t, buildErr, &ce)
			},
		},
		{
			name: "invalid inner stage expr is a build-time ConfigError naming the inner stage",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: broken
        type: single-expr
        expr: "line.price +"
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "broken", ce.Stage)
			},
		},
		{
			name: "foreach with no inner stages is a build-time ConfigError",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "lines", ce.Stage)
				assert.Equal(t, "stages", ce.Field)
			},
		},
		{
			name: "a rollup missing its as is a build-time error (fail loud, not a runtime path error)",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: amt
        type: single-expr
        expr: item.price
    rollups:
      - key: amt
        agg: sum
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.ErrorIs(t, err, pipe.ErrForEachEmptyRollup)
			},
		},
		{
			name: "duplicate inner stage names surface the pipeline construction error",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: amt
        type: single-expr
        expr: line.price
      - name: amt
        type: single-expr
        expr: line.qty
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "lines", ce.Stage)

				var dupErr *pipe.DuplicateStageError
				assert.ErrorAs(t, err, &dupErr)
			},
		},
		{
			name: "empty collection path surfaces the stage construction error",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: ""
    stages:
      - name: amt
        type: single-expr
        expr: line.price
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)

				var se *pipe.StageError
				assert.ErrorAs(t, err, &se)
			},
		},
		{
			name: "a malformed $dec literal in an inner stage's expr globals hydrates recursively and errors",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: amt
        type: single-expr
        expr:
          expr: item.price + rate
          globals:
            rate:
              $dec: "not-a-number"
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrDecimalLiteral)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "amt", ce.Stage)
			},
		},
		{
			name: "a valid $dec literal in an inner stage's expr globals hydrates recursively end-to-end",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: items
    stages:
      - name: amt
        type: single-expr
        expr:
          expr: rate
          globals:
            rate:
              $dec: "1.5"
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				p, err := d.Build()
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"price": decimal.RequireFromString("2.0")},
					},
				})
				require.NoError(t, p.Run(t.Context(), sc))

				results, ok := sc.Get("lines.items")
				require.True(t, ok)
				items, ok := results.([]any)
				require.True(t, ok)
				require.Len(t, items, 1)
				el0, ok := items[0].(map[string]any)
				require.True(t, ok)
				got, ok := el0["amt"].(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", el0["amt"])
				assert.True(t, decimal.RequireFromString("1.5").Equal(got), "got %s (hydrated $dec global was not applied to the inner stage)", got)
			},
		},
		{
			name: "nested foreach builds, runs, and fires per inner element",
			yaml: `
stages:
  - name: lines
    type: foreach
    collection: orders
    as: line
    stages:
      - name: taxes
        type: foreach
        collection: line.taxes
        as: tax
        stages:
          - name: vat
            type: decision-table
            rules:
              - id: VAT_STD
                condition: tax.rate >= 10
                decisions:
                  band: '"standard"'
              - id: VAT_RED
                condition: tax.rate < 10
                decisions:
                  band: '"reduced"'
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				p, err := d.Build()
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"orders": []any{
						map[string]any{"taxes": []any{
							map[string]any{"rate": int64(5)},
							map[string]any{"rate": int64(20)},
						}},
					},
				})
				require.NoError(t, p.Run(t.Context(), sc))

				assert.Equal(t, "VAT_RED", sc.FiringRulesFor("lines[0].taxes[0].vat")[0].RuleID)
				assert.Equal(t, "VAT_STD", sc.FiringRulesFor("lines[0].taxes[1].vat")[0].RuleID)
			},
		},
		{
			name: "nested foreach reusing `as` down the chain is rejected naming both stages",
			yaml: `
stages:
  - name: outer
    type: foreach
    collection: items
    stages:
      - name: inner
        type: foreach
        collection: item.sub
        stages:
          - name: amt
            type: single-expr
            expr: item.price
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrForEachAsCollision)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "inner", ce.Stage)
				assert.Equal(t, "as", ce.Field)
				assert.Contains(t, err.Error(), "outer") // enclosing stage named
				assert.Contains(t, err.Error(), "inner") // colliding stage named
			},
		},
		{
			name: "sibling foreach stages may reuse the same `as` (independent chains)",
			yaml: `
stages:
  - name: a
    type: foreach
    collection: xs
    stages:
      - name: ax
        type: single-expr
        expr: item.v
  - name: b
    type: foreach
    collection: ys
    stages:
      - name: bx
        type: single-expr
        expr: item.v
`,
			assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
				require.NoError(t, buildErr)
				_, err := d.Build()
				require.NoError(t, err, "siblings default `as: item` on independent chains must build")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := config.Parse(t.Context(), config.FromYAMLString(tc.yaml))
			tc.assert(t, d, err)
		})
	}
}

// TestBuildForEachRollupAggregationError verifies an unknown roll-up
// aggregation name surfaces as a *config.ConfigError naming the field, using
// errors.Is-style wrapping the same way parseAggregation is exercised for
// decision-table aggregation.
func TestBuildForEachRollupAggregationError(t *testing.T) {
	t.Parallel()

	d := config.PipelineDef{
		Stages: []config.StageDef{
			{
				Name: "lines", Type: "foreach", Collection: "items",
				Stages: []config.StageDef{
					{Name: "amt", Type: "single-expr", Expr: &config.ExprDef{Expr: "line.price"}},
				},
				Rollups: []config.RollupDef{{Key: "amt", Agg: "median", As: "total"}},
			},
		},
	}

	_, err := d.Build()
	require.Error(t, err)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "lines", ce.Stage)
	assert.Equal(t, "rollups", ce.Field)
}
