package pipe_test

import (
	"strings"
	"testing"

	"github.com/kartaladev/rlng/pipe"
)

// TestExplain_MemberPathInputLinksToSeedAncestor drives the nearest-ancestor
// fallback in derivationsFor: a Derivation input keyed by a member path
// (applicant.score) whose value lives under a top-level seed (applicant)
// reconciles up to that seed, so Explain renders the seed subtree.
func TestExplain_MemberPathInputLinksToSeedAncestor(t *testing.T) {
	sc := pipe.NewScope(map[string]any{"applicant": map[string]any{"score": 700}}, pipe.WithProvenance())
	if err := sc.Derive("decision.ok", true, pipe.Derivation{
		Stage:      "decision",
		StageType:  pipe.TypeSingleExpr,
		Operation:  "eval",
		Expression: "applicant.score >= 650",
		Inputs:     map[string]any{"applicant.score": 700},
	}); err != nil {
		t.Fatalf("Derive: %v", err)
	}
	out := sc.Explain("decision.ok")
	if !strings.Contains(out, "applicant =") || !strings.Contains(out, "[seed]") {
		t.Fatalf("Explain did not link member-path input to the seed ancestor:\n%s", out)
	}
}
