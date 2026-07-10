package expr_test

import (
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

func ExampleFunction() {
	f, err := expr.NewFunction("total", "price * qty")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	got, _ := f.Apply(map[string]any{"price": 10, "qty": 3})
	fmt.Println(got)
	// Output: 30
}
