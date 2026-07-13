package examples_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
)

// Example_decimalMoney computes a loan origination fee with exact-decimal
// arithmetic and shows the value keeping its type and precision across every
// serde boundary (spec 014 / ADR-0038, ADR-0039).
//
// $250,000 at 7.25% is exactly $18,125.00 — not the 18124.999999999996 a
// float64 multiply produces. The rate is declared once as a decimal config
// constant ({"$dec": "0.0725"}); the principal is seeded as a decimal; the
// arithmetic stays decimal via the decimal() builtin and operator overloads;
// roundBank rounds deterministically (banker's, half-even); the result survives
// a Scope JSON round-trip (audit persistence) reloading as the same decimal;
// and it maps into a typed result preserving the exact decimal.
func Example_decimalMoney() {
	def, err := config.Parse(context.Background(), config.FromYAMLString(`
constants:
  rate: {$dec: "0.0725"}
stages:
  - name: loan
    type: single-expr
    expr: roundBank(decimal(principal) * decimal(rate), 2)
`))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	engine, err := rlng.New(pipeline)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}

	// Seed the principal as an exact decimal (the map seed path preserves it).
	scope, err := engine.EvaluateScope(context.Background(), map[string]any{
		"principal": decimal.NewFromInt(250000),
	})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fee, err := pipe.GetAs[decimal.Decimal](scope, "loan")
	if err != nil {
		fmt.Println("get fee:", err)
		return
	}
	fmt.Printf("fee=%s\n", fee.StringFixed(2))

	// Persist the decision for audit, then reload: the decimal reloads as a
	// decimal (type-tagged Scope JSON, "v":2), not a lossy float.
	blob, err := json.Marshal(scope)
	if err != nil {
		fmt.Println("marshal:", err)
		return
	}
	var reloaded pipe.Scope
	if err := json.Unmarshal(blob, &reloaded); err != nil {
		fmt.Println("unmarshal:", err)
		return
	}
	rfee, err := pipe.GetAs[decimal.Decimal](&reloaded, "loan")
	if err != nil {
		fmt.Println("get reloaded:", err)
		return
	}
	fmt.Printf("reloaded=%s\n", rfee.StringFixed(2))

	// Map into a typed result: the mapper preserves the decimal.
	type Result struct {
		Fee decimal.Decimal `mapstructure:"fee"`
	}
	mapper, err := rlng.NewMapper[Result](rlng.MappingTemplate{"fee": "loan"})
	if err != nil {
		fmt.Println("mapper:", err)
		return
	}
	res, err := mapper.Map(scope.Snapshot())
	if err != nil {
		fmt.Println("map:", err)
		return
	}
	fmt.Printf("mapped=%s\n", res.Fee.StringFixed(2))

	// Output:
	// fee=18125.00
	// reloaded=18125.00
	// mapped=18125.00
}
