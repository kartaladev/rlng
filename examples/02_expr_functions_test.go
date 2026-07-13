// 02 — expr value functions & fallbacks. A Function is a compiled,
// value-producing expression (as opposed to a Predicate's yes/no) — this is
// what a decision-table rule's `decisions` entries and a single-expr stage's
// `expr` compile down to. Beyond the bare compile-and-apply, Function offers
// a fallback mechanism (WithFallback / WithFallbackOnNil / WithFallbackObserver)
// so a rule can degrade to a safe default instead of failing the whole
// evaluation when its inputs are missing or malformed.
package examples_test

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kartaladev/rlng/expr"
)

// Example_functionLineItemTotal is the simplest Function: one expression, no
// error path, no fallback — the y = f(x) building block every stage in the
// engine is compiled from.
func Example_functionLineItemTotal() {
	lineTotal, err := expr.NewFunction("lineTotal", "unitPrice * quantity")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	amount, err := lineTotal.Apply(map[string]any{"unitPrice": 24.50, "quantity": 3})
	fmt.Printf("amount=%v err=%v\n", amount, err)

	// Output:
	// amount=73.5 err=<nil>
}

// Example_functionFallbackSafeDivision shows WithFallback: when the main
// expression errors, a separately-compiled fallback expression supplies a
// default instead of propagating the error. Two quirks matter for production
// use: the fallback is compiled EAGERLY at NewFunction time (a broken
// fallback expression fails construction, not some later unlucky Apply
// call), and the fallback is evaluated over an EMPTY env — it cannot read the
// main expression's variables, so a fallback that references an input
// variable silently sees nil for it, not the caller's value.
func Example_functionFallbackSafeDivision() {
	// A currency conversion: converted = amount / spotRate. If the spot rate
	// is missing, dividing by nil errors, and the fallback should supply a
	// safe replacement RESULT for "converted" (the fallback replaces the
	// whole function result, it does not patch just the missing operand). A
	// first (naive) attempt reuses "amount", assuming the fallback can see
	// the caller's input the way the main expression did.
	naive, err := expr.NewFunction("converted", "amount / spotRate", expr.WithFallback("amount"))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	got, err := naive.Apply(map[string]any{"amount": 500}) // spotRate missing
	fmt.Printf("naive fallback (references amount):  converted=%v err=%v\n", got, err)

	// The fix: a fallback that does not depend on the caller's variables — a
	// literal sentinel of 0, flagging the line for manual FX review.
	fixed, _ := expr.NewFunction("converted", "amount / spotRate", expr.WithFallback("0"))
	got, err = fixed.Apply(map[string]any{"amount": 500})
	fmt.Printf("literal fallback (sentinel 0):       converted=%v err=%v\n", got, err)

	// The happy path still uses the main expression when the rate is present.
	got, err = fixed.Apply(map[string]any{"amount": 500, "spotRate": 2.0})
	fmt.Printf("spot rate present:                   converted=%v err=%v\n", got, err)

	// Output:
	// naive fallback (references amount):  converted=<nil> err=<nil>
	// literal fallback (sentinel 0):       converted=0 err=<nil>
	// spot rate present:                   converted=250 err=<nil>
}

// Example_functionFallbackOnNilAndObserver shows two more fallback controls.
// WithFallbackOnNil widens the fallback trigger: by default nil is a
// first-class result (a Function that legitimately computes nil does NOT
// fall back), but WithFallbackOnNil also falls back when the main expression
// yields nil — useful for a map lookup that "misses" by returning nil rather
// than erroring. WithFallbackObserver reports the masked error whenever the
// fallback fires because of an actual ERROR — it is deliberately NOT called
// for the nil-triggered path, since there is no error to report there.
func Example_functionFallbackOnNilAndObserver() {
	var observed []string
	observe := expr.WithFallbackObserver(func(name, expression string, cause error) {
		observed = append(observed, fmt.Sprintf("%s (%s): %v", name, expression, cause))
	})

	// Case 1: a loyalty-tier override map lookup. A missing key returns nil
	// (not an error) — WithFallbackOnNil catches it and substitutes the
	// standard discount rate. The observer does NOT fire: no error occurred.
	override, _ := expr.NewFunction("discountRate", "overrides[tier]",
		expr.WithFallback("0.05"), expr.WithFallbackOnNil(), observe)
	rate, err := override.Apply(map[string]any{
		"tier":      "bronze",
		"overrides": map[string]any{"gold": 0.20},
	})
	fmt.Printf("bronze rate=%v err=%v observed=%d\n", rate, err, len(observed))

	// Case 2: dividing the total by a genuinely missing (nil) volume ERRORS —
	// the observer fires, recording the masked cause before the fallback runs.
	perUnit, _ := expr.NewFunction("perUnitCost", "totalCost / volume",
		expr.WithFallback("0"), expr.WithFallbackOnNil(), observe)
	cost, err := perUnit.Apply(map[string]any{"totalCost": 500})
	fmt.Printf("perUnit=%v err=%v observed=%d\n", cost, err, len(observed))

	// The observer log names the function and its expression alongside the
	// masked cause — enough to diagnose which rule silently degraded.
	for _, o := range observed {
		fmt.Println("observed mentions perUnitCost:", strings.Contains(o, "perUnitCost"))
		fmt.Println("observed mentions expression: ", strings.Contains(o, "totalCost / volume"))
	}

	// Output:
	// bronze rate=0.05 err=<nil> observed=0
	// perUnit=0 err=<nil> observed=1
	// observed mentions perUnitCost: true
	// observed mentions expression:  true
}

// Example_functionReturnKindDiscountTierCount shows WithReturnKind(reflect.Kind):
// it compiles the Function's expression with expr-lang's AsKind wrapper, so
// Apply coerces its result to the declared reflect.Kind instead of leaving it
// as whatever native type the expression produced. Useful when a computed
// value feeds a typed field — a loyalty program's discount-tier count, a
// score bucket — and the caller wants a Go int, not a float64 that needs a
// manual conversion (and a truncation-vs-rounding decision) at every call
// site. The quirk to know: the SAME coercion is wired into BOTH the main
// expression AND the fallback expression at NewFunction time (one config
// value feeds both compiles) — a fallback literal that "looks like" a float
// is truncated to the declared kind exactly like the main result would be,
// it never surfaces as a float64 just because it's a "default".
func Example_functionReturnKindDiscountTierCount() {
	// A loyalty program awards one discount tier per $1000 of spend. expr's
	// "/" always yields a float64; WithReturnKind(reflect.Int) coerces the
	// Apply result to Go's int, so a switch over tiers downstream needs no
	// extra cast.
	tierCount, err := expr.NewFunction("discountTierCount", "totalSpend / 1000",
		expr.WithReturnKind(reflect.Int), expr.WithFallback("2.9"))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	tiers, err := tierCount.Apply(map[string]any{"totalSpend": 4500.0})
	fmt.Printf("spend=4500 -> tiers=%v (%T) err=%v\n", tiers, tiers, err)

	// The fallback quirk: totalSpend missing means "totalSpend / 1000" divides
	// nil, which errors, triggering the fallback. WithReturnKind's coercion
	// applies there too — the fallback literal "2.9" is truncated to 2, not
	// returned as 2.9.
	degraded, err := tierCount.Apply(map[string]any{})
	fmt.Printf("spend missing -> fallback tiers=%v (%T) err=%v\n", degraded, degraded, err)

	// Output:
	// spend=4500 -> tiers=4 (int) err=<nil>
	// spend missing -> fallback tiers=2 (int) err=<nil>
}
