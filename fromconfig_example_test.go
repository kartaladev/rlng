package rlng_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
)

// ExampleNewFromYAML builds an engine from a YAML ruleset in a single call and
// evaluates it.
func ExampleNewFromYAML() {
	const ruleset = `
stages:
  - name: total
    type: single-expr
    expr: input.qty * input.price
`
	eng, err := rlng.NewFromYAML(context.Background(), ruleset)
	if err != nil {
		panic(err)
	}
	out, err := eng.Evaluate(context.Background(), map[string]any{
		"input": map[string]any{"qty": 3, "price": 4},
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("total = %v\n", out["total"])

	// Output:
	// total = 12
}
