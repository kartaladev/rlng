# Plan 025 — nested foreach support (B9)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make nested `foreach` a first-class declarative feature with correct per-element provenance at any
depth — lift the config-layer gate, fix firing to compose hierarchically (like derivations already do), and
reject `as` collisions along a nesting chain at build time.

- **Implements:** Spec 025 (`docs/specs/025-nested-foreach.md`) — approved 2026-07-13, committed `47e77df`.
- **Records:** ADR-0050 (created in Task 1, rides in that feat commit); supersedes ADR-0040 D7 and adds a
  superseding note to ADR-0049 / Spec 015 D5 (the firing-query shape).
- **Backlog:** graduates B9 (`docs/BACKLOG.md`).

**Architecture:** The `pipe` layer stays nesting-agnostic (ADR-0040). The only `pipe` change is replacing
`ForEach.Execute`'s firing **flatten** (one `<name>[i]` key) with a firing **map re-prefix**
(`<name>[i].<inner key>`), mirroring the existing `recordElementDerivations`; this composes to any depth for
free because a nested `ForEach` already produced the inner composite key in the per-element scope. Nested
lineage needs **no** new code — derivations already compose via the per-element `recordElementDerivations`
call. The `config` layer deletes the `ErrNestedForEach` gate (nesting then builds by existing `isd.build`
recursion) and adds a build-time `as`-chain guard.

**Tech Stack:** Go 1.25+; no new dependencies.

## Global Constraints

- Pure Go, no cgo; no new dependencies; Go 1.25+.
- Blackbox tests only (external `_test` package); assert-closure tables (`table-test` skill), never
  `want`/`wantErr` fields; `t.Context()` over `context.Background()`.
- Exported sentinels are the debuggability contract: add `config.ErrForEachAsCollision`; remove the now-dead
  `config.ErrNestedForEach`. Both are deliberate pre-1.0 API changes recorded in ADR-0050.
- No `Hash()` or config-schema change — lifting the gate does not alter the parsed `PipelineDef` shape, so
  pre-025 rulesets hash byte-identically (guarded by the existing golden-hash test; do not regenerate it).
- `FiringRule.Stage` keeps its meaning (the bare decision-table stage name); nesting lives only in the map
  key.
- Test-coverage gate: target ≥ 85% on `pipe` and `config`; every new/changed branch covered —
  `recordElementFirings` (non-empty re-key, empty no-op), `validateForEachAsChains` (collision hit, distinct
  pass, sibling reuse, non-foreach skip).
- Two accepted breaking changes: firing-query shape (`FiringRulesFor("<name>[i]")` →
  `FiringRulesFor("<name>[i].<inner>")`) and `ErrNestedForEach` removal.

---

### Task 1: Hierarchical firing re-prefix (`pipe`) + contract-ripple updates

Replace the firing flatten with a map re-prefix so per-element firing keeps the inner stage (and, nested,
the inner element index). Update every call site that queried the old flat `<name>[i]` key. Author ADR-0050
and stage plan 025 with this feat commit (docs ride with the feat).

**Files:**
- Modify: `pipe/firing.go` (add `firingMap` + `recordElementFirings` after `recordFirings`, ~line 33)
- Modify: `pipe/foreach.go:192-194` (swap flatten → map re-prefix)
- Modify: `pipe/foreach_test.go` (update 3 single-level firing queries; add nested-firing + empty-no-op cases)
- Modify: `config/foreach_test.go:115-121` (single-level firing query → new key shape)
- Modify: `examples/foreach_lineitem_test.go:105` (firing query → new key shape; `// Output:` unchanged)
- Create: `docs/adrs/0050-nested-foreach.md`
- (plan `docs/plans/025-nested-foreach.md` already exists — staged in this commit)

**Interfaces:**
- Produces: `(*Scope).firingMap() map[string][]FiringRule` (unexported); `(*Scope).recordElementFirings(prefix string, src map[string][]FiringRule)` (unexported). New firing key shape: `"<stage>[i].<inner stage key>"`, e.g. `"lines[0].check"` (single level), `"lines[0].taxes[1].vat"` (nested).
- Consumes: existing `FiringRule`, `s.firing map[string][]FiringRule`, `s.mu sync.RWMutex`.

