# Config-path safety parity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the config/YAML authoring path to safety parity with the programmatic Go API — strict decoding of the expression object form, correct build-error attribution, lint enforcement at Build, and opt-in strict typed-env compilation from config.

**Architecture:** All changes live in the `config` package and build on existing `expr`/`pipe` primitives (`expr.WithEnv`, `(*PipelineDef).Lint`). A new variadic `BuildOption` on `Build` (backward compatible — `Build()` still works) carries the strict-env and lint-enforcement toggles; a new `PipelineDef.Schema` field (plus a `WithSchema` build option) declares the type-check environment threaded into every stage expression exactly as `constants` already are.

**Tech Stack:** Go 1.25, `gopkg.in/yaml.v3`, `encoding/json`, `github.com/expr-lang/expr` (via the local `expr` package), `stretchr/testify`.

## Global Constraints

- Go 1.25+; pure Go, no cgo (`CGO_ENABLED=0 go build ./...` must pass).
- Blackbox tests only: every `_test.go` uses `package config_test` and drives the exported API. Assert-closure table form (`assert func(t, ...)`), `t.Context()` over `context.Background()`.
- Library must not `panic`/`os.Exit`/`log.Fatal` on caller input; return typed errors. Config errors are `*ConfigError` naming the offending `Stage`/`Field`.
- Every exported symbol has a godoc comment. Minimal deps — add none.
- Target ≥ 85% statement coverage on `config`; every new error/validation branch has a covering test.
- Traceability: commits carry `Spec: 011`, `Plan: 011`, and the relevant `ADR:` trailer. Implements Spec 011; see `docs/specs/011-config-path-safety-parity.md`.

---

### Task 1: Honest strict decoding in the `ExprDef` object form (G2, ADR-0032)

The object form of `ExprDef` silently drops unknown keys in both YAML (`value.Decode` starts a fresh decoder without `KnownFields`) and JSON (`json.Unmarshal` without `DisallowUnknownFields`), so a typo'd `fallbck:`/`globals:`/`coerc:` is ignored. Make both reject unknown keys, matching the `PipelineDef`/`StageDef` contract.

**Files:**
- Modify: `config/expr_def.go:26-58` (`UnmarshalYAML`, `UnmarshalJSON`)
- Test: `config/expr_def_test.go` (add cases; blackbox `package config_test`)

**Interfaces:**
- Consumes: `config.ParseYAML(data []byte) (*PipelineDef, error)`, `config.ParseJSON(data []byte) (*PipelineDef, error)` (existing).
- Produces: no signature change; behavior change — an unknown key inside an `expr`/`condition`/`decision` object is now a decode error surfaced from `ParseYAML`/`ParseJSON`.

- [ ] **Step 1: Write the failing tests**

Add to `config/expr_def_test.go`:

```go
func TestExprDefObjectFormRejectsUnknownKeyYAML(t *testing.T) {
	doc := []byte(`
stages:
  - name: s
    type: single-expr
    expr:
      expr: "1"
      fallbck: "2"
`)
	_, err := config.ParseYAML(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallbck")
}

func TestExprDefObjectFormRejectsUnknownKeyJSON(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":{"expr":"1","fallbck":"2"}}]}`)
	_, err := config.ParseJSON(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallbck")
}

func TestExprDefObjectFormValidKeysStillParse(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":{"expr":"1","fallback":"2","coerce":true}}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	require.Equal(t, "1", d.Stages[0].Expr.Expr)
	require.Equal(t, "2", d.Stages[0].Expr.Fallback)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run 'TestExprDefObjectForm' -v`
Expected: the two "RejectsUnknownKey" tests FAIL (no error returned); the "ValidKeys" test passes.

- [ ] **Step 3: Implement strict object-form decoding**

In `config/expr_def.go`, replace the `MappingNode` branch of `UnmarshalYAML`:

```go
	case yaml.MappingNode:
		type raw ExprDef // alias breaks the UnmarshalYAML recursion
		var r raw
		if err := value.DecodeWithOptions(&r, yaml.DecodeOptions{KnownFields: true}); err != nil {
			return err
		}
		*e = ExprDef(r)
		return nil
```

If the pinned `yaml.v3` lacks `DecodeWithOptions`, fall back to validating keys against the known set before decoding:

```go
	case yaml.MappingNode:
		known := map[string]bool{"expr": true, "fallback": true, "globals": true, "coerce": true}
		for i := 0; i < len(value.Content); i += 2 {
			if k := value.Content[i].Value; !known[k] {
				return &ConfigError{Field: "expr", Cause: fmt.Errorf("unknown field %q", k)}
			}
		}
		type raw ExprDef
		var r raw
		if err := value.Decode(&r); err != nil {
			return err
		}
		*e = ExprDef(r)
		return nil
```

Replace the object branch of `UnmarshalJSON`:

```go
	type raw ExprDef // alias breaks the UnmarshalJSON recursion
	var r raw
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return err
	}
	*e = ExprDef(r)
	return nil
