package stage_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleDecisionTable() {
	d, err := stage.NewDecisionTable("tier", []stage.Rule{
		{Condition: "amount >= 1000", Decisions: map[string]string{"level": `"gold"`}},
		{Condition: "amount >= 100", Decisions: map[string]string{"level": `"silver"`}},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"amount": 5000})
	if err := d.Execute(context.TODO(), sc); err != nil {
		fmt.Println("error:", err)
		return
	}

	level, _ := sc.Get("tier.level")
	fmt.Println(level)
	// Output: gold
}
