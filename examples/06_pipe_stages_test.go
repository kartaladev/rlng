// 06 — pipe stages: SingleExpr (one gated value expression) and MultiExpr
// (several named expressions evaluated in ascending priority order, each
// visible to later ones in the same stage by its bare name). Both compile
// their expressions at construction; Execute only evaluates against a Scope
// snapshot.
package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// Example_singleExprConditionGate shows WithCondition gating a SingleExpr: the
// value expression only runs — and only writes an output — when the condition
// tests true. The quirk that trips up newcomers: a false condition is a NO-OP,
// not "write false" or "write zero" — the output path is simply absent, so a
// later stage reading it must treat "missing" as "did not apply" (expr's
// lenient env handling reads a missing path as nil). WithOutput retargets the
// write from the stage's default output path (its own name) to an arbitrary
// Scope path — here, nesting it under "pricing".
func Example_singleExprConditionGate() {
	loyaltyDiscount, err := pipe.NewSingleExpr("loyaltyDiscount", "0.10",
		pipe.WithCondition(`tier == "gold"`),
		pipe.WithOutput("pricing.discountRate"),
	)
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{loyaltyDiscount})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	gold := pipe.NewScope(map[string]any{"tier": "gold"})
	_ = pipeline.Run(context.Background(), gold)
	rate, ok := gold.Get("pricing.discountRate")
	fmt.Println("gold tier: discount written =", ok, "rate =", rate)

	silver := pipe.NewScope(map[string]any{"tier": "silver"})
	_ = pipeline.Run(context.Background(), silver)
	_, ok = silver.Get("pricing.discountRate")
	fmt.Println("silver tier: discount written =", ok)

	// Output:
	// gold tier: discount written = true rate = 0.1
	// silver tier: discount written = false
}

// Example_multiExprPriorityAndAliasing shows a MultiExpr stage evaluating
// several named expressions in ascending Priority order — NOT the order they
// appear in the []NamedExpr slice — and each result becoming visible to LATER
// expressions in the same stage under its bare name (the intra-stage alias),
// distinct from the qualified "stage.name" Scope path it is persisted under
// once the stage finishes. "tax" is listed first but has the higher priority
// number, so it runs after "subtotal" and can reference it by the bare name
// "subtotal" — exactly as if subtotal were already in the environment.
func Example_multiExprPriorityAndAliasing() {
	order, err := pipe.NewMultiExpr("order", []pipe.NamedExpr{
		{Name: "tax", Expression: "subtotal * 0.08", Priority: 2},      // listed first, runs second
		{Name: "subtotal", Expression: "unitPrice * qty", Priority: 1}, // listed second, runs first
		{Name: "total", Expression: "subtotal + tax", Priority: 3},
	})
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{order})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"unitPrice": 25, "qty": 4})
	_ = pipeline.Run(context.Background(), sc)

	subtotal, _ := sc.Get("order.subtotal")
	tax, _ := sc.Get("order.tax")
	total, _ := sc.Get("order.total")
	fmt.Println("subtotal:", subtotal)
	fmt.Println("tax:", tax)
	fmt.Println("total:", total)

	// Output:
	// subtotal: 100
	// tax: 8
	// total: 108
}