- [ ] **Step 1: Write the failing nested-firing test**

Add this case to the `cases` table in `TestForEachExecute` in `pipe/foreach_test.go` (insert before the
closing `provenancePropagationCase(),` line ~837):

```go
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pipe/ -run 'TestForEachExecute/nested_foreach_preserves' -v`
Expected: FAIL — today `esc.FiringRules()` flattens the inner map, so the firing lands under `lines[0]`;
`FiringRulesFor("lines[0].taxes[1].vat")` returns nil (`require.Len(f01, 1)` fails).

- [ ] **Step 3: Add the two `Scope` methods**

In `pipe/firing.go`, immediately after `recordFirings` (ends ~line 33), add:

```go
// firingMap returns a shallow copy of the raw firing map (composite stage key ->
// firing rules) so a caller can re-key it without holding the lock. The
// FiringRule slices are shared (recorded firings are treated as immutable).
// Returns nil when nothing has fired.
func (s *Scope) firingMap() map[string][]FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.firing) == 0 {
		return nil
	}
	out := make(map[string][]FiringRule, len(s.firing))
	for k, v := range s.firing {
		out[k] = v
	}
	return out
}

// recordElementFirings merges src (a per-element scope's firing map) into s,
// re-keying each entry to prefix + "." + key. A foreach uses it to surface each
// element's firing under "<stage>[i].<inner stage key>", preserving the inner
// stage — and, for a nested foreach, the inner element index — instead of
// flattening it away. Always recorded (independent of provenance, like
// recordFirings); a no-op when src is empty. Mirrors recordElementDerivations.
func (s *Scope) recordElementFirings(prefix string, src map[string][]FiringRule) {
	if len(src) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string][]FiringRule, len(src))
	}
	for k, rules := range src {
		s.firing[prefix+"."+k] = rules
	}
}
```

- [ ] **Step 4: Swap the flatten for the re-prefix in `ForEach.Execute`**

In `pipe/foreach.go`, replace lines 192-194:

```go
		if elemFirings := esc.FiringRules(); len(elemFirings) > 0 {
			sc.recordFirings(fmt.Sprintf("%s[%d]", f.name, i), elemFirings)
		}
```

with:

```go
		if elemFirings := esc.firingMap(); len(elemFirings) > 0 {
			sc.recordElementFirings(fmt.Sprintf("%s[%d]", f.name, i), elemFirings)
		}
```

Also update the `Execute` doc comment (the sentence describing per-element firing, ~lines 143-146): change
"recorded on sc under the composite stage key `<name>[i]`" to "recorded on sc under the composite key
`<name>[i].<inner stage>` (e.g. `<name>[i].<table>`), preserving the inner stage — and, for a nested
foreach, the inner element index".

- [ ] **Step 5: Run the nested-firing test to verify it passes**

Run: `go test ./pipe/ -run 'TestForEachExecute/nested_foreach_preserves' -v`
Expected: PASS.

- [ ] **Step 6: Update the single-level firing queries broken by the contract change**

The flat `<name>[i]` key is gone; each single-level query gains the inner DT name. Make these exact edits:

`pipe/foreach_test.go` — in the case named `"per-element firing recorded under the composite stage key <stage>[i]"` (inner DT is `"check"`):
- line 743: `sc.FiringRulesFor("lines[0]")` → `sc.FiringRulesFor("lines[0].check")`
- line 748: `sc.FiringRulesFor("lines[1]")` → `sc.FiringRulesFor("lines[1].check")`
- line 751: `sc.FiringRulesFor("lines[2]")` → `sc.FiringRulesFor("lines[2].check")`
- (leave line 758 `sc.FiringRulesFor("check")` — the bare inner name is still not a key; it must stay empty)
- rename the case to `"per-element firing recorded under the composite key <stage>[i].<inner>"`

