# Plan 021 — per-decision options in decision tables (B5)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Let each decision-table decision (and each default decision) carry its own
`fallback`/`globals`/`coerce`, honored end-to-end, by modelling a decision as expression **plus options**.

- **Implements:** Spec 021 (`docs/specs/021-per-decision-options.md`) — Option A (approved 2026-07-13).
- **Records:** ADR-0046 (rides in the feat commit); supersedes ADR-0007 §5's per-decision-options rejection.
- **Backlog:** graduates B5 (`docs/BACKLOG.md`).

**Architecture:** Introduce an exported `pipe.Decision{Expr, Options}` type; change `pipe.Rule.Decisions`
from `map[string]string` to `map[string]Decision` and remove the shared `Rule.DecisionOptions`;
`compileDecisions` compiles each decision with its own options. `WithDefault` takes `map[string]Decision`
symmetrically. In `config`, replace `bareDecisions` (which rejected per-decision options) with a converter
that threads each `ExprDef`'s options through, wrapped with the shared constants+strict-env.

**Tech Stack:** Go 1.25+, `github.com/expr-lang/expr`, `shopspring/decimal` (unchanged).

## Global Constraints

- Pure Go, no cgo (`CGO_ENABLED=0 go build ./...` must pass); no new dependencies.
- Blackbox tests only (`package pipe_test` / `package config_test`); assert-closure tables (`table-test`
  skill); `t.Context()` over `context.Background()`.
- Every exported symbol has a godoc comment; small deliberate public surface.
- **Breaking pre-1.0 API change** — flag it in the commit subject and note `apidiff`; ADR-0046 records it.
- `Hash()` of an unchanged ruleset stays byte-identical (the parsed `PipelineDef` shape is untouched).
- Test-coverage gate: ≥ 85% on changed packages; every new/changed hot-path + typed-error branch covered.

## Why one task (green-unit coupling)

`config` imports `pipe`. The moment `pipe.Rule.Decisions` changes type, every caller in both packages
fails to compile, so the whole module cannot reach a green `go test ./... -race` until the API change,
the config converter, and all in-repo test migrations land together. This is therefore **one green
commit**. The steps below are TDD-ordered within that single task.

---

### Task 1: Per-decision options end-to-end (feat, one green commit)

**Files:**
- Modify: `pipe/table.go` (add `Decision` type; `Rule.Decisions map[string]Decision`; drop
  `Rule.DecisionOptions`; `compileDecisions` signature; `sortedKeys` for the new map type).
- Modify: `pipe/options.go` (`stageConfig.defaults map[string]Decision`, drop `defaultsOpts`;
  `WithDefault(map[string]Decision)`).
- Modify: `config/build.go` (replace `bareDecisions` with `decisionsFrom`; update the two call sites;
  drop the `DecisionOptions` field write and the `WithDefault` opts argument).
- Modify (delete method): `config/expr_def.go` — remove `hasOptions()` (its only caller disappears).
- Test (new behavior): `pipe/table_options_test.go` (create) — per-decision fallback/globals/coerce, and
  a `WithDefault` per-decision fallback.
- Test (new behavior): `config/table_options_test.go` (create) — config builds a decision with options
  (no `*ConfigError`); constants+schema still reach every decision.
- Test (migrate, mechanical): `pipe/{table_test,table_example_test,table_edges_test,table_policies_test,`
  `firing_test,firing_example_test,foreach_test,json_test}.go`; `examples/eligibility_test.go`.
  `map[string]string{"k":"e"}` → `map[string]pipe.Decision{"k":{Expr:"e"}}`;
  `WithDefault(map[string]string{…})` → `WithDefault(map[string]pipe.Decision{…})`.

**Interfaces:**
- Produces (new/changed exported surface):
  - `type pipe.Decision struct { Expr string; Options []expr.Option }`
  - `pipe.Rule.Decisions map[string]pipe.Decision` (was `map[string]string`)
  - `pipe.Rule.DecisionOptions` — **removed**
  - `func pipe.WithDefault(decisions map[string]pipe.Decision) pipe.Option` (was
    `(map[string]string, ...expr.Option)`)
