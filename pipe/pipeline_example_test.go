package stage_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

// ExamplePipeline shows a linear pipeline where a later stage reads an earlier
// stage's output from the shared Scope; the pipeline orders by dependency, not
// declaration.
func ExamplePipeline() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	taxed, _ := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))

	p, _ := stage.NewPipeline(taxed, base) // declared out of order; ordered by deps
	sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	v, _ := sc.Get("taxed")
	fmt.Printf("%.1f\n", v)
	// Output: 22.0
}

// ExamplePipeline_cycle shows that a dependency cycle is reported at
// construction with the concrete loop path.
func ExamplePipeline_cycle() {
	a, _ := stage.NewSingleExpr("a", "b", stage.WithDependsOn("b"))
	b, _ := stage.NewSingleExpr("b", "a", stage.WithDependsOn("a"))

	_, err := stage.NewPipeline(a, b)
	var ce *stage.CycleError
	if errors.As(err, &ce) {
		fmt.Println(err)
	}
	// Output: pipeline: dependency cycle: a -> b -> a
}
