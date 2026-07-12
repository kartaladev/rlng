package rlng_test

import (
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
)

// BenchmarkTypedEngineEvaluate measures Evaluate with provenance disabled — the
// default, zero-added-cost path.
func BenchmarkTypedEngineEvaluate(b *testing.B) {
	e := buildTypedEngine(b)
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

// BenchmarkTypedEngineEvaluateProvenance measures Evaluate with WithProvenance
// enabled on the TypedEngine's Scope, for comparison against the default path.
func BenchmarkTypedEngineEvaluateProvenance(b *testing.B) {
	e := buildTypedEngine(b, rlng.WithScopeOptions(pipe.WithProvenance()))
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
