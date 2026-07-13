package pipe_test

import (
	"fmt"
	"testing"

	"github.com/kartaladev/rlng/pipe"
)

// flatSeed returns an outer-scope seed with n flat top-level scalar keys
// ("k0".."k{n-1}"), modeling a header of n plain fields. NewScope's cloneValue
// touches each of these once per element (they are scalars, so the clone is a
// shallow top-level copy), and Snapshot copies each per element — this is the
// "outer-scope size" axis of ADR-0040's O(elements x outer-scope size).
func flatSeed(n int) map[string]any {
	m := make(map[string]any, n)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("k%d", i)] = i
	}
	return m
}

// nestedSeed returns an outer-scope seed whose spine has nested map[string]any
// levels (depth ~3), so NewScope's cloneValue recursion is exercised — not just
// the shallow top level a flat seed reaches. This is the "nested" scope shape of
// Spec 018 D2.
func nestedSeed() map[string]any {
	return map[string]any{
		"customer": map[string]any{
			"id": 1,
			"tier": map[string]any{
				"name":   "gold",
				"since":  2020,
				"limits": map[string]any{"credit": 1000, "risk": 3},
			},
		},
		"region": map[string]any{
			"code": "EU",
			"tax":  map[string]any{"vat": 20, "reduced": 5},
		},
		"order": map[string]any{"currency": "USD", "channel": "web"},
	}
}

// BenchmarkForEachScopeCopy measures ForEach.Execute's per-element scope-copy
// cost (ADR-0040's flagged O(elements x outer-scope size)) across the two axes
// of that claim: the collection size (elements) and the outer-scope size/shape
// (scope). The inner pipeline is empty (a valid no-op Run) and no rollups are
// configured, so Execute measures exactly the scope-copy machinery under study —
// Snapshot + element bind + NewScope/cloneValue + per-element Snapshot + append
// + the final Set — with no inner-stage evaluation confounding the numbers
// (Spec 018 D1). Provenance is off (the common path; Spec 018 Non-goals).
//
// The outer scope is built once per sub-benchmark and reused across b.Loop
// iterations. After the first Execute the scope carries the stage's own output
// key ("each.items"), which Set overwrites (never grows) each iteration. So
// from the second iteration on, each of the m per-element Snapshot+NewScope
// copies one extra top-level key — a one-entry map ({"items": …}); the []any
// item list itself is shared, not deep-copied. That adds a small per-element
// constant, proportional to the elements axis (not the scope axis), inflating
// each cell by a small fixed fraction without changing the measured linear
// scaling.
func BenchmarkForEachScopeCopy(b *testing.B) {
	scopes := []struct {
		name string
		seed func() map[string]any
	}{
		{"flat8", func() map[string]any { return flatSeed(8) }},
		{"flat64", func() map[string]any { return flatSeed(64) }},
		{"nested", nestedSeed},
	}
	elementCounts := []int{1, 10, 100, 1000}

	for _, sh := range scopes {
		for _, m := range elementCounts {
			b.Run(fmt.Sprintf("scope=%s/elements=%d", sh.name, m), func(b *testing.B) {
				coll := make([]any, m)
				for i := range coll {
					coll[i] = map[string]any{"amount": i}
				}
				seed := sh.seed()
				seed["lines"] = coll
				sc := pipe.NewScope(seed)

				inner, err := pipe.NewPipeline()
				if err != nil {
					b.Fatalf("build inner pipeline: %v", err)
				}
				f, err := pipe.NewForEach("each", "lines", inner)
				if err != nil {
					b.Fatalf("build foreach: %v", err)
				}

				ctx := b.Context()
				b.ReportAllocs()
				for b.Loop() {
					if err := f.Execute(ctx, sc); err != nil {
						b.Fatalf("execute: %v", err)
					}
				}
			})
		}
	}
}
