# Plan 022 — precise member-path references in provenance (B6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Record each provenance input at its deepest statically-known member path (`grade.tier`,
`applicant.score`) with its precise value, and reconcile lineage to the exact derivation (or nearest
recorded ancestor), so `Explain`/`Lineage` stop over-linking to sibling outputs.

- **Implements:** Spec 022 (`docs/specs/022-member-path-provenance.md`) — approved 2026-07-13.
- **Records:** ADR-0047 (rides in Task 2's commit); refines ADR-0011 point 4 & Consequences.
- **Backlog:** graduates B6 (`docs/BACKLOG.md`).

**Architecture:** Two green commits. **Task 1** makes `pipe`'s provenance reconciliation path-aware —
`snapshotRefs` resolves via `lookupPath`, and `derivationsFor` gains a nearest-ancestor fallback — a
behavior-preserving change for today's top-level refs (single-segment `lookupPath` == `env[r]`; the
ancestor branch is unreachable until refs are member paths, and is covered via the public `Derive`+`Explain`
surface). **Task 2** redefines `expr.references()` to emit deepest static member paths, updates the
`References()` godoc, and tightens the now-precise test expectations.

**Tech Stack:** Go 1.25+, `github.com/expr-lang/expr` (`ast`, `parser` subpackages, already imported).

## Global Constraints

- Pure Go, no cgo; no new dependencies (`expr-lang/expr/ast` + `/parser` are already used by `expr/refs.go`).
- Blackbox tests only (`package expr_test` / `package pipe_test`); assert-closure tables (`table-test`
  skill); `t.Context()` over `context.Background()`.
- Every exported symbol keeps a godoc comment; `References()` signature (`[]string`) is unchanged — only its
  documented semantics change (BREAKING semantics, provenance-only consumer).
- No `Hash()` or config-schema change (this is `expr`/`pipe` provenance only; the parsed def is untouched).
- Test-coverage gate: ≥ 85% on changed packages; every new/changed hot-path branch covered.

---

### Task 1: Path-aware provenance reconciliation in `pipe` (feat, green)

**Files:**
- Modify: `pipe/provenance.go` — `snapshotRefs` (use `lookupPath`); `derivationsFor` (nearest-ancestor
  fallback). `strings` is already imported.
- Test: `pipe/provenance_ancestor_test.go` (create) — the ancestor-fallback branch via `Derive`+`Explain`.

**Interfaces:**
- Consumes: `lookupPath(m map[string]any, path string) (any, bool)` (`pipe/scope.go`, unexported, same
  package); `Scope.Derive(path string, v any, d Derivation) error`, `Scope.Explain(path) string` (exported).
- Produces: no signature change; `derivationsFor` and `snapshotRefs` behavior widened (path-aware).

- [ ] **Step 1 (red): write the ancestor-fallback test.**

  Create `pipe/provenance_ancestor_test.go` (`package pipe_test`). It drives the new branch through the
  public API: a provenance Scope with a nested **seed** `applicant`, then a hand-built `Derive` whose
  `Inputs` names the member path `applicant.score`; `Explain` must reconcile that input up to the seed
  `applicant`.

```go
package pipe_test

import (
	"strings"
	"testing"

	"github.com/kartaladev/rlng/pipe"
)

// A Derivation input keyed by a member path (applicant.score) whose value lives
// under a top-level seed (applicant) reconciles to that seed via the
// nearest-ancestor fallback in derivationsFor.
func TestExplain_MemberPathInputLinksToSeedAncestor(t *testing.T) {
	sc := pipe.NewScope(map[string]any{"applicant": map[string]any{"score": 700}}, pipe.WithProvenance())
	// A stage-like write whose Inputs reference the seed by member path.
	if err := sc.Derive("decision.ok", true, pipe.Derivation{
		Stage:      "decision",
		StageType:  pipe.TypeSingleExpr,
		Operation:  "eval",
		Expression: "applicant.score >= 650",
		Inputs:     map[string]any{"applicant.score": 700},
	}); err != nil {
		t.Fatalf("Derive: %v", err)
	}
	out := sc.Explain("decision.ok")
	if !strings.Contains(out, "applicant =") || !strings.Contains(out, "[seed]") {
		t.Fatalf("Explain did not link member-path input to the seed ancestor:\n%s", out)
	}
}
```

