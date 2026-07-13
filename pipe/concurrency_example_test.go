package pipe_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// ExamplePipeline_withConcurrency runs two independent stages concurrently. The
// result is deterministic despite the parallelism: concurrency is a pure
// speedup, never observable in the output (ADR-0052).
func ExamplePipeline_withConcurrency() {
	a, _ := pipe.NewSingleExpr("a", "1 + 1")
	b, _ := pipe.NewSingleExpr("b", "10 * 2")

	p, _ := pipe.NewPipeline([]pipe.Stage{a, b}, pipe.WithConcurrency())

	sc := pipe.NewScope(nil)
	_ = p.Run(context.Background(), sc)

	fmt.Println(sc.Snapshot()["a"], sc.Snapshot()["b"])
	// Output: 2 20
}