`pipe/foreach_test.go` — case `"per-element lineage merged under <stage>[i] with provenance on"`:
- line 807: `sc.FiringRulesFor("lines[0]")` → `sc.FiringRulesFor("lines[0].check")`

`pipe/foreach_test.go` — case `"no per-element derivations recorded when provenance is off"`:
- line 834: `sc.FiringRulesFor("lines[0]")` → `sc.FiringRulesFor("lines[0].check")`

`config/foreach_test.go` — case `"valid foreach with inner decision-table fires per element"` (inner DT is `"check"`):
- line 115: `sc.FiringRulesFor("lines[0]")` → `sc.FiringRulesFor("lines[0].check")`
- line 119: `sc.FiringRulesFor("lines[1]")` → `sc.FiringRulesFor("lines[1].check")`

`examples/foreach_lineitem_test.go` (inner DT is `"check"`):
- line 105: `sc.FiringRulesFor("lines[2]")` → `sc.FiringRulesFor("lines[2].check")`
  (The printed rule ID/message are unchanged, so the `// Output:` block is untouched.)

- [ ] **Step 7: Add the empty-firing no-op case**

Add to the `TestForEachExecute` table in `pipe/foreach_test.go`:

```go
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
```

- [ ] **Step 8: Author ADR-0050**

Create `docs/adrs/0050-nested-foreach.md`:

```markdown
# ADR-0050 — nested foreach: hierarchical firing keys, config gate lift, `as`-chain guard

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 025 / Plan 025, graduating backlog B9 — the nested-foreach deferral recorded in
  ADR-0040 D7 (enforced by config.ErrNestedForEach).

## Context

Nested `foreach` already ran programmatically (ADR-0040 kept the `pipe` layer nesting-agnostic), but the
config layer rejected it (ErrNestedForEach), and "runs" was not "works well": the outer recorded an
element's inner firings via a flatten (`esc.FiringRules()`) that discarded the inner element index, and
outer/inner both defaulting `as: item` silently shadowed the outer element. Derivations, by contrast,
already composed via the per-element `recordElementDerivations` call (a nested ForEach runs inside the
per-element scope, so its derivations were already keyed `<inner>[j].…`).

## Decision

- **Firing composes by hierarchical re-prefix, mirroring derivations.** `ForEach.Execute` now copies the
  per-element scope's firing *map* under a `<stage>[i].` prefix (`recordElementFirings`) instead of
  flattening it into one key. Keys end at the decision-table stage (`lines[0].check`;
  `lines[0].taxes[1].vat`), composing to any depth. `FiringRule.Stage` keeps its meaning (the bare DT name);
  nesting lives only in the map key. No nesting-detection logic enters `ForEach` — the change is uniform for
  single-level and nested.
- **Config gate lifted; nesting builds by existing recursion.** `buildForEach` already builds inner stages
  via `isd.build`, which dispatches a `type: foreach` inner stage back into `buildForEach`; deleting the
  ErrNestedForEach rejection is sufficient.
- **`as`-chain collision rejected at build time.** `validateForEachAsChains` (called from `Build`) rejects
  any root-to-leaf nesting chain that reuses an effective `as` name (`config.ErrForEachAsCollision`, naming
  both stages). Siblings on different chains may reuse a name. The check lives at the config layer, the
  altitude where ADR-0040 placed nesting-aware validation.
- **No static nesting-depth cap.** Nesting depth is author-controlled static config, and `ctx.Err()` is
  already checked per element at every level, so runaway iteration is cancellable. Multiplicative fan-out is
  documented, not capped.

## Consequences

- **Two breaking pre-1.0 API changes:** (1) the single-level firing query shape moves from
  `FiringRulesFor("<name>[i]")` (flat aggregate) to `FiringRulesFor("<name>[i].<inner>")` — a superseding
  note to ADR-0049 / Spec 015 D5; (2) `config.ErrNestedForEach` is removed (a never-firing sentinel is
  misleading surface).
- **Nested lineage works with zero new derivation code** — the per-element merge composes to full nested
  paths (`lines[0].taxes[1].vat.amount`), reconciled by B6's exact/ancestor logic.
- **Debuggability preserved:** typed `*StageError` at every level (nested inner errors name each index),
  typed `*ConfigError` wrapping `ErrForEachAsCollision`, no panics on caller input.

## Traceability

Spec: 025. Plan: 025. Supersedes: ADR-0040 D7 (nested deferral). Superseding note: ADR-0049 / Spec 015 D5
(firing-query shape). Related: ADR-0047 (B6 reconciliation, reused for nested lineage), ADR-0006
(deterministic sequential execution, preserved at every level).
```

