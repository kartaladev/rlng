package pipe_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

func ExampleSingleExpr() {
	s, err := pipe.NewSingleExpr("total", "price * qty")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 3})
	if err := s.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	total, _ := sc.Get("total")
	fmt.Println(total)
	// Output: 30
}
