package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// Example_lendingDecision shows a full business-rule decision authored as one
// YAML document: a pipeline-level constant threshold, an identified decision
// rule with a message, and an explicit default (else) branch. After running, the
// firing rule explains *why* the applicant got their tier — the audit trail a
// compliance review needs.
func Example_lendingDecision() {
	const rules = `
constants:
  primeMin: 750
stages:
  - name: grade
    type: decision-table
    rules:
      - id: PRIME
        message: score at or above prime threshold
        condition: score >= primeMin
        decisions:
          tier: '"prime"'
          limit: score * 100
    default:
      tier: '"declined"'
      limit: '0'
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(rules))
	if err != nil {
		panic(err)
	}

	// Lint the ruleset before running it (a catch-all/default safety check).
	fmt.Println("lint findings:", len(def.Lint()))

	pipeline, err := def.Build()
	if err != nil {
		panic(err)
	}

	// A declined applicant: below the prime threshold, so the default fires.
	sc := pipe.NewScope(map[string]any{"score": 620})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		panic(err)
	}

	tier, _ := sc.Get("grade.tier")
	fr, _ := sc.FiringRule("grade")
	fmt.Printf("tier: %v\n", tier)
	fmt.Printf("default fired: %v\n", fr.IsDefault)

	// Output:
	// lint findings: 0
	// tier: declined
	// default fired: true
}

// Example_feeAggregation shows a collect table reducing several matching rules
// into a single number — "sum of all applicable fees" — a common pricing shape.
func Example_feeAggregation() {
	const rules = `
stages:
  - name: fees
    type: decision-table
    hit_policy: collect
    aggregation: sum
    rules:
      - condition: wire
        decisions: {fee: '25'}
      - condition: rush
        decisions: {fee: '15'}
      - condition: international
        decisions: {fee: '30'}
`
	def, _ := config.Parse(context.Background(), config.FromYAMLString(rules))
	pipeline, _ := def.Build()

	sc := pipe.NewScope(map[string]any{"wire": true, "rush": false, "international": true})
	_ = pipeline.Run(context.Background(), sc)

	fee, _ := sc.Get("fees.fee")
	fmt.Printf("total fee: %v\n", fee)

	// Output:
	// total fee: 55
}