- [ ] **Step 9: Run the full `pipe`, `config`, and `examples` suites**

Run: `go build ./... && go test ./pipe/ ./config/ ./examples/ -race`
Expected: PASS (all firing-query ripples resolved).

- [ ] **Step 10: Commit**

```bash
git add pipe/firing.go pipe/foreach.go pipe/foreach_test.go config/foreach_test.go \
        examples/foreach_lineitem_test.go docs/adrs/0050-nested-foreach.md docs/plans/025-nested-foreach.md
git commit -m "$(cat <<'EOF'
feat(pipe)!: hierarchical per-element firing keys for nested foreach (B9)

Replace ForEach.Execute's firing flatten with a map re-prefix
(recordElementFirings), so per-element firing keeps the inner stage and,
for a nested foreach, the inner element index: keys become
"<name>[i].<inner>" (e.g. lines[0].taxes[1].vat), composing to any depth.
The pipe layer stays nesting-agnostic. FiringRule.Stage is unchanged.

BREAKING (pre-1.0): single-level firing query moves from
FiringRulesFor("<name>[i]") to FiringRulesFor("<name>[i].<inner>").

Plan 025 + ADR-0050 ride here.

Spec: 025
Plan: 025
ADR: 0050
EOF
)"
```

---

### Task 2: Nested provenance & semantics tests (`pipe`, test-only)

Prove the behaviors that compose "for free" once Task 1 lands: nested lineage, nested output shape, an outer
rollup over a nested value, cancellation, and a nested inner error naming both indices. No non-test code.

**Files:**
- Modify: `pipe/foreach_test.go` (add nested cases to the `TestForEachExecute` table)

**Interfaces:**
- Consumes: Task 1's firing key shape; existing `WithProvenance`, `recordElementDerivations` (unchanged),
  `ErrForEachNotList`.

- [ ] **Step 1: Write the nested-lineage + output + rollup case**

Add to the `TestForEachExecute` table in `pipe/foreach_test.go`:

```go
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
```

- [ ] **Step 2: Write the nested inner-error case (names both indices)**

```go
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
```

- [ ] **Step 3: Write the nested cancellation case**

```go
{
	name: "canceled context stops a nested foreach with a StageError, no output",
	ctx: func(ctx context.Context) context.Context {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return cctx
	},
	build: func(t *testing.T) (*pipe.ForEach, *pipe.Scope) {
		vat, err := pipe.NewSingleExpr("amt", "tax.rate")
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
			"orders": []any{map[string]any{"taxes": []any{map[string]any{"rate": int64(1)}}}},
		})
		return lines, sc
	},
	assert: func(t *testing.T, sc *pipe.Scope, err error) {
		var se *pipe.StageError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, "lines", se.Stage)
		assert.ErrorIs(t, err, context.Canceled)
		_, ok := sc.Get("lines.items")
		assert.False(t, ok, "no output written when canceled before iteration")
	},
},
```

Note: confirm the table's case struct has a `ctx func(context.Context) context.Context` modifier field (it
does — see the existing `midIterationCancelCase()` usage and the `if tc.ctx != nil` block at ~line 847). If
the struct field is named differently, match it.

- [ ] **Step 4: Run the new cases**

Run: `go test ./pipe/ -run 'TestForEachExecute/nested' -race -v`
Expected: PASS for all three nested cases.

- [ ] **Step 5: Run the whole `pipe` suite**

