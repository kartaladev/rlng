# Evaluation correctness & explainability — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix four evaluation-logic defects the deeper audit found — collect/any decisions record no firing rule, a configured fallback masks genuine errors and conflates `nil` with failure, and lenient truthiness has unsafe/inconsistent coercions.

**Architecture:** Extend the `pipe` firing model to hold multiple firing rules per stage (backward-compatibly), record firing on every collect/any match, make the `expr` fallback trigger on error only (with `nil`→fallback opt-in) and observable, and make `WithCoerce` truthiness safe and honest. Changes are confined to `pipe/firing.go`, `pipe/table.go`, `expr/function.go`, `expr/options.go`, `expr/predicate.go`.

**Tech Stack:** Go 1.25, `github.com/expr-lang/expr`, `stretchr/testify`.

## Global Constraints

- Go 1.25+; pure Go, no cgo. Library must not panic/os.Exit/log.Fatal on caller input; return typed errors; no global logger (accept an injected hook if needed).
- Blackbox tests only: every `_test.go` uses `package <pkg>_test` and drives the exported API. Mandatory `table-test` assert-closure form (`assert func(t, ...)` closures, NOT want/wantErr) for ≥2 same-SUT cases. `t.Context()` over `context.Background()`.
- Every exported symbol has a godoc comment. Add no new dependencies. Target ≥85% coverage on changed packages; every hot-path and typed-error branch has a covering test (the hot path is `Execute`/`Apply`/`Test` and everything they call per invocation).
- Behavior changes (fallback default; coercion of NaN/unknown-type) are pre-`v0.1.0` API changes: document each in an ADR and update any existing test that asserted the old behavior. Note them as breaking in the commit body.
- Traceability: commits carry `Spec: 012`, `Plan: 012`, and the relevant `ADR:` trailer. End every commit message with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Implements Spec 012; see `docs/specs/012-evaluation-correctness-and-explainability.md`.

---

### Task 1: Multi-rule firing model (`pipe/firing.go`)

Extend the firing store from one rule per stage to an ordered list, so collect/any can record every contributing rule. Keep `FiringRule`/`FiringRules` working; add `FiringRulesFor`.

**Files:**
- Modify: `pipe/firing.go` (whole file), `pipe/scope.go:34` (field type)
- Test: `pipe/firing_test.go`

**Interfaces:**
- Produces:
  - `s.firing` becomes `map[string][]FiringRule` (unexported; `pipe/scope.go:34`).
  - `func (s *Scope) recordFiring(stage, ruleID, message string, isDefault bool)` — unchanged signature; now stores a single-element slice (replaces any prior).
  - `func (s *Scope) recordFirings(stage string, rules []FiringRule)` — NEW; stores an ordered multi-rule set for a stage.
  - `func (s *Scope) FiringRule(stage string) (FiringRule, bool)` — unchanged; returns the FIRST firing rule for the stage.
  - `func (s *Scope) FiringRules() []FiringRule` — unchanged signature; now flattens all stages' rules, sorted by stage then original order.
  - `func (s *Scope) FiringRulesFor(stage string) []FiringRule` — NEW; the full ordered slice for one stage (nil if none).

- [ ] **Step 1: Write the failing tests**

Add to `pipe/firing_test.go` (blackbox `package pipe_test`). Task 1 changes only the firing *model*, not table.go — so exercise it through a SINGLE-match path (which records one firing today); the multi-rule assertions live in Task 2, once collect/any actually record several. New cases:

