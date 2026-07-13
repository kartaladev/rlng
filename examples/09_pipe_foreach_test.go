// 09 — pipe per-element iteration. ForEach runs an inner *Pipeline once per
// element of a Scope collection, against a fresh per-element Scope, and
// collects each element's result back as an ordered list; Rollup then reduces
// a per-element output key across every element into a single header value.
// Nesting composes (a foreach's inner stages may include another foreach),
// and every inner stage's firing/derivation is surfaced under a composite key
// ("<outer>[i].<inner>", or "<outer>[i].<middle>[j].<inner>" when nested) so a
// decision stays explainable down to the exact element that produced it.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
)

// Example_foreachLineItemAdjudication adjudicates loan collateral line items:
// an inner decision table runs per collateral line ("item"), denying any line
// whose LTV exceeds 80%; a companion single-expr stage carries an approved
// line's amount forward, gated on the table's own outcome, so the collection
// rolls up into an exact-decimal header total (Rollup.Key "approved" reaches a
// TOP-LEVEL field of the per-element result — a decision-table's own output
// would need a dotted key like "check.status", since it is namespaced under
// the table's own stage name). Per-element explainability comes free:
// sc.FiringRulesFor("lines[2].check") names exactly which rule denied line 3,
// the composite key "<foreach>[i].<inner stage>" giving every element its own
// audit trail without disturbing the inner stage's own name.
//
// The quirk: rolling up an EMPTY collection is not "zero" across the board.
// AggregateCount has an obvious empty answer (0) and AggregateList has an
// obvious empty answer ([]), so both are written. AggregateSum/Min/Max do NOT
// — folding an empty collection has no defined sum/min/max — so the rollup
// key is left ABSENT entirely (Get reports ok=false), not written as 0. A
// consumer that assumes a numeric rollup is always present will panic or
// silently read a stale value; check presence explicitly.
func Example_foreachLineItemAdjudication() {
	const ruleset = `
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
      - key: check.status
        agg: count
        as: lineCount
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(ruleset))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
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
		fmt.Println("run:", err)
		return
	}

	items, err := sc.GetSlice("lines.items")
	if err != nil {
		fmt.Println("get items:", err)
		return
	}
	for i, raw := range items {
		el := raw.(map[string]any)            //nolint:errcheck // foreach always writes map[string]any elements
		check := el["check"].(map[string]any) //nolint:errcheck // decision-table always writes its namespaced map
		fmt.Printf("line %d: status=%v\n", i+1, check["status"])
	}

	total, err := pipe.GetAs[decimal.Decimal](sc, "lines.totalApproved")
	if err != nil {
		fmt.Println("get total:", err)
		return
	}
	fmt.Printf("total approved: %s\n", total.StringFixed(2))

	count, err := sc.GetInt("lines.lineCount")
	if err != nil {
		fmt.Println("get count:", err)
		return
	}
	fmt.Println("line count:", count)

	firings := sc.FiringRulesFor("lines[2].check")
	fmt.Printf("line 3 denied by rule: %s (%s)\n", firings[0].RuleID, firings[0].Message)

	// Empty-collection quirk: run the same pipeline over zero lines. The
	// AggregateSum rollup ("totalApproved") is left ABSENT — no defined sum
	// over nothing — while the AggregateCount rollup ("lineCount") is written
	// as 0, an explicit, present answer.
	empty := pipe.NewScope(map[string]any{"collateral": []any{}})
	if err := pipeline.Run(context.Background(), empty); err != nil {
		fmt.Println("run empty:", err)
		return
	}
	_, totalPresent := empty.Get("lines.totalApproved")
	emptyCount, _ := empty.GetInt("lines.lineCount")
	fmt.Println("empty collection: totalApproved present:", totalPresent, "lineCount:", emptyCount)

	// Output:
	// line 1: status=approved
	// line 2: status=approved
	// line 3: status=denied
	// line 4: status=approved
	// total approved: 260000.75
	// line count: 4
	// line 3 denied by rule: LTV_MAX_80 (LTV exceeds 80%)
	// empty collection: totalApproved present: false lineCount: 0
}

// Example_nestedForEachTaxLines adjudicates a nested collection: each order
// carries tax lines, and each tax line is banded by a decision table. Nesting
// composes without special-casing — an inner foreach's own inner stages are
// just another StageDef list — and the composite firing key grows one segment
// per nesting level: "orders[i].taxes[j].vat" names exactly the order and tax
// line that produced a given VAT band, not just "which table".
func Example_nestedForEachTaxLines() {
	const ruleset = `
stages:
  - name: orders
    type: foreach
    collection: cart
    as: order
    stages:
      - name: taxes
        type: foreach
        collection: order.lines
        as: line
        stages:
          - name: vat
            type: decision-table
            rules:
              - id: VAT_STD
                condition: line.rate >= 10
                decisions:
                  band: '"standard"'
              - id: VAT_RED
                condition: line.rate < 10
                decisions:
                  band: '"reduced"'
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(ruleset))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{
		"cart": []any{
			map[string]any{"lines": []any{
				map[string]any{"rate": 5},
				map[string]any{"rate": 20},
			}},
			map[string]any{"lines": []any{
				map[string]any{"rate": 12},
			}},
		},
	})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}

	// Per-(order, line) firing under the two-level composite key.
	for _, key := range []string{
		"orders[0].taxes[0].vat",
		"orders[0].taxes[1].vat",
		"orders[1].taxes[0].vat",
	} {
		fmt.Printf("%s -> %s\n", key, sc.FiringRulesFor(key)[0].RuleID)
	}

	// ErrForEachAsCollision: an inner foreach reusing the enclosing foreach's
	// `as` binding would silently shadow the outer element inside the inner
	// per-element scope (both bound under, say, "order") — build.go's
	// validateForEachAsChains rejects this at Build (config layer), not at
	// runtime, so the mistake is caught before a single rule ever evaluates.
	// Contrast the ruleset above (distinct "order" / "line") with this one,
	// where both levels reuse "order".
	const colliding = `
stages:
  - name: orders
    type: foreach
    collection: cart
    as: order
    stages:
      - name: taxes
        type: foreach
        collection: order.lines
        as: order
        stages:
          - name: vat
            type: decision-table
            rules:
              - condition: order.rate >= 10
                decisions:
                  band: '"standard"'
`
	badDef, err := config.Parse(context.Background(), config.FromYAMLString(colliding))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	_, err = badDef.Build()
	fmt.Println("colliding `as` rejected:", errors.Is(err, config.ErrForEachAsCollision))
	fmt.Println(err)

	// Output:
	// orders[0].taxes[0].vat -> VAT_RED
	// orders[0].taxes[1].vat -> VAT_STD
	// orders[1].taxes[0].vat -> VAT_STD
	// colliding `as` rejected: true
	// config: stage "taxes" field "as": foreach: `as` element binding reused by a nested foreach: inner foreach "taxes" reuses element binding "order" of enclosing foreach "orders"
}