- Consumes: `expr.Option`, `expr.NewFunction` (unchanged); config `ExprDef.options()` (unchanged).

- [ ] **Step 1 (red — pipe): write the new-behavior test file.**

  Create `pipe/table_options_test.go` (`package pipe_test`). It will not compile until Step 3 adds
  `pipe.Decision`; that compile failure is the red state. Cover Spec 021 success criteria 1–3:

```go
package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/kartaladev/rlng/pipe"
)

// A per-decision fallback recovers one output while a sibling without one still
// errors (Spec 021 criterion 1).
func TestDecisionTable_PerDecisionFallback(t *testing.T) {
	tests := []struct {
		name   string
		rule   pipe.Rule
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}{
		{
			name: "fallback output recovers, sibling has none",
			rule: pipe.Rule{
				Condition: "true",
				Decisions: map[string]pipe.Decision{
					"safe": {Expr: "missing.field + 1", Options: []expr.Option{expr.WithFallback("0")}},
				},
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				if err != nil {
					t.Fatalf("expected fallback to recover, got %v", err)
				}
				if got, _ := sc.Get("t.safe"); got != 0 {
					t.Fatalf("safe = %v, want 0", got)
				}
			},
		},
		{
			name: "sibling without fallback still errors",
			rule: pipe.Rule{
				Condition: "true",
				Decisions: map[string]pipe.Decision{
					"boom": {Expr: "1 / n"}, // n = 0 at runtime -> error, no fallback
				},
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				if err == nil {
					t.Fatal("expected error from fallback-less decision, got nil")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt, err := pipe.NewDecisionTable("t", []pipe.Rule{tt.rule})
			if err != nil {
				t.Fatalf("NewDecisionTable: %v", err)
			}
			sc := pipe.NewScope(map[string]any{"n": 0})
			err = dt.Execute(t.Context(), sc)
			tt.assert(t, sc, err)
		})
	}
}

// Per-decision globals and coerce are honored (Spec 021 criterion 2).
func TestDecisionTable_PerDecisionGlobalsAndCoerce(t *testing.T) {
	dt, err := pipe.NewDecisionTable("t", []pipe.Rule{{
		Condition: "true",
		Decisions: map[string]pipe.Decision{
			"g": {Expr: "threshold", Options: []expr.Option{expr.WithGlobals(map[string]any{"threshold": 42})}},
		},
	}})
	if err != nil {
		t.Fatalf("NewDecisionTable: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := dt.Execute(t.Context(), sc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := sc.Get("t.g"); got != 42 {
		t.Fatalf("g = %v, want 42", got)
	}
}

// A default decision carries its own fallback (Spec 021 criterion 3 — D3 symmetry).
func TestDecisionTable_DefaultPerDecisionFallback(t *testing.T) {
	dt, err := pipe.NewDecisionTable("t",
		[]pipe.Rule{{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}}},
		pipe.WithDefault(map[string]pipe.Decision{
			"x": {Expr: "missing.field", Options: []expr.Option{expr.WithFallback("7")}},
		}),
	)
	if err != nil {
		t.Fatalf("NewDecisionTable: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := dt.Execute(t.Context(), sc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := sc.Get("t.x"); got != 7 {
		t.Fatalf("default x = %v, want 7 (fallback)", got)
	}
}
```

- [ ] **Step 2 (red): run it, confirm it fails to compile.**

  Run: `go test ./pipe/ -run TestDecisionTable_PerDecision 2>&1 | head`
  Expected: build failure — `undefined: pipe.Decision` (and `WithDefault` arity). That is the red state.

- [ ] **Step 3 (green — pipe API):** In `pipe/table.go`:
  - Add the exported type (godoc it):

```go
// Decision is one output of a decision-table Rule: its value expression and the
// compile options (fallback, globals, coerce) that apply to that output alone.
// A bare output with no options is Decision{Expr: "..."}.
type Decision struct {
	Expr    string
	Options []expr.Option
}
```

  - Change `Rule`: `Decisions map[string]Decision` and **remove** the `DecisionOptions` field. Update the
    struct godoc to say each decision carries its own options.
  - Change `compileDecisions` to drop the shared `opts` param and compile each decision with its own
    options:

