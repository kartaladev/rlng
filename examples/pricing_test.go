package examples_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kartaladev/rlng/stage"
)

// deterministicClock returns a clock that yields `start`, then `start+step` on
// every subsequent call — so evaluation duration is stable in example output.
func deterministicClock(start time.Time, step time.Duration) func() time.Time {
	times := []time.Time{start, start.Add(step)}
	i := 0
	return func() time.Time {
		t := times[i]
		if i < len(times)-1 {
			i++
		}
		return t
	}
}

// Example_pricing computes an order total through a single-expr stage feeding a
// multi-expr stage (two named results), reads results with typed getters, shows
// evaluation timing, and round-trips the Scope through JSON (as a jsonb column).
func Example_pricing() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	calc, _ := stage.NewMultiExpr("calc", []stage.NamedExpr{
		{Name: "taxed", Expression: "base * 1.1", Priority: 0},
		{Name: "discounted", Expression: "base * 0.9", Priority: 1},
	}, stage.WithDependsOn("base"))
	p, _ := stage.NewPipeline(base, calc)

	clock := deterministicClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 5*time.Millisecond)
	sc := stage.NewScope(map[string]any{"price": 10, "qty": 2}, stage.WithClock(clock))
	_ = p.Run(context.Background(), sc)

	taxed, _ := sc.GetFloat64("calc.taxed")
	discounted, _ := sc.GetFloat64("calc.discounted")
	dur, _ := sc.Duration()
	fmt.Printf("taxed=%.1f discounted=%.1f took=%s\n", taxed, discounted, dur)

	blob, _ := json.Marshal(sc) // persist to jsonb
	var reloaded stage.Scope
	_ = json.Unmarshal(blob, &reloaded) // read back
	back, _ := reloaded.GetFloat64("calc.taxed")
	fmt.Printf("reloaded taxed=%.1f\n", back)

	// Output:
	// taxed=22.0 discounted=18.0 took=5ms
	// reloaded taxed=22.0
}
