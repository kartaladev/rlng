package pipe_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// ExampleScope_firingRulesFor shows the explainability trail for a
// HitPolicyCollect decision table: every matching rule is recorded, not just
// the first, so an adverse-action decision ("denied for reasons A, B") can
// name each contributing rule.
func ExampleScope_firingRulesFor() {
	d, err := pipe.NewDecisionTable("denial", []pipe.Rule{
		{
			ID:        "CREDIT_MIN_650",
			Message:   "credit score below minimum",
			Condition: "score < 650",
			Decisions: map[string]pipe.Decision{"reason": {Expr: `"credit score below minimum"`}},
		},
		{
			ID:        "DEBT_RATIO_MAX",
			Message:   "debt-to-income ratio too high",
			Condition: "debt_ratio > 0.4",
			Decisions: map[string]pipe.Decision{"reason": {Expr: `"debt-to-income ratio too high"`}},
		},
	}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"score": 580, "debt_ratio": 0.5})
	if err := d.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	for _, fr := range sc.FiringRulesFor("denial") {
		fmt.Printf("%s: %s\n", fr.RuleID, fr.Message)
	}
	// Output:
	// CREDIT_MIN_650: credit score below minimum
	// DEBT_RATIO_MAX: debt-to-income ratio too high
}
