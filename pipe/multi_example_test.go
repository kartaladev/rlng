package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleMultiExpr() {
	m, err := stage.NewMultiExpr("calc", []stage.NamedExpr{
		{Name: "base", Expression: "price * qty", Priority: 0},
		{Name: "taxed", Expression: "base * 1.1", Priority: 1},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := m.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	taxed, _ := sc.Get("calc.taxed")
	fmt.Println(taxed)
	// Output: 22
}