```

(`bytes` is already imported.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./config/ -run 'TestExprDefObjectForm' -v`
Expected: PASS (all three).

- [ ] **Step 5: Run the package suite**

Run: `go test ./config/ -race`
Expected: ok — confirm no existing valid-object-form test regressed.

- [ ] **Step 6: Commit**

```bash
git add config/expr_def.go config/expr_def_test.go
git commit -m "$(cat <<'EOF'
fix(config): reject unknown keys in expr object form (YAML+JSON)

The ExprDef object-form unmarshalers bypassed the strict decoding the rest
of the config path enforces, so a compile-option typo (fallbck, coerc) was
silently dropped. Both YAML and JSON object forms now reject unknown keys.

Spec: 011
Plan: 011
ADR: 0032
EOF
)"
```

---

### Task 2: Correct single-expr build-error attribution (G4)

When a single-expr stage's *value* expression fails to compile, `buildSingle` re-tests the condition and, if it also fails, blames `condition` — hiding the value-expr failure and double-compiling the predicate. Attribute the error to the field that actually failed.

**Files:**
- Modify: `config/build.go:82-93` (`buildSingle` failure path)
- Test: `config/build_test.go`

**Interfaces:**
- Consumes: `pipe.NewSingleExpr`, `expr.NewFunction`, `expr.NewPredicate` (existing).
- Produces: no signature change; a value-expr compile failure now yields `ConfigError{Stage, Field: "expr", ...}`; a condition-only failure yields `Field: "condition"`.

- [ ] **Step 1: Write the failing tests**

Add to `config/build_test.go`:

```go
func TestBuildSingleAttributesValueExprFailure(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":"@@@","condition":"###"}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build()
	require.Error(t, err)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, "s", ce.Stage)
	require.Equal(t, "expr", ce.Field) // the value expr is the real first failure
}

func TestBuildSingleAttributesConditionFailure(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":"1","condition":"###"}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build()
	require.Error(t, err)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, "condition", ce.Field)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run 'TestBuildSingleAttributes' -v`
Expected: `TestBuildSingleAttributesValueExprFailure` FAILS (`Field` is "condition", want "expr").

- [ ] **Step 3: Fix attribution in `buildSingle`**

Replace the failure block in `config/build.go` (lines 82-93):

```go
	s, err := pipe.NewSingleExpr(sd.Name, sd.Expr.Expr, opts...)
	if err != nil {
		// Attribute to the field that actually failed to compile. The value
		// expression is compiled first, so a value error is the true first
		// failure; only a value expression that compiles cleanly leaves the
		// condition as the culprit.
		if _, verr := expr.NewFunction(sd.Name, sd.Expr.Expr, withConstants(constants, sd.Expr.options())...); verr != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: verr}
		}
		if sd.Condition != nil {
			return nil, &ConfigError{Stage: sd.Name, Field: "condition", Cause: err}
		}
		return nil, &ConfigError{Cause: err}
	}
	return s, nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./config/ -run 'TestBuildSingle' -v`
Expected: PASS (both new tests and any existing single-expr build tests).

- [ ] **Step 5: Commit**

