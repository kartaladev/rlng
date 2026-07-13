// 05 — pipe Scope: the mutable, concurrency-safe map[string]any accumulator
// threaded through stage evaluation. Every stage reads and writes it by
// dot-separated path, and it doubles as the expr evaluation environment. This
// file covers Scope's two building blocks: Set/Get path addressing (with the
// optional WithStrict collision guard) and the typed getters, which come in a
// strict flavor (exact Go type required) and a coercing flavor (a wider set of
// "close enough" runtime shapes accepted) — but coercion NEVER silently rounds
// or wraps a value; it either converts exactly or fails loudly.
package examples_test

import (
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// Example_scopeDotPathAddressing shows the basic Set/Get contract a stage
// relies on: a result is written under a dot path (conventionally
// "<stage>.<field>"), and a later stage (or the caller) reads it the same way,
// walking through intermediate maps that Set creates on demand. WithStrict
// turns an accidental duplicate write — two stages racing to own the same
// output path — into an explicit ErrPathConflict instead of a silently
// clobbered value, which matters most once concurrent execution is in play
// (see file 08).
func Example_scopeDotPathAddressing() {
	sc := pipe.NewScope(map[string]any{"applicant": map[string]any{"name": "R. Alden"}})

	_ = sc.Set("underwriting.tier", "prime")
	_ = sc.Set("underwriting.limit", 45000)

	name, _ := sc.Get("applicant.name")
	tier, _ := sc.Get("underwriting.tier")
	limit, _ := sc.Get("underwriting.limit")
	fmt.Println("applicant:", name)
	fmt.Println("tier:", tier, "limit:", limit)

	// A second stage naively reusing "underwriting.tier" is a config bug —
	// under WithStrict it is caught immediately as ErrPathConflict rather than
	// silently overwriting the first stage's decision.
	strict := pipe.NewScope(nil, pipe.WithStrict())
	_ = strict.Set("underwriting.tier", "prime")
	err := strict.Set("underwriting.tier", "near_prime")
	fmt.Println("duplicate write rejected:", errors.Is(err, pipe.ErrPathConflict))

	// Output:
	// applicant: R. Alden
	// tier: prime limit: 45000
	// duplicate write rejected: true
}

// Example_scopeGettersStrictVsCoerce contrasts the strict typed getters
// (GetInt, GetFloat64, GetString — the exact Go type, or a lossless
// json.Number) with their Coerce siblings (GetIntCoerce, GetFloat64Coerce —
// also accept a numeric string or a wider numeric kind). The quirk that
// matters most for a money/underwriting engine: coercion NEVER silently
// truncates or wraps. A numeric string parses exactly; a non-integral float or
// a value outside int64's range is rejected as a *ScopeTypeError rather than
// rounded or masked, so a bad upstream value fails loudly instead of quietly
// corrupting a decision.
func Example_scopeGettersStrictVsCoerce() {
	sc := pipe.NewScope(map[string]any{
		"annualIncome":   "125000",   // arrives as a string from a web form
		"declaredFee":    int64(150), // an already-typed stage output
		"utilization":    42.5,       // a ratio, not a count — not integral
		"lifetimeVolume": 1e19,       // integral, but overflows int64
	})

	// Strict GetInt refuses a numeric string outright — it is not an int.
	_, err := sc.GetInt("annualIncome")
	var typeErr *pipe.ScopeTypeError
	fmt.Println("strict GetInt on a numeric string fails:", errors.As(err, &typeErr))

	// GetIntCoerce accepts the same numeric string, parsed exactly.
	income, err := sc.GetIntCoerce("annualIncome")
	fmt.Println("coerced income:", income, err)

	fee, err := sc.GetInt("declaredFee")
	fmt.Println("strict GetInt on an int64:", fee, err)

	// A non-integral float must not be silently truncated to an int —
	// GetIntCoerce rejects it loudly instead of dropping the fraction.
	_, err = sc.GetIntCoerce("utilization")
	if errors.As(err, &typeErr) {
		fmt.Println("non-integral float rejected:", typeErr.Actual)
	}

	// A value beyond int64's range is likewise rejected rather than wrapped.
	_, err = sc.GetIntCoerce("lifetimeVolume")
	if errors.As(err, &typeErr) {
		fmt.Println("out-of-range float rejected:", typeErr.Actual)
	}

	// Output:
	// strict GetInt on a numeric string fails: true
	// coerced income: 125000 <nil>
	// strict GetInt on an int64: 150 <nil>
	// non-integral float rejected: float64(42.5) is not integral
	// out-of-range float rejected: float64(1e+19) overflows int64
}
