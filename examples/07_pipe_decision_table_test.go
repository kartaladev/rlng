// 07 — pipe decision tables: an ordered set of rules, each a boolean
// condition plus a set of named decisions, evaluated against a Scope
// snapshot. The HitPolicy controls how matches are resolved: HitPolicySingle
// (first match wins, the default), HitPolicyCollect (every match
// contributes, reduced per an aggregation), HitPolicyUnique (at most one
// match is allowed), and HitPolicyAny (several matches are allowed but must
// agree). WithDefault supplies decisions for the "nothing matched" case, so
// "no match" is an explicit, named outcome rather than a silently-missing
// output.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

// Example_decisionTableCreditTierSingle shows the default hit policy,
// HitPolicySingle: rules are tried in order and the first match wins. Each
// rule's optional ID and Message make the outcome explainable — after Run,
// Scope.FiringRule reports exactly which rule (or, absent a match, the
// WithDefault decisions) produced the result, the audit trail a compliance
// review needs. WithDefault turns "no rule matched" into an explicit decline
// rather than an absent output path.
func Example_decisionTableCreditTierSingle() {
	grade, err := pipe.NewDecisionTable("grade", []pipe.Rule{
		{
			ID:        "PRIME",
			Message:   "score at or above the prime threshold",
			Condition: "score >= 750",
			Decisions: map[string]pipe.Decision{
				"tier":  {Expr: `"prime"`},
				"limit": {Expr: "score * 100"},
			},
		},
		{
			ID:        "NEAR_PRIME",
			Message:   "score in the near-prime band",
			Condition: "score >= 650",
			Decisions: map[string]pipe.Decision{
				"tier":  {Expr: `"near_prime"`},
				"limit": {Expr: "score * 50"},
			},
		},
	}, pipe.WithDefault(map[string]pipe.Decision{
		"tier":  {Expr: `"declined"`},
		"limit": {Expr: "0"},
	}))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	pipeline, err := pipe.NewPipeline([]pipe.Stage{grade})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	prime := pipe.NewScope(map[string]any{"score": 780})
	_ = pipeline.Run(context.Background(), prime)
	tier, _ := prime.GetString("grade.tier")
	fr, _ := prime.FiringRule("grade")
	fmt.Printf("score 780: tier=%s rule=%s message=%q default=%v\n", tier, fr.RuleID, fr.Message, fr.IsDefault)

	declined := pipe.NewScope(map[string]any{"score": 500})
	_ = pipeline.Run(context.Background(), declined)
	tier2, _ := declined.GetString("grade.tier")
	fr2, _ := declined.FiringRule("grade")
	fmt.Printf("score 500: tier=%s default=%v\n", tier2, fr2.IsDefault)

	// Output:
	// score 780: tier=prime rule=PRIME message="score at or above the prime threshold" default=false
	// score 500: tier=declined default=true
}

// Example_decisionTableCollectFeesAndFlags shows HitPolicyCollect: every
// matching rule contributes, not just the first. Two aggregation shapes: fees
// use AggregateSum, and mixing an int-valued fee with a decimal-valued fee
// shows widest-kind promotion — the sum promotes to decimal.Decimal rather
// than losing the decimal fee's exactness by folding it into a plain int or a
// lossy float. Adverse-action flags use the default aggregation
// (AggregateList), collecting every matched flag into a []any (read with
// GetSlice). When NO adverse-action rule matches, the collect path never
// materializes an empty list — the table's WithDefault decisions fire
// instead, so "nothing to flag" is an explicit "clean" outcome, not a
// silently-absent path.
func Example_decisionTableCollectFeesAndFlags() {
	fees, err := pipe.NewDecisionTable("fees", []pipe.Rule{
		{Condition: "wire", Decisions: map[string]pipe.Decision{"amount": {Expr: "25"}}},
		{Condition: "rush", Decisions: map[string]pipe.Decision{"amount": {Expr: "15"}}},
		{Condition: "international", Decisions: map[string]pipe.Decision{"amount": {Expr: "decimal(30.5)"}}},
	}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	adverseAction, err := pipe.NewDecisionTable("adverseAction", []pipe.Rule{
		{Condition: "score < 620", Decisions: map[string]pipe.Decision{"flag": {Expr: `"low_credit_score"`}}},
		{Condition: "dti > 0.45", Decisions: map[string]pipe.Decision{"flag": {Expr: `"high_debt_to_income"`}}},
	}, pipe.WithHitPolicy(pipe.HitPolicyCollect),
		pipe.WithDefault(map[string]pipe.Decision{"flag": {Expr: `"no_adverse_action"`}}))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}

	pipeline, err := pipe.NewPipeline([]pipe.Stage{fees, adverseAction})
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	// A wire, international transfer from a marginal-score applicant: two fee
	// rules apply (one int, one decimal — promoting the sum to decimal), and one
	// adverse-action flag applies.
	flagged := pipe.NewScope(map[string]any{
		"wire": true, "rush": false, "international": true,
		"score": 600, "dti": 0.3,
	})
	_ = pipeline.Run(context.Background(), flagged)
	total, _ := flagged.Get("fees.amount")
	matchedFlags, _ := flagged.GetSlice("adverseAction.flag")
	fmt.Println("fee total (int + decimal -> decimal):", total)
	fmt.Println("flags:", matchedFlags)

	// A clean applicant: no adverse-action rule matches, so the default fires —
	// a single explicit "clean" value, never an empty collected list.
	clean := pipe.NewScope(map[string]any{
		"wire": false, "rush": false, "international": false,
		"score": 720, "dti": 0.2,
	})
	_ = pipeline.Run(context.Background(), clean)
	cleanFlag, _ := clean.Get("adverseAction.flag")
	fr, _ := clean.FiringRule("adverseAction")
	fmt.Println("no rules matched -> default flag:", cleanFlag, "isDefault:", fr.IsDefault)

	// Output:
	// fee total (int + decimal -> decimal): 55.5
	// flags: [low_credit_score]
	// no rules matched -> default flag: no_adverse_action isDefault: true
}

