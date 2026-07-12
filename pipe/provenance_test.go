package pipe_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeProvenanceDisabled(t *testing.T) {
	t.Parallel()

	sc := pipe.NewScope(map[string]any{"price": 10})
	assert.False(t, sc.TracksProvenance())

	// Derive behaves exactly like Set when provenance is off.
	require.NoError(t, sc.Derive("base", 20, pipe.Derivation{Stage: "base", Operation: "eval"}))
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

	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2}, pipe.WithProvenance())
	assert.True(t, sc.TracksProvenance())

	d, ok := sc.Derivation("price")
	require.True(t, ok)
	assert.Equal(t, "price", d.Path)
	assert.Equal(t, "seed", d.StageType)
	assert.Equal(t, "seed", d.Operation)
	assert.Equal(t, 10, d.Value)
	assert.Nil(t, d.Inputs)
}

func TestScopeProvenanceDeriveAndExplain(t *testing.T) {
	t.Parallel()

	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2}, pipe.WithProvenance())

	require.NoError(t, sc.Derive("base", 20, pipe.Derivation{
		Stage: "base", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "price * qty", Inputs: map[string]any{"price": 10, "qty": 2},
	}))
	require.NoError(t, sc.Derive("taxed", 22, pipe.Derivation{
		Stage: "taxed", StageType: pipe.TypeSingleExpr, Operation: "eval",
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

	sc := pipe.NewScope(map[string]any{"amount": 150}, pipe.WithProvenance())

	// A decision-table-style write under the "tiers" namespace...
	require.NoError(t, sc.Derive("tiers.tag", "big", pipe.Derivation{
		Stage: "tiers", StageType: pipe.TypeDecisionTable, Operation: "decision:tag",
		Expression: `"big"`, Inputs: map[string]any{"amount": 150},
	}))
	// ...and a later stage that reads the whole "tiers" namespace as an input.
	require.NoError(t, sc.Derive("out", "big!", pipe.Derivation{
		Stage: "out", StageType: pipe.TypeSingleExpr, Operation: "eval",
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
	sc := pipe.NewScope(map[string]any{"amount": 150}, pipe.WithProvenance())
	require.NoError(t, sc.Derive("tiers.b", "second", pipe.Derivation{
		Stage: "tiers", StageType: pipe.TypeDecisionTable, Operation: "decision:b",
		Expression: `"second"`, Inputs: map[string]any{"amount": 150},
	}))
	require.NoError(t, sc.Derive("tiers.a", "first", pipe.Derivation{
		Stage: "tiers", StageType: pipe.TypeDecisionTable, Operation: "decision:a",
		Expression: `"first"`, Inputs: map[string]any{"amount": 150},
	}))
	require.NoError(t, sc.Derive("out", "x", pipe.Derivation{
		Stage: "out", StageType: pipe.TypeSingleExpr, Operation: "eval",
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

	sc := pipe.NewScope(map[string]any{"price": 10}, pipe.WithProvenance())
	require.NoError(t, sc.Derive("base", 20, pipe.Derivation{
		Stage: "base", StageType: pipe.TypeSingleExpr, Operation: "eval",
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
	sc := pipe.NewScope(map[string]any{"x": 2}, pipe.WithProvenance())
	require.NoError(t, sc.Derive("mid", 4, pipe.Derivation{
		Stage: "mid", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "x * 2", Inputs: map[string]any{"x": 2},
	}))
	require.NoError(t, sc.Derive("a", 5, pipe.Derivation{
		Stage: "a", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "mid + 1", Inputs: map[string]any{"mid": 4},
	}))
	require.NoError(t, sc.Derive("b", 6, pipe.Derivation{
		Stage: "b", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "mid + 2", Inputs: map[string]any{"mid": 4},
	}))
	require.NoError(t, sc.Derive("c", 11, pipe.Derivation{
		Stage: "c", StageType: pipe.TypeSingleExpr, Operation: "eval",
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

func TestScopeLineageDepthCapped(t *testing.T) {
	t.Parallel()
	sc := pipe.NewScope(map[string]any{}, pipe.WithProvenance())
	// chain: n0 <- n1 <- ... each derivation's input references the next path.
	const chain = pipe.MaxLineageDepth + 50
	for i := 0; i < chain; i++ {
		next := fmt.Sprintf("n%d", i+1)
		require.NoError(t, sc.Derive(fmt.Sprintf("n%d", i), i, pipe.Derivation{
			Stage: "s", StageType: pipe.TypeSingleExpr, Operation: "eval",
			Expression: "x", Inputs: map[string]any{next: 0},
		}))
	}
	// Must return (not stack-overflow) and be bounded by the depth cap.
	lin := sc.Lineage("n0")
	assert.LessOrEqual(t, len(lin), pipe.MaxLineageDepth)
	// Explain terminates and marks the truncation point rather than dropping it silently.
	explained := sc.Explain("n0")
	assert.NotEmpty(t, explained)
	assert.Contains(t, explained, "truncated: max lineage depth")
}