```go
func TestScopeFiringRulesForSingleMatch(t *testing.T) {
	// A first-match (single) table records the matched rule; FiringRulesFor
	// returns it as a one-element slice, and FiringRule returns that first rule.
	tbl, err := pipe.NewDecisionTable("t", []pipe.Rule{
		{ID: "R1", Condition: "x > 0", Decisions: map[string]string{"tag": `"a"`}},
	}, pipe.WithHitPolicy(pipe.HitPolicySingle))
	require.NoError(t, err)
	sc := pipe.NewScope(map[string]any{"x": 2})
	require.NoError(t, tbl.Execute(t.Context(), sc))

	got := sc.FiringRulesFor("t")
	require.Len(t, got, 1)
	assert.Equal(t, "R1", got[0].RuleID)

	first, ok := sc.FiringRule("t")
	require.True(t, ok)
	assert.Equal(t, "R1", first.RuleID)

	assert.Len(t, sc.FiringRules(), 1)
}

func TestScopeFiringRulesForAbsent(t *testing.T) {
	sc := pipe.NewScope(nil)
	assert.Nil(t, sc.FiringRulesFor("nope"))
	_, ok := sc.FiringRule("nope")
	assert.False(t, ok)
}
```

The multi-rule assertions (a collect/any table firing several rules, `FiringRulesFor` returning all in order, `FiringRules` flattening them) are added in Task 2, where collect/any begin recording them.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestScopeFiringRulesFor' -v`
Expected: compile failure (`FiringRulesFor` undefined) and/or the collect case FAILS (collect records no firing today).

- [ ] **Step 3: Change the field type**

In `pipe/scope.go:34`, change:

```go
	firing      map[string][]FiringRule // stage name -> the ordered rules that fired
```

- [ ] **Step 4: Rewrite `pipe/firing.go`**

Replace the recorder and accessors:

```go
// recordFiring notes the single rule that fired for a stage (single/unique
// policies, or a default), replacing any prior record for that stage. It is
// always recorded (independent of provenance) for a cheap audit.
func (s *Scope) recordFiring(stage, ruleID, message string, isDefault bool) {
	s.recordFirings(stage, []FiringRule{{Stage: stage, RuleID: ruleID, Message: message, IsDefault: isDefault}})
}

// recordFirings notes the ordered set of rules that fired for a stage (collect
// and any policies can fire several), replacing any prior record for that stage.
func (s *Scope) recordFirings(stage string, rules []FiringRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string][]FiringRule)
	}
	s.firing[stage] = rules
}

// FiringRule returns the first rule that fired for the named decision-table
// stage, or false if that stage did not run, matched nothing, and had no
// default. For a policy that can fire several rules (collect, any), use
// FiringRulesFor to get all of them.
func (s *Scope) FiringRule(stage string) (FiringRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := s.firing[stage]
	if len(rules) == 0 {
		return FiringRule{}, false
	}
	return rules[0], true
}

// FiringRulesFor returns every rule that fired for the named stage, in firing
// order (nil if the stage recorded none).
func (s *Scope) FiringRulesFor(stage string) []FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := s.firing[stage]
	if len(rules) == 0 {
		return nil
	}
	out := make([]FiringRule, len(rules))
	copy(out, rules)
	return out
}

// FiringRules returns every recorded firing rule across all stages, sorted by
// stage name (and, within a stage, in firing order) — a compact audit trail of
// which rules decided each decision-table stage.
func (s *Scope) FiringRules() []FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stages := make([]string, 0, len(s.firing))
	for stage := range s.firing {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	var out []FiringRule
	for _, stage := range stages {
		out = append(out, s.firing[stage]...)
	}
	return out
}
```

Update the `FiringRule` struct godoc (`pipe/firing.go:5-8`) to note that collect/any can record several rules per stage (retrievable via `FiringRulesFor`).

- [ ] **Step 5: Run to verify pass**

Run: `go test ./pipe/ -run 'TestScopeFiring' -v && go test ./pipe/ -race`
Expected: PASS. Existing firing tests (single/default) still green — `FiringRule` and `FiringRules` behavior is preserved for the single-rule case.

- [ ] **Step 6: Commit**

