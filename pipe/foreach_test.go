package pipe_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cancelHolder carries a context.CancelFunc from a table case's ctx modifier
// (which runs after build, before Execute) to a probe stage built earlier in
// build, so a mid-iteration cancellation can be triggered deterministically
// from inside the inner pipeline rather than by racing real time.
type cancelHolder struct {
	cancel context.CancelFunc
}

// cancelAfterFirst is a test-only Stage that cancels ctx (via holder) the
// first time it runs, so the *second* ForEach iteration observes a canceled
// context at its pre-element ctx.Err() check.
type cancelAfterFirst struct {
	holder *cancelHolder
	calls  int
}

func (c *cancelAfterFirst) Name() string        { return "cancel-probe" }
func (c *cancelAfterFirst) Type() string        { return "cancel-probe" }
func (c *cancelAfterFirst) DependsOn() []string { return nil }

func (c *cancelAfterFirst) Execute(_ context.Context, _ *pipe.Scope) error {
	c.calls++
	if c.calls == 1 {
		c.holder.cancel()
	}
	return nil
}

// errInjectedInner is the sentinel cause returned by errAfterFirst, so tests
// can assert errors.Is on the failure that propagates up through nested
// foreach wrapping.
var errInjectedInner = errors.New("injected inner failure")

// errAfterFirst is a test-only Stage that succeeds the first time it runs and
// fails with errInjectedInner every call after, so it can be placed as the
// sole stage of an inner-most pipeline to fail a chosen per-element iteration
// (e.g. the second inner element) deterministically.
type errAfterFirst struct {
	calls int
}

func (e *errAfterFirst) Name() string        { return "err-probe" }
func (e *errAfterFirst) Type() string        { return "err-probe" }
func (e *errAfterFirst) DependsOn() []string { return nil }

func (e *errAfterFirst) Execute(_ context.Context, _ *pipe.Scope) error {
	e.calls++
	if e.calls > 1 {
		return errInjectedInner
	}
	return nil
}

// provenanceProbe is a test-only Stage that records, into got, whether the
// Scope it runs against tracks provenance — used to verify ForEach threads
// WithProvenance() into the per-element Scope when the outer Scope has it on.
type provenanceProbe struct {
	got *bool
}

func (p *provenanceProbe) Name() string        { return "prov-probe" }
func (p *provenanceProbe) Type() string        { return "prov-probe" }
func (p *provenanceProbe) DependsOn() []string { return nil }

func (p *provenanceProbe) Execute(_ context.Context, sc *pipe.Scope) error {
	*p.got = sc.TracksProvenance()
	return nil
}

// provenancePropagationCase builds the case exercising ForEach's
// provenance-propagation branch: got must be shared, by closure, between this
// case's build and assert fields.
func provenancePropagationCase() forEachExecuteCase {
	var got bool
	return forEachExecuteCase{
		name: "provenance-tracking outer scope propagates to the per-element scope",
		build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
			probe := &provenanceProbe{got: &got}
			innerPipe, err := pipe.NewPipeline(probe)
			require.NoError(t, err)

			fe, err := pipe.NewForEach("lines", "items", innerPipe)
			require.NoError(t, err)

			sc := pipe.NewScope(map[string]any{
				"items": []any{map[string]any{"n": 1}},
			}, pipe.WithProvenance())
			return fe, sc
		},
		assert: func(t *testing.T, sc *pipe.Scope, err error) {
			require.NoError(t, err)
			assert.True(t, got, "per-element scope must track provenance when the outer scope does")
		},
	}
}

type forEachExecuteCase struct {
	name   string
	build  func(t *testing.T) (*pipe.ForEach, *pipe.Scope)
	ctx    func(ctx context.Context) context.Context // nil = identity
	assert func(t *testing.T, sc *pipe.Scope, err error)
}

