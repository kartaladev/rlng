package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionReferences(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		expr string
		want []string
	}

	cases := []testCase{
		{name: "identifiers deduped and sorted", expr: "price * qty + price", want: []string{"price", "qty"}},
		{name: "member access uses top-level", expr: "tiers.tag + base", want: []string{"base", "tiers"}},
		{name: "literal only has no refs", expr: "1 + 2", want: nil},
		{name: "call callee is not a data reference", expr: "discount(price) + len(items)", want: []string{"items", "price"}},
		{name: "expression of only a callee and literal has no data refs", expr: "discount(1)", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := NewFunction("f", tc.expr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, f.References())
			assert.Equal(t, tc.expr, f.Source())
		})
	}
}

func TestPredicateReferences(t *testing.T) {
	t.Parallel()
	p, err := NewPredicate("amount > threshold")
	require.NoError(t, err)
	assert.Equal(t, []string{"amount", "threshold"}, p.References())
	assert.Equal(t, "amount > threshold", p.Source())
}

func BenchmarkFunctionReferences(b *testing.B) {
	f, err := NewFunction("f", "price * qty + tiers.tag")
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.References()
	}
}
