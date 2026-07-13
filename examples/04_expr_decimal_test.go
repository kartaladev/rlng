// 04 — expr exact-decimal money. Float64 is unsafe for money: $250,000 at
// 7.25% computed in float64 is 18124.999999999996, not $18,125.00. Every
// compiled expression has a decimal(x) constructor, decimal-aware +, -, *, /
// operators, and two rounding builtins (round, half-away-from-zero; roundBank,
// half-even / banker's) available at all times — no option needed to turn
// exact decimal on. What DOES need care is HOW the operands enter the
// expression: decimal(...) makes an operand statically decimal at compile
// time, so wrapped arithmetic is exact in any mode; a BARE variable that
// happens to hold a decimal.Decimal at runtime is not statically known to be
// decimal unless the expression is compiled with WithEnv.
package examples_test

import (
	"fmt"

	"github.com/kartaladev/rlng/expr"
	"github.com/shopspring/decimal"
)

// Example_decimalInvoiceTotal computes a loan's daily interest accrual with
// exact-decimal arithmetic, then rounds it to cents two ways to show that
// round (half-away-from-zero) and roundBank (half-even / banker's) genuinely
// disagree on a tie: $125.00 principal at a 1.7% daily rate accrues exactly
// $2.125 — a dead-even half-cent. round pushes it up to $2.13; roundBank, tied
// between $2.12 and $2.13, picks the EVEN cent, $2.12. Financial ledgers
// typically want roundBank precisely because repeated half-away rounding
// biases a running total upward.
func Example_decimalInvoiceTotal() {
	accrual, err := expr.NewFunction("accrual", "decimal(balance) * decimal(dailyRate)")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	env := map[string]any{"balance": 125, "dailyRate": "0.017"}

	exact, _ := accrual.Apply(env)
	fmt.Println("exact accrual:", exact)

	roundHalfAway, _ := expr.NewFunction("rounded", "round(decimal(balance) * decimal(dailyRate), 2)")
	rv, _ := roundHalfAway.Apply(env)
	fmt.Println("round (half-away-from-zero):", rv)

	roundBankers, _ := expr.NewFunction("rounded", "roundBank(decimal(balance) * decimal(dailyRate), 2)")
	bv, _ := roundBankers.Apply(env)
	fmt.Println("roundBank (half-even):      ", bv)

	// Output:
	// exact accrual: 2.125
	// round (half-away-from-zero): 2.13
	// roundBank (half-even):       2.12
}

// Example_decimalBareVariableNeedsWithEnv shows the compile-time operator
// resolution quirk. decimal(x) * decimal(y) always works: both operands are
// statically decimal at compile time, so the "*" operator resolves to the
// decimal-aware multiply regardless of mode. But a rule that receives ALREADY
// decimal values from the caller (e.g. a decimal.Decimal seeded straight into
// the env, skipping decimal(...) in the expression text) and writes the bare
// "principal * rate" fails at eval time in the default lenient mode — the
// compiler cannot see the runtime type, so "*" resolves to the ordinary
// numeric multiply, which then rejects two decimal.Decimal operands. WithEnv
// fixes this: declaring the env's shape (with representative decimal.Decimal
// values) lets the compiler see the operand types and resolve "*" to the
// decimal-aware overload.
func Example_decimalBareVariableNeedsWithEnv() {
	principal := decimal.NewFromInt(125_000)
	rate := decimal.RequireFromString("0.0725")
	env := map[string]any{"principal": principal, "rate": rate}

	// Without WithEnv: "*" resolves at compile time as the ordinary numeric
	// operator, which then errors on two decimal.Decimal runtime operands.
	lenient, err := expr.NewFunction("fee", "principal * rate")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	_, err = lenient.Apply(env)
	fmt.Println("bare vars, no WithEnv, fails:", err != nil)

	// WithEnv, declaring the env's shape: the compiler now knows principal
	// and rate are decimal.Decimal, and resolves "*" to the exact overload.
	declared := map[string]any{"principal": decimal.Decimal{}, "rate": decimal.Decimal{}}
	strict, err := expr.NewFunction("fee", "principal * rate", expr.WithEnv(declared))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	fee, err := strict.Apply(env)
	fmt.Printf("bare vars, with WithEnv: fee=%v err=%v\n", fee, err)

	// Output:
	// bare vars, no WithEnv, fails: true
	// bare vars, with WithEnv: fee=9062.5 err=<nil>
}