// midIterationCancelCase builds the case exercising ctx cancellation observed
// between elements (as opposed to before Execute is even called). It is a
// function (not a plain literal) because the cancelHolder must be shared,
// by closure, between this case's build and ctx fields.
func midIterationCancelCase() forEachExecuteCase {
	holder := &cancelHolder{}
	return forEachExecuteCase{
		name: "context canceled mid-iteration stops at the next element boundary",
		build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
			probe := &cancelAfterFirst{holder: holder}
			innerPipe, err := pipe.NewPipeline(probe)
			require.NoError(t, err)

			fe, err := pipe.NewForEach("lines", "items", innerPipe)
			require.NoError(t, err)

			sc := pipe.NewScope(map[string]any{
				"items": []any{
					map[string]any{"n": 1},
					map[string]any{"n": 2},
					map[string]any{"n": 3},
				},
			})
			return fe, sc
		},
		ctx: func(ctx context.Context) context.Context {
			cctx, cancel := context.WithCancel(ctx)
			holder.cancel = cancel
			return cctx
		},
		assert: func(t *testing.T, sc *pipe.Scope, err error) {
			var se *pipe.StageError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, "lines", se.Stage)
			assert.Equal(t, pipe.TypeForEach, se.Type)
			assert.ErrorIs(t, se, context.Canceled)
			assert.Contains(t, se.Error(), "element 1")

			_, ok := sc.Get("lines.items")
			assert.False(t, ok, "no items should be written on a mid-iteration cancel")
		},
	}
}

// nestedMidIterationCancelCase builds the case exercising spec 025 success
// criterion #8: cancellation observed WHILE the inner foreach is iterating
// (not before the outer ForEach.Execute even starts). The cancel-probe stage
// sits inside the inner-most pipeline run by the "taxes" ForEach, so it fires
// once the outer has already entered element 0 and the inner loop is
// underway; the next per-element ctx.Err() check inside "taxes" (before its
// second element) is what actually trips. It is a function (not a plain
// literal) because the cancelHolder must be shared, by closure, between this
// case's build and ctx fields.
func nestedMidIterationCancelCase() forEachExecuteCase {
	holder := &cancelHolder{}
	return forEachExecuteCase{
		name: "context canceled mid-inner-iteration stops a nested foreach with a StageError, no output",
		build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
			probe := &cancelAfterFirst{holder: holder}
			taxPipe, err := pipe.NewPipeline(probe)
			require.NoError(t, err)
			taxes, err := pipe.NewForEach("taxes", "line.taxes", taxPipe, pipe.WithForEachAs("tax"))
			require.NoError(t, err)
			linesPipe, err := pipe.NewPipeline(taxes)
			require.NoError(t, err)
			lines, err := pipe.NewForEach("lines", "orders", linesPipe, pipe.WithForEachAs("line"))
			require.NoError(t, err)

			// At least two inner elements, so there is a genuine "next
			// element" boundary for the cancel-probe to trip at.
			sc := pipe.NewScope(map[string]any{
				"orders": []any{
					map[string]any{"taxes": []any{
						map[string]any{"rate": int64(1)},
						map[string]any{"rate": int64(2)},
					}},
				},
			})
			return lines, sc
		},
		ctx: func(ctx context.Context) context.Context {
			cctx, cancel := context.WithCancel(ctx)
			holder.cancel = cancel
			return cctx
		},
		assert: func(t *testing.T, sc *pipe.Scope, err error) {
			var se *pipe.StageError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, "lines", se.Stage) // outermost stage owns the returned error
			assert.Equal(t, pipe.TypeForEach, se.Type)
			assert.ErrorIs(t, err, context.Canceled)

			assert.Contains(t, err.Error(), "element 0", "outer element index")
			assert.Contains(t, err.Error(), "element 1", "inner boundary where cancellation is observed")

			_, ok := sc.Get("lines.items")
			assert.False(t, ok, "no output should be written on a mid-inner-iteration cancel")
		},
	}
}

