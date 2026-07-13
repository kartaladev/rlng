# Plan 023 — intra-stage MultiExpr local-alias provenance (B7)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Trace an intra-stage `MultiExpr` local reference: key such an input under its scope path
(`calc.base`) instead of the bare local name, so `Lineage`/`Explain` reconcile it to the earlier
expression that produced it (reusing B6's exact/ancestor reconciliation).

- **Implements:** Spec 023 (`docs/specs/023-multiexpr-local-alias-provenance.md`) — approved 2026-07-13.
- **Records:** ADR-0048 (rides in the feat commit); resolves ADR-0011's known limitation.
- **Backlog:** graduates B7 (`docs/BACKLOG.md`).

**Architecture:** One green commit, localized to `pipe`. Generalize `snapshotRefs` into a keyed variant
(`snapshotRefsKeyed(env, refs, keyOf)`); `snapshotRefs` delegates with `keyOf == nil` — so `single`/`table`
are byte-for-byte unchanged. `MultiExpr.Execute` tracks the set of earlier expression names in the stage and
passes a `keyOf` that rewrites a local-alias reference (first path segment names an earlier expr) to
`stage.<ref>`. B6's `derivationsFor` (exact → descendants → ancestor) then links `calc.base` (exact) or
`calc.base.x` (ancestor) to the producing derivation.

**Tech Stack:** Go 1.25+; no new dependencies (adds `strings` to `pipe/multi.go`).

## Global Constraints

- Pure Go, no cgo; no new dependencies.
- Blackbox tests only (`package pipe_test`); assert-closure tables (`table-test` skill); `t.Context()`.
- Every exported symbol keeps a godoc comment; no exported API signature change (provenance recording
  internals only).
- No `Hash()` or config-schema change (provenance recording only; the parsed def is untouched).
- Test-coverage gate: ≥ 85% on `pipe`; every new branch (local-alias qualify, D2 shadowing, member-alias
  ancestor) covered.

---

### Task 1: MultiExpr local-alias provenance (feat, one green commit)

**Files:**
- Modify: `pipe/provenance.go` — add `snapshotRefsKeyed`; `snapshotRefs` delegates to it.
- Modify: `pipe/multi.go` — `Execute` builds a `locals` set and passes a `keyOf`; add `qualifyLocal`;
  import `strings`.
- Test: `pipe/multi_test.go` — update the provenance case's `Inputs` key; add Lineage/Explain, D2
  shadowing, and member-alias cases (fold into the existing `table-test`).

**Interfaces:**
- Consumes: `lookupPath` (`pipe/scope.go`), `Scope.Derive`, `Scope.Lineage`, `Scope.Explain`, `derivationsFor`
  (B6 exact→descendants→ancestor).
- Produces: `snapshotRefsKeyed(env map[string]any, refs []string, keyOf func(string) string) map[string]any`
  (unexported); `(*MultiExpr).qualifyLocal(locals map[string]struct{}) func(string) string` (unexported).
  No exported signature change.

- [ ] **Step 1 (red): update + extend `pipe/multi_test.go`.**

  Change the existing provenance case's `taxed.Inputs` expectation to the qualified key, and add the new
  cases (Lineage trace, D2 shadowing, member alias). Replace the `taxed.Inputs` assertion:

```go
// was: assert.Equal(t, map[string]any{"base": 20.0}, taxed.Inputs)
assert.Equal(t, map[string]any{"calc.base": 20.0}, taxed.Inputs)
```

  Add these `table-test` cases to `TestMultiExprExecute`'s table (each `build` returns the stage + scope;
  `assert` inspects the run result):

```go
{
	name: "intra-stage local ref traces to its producer's lineage",
	build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
		m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
			{Name: "base", Expression: "price * qty", Priority: 0},
			{Name: "taxed", Expression: "base * 1.1", Priority: 1},
		})
		require.NoError(t, err)
		return m, pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0}, pipe.WithProvenance())
	},
	assert: func(t *testing.T, sc *pipe.Scope, err error) {
		require.NoError(t, err)
		lin := sc.Lineage("calc.taxed")
		paths := make([]string, len(lin))
		for i, d := range lin {
			paths[i] = d.Path
		}
		// seeds-first, and the intra-stage producer calc.base now appears.
		assert.Equal(t, []string{"price", "qty", "calc.base", "calc.taxed"}, paths)
		assert.Contains(t, sc.Explain("calc.taxed"), "calc.base =")
	},
},
{
	name: "D2 shadowing: first x reads seed (unqualified), later x reads local (qualified)",
	build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
		m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
			{Name: "x", Expression: "x + 1", Priority: 0}, // reads seed x
			{Name: "y", Expression: "x", Priority: 1},     // reads local x
		})
		require.NoError(t, err)
		return m, pipe.NewScope(map[string]any{"x": 10}, pipe.WithProvenance())
	},
	assert: func(t *testing.T, sc *pipe.Scope, err error) {
		require.NoError(t, err)
		dx, _ := sc.Derivation("calc.x")
		assert.Equal(t, map[string]any{"x": 10}, dx.Inputs) // seed key, unqualified
		dy, _ := sc.Derivation("calc.y")
		assert.Equal(t, map[string]any{"calc.x": 11}, dy.Inputs) // local, qualified
	},
},
{
	name: "member local alias reconciles to its producer via ancestor",
	build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
		m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
			{Name: "base", Expression: `{"k": price}`, Priority: 0}, // map-valued local
			{Name: "t", Expression: "base.k", Priority: 1},          // member read of local
		})
		require.NoError(t, err)
		return m, pipe.NewScope(map[string]any{"price": 7}, pipe.WithProvenance())
	},
	assert: func(t *testing.T, sc *pipe.Scope, err error) {
		require.NoError(t, err)
		dt, _ := sc.Derivation("calc.t")
		assert.Equal(t, map[string]any{"calc.base.k": 7}, dt.Inputs) // qualified member path
		assert.Contains(t, sc.Explain("calc.t"), "calc.base =")      // ancestor link
	},
},
```

  (If the existing table's `assert` signature differs, match it — the run harness at the bottom of
  `TestMultiExprExecute` already builds, runs, and calls `tc.assert`. Confirm the exact map-literal syntax
  `{"k": price}` compiles under expr-lang; if not, use `{k: price}`.)

