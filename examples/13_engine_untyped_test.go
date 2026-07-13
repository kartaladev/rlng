// 13 — the rlng engine facade: rlng.Engine wraps a *pipe.Pipeline (built by
// hand with Go constructors, as files 05-10 do directly, or from a config
// document, as file 11 does) and owns the per-call boilerplate — seed a
// Scope, Run the pipeline, hand back a result — so a caller only has to
// think about input in and output out. Evaluate returns the accumulated
// map[string]any; EvaluateScope returns the *pipe.Scope itself for when the
// caller also needs what the map throws away (timing, JSON persistence,
// provenance). rlng.NewFromYAML folds config.Parse + PipelineDef.Build +
// rlng.New into one call, the one-line form a caller reaches for once the
// pipeline is authored as a document rather than Go code.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
)

// Example_engineEvaluateAndEvaluateScope builds a two-stage loan-offer
// pipeline (an APR decision keyed off credit utilization) and wraps it in an
// rlng.Engine. Evaluate is the "just give me the numbers" call — it runs the
// pipeline and hands back Scope.Snapshot() directly. EvaluateScope instead
// hands back the live *pipe.Scope, unlocking everything a plain map can't
// carry; here, constructing the Engine with
// WithScopeOptions(pipe.WithProvenance()) means every Scope it seeds records
// derivations, so the Scope returned by EvaluateScope supports Explain — a
// caller can trace a computed value ("utilization") back through the
// single-expr stage that derived it to the seed balance/limit that fed it,
// the same trail file 10 builds directly against a Scope.
func Example_engineEvaluateAndEvaluateScope() {
	utilization, err := pipe.NewSingleExpr("utilization", "balance / limit")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	offer, err := pipe.NewDecisionTable("offer", []pipe.Rule{
		{
			ID:        "PRIME",
			Condition: "score >= 700 && utilization < 0.3",
			Decisions: map[string]pipe.Decision{"apr": {Expr: "0.0499"}},
		},
		{
			ID:        "STANDARD",
			Condition: "score >= 650",
			Decisions: map[string]pipe.Decision{"apr": {Expr: "0.0699"}},
		},
	}, pipe.WithDependsOn("utilization"), pipe.WithDefault(map[string]pipe.Decision{
		"apr": {Expr: "0.1299"},
	}))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{utilization, offer})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	engine, err := rlng.New(pipeline, rlng.WithScopeOptions(pipe.WithProvenance()))
	if err != nil {
		fmt.Println("engine:", err)
		return
	}

	applicant := map[string]any{"score": 720, "balance": 1200, "limit": 5000}

	// Evaluate: the raw accumulated map, for a caller that just wants values.
	// Each Evaluate call seeds and runs a fresh Scope, so the Engine itself is
	// reusable and safe for concurrent callers.
	out, err := engine.Evaluate(context.Background(), applicant)
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	offerMap := out["offer"].(map[string]any)
	fmt.Printf("utilization=%.2f apr=%v\n", out["utilization"], offerMap["apr"])

	// EvaluateScope: the live Scope, for a caller that needs more than the
	// numbers — here, Explain traces "utilization" back to the inputs that
	// derived it.
	sc, err := engine.EvaluateScope(context.Background(), applicant)
	if err != nil {
		fmt.Println("evaluate scope:", err)
		return
	}
	fmt.Print(sc.Explain("utilization"))

	// Output:
	// utilization=0.24 apr=0.0499
	// utilization = 0.24 [utilization single-expr] expr: balance / limit
	//   balance = 1200 [seed]
	//   limit = 5000 [seed]
}

// Example_engineFromYAML shows rlng.NewFromYAML: the one-call shortcut for
// "parse this YAML document, build the pipeline, wrap it in an Engine" — it
// is exactly shorthand for
// rlng.NewFromProvider(ctx, config.FromYAMLString(yaml), opts...), skipping
// the explicit config.Parse/PipelineDef.Build steps file 11 spells out by
// hand. Reach for the explicit form instead when a build-time option (a
// strict schema, WithLintErrors, a ruleset version override) is needed —
// NewFromYAML only exposes the Engine-level Options (WithScopeOptions,
// WithConcurrency, WithMaxParallel; see file 14 for the concurrency one).
func Example_engineFromYAML() {
	const shippingRules = `
version: 2026.1
stages:
  - name: shipping
    type: decision-table
    rules:
      - id: FREE_OVER_THRESHOLD
        condition: "subtotal >= 75"
        decisions:
          fee: "0"
      - id: STANDARD
        condition: "true"
        decisions:
          fee: "6.99"
`
	engine, err := rlng.NewFromYAML(context.Background(), shippingRules)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}

	small, err := engine.Evaluate(context.Background(), map[string]any{"subtotal": 42.50})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Println("small order fee:", small["shipping"].(map[string]any)["fee"])

	large, err := engine.Evaluate(context.Background(), map[string]any{"subtotal": 80})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Println("large order fee:", large["shipping"].(map[string]any)["fee"])

	// Output:
	// small order fee: 6.99
	// large order fee: 0
}

// Example_engineEvaluateNilInput demonstrates the rlng.ErrNilInput quirk:
// Evaluate treats a genuinely absent input differently from an empty one. An
// untyped nil (or a nil pointer) is rejected as ErrNilInput — if it were
// accepted, it would silently seed an empty Scope and hand back a bogus zero
// result instead of surfacing the caller's mistake. A non-nil empty
// map[string]any{}, by contrast, is a perfectly valid empty seed: a stage
// that doesn't read the seed at all, like the constant expression below,
// evaluates normally against it.
func Example_engineEvaluateNilInput() {
	greeting, err := pipe.NewSingleExpr("greeting", `"hello"`)
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{greeting})
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	engine, err := rlng.New(pipeline)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}

	// nil input: rejected outright, never silently treated as empty.
	_, err = engine.Evaluate(context.Background(), nil)
	fmt.Println("nil input is ErrNilInput:", errors.Is(err, rlng.ErrNilInput))

	// A non-nil empty map, by contrast, is a valid empty seed.
	out, err := engine.Evaluate(context.Background(), map[string]any{})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Println("empty map seed greeting:", out["greeting"])

	// Output:
	// nil input is ErrNilInput: true
	// empty map seed greeting: hello
}