// nestedInnerErrorCase builds the case exercising spec 025 success
// criterion #9: an error during an inner PER-ELEMENT iteration names both the
// outer element index and the inner element index, and surfaces the inner
// stage's own name and cause. errAfterFirst fails on the inner-most
// pipeline's second call (inner element 1), so the failure is a genuine
// per-element error rather than a collection-type check that never reaches
// the inner loop.
func nestedInnerErrorCase() forEachExecuteCase {
	return forEachExecuteCase{
		name: "nested inner error names both the outer and inner element index",
		build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
			probe := &errAfterFirst{}
			taxPipe, err := pipe.NewPipeline(probe)
			require.NoError(t, err)
			taxes, err := pipe.NewForEach("taxes", "line.taxes", taxPipe, pipe.WithForEachAs("tax"))
			require.NoError(t, err)
			linesPipe, err := pipe.NewPipeline(taxes)
			require.NoError(t, err)
			lines, err := pipe.NewForEach("lines", "orders", linesPipe, pipe.WithForEachAs("line"))
			require.NoError(t, err)

			sc := pipe.NewScope(map[string]any{
				"orders": []any{
					map[string]any{"taxes": []any{
						map[string]any{"rate": int64(1)}, // inner element 0: probe succeeds
						map[string]any{"rate": int64(2)}, // inner element 1: probe fails
					}},
				},
			})
			return lines, sc
		},
		assert: func(t *testing.T, sc *pipe.Scope, err error) {
			var se *pipe.StageError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, "lines", se.Stage) // outermost stage owns the returned error
			assert.Equal(t, pipe.TypeForEach, se.Type)
			assert.ErrorIs(t, err, errInjectedInner)

			assert.Contains(t, err.Error(), "element 0", "outer element index")
			assert.Contains(t, err.Error(), "element 1", "failing inner element index")
			assert.Contains(t, err.Error(), "taxes", "inner stage name surfaces in the wrapped cause")

			_, ok := sc.Get("lines.items")
			assert.False(t, ok, "no output should be written when an inner element fails")
		},
	}
}