```bash
git add pipe/firing.go pipe/scope.go pipe/firing_test.go docs/plans/012-evaluation-correctness-and-explainability.md
git commit -m "$(cat <<'EOF'
feat(pipe): multi-rule firing model (FiringRulesFor)

Store firing rules as an ordered slice per stage so collect/any policies can
record every contributing rule. FiringRule (first) and FiringRules (flattened)
are preserved; FiringRulesFor returns all rules for a stage.

Spec: 012
Plan: 012
ADR: 0036
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Record firing on collect & any matches (`pipe/table.go`)

`executeCollect` records no firing on a match (only on default); `executeAny` records only the first agreeing rule. Record every matched rule for both, via `recordFirings`.

**Files:**
- Modify: `pipe/table.go` (`executeCollect` ~333-396, `executeAny` ~292-328)
- Test: `pipe/firing_test.go` or `pipe/table_policies_test.go`

**Interfaces:**
- Consumes: `recordFirings(stage string, []FiringRule)` (Task 1).
- Produces: after a collect/any run that matched rules, `FiringRulesFor(stage)` returns one entry per matched rule (in rule order); a no-match run still records the default via `applyDefaults`.

- [ ] **Step 1: Write the failing tests**

Add to `pipe/table_policies_test.go`:

```go
func TestCollectRecordsFiringPerMatch(t *testing.T) {
	tbl, err := pipe.NewDecisionTable("fees", []pipe.Rule{
		{ID: "BASE", Condition: "true", Decisions: map[string]string{"fee": "10"}},
		{ID: "SURCHARGE", Condition: "risky", Decisions: map[string]string{"fee": "5"}},
	}, pipe.WithHitPolicy(pipe.HitPolicyCollect), pipe.WithCollectAggregation(pipe.AggregateSum))
	require.NoError(t, err)
	sc := pipe.NewScope(map[string]any{"risky": true})
	require.NoError(t, tbl.Execute(t.Context(), sc))

	fired := sc.FiringRulesFor("fees")
	require.Len(t, fired, 2)
	assert.Equal(t, "BASE", fired[0].RuleID)
	assert.Equal(t, "SURCHARGE", fired[1].RuleID)
}

