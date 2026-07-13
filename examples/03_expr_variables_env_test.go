// 03 — expr variable defaults, strict typing, and host functions. Beyond a
// bare expression, three mechanisms shape how variables and functions are
// resolved: WithGlobals/WithLocals inject `x ?? <default>` defaults at
// compile time (so runtime input always wins, but a rule need not repeat its
// own thresholds); WithEnv turns on strict type-checking against a declared
// environment, catching a field typo at compile time instead of letting it
// silently evaluate to nil; and WithFunction registers a Go function callable
// by name from inside the expression.
package examples_test

import (
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

// Example_variableDefaultsGlobalsLocals shows WithGlobals/WithLocals:
// declared defaults are patched into the expression as `x ?? <literal>`, so a
// value present in the runtime env always wins over both. When the SAME name
// is declared by both, LOCALS win over globals (a per-rule override beats a
// pipeline-wide default) — and multiple calls to either option merge,
// last-value-wins per key, rather than the later call discarding the earlier
// declarations.
func Example_variableDefaultsGlobalsLocals() {
	gate, err := expr.NewPredicate("score >= minScore",
		expr.WithGlobals(map[string]any{"minScore": 650}), // pipeline-wide default
		expr.WithLocals(map[string]any{"minScore": 700}))  // this rule's stricter override
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	// No minScore in the runtime env: the LOCAL default (700) applies, not
	// the global (650), because locals take precedence.
	ok, _ := gate.Test(map[string]any{"score": 680})
	fmt.Println("no override, score 680 (local default 700 wins):", ok)

	// Runtime input always wins over both compiled-in defaults.
	ok, _ = gate.Test(map[string]any{"score": 680, "minScore": 600})
	fmt.Println("runtime override 600, score 680:", ok)

	// Output:
	// no override, score 680 (local default 700 wins): false
	// runtime override 600, score 680: true
}

// Example_variableStrictEnvCatchesTypo shows WithEnv: the expression is
// type-checked against a declared environment (a map of name -> a
// representative value of its type), and expr's default tolerance for
// undefined variables is dropped. The classic production bug — a field typo
// that quietly evaluates to nil forever — becomes a *expr.CompileError
// instead of a silent no-op.
func Example_variableStrictEnvCatchesTypo() {
	// Lenient (default): "scoer" (missing the 'e' in "score") is tolerated —
	// expr.AllowUndefinedVariables treats it as always-nil, so the rule
	// compiles and would evaluate to false forever without ever failing loudly.
	_, err := expr.NewPredicate("scoer >= 650")
	fmt.Println("lenient: typo compiles:", err == nil)

	// Strict: declare the expected shape of the env, and the same typo is
	// rejected before the rule ever runs.
	declaredEnv := map[string]any{"score": 0}
	_, err = expr.NewPredicate("scoer >= 650", expr.WithEnv(declaredEnv))
	var compileErr *expr.CompileError
	fmt.Println("strict: typo rejected:", errors.As(err, &compileErr))

	// The correctly-spelled expression still compiles under WithEnv.
	fixed, err := expr.NewPredicate("score >= 650", expr.WithEnv(declaredEnv))
	fmt.Println("strict: correct field compiles:", err == nil)
	ok, _ := fixed.Test(map[string]any{"score": 700})
	fmt.Println("strict: evaluates:", ok)

	// Output:
	// lenient: typo compiles: true
	// strict: typo rejected: true
	// strict: correct field compiles: true
	// strict: evaluates: true
}

// Example_variableHostFunction registers a Go function callable from inside
// the expression via WithFunction — the mechanism a rule uses to reach a
// domain helper (a lookup table, a business-day calculator) that is awkward
// to express as pure expr syntax. Registering the SAME name more than once
// keeps the LAST registration, which is what lets a tenant-specific
// implementation override a base rule set's default helper.
func Example_variableHostFunction() {
	standardDiscount := func(args ...any) (any, error) {
		switch args[0].(string) {
		case "gold":
			return 0.15, nil
		case "silver":
			return 0.08, nil
		default:
			return 0.0, nil
		}
	}
	premiumTenantDiscount := func(args ...any) (any, error) {
		if args[0].(string) == "gold" {
			return 0.20, nil // this tenant runs a richer gold-tier discount
		}
		return standardDiscount(args...)
	}

	discount, err := expr.NewFunction("discount", "discountFor(tier)",
		expr.WithFunction("discountFor", standardDiscount),
		expr.WithFunction("discountFor", premiumTenantDiscount)) // last registration wins
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	gold, _ := discount.Apply(map[string]any{"tier": "gold"})
	fmt.Println("gold discount (tenant override applies):", gold)

	silver, _ := discount.Apply(map[string]any{"tier": "silver"})
	fmt.Println("silver discount (falls through to standard):", silver)

	// Output:
	// gold discount (tenant override applies): 0.2
	// silver discount (falls through to standard): 0.08
}