// Example_decisionTableUniqueAndAny contrasts the two "must not silently pick
// a winner" hit policies. HitPolicyUnique guards a table meant to be mutually
// exclusive: overlapping rules are a config bug, not a "first match wins"
// surprise — they surface loudly as ErrMultipleMatches (wrapped in a
// *pipe.StageError; unwrap with errors.Is) instead of silently choosing one.
// HitPolicyAny allows several rules to match as long as they AGREE on every
// shared output key — useful when independent risk checks should reinforce
// each other's conclusion rather than race for it; a disagreement surfaces as
// ErrConflictingMatches, also via errors.Is on the StageError cause.
func Example_decisionTableUniqueAndAny() {
	uniqueTier, err := pipe.NewDecisionTable("tier", []pipe.Rule{
		{Condition: "score >= 700", Decisions: map[string]pipe.Decision{"tier": {Expr: `"prime"`}}},
		{Condition: "score >= 650", Decisions: map[string]pipe.Decision{"tier": {Expr: `"near_prime"`}}},
	}, pipe.WithHitPolicy(pipe.HitPolicyUnique))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	uniquePipeline, err := pipe.NewPipeline([]pipe.Stage{uniqueTier})
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	overlap := pipe.NewScope(map[string]any{"score": 720}) // matches BOTH rules
	err = uniquePipeline.Run(context.Background(), overlap)
	fmt.Println("overlapping unique rules rejected:", errors.Is(err, pipe.ErrMultipleMatches))

	agreeing, err := pipe.NewDecisionTable("consensus", []pipe.Rule{
		{Condition: "score >= 700", Decisions: map[string]pipe.Decision{"tier": {Expr: `"prime"`}}},
		{Condition: "utilization < 0.3", Decisions: map[string]pipe.Decision{"tier": {Expr: `"prime"`}}},
	}, pipe.WithHitPolicy(pipe.HitPolicyAny))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	agreePipeline, err := pipe.NewPipeline([]pipe.Stage{agreeing})
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	agreeScope := pipe.NewScope(map[string]any{"score": 720, "utilization": 0.2})
	_ = agreePipeline.Run(context.Background(), agreeScope)
	tier, _ := agreeScope.GetString("consensus.tier")
	fmt.Println("agreeing rules -> tier:", tier)

	conflicting, err := pipe.NewDecisionTable("conflict", []pipe.Rule{
		{Condition: "score >= 700", Decisions: map[string]pipe.Decision{"tier": {Expr: `"prime"`}}},
		{Condition: "utilization > 0.3", Decisions: map[string]pipe.Decision{"tier": {Expr: `"near_prime"`}}},
	}, pipe.WithHitPolicy(pipe.HitPolicyAny))
	if err != nil {
		fmt.Println("compile:", err)
		return
	}
	conflictPipeline, err := pipe.NewPipeline([]pipe.Stage{conflicting})
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	conflictScope := pipe.NewScope(map[string]any{"score": 720, "utilization": 0.5})
	err = conflictPipeline.Run(context.Background(), conflictScope)
	fmt.Println("conflicting rules rejected:", errors.Is(err, pipe.ErrConflictingMatches))

	// Output:
	// overlapping unique rules rejected: true
	// agreeing rules -> tier: prime
	// conflicting rules rejected: true
}
