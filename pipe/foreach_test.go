package pipe_test

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/pipe"
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
		provenancePropagationCase(),
		midIterationCancelCase(),
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
