package pipe

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kartaladev/rlng/expr"
)

// NamedExpr is one entry in a MultiExpr stage.
type NamedExpr struct {
	Name       string
	Expression string
	Priority   int
	Options    []expr.Option
}

// MultiExpr evaluates several named expressions in ascending priority order,
// each visible to later ones within the stage, writing each result to
// name.<exprName> in the Scope.
type MultiExpr struct {
	name  string
	deps  []string
	exprs []compiledNamed
}

type compiledNamed struct {
	name string
	fn   *expr.Function
}

// NewMultiExpr compiles a MultiExpr stage. Expression names must be non-empty
// and unique within the stage; entries are ordered by ascending Priority
// (stable for ties).
func NewMultiExpr(name string, exprs []NamedExpr, opts ...Option) (*MultiExpr, error) {
	if name == "" {
		return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: ErrEmptyStageName}
	}
	if len(exprs) == 0 {
		return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: errors.New("multi-expr requires at least one expression")}
	}
	cfg := newStageConfig(opts)

	ordered := make([]NamedExpr, len(exprs))
	copy(ordered, exprs)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })

	seen := make(map[string]struct{}, len(ordered))
	compiled := make([]compiledNamed, 0, len(ordered))
	for _, e := range ordered {
		if e.Name == "" {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: errors.New("expression name must not be empty")}
		}
		if _, dup := seen[e.Name]; dup {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: fmt.Errorf("duplicate expression name %q", e.Name)}
		}
		seen[e.Name] = struct{}{}

		fn, err := expr.NewFunction(e.Name, e.Expression, e.Options...)
		if err != nil {
			return nil, &StageError{Stage: name, Type: TypeMultiExpr, Cause: err}
		}
		compiled = append(compiled, compiledNamed{name: e.Name, fn: fn})
	}
	return &MultiExpr{name: name, deps: cfg.deps, exprs: compiled}, nil
}

// Name returns the stage's name; results are written under name.<exprName>.
func (m *MultiExpr) Name() string { return m.name }

// Type returns TypeMultiExpr.
func (m *MultiExpr) Type() string { return TypeMultiExpr }

// DependsOn returns the names of the stages this stage depends on.
func (m *MultiExpr) DependsOn() []string { return m.deps }

// Execute evaluates the expressions in priority order. Each result is visible to
// later expressions in this stage (by its bare name) and persisted to the Scope
// under name.<exprName>.
func (m *MultiExpr) Execute(ctx context.Context, sc *Scope) error {
	if err := ctx.Err(); err != nil {
		return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
	}

	env := sc.Snapshot()
	tracking := sc.TracksProvenance()
	var locals map[string]struct{}
	if tracking {
		locals = make(map[string]struct{}, len(m.exprs))
	}
	for _, e := range m.exprs {
		v, err := e.fn.Apply(env)
		if err != nil {
			return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
		}
		if tracking {
			d := Derivation{
				Stage:      m.name,
				StageType:  TypeMultiExpr,
				Operation:  "expr:" + e.name,
				Expression: e.fn.Source(),
				Inputs:     snapshotRefsKeyed(env, e.fn.References(), m.qualifyLocal(locals)),
			}
			if err := sc.Derive(m.name+"."+e.name, v, d); err != nil {
				return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
			}
			locals[e.name] = struct{}{} // an earlier local alias for later expressions
		} else if err := sc.Set(m.name+"."+e.name, v); err != nil {
			return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
		}
		env[e.name] = v // visible to later expressions within this stage
	}
	return nil
}

// qualifyLocal returns a key transform for snapshotRefsKeyed: a reference whose
// first path segment names an earlier expression in this stage (a local alias) is
// keyed under its scope path (stage.<ref>), so Lineage/Explain reconcile it to
// that expression's derivation; seed and cross-stage references keep their key. It
// returns nil when there are no earlier locals yet (nothing to qualify).
func (m *MultiExpr) qualifyLocal(locals map[string]struct{}) func(string) string {
	if len(locals) == 0 {
		return nil
	}
	return func(ref string) string {
		seg := ref
		if i := strings.IndexByte(ref, '.'); i >= 0 {
			seg = ref[:i]
		}
		if _, ok := locals[seg]; ok {
			return m.name + "." + ref
		}
		return ref
	}
}
