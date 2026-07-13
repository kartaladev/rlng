# Plan 024 — per-element foreach lineage (B8)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Surface each `foreach` element's derivation graph on the outer scope so
`Explain`/`Lineage`/`Derivations` answer per-element lineage — by merging the per-element derivations under
a `<name>[i].` path prefix (always-on when the outer scope tracks provenance).

- **Implements:** Spec 024 (`docs/specs/024-foreach-per-element-lineage.md`) — approved 2026-07-13.
- **Records:** ADR-0049 (rides in the feat commit); resolves ADR-0040 D5 / Spec 015 D5.
- **Backlog:** graduates B8 (`docs/BACKLOG.md`).

**Architecture:** One green commit, localized to `pipe`. A new unexported `Scope` merge helper
(`recordElementDerivations`) inserts a prefix-rewritten copy of a per-element scope's derivations (path +
`Inputs` keys) into the outer scope's derivation map; `ForEach.Execute` calls it per element right after the
existing firing recording. B6's `derivationsFor` (exact → descendants → ancestor) then reconciles each
element's prefixed subgraph. No change to `Lineage`/`Explain`/`derivationsFor`/`Derivation`.

**Tech Stack:** Go 1.25+; no new dependencies.

## Global Constraints

- Pure Go, no cgo; no new dependencies.
- Blackbox tests only (`package pipe_test`); assert-closure tables (`table-test` skill); `t.Context()`.
- No exported API signature change (a new unexported `Scope` method + a `ForEach.Execute` internal call).
- No `Hash()` or config-schema change; `items`/rollup data outputs and eval semantics unchanged; zero cost
  when provenance is off.
- Test-coverage gate: ≥ 85% on `pipe`; the new merge branch + the provenance-off no-op covered.

---

### Task 1: Per-element foreach lineage (feat, one green commit)

**Files:**
- Modify: `pipe/provenance.go` — add `(*Scope).recordElementDerivations`.
- Modify: `pipe/foreach.go` — call it per element in `Execute` after firing recording (`fmt` already
  imported).
- Test: `pipe/foreach_test.go` — add lineage cases to `TestForEachExecute` (provenance on: Explain/Lineage/
  independence; provenance off: no per-element derivations, firing intact).

**Interfaces:**
- Consumes: `Scope.Derivations()` (exported, locked copy), `Scope.TracksProvenance()`, `Scope.Explain`,
  `Scope.Lineage`, `Scope.Derivation`, `derivationsFor` (B6).
- Produces: `(*Scope).recordElementDerivations(prefix string, src map[string]Derivation)` (unexported).

- [ ] **Step 1 (red): add lineage cases to `pipe/foreach_test.go`.**

  Fold these into `TestForEachExecute`'s `cases` slice (the harness at the bottom builds, runs, and calls
  `tc.assert`). They build a foreach whose inner pipeline is a decision table reading `item.ltv`:

```go
{
	name: "per-element lineage merged under <stage>[i] with provenance on",
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
				map[string]any{"ltv": 30}, // fires LOW
			},
		}, pipe.WithProvenance())
		return fe, sc
	},
	assert: func(t *testing.T, sc *pipe.Scope, err error) {
		require.NoError(t, err)

		// Element 0's decision output is a derivation on the outer scope.
		d0, ok := sc.Derivation("lines[0].check.flag")
		require.True(t, ok)
		assert.Equal(t, "check", d0.Stage)
		assert.Equal(t, "high", d0.Value)

		// Explain traces element 0 back to its element seed (via the B6
		// ancestor fallback under the prefix).
		ex0 := sc.Explain("lines[0].check.flag")
		assert.Contains(t, ex0, "lines[0].check.flag = high")
		assert.Contains(t, ex0, "lines[0].item")
		assert.Contains(t, ex0, "[seed]")

		// Element 1 is independent and reconciles within its own prefix.
		d1, ok := sc.Derivation("lines[1].check.flag")
		require.True(t, ok)
		assert.Equal(t, "low", d1.Value)
		assert.NotEmpty(t, sc.Lineage("lines[1].check.flag"))

		// Per-element firing is still recorded (regression).
		f0 := sc.FiringRulesFor("lines[0]")
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
		assert.Len(t, sc.FiringRulesFor("lines[0]"), 1)
	},
},
```

