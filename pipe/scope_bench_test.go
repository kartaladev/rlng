package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
)

// BenchmarkScopeSet measures a bare Set on a Scope created without
// WithProvenance — the baseline write path.
func BenchmarkScopeSet(b *testing.B) {
	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.Set("total", 20)
	}
}

// BenchmarkScopeDeriveOff measures Derive on a Scope created WITHOUT
// WithProvenance. Derive must fall back to exactly Set in this case, so its
// allocs/op must match BenchmarkScopeSet — proving the provenance-off path
// adds no cost.
func BenchmarkScopeDeriveOff(b *testing.B) {
	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2})
	d := pipe.Derivation{Stage: "total", StageType: pipe.TypeSingleExpr, Operation: "eval", Expression: "price * qty"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.Derive("total", 20, d)
	}
}

// BenchmarkScopeDeriveOn measures Derive on a Scope created WITH
// WithProvenance, where each call also records a Derivation.
func BenchmarkScopeDeriveOn(b *testing.B) {
	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2}, pipe.WithProvenance())
	d := pipe.Derivation{Stage: "total", StageType: pipe.TypeSingleExpr, Operation: "eval", Expression: "price * qty"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.Derive("total", 20, d)
	}
}

// BenchmarkExplain measures rendering the derivation tree for a value with one
// upstream input, on a provenance-enabled Scope.
func BenchmarkExplain(b *testing.B) {
	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2}, pipe.WithProvenance())
	baseDerivation := pipe.Derivation{
		Stage: "base", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "price * qty", Inputs: map[string]any{"price": 10, "qty": 2},
	}
	_ = sc.Derive("base", 20, baseDerivation)
	taxedDerivation := pipe.Derivation{
		Stage: "taxed", StageType: pipe.TypeSingleExpr, Operation: "eval",
		Expression: "base * 1.1", Inputs: map[string]any{"base": 20},
	}
	_ = sc.Derive("taxed", 22, taxedDerivation)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.Explain("taxed")
	}
}
