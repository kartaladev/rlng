package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// Example_eligibility grades a loan application with a decision table (first
// match wins) and explains how the winning decision was derived, recursing
// through the decision expression back to the seed input it reads from.
func Example_eligibility() {
	grade, _ := pipe.NewDecisionTable("grade", []pipe.Rule{
		{Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`, "limit": "score * 100"}},
		{Condition: "score >= 650", Decisions: map[string]string{"tier": `"near_prime"`, "limit": "score * 50"}},
		{Condition: "true", Decisions: map[string]string{"tier": `"subprime"`, "limit": "score * 10"}},
	})
	p, _ := pipe.NewPipeline(grade)

	sc := pipe.NewScope(map[string]any{"score": 700}, pipe.WithProvenance())
	_ = p.Run(context.Background(), sc)

	tier, _ := sc.GetString("grade.tier")
	fmt.Println("tier:", tier)
	fmt.Print(sc.Explain("grade.limit"))

	// Output:
	// tier: near_prime
	// grade.limit = 35000 [grade decision-table] expr: score * 50
	//   score = 700 [seed]
}

// Example_eligibility_flags shows a collect-mode decision table: every matching
// rule contributes to a slice of risk flags.
func Example_eligibility_flags() {
	checks, _ := pipe.NewDecisionTable("checks", []pipe.Rule{
		{Condition: "score < 650", Decisions: map[string]string{"flag": `"low_score"`}},
		{Condition: "dti > 0.4", Decisions: map[string]string{"flag": `"high_dti"`}},
	}, pipe.WithHitPolicy(pipe.HitPolicyCollect))
	p, _ := pipe.NewPipeline(checks)

	sc := pipe.NewScope(map[string]any{"score": 600, "dti": 0.5})
	_ = p.Run(context.Background(), sc)

	flags, _ := sc.GetSlice("checks.flag")
	fmt.Println("flags:", flags)

	// Output:
	// flags: [low_score high_dti]
}