- [ ] **Step 2 (red): run it, confirm failure.**

  Run: `go test ./pipe/ -run TestForEachExecute -v`
  Expected: FAIL on the provenance-on case — `sc.Derivation("lines[0].check.flag")` is absent (the
  per-element graph is discarded today). The provenance-off case passes already (nothing recorded).

- [ ] **Step 3 (green): add `recordElementDerivations` to `pipe/provenance.go`.**

```go
// recordElementDerivations merges src (a per-element scope's derivations) into s
// under prefix, rewriting each derivation's Path and its Inputs keys to
// prefix + "." + <original> so the element's subgraph reconciles within s (via
// derivationsFor's exact/descendants/ancestor logic). It is a no-op when s does
// not track provenance or src is empty. ForEach uses it to surface per-element
// lineage under the composite key "<stage>[i]", mirroring per-element firing.
func (s *Scope) recordElementDerivations(prefix string, src map[string]Derivation) {
	if !s.provenance || len(src) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range src {
		nd := d
		nd.Path = prefix + "." + d.Path
		if len(d.Inputs) > 0 {
			ins := make(map[string]any, len(d.Inputs))
			for k, v := range d.Inputs {
				ins[prefix+"."+k] = v
			}
			nd.Inputs = ins
		}
		s.derivations[nd.Path] = nd
	}
}
```

- [ ] **Step 4 (green): call it per element in `pipe/foreach.go`.**

  In `Execute`, immediately after the firing-recording block, before `items = append(...)`:

```go
		if elemFirings := esc.FiringRules(); len(elemFirings) > 0 {
			sc.recordFirings(fmt.Sprintf("%s[%d]", f.name, i), elemFirings)
		}
		if sc.TracksProvenance() {
			sc.recordElementDerivations(fmt.Sprintf("%s[%d]", f.name, i), esc.Derivations())
		}
		items = append(items, esc.Snapshot())
```

  Update `Execute`'s doc comment to note that, when the outer scope tracks provenance, each element's full
  derivation graph is merged under `<name>[i]` (queryable via `Lineage`/`Explain`), alongside the firing.

- [ ] **Step 5 (green): run the pipe suite.**

  Run: `go test ./pipe/ -race`
  Expected: PASS — the new provenance-on case traces per-element lineage; the provenance-off case records
  nothing; every existing foreach/firing/provenance test is unchanged.

- [ ] **Step 6 — docs & ADR:** Author **ADR-0049** (Nygard): context = ADR-0040 D5 deferral (per-element
  derivation graph discarded, only firing surfaced); decision = merge each element's derivations onto the
  outer scope under `<name>[i].` prefix (path + Inputs keys rewritten), always-on when provenance is on,
  reusing B6 reconciliation; consequences = per-element `Explain`/`Lineage` now answerable, N×graph memory
  bounded by the traced collection (debug-only), no data/`Hash()`/config/eval change, header-output
  derivation still out of scope. Move **B8** to the Resolved section of `docs/BACKLOG.md`. Update
  `docs/HANDOVER.md` to B8-done / B9-next. Add a "resolved by ADR-0049" pointer to ADR-0040's D5 note.

- [ ] **Step 7 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) /
  `go mod verify`; `pipe` coverage ≥ 85% with the new merge + no-op branches covered.

- [ ] **Step 8 — commit:** `feat(pipe): surface per-element foreach lineage (B8)` with trailers
  `Spec: 024`, `Plan: 024`, `ADR: 0049`. (Plan + ADR + BACKLOG + HANDOVER ride in this commit.)

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item
(**B9** — nested `foreach` support; a design-checkpoint item, so pause for the user's design approval
before implementing).
