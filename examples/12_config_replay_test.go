// 12 — config ruleset identity & replay: a PipelineDef's Hash() is a
// deterministic content fingerprint, and Build stamps it (with the author's
// Version label) onto every Scope a built Pipeline runs — so a decision
// carries proof of exactly which ruleset produced it. A Scope round-trips
// through JSON without losing that stamp, so a persisted decision can be
// reloaded and checked against a candidate ruleset before anyone trusts it
// as a replay.
package examples_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// Example_rulesetHashStableAndVersionIndependent shows what Hash() does and
// does not fingerprint. It is a pure function of content — calling it twice
// on the same *PipelineDef returns the same value — and it deliberately
// EXCLUDES the author-declared version: label: relabeling a release
// (2026.1 -> 2026.2) with no rule change leaves the hash unchanged, because
// Hash answers "what will this ruleset decide", not "which release is this".
func Example_rulesetHashStableAndVersionIndependent() {
	const riskV1 = `
version: 2026.1
stages:
  - name: risk
    type: decision-table
    rules:
      - id: LOW
        condition: "claimsScore >= 80"
        decisions:
          tier: '"low"'
      - id: MEDIUM
        condition: "claimsScore >= 50"
        decisions:
          tier: '"medium"'
    default:
      tier: '"high"'
`
	const riskV2 = `
version: 2026.2
stages:
  - name: risk
    type: decision-table
    rules:
      - id: LOW
        condition: "claimsScore >= 80"
        decisions:
          tier: '"low"'
      - id: MEDIUM
        condition: "claimsScore >= 50"
        decisions:
          tier: '"medium"'
    default:
      tier: '"high"'
`
	v1, err := config.Parse(context.Background(), config.FromYAMLString(riskV1))
	if err != nil {
		fmt.Println("parse v1:", err)
		return
	}
	v2, err := config.Parse(context.Background(), config.FromYAMLString(riskV2))
	if err != nil {
		fmt.Println("parse v2:", err)
		return
	}

	fmt.Println("hash stable across repeated calls:", v1.Hash() == v1.Hash())
	fmt.Println("relabeling version leaves hash unchanged:", v1.Hash() == v2.Hash())

	// Output:
	// hash stable across repeated calls: true
	// relabeling version leaves hash unchanged: true
}

// Example_rulesetReplaySafety runs a risk-tiering ruleset, persists the
// resulting Scope as JSON (the form a decision record is archived in), and
// reloads it. The reloaded Scope still carries the ruleset identity Build
// stamped during Run (Scope.Ruleset) and the rule that fired
// (FiringRulesFor) — a self-describing decision record. MatchesRuleset then
// answers the replay-safety question a caller must ask before trusting a
// candidate ruleset against that record: does IT actually match the ruleset
// that produced this decision? The governing definition matches; an amended
// definition (a raised claims-score threshold) does not — exactly the check
// that catches "replaying against the wrong ruleset version" before it
// silently produces a different answer.
func Example_rulesetReplaySafety() {
	const riskRules = `
version: 2026.1
stages:
  - name: risk
    type: decision-table
    rules:
      - id: LOW
        condition: "claimsScore >= 80"
        decisions:
          tier: '"low"'
      - id: MEDIUM
        condition: "claimsScore >= 50"
        decisions:
          tier: '"medium"'
    default:
      tier: '"high"'
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(riskRules))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"claimsScore": 85})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}

	id, _ := sc.Ruleset()
	fmt.Println("stamped version:", id.Version)
	if fired := sc.FiringRulesFor("risk"); len(fired) > 0 {
		fmt.Println("fired rule:", fired[0].RuleID)
	}

	// Persist the decision, then reload it as if it had been archived and
	// retrieved later — the ruleset stamp and firing trail travel with the data.
	b, err := json.Marshal(sc)
	if err != nil {
		fmt.Println("marshal:", err)
		return
	}
	reloaded := &pipe.Scope{}
	if err := json.Unmarshal(b, reloaded); err != nil {
		fmt.Println("unmarshal:", err)
		return
	}
	reloadedID, _ := reloaded.Ruleset()

	// The governing definition matches the persisted decision's stamp.
	fmt.Println("governing ruleset matches:", def.MatchesRuleset(reloadedID))

	// An amended ruleset (the LOW threshold raised from 80 to 90) is a
	// DIFFERENT ruleset — its content hash differs, so it correctly fails the
	// replay-safety check against this persisted decision.
	const amendedRules = `
version: 2026.1
stages:
  - name: risk
    type: decision-table
    rules:
      - id: LOW
        condition: "claimsScore >= 90"
        decisions:
          tier: '"low"'
      - id: MEDIUM
        condition: "claimsScore >= 50"
        decisions:
          tier: '"medium"'
    default:
      tier: '"high"'
`
	amendedDef, err := config.Parse(context.Background(), config.FromYAMLString(amendedRules))
	if err != nil {
		fmt.Println("parse amended:", err)
		return
	}
	fmt.Println("amended ruleset rejected:", !amendedDef.MatchesRuleset(reloadedID))

	// Output:
	// stamped version: 2026.1
	// fired rule: LOW
	// governing ruleset matches: true
	// amended ruleset rejected: true
}