func TestAnyRecordsFiringPerAgreeingRule(t *testing.T) {
	tbl, err := pipe.NewDecisionTable("decide", []pipe.Rule{
		{ID: "A", Condition: "x > 0", Decisions: map[string]string{"ok": "true"}},
		{ID: "B", Condition: "x > 1", Decisions: map[string]string{"ok": "true"}},
	}, pipe.WithHitPolicy(pipe.HitPolicyAny))
	require.NoError(t, err)
	sc := pipe.NewScope(map[string]any{"x": 2})
	require.NoError(t, tbl.Execute(t.Context(), sc))

	fired := sc.FiringRulesFor("decide")
	require.Len(t, fired, 2)
	assert.Equal(t, "A", fired[0].RuleID)
	assert.Equal(t, "B", fired[1].RuleID)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestCollectRecordsFiring|TestAnyRecordsFiring' -v`
Expected: both FAIL — collect records nothing on match; any records only the first rule.

- [ ] **Step 3: Record firing in `executeCollect`**

In `pipe/table.go`, in `executeCollect`, collect the matched rules' identities alongside the decisions. Add a slice and record it after the match loop, before aggregation. Inside the `for _, r := range d.rules` loop, in the `matchedAny = true` block, append the rule's identity; after the loop (in the `matchedAny` branch) record them:

```go
	matchedAny := false
	var firings []FiringRule

	for _, r := range d.rules {
		ok, err := r.cond.Test(env)
		if err != nil {
			return d.stageErr(err)
		}
		if !ok {
			continue
		}
		matchedAny = true
		firings = append(firings, FiringRule{Stage: d.name, RuleID: r.id, Message: r.message})
		for _, dec := range r.decisions {
			// ... unchanged decision collection ...
		}
	}

	if !matchedAny {
		return d.applyDefaults(env, sc)
	}
	sc.recordFirings(d.name, firings)
```

(Insert the `sc.recordFirings(d.name, firings)` immediately after the `if !matchedAny { return d.applyDefaults(...) }` guard, before the aggregation loop.)

- [ ] **Step 4: Record all agreeing rules in `executeAny`**

In `executeAny`, replace the single `sc.recordFiring(d.name, d.rules[matched[0]].id, ...)` (~line 300) with a multi-record built from all matched indices, placed after the agreement loop succeeds (so a conflict still errors without recording a firing):

```go
	// ... after the agreement loop completes without conflict, before writing:
	firings := make([]FiringRule, 0, len(matched))
	for _, idx := range matched {
		firings = append(firings, FiringRule{Stage: d.name, RuleID: d.rules[idx].id, Message: d.rules[idx].message})
	}
	sc.recordFirings(d.name, firings)
```

Remove the old single `sc.recordFiring(d.name, d.rules[matched[0]].id, d.rules[matched[0]].message, false)` call.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./pipe/ -run 'TestCollect|TestAny' -v && go test ./pipe/ -race`
Expected: PASS; existing collect/any value tests still green (firing is additive; decision outputs unchanged).

- [ ] **Step 6: Commit**

```bash
git add pipe/table.go pipe/table_policies_test.go
git commit -m "$(cat <<'EOF'
fix(pipe): record firing rules for collect and any matches

executeCollect recorded no firing rule on a match (only on default), and
executeAny recorded only the first agreeing rule — blanking the explainability
trail for multi-rule adverse-action decisions. Both now record every matched
rule via recordFirings, in rule order.

Spec: 012
Plan: 012
ADR: 0036
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Fallback triggers on error only; `nil` stays first-class; error observable (`expr/function.go`, `expr/options.go`)

A configured fallback fires on error OR `nil` today, so a Function with a fallback can never return `nil`, and an error-triggered fallback silently discards the cause. Make `nil`→fallback opt-in and add an optional observer for the discarded cause.

**Files:**
- Modify: `expr/function.go` (`Function` struct, `Apply` ~66-84, `runFallback` ~97-107, `NewFunction` to wire the new fields), `expr/options.go` (`config` struct + new options)
- Test: `expr/function_test.go`

**Interfaces:**
- Produces:
  - `func WithFallbackOnNil() Option` — makes a `nil` main result trigger the fallback (default: only an error does).
  - `func WithFallbackObserver(fn func(name, expression string, cause error)) Option` — called when a fallback fires because the main expression errored, with the original cause; default nil (no-op). Never called for a `nil`-triggered fallback.
  - `Function` gains unexported `fallbackOnNil bool` and `fallbackObserver func(string, string, error)` fields, set from `config`.
- Behavior change: by default a `nil` main result is returned as-is (fallback NOT fired). ADR-0034.

- [ ] **Step 1: Write the failing tests**

In `expr/function_test.go`, add (and locate the existing test that asserts a `nil`/error main triggers the fallback — it must be updated to the new default):

```go
func TestApplyNilResultNotFallbackByDefault(t *testing.T) {
	fn, err := expr.NewFunction("f", "nil", expr.WithFallback("99"))
	require.NoError(t, err)
	got, err := fn.Apply(map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, got, "nil is first-class: fallback must not fire on a nil main result by default")
}

func TestApplyNilResultFallbackWhenOptedIn(t *testing.T) {
	fn, err := expr.NewFunction("f", "nil", expr.WithFallback("99"), expr.WithFallbackOnNil())
	require.NoError(t, err)
	got, err := fn.Apply(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 99, got)
}

func TestApplyErrorFallbackObserverSeesCause(t *testing.T) {
	var seen error
	fn, err := expr.NewFunction("ratio", "a / b", expr.WithFallback("-1"),
		expr.WithFallbackObserver(func(_, _ string, cause error) { seen = cause }))
	require.NoError(t, err)
	got, err := fn.Apply(map[string]any{"a": 1, "b": 0})
	require.NoError(t, err)
	assert.Equal(t, -1, got)
	require.Error(t, seen, "the masked division error must be surfaced to the observer")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./expr/ -run 'TestApplyNilResult|TestApplyErrorFallbackObserver' -v`
Expected: compile failure (`WithFallbackOnNil`, `WithFallbackObserver` undefined) → FAIL.

- [ ] **Step 3: Add config fields and options (`expr/options.go`)**

In the `config` struct, add:

```go
	fallbackOnNil    bool
	fallbackObserver func(name, expression string, cause error)
```

Add the options:

```go
// WithFallbackOnNil makes a Function's fallback also fire when the main
// expression evaluates to nil (not only when it errors). By default nil is a
// first-class result and the fallback fires only on an error.
func WithFallbackOnNil() Option { return func(c *config) { c.fallbackOnNil = true } }

// WithFallbackObserver registers a callback invoked when a Function's fallback
// fires because the main expression ERRORED, receiving the function name, the
// main expression, and the triggering cause — so the masked error is observable
// rather than silently discarded. It is not called for a nil-triggered fallback.
func WithFallbackObserver(fn func(name, expression string, cause error)) Option {
	return func(c *config) { c.fallbackObserver = fn }
}
```

- [ ] **Step 4: Wire the fields and change `Apply` (`expr/function.go`)**

Add to the `Function` struct:

```go
	fallbackOnNil    bool
	fallbackObserver func(name, expression string, cause error)
```

In `NewFunction`, after `fn := &Function{...}` is built, set them from cfg (do this where the fallback is wired):

```go
	fn.fallbackOnNil = cfg.fallbackOnNil
	fn.fallbackObserver = cfg.fallbackObserver
```

Change `Apply` so `nil` no longer triggers the fallback by default, and the observer sees an error-triggered fallback's cause:

```go
func (f *Function) Apply(env any) (any, error) {
	m, err := toEnv(env)
	if err != nil {
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}

	result, err := exprlang.Run(f.program, m)
	if err != nil {
		if f.fallback != nil {
			if f.fallbackObserver != nil {
				f.fallbackObserver(f.name, f.expression, err)
			}
			return f.runFallback(err)
		}
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}
	if result == nil && f.fallback != nil && f.fallbackOnNil {
		return f.runFallback(nil)
	}
	return result, nil
}
```

(`runFallback` is unchanged: it still joins `mainErr` into a returned error if the fallback itself fails.)

- [ ] **Step 5: Update the existing test that asserted the old default**

Find the existing test in `expr/function_test.go` that asserts a `nil` main result returns the fallback (the audit named `require.NoError` locking in the masking around a `nil`/error case). Update it to the new semantics: a `nil` main with a fallback but no `WithFallbackOnNil` returns `nil`; keep the error-triggered-fallback assertion (that still returns the fallback). If ≥2 cases exercise `Apply` with varying input, fold them into an assert-closure table per the `table-test` skill.

- [ ] **Step 6: Run to verify pass**

Run: `go test ./expr/ -run 'TestApply' -v && go test ./expr/ -race`
Expected: PASS.

- [ ] **Step 7: Commit** (include ADR — see Task 5 note; ADR file authored by controller and added here or in Task 5)

```bash
git add expr/function.go expr/options.go expr/function_test.go
git commit -m "$(cat <<'EOF'
feat(expr)!: fallback fires on error only; nil stays first-class

BREAKING (pre-v0.1.0): a nil main result no longer triggers a configured
fallback by default — nil is a first-class value. Opt in with WithFallbackOnNil
for the old behavior. WithFallbackObserver surfaces the cause of an
error-triggered fallback so it is no longer silently discarded.

Spec: 012
Plan: 012
ADR: 0034
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Safe, honest coercion (`expr/predicate.go`)

`truthy` maps `NaN`→true and silently coerces unknown types to false. Make `NaN`/`±Inf`→false, an unhandled type an `EvalError`, and document the string rules precisely.

**Files:**
- Modify: `expr/predicate.go` (`Test` ~47-70, `truthy` ~77-106)
- Test: `expr/predicate_test.go`

**Interfaces:**
- Behavior change: a coerced (`WithCoerce`) predicate whose result is `NaN`/`±Inf` is `false` (was true for NaN); an unhandled result type (struct, `time.Time`, non-nil pointer, chan, func) is an `EvalError` (was silent false). ADR-0035.
- `truthy` becomes `truthy(v any) (bool, error)` (returns an error for unhandled kinds); `Test` propagates it.

- [ ] **Step 1: Write the failing tests**

Add to `expr/predicate_test.go` (fold into the existing coerce table if present, per `table-test`):

```go
func TestCoerceNaNIsFalse(t *testing.T) {
	p, err := expr.NewPredicate("x", expr.WithCoerce())
	require.NoError(t, err)
	got, err := p.Test(map[string]any{"x": math.NaN()})
	require.NoError(t, err)
	assert.False(t, got, "NaN must coerce to false, not true")
}

func TestCoerceUnknownTypeErrors(t *testing.T) {
	p, err := expr.NewPredicate("x", expr.WithCoerce())
	require.NoError(t, err)
	_, err = p.Test(map[string]any{"x": time.Now()})
	require.Error(t, err, "an unhandled result type must be a typed error, not a silent false")
	var ee *expr.EvalError
	require.ErrorAs(t, err, &ee)
}
```

(Add `math` and `time` imports to the test file.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./expr/ -run 'TestCoerceNaN|TestCoerceUnknownType' -v`
Expected: NaN case FAILS (returns true); unknown-type case FAILS (returns false, no error).

- [ ] **Step 3: Make `truthy` return an error and fix the cases**

Replace `truthy` in `expr/predicate.go`:

```go
// truthy implements lenient truthiness for WithCoerce predicates: nil is false;
// bool is itself; a string is parsed via strconv.ParseBool when it names one
// (1/t/T/TRUE/true/True and 0/f/F/FALSE/false/False), else true iff non-empty
// after trimming; any integer/uint kind is true iff non-zero; a float is true
// iff non-zero AND finite (NaN and ±Inf are false); a slice/array/map is true
// iff non-empty. Any other kind (struct, pointer, chan, func, time.Time, ...) is
// an error, so a mistyped predicate fails loudly instead of silently as false.
func truthy(v any) (bool, error) {
	if v == nil {
		return false, nil
	}
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		s := strings.TrimSpace(x)
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		}
		return s != "", nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0, nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		return f != 0 && !math.IsNaN(f) && !math.IsInf(f, 0), nil
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0, nil
	default:
		return false, fmt.Errorf("%w: cannot coerce %T to bool", ErrNotBool, v)
	}
}
```

Add `math` to the imports. `fmt` and `ErrNotBool` are already available (used by `Test`).

- [ ] **Step 4: Propagate the error in `Test`**

In `Test`, change the coerce branch:

```go
	if p.coerce {
		b, err := truthy(result)
		if err != nil {
			return false, &EvalError{Expression: p.expression, Cause: err}
		}
		return b, nil
	}
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./expr/ -run 'TestCoerce|TestPredicate' -v && go test ./expr/ -race`
Expected: PASS. Confirm the existing coerce cases (bool/string/number/slice) still pass; only NaN and unhandled-type behavior changed.

- [ ] **Step 6: Commit**

```bash
git add expr/predicate.go expr/predicate_test.go
git commit -m "$(cat <<'EOF'
fix(expr)!: safe, honest lenient truthiness

BREAKING (pre-v0.1.0): under WithCoerce, NaN and ±Inf now coerce to false
(was true for NaN), and an unhandled result type (struct, pointer, time.Time)
is a typed EvalError instead of a silent false — a mistyped predicate fails
loudly. The string coercion rules are documented precisely.

Spec: 012
Plan: 012
ADR: 0035
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: ADRs, docs/examples, whole-branch gate

**Files:**
- Create (controller-authored, added here): `docs/adrs/0034-fallback-error-only-semantics.md`, `docs/adrs/0035-coercion-truthiness-rules.md`, `docs/adrs/0036-multi-rule-firing.md`
- Modify: `pipe/doc.go` and/or `expr/doc.go` if they summarize firing/fallback/coerce; a runnable `Example` for multi-rule firing (e.g. `pipe/firing_example_test.go` or extend an existing example)
- Modify: `README.md` if it documents fallback/coerce/firing behavior

**Interfaces:** none new — documentation + an `Example` test doubling as godoc.

- [ ] **Step 1: Controller authors the three ADR files** (Nygard format: Status/Date/Prompted-by/Context/Decision/Consequences), each citing Spec 012 / Plan 012 and the audit finding, and the behavior-change/SemVer note for 0034 and 0035. (The controller writes these before dispatching this task; the implementer `git add`s them.)

- [ ] **Step 2: Add a runnable multi-rule firing example**

Add an `Example` that runs a collect table and prints `FiringRulesFor`, demonstrating the explainability trail. Verify: `go test ./pipe/ -run '^Example' -v` PASS.

- [ ] **Step 3: Update doc comments / README** to reflect: multi-rule firing (`FiringRulesFor`), fallback-on-error-only + `WithFallbackOnNil`/`WithFallbackObserver`, and the coercion rules. Accurate to shipped code only.

- [ ] **Step 4: Whole-package gate**

Run: `go test ./... -race && go vet ./... && gofmt -l . && CGO_ENABLED=0 go build ./...` — all clean; `gofmt -l` empty. Coverage: `go test ./pipe/ ./expr/ -cover` ≥ 85%.

- [ ] **Step 5: Commit**

```bash
git add docs/adrs/0034-fallback-error-only-semantics.md docs/adrs/0035-coercion-truthiness-rules.md docs/adrs/0036-multi-rule-firing.md pipe/doc.go expr/doc.go pipe/firing_example_test.go README.md
git commit -m "$(cat <<'EOF'
docs(rlng): ADRs 0034-0036 + firing/fallback/coerce docs

Spec: 012
Plan: 012
ADR: 0034, 0035, 0036
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Only `git add` the doc files that actually exist/were changed.)