func TestForEachExecute(t *testing.T) {
	t.Parallel()

	cases := []forEachExecuteCase{
		{
			name: "each element evaluated with item bound and outer constant readable",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("adjusted", "item.price * (1 - rate)")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"price": 100},
						map[string]any{"price": 200},
					},
					"rate": 0.25,
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				got, ok := sc.Get("lines.items")
				require.True(t, ok)
				items, ok := got.([]any)
				require.True(t, ok)
				require.Len(t, items, 2)

				el0, ok := items[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, map[string]any{"price": 100}, el0["item"])
				assert.Equal(t, 75.0, el0["adjusted"])

				el1, ok := items[1].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, map[string]any{"price": 200}, el1["item"])
				assert.Equal(t, 150.0, el1["adjusted"])

				// Outer scope must not be mutated by iteration (D2).
				_, hasItem := sc.Get("item")
				assert.False(t, hasItem, "outer scope must not gain an `item` key")
				rate, ok := sc.Get("rate")
				require.True(t, ok)
				assert.Equal(t, 0.25, rate)
			},
		},
		{
			name: "custom element binding and output key",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("doubled", "line.n * 2")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "nums", innerPipe,
					pipe.WithForEachAs("line"), pipe.WithForEachOutput("results"))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"nums": []any{map[string]any{"n": 3}},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.results")
				require.True(t, ok)
				items, ok := got.([]any)
				require.True(t, ok)
				require.Len(t, items, 1)
				el0, ok := items[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, 6, el0["doubled"])
			},
		},
		{
			name: "empty collection writes an empty items list, no error",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				innerPipe, err := pipe.NewPipeline()
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{"items": []any{}})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.items")
				require.True(t, ok)
				items, ok := got.([]any)
				require.True(t, ok)
				assert.Empty(t, items)
			},
		},
		{
			name: "non-list collection value surfaces ErrForEachNotList",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				innerPipe, err := pipe.NewPipeline()
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{"items": 5})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrForEachNotList)
			},
		},
		{
			name: "missing collection path surfaces ErrForEachNoCollection",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				innerPipe, err := pipe.NewPipeline()
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrForEachNoCollection)
			},
		},
		{
			name: "per-element inner error names the element index",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("ratio", "item.a % item.b")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"a": 1, "b": 2},
						map[string]any{"a": 1, "b": 0},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.Contains(t, se.Error(), "element 1")
			},
		},
		{
			name: "context canceled before Execute short-circuits with no items written",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("adjusted", "item.price")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{map[string]any{"price": 10}},
				})
				return fe, sc
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, context.Canceled)

				_, ok := sc.Get("lines.items")
				assert.False(t, ok, "no items should be written when canceled up front")
			},
		},
		{
			name: "output write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				innerPipe, err := pipe.NewPipeline()
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				// "lines" is already a scalar, so the final write to
				// "lines.items" must traverse it as an intermediate map and
				// fails with ErrPathNotMap.
				sc := pipe.NewScope(map[string]any{
					"lines": 5,
					"items": []any{},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "rollup sum of decimal outputs stays exact decimal.Decimal",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateSum, As: "total"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": decimal.RequireFromString("0.1")},
						map[string]any{"amt": decimal.RequireFromString("0.2")},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.total")
				require.True(t, ok)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("0.3").Equal(d), "got %s", d)
			},
		},
		{
			name: "rollup sum of int64 outputs stays int64",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateSum, As: "total"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(10)},
						map[string]any{"amt": int64(32)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.total")
				require.True(t, ok)
				assert.Equal(t, int64(42), got)
			},
		},
		{
			// B1 (spec 017): a dot-path Rollup.Key rolls up a decision-table
			// output (<table>.<key>) directly. Only elements the table matched
			// supply "grade.score", so the sum folds over the matched subset.
			name: "rollup Key resolves a decision-table dot-path output, folding matched elements",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				grade, err := pipe.NewDecisionTable("grade", []pipe.Rule{
					{ID: "OK", Condition: "item.amt >= 20", Decisions: map[string]pipe.Decision{"score": {Expr: "item.amt"}}},
				})
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(grade)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "grade.score", Agg: pipe.AggregateSum, As: "total"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(10)}, // below threshold: no grade.score
						map[string]any{"amt": int64(30)},
						map[string]any{"amt": int64(40)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.total")
				require.True(t, ok)
				assert.Equal(t, int64(70), got, "sum over the two matched elements' grade.score")
			},
		},
		{
			// A dot-path whose intermediate segment is a scalar (not a map) is
			// unresolvable, so every element is skipped — Sum over an empty fold
			// leaves the output key absent (same contract as a missing flat key).
			name: "rollup dot-path into a non-map intermediate skips every element",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt.score", Agg: pipe.AggregateSum, As: "total"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(10)},
						map[string]any{"amt": int64(20)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("lines.total")
				assert.False(t, ok, "Sum over an empty fold leaves the key absent")
			},
		},
		{
			// A dot-path whose top segment no element produces (the table never
			// matches) is skipped for all; Count over an empty fold writes 0.
			name: "rollup dot-path missing on every element yields Count 0",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				grade, err := pipe.NewDecisionTable("grade", []pipe.Rule{
					{ID: "NEVER", Condition: "false", Decisions: map[string]pipe.Decision{"score": {Expr: "item.amt"}}},
				})
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(grade)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "grade.score", Agg: pipe.AggregateCount, As: "n"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(10)},
						map[string]any{"amt": int64(20)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.n")
				require.True(t, ok)
				assert.Equal(t, 0, got, "Count over an empty fold is 0")
			},
		},
		{
			name: "rollup min over decimal outputs returns the exact matched element",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateMin, As: "smallest"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": decimal.RequireFromString("2.5")},
						map[string]any{"amt": decimal.RequireFromString("1.5")},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.smallest")
				require.True(t, ok)
				d, ok := got.(decimal.Decimal)
				require.True(t, ok, "expected decimal.Decimal, got %T", got)
				assert.True(t, decimal.RequireFromString("1.5").Equal(d), "got %s", d)
			},
		},
		{
			name: "rollup max over int64 outputs returns the exact matched element",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateMax, As: "largest"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(5)},
						map[string]any{"amt": int64(9)},
						map[string]any{"amt": int64(3)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.largest")
				require.True(t, ok)
				assert.Equal(t, int64(9), got)
			},
		},
		{
			name: "rollup count over per-element outputs",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateCount, As: "n"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(5)},
						map[string]any{"amt": int64(9)},
						map[string]any{"amt": int64(3)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.n")
				require.True(t, ok)
				assert.Equal(t, 3, got)
			},
		},
		{
			name: "rollup list collects all per-element values in order",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateList, As: "all"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": int64(5)},
						map[string]any{"amt": int64(9)},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("lines.all")
				require.True(t, ok)
				assert.Equal(t, []any{int64(5), int64(9)}, got)
			},
		},
		{
			name: "rollup over an empty collection: count 0, list empty, sum/min/max absent",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				innerPipe, err := pipe.NewPipeline()
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(
						pipe.Rollup{Key: "amt", Agg: pipe.AggregateCount, As: "n"},
						pipe.Rollup{Key: "amt", Agg: pipe.AggregateList, As: "all"},
						pipe.Rollup{Key: "amt", Agg: pipe.AggregateSum, As: "total"},
						pipe.Rollup{Key: "amt", Agg: pipe.AggregateMin, As: "smallest"},
						pipe.Rollup{Key: "amt", Agg: pipe.AggregateMax, As: "largest"},
					))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{"items": []any{}})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				n, ok := sc.Get("lines.n")
				require.True(t, ok)
				assert.Equal(t, 0, n)

				all, ok := sc.Get("lines.all")
				require.True(t, ok)
				assert.Equal(t, []any{}, all)

				_, ok = sc.Get("lines.total")
				assert.False(t, ok, "sum over empty must leave the key absent")
				_, ok = sc.Get("lines.smallest")
				assert.False(t, ok, "min over empty must leave the key absent")
				_, ok = sc.Get("lines.largest")
				assert.False(t, ok, "max over empty must leave the key absent")
			},
		},
		{
			name: "rollup over non-numeric values surfaces the aggregate error",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateSum, As: "total"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"amt": "not-a-number"},
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrNonNumericAggregate)
			},
		},
		{
			name: "rollup write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				inner, err := pipe.NewSingleExpr("amt", "item.amt")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(inner)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe,
					pipe.WithRollups(pipe.Rollup{Key: "amt", Agg: pipe.AggregateCount, As: "n"}))
				require.NoError(t, err)

				// "lines" is already a scalar, so the rollup write to
				// "lines.n" (reached before the final items write) must
				// traverse it as an intermediate map and fails with
				// ErrPathNotMap.
				sc := pipe.NewScope(map[string]any{
					"lines": 5,
					"items": []any{map[string]any{"amt": int64(1)}},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "per-element firing recorded under the composite key <stage>[i].<inner>",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				table, err := pipe.NewDecisionTable("check", []pipe.Rule{
					{ID: "HIGH", Condition: "item.ltv > 80", Decisions: map[string]pipe.Decision{"flag": {Expr: `"high"`}}},
					{ID: "LOW", Condition: "item.ltv < 50", Decisions: map[string]pipe.Decision{"flag": {Expr: `"low"`}}},
				})
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(table)
				require.NoError(t, err)

				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"ltv": 90}, // fires HIGH
						map[string]any{"ltv": 65}, // fires nothing (gap between rules)
						map[string]any{"ltv": 30}, // fires LOW
					},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				f0 := sc.FiringRulesFor("lines[0].check")
				require.Len(t, f0, 1)
				assert.Equal(t, "check", f0[0].Stage)
				assert.Equal(t, "HIGH", f0[0].RuleID)

				f1 := sc.FiringRulesFor("lines[1].check")
				assert.Empty(t, f1, "no rule matched element 1, so nothing should be recorded")

				f2 := sc.FiringRulesFor("lines[2].check")
				require.Len(t, f2, 1)
				assert.Equal(t, "check", f2[0].Stage)
				assert.Equal(t, "LOW", f2[0].RuleID)

				// The inner stage's own name must not be conflated with the
				// per-element composite key.
				assert.Empty(t, sc.FiringRulesFor("check"))
			},
		},
		{
			name: "per-element lineage merged under <stage>[i] with provenance on",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				// Decision VALUES read the element (item.ltv) so per-element
				// lineage has something to trace back to the element seed.
				table, err := pipe.NewDecisionTable("check", []pipe.Rule{
					{ID: "HIGH", Condition: "item.ltv > 80", Decisions: map[string]pipe.Decision{"score": {Expr: "item.ltv"}}},
					{ID: "LOW", Condition: "item.ltv < 50", Decisions: map[string]pipe.Decision{"score": {Expr: "item.ltv"}}},
				})
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(table)
				require.NoError(t, err)
				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{
					"items": []any{
						map[string]any{"ltv": 90}, // fires HIGH
						map[string]any{"ltv": 30}, // fires LOW
					},
				}, pipe.WithProvenance())
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				// Element 0's decision output is a derivation on the outer scope.
				d0, ok := sc.Derivation("lines[0].check.score")
				require.True(t, ok)
				assert.Equal(t, "check", d0.Stage)
				assert.Equal(t, 90, d0.Value)

				// Explain traces element 0's output through its input (prefixed
				// member path) back to its element seed (via the B6 ancestor
				// fallback under the "lines[0]" prefix).
				ex0 := sc.Explain("lines[0].check.score")
				assert.Contains(t, ex0, "lines[0].check.score = 90")
				assert.Contains(t, ex0, "lines[0].item")
				assert.Contains(t, ex0, "[seed]")

				// Element 1 is independent and reconciles within its own prefix.
				d1, ok := sc.Derivation("lines[1].check.score")
				require.True(t, ok)
				assert.Equal(t, 30, d1.Value)
				assert.NotEmpty(t, sc.Lineage("lines[1].check.score"))

				// Per-element firing is still recorded (regression).
				f0 := sc.FiringRulesFor("lines[0].check")
				require.Len(t, f0, 1)
				assert.Equal(t, "HIGH", f0[0].RuleID)
			},
		},
		{
			name: "no per-element derivations recorded when provenance is off",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				table, err := pipe.NewDecisionTable("check", []pipe.Rule{
					{ID: "HIGH", Condition: "item.ltv > 80", Decisions: map[string]pipe.Decision{"flag": {Expr: `"high"`}}},
				})
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(table)
				require.NoError(t, err)
				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{
					"items": []any{map[string]any{"ltv": 90}},
				}) // no WithProvenance
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Derivation("lines[0].check.flag")
				assert.False(t, ok, "no per-element derivations when provenance is off")
				assert.Empty(t, sc.Derivations())
				// Firing is independent of provenance and still recorded.
				assert.Len(t, sc.FiringRulesFor("lines[0].check"), 1)
			},
		},
		{
			name: "nested foreach preserves the inner element index in the firing key",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				vat, err := pipe.NewDecisionTable("vat", []pipe.Rule{
					{ID: "VAT_STD", Condition: "tax.rate >= 10", Decisions: map[string]pipe.Decision{"band": {Expr: `"standard"`}}},
					{ID: "VAT_RED", Condition: "tax.rate < 10", Decisions: map[string]pipe.Decision{"band": {Expr: `"reduced"`}}},
				})
				require.NoError(t, err)
				vatPipe, err := pipe.NewPipeline(vat)
				require.NoError(t, err)
				taxes, err := pipe.NewForEach("taxes", "line.taxes", vatPipe, pipe.WithForEachAs("tax"))
				require.NoError(t, err)
				linesPipe, err := pipe.NewPipeline(taxes)
				require.NoError(t, err)
				lines, err := pipe.NewForEach("lines", "orders", linesPipe, pipe.WithForEachAs("line"))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"orders": []any{
						map[string]any{"taxes": []any{
							map[string]any{"rate": 5},  // element [0][0] -> VAT_RED
							map[string]any{"rate": 20}, // element [0][1] -> VAT_STD
						}},
					},
				})
				return lines, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				f00 := sc.FiringRulesFor("lines[0].taxes[0].vat")
				require.Len(t, f00, 1)
				assert.Equal(t, "VAT_RED", f00[0].RuleID)
				assert.Equal(t, "vat", f00[0].Stage) // .Stage stays the bare DT name

				f01 := sc.FiringRulesFor("lines[0].taxes[1].vat")
				require.Len(t, f01, 1)
				assert.Equal(t, "VAT_STD", f01[0].RuleID)

				// The flat prefix keys are NOT firing keys (only leaf DT keys are).
				assert.Empty(t, sc.FiringRulesFor("lines[0]"))
				assert.Empty(t, sc.FiringRulesFor("lines[0].taxes[1]"))
			},
		},
		{
			name: "no firing keys recorded when no inner rule fires",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				// A single-expr inner pipeline fires no decision-table rules.
				amt, err := pipe.NewSingleExpr("amt", "item.price * 2")
				require.NoError(t, err)
				innerPipe, err := pipe.NewPipeline(amt)
				require.NoError(t, err)
				fe, err := pipe.NewForEach("lines", "items", innerPipe)
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{
					"items": []any{map[string]any{"price": int64(3)}},
				})
				return fe, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				assert.Empty(t, sc.FiringRules(), "no rules fired, so no firing keys recorded")
				// The data output is still written.
				_, ok := sc.Get("lines.items")
				assert.True(t, ok)
			},
		},
		{
			name: "nested foreach composes lineage, output shape, and an outer rollup over a nested value",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				// Decision reads the element (pass-through keeps the int64 type stable,
				// so the rollup sum stays int64 per ADR-0025) so lineage traces to the
				// element seed.
				vat, err := pipe.NewDecisionTable("vat", []pipe.Rule{
					{ID: "ANY", Condition: "tax.rate >= 0", Decisions: map[string]pipe.Decision{"amount": {Expr: "tax.rate"}}},
				})
				require.NoError(t, err)
				vatPipe, err := pipe.NewPipeline(vat)
				require.NoError(t, err)
				// Inner rollup: sum each order's per-tax vat.amount into taxes.sumAmt.
				taxes, err := pipe.NewForEach("taxes", "line.taxes", vatPipe,
					pipe.WithForEachAs("tax"),
					pipe.WithRollups(pipe.Rollup{Key: "vat.amount", Agg: pipe.AggregateSum, As: "sumAmt"}),
				)
				require.NoError(t, err)
				linesPipe, err := pipe.NewPipeline(taxes)
				require.NoError(t, err)
				// Outer rollup over the nested value taxes.sumAmt.
				lines, err := pipe.NewForEach("lines", "orders", linesPipe,
					pipe.WithForEachAs("line"),
					pipe.WithRollups(pipe.Rollup{Key: "taxes.sumAmt", Agg: pipe.AggregateSum, As: "grandTotal"}),
				)
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{
					"orders": []any{
						map[string]any{"taxes": []any{
							map[string]any{"rate": int64(5)},  // amount 5
							map[string]any{"rate": int64(20)}, // amount 20 -> order sumAmt 25
						}},
					},
				}, pipe.WithProvenance())
				return lines, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				// Nested lineage: the innermost output traces through both prefixes to
				// the element seed.
				d, ok := sc.Derivation("lines[0].taxes[1].vat.amount")
				require.True(t, ok)
				assert.Equal(t, int64(20), d.Value)
				ex := sc.Explain("lines[0].taxes[1].vat.amount")
				assert.Contains(t, ex, "lines[0].taxes[1].vat.amount = 20")
				assert.Contains(t, ex, "lines[0].taxes[1].tax")
				assert.Contains(t, ex, "[seed]")
				assert.NotEmpty(t, sc.Lineage("lines[0].taxes[1].vat.amount"))

				// Nested output shape: lines.items[0].taxes.items[1].vat.amount.
				got, ok := sc.Get("lines.items")
				require.True(t, ok)
				order0 := got.([]any)[0].(map[string]any)
				innerItems := order0["taxes"].(map[string]any)["items"].([]any)
				require.Len(t, innerItems, 2)
				assert.Equal(t, int64(20), innerItems[1].(map[string]any)["vat"].(map[string]any)["amount"])

				// Outer rollup over the nested value (5 + 20 = 25, int64-preserving).
				grand, ok := sc.Get("lines.grandTotal")
				require.True(t, ok)
				assert.Equal(t, int64(25), grand)
			},
		},
		{
			name: "nested inner error names the outer element and surfaces the inner cause",
			build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
				vat, err := pipe.NewDecisionTable("vat", []pipe.Rule{
					{ID: "ANY", Condition: "true", Decisions: map[string]pipe.Decision{"band": {Expr: `"x"`}}},
				})
				require.NoError(t, err)
				vatPipe, err := pipe.NewPipeline(vat)
				require.NoError(t, err)
				taxes, err := pipe.NewForEach("taxes", "line.taxes", vatPipe, pipe.WithForEachAs("tax"))
				require.NoError(t, err)
				linesPipe, err := pipe.NewPipeline(taxes)
				require.NoError(t, err)
				lines, err := pipe.NewForEach("lines", "orders", linesPipe, pipe.WithForEachAs("line"))
				require.NoError(t, err)

				// Outer element 0's "taxes" is NOT a list -> the inner foreach errors.
				sc := pipe.NewScope(map[string]any{
					"orders": []any{map[string]any{"taxes": int64(5)}},
				})
				return lines, sc
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage) // outermost stage owns the returned error
				assert.ErrorIs(t, err, pipe.ErrForEachNotList)
				assert.Contains(t, err.Error(), "element 0") // the outer element index is named
			},
		},
		provenancePropagationCase(),
		midIterationCancelCase(),
		nestedMidIterationCancelCase(),
		nestedInnerErrorCase(),
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := s.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewForEachErrors(t *testing.T) {
	t.Parallel()

	validInner, err := pipe.NewPipeline()
	require.NoError(t, err)

	type testCase struct {
		name   string
		build  func() (*pipe.ForEach, error)
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "empty name",
			build: func() (*pipe.ForEach, error) {
				return pipe.NewForEach("", "items", validInner)
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrEmptyStageName)
			},
		},
		{
			name: "nil inner pipeline",
			build: func() (*pipe.ForEach, error) {
				return pipe.NewForEach("lines", "items", nil)
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
			},
		},
		{
			name: "empty collection path",
			build: func() (*pipe.ForEach, error) {
				return pipe.NewForEach("lines", "", validInner)
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
			},
		},
		{
			name: "rollup with an empty As is rejected at construction",
			build: func() (*pipe.ForEach, error) {
				return pipe.NewForEach("lines", "items", validInner,
					pipe.WithRollups(pipe.Rollup{Key: "amount", Agg: pipe.AggregateSum, As: ""}))
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.Equal(t, pipe.TypeForEach, se.Type)
				assert.ErrorIs(t, se, pipe.ErrForEachEmptyRollup)
			},
		},
		{
			name: "rollup with an empty Key is rejected at construction",
			build: func() (*pipe.ForEach, error) {
				return pipe.NewForEach("lines", "items", validInner,
					pipe.WithRollups(pipe.Rollup{Key: "", Agg: pipe.AggregateSum, As: "total"}))
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "lines", se.Stage)
				assert.ErrorIs(t, se, pipe.ErrForEachEmptyRollup)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.build()
			tc.assert(t, err)
		})
	}
}

func TestForEachAccessors(t *testing.T) {
	t.Parallel()

	inner, err := pipe.NewPipeline()
	require.NoError(t, err)

	fe, err := pipe.NewForEach("lines", "items", inner,
		pipe.WithForEachDependsOn("base"))
	require.NoError(t, err)

	assert.Equal(t, "lines", fe.Name())
	assert.Equal(t, pipe.TypeForEach, fe.Type())
	assert.Equal(t, []string{"base"}, fe.DependsOn())
}
