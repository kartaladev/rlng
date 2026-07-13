package pipe_test

import (
	"strings"
	"testing"

	"github.com/kartaladev/rlng/pipe"
)

// TestExplain_MemberPathPrecisionEndToEnd proves the B6 win end-to-end: a
// downstream single-expr that reads only grade.tier records the precise
// member-path input, so Explain links to the grade.tier derivation and NOT its
// sibling grade.limit (before B6 the top-level "grade" input fanned out to both).
func TestExplain_MemberPathPrecisionEndToEnd(t *testing.T) {
	grade, err := pipe.NewDecisionTable("grade", []pipe.Rule{{
		Condition: "true",
		Decisions: map[string]pipe.Decision{
			"tier":  {Expr: `"prime"`},
			"limit": {Expr: "score * 2"},
		},
	}})
	if err != nil {
		t.Fatalf("NewDecisionTable: %v", err)
	}
	note, err := pipe.NewSingleExpr("note", `grade.tier + "!"`, pipe.WithDependsOn("grade"))
	if err != nil {
		t.Fatalf("NewSingleExpr: %v", err)
	}
	p, err := pipe.NewPipeline(grade, note)
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	sc := pipe.NewScope(map[string]any{"score": 100}, pipe.WithProvenance())
	if err := p.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The precise input value is recorded at the member path.
	d, ok := sc.Derivation("note")
	if !ok {
		t.Fatal("no derivation for note")
	}
	if _, has := d.Inputs["grade.tier"]; !has {
		t.Fatalf("note inputs missing precise key grade.tier: %v", d.Inputs)
	}
	if _, has := d.Inputs["grade"]; has {
		t.Fatalf("note inputs still carry the coarse top-level key grade: %v", d.Inputs)
	}

	out := sc.Explain("note")
	if !strings.Contains(out, "grade.tier =") {
		t.Fatalf("Explain omits the read output grade.tier:\n%s", out)
	}
	if strings.Contains(out, "grade.limit") {
		t.Fatalf("Explain still fans out to the unread sibling grade.limit:\n%s", out)
	}
}