- [ ] **Step 2 (red): run it, confirm it fails.**

  Run: `go test ./pipe/ -run TestExplain_MemberPathInputLinksToSeedAncestor -v`
  Expected: FAIL — `Explain` output omits the `applicant` seed (today `derivationsFor("applicant.score")`
  has no exact entry, no `applicant.score.*` descendants, and no ancestor step).

- [ ] **Step 3 (green): add the nearest-ancestor fallback to `derivationsFor`.**

  In `pipe/provenance.go`, replace `derivationsFor`:

```go
// derivationsFor returns the derivation(s) that produced the value at key, in
// order of precision: the derivation recorded exactly at key; else every
// derivation under the key namespace (key + ".", from the precomputed index);
// else the nearest recorded ancestor (walking a.b.c -> a.b -> a), which links a
// member-path input to the top-level seed (or coarser output) that contains it.
func derivationsFor(derivations map[string]Derivation, idx map[string][]Derivation, key string) []Derivation {
	if d, ok := derivations[key]; ok {
		return []Derivation{d}
	}
	if ds := idx[key]; len(ds) > 0 {
		return ds
	}
	for i := len(key) - 1; i > 0; i-- {
		if key[i] == '.' {
			if d, ok := derivations[key[:i]]; ok {
				return []Derivation{d}
			}
		}
	}
	return nil
}
```

- [ ] **Step 4 (green): make `snapshotRefs` path-aware.**

  In `pipe/provenance.go`, change the lookup to resolve dot-paths:

```go
// snapshotRefs returns the subset of env named by refs (the paths an expression
// reads), as the Inputs of a Derivation. Each ref is resolved via lookupPath, so
// a member path (a.b.c) yields the precise nested value; an unresolvable ref is
// omitted. Returns nil when refs is empty so a no-input derivation carries a nil
// (not empty) Inputs map.
func snapshotRefs(env map[string]any, refs []string) map[string]any {
	if len(refs) == 0 {
		return nil
	}
	out := make(map[string]any, len(refs))
	for _, r := range refs {
		if v, ok := lookupPath(env, r); ok {
			out[r] = v
		}
	}
	return out
}
```

- [ ] **Step 5 (green): run the test + the whole pipe suite.**

  Run: `go test ./pipe/ -race`
  Expected: PASS — the new test passes; every existing provenance test stays green (top-level refs are
  single-segment, so `lookupPath` == `env[r]`, and the ancestor branch is unreached by existing refs).

- [ ] **Step 6 — commit:** `feat(pipe): path-aware provenance reconciliation (B6)` (Backlog B6, Spec 022,
  Plan 022). Body: prep for member-path refs — `snapshotRefs` resolves via `lookupPath`, `derivationsFor`
  gains a nearest-ancestor fallback; behavior-preserving for top-level refs.

---

### Task 2: Member-path references in `expr` (feat, green — the behavior change)

**Files:**
- Modify: `expr/refs.go` — rewrite `references()` + `refVisitor.Visit`; add `staticPath` and
  `isStrictPrefixOfAny`; import `strings`.
- Modify: `expr/function.go` (godoc on `References`), `expr/predicate.go` (godoc on `References`).
- Test: `expr/refs_test.go` — update member-access expectations; add dynamic-index + method-call cases.
- Test (tighten): the `pipe` provenance/`Explain`/`Lineage` tests that assert a cross-stage member read's
  lineage — narrow the expected output to the precise subtree (grep below).

**Interfaces:**
- Consumes: `github.com/expr-lang/expr/ast` (`IdentifierNode`, `MemberNode{Node, Property, Method}`,
  `StringNode{Value}`, `CallNode{Callee}`), `parser.Parse`.
