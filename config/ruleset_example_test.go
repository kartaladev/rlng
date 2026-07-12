package config_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// ExamplePipelineDef_Hash_replay demonstrates the ruleset-identity flow (spec
// 013 / plan 013 / ADR-0037): a content hash that proves WHAT ran, an author
// version label naming WHICH release it was, and a Scope JSON round-trip that
// carries both plus the firing-rule trail — a self-describing, replayable
// decision record.
func ExamplePipelineDef_Hash_replay() {
	const rulesV1 = `
version: v1.0.0
stages:
  - name: tier
    type: decision-table
    rules:
      - id: GOLD
        condition: "score >= 700"
        decisions:
          label: '"gold"'
      - id: SILVER
        condition: "score >= 500"
        decisions:
          label: '"silver"'
    default:
      label: '"bronze"'
`
	const rulesV2 = `
version: v2.0.0
stages:
  - name: tier
    type: decision-table
    rules:
      - id: GOLD
        condition: "score >= 700"
        decisions:
          label: '"gold"'
      - id: SILVER
        condition: "score >= 500"
        decisions:
          label: '"silver"'
    default:
      label: '"bronze"'
`
	def, err := config.ParseYAML([]byte(rulesV1))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	// Hash() is a pure function of content: stable across repeated calls.
	fmt.Println("hash stable:", def.Hash() == def.Hash())

	// Re-labelling the version (v1.0.0 -> v2.0.0) does not change the content
	// hash: Version is excluded from what Hash() fingerprints.
	d2, err := config.ParseYAML([]byte(rulesV2))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	fmt.Println("version-independent:", def.Hash() == d2.Hash())

	p, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"score": 720})
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}

	id, _ := sc.Ruleset()
	fmt.Println("version:", id.Version)
	if fired := sc.FiringRulesFor("tier"); len(fired) > 0 {
		fmt.Println("rule:", fired[0].RuleID)
	}

	// Persist the decision as JSON — the ruleset stamp and firing trail
	// round-trip alongside the result data.
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
	fmt.Println("reloaded version:", reloadedID.Version)
	if fired := reloaded.FiringRulesFor("tier"); len(fired) > 0 {
		fmt.Println("reloaded rule:", fired[0].RuleID)
	}
	// The reloaded identity still matches the (re-parsed) governing ruleset —
	// the replay-safety check a caller runs before trusting a replay.
	fmt.Println("matches:", def.MatchesRuleset(reloadedID))

	// Output:
	// hash stable: true
	// version-independent: true
	// version: v1.0.0
	// rule: GOLD
	// reloaded version: v1.0.0
	// reloaded rule: GOLD
	// matches: true
}