```bash
git add config/build.go config/build_test.go
git commit -m "$(cat <<'EOF'
fix(config): attribute single-expr build failure to the failing field

A value-expression compile error was misreported against `condition`,
pointing authors at the wrong expression. Attribute to `expr` when the
value expression is the culprit; `condition` only when the value compiles.

Spec: 011
Plan: 011
EOF
)"
```

---

### Task 3: `BuildOption` mechanism + lint enforcement at Build (G3, ADR-0033)

Add a backward-compatible variadic `BuildOption` to `Build`, and a `WithLintErrors` option that runs `Lint` during `Build` and promotes findings to a construction error. Also make lint honest about its syntactic catch-all heuristic (recognize a small semantic set, and document best-effort).

**Files:**
- Create: `config/build_options.go`
- Modify: `config/build.go:21-38` (`Build` signature + lint hook), `config/lint.go:102-105` (`isCatchAll`)
- Test: `config/build_options_test.go`, `config/lint_test.go`

**Interfaces:**
- Produces:
  - `type BuildOption func(*buildConfig)`
  - `func WithLintErrors() BuildOption` — promote all `Lint` findings to a `*LintError` returned from `Build`.
  - `type LintError struct{ Findings []Finding }`; `func (e *LintError) Error() string`.
  - `func (d *PipelineDef) Build(opts ...BuildOption) (*pipe.Pipeline, error)` (was no-arg).
- Consumes: `(*PipelineDef).Lint() []Finding` (existing).

- [ ] **Step 1: Write the failing tests**

