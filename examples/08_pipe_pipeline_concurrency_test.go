// 08 — pipe Pipeline: orders a set of Stages into a dependency DAG
// (WithDependsOn) via topological sort, validating the graph once at
// construction (duplicate names, unknown dependencies, cycles) so Run only
// evaluates. Sequential execution is the default; WithConcurrency /
// WithMaxParallel opt into running each dependency level's independent
// stages in parallel without changing the result.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// Example_pipelineDependencyOrder shows NewPipeline computing execution order
// from WithDependsOn, NOT the order stages are declared in the slice — the
// DAG, not the literal listing, decides what runs first. "total" is declared
// FIRST but depends (transitively, through "tax") on "subtotal", so the
// pipeline still runs subtotal -> tax -> total. Note "total"'s expression
// also reads "subtotal" directly yet declares no direct edge to it — that's
// safe because total->tax->subtotal is already a COMPLETE transitive
// dependency path, unlike the undeclared-read-with-no-covering-edge
// anti-pattern the concurrency example below warns against. A stage set with
// a mutual dependency (a cycle) is rejected AT CONSTRUCTION as a *CycleError,
// which carries the concrete loop path (via errors.As) so the offending
// stages are identifiable without stepping through a debugger.
func Example_pipelineDependencyOrder() {
	total, _ := pipe.NewSingleExpr("total", "subtotal + tax", pipe.WithDependsOn("tax"))
	tax, _ := pipe.NewSingleExpr("tax", "subtotal * 0.08", pipe.WithDependsOn("subtotal"))
	subtotal, _ := pipe.NewSingleExpr("subtotal", "unitPrice * qty")

	pipeline, err := pipe.NewPipeline([]pipe.Stage{total, tax, subtotal})
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	sc := pipe.NewScope(map[string]any{"unitPrice": 20, "qty": 3})
	_ = pipeline.Run(context.Background(), sc)
	t, _ := sc.Get("total")
	fmt.Println("total (computed despite declaration order):", t)

	// A mutual dependency — a -> b -> a — is a config bug caught up front,
	// before any stage ever runs.
	a, _ := pipe.NewSingleExpr("a", "b + 1", pipe.WithDependsOn("b"))
	b, _ := pipe.NewSingleExpr("b", "a + 1", pipe.WithDependsOn("a"))
	_, err = pipe.NewPipeline([]pipe.Stage{a, b})
	var cycleErr *pipe.CycleError
	if errors.As(err, &cycleErr) {
		fmt.Println("cycle detected:", cycleErr.Cycle)
	}

	// Output:
	// total (computed despite declaration order): 64.8
	// cycle detected: [a b a]
}

// Example_pipelineConcurrencyDeterminism shows WithConcurrency /
// WithMaxParallel running independent stages of the same dependency level in
// parallel while keeping the output identical to sequential execution — the
// pipeline's core promise (ADR-0052) is that turning concurrency on is purely
// a performance knob, never a behavior change, PROVIDED every cross-stage read
// is declared via WithDependsOn (three risk checks here read only seed data,
// so they are safely independent). A WithMaxParallel bound below 1 is a config
// mistake rejected at construction as *InvalidMaxParallelError, never silently
// clamped to 1 or treated as "unbounded".
func Example_pipelineConcurrencyDeterminism() {
	creditCheck, _ := pipe.NewSingleExpr("creditCheck", `score >= 680 ? "pass" : "fail"`)
	incomeCheck, _ := pipe.NewSingleExpr("incomeCheck", `income >= 50000 ? "pass" : "fail"`)
	fraudCheck, _ := pipe.NewSingleExpr("fraudCheck", `!flagged ? "pass" : "fail"`)
	stages := []pipe.Stage{creditCheck, incomeCheck, fraudCheck} // no dependencies among them
	seed := map[string]any{"score": 700, "income": 62000, "flagged": false}

	sequential, _ := pipe.NewPipeline(stages)
	seqScope := pipe.NewScope(seed)
	_ = sequential.Run(context.Background(), seqScope)

	unbounded, _ := pipe.NewPipeline(stages, pipe.WithConcurrency())
	concScope := pipe.NewScope(seed)
	_ = unbounded.Run(context.Background(), concScope)

	bounded, _ := pipe.NewPipeline(stages, pipe.WithMaxParallel(2))
	boundScope := pipe.NewScope(seed)
	_ = bounded.Run(context.Background(), boundScope)

	checksAgree := func(a, b *pipe.Scope) bool {
		for _, path := range []string{"creditCheck", "incomeCheck", "fraudCheck"} {
			av, _ := a.Get(path)
			bv, _ := b.Get(path)
			if av != bv {
				return false
			}
		}
		return true
	}
	fmt.Println("sequential vs. unbounded concurrent agree:", checksAgree(seqScope, concScope))
	fmt.Println("sequential vs. max-parallel(2) agree:", checksAgree(seqScope, boundScope))

	credit, _ := seqScope.Get("creditCheck")
	income, _ := seqScope.Get("incomeCheck")
	fraud, _ := seqScope.Get("fraudCheck")
	fmt.Println("checks:", credit, income, fraud)

	_, err := pipe.NewPipeline(stages, pipe.WithMaxParallel(0))
	var invalidErr *pipe.InvalidMaxParallelError
	fmt.Println("WithMaxParallel(0) rejected:", errors.As(err, &invalidErr))

	// Output:
	// sequential vs. unbounded concurrent agree: true
	// sequential vs. max-parallel(2) agree: true
	// checks: pass pass pass
	// WithMaxParallel(0) rejected: true
}