- [ ] **Step 2 (red): run it, confirm failure.**

  Run: `go test ./pipe/ -run TestMultiExprExecute -v`
  Expected: FAIL — `taxed.Inputs` is `{"base": 20.0}` (bare key); Lineage omits `calc.base`; the D2/member
  cases fail on the qualified keys.

- [ ] **Step 3 (green): generalize `snapshotRefs` in `pipe/provenance.go`.**

```go
// snapshotRefs returns the subset of env named by refs (the paths an expression
// reads), as the Inputs of a Derivation. Each ref is resolved via lookupPath, so
// a member path ("a.b.c") yields the precise nested value while a single-segment
// ref stays a direct lookup; an unresolvable ref is omitted. It returns nil when
// refs is empty so a no-input derivation carries a nil (not empty) Inputs map.
func snapshotRefs(env map[string]any, refs []string) map[string]any {
	return snapshotRefsKeyed(env, refs, nil)
}

// snapshotRefsKeyed is snapshotRefs with an optional key transform: each ref is
// resolved from env by its own path, but recorded under keyOf(ref) when keyOf is
// non-nil. MultiExpr uses it to key an intra-stage local alias ("base") under its
// scope path ("calc.base") while still reading the value by the bare name, so
// Lineage/Explain can reconcile it to the earlier expression's derivation.
func snapshotRefsKeyed(env map[string]any, refs []string, keyOf func(string) string) map[string]any {
	if len(refs) == 0 {
		return nil
	}
	out := make(map[string]any, len(refs))
	for _, r := range refs {
		v, ok := lookupPath(env, r)
		if !ok {
			continue
		}
		key := r
		if keyOf != nil {
			key = keyOf(r)
		}
		out[key] = v
	}
	return out
}
```

- [ ] **Step 4 (green): qualify local aliases in `pipe/multi.go`.**

  Add `strings` to the import block. Rewrite `Execute`'s loop to track `locals` and pass a `keyOf`, and add
  `qualifyLocal`:

```go
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
```

  Note: `env[e.name] = v` moves to the end of the loop body (unified for both paths); it must run **after**
  the input snapshot so an expression's own name is not yet a local when its inputs are recorded, and
  `locals[e.name]` is added only in the tracking path (only provenance needs it).

- [ ] **Step 5 (green): run the pipe suite.**

  Run: `go test ./pipe/ -race`
  Expected: PASS — the migrated + new cases green; every other provenance/multi test unchanged (seed and
  cross-stage keys are not qualified; `single`/`table` use `snapshotRefs` with `keyOf == nil`).

- [ ] **Step 6 — docs & ADR:** Author **ADR-0048** (Nygard): context = ADR-0011 known limitation (intra-
  stage `MultiExpr` local aliases untraced because `Inputs` is keyed by bare name while the value lives at
  `stage.<name>`); decision = qualify a local-alias ref (first segment = earlier expr name) to `stage.<ref>`
  when recording `MultiExpr` inputs, via a keyed `snapshotRefs`, reusing B6 reconciliation; consequences =
  intra-stage lineage now traces, `Inputs`-keying contract change (MultiExpr local refs keyed by scope
  path), no signature/`Hash()`/config change, single/table unaffected. Move **B7** to the Resolved section
  of `docs/BACKLOG.md`. Update `docs/HANDOVER.md` to B7-done / B8-next. Update ADR-0011's known-limitation
  note with a "resolved by ADR-0048" pointer.

- [ ] **Step 7 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) /
  `go mod verify`; `pipe` coverage ≥ 85% with the new branches covered.

- [ ] **Step 8 — commit:** `feat(pipe): trace intra-stage MultiExpr local-alias provenance (B7)` with
  trailers `Spec: 023`, `Plan: 023`, `ADR: 0048`. (Plan + ADR + BACKLOG + HANDOVER ride in this commit.)

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item
(**B8** — per-element `foreach` lineage; a design-checkpoint item, so pause for the user's design approval
before implementing).