`config/build_options_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/require"
)

func TestBuildDefaultDoesNotEnforceLint(t *testing.T) {
	// A first-match table with no default and no catch-all: a lint smell,
	// but the default Build stays advisory and succeeds.
	doc := []byte(`{"stages":[{"name":"t","type":"decision-table","rules":[{"condition":"x > 1","decisions":{"y":"1"}}]}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build()
	require.NoError(t, err)
}

func TestBuildWithLintErrorsPromotesFindings(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"t","type":"decision-table","rules":[{"condition":"x > 1","decisions":{"y":"1"}}]}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build(config.WithLintErrors())
	require.Error(t, err)
	var le *config.LintError
	require.ErrorAs(t, err, &le)
	require.NotEmpty(t, le.Findings)
	require.Equal(t, config.LintMissingDefault, le.Findings[0].Code)
}
```

Add to `config/lint_test.go`:

```go
func TestLintSemanticCatchAllNotFlaggedMissingDefault(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"t","type":"decision-table","rules":[{"condition":"1 == 1","decisions":{"y":"1"}}]}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	findings := d.Lint()
	for _, f := range findings {
		require.NotEqual(t, config.LintMissingDefault, f.Code, "1 == 1 is a catch-all; must not flag missing-default")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run 'TestBuild(Default|WithLint)|TestLintSemantic' -v`
Expected: compile failure (`WithLintErrors`, `LintError` undefined) → all FAIL.

- [ ] **Step 3: Implement `BuildOption` and `LintError`**

Create `config/build_options.go`:

```go
package config

import (
	"fmt"
	"strings"
)

// buildConfig holds Build-time toggles assembled from BuildOption values.
type buildConfig struct {
	lintErrors bool
	schema     map[string]any // set in Task 4
	strict     bool           // set in Task 4
}

// BuildOption configures (*PipelineDef).Build.
type BuildOption func(*buildConfig)

// WithLintErrors runs Lint during Build and promotes every finding to a
// *LintError, so an authoring smell (missing default, unreachable rule) fails
// construction instead of surfacing only if the caller separately calls Lint.
func WithLintErrors() BuildOption {
	return func(c *buildConfig) { c.lintErrors = true }
}

// LintError reports Lint findings promoted to a Build error by WithLintErrors.
type LintError struct{ Findings []Finding }

// Error renders the finding count and each finding's stage/code/message.
func (e *LintError) Error() string {
	msgs := make([]string, 0, len(e.Findings))
	for _, f := range e.Findings {
		msgs = append(msgs, fmt.Sprintf("stage %q: %s: %s", f.Stage, f.Code, f.Message))
	}
	return fmt.Sprintf("config: %d lint finding(s): %s", len(e.Findings), strings.Join(msgs, "; "))
}
```

- [ ] **Step 4: Wire the option into `Build` and fix `isCatchAll`**

In `config/build.go`, change the signature and add the lint hook at the end of `Build`:

```go
func (d *PipelineDef) Build(opts ...BuildOption) (*pipe.Pipeline, error) {
	cfg := &buildConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if len(d.Stages) == 0 {
		return nil, &ConfigError{Cause: ErrNoStages}
	}
	if cfg.lintErrors {
		if findings := d.Lint(); len(findings) > 0 {
			return nil, &LintError{Findings: findings}
		}
	}
	stages := make([]pipe.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build(d.Constants)
		if err != nil {
			return nil, err
		}
		stages = append(stages, st)
	}
	p, err := pipe.NewPipeline(stages...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return p, nil
}
```

In `config/lint.go`, broaden `isCatchAll` to a documented best-effort set and update the godoc:

```go
// isCatchAll reports whether a condition is an unconditional truth. Detection is
// best-effort and syntactic: it recognizes the literal `true`, a parenthesized
// `(true)`, and the trivial tautology `1 == 1`. A semantic always-true condition
// it does not recognize may still be flagged missing-default (a false positive);
// this is advisory analysis, not evaluation.
func isCatchAll(condition string) bool {
	s := strings.TrimSpace(condition)
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"))
	switch s {
	case "true", "1 == 1":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./config/ -run 'TestBuild(Default|WithLint)|TestLint' -v`
Expected: PASS. Then `go test ./config/ -race` — confirm existing lint/build tests still green (the `Build()` no-arg call sites still compile via the variadic).

- [ ] **Step 6: Update in-repo `Build()` call sites if needed**

Run: `grep -rn '\.Build(' --include='*.go' | grep -v '_test.go'`
Expected: variadic is backward compatible — no changes required. If any example passes exactly one arg positionally, it still matches `opts ...BuildOption`. Confirm `go build ./...` is clean.

- [ ] **Step 7: Commit**

```bash
git add config/build_options.go config/build.go config/lint.go config/build_options_test.go config/lint_test.go
git commit -m "$(cat <<'EOF'
feat(config): lint enforcement at Build + honest catch-all heuristic

Add a backward-compatible variadic BuildOption and WithLintErrors, which
promotes Lint findings (missing-default, unreachable-rule) to a *LintError
at construction. Broaden isCatchAll to a documented best-effort set
(true, (true), 1 == 1) so a real catch-all is not false-flagged, and state
the heuristic's limits in godoc.

Spec: 011
Plan: 011
ADR: 0033
EOF
)"
```

---

### Task 4: Strict typed env from config (G1, ADR-0031)

Add a `schema` block declaring the input shape, threaded into every stage expression via `expr.WithEnv` so a field typo is a Build-time error. Enabled by `WithStrict()` (errors without a schema) and injectable programmatically via `WithSchema`.

**Files:**
- Modify: `config/def.go:15-19` (add `Schema` field), `config/build.go` (thread schema; `withSchema` helper; pass through all stage builders), `config/build_options.go` (add `WithStrict`, `WithSchema`)
- Test: `config/strict_env_test.go` (new, blackbox)

**Interfaces:**
- Consumes: `expr.WithEnv(env map[string]any) expr.Option` (existing).
- Produces:
  - `PipelineDef.Schema map[string]any` (yaml/json `schema`).
  - `func WithStrict() BuildOption` — require strict compilation; a Build with no schema (neither `PipelineDef.Schema` nor `WithSchema`) returns a `*ConfigError`.
  - `func WithSchema(env map[string]any) BuildOption` — supply/override the type-check env programmatically.
  - Strict is enabled when a schema is present (via the block or `WithSchema`) or `WithStrict()` is passed.

- [ ] **Step 1: Write the failing tests**

`config/strict_env_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/require"
)

func TestStrictSchemaRejectsFieldTypoAtBuild(t *testing.T) {
	doc := []byte(`
schema:
  score: 0
stages:
  - name: gate
    type: single-expr
    expr: "scoer >= 650"
`)
	d, err := config.ParseYAML(doc)
	require.NoError(t, err)
	_, err = d.Build()
	require.Error(t, err, "typo 'scoer' must fail at build under a declared schema")
	require.Contains(t, err.Error(), "scoer")
}

func TestStrictSchemaAcceptsDeclaredField(t *testing.T) {
	doc := []byte(`
schema:
  score: 0
stages:
  - name: gate
    type: single-expr
    expr: "score >= 650"
`)
	d, err := config.ParseYAML(doc)
	require.NoError(t, err)
	_, err = d.Build()
	require.NoError(t, err)
}

func TestNoSchemaStaysLenient(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"gate","type":"single-expr","expr":"scoer >= 650"}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build() // lenient: undefined var tolerated, builds fine
	require.NoError(t, err)
}

func TestWithStrictWithoutSchemaErrors(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"gate","type":"single-expr","expr":"score >= 650"}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	_, err = d.Build(config.WithStrict())
	require.Error(t, err)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run 'Strict|NoSchema|WithStrict' -v`
Expected: compile failure (`WithStrict`, `WithSchema`, `Schema` undefined) → FAIL.

- [ ] **Step 3: Add the `Schema` field and build options**

In `config/def.go`, add to `PipelineDef`:

```go
	// Schema declares the input shape (field name -> a representative value
	// giving its type). When present, every stage expression compiles strictly
	// against it (expr.WithEnv): a field typo is a Build-time error instead of a
	// silent nil. Absent, compilation stays lenient (undefined vars allowed).
	Schema map[string]any `yaml:"schema" json:"schema"`
```

In `config/build_options.go`, add:

```go
// WithStrict requires strict compilation against the declared schema. Build
// returns a *ConfigError if no schema is available (neither PipelineDef.Schema
// nor WithSchema). Without WithStrict, strict mode is still enabled whenever a
// schema is present.
func WithStrict() BuildOption {
	return func(c *buildConfig) { c.strict = true }
}

// WithSchema supplies or overrides the type-check environment programmatically,
// for callers who cannot edit the document. It enables strict compilation.
func WithSchema(env map[string]any) BuildOption {
	return func(c *buildConfig) { c.schema = env }
}
```

- [ ] **Step 4: Thread the schema through Build into every stage**

In `config/build.go`, resolve the schema in `Build` and pass it down. Update `Build` (after the `cfg` assembly from Task 3):

```go
	schema := cfg.schema
	if schema == nil {
		schema = d.Schema
	}
	strict := cfg.strict || len(schema) > 0
	if cfg.strict && len(schema) == 0 {
		return nil, &ConfigError{Field: "schema", Cause: errors.New("strict build requires a schema")}
	}
```

Change `sd.build(d.Constants)` to `sd.build(d.Constants, schema, strict)` and thread `schema, strict` through `build`, `buildSingle`, `buildMulti`, `buildTable`. Add a `withStrictEnv` helper next to `withConstants`:

```go
// withStrictEnv appends expr.WithEnv(schema) to opts when strict, so the
// expression type-checks against the declared input shape. Declared
// globals/locals and registered functions are merged into the check env by
// expr.buildExprOpts, so they remain usable.
func withStrictEnv(strict bool, schema map[string]any, opts []expr.Option) []expr.Option {
	if !strict || len(schema) == 0 {
		return opts
	}
	return append(opts, expr.WithEnv(schema))
}
```

Wrap each `withConstants(...)` result that feeds a compiled expression with `withStrictEnv`. For example in `buildSingle`:

```go
	condOpts := withStrictEnv(strict, schema, withConstants(constants, sd.condOptions()))
	opts := append([]pipe.Option{}, base...)
	opts = append(opts, pipe.WithExprOptions(withStrictEnv(strict, schema, withConstants(constants, sd.Expr.options()))...))
```

and mirror it in the `buildSingle` attribution fallback (Task 2), `buildMulti` (`Options:` field), and `buildTable` (`ConditionOptions`, `DecisionOptions`, and the `WithDefault` options). Signatures become:

```go
func (sd StageDef) build(constants, schema map[string]any, strict bool) (pipe.Stage, error)
func (sd StageDef) buildSingle(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error)
func (sd StageDef) buildMulti(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error)
func (sd StageDef) buildTable(base []pipe.Option, constants, schema map[string]any, strict bool) (pipe.Stage, error)
```

(`errors` is already imported in `build.go`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./config/ -run 'Strict|NoSchema|WithStrict' -v`
Expected: PASS. The typo test's error contains `scoer` (surfaced from expr's strict compile via the `ConfigError` chain).

