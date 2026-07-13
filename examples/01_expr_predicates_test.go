// 01 — expr boolean predicates: the simplest building block in rlng. A
// Predicate compiles a boolean expression once and evaluates it repeatedly
// against different envs — this is what a decision-table rule's `condition`
// and a stage's `condition` gate compile down to. Two modes: strict (the
// default — the expression MUST evaluate to bool, or evaluation fails loudly)
// and lenient (WithCoerce — a broad set of non-bool values are coerced to
// bool by well-defined rules, useful when upstream data is loosely typed).
package examples_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/kartaladev/rlng/expr"
)

// Example_predicateStrictEligibilityGate shows the default, strict mode: the
// expression must evaluate to bool. This is the right default for a
// production eligibility gate — a typo or a non-bool result must never be
// silently treated as "false" (which would wrongly gate an applicant out) or
// "true" (which would wrongly let one through). Instead it fails loudly as an
// *expr.EvalError wrapping expr.ErrNotBool, checkable with errors.Is.
func Example_predicateStrictEligibilityGate() {
	gate, err := expr.NewPredicate("age >= 18 && verified")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	adultVerified, err := gate.Test(map[string]any{"age": 34, "verified": true})
	fmt.Println("adult, verified:", adultVerified, err)

	minor, err := gate.Test(map[string]any{"age": 16, "verified": true})
	fmt.Println("minor, verified:", minor, err)

	// A predicate that accidentally returns a non-bool value (a common typo:
	// forgetting a comparison operator) fails loudly instead of silently
	// gating every applicant the same way.
	broken, err := expr.NewPredicate("age") // missing ">= 18" — not a bool
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	_, err = broken.Test(map[string]any{"age": 34})
	fmt.Println("non-bool result rejected:", errors.Is(err, expr.ErrNotBool))

	// Output:
	// adult, verified: true <nil>
	// minor, verified: false <nil>
	// non-bool result rejected: true
}

// Example_predicateCoerceFeatureFlag shows WithCoerce's lenient truthiness,
// useful when a flag arrives from a loosely-typed upstream source (an env
// var string, a JSON-decoded number). The quirks that matter in practice: an
// empty string is false (not an error); a numeric-looking string like "3"
// does NOT parse as a bool literal (only "0"/"1"/"true"/"false"/... do), so
// it falls through to the non-empty-is-true rule; and encoding/json's
// json.Number is a defined STRING type, so it is coerced via the same
// string-parsing path as a plain string, not treated as a number.
func Example_predicateCoerceFeatureFlag() {
	rollout, err := expr.NewPredicate("promoActive", expr.WithCoerce())
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	cases := []struct {
		label string
		env   map[string]any
	}{
		{`env var "true"`, map[string]any{"promoActive": "true"}},
		{`env var "1"`, map[string]any{"promoActive": "1"}},
		{`env var "" (unset)`, map[string]any{"promoActive": ""}},
		{`json.Number("3") - a redemption count, not 0/1`, map[string]any{"promoActive": json.Number("3")}},
		{`json.Number("0")`, map[string]any{"promoActive": json.Number("0")}},
	}
	for _, c := range cases {
		enabled, err := rollout.Test(c.env)
		fmt.Printf("%-46s -> enabled=%v err=%v\n", c.label, enabled, err)
	}

	// Output:
	// env var "true"                                 -> enabled=true err=<nil>
	// env var "1"                                    -> enabled=true err=<nil>
	// env var "" (unset)                             -> enabled=false err=<nil>
	// json.Number("3") - a redemption count, not 0/1 -> enabled=true err=<nil>
	// json.Number("0")                               -> enabled=false err=<nil>
}

// Example_predicateCoerceNumericAndCollections rounds out WithCoerce's
// lenient truthiness with two more quirks: a slice/map is truthy iff it is
// non-empty, and a float is truthy iff it is BOTH non-zero AND finite — NaN
// and +-Inf coerce to false rather than erroring. That matters for a rule
// gated on a computed ratio: a degenerate 0/0 division must not blow up the
// gate, it should simply fail the gate.
func Example_predicateCoerceNumericAndCollections() {
	hasHolds, _ := expr.NewPredicate("holds", expr.WithCoerce())
	withHold, _ := hasHolds.Test(map[string]any{"holds": []any{"pending-kyc"}})
	fmt.Println("non-empty holds list:", withHold)
	noHold, _ := hasHolds.Test(map[string]any{"holds": []any{}})
	fmt.Println("empty holds list:", noHold)

	ratioOK, _ := expr.NewPredicate("utilization", expr.WithCoerce())
	nan, _ := ratioOK.Test(map[string]any{"utilization": math.NaN()})
	fmt.Println("NaN utilization (e.g. 0/0):", nan)
	inf, _ := ratioOK.Test(map[string]any{"utilization": math.Inf(1)})
	fmt.Println("+Inf utilization (e.g. x/0):", inf)
	negInf, _ := ratioOK.Test(map[string]any{"utilization": math.Inf(-1)})
	fmt.Println("-Inf utilization (e.g. -x/0):", negInf)
	finite, _ := ratioOK.Test(map[string]any{"utilization": 0.72})
	fmt.Println("finite utilization:", finite)

	// Output:
	// non-empty holds list: true
	// empty holds list: false
	// NaN utilization (e.g. 0/0): false
	// +Inf utilization (e.g. x/0): false
	// -Inf utilization (e.g. -x/0): false
	// finite utilization: true
}
