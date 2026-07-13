// 14 — the typed engine: rlng.TypedEngine[I, R] pairs an rlng.Engine with an
// rlng.Mapper[R], so Evaluate accepts a typed Go struct input and returns a
// typed Go struct result instead of map[string]any in, map[string]any out.
// The struct boundary has two notable exact-decimal behaviors: a
// decimal.Decimal input field survives the struct->seed conversion exactly
// (restored, not decomposed), and a decimal.Decimal scope value survives the
// scope->result conversion exactly too — UNLESS the result field's type can't
// represent it exactly, in which case Mapper.Map refuses to narrow it rather
// than silently truncate. The second file topic, ErrConcurrencyRequiresConfig,
// is really about the untyped Engine too, but is grouped here because both
// engine flavors share the same construction-time Option handling.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
)

// Example_typedEngineDecimalRoundTripAndLossyNarrowing builds a
// TypedEngine[LoanApplication, LoanQuote] computing a loan fee with
// exact-decimal arithmetic. LoanApplication's Principal and Rate are
// decimal.Decimal fields: flatten's restoreDecimals step rewrites them back
// into the Scope seed exactly after mapstructure decomposes the struct
// (decimal.Decimal has no exported fields, so mapstructure would otherwise
// flatten it into an empty map), so the pipeline's decimal(...) arithmetic
// sees the real value, not a zero. On the way out, Mapper.Map's decode hook
// is equally exact by default: mapping the resulting decimal fee into
// LoanQuote's own decimal.Decimal field keeps it exact. But that hook refuses
// to narrow a FRACTIONAL decimal into an integer-kind result field rather
// than truncate it — reusing the exact same pipeline output with a result
// type that asks for an int fee demonstrates the refusal as a
// *rlng.MappingError wrapping the rlng.ErrLossyResultNarrowing sentinel,
// checkable with errors.As/errors.Is.
func Example_typedEngineDecimalRoundTripAndLossyNarrowing() {
	type LoanApplication struct {
		Principal decimal.Decimal `mapstructure:"principal"`
		Rate      decimal.Decimal `mapstructure:"rate"`
	}
	type LoanQuote struct {
		Fee decimal.Decimal `mapstructure:"fee"`
	}

	fee, err := pipe.NewSingleExpr("fee", "roundBank(decimal(principal) * decimal(rate), 2)")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{fee})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	mapper, err := rlng.NewMapper[LoanQuote](rlng.MappingTemplate{"fee": "fee"})
	if err != nil {
		fmt.Println("mapper:", err)
		return
	}
	engine, err := rlng.NewTypedEngine[LoanApplication, LoanQuote](pipeline, mapper)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}

	application := LoanApplication{
		Principal: decimal.NewFromInt(1_000),
		Rate:      decimal.RequireFromString("0.0725"),
	}
	quote, err := engine.Evaluate(context.Background(), application)
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("fee=%s\n", quote.Fee.StringFixed(2))

	// Same pipeline, same application, but a result type that asks for an
	// integer fee: 72.50 has a fractional part, so decoding it into an int
	// would lose precision, and the hook refuses.
	type IntQuote struct {
		Fee int `mapstructure:"fee"`
	}
	intMapper, err := rlng.NewMapper[IntQuote](rlng.MappingTemplate{"fee": "fee"})
	if err != nil {
		fmt.Println("mapper:", err)
		return
	}
	intEngine, err := rlng.NewTypedEngine[LoanApplication, IntQuote](pipeline, intMapper)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}
	_, err = intEngine.Evaluate(context.Background(), application)
	var mappingErr *rlng.MappingError
	fmt.Println("rejected as *rlng.MappingError:", errors.As(err, &mappingErr))
	fmt.Println("cause is ErrLossyResultNarrowing:", errors.Is(err, rlng.ErrLossyResultNarrowing))

	// Output:
	// fee=72.50
	// rejected as *rlng.MappingError: true
	// cause is ErrLossyResultNarrowing: true
}

// Example_concurrencyRequiresConfig shows why WithConcurrency is only
// accepted by the config-driven constructors. rlng.New and rlng.NewTypedEngine
// both wrap an ALREADY-BUILT *pipe.Pipeline — concurrency (independent stages
// running on separate goroutines, see file 08) is a property baked in at
// pipe.NewPipeline/PipelineDef.Build time, and there is no pipeline left to
// rebuild by the time New sees it. Passing WithConcurrency there fails fast,
// at construction, with ErrConcurrencyRequiresConfig, rather than silently
// ignoring the option. NewFromYAML/NewFromProvider (and their typed
// counterparts, NewTypedFromYAML/NewTypedFromProvider — the typed one-call
// shortcut mirroring NewFromYAML from file 13, now for a TypedEngine) DO own
// the build step, so the identical option threads into PipelineDef.Build and
// works.
func Example_concurrencyRequiresConfig() {
	rate, err := pipe.NewSingleExpr("rate", "apr / 12")
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{rate})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	_, err = rlng.New(pipeline, rlng.WithConcurrency())
	fmt.Println("New + WithConcurrency rejected:", errors.Is(err, rlng.ErrConcurrencyRequiresConfig))

	const monthlyRateRules = `
stages:
  - name: rate
    type: single-expr
    expr: apr / 12
`
	engine, err := rlng.NewFromYAML(context.Background(), monthlyRateRules, rlng.WithConcurrency())
	if err != nil {
		fmt.Println("NewFromYAML:", err)
		return
	}
	out, err := engine.Evaluate(context.Background(), map[string]any{"apr": 0.06})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Println("monthly rate:", out["rate"])

	// Output:
	// New + WithConcurrency rejected: true
	// monthly rate: 0.005
}