- [ ] **Step 6: Run the whole package suite + vet/fmt**

Run: `go test ./config/ -race && go vet ./config/ && gofmt -l config/`
Expected: ok; no vet output; `gofmt -l` prints nothing.

- [ ] **Step 7: Commit**

```bash
git add config/def.go config/build.go config/build_options.go config/strict_env_test.go
git commit -m "$(cat <<'EOF'
feat(config): strict typed env from a schema block

A top-level `schema` block (or WithSchema) declares the input shape; every
stage expression then compiles strictly via expr.WithEnv, turning a field
typo into a Build-time error instead of a silent nil misfire. WithStrict
requires a schema. Absent a schema, compilation stays lenient (unchanged).

Spec: 011
Plan: 011
ADR: 0031
EOF
)"
```

---

### Task 5: Config-surface docs + whole-branch gate

**Files:**
- Modify: `config/doc.go` (document `schema`, strict mode, lint-at-build), `config/example_test.go` (add a runnable strict-schema example)
- Modify: `README.md` (note the config `schema` block and `WithLintErrors`/`WithStrict`)

**Interfaces:** none new — documentation and an `Example` test doubling as godoc.

- [ ] **Step 1: Write a runnable Example test**

Add to `config/example_test.go`:

```go
func ExamplePipelineDef_Build_strict() {
	doc := []byte(`
