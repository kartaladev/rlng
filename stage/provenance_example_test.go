package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

// ExampleScope_explain shows the derivation tree for a value computed by a
// two-stage pipeline, tracing "taxed" back through "base" to the seed inputs.
func ExampleScope_explain() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	taxed, _ := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	p, _ := stage.NewPipeline(base, taxed)

	sc := stage.NewScope(map[string]any{"price": 10, "qty": 2}, stage.WithProvenance())
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Print(sc.Explain("taxed"))
	// Output:
	// taxed = 22 [taxed single-expr] expr: base * 1.1
	//   base = 20 [base single-expr] expr: price * qty
	//     price = 10 [seed]
	//     qty = 2 [seed]
}

// ExampleScope_getInt shows a strict typed getter returning a stored int and
// the error-nil path when the value is present with the expected type.
func ExampleScope_getInt() {
	sc := stage.NewScope(map[string]any{"qty": 2})

	qty, err := sc.GetInt("qty")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(qty)
	// Output: 2
}
