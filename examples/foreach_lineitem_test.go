package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
)

// Example_foreachLineItemAdjudication adjudicates loan collateral line items
// with a foreach stage (spec 015 / ADR-0040): a decision table runs per
// collateral line ("item"), denying any line whose LTV exceeds 80%; approved
// lines carry their amount forward through a small per-element expression
// gated on the decision table's own outcome, so the collection can be rolled
// up into an exact-decimal header total. A rollup key must resolve to a
// top-level field in the per-element result (the same shape the config
// package's own foreach tests use), so the "approved" amount is written by
// a companion single-expr stage rather than nested inside the decision
// table's own decisions — the decision table alone still owns the deny/
// approve call and its audit trail.
//
// This proves all four foreach capabilities end to end:
//   - iteration + an inner decision-table per element (LTV_MAX_80)
//   - structured per-element output ("lines.items")
//   - an exact-decimal roll-up of only the approved lines' amounts
//   - per-element explainability: which rule denied a specific line, via
//     FiringRulesFor("lines[i]")
func Example_foreachLineItemAdjudication() {
	const rules = `
stages:
  - name: lines
    type: foreach
    collection: collateral
    as: item
    stages:
      - name: check
        type: decision-table
        rules:
          - id: LTV_MAX_80
            message: LTV exceeds 80%
            condition: item.ltv > 80
            decisions:
              status: '"denied"'
        default:
          status: '"approved"'
      - name: approved
        type: single-expr
        condition: check.status == "approved"
        expr: item.amount
        depends_on: [check]
    rollups:
      - key: approved
        agg: sum
        as: totalApproved
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(rules))
	if err != nil {
		panic(err)
	}
	pipeline, err := def.Build()
	if err != nil {
		panic(err)
	}

	// Four collateral lines; only line 3 (index 2) exceeds the 80% LTV cap.
	sc := pipe.NewScope(map[string]any{
		"collateral": []any{
			map[string]any{"amount": decimal.RequireFromString("125000.00"), "ltv": 65},
			map[string]any{"amount": decimal.RequireFromString("95000.50"), "ltv": 78},
			map[string]any{"amount": decimal.RequireFromString("60000.00"), "ltv": 85},
			map[string]any{"amount": decimal.RequireFromString("40000.25"), "ltv": 50},
		},
	})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		panic(err)
	}

	// Structured per-element output: each line's decision-table status.
	items, err := sc.GetSlice("lines.items")
	if err != nil {
		panic(err)
	}
	for i, raw := range items {
		el, ok := raw.(map[string]any)
		if !ok {
			panic(fmt.Sprintf("line %d: unexpected element type %T", i, raw))
		}
		check, ok := el["check"].(map[string]any)
		if !ok {
			panic(fmt.Sprintf("line %d: missing check result", i))
		}
		fmt.Printf("line %d: status=%v\n", i+1, check["status"])
	}

	// Roll-up: exact-decimal sum of only the approved lines' amounts.
	total, err := pipe.GetAs[decimal.Decimal](sc, "lines.totalApproved")
	if err != nil {
		panic(err)
	}
	fmt.Printf("total approved: %s\n", total.StringFixed(2))

	// Per-element explainability: which rule denied line 3.
	firings := sc.FiringRulesFor("lines[2]")
	if len(firings) != 1 {
		panic(fmt.Sprintf("expected exactly one firing for line 3, got %d", len(firings)))
	}
	fmt.Printf("line 3 denied by rule: %s (%s)\n", firings[0].RuleID, firings[0].Message)

	// Output:
	// line 1: status=approved
	// line 2: status=approved
	// line 3: status=denied
	// line 4: status=approved
	// total approved: 260000.75
	// line 3 denied by rule: LTV_MAX_80 (LTV exceeds 80%)
}