schema:
  score: 0
stages:
  - name: gate
    type: single-expr
    expr: "score >= 650"
    output: eligible
`)
	d, _ := config.ParseYAML(doc)
	_, err := d.Build(config.WithStrict())
	fmt.Println(err)
	// Output: <nil>
}
```

- [ ] **Step 2: Run the example**

Run: `go test ./config/ -run 'ExamplePipelineDef_Build_strict' -v`
Expected: PASS.

- [ ] **Step 3: Update `config/doc.go` and `README.md`**

Add a short paragraph to `config/doc.go` documenting the `schema` block (opt-in strict compilation), `WithStrict`/`WithSchema`, and `WithLintErrors`. Add a matching bullet to the README's config section. (Prose — keep to the exact features shipped in Tasks 1–4.)

- [ ] **Step 4: Whole-package verification**

Run: `go test ./... -race && go vet ./... && gofmt -l . && CGO_ENABLED=0 go build ./...`
Expected: all green; `gofmt -l` empty. Check coverage: `go test ./config/ -cover` ≥ 85%.

- [ ] **Step 5: Commit**

```bash
git add config/doc.go config/example_test.go README.md
git commit -m "$(cat <<'EOF'
docs(config): document schema/strict mode and lint-at-build

Spec: 011
Plan: 011
EOF
)"
```

---

## Self-review

- **Spec coverage:** G1 → Task 4; G2 → Task 1; G3 → Task 3; G4 → Task 2; docs/examples → Task 5. All four spec goals + the hot-path test targets (object-form strict decoding both formats, lint-in-build promote + advisory default, semantic catch-all, strict-env accept/reject/lenient/no-schema, attribution both directions) are covered.
- **Placeholder scan:** none — every code step shows complete code; the one prose step (Task 5 Step 3) is documentation, not logic.
- **Type consistency:** `BuildOption`/`buildConfig`/`WithLintErrors`/`LintError`/`WithStrict`/`WithSchema` defined in Task 3–4 and used consistently; `build`/`buildSingle`/`buildMulti`/`buildTable` signature change is applied uniformly in Task 4 Step 4; `withStrictEnv`/`withConstants` compose in the same order everywhere.
- **Sequencing note:** Tasks 1, 2 are independent. Task 3 introduces `BuildOption`/`buildConfig`; Task 4 extends `buildConfig` (`schema`,`strict`) and the `build*` signatures — do Task 3 before Task 4. Task 5 last.