- Produces: `references(src) []string` now returns deepest static member paths; `Function.References()` /
  `Predicate.References()` semantics change (values only; signature `[]string` unchanged).

- [ ] **Step 1 (red): update `expr/refs_test.go` expectations to member paths + add new cases.**

  Change the existing "member access uses top-level" case and add dynamic/method cases. The file is a
  `table-test`; fold the new cases into the existing table:

```go
{
	name: "member access uses deepest static path",
	expr: "tiers.tag + base",
	assert: func(t *testing.T, refs []string) {
		assert.Equal(t, []string{"base", "tiers.tag"}, refs)
	},
},
{
	name: "nested member chain drops intermediates",
	expr: "a.b.c + a.b.d",
	assert: func(t *testing.T, refs []string) {
		assert.Equal(t, []string{"a.b.c", "a.b.d"}, refs)
	},
},
{
	name: "dynamic index stops the static chain and keeps the index ref",
	expr: "items[i].price",
	assert: func(t *testing.T, refs []string) {
		assert.Equal(t, []string{"i", "items"}, refs)
	},
},
{
	name: "bracket string access is a static path",
	expr: `a["b"]`,
	assert: func(t *testing.T, refs []string) {
		assert.Equal(t, []string{"a.b"}, refs)
	},
},
{
	name: "method call is not a data path; receiver is",
	expr: "name.startsWith(prefix)",
	assert: func(t *testing.T, refs []string) {
		assert.Equal(t, []string{"name", "prefix"}, refs)
	},
},
```

  (Keep the existing "identifiers deduped and sorted", "literal only has no refs", and call-callee cases —
  a bare-identifier callee like `discount(x)` still yields `["x"]`.)

- [ ] **Step 2 (red): run it, confirm failure.**

  Run: `go test ./expr/ -run TestFunctionReferences -v`
  Expected: FAIL — current `references()` returns `["base","tiers"]`, not `["base","tiers.tag"]`.

- [ ] **Step 3 (green): rewrite `expr/refs.go`.**

