package stage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeProvenanceDisabled(t *testing.T) {
	t.Parallel()

	sc := NewScope(map[string]any{"price": 10})
	assert.False(t, sc.TracksProvenance())

	// Derive behaves exactly like Set when provenance is off.
	require.NoError(t, sc.Derive("base", 20, Derivation{Stage: "base", Operation: "eval"}))
	v, ok := sc.Get("base")
	require.True(t, ok)
	assert.Equal(t, 20, v)

	_, ok = sc.Derivation("base")
	assert.False(t, ok)
	assert.Empty(t, sc.Lineage("base"))
	assert.Empty(t, sc.Explain("base"))
	assert.Empty(t, sc.Derivations())
}

func TestScopeProvenanceSeed(t *testing.T) {
	t.Parallel()

	sc := NewScope(map[string]any{"price": 10, "qty": 2}, WithProvenance())
	assert.True(t, sc.TracksProvenance())

	d, ok := sc.Derivation("price")
	require.True(t, ok)
	assert.Equal(t, "price", d.Path)
	assert.Equal(t, seedStageType, d.StageType)
	assert.Equal(t, "seed", d.Operation)
	assert.Equal(t, 10, d.Value)
	assert.Nil(t, d.Inputs)
}

func TestScopeProvenanceDeriveAndExplain(t *testing.T) {
	t.Parallel()

	sc := NewScope(map[string]any{"price": 10, "qty": 2}, WithProvenance())

	require.NoError(t, sc.Derive("base", 20, Derivation{
		Stage: "base", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "price * qty", Inputs: map[string]any{"price": 10, "qty": 2},
	}))
	require.NoError(t, sc.Derive("taxed", 22, Derivation{
		Stage: "taxed", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "base * 1.1", Inputs: map[string]any{"base": 20},
	}))

	d, ok := sc.Derivation("taxed")
	require.True(t, ok)
	assert.Equal(t, "base * 1.1", d.Expression)
	assert.Equal(t, map[string]any{"base": 20}, d.Inputs)

	// Lineage is seeds-first: price, qty, base, taxed.
	lin := sc.Lineage("taxed")
	paths := make([]string, len(lin))
	for i, d := range lin {
		paths[i] = d.Path
	}
	assert.Equal(t, []string{"price", "qty", "base", "taxed"}, paths)

	want := "" +
		"taxed = 22 [taxed single-expr] expr: base * 1.1\n" +
		"  base = 20 [base single-expr] expr: price * qty\n" +
		"    price = 10 [seed]\n" +
		"    qty = 2 [seed]\n"
	assert.Equal(t, want, sc.Explain("taxed"))

	assert.Empty(t, sc.Explain("nonexistent"))
}

func TestScopeProvenanceNamespacedReconciliation(t *testing.T) {
	t.Parallel()

	sc := NewScope(map[string]any{"amount": 150}, WithProvenance())

	// A decision-table-style write under the "tiers" namespace...
	require.NoError(t, sc.Derive("tiers.tag", "big", Derivation{
		Stage: "tiers", StageType: TypeDecisionTable, Operation: "decision:tag",
		Expression: `"big"`, Inputs: map[string]any{"amount": 150},
	}))
	// ...and a later stage that reads the whole "tiers" namespace as an input.
	require.NoError(t, sc.Derive("out", "big!", Derivation{
		Stage: "out", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "tiers.tag + \"!\"", Inputs: map[string]any{"tiers": map[string]any{"tag": "big"}},
	}))

	// Lineage of "out" reconciles the "tiers" input to the "tiers.tag" derivation.
	lin := sc.Lineage("out")
	paths := make([]string, len(lin))
	for i, d := range lin {
		paths[i] = d.Path
	}
	assert.Equal(t, []string{"amount", "tiers.tag", "out"}, paths)
}

func TestScopeLineageNamespaceMultipleEntries(t *testing.T) {
	t.Parallel()

	// Two values written under the "tiers" namespace, then read as a whole by a
	// later stage. derivationsFor("tiers") must sort the two matches by path.
	sc := NewScope(map[string]any{"amount": 150}, WithProvenance())
	require.NoError(t, sc.Derive("tiers.b", "second", Derivation{
		Stage: "tiers", StageType: TypeDecisionTable, Operation: "decision:b",
		Expression: `"second"`, Inputs: map[string]any{"amount": 150},
	}))
	require.NoError(t, sc.Derive("tiers.a", "first", Derivation{
		Stage: "tiers", StageType: TypeDecisionTable, Operation: "decision:a",
		Expression: `"first"`, Inputs: map[string]any{"amount": 150},
	}))
	require.NoError(t, sc.Derive("out", "x", Derivation{
		Stage: "out", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "tiers.a + tiers.b", Inputs: map[string]any{"tiers": map[string]any{"a": "first", "b": "second"}},
	}))

	lin := sc.Lineage("out")
	paths := make([]string, len(lin))
	for i, d := range lin {
		paths[i] = d.Path
	}
	// Seeds-first, and the two "tiers.*" entries appear in sorted (a before b) order.
	assert.Equal(t, []string{"amount", "tiers.a", "tiers.b", "out"}, paths)
}

func TestScopeDerivationsCopy(t *testing.T) {
	t.Parallel()

	sc := NewScope(map[string]any{"price": 10}, WithProvenance())
	require.NoError(t, sc.Derive("base", 20, Derivation{
		Stage: "base", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "price * 2", Inputs: map[string]any{"price": 10},
	}))

	got := sc.Derivations()
	require.Contains(t, got, "price")
	require.Contains(t, got, "base")
	assert.Equal(t, 20, got["base"].Value)

	// The returned map is a copy: mutating it must not affect the Scope.
	delete(got, "base")
	_, ok := sc.Derivation("base")
	assert.True(t, ok, "Derivations must return a copy, not the live map")
}

func TestScopeLineageAndExplainDiamond(t *testing.T) {
	t.Parallel()

	// x → mid → {a, b} → c. "mid" is a shared upstream of both a and b, so the
	// seeds-first walk must visit it exactly once (dedup guards in
	// collectLineage and explain).
	sc := NewScope(map[string]any{"x": 2}, WithProvenance())
	require.NoError(t, sc.Derive("mid", 4, Derivation{
		Stage: "mid", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "x * 2", Inputs: map[string]any{"x": 2},
	}))
	require.NoError(t, sc.Derive("a", 5, Derivation{
		Stage: "a", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "mid + 1", Inputs: map[string]any{"mid": 4},
	}))
	require.NoError(t, sc.Derive("b", 6, Derivation{
		Stage: "b", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "mid + 2", Inputs: map[string]any{"mid": 4},
	}))
	require.NoError(t, sc.Derive("c", 11, Derivation{
		Stage: "c", StageType: TypeSingleExpr, Operation: "eval",
		Expression: "a + b", Inputs: map[string]any{"a": 5, "b": 6},
	}))

	// Lineage is seeds-first with each node once, even though "mid" (and "x")
	// is reachable through both a and b.
	lin := sc.Lineage("c")
	paths := make([]string, len(lin))
	for i, d := range lin {
		paths[i] = d.Path
	}
	assert.Equal(t, []string{"x", "mid", "a", "b", "c"}, paths)

	// Explain does not re-expand a shared subtree: "x" appears once.
	explained := sc.Explain("c")
	assert.Equal(t, 1, strings.Count(explained, "x = 2 [seed]"))
}
