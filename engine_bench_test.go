package rlng

import (
	"testing"

	"github.com/kartaladev/rlng/stage"
)

// BenchmarkEngineEvaluate measures Evaluate with provenance disabled — the
// default, zero-added-cost path.
func BenchmarkEngineEvaluate(b *testing.B) {
	e := buildEngine(b)
	ctx := b.Context()
	in := order{Price: 10, Qty: 2}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEngineEvaluateProvenance measures Evaluate with WithProvenance
// enabled on the Engine's Scope, for comparison against the default path.
func BenchmarkEngineEvaluateProvenance(b *testing.B) {
	e := buildEngine(b, WithScopeOptions(stage.WithProvenance()))
	ctx := b.Context()
	in := order{Price: 10, Qty: 2}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}