Run: `go test ./pipe/ -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pipe/foreach_test.go
git commit -m "$(cat <<'EOF'
test(pipe): nested foreach lineage, output, rollup, cancel, error (B9)

Prove the behaviors that compose once firing re-prefixing lands: nested
lineage (lines[0].taxes[1].vat.amount -> element seed), nested output
shape, an outer rollup over a nested inner value, canceled-context stop,
and a nested inner error naming the outer element index.

Spec: 025
Plan: 025
ADR: 0050
EOF
)"
```

---

### Task 3: Config — lift the gate + `as`-chain guard (`config`)

Delete the `ErrNestedForEach` rejection (nesting then builds by existing recursion), add
`ErrForEachAsCollision` + `validateForEachAsChains` called from `Build`, and refresh the `def.go` doc.

**Files:**
- Modify: `config/build.go` (remove `ErrNestedForEach` decl + rejection; add `ErrForEachAsCollision`, `asBinding`, `validateForEachAsChains`; call it in `Build`)
- Modify: `config/def.go:57-58` (doc comment: nesting now supported)
- Modify: `config/foreach_test.go:341-368` (replace the rejection case with a nested-builds case); `:407` (remove the `ErrNestedForEach` reference); add `as`-collision + sibling-reuse cases

**Interfaces:**
- Produces: `config.ErrForEachAsCollision` (exported sentinel). `validateForEachAsChains(stages []StageDef, chain []asBinding) error` (unexported).
- Consumes: `pipe.TypeForEach`, `StageDef.{Type,As,Name,Stages}`, `*ConfigError`.

- [ ] **Step 1: Write the failing "nested builds and fires" test**

Replace the entire case at `config/foreach_test.go:341-368` (`"nested foreach is rejected naming both outer and inner stage"`) with:

```go
{
	name: "nested foreach builds, runs, and fires per inner element",
	yaml: `
stages:
  - name: lines
    type: foreach
    collection: orders
    as: line
    stages:
      - name: taxes
        type: foreach
        collection: line.taxes
        as: tax
        stages:
          - name: vat
            type: decision-table
            rules:
              - id: VAT_STD
                condition: tax.rate >= 10
                decisions:
                  band: '"standard"'
              - id: VAT_RED
                condition: tax.rate < 10
                decisions:
                  band: '"reduced"'
`,
	assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
		require.NoError(t, buildErr)
		p, err := d.Build()
		require.NoError(t, err)

		sc := pipe.NewScope(map[string]any{
			"orders": []any{
				map[string]any{"taxes": []any{
					map[string]any{"rate": int64(5)},
					map[string]any{"rate": int64(20)},
				}},
			},
		})
		require.NoError(t, p.Run(t.Context(), sc))

		assert.Equal(t, "VAT_RED", sc.FiringRulesFor("lines[0].taxes[0].vat")[0].RuleID)
		assert.Equal(t, "VAT_STD", sc.FiringRulesFor("lines[0].taxes[1].vat")[0].RuleID)
	},
},
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./config/ -run 'TestBuildForEach.*/nested_foreach_builds' -v`
Expected: FAIL — `d.Build()` returns a `*ConfigError` wrapping `ErrNestedForEach` (`require.NoError(err)`
fails). (Exact parent test name: match the function wrapping this table, e.g. `TestBuildForEach`.)

- [ ] **Step 3: Remove the gate; add the sentinel + guard**

In `config/build.go`:

a) Delete the `ErrNestedForEach` declaration (lines 15-20) and add in its place:

```go
// ErrForEachAsCollision is the Cause of the ConfigError returned when a foreach
// nesting chain reuses an `as` element-binding name (e.g. an outer and inner
// foreach both defaulting to "item"). A reused name would silently shadow the
// enclosing element in the inner per-element scope, so it is rejected at build
// time. Sibling foreach stages on different root-to-leaf chains may reuse a
// name (they run in independent scopes).
var ErrForEachAsCollision = errors.New("foreach: `as` element binding reused by a nested foreach")
```

