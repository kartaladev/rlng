package pipe_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// ExamplePipeline shows a linear pipeline where a later stage reads an earlier
// stage's output from the shared Scope; the pipeline orders by dependency, not
// declaration.
func ExamplePipeline() {
	base, _ := pipe.NewSingleExpr("base", "price * qty")
	taxed, _ := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))

	p, _ := pipe.NewPipeline(taxed, base) // declared out of order; ordered by deps
	sc := pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
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
	a, _ := pipe.NewSingleExpr("a", "b", pipe.WithDependsOn("b"))
	b, _ := pipe.NewSingleExpr("b", "a", pipe.WithDependsOn("a"))

	_, err := pipe.NewPipeline(a, b)
	var ce *pipe.CycleError
	if errors.As(err, &ce) {
		fmt.Println(err)
	}
	// Output: pipeline: dependency cycle: a -> b -> a
}