```go
package expr

import (
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// references returns the sorted, unique referenced paths read by src: the
// deepest statically-known member path per reference (a.b.c), or the bare
// identifier when the chain is not statically resolvable (dynamic/index access,
// method calls). src is already known to compile, so a parse failure yields nil.
// Computed once at compile time and cached on the Function/Predicate.
func references(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}
	v := &refVisitor{paths: map[string]struct{}{}, callees: map[string]struct{}{}}
	node := tree.Node
	ast.Walk(&node, v)
	if len(v.paths) == 0 {
		return nil
	}
	// Exclude call callees (function names) and any path that is a strict prefix
	// (proper ancestor) of another collected path — deepest static path wins.
	all := make([]string, 0, len(v.paths))
	for p := range v.paths {
		if _, isCallee := v.callees[p]; isCallee {
			continue
		}
		all = append(all, p)
	}
	refs := make([]string, 0, len(all))
	for _, p := range all {
		if isStrictPrefixOfAny(p, all) {
			continue
		}
		refs = append(refs, p)
	}
	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return refs
}

type refVisitor struct {
	paths   map[string]struct{}
	callees map[string]struct{}
}

func (r *refVisitor) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.IdentifierNode:
		r.paths[n.Value] = struct{}{}
	case *ast.MemberNode:
		if p, ok := staticPath(*node); ok {
			r.paths[p] = struct{}{}
		}
	case *ast.CallNode:
		// A call callee (e.g. `discount` in `discount(x)`) is a function name
		// supplied by the env, not a data field; exclude it. Method-call members
		// (`foo.bar()`, MemberNode.Method) are never recorded as a path by
		// staticPath, so only the receiver identifier survives.
		if id, ok := n.Callee.(*ast.IdentifierNode); ok {
			r.callees[id.Value] = struct{}{}
		}
	}
}

// staticPath returns the dot-path of a fully static member chain rooted at an
// identifier (a, a.b, a["b"]), or ok=false for a dynamic/index property or a
// method-call member. Builtins are BuiltinNode, not handled here.
func staticPath(n ast.Node) (string, bool) {
	switch t := n.(type) {
	case *ast.IdentifierNode:
		return t.Value, true
	case *ast.MemberNode:
		if t.Method {
			return "", false
		}
		prop, ok := t.Property.(*ast.StringNode)
		if !ok {
			return "", false
		}
		base, ok := staticPath(t.Node)
		if !ok {
			return "", false
		}
		return base + "." + prop.Value, true
	default:
		return "", false
	}
}

// isStrictPrefixOfAny reports whether p is a proper ancestor path of some other
// entry in all (q == p + "." + …), so intermediate paths (a, a.b for a.b.c) drop
// out in favor of the deepest.
func isStrictPrefixOfAny(p string, all []string) bool {
	for _, q := range all {
		if len(q) > len(p) && strings.HasPrefix(q, p+".") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4 (green): update `References()` godoc** in `expr/function.go` and `expr/predicate.go`:

```go
// References returns the sorted, unique paths this Function reads: the deepest
// statically-known member path per reference (e.g. "grade.tier"), or the bare
// identifier when the chain is not statically resolvable (dynamic/index access,
// method calls). Computed once at compile time. Used to record provenance inputs.
func (f *Function) References() []string { return f.refs }
```

  (Mirror the wording for `Predicate.References()`.)

- [ ] **Step 5 (green): run the expr suite.**

  Run: `go test ./expr/ -race`
  Expected: PASS — all `references` cases green.

- [ ] **Step 6 (green): tighten the `pipe` provenance tests to the precise output.**

  Find provenance/`Explain`/`Lineage` tests whose stages read a cross-stage member path and assert the
  (previously over-broad) lineage:

```bash
grep -rn "Explain\|Lineage" pipe/*_test.go | grep -i "test\|assert" 
grep -rln "Explain(" pipe/*_test.go
```

  For each, the value that reads `X.k` now records input `X.k` (exact) instead of top-level `X`
  (descendants), so `Explain` no longer prints sibling `X.*` outputs it never read. Update the expected
  strings/derivation sets to the precise subtree. Do **not** weaken an assertion to hide a change — assert
  the sibling is now *absent*. Run `go test ./pipe/ -race` until green.

- [ ] **Step 7 — docs & ADR:** Author **ADR-0047** (Nygard): context = ADR-0011 point 4 recorded top-level
  identifiers and flagged precise member paths as a future refinement; decision = redefine `References()` to
  deepest static member paths (provenance-only consumer, no functional blast radius), `snapshotRefs`
  resolves via `lookupPath`, `derivationsFor` gains a nearest-ancestor fallback (exact → descendants →
  ancestor); consequences = precise `Explain`/`Lineage`, `References()` semantics change (no signature
  break, no `Hash()`/config change), deepest-path dedup degradation documented, B7 still separate. Move
  **B6** to the Resolved section of `docs/BACKLOG.md` (closing increment 022 / ADR-0047). Update
  `docs/HANDOVER.md` to B6-done / B7-next.

- [ ] **Step 8 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) /
  `go mod verify`; `expr` and `pipe` coverage ≥ 85% with the new branches covered (member chain,
  dynamic-index fallback, method-call exclusion, deepest-path dedup, ancestor reconciliation).

- [ ] **Step 9 — commit:** `feat(expr)!: member-path references in provenance (B6)` with a body noting the
  `References()` semantics change (no signature break; provenance-only consumer) and trailers `Spec: 022`,
  `Plan: 022`, `ADR: 0047`. (Plan + ADR + BACKLOG + HANDOVER ride in this commit.)

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item
(**B7** — intra-stage `MultiExpr` local-alias provenance; a design-checkpoint item, so pause for the user's
design approval before implementing).
