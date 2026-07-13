package examples_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kartaladev/rlng/pipe"
)

// deterministicClock returns a clock that yields `start`, then `start+step` on
// every subsequent call — so evaluation duration is stable in example output.
// stepClock is a deterministic pipe.Clock returning start, then start+step
// (repeating the last time), for stable example timing output.
type stepClock struct {
	times []time.Time
	i     int
}

func (c *stepClock) Now() time.Time {
	t := c.times[c.i]
	if c.i < len(c.times)-1 {
		c.i++
	}
	return t
}

func deterministicClock(start time.Time, step time.Duration) pipe.Clock {
	return &stepClock{times: []time.Time{start, start.Add(step)}}
}

// Example_pricing computes an order total through a single-expr stage feeding a
// multi-expr stage (two named results), reads results with typed getters, shows
// evaluation timing, and round-trips the Scope through JSON (as a jsonb column).
func Example_pricing() {
	base, _ := pipe.NewSingleExpr("base", "price * qty")
	calc, _ := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
		{Name: "taxed", Expression: "base * 1.1", Priority: 0},
		{Name: "discounted", Expression: "base * 0.9", Priority: 1},
	}, pipe.WithDependsOn("base"))
	p, _ := pipe.NewPipeline([]pipe.Stage{base, calc})

	clock := deterministicClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 5*time.Millisecond)
	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2}, pipe.WithClock(clock))
	_ = p.Run(context.Background(), sc)

	taxed, _ := sc.GetFloat64("calc.taxed")
	discounted, _ := sc.GetFloat64("calc.discounted")
	dur, _ := sc.Duration()
	fmt.Printf("taxed=%.1f discounted=%.1f took=%s\n", taxed, discounted, dur)

	blob, _ := json.Marshal(sc) // persist to jsonb
	var reloaded pipe.Scope
	_ = json.Unmarshal(blob, &reloaded) // read back
	back, _ := reloaded.GetFloat64("calc.taxed")
	fmt.Printf("reloaded taxed=%.1f\n", back)

	// Output:
	// taxed=22.0 discounted=18.0 took=5ms
	// reloaded taxed=22.0
}