---

## Self-review

- **Spec coverage:** G1 (collect/any firing + multi-rule model) → Tasks 1–2; G2 (fallback error-only, nil first-class, observable) → Task 3; G3 (safe/honest coercion) → Task 4; ADRs/docs/examples → Task 5. Spec 012's out-of-scope note (numeric fidelity → Spec 014) is respected: no `foldNumeric`/`HitPolicyAny`-equality changes here.
- **Placeholder scan:** none — every code step shows complete code; the prose steps (Task 5) are documentation.
- **Type consistency:** `firing map[string][]FiringRule`, `recordFirings`, `FiringRulesFor` defined in Task 1 and consumed in Task 2; `WithFallbackOnNil`/`WithFallbackObserver` + `Function.fallbackOnNil`/`fallbackObserver` consistent across Task 3; `truthy(v any) (bool, error)` signature change applied in both Task 4 steps (definition + `Test` call site).
- **Sequencing:** Task 2 depends on Task 1 (recordFirings). Tasks 3 and 4 are independent of 1–2 and of each other. Task 5 last (ADRs/docs/gate). Behavior-change tasks (3, 4) each update the pre-existing test that asserted the old behavior.
- **Hot-path/typed-error branches covered:** collect-match firing, any-agreement firing, no-match default firing (existing), fallback-on-error (observer), fallback-on-nil opt-in, nil-first-class default, coerce NaN/±Inf→false, coerce unknown-type→EvalError.
