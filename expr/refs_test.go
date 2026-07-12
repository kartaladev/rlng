package expr_test

import (
	"github.com/kartaladev/rlng/expr"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionReferences(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		expr   string
		assert func(t *testing.T, refs []string)
	}

	cases := []testCase{
		{
			name: "identifiers deduped and sorted",
			expr: "price * qty + price",
			assert: func(t *testing.T, refs []string) {
				assert.Equal(t, []string{"price", "qty"}, refs)
			},
		},
		{
			name: "member access uses top-level",
			expr: "tiers.tag + base",
			assert: func(t *testing.T, refs []string) {
				assert.Equal(t, []string{"base", "tiers"}, refs)
			},
		},
		{
			name: "literal only has no refs",
			expr: "1 + 2",
			assert: func(t *testing.T, refs []string) {
				assert.Nil(t, refs)
			},
		},
		{
			name: "call callee is not a data reference",
			expr: "discount(price) + len(items)",
			assert: func(t *testing.T, refs []string) {
				assert.Equal(t, []string{"items", "price"}, refs)
			},
		},
		{
			name: "expression of only a callee and literal has no data refs",
			expr: "discount(1)",
			assert: func(t *testing.T, refs []string) {
				assert.Nil(t, refs)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := expr.NewFunction("f", tc.expr)
			require.NoError(t, err)
			tc.assert(t, f.References())
			assert.Equal(t, tc.expr, f.Source())
		})
	}
}

func TestPredicateReferences(t *testing.T) {
	t.Parallel()
	p, err := expr.NewPredicate("amount > threshold")
	require.NoError(t, err)
	assert.Equal(t, []string{"amount", "threshold"}, p.References())
	assert.Equal(t, "amount > threshold", p.Source())
}

func BenchmarkFunctionReferences(b *testing.B) {
	f, err := expr.NewFunction("f", "price * qty + tiers.tag")
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.References()
	}
}
