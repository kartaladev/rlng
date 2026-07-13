// 10 — pipe explainability and deterministic time. WithProvenance makes a
// Scope record a Derivation for every value it holds — each seed input and
// each stage write — so Explain/Lineage/Derivations can trace a decision back
// to the inputs that produced it, an audit trail for "why did the engine
// decide this". WithClock replaces the wall clock a Scope stamps stage timing
// with, and pipe.NowFunc turns that same clock into an expr host function
// (now()) so a temporal rule reads injected time instead of time.Now() — the
// difference between a reproducible test/replay and a flaky one.
package examples_test

import (
	"context"
	"fmt"
	"time"

	"github.com/kartaladev/rlng/expr"
	"github.com/kartaladev/rlng/pipe"
)

// Example_explainCreditDecision traces a credit-tier decision back to its seed
// inputs. WithProvenance is the only thing that turns this on — without it,
// Explain/Lineage/Derivations are all no-ops (empty/false).
//
// Two reconciliation quirks live in derivationsFor, both demonstrated here:
//
//   - ancestor: the decision table reads "applicant.score", a member path, but
//     only the top-level seed key "applicant" (the whole map) was recorded as
//     a Derivation — a Scope's seed derivation is per top-level key, not per
//     leaf. Explain walks UP from "applicant.score" to the nearest recorded
//     ancestor, "applicant", and reports that.
//   - descendants (the "bare ref" case): the "archive" stage's expression is
//     just the bare identifier "grade" — no dot — copying the whole grade
//     decision map forward. "grade" itself was never written as its own
//     Derivation (the table wrote "grade.tier" and "grade.limit" under their
//     own dotted paths); Explain instead reconciles the bare "grade" reference
//     to EVERY derivation recorded under that namespace, walking into each.
func Example_explainCreditDecision() {
	grade, err := pipe.NewDecisionTable("grade", []pipe.Rule{
		{
			ID:        "PRIME",
			Condition: "applicant.score >= 750",
			Decisions: map[string]pipe.Decision{
				"tier":  {Expr: `"prime"`},
				"limit": {Expr: "applicant.score * 100"},
			},
		},
		{
			ID:        "NEAR_PRIME",
			Condition: "applicant.score >= 650",
			Decisions: map[string]pipe.Decision{
				"tier":  {Expr: `"near_prime"`},
				"limit": {Expr: "applicant.score * 50"},
			},
		},
	})
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	// archive copies the whole "grade" decision map forward under its own
	// name, via a bare (dot-free) reference to "grade".
	archive, err := pipe.NewSingleExpr("archive", "grade", pipe.WithDependsOn("grade"))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{grade, archive})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{
		"applicant": map[string]any{"score": 700},
	}, pipe.WithProvenance())
	if err := pipeline.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}

	tier, _ := sc.GetString("grade.tier")
	fmt.Println("tier:", tier)

	fmt.Print(sc.Explain("grade.limit"))
	fmt.Println("---")
	fmt.Print(sc.Explain("archive"))

	fmt.Println("lineage entries for grade.limit:", len(sc.Lineage("grade.limit")))

	// Derivations returns the full recorded graph: every seed input and every
	// stage write, as a map[string]Derivation keyed by scope dot-path. Explain
	// and Lineage above are both built by walking this same map; printing its
	// size (not its contents, which are map-order-unstable) confirms
	// WithProvenance recorded one Derivation per seed key and per stage output
	// ("applicant" seed, "grade.tier", "grade.limit", "archive").
	fmt.Println("recorded derivations:", len(sc.Derivations()))

	// FiringRule names the decision-table rule that produced "grade" — a
	// firing trail kept independently of provenance (recorded even without
	// WithProvenance), so a caller can ask "which rule decided this?" without
	// paying for full derivation tracking.
	if fr, ok := sc.FiringRule("grade"); ok {
		fmt.Println("grade fired rule:", fr.RuleID)
	}

	// Output:
	// tier: near_prime
	// grade.limit = 35000 [grade decision-table] expr: applicant.score * 50
	//   applicant = map[score:700] [seed]
	// ---
	// archive = map[limit:35000 tier:near_prime] [archive single-expr] expr: grade
	//   grade.limit = 35000 [grade decision-table] expr: applicant.score * 50
	//     applicant = map[score:700] [seed]
	//   grade.tier = near_prime [grade decision-table] expr: "near_prime"
	// lineage entries for grade.limit: 2
	// recorded derivations: 4
	// grade fired rule: NEAR_PRIME
}

// Example_deterministicTemporalRule shows WithClock + pipe.NowFunc making a
// time-dependent rule reproducible: expires.Before(now()) reads the injected
// clock, not the wall clock, so the exact same Scope and pipeline always
// evaluate the same way — a test (or a replayed production decision, see file
// 12) does not flip from pass to fail depending on when it happens to run.
// stepClock also backs the Scope's own timing (WithClock is a single option
// covering both), so Duration/StageTimings are stable across runs too — real
// wall-clock timing would make // Output: flaky by definition.
func Example_deterministicTemporalRule() {
	clock := &stepClock{times: []time.Time{
		time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 1, 0, 0, 0, 5_000_000, time.UTC), // +5ms
	}}

	status, err := pipe.NewSingleExpr("status", `expires.Before(now()) ? "expired" : "active"`,
		pipe.WithExprOptions(expr.WithFunction("now", pipe.NowFunc(clock))))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{status})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{
		"expires": time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC),
	}, pipe.WithClock(clock))
	if err := pipeline.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}

	result, _ := sc.GetString("status")
	fmt.Println("offer status:", result)

	dur, ok := sc.Duration()
	fmt.Println("run duration:", dur, "recorded:", ok)

	for _, st := range sc.StageTimings() {
		fmt.Println("stage timing:", st.Stage, st.Duration)
	}

	// Output:
	// offer status: expired
	// run duration: 5ms recorded: true
	// stage timing: status 0s
}

// stepClock is a deterministic pipe.Clock: it returns times[0] on the first
// call, times[1] on every call after that — enough steps to give
// Scope.Duration a stable, non-zero elapsed value without depending on the
// wall clock. (Same shape as the retired pricing_test.go's clock helper.)
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