```go
// compileDecisions compiles a key->Decision set in sorted-key order, compiling
// each decision's expression with that decision's own options, wrapping failures
// in a StageError attributed to where (e.g. "rule 2").
func compileDecisions(stage, where string, decisions map[string]Decision) ([]compiledDecision, error) {
	out := make([]compiledDecision, 0, len(decisions))
	for _, key := range sortedKeys(decisions) {
		if key == "" {
			return nil, &StageError{Stage: stage, Type: TypeDecisionTable, Cause: fmt.Errorf("%s has an empty output key", where)}
		}
		dec := decisions[key]
		fn, err := expr.NewFunction(key, dec.Expr, dec.Options...)
		if err != nil {
			return nil, &StageError{Stage: stage, Type: TypeDecisionTable, Cause: fmt.Errorf("%s decision %q: %w", where, key, err)}
		}
		out = append(out, compiledDecision{key: key, fn: fn})
	}
	return out, nil
}
```

  - Update the two call sites in `NewDecisionTable`: `compileDecisions(name, fmt.Sprintf("rule %d", i),
    r.Decisions)` and `compileDecisions(name, "default", cfg.defaults)`.
  - Change `sortedKeys` to accept `map[string]Decision`:

```go
func sortedKeys(m map[string]Decision) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

- [ ] **Step 4 (green — pipe options):** In `pipe/options.go`:
  - `stageConfig`: change `defaults map[string]string` → `defaults map[string]Decision`; **remove**
    `defaultsOpts []expr.Option`.
  - Change `WithDefault` (update godoc to note per-decision options):

```go
// WithDefault sets a DecisionTable's default (else) decisions, applied when no
// rule matches — so "no match" is an explicit outcome rather than a silent
// missing output. The map is output key -> Decision, like a rule's Decisions,
// so each default output carries its own fallback/globals/coerce. Ignored by
// SingleExpr and MultiExpr.
func WithDefault(decisions map[string]Decision) Option {
	return func(c *stageConfig) { c.defaults = decisions }
}
```

- [ ] **Step 5 (green — config converter):** In `config/build.go`:
  - Replace `bareDecisions` with a converter that threads per-decision options (drop the rejection):

```go
// decisionsFrom converts a key->ExprDef decision set to key->pipe.Decision,
// threading each decision's own options (fallback/globals/coerce) plus the
// shared constants and strict env — so a per-decision option composes with the
// pipeline env instead of being rejected.
func decisionsFrom(constants, schema map[string]any, strict bool, in map[string]ExprDef) map[string]pipe.Decision {
	out := make(map[string]pipe.Decision, len(in))
	for key, ed := range in {
		out[key] = pipe.Decision{
			Expr:    ed.Expr,
			Options: withStrictEnv(strict, schema, withConstants(constants, ed.options())),
		}
	}
	return out
}
```

  - In `buildTable`, the rule loop: drop the `bareDecisions` error return and the `DecisionOptions` field;
    set `Decisions: decisionsFrom(constants, schema, strict, r.Decisions)`. The rule condition options are
    unchanged.
  - The default block: `pipe.WithDefault(decisionsFrom(constants, schema, strict, sd.Default))` (no opts
    arg). The `len(sd.Default) > 0` guard stays.
  - Because `decisionsFrom` no longer returns an error, remove the now-dead `err` handling around those
    two call sites.

- [ ] **Step 6 (green — drop dead code):** In `config/expr_def.go`, remove `hasOptions()` (only caller
  was `bareDecisions`). Leave `options()` — `decisionsFrom` uses it.

- [ ] **Step 7 (green — migrate callers):** Mechanically migrate every test listed under **Files**:
  - `Decisions: map[string]string{"k": "e"}` → `Decisions: map[string]pipe.Decision{"k": {Expr: "e"}}`.
    In `package pipe` internal spots? None — all are `_test` blackbox; use `pipe.Decision`. In
    `pipe/*_test.go` the package is `pipe_test`, so qualify as `pipe.Decision`.
  - `pipe.WithDefault(map[string]string{"k": "e"})` → `pipe.WithDefault(map[string]pipe.Decision{"k":
    {Expr: "e"}})`; `pipe.WithDefault(m, opt...)` (config-style) has no test callers with opts.
  - `pipe/table_test.go:603` builds `Decisions: map[string]string{"v": e}` in a loop → `map[string]
    pipe.Decision{"v": {Expr: e}}`.
  - No behavior change in any migrated case — bare decisions carry no options.

- [ ] **Step 8 (red→green — config new-behavior test):** Create `config/table_options_test.go`
  (`package config_test`), covering Spec 021 criteria 5–6:

```go
package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
)

// A decision with its own options now BUILDS (the old rejection is gone) and the
// option is honored at runtime (Spec 021 criteria 5); constants still reach a
// decision that also declares its own options (criterion 6).
func TestBuild_PerDecisionOptions(t *testing.T) {
	yaml := `
version: v1
constants:
  base: 100
stages:
  - name: t
    type: decision-table
    rules:
      - condition: "true"
        decisions:
          limit:
            expr: "missing.field + base"
            fallback: "base"
`
	def, err := config.Parse(t.Context(), config.FromYAMLString(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := def.Build() // previously *ConfigError "per-decision options are not supported"
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := p.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// fallback expr is `base`, and base (a constant) must reach the decision: 100.
	if got, _ := sc.Get("t.limit"); got != 100 {
		t.Fatalf("t.limit = %v, want 100 (fallback to constant base)", got)
	}
}
```

  Entry points (verified): `config.Parse(ctx, config.FromYAMLString(s)) (*PipelineDef, error)`,
  `def.Build() (*pipe.Pipeline, error)`, `pipe.NewScope(seed)`, `p.Run(ctx, sc) error`,
  `sc.Get(path) (any, bool)`. Import `pipe` in the test. Run:
  `go test ./config/ -run TestBuild_PerDecisionOptions -v` — Expected PASS once Steps 3–6 land.

- [ ] **Step 9 (green — whole suite):** Run `go test ./... -race`. Fix any remaining migration misses
  (grep `map\[string\]string{` near `Decisions`/`WithDefault` to catch stragglers). Expected: all green.

- [ ] **Step 10 — Hash stability regression:** Confirm the golden-hash test (if present) stays green:
  `go test ./config/ -run Hash -v`. The parsed `PipelineDef` shape is unchanged, so hashes are
  byte-identical. If no golden test exists for a decision-table config, this is covered by the existing
  config suite staying green.

- [ ] **Step 11 — docs & ADR:** Author **ADR-0046** (Nygard): context = ADR-0007 §5 / Spec 004 rejected
  per-decision options because `stage.Rule` carried one shared `DecisionOptions`; decision = Option A, model
  a decision as `Decision{Expr, Options}`, remove the shared field, delete the config rejection, accept the
  breaking `pipe.Rule`/`WithDefault` change pre-1.0; consequences = breaking exported surface (apidiff),
  no `Hash()`/config-schema change, per-decision fallback/globals/coerce now honored. Set ADR-0007's
  affected note as superseded-in-part (add a pointer; do not rewrite it). Move **B5** to the Resolved
  section of `docs/BACKLOG.md` (closing increment 021 / ADR-0046). Update `docs/HANDOVER.md` to B5-done /
  B6-next at the end.

- [ ] **Step 12 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) /
  `go mod verify`; `pipe` and `config` coverage ≥ 85% with the new per-decision branches covered.

- [ ] **Step 13 — commit:** `feat(pipe)!: per-decision options in decision tables (B5)` with a body noting
  the breaking `pipe.Rule.Decisions`/`WithDefault` change + `apidiff`, and trailers `Spec: 021`,
  `Plan: 021`, `ADR: 0046`. (Plan + ADR + BACKLOG + HANDOVER ride in this commit.)

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item
(**B6** — precise member-path references in provenance; a design-checkpoint item, so pause for the user's
design approval before implementing).