b) In `buildForEach` delete the rejection (lines 307-309):

```go
		if isd.Type == pipe.TypeForEach {
			return nil, &ConfigError{Stage: sd.Name, Field: "stages", Cause: fmt.Errorf("inner stage %q: %w", isd.Name, ErrNestedForEach)}
		}
```

so the inner loop is just `st, err := isd.build(...)` (which now dispatches a nested foreach back into
`buildForEach`). Update the `buildForEach` doc comment (lines 292-299) to drop the "A nested foreach … is
rejected" sentence and note nesting is supported (the `as`-chain guard runs in `Build`).

c) Add the guard type + function (place after `buildForEach`, near the other build helpers):

```go
// asBinding pairs a foreach stage name with its effective `as` element binding,
// used to detect an `as` reused down a nesting chain.
type asBinding struct{ stage, as string }

// validateForEachAsChains walks the foreach nesting tree in stages and rejects
// any root-to-leaf chain that reuses an `as` element binding, which would
// silently shadow an enclosing element in the inner per-element scope. chain
// holds the (stage, effective-as) of the enclosing foreach ancestors; sibling
// foreaches on different chains may reuse a name. Non-foreach stages are
// skipped (their own inner Stages, if any, belong to a foreach handled here).
func validateForEachAsChains(stages []StageDef, chain []asBinding) error {
	for _, sd := range stages {
		if sd.Type != pipe.TypeForEach {
			continue
		}
		as := sd.As
		if as == "" {
			as = "item" // must match pipe.NewForEach's default (WithForEachAs)
		}
		for _, prior := range chain {
			if prior.as == as {
				return &ConfigError{
					Stage: sd.Name, Field: "as",
					Cause: fmt.Errorf("%w: inner foreach %q reuses element binding %q of enclosing foreach %q",
						ErrForEachAsCollision, sd.Name, as, prior.stage),
				}
			}
		}
		// Copy the chain so sibling recursions cannot alias the backing array.
		child := make([]asBinding, len(chain), len(chain)+1)
		copy(child, chain)
		child = append(child, asBinding{stage: sd.Name, as: as})
		if err := validateForEachAsChains(sd.Stages, child); err != nil {
			return err
		}
	}
	return nil
}
```

d) Call it in `Build`, right after `hydrateConstants()` succeeds (after line 42, before the lint block):

```go
	if err := validateForEachAsChains(d.Stages, nil); err != nil {
		return err
	}
```

- [ ] **Step 4: Run the "nested builds" test to verify it passes**

Run: `go test ./config/ -run 'TestBuildForEach.*/nested_foreach_builds' -v`
Expected: PASS.

- [ ] **Step 5: Fix the stale `ErrNestedForEach` reference in `TestBuildForEachRollupAggregationError`**

In `config/foreach_test.go`, delete line 407:

```go
	assert.False(t, errors.Is(err, config.ErrNestedForEach))
```

Then run `go vet ./config/`; if the `errors` import is now unused in the file, remove it. (Check first —
other cases may still use `errors`.)

- [ ] **Step 6: Add the `as`-collision and sibling-reuse cases**

Add to the same table (the one that held the removed rejection case):

```go
{
	name: "nested foreach reusing `as` down the chain is rejected naming both stages",
	yaml: `
stages:
  - name: outer
    type: foreach
    collection: items
    stages:
      - name: inner
        type: foreach
        collection: item.sub
        stages:
          - name: amt
            type: single-expr
            expr: item.price
`,
	assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
		require.NoError(t, buildErr)
		_, err := d.Build()
		require.Error(t, err)
		assert.ErrorIs(t, err, config.ErrForEachAsCollision)
		var ce *config.ConfigError
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, "inner", ce.Stage)
		assert.Equal(t, "as", ce.Field)
		assert.Contains(t, err.Error(), "outer") // enclosing stage named
		assert.Contains(t, err.Error(), "inner") // colliding stage named
	},
},
{
	name: "sibling foreach stages may reuse the same `as` (independent chains)",
	yaml: `
