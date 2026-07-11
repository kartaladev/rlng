package stage

import (
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
