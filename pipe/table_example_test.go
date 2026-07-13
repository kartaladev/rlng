package pipe_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

func ExampleDecisionTable() {
	d, err := pipe.NewDecisionTable("tier", []pipe.Rule{
		{Condition: "amount >= 1000", Decisions: map[string]pipe.Decision{"level": {Expr: `"gold"`}}},
		{Condition: "amount >= 100", Decisions: map[string]pipe.Decision{"level": {Expr: `"silver"`}}},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"amount": 5000})
	if err := d.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	level, _ := sc.Get("tier.level")
	fmt.Println(level)
	// Output: gold
}
