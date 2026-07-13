package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// Example_nestedForEach adjudicates a nested collection: each order carries tax
// lines, and each tax line is banded by a decision table. Nested foreach keeps
// every inner element's firing under the composite key
// "<outer>[i].<inner>[j].<table>", so a decision stays explainable down to the
// exact line.
func Example_nestedForEach() {
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
		panic(err)
	}
	pipeline, err := def.Build()
	if err != nil {
		panic(err)
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
		panic(err)
	}

	// Per-(order, line) firing under the nested composite key.
	for _, key := range []string{
		"orders[0].taxes[0].vat",
		"orders[0].taxes[1].vat",
		"orders[1].taxes[0].vat",
	} {
		fmt.Printf("%s -> %s\n", key, sc.FiringRulesFor(key)[0].RuleID)
	}

	// Output:
	// orders[0].taxes[0].vat -> VAT_RED
	// orders[0].taxes[1].vat -> VAT_STD
	// orders[1].taxes[0].vat -> VAT_STD
}
