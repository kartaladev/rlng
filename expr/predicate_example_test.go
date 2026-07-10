package expr_test

import (
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

func ExamplePredicate() {
	p, err := expr.NewPredicate("amount > threshold",
		expr.WithGlobals(map[string]any{"threshold": 100}))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	ok, _ := p.Test(map[string]any{"amount": 150})
	fmt.Println(ok)
	// Output: true
}
