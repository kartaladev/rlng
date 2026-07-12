package examples_test

import (
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

// Example_strictTyping shows how WithEnv turns a field typo — which would
// silently evaluate to nil in the lenient default — into a compile-time error,
// the single most important correctness guard for production rules.
func Example_strictTyping() {
	env := map[string]any{"score": 0}

	// Lenient default: the typo "scoer" is tolerated (evaluates to nil).
	if _, err := expr.NewPredicate("scoer >= 650"); err == nil {
		fmt.Println("lenient: typo compiled (evaluates nil at runtime)")
	}

	// Strict: the same typo is rejected at compile time.
	_, err := expr.NewPredicate("scoer >= 650", expr.WithEnv(env))
	var compileErr *expr.CompileError
	fmt.Println("strict: typo rejected:", errors.As(err, &compileErr))

	// Output:
	// lenient: typo compiled (evaluates nil at runtime)
	// strict: typo rejected: true
}