stages:
  - name: a
    type: foreach
    collection: xs
    stages:
      - name: ax
        type: single-expr
        expr: item.v
  - name: b
    type: foreach
    collection: ys
    stages:
      - name: bx
        type: single-expr
        expr: item.v
`,
	assert: func(t *testing.T, d *config.PipelineDef, buildErr error) {
		require.NoError(t, buildErr)
		_, err := d.Build()
		require.NoError(t, err, "siblings default `as: item` on independent chains must build")
	},
},
```

(Both `outer`/`inner` above omit `as`, so both default to `"item"` — the collision. The siblings `a`/`b`
also both default `"item"` but are on different chains, so they pass.)

- [ ] **Step 7: Update the `config/def.go` doc comment**

In `config/def.go`, replace the parenthetical in the `foreach` field doc (lines 57-58):

```go
	// sorted the same way as the top-level pipeline (a nested foreach among
	// them is rejected — see ErrNestedForEach); Rollups reduce a per-element
```

with:

```go
	// sorted the same way as the top-level pipeline (an inner stage may itself
	// be a foreach — nesting is supported; each foreach in a nesting chain must
	// bind its element under a distinct `as`, see ErrForEachAsCollision, and
	// nested fan-out is multiplicative in the collection sizes); Rollups reduce
	// a per-element
```

- [ ] **Step 8: Run the `config` suite**

Run: `go build ./... && go test ./config/ -race`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add config/build.go config/def.go config/foreach_test.go
git commit -m "$(cat <<'EOF'
feat(config)!: support nested foreach; guard `as`-chain collisions (B9)

Delete the ErrNestedForEach gate — nesting now builds via the existing
isd.build recursion. Add validateForEachAsChains (run from Build) rejecting
any nesting chain that reuses an `as` binding (ErrForEachAsCollision, naming
both stages); siblings on independent chains may reuse a name.

BREAKING (pre-1.0): config.ErrNestedForEach removed (nesting supported).

Spec: 025
Plan: 025
ADR: 0050
EOF
)"
```

---

### Task 4: Nested acceptance example + backlog/handover (docs)

Add a runnable `Example` demonstrating a two-level nested `foreach` end-to-end through YAML, then close out
B9 in the backlog and refresh the handover.

**Files:**
- Create: `examples/nested_foreach_test.go` (runnable `Example` with `// Output:`)
- Modify: `docs/BACKLOG.md` (move B9 to Resolved)
- Modify: `docs/HANDOVER.md` (increment 025 done; B10 next)

**Interfaces:**
- Consumes: `config.Parse` / `FromYAMLString`, `PipelineDef.Build`, `pipe.NewScope`, the nested firing key
  shape `outer[i].inner[j].<dt>`.

- [ ] **Step 1: Write the runnable nested example**

Create `examples/nested_foreach_test.go` (same external `examples_test` package as the existing example).
Confirm the import paths/aliases match `examples/foreach_lineitem_test.go` before running.

```go
package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// Example_nestedForEach adjudicates a nested collection: each order carries tax
// lines, and each tax line is banded by a decision table. Nested foreach keeps
// every inner element's firing under the composite key
// "<outer>[i].<inner>[j].<table>", so a decision stays explainable down to the
// exact line.
func Example_nestedForEach() {
	const ruleset = `
