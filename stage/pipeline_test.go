package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordStage is a minimal Stage that appends its name to *order when executed
// and writes a marker into the Scope, so tests can observe execution order and
// dependency satisfaction. If execErr is non-nil, Execute returns it.
type recordStage struct {
	name    string
	deps    []string
	order   *[]string
	execErr error
}

func (s *recordStage) Name() string        { return s.name }
func (s *recordStage) Type() string        { return "record" }
func (s *recordStage) DependsOn() []string { return s.deps }
func (s *recordStage) Execute(ctx context.Context, sc *Scope) error {
	if s.execErr != nil {
		return s.execErr
	}
	*s.order = append(*s.order, s.name)
	return sc.Set(s.name, true)
}

func TestNewPipelineValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() []Stage
		assert func(t *testing.T, p *Pipeline, err error)
	}

	var order []string
	rs := func(name string, deps ...string) *recordStage {
		return &recordStage{name: name, deps: deps, order: &order}
	}

	cases := []testCase{
		{
			name:  "empty set is valid",
			build: func() []Stage { return nil },
			assert: func(t *testing.T, p *Pipeline, err error) {
				require.NoError(t, err)
				require.NotNil(t, p)
			},
		},
		{
			name:  "valid acyclic set constructs",
			build: func() []Stage { return []Stage{rs("a"), rs("b", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				require.NoError(t, err)
				require.NotNil(t, p)
			},
		},
		{
			name:  "duplicate name is rejected",
			build: func() []Stage { return []Stage{rs("a"), rs("a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var de *DuplicateStageError
				require.ErrorAs(t, err, &de)
				assert.Equal(t, "a", de.Name)
				assert.Equal(t, `pipeline: duplicate stage "a"`, de.Error())
			},
		},
		{
			name:  "unknown dependency is rejected",
			build: func() []Stage { return []Stage{rs("a", "ghost")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ue *UnknownDependencyError
				require.ErrorAs(t, err, &ue)
				assert.Equal(t, "a", ue.Stage)
				assert.Equal(t, "ghost", ue.Dependency)
				assert.Equal(t, `pipeline: stage "a" depends on unknown stage "ghost"`, ue.Error())
			},
		},
		{
			name:  "self dependency is a cycle",
			build: func() []Stage { return []Stage{rs("a", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, []string{"a", "a"}, ce.Cycle)
			},
		},
		{
			name:  "two node cycle reports concrete path",
			build: func() []Stage { return []Stage{rs("a", "b"), rs("b", "a")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, []string{"a", "b", "a"}, ce.Cycle)
				assert.Equal(t, "pipeline: dependency cycle: a -> b -> a", ce.Error())
			},
		},
		{
			name:  "three node cycle is detected",
			build: func() []Stage { return []Stage{rs("a", "c"), rs("b", "a"), rs("c", "b")} },
			assert: func(t *testing.T, p *Pipeline, err error) {
				assert.Nil(t, p)
				var ce *CycleError
				require.ErrorAs(t, err, &ce)
				// The cycle closes on the repeated node.
				assert.Equal(t, ce.Cycle[0], ce.Cycle[len(ce.Cycle)-1])
				assert.Len(t, ce.Cycle, 4)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := NewPipeline(tc.build()...)
			tc.assert(t, p, err)
		})
	}
}

func TestPipelineRun(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		seed   map[string]any
		build  func(order *[]string) []Stage
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, order []string, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "runs in dependency order overriding declaration order",
			build: func(order *[]string) []Stage {
				// Declared b-before-a, but b depends on a, so a must run first.
				return []Stage{
					&recordStage{name: "b", deps: []string{"a"}, order: order},
					&recordStage{name: "a", order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"a", "b"}, order)
			},
		},
		{
			name: "independent stages preserve input order",
			build: func(order *[]string) []Stage {
				return []Stage{
					&recordStage{name: "x", order: order},
					&recordStage{name: "y", order: order},
					&recordStage{name: "z", order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"x", "y", "z"}, order)
			},
		},
		{
			name: "diamond runs dependencies before dependents",
			build: func(order *[]string) []Stage {
				// a -> {b, c} -> d
				return []Stage{
					&recordStage{name: "a", order: order},
					&recordStage{name: "b", deps: []string{"a"}, order: order},
					&recordStage{name: "c", deps: []string{"a"}, order: order},
					&recordStage{name: "d", deps: []string{"b", "c"}, order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"a", "b", "c", "d"}, order)
			},
		},
		{
			name: "dependent reads dependency output from scope",
			build: func(order *[]string) []Stage {
				a, err := NewSingleExpr("a", "21")
				require.NoError(t, err)
				b, err := NewSingleExpr("b", "a * 2", WithDependsOn("a"))
				require.NoError(t, err)
				return []Stage{b, a} // declared out of order on purpose
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				v, ok := sc.Get("b")
				require.True(t, ok)
				assert.Equal(t, 42, v)
			},
		},
		{
			name: "empty pipeline run is a no-op",
			build: func(order *[]string) []Stage {
				return nil
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.NoError(t, err)
				assert.Empty(t, order)
			},
		},
		{
			name: "first stage error stops the run",
			// x is a runtime value, so `x % 0` is not constant-folded at compile;
			// it fails at eval with an integer divide by zero (expr does float
			// division, so `/` would yield +Inf — modulo forces a real error).
			seed: map[string]any{"x": 1},
			build: func(order *[]string) []Stage {
				boom, err := NewSingleExpr("boom", "x % 0")
				require.NoError(t, err)
				return []Stage{
					boom,
					&recordStage{name: "after", deps: []string{"boom"}, order: order},
				}
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "boom", se.Stage)
				assert.Empty(t, order) // "after" never ran
			},
		},
		{
			name: "canceled context short-circuits before any stage",
			build: func(order *[]string) []Stage {
				return []Stage{&recordStage{name: "a", order: order}}
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, order []string, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
				assert.Empty(t, order)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var order []string
			p, err := NewPipeline(tc.build(&order)...)
			require.NoError(t, err)

			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			sc := NewScope(tc.seed)
			runErr := p.Run(ctx, sc)
			tc.assert(t, order, sc, runErr)
		})
	}
}
