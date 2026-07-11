# Scope Provenance/Lineage + Typed Getters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make every `Scope` value's derivation inspectable (opt-in provenance + recursive `Explain`) and add strict typed getters, with benchmarks proving the provenance-off path is zero-cost.

**Architecture:** `expr` gains `References()`/`Source()` (compile-time identifier extraction, cached). `stage` gains provenance (`Derivation`, `WithProvenance`, `Derive`, `Lineage`, `Explain`) recorded by the three stages behind a lock-free `TracksProvenance()` guard, plus strict generic getters. No new module dependency (uses `expr-lang/expr`'s `parser`/`ast` subpackages).

**Tech Stack:** Go 1.25+; `github.com/expr-lang/expr` (+ its `parser`/`ast` subpackages, no new module); tests use `testify`.

## Global Constraints

- Module `github.com/kartaladev/rlng`; packages `expr`, `stage`. (Spec 006)
- **No new module dependency.** (Spec 006 §Dependencies)
- **Provenance is opt-in and zero-cost when off** — off path adds no allocations vs baseline; `TracksProvenance()` is lock-free. (ADR-0011)
- **Strict getters, no coercion**; typed `ErrPathNotFound`/`*ScopeTypeError`. (ADR-0012)
- **Tests:** `table-test` skill (assert closures); provenance-on cases added to the stages' existing `ctx` tables; getter/provenance tables are context-free.
- **Coverage gate:** ≥85% on changed packages; every hot-path + typed-error branch tested.
- **Benchmarks:** `go test -bench . -benchmem`; the provenance-off allocs/op must equal baseline (record benchstat in the commit narrative).
- Gates: `go build`, `go test ./... -race`, `go vet`, `gofmt -l .`, `golangci-lint run ./...` clean.

---

### Task 1: `expr` — `References()` and `Source()`

**Files:** Create `expr/refs.go`, `expr/refs_test.go`; modify `expr/function.go`, `expr/predicate.go`.

**Produces:** `func (f *Function) References() []string`, `func (f *Function) Source() string`, `func (p *Predicate) References() []string`, `func (p *Predicate) Source() string`.

**Branches to cover:** identifiers collected; member access `a.b` → `a`; literal-only expression → nil; dedup + sort; `Source()` returns the original expression.

- [ ] **Step 1 — failing test** `expr/refs_test.go`:

```go
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
```

- [ ] **Step 2 — run red:** `go test ./expr/ -run References` → FAIL (undefined).

- [ ] **Step 3 — `expr/refs.go`:**

```go
package expr

import (
	"sort"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// references returns the sorted, unique top-level identifiers read by src. src is
// already known to compile; a parse failure yields nil. Computed once at compile.
func references(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}
	v := &refVisitor{seen: map[string]struct{}{}}
	node := tree.Node
	ast.Walk(&node, v)
	if len(v.seen) == 0 {
		return nil
	}
	refs := make([]string, 0, len(v.seen))
	for name := range v.seen {
		refs = append(refs, name)
	}
	sort.Strings(refs)
	return refs
}

type refVisitor struct{ seen map[string]struct{} }

func (r *refVisitor) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		r.seen[id.Value] = struct{}{}
	}
}
```

- [ ] **Step 4 — wire into `Function`/`Predicate`:** add a `references []string` field; in each constructor set it from `references(src)` (the trimmed source already compiled); add:

```go
// References returns the sorted top-level identifiers this Function reads,
// computed once at compile. The returned slice must not be mutated.
func (f *Function) References() []string { return f.references }

// Source returns the Function's original expression string.
func (f *Function) Source() string { return f.expression }
```
(and the analogous `Predicate` methods; `Predicate` stores its source string — add a field if absent.)

- [ ] **Step 5 — run green:** `go test ./expr/ -race` PASS. Add `BenchmarkFunctionReferences` in `expr/refs_test.go` (constructs a Function in a loop; asserts `References()` is O(1) at call time by benchmarking the accessor separately).

- [ ] **Step 6 — commit** `feat(expr): expose References() and Source() on Function/Predicate` (`Spec: 006`, `ADR: 0011`, plus the spec + ADR files + this plan on the first feat commit).

---

### Task 2: `stage.Scope` — provenance core

**Files:** Create `stage/provenance.go`, `stage/provenance_test.go`; modify `stage/scope.go` (fields + seed recording).

**Produces:** `Derivation`, `WithProvenance`, `TracksProvenance`, `Derive`, `Derivation(path)`, `Derivations`, `Lineage`, `Explain`, and internal `snapshotRefs`.

**Branches:** off — `TracksProvenance()==false`, `Derive`==`Set`, `Derivation` absent, `Explain`/`Lineage` empty; on — seed recorded; `Derive` records; `Lineage`/`Explain` for linear + namespaced (prefix) + shared-input (diamond) chains; unknown path.

- [ ] **Step 1 — `stage/scope.go` fields + seed recording.** Add `provenance bool` and `derivations map[string]Derivation` to `Scope`; in `NewScope`, after applying options:

```go
	if s.provenance {
		s.derivations = make(map[string]Derivation, len(data))
		for k, v := range data {
			s.derivations[k] = Derivation{Path: k, StageType: seedStageType, Operation: "seed", Value: v}
		}
	}
```

- [ ] **Step 2 — `stage/provenance.go`** (full code): `Derivation` struct; `seedStageType = "seed"`; `WithProvenance`; lock-free `TracksProvenance`; `Derive` (Set then, if provenance, record under lock); `Derivation`/`Derivations` (RLock copies); `snapshotRefs`; and `Lineage`/`Explain` with `derivationsFor` (exact key else `key+"."` prefix children, sorted by path) + a `visited` set. `Explain` prints `path = value [stage type] expr: src` (or `path = value [seed]`) indented 2 spaces per depth; inputs sorted, expanded once. (See spec §Design for the exact shapes.)

- [ ] **Step 3 — tests** `stage/provenance_test.go` covering the branches above, including an `Explain` exact-string assertion for a linear chain and a namespaced decision-table read.

- [ ] **Step 4 — green + benchmarks** `stage/scope_bench_test.go`: `BenchmarkScopeSet`, `BenchmarkScopeDeriveOff` (must match Set allocs), `BenchmarkScopeDeriveOn`, `BenchmarkExplain`. Run `go test ./stage/ -bench 'Scope|Explain' -benchmem`.

- [ ] **Step 5 — commit** `feat(stage): opt-in Scope value provenance and recursive lineage`.

---

### Task 3: stage recording (single/multi/table)

**Files:** modify `stage/single.go`, `stage/multi.go`, `stage/table.go`; add provenance-on test cases to their `_test.go` tables.

**Branches:** each stage, provenance on, records a `Derivation` with correct `Operation`/`Expression`/`Inputs`; off path unchanged (plain `Set`). `SingleExpr` → `eval`; `MultiExpr` → `expr:<name>`; `DecisionTable` single → `decision:<key>`, collect → `collect:<key>` (Inputs = union of matched refs).

- [ ] **Step 1** — In each `Execute`, guard with `if sc.TracksProvenance()`: build `Inputs = snapshotRefs(env, fn.References())`, `Expression = fn.Source()`, and call `sc.Derive(path, v, Derivation{...})`; else the existing `sc.Set(path, v)`. For `DecisionTable` collect, accumulate per-key expressions (join with `"; "`) and the union of refs while iterating, then `Derive` each key with the `[]any` value. Wrap `Derive` errors in the stage's `*StageError` exactly as `Set` errors are today.

- [ ] **Step 2** — add provenance-on cases to `single_test.go`/`multi_test.go`/`table_test.go` asserting `sc.Derivation("stage.key")` has the expected `Operation`/`Expression`/`Inputs`, and one `Explain` end-to-end.

- [ ] **Step 3** — green; `go test ./stage/ -race -cover` (≥85%).

- [ ] **Step 4 — commit** `feat(stage): record derivations from the three stage types`.

---

### Task 4: strict typed getters

**Files:** Create `stage/get.go`, `stage/get_test.go`.

**Produces:** `ErrPathNotFound`, `ScopeTypeError`, `GetAs[T]`, `GetInt/GetInt64/GetFloat64/GetString/GetBool/GetSlice/GetMap`.

**Branches:** hit per type; missing → `ErrPathNotFound`; wrong type → `*ScopeTypeError` (assert `Expected`/`Actual`); stored `nil` → `*ScopeTypeError`; `GetAs[[]any]` for a slice.

- [ ] **Step 1 — `stage/get.go`:**

```go
package stage

import (
	"errors"
	"fmt"
)

// ErrPathNotFound is returned by the typed getters when a path is absent.
var ErrPathNotFound = errors.New("scope: path not found")

// ScopeTypeError reports a typed getter finding a value of the wrong type.
type ScopeTypeError struct{ Path, Expected, Actual string }

func (e *ScopeTypeError) Error() string {
	return fmt.Sprintf("scope: path %q: expected %s, got %s", e.Path, e.Expected, e.Actual)
}

// GetAs returns the value at path as T. It returns ErrPathNotFound if the path is
// absent and a *ScopeTypeError if the value is not a T. Strict: no coercion.
func GetAs[T any](s *Scope, path string) (T, error) {
	var zero T
	v, ok := s.Get(path)
	if !ok {
		return zero, ErrPathNotFound
	}
	t, ok := v.(T)
	if !ok {
		return zero, &ScopeTypeError{Path: path, Expected: fmt.Sprintf("%T", zero), Actual: fmt.Sprintf("%T", v)}
	}
	return t, nil
}

func (s *Scope) GetInt(path string) (int, error)                { return GetAs[int](s, path) }
func (s *Scope) GetInt64(path string) (int64, error)            { return GetAs[int64](s, path) }
func (s *Scope) GetFloat64(path string) (float64, error)        { return GetAs[float64](s, path) }
func (s *Scope) GetString(path string) (string, error)          { return GetAs[string](s, path) }
func (s *Scope) GetBool(path string) (bool, error)              { return GetAs[bool](s, path) }
func (s *Scope) GetSlice(path string) ([]any, error)            { return GetAs[[]any](s, path) }
func (s *Scope) GetMap(path string) (map[string]any, error)     { return GetAs[map[string]any](s, path) }
```
Add godoc to each named method (one line each: "GetInt returns the value at path as an int …").

- [ ] **Step 2 — tests** `stage/get_test.go` (table): each getter hit; missing → `ErrPathNotFound`; wrong type → `*ScopeTypeError` with asserted `Expected`/`Actual`; `GetAs[[]any]`.

- [ ] **Step 3 — green + `BenchmarkGetInt`/`BenchmarkGetString`** in `get_test.go`.

- [ ] **Step 4 — commit** `feat(stage): strict typed Scope getters (GetInt/GetString/… + GetAs)`.

---

### Task 5: examples + benchmark summary

**Files:** Create `stage/provenance_example_test.go`; optional root `example_test.go` addition; run all benchmarks.

- [ ] **Step 1** — `ExampleScope_explain` (build a small pipeline with `stage.WithProvenance()`, run it, print `sc.Explain("...")`) and `ExampleScope_getInt` with `// Output:` blocks.
- [ ] **Step 2** — Run `go test ./... -bench . -benchmem -run '^$'`; capture the provenance-off vs baseline allocs and the on/getter/Explain numbers; put a short table in the final commit message.
- [ ] **Step 3** — Full gate; **commit** `docs(stage): runnable provenance/getter examples`.

---

## Post-implementation (delivery gate)

1. `/code-review` over `main..HEAD`, then `/security-review`; resolve/triage findings; re-run `-race` + `-bench`.
2. `go vet`, `gofmt`, `golangci-lint`, `go mod verify`, `govulncheck` (if available) clean; coverage ≥85% on `expr`/`stage`.
3. Update `docs/HANDOVER.md`.
4. **Ask the user** before merge/push (autonomy grant is spent).