stages:
  - name: orders
    type: foreach
    collection: cart
    as: order
    stages:
      - name: taxes
        type: foreach
        collection: order.lines
        as: line
        stages:
          - name: vat
            type: decision-table
            rules:
              - id: VAT_STD
                condition: line.rate >= 10
                decisions:
                  band: '"standard"'
              - id: VAT_RED
                condition: line.rate < 10
                decisions:
                  band: '"reduced"'
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(ruleset))
	if err != nil {
		panic(err)
	}
	pipeline, err := def.Build()
	if err != nil {
		panic(err)
	}

	sc := pipe.NewScope(map[string]any{
		"cart": []any{
			map[string]any{"lines": []any{
				map[string]any{"rate": 5},
				map[string]any{"rate": 20},
			}},
			map[string]any{"lines": []any{
				map[string]any{"rate": 12},
			}},
		},
	})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		panic(err)
	}

	// Per-(order, line) firing under the nested composite key.
	for _, key := range []string{
		"orders[0].taxes[0].vat",
		"orders[0].taxes[1].vat",
		"orders[1].taxes[0].vat",
	} {
		fmt.Printf("%s -> %s\n", key, sc.FiringRulesFor(key)[0].RuleID)
	}

	// Output:
	// orders[0].taxes[0].vat -> VAT_RED
	// orders[0].taxes[1].vat -> VAT_STD
	// orders[1].taxes[0].vat -> VAT_STD
}
```

The `// Output:` values are deterministic (rate 5 → VAT_RED; 20, 12 → VAT_STD). Run it once to confirm the
observed output matches exactly before committing (runnable-example gate).

- [ ] **Step 2: Run the example to capture and verify its output**

Run: `go test ./examples/ -run 'Example.*Nested' -v`
Expected: PASS (fill the `// Output:` block from the first run, then re-run to confirm it matches).

- [ ] **Step 3: Move B9 to Resolved in `docs/BACKLOG.md`**

- Change the B9 table row (line 28) to strikethrough `Done`, matching B1-B8:
  `| ~~**B9**~~ | ~~Nested \`foreach\` support~~ | — | — | — | ✅ **Done** (incr 025, ADR-0050) |`
- Update the B9 **Details** paragraph (lines 86-88) to a resolved note (nesting supported; hierarchical
  firing keys; `as`-chain guard; ADR-0050).
- Add a row to the "Recently resolved deferrals" table: `| Nested foreach (B9; ADR-0040 D7 / Spec 015 D7) | Increment 025 / ADR-0050 |`.

- [ ] **Step 4: Refresh `docs/HANDOVER.md`**

Update: increments through 025 done (B9, ADR-0050, this branch); artifact numbering (specs/plans **025
done**, ADRs **0050 done**); "Next action" → **B10** (Convenience constructors; `docs/BACKLOG.md`; a design
that is NOT a checkpoint item per the standing decision — design + implement autonomously). Keep the
read-first pointers and standing decisions.

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go test ./... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add examples/nested_foreach_test.go docs/BACKLOG.md docs/HANDOVER.md
git commit -m "$(cat <<'EOF'
docs(rlng): nested foreach acceptance example; close B9 (B9)

Runnable two-level nested-foreach example through YAML; move B9 to
Resolved and refresh the handover (B10 next).

Spec: 025
Plan: 025
ADR: 0050
EOF
)"
```

---

## Whole-branch delivery gate (after Task 4)

Per CLAUDE.md §5 and the standing program authorization:

- [ ] `go build ./...`, `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) / `go mod verify` all clean.
- [ ] `/code-review high main..HEAD` — resolve/triage every finding; re-run affected review + `-race`.
- [ ] `/security-review` on the branch diff — resolve/triage findings.
- [ ] Confirm coverage: `go test ./pipe/ ./config/ -cover` (target ≥ 85%; new branches covered).
- [ ] Auto merge to `main` + push + delete branch (standing authorization; does NOT extend to release tags).

## Self-review notes (spec coverage)

- Spec D1 (firing re-prefix) → Task 1. D2 (lineage composes, no new code) → Task 2 tests. D3 (gate lift) →
  Task 3. D4 (`as`-chain guard) → Task 3. D5 (no depth cap; document cost) → Task 3 def.go doc + ADR. D6
  (remove `ErrNestedForEach`) → Task 3.
- Success criteria 1-2 → Task 1; 3-5 → Task 2; 6-7 → Task 3; 8-9 → Task 2; 10 (Hash stability) → guarded by
  the existing golden-hash test (unchanged; Global Constraints).
- Breaking changes (firing-query shape, `ErrNestedForEach` removal) → ADR-0050 (Task 1), `!` commit types
  (Tasks 1, 3).
