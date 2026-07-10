# Declarative Config (YAML/JSON Loaders) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `config` package that parses declarative pipeline definitions from YAML/JSON into a `*PipelineDef` and builds a `*stage.Pipeline` from it.

**Architecture:** A new `config` package with four concerns in four files: `errors.go` (`ConfigError`), `expr_def.go` (the reusable `ExprDef` with scalar-shorthand `UnmarshalYAML`/`UnmarshalJSON` and `options()`), `def.go` (`PipelineDef`/`StageDef`/`NamedExprDef`/`RuleDef`) with `parse.go` (`ParseYAML`/`ParseJSON`/`LoadFile`), and `build.go` (`(*PipelineDef).Build` mapping defs to `stage.New*` + `stage.NewPipeline`). Decode is delegated to `gopkg.in/yaml.v3` / `encoding/json`; expression/name validation is delegated to the existing `stage`/`expr` constructors; the builder adds only config-shape checks and wraps failures in a `ConfigError` naming the stage/field.

**Tech Stack:** Go 1.25+; `gopkg.in/yaml.v3` (the one new dependency); stdlib `encoding/json`, `os`, `path/filepath`. Reuses `github.com/kartaladev/rlng/stage` and `.../expr`. Tests use `github.com/stretchr/testify`.

## Global Constraints

- Module path `github.com/kartaladev/rlng`; new package `config`. (Spec 004 §Design)
- **Exactly one new consumer-visible dependency: `gopkg.in/yaml.v3`.** No `mimetype`, no `go-playground/validator`. (ADR-0008)
- **Pure Go, no cgo; no `os.Exit`/`log.Fatal`/`panic` on caller input** — return typed errors. (CLAUDE.md)
- **Typed, `errors.As`-reachable errors** naming the offending stage/field; `ConfigError.Cause` unwraps to the underlying `stage`/`expr`/pipeline error. (Spec 004 §Error model)
- **Delegate validation to existing constructors**; the builder adds only config-shape checks. (ADR-0007)
- **Tests follow the `table-test` skill:** `assert` closure form; parsers/Build take no `context.Context`, so tables are context-free (no lifecycle path this increment). (table-test skill)
- **Test-coverage gate:** target ≥ 85% on `config`; every hot-path + typed-error branch listed per task has a covering test. (CLAUDE.md §Test-coverage gate)
- Quality gates before delivery: `go build ./...`, `go vet ./...`, `gofmt -l .` empty, `golangci-lint run ./...`, `go test ./... -race`. `go mod tidy` updates `go.mod`/`go.sum` **once** (adds `yaml.v3`), then is a no-op. (CLAUDE.md §Library quality gates)

---

### Task 1: `ConfigError` + `ExprDef` scalar shorthand

**Files:**
- Create: `config/errors.go`, `config/expr_def.go`
- Test: `config/expr_def_test.go`
- Also stage in this task's commit (ride-with-code): `docs/adrs/0007-config-package-and-schema.md`, `docs/adrs/0008-config-dependencies.md`, `docs/plans/004-declarative-config.md`

**Interfaces:**
- Consumes: `github.com/kartaladev/rlng/expr` (`expr.Option`, `expr.WithFallback`, `expr.WithGlobals`, `expr.WithCoerce`); `gopkg.in/yaml.v3`.
- Produces: `type ConfigError struct{ Stage, Field string; Cause error }` (with `Error()`/`Unwrap()`); `type ExprDef struct{ Expr, Fallback string; Globals map[string]any; Coerce bool }` with `UnmarshalYAML(*yaml.Node) error`, `UnmarshalJSON([]byte) error`, and `options() []expr.Option`.

**Hot-path branches to cover:** YAML scalar → `Expr`; YAML mapping → fields; YAML non-scalar/non-mapping → `ConfigError`; JSON string → `Expr`; JSON object → fields; JSON invalid → error; `options()` with each of fallback/globals/coerce present and all absent; `ConfigError.Error()` for each of the four Stage/Field combinations and `Unwrap`.

- [ ] **Step 1: Write the failing tests**

Create `config/expr_def_test.go`:

```go
package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExprDefUnmarshalYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, e ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "scalar shorthand sets Expr",
			yaml: `price * qty`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "mapping decodes fields",
			yaml: "expr: base * 1.1\nfallback: \"0\"\ncoerce: true",
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "sequence node is rejected",
			yaml: `[1, 2, 3]`,
			assert: func(t *testing.T, e ExprDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e ExprDef
			err := yaml.Unmarshal([]byte(tc.yaml), &e)
			tc.assert(t, e, err)
		})
	}
}

func TestExprDefUnmarshalJSON(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		json   string
		assert func(t *testing.T, e ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "string shorthand sets Expr",
			json: `"price * qty"`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "object decodes fields",
			json: `{"expr": "base * 1.1", "fallback": "0", "coerce": true}`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "malformed json errors",
			json: `{bad`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e ExprDef
			err := json.Unmarshal([]byte(tc.json), &e)
			tc.assert(t, e, err)
		})
	}
}

func TestExprDefOptions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		def  ExprDef
		want int // number of options produced
	}

	cases := []testCase{
		{name: "none", def: ExprDef{Expr: "1"}, want: 0},
		{name: "fallback only", def: ExprDef{Expr: "1", Fallback: "0"}, want: 1},
		{name: "globals only", def: ExprDef{Expr: "1", Globals: map[string]any{"k": 1}}, want: 1},
		{name: "coerce only", def: ExprDef{Expr: "1", Coerce: true}, want: 1},
		{name: "all three", def: ExprDef{Expr: "1", Fallback: "0", Globals: map[string]any{"k": 1}, Coerce: true}, want: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Len(t, tc.def.options(), tc.want)
		})
	}
}

func TestConfigErrorMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		err  *ConfigError
		want string
	}

	cause := errors.New("boom")
	cases := []testCase{
		{name: "stage and field", err: &ConfigError{Stage: "s", Field: "f", Cause: cause}, want: `config: stage "s" field "f": boom`},
		{name: "stage only", err: &ConfigError{Stage: "s", Cause: cause}, want: `config: stage "s": boom`},
		{name: "field only", err: &ConfigError{Field: "f", Cause: cause}, want: `config: field "f": boom`},
		{name: "neither", err: &ConfigError{Cause: cause}, want: `config: boom`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.err.Error())
			assert.ErrorIs(t, tc.err, cause)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ 2>&1 | head`
Expected: FAIL — package/types undefined (and `yaml.v3` not yet in go.mod).

- [ ] **Step 3: Add the yaml dependency**

Run: `go get gopkg.in/yaml.v3@latest`
Expected: `go.mod`/`go.sum` updated with `gopkg.in/yaml.v3`.

- [ ] **Step 4: Write `config/errors.go`**

```go
// Package config parses declarative YAML/JSON pipeline definitions and builds
// stage.Pipeline values from them.
package config

import "fmt"

// ConfigError reports a failure loading or building a pipeline definition. It
// names the stage and field where known, and unwraps to the underlying cause
// (a decode error, a *stage.StageError, or a pipeline construction error).
type ConfigError struct {
	Stage string // "" when not stage-scoped (e.g. a decode error)
	Field string // "" when not field-scoped
	Cause error
}

func (e *ConfigError) Error() string {
	switch {
	case e.Stage != "" && e.Field != "":
		return fmt.Sprintf("config: stage %q field %q: %v", e.Stage, e.Field, e.Cause)
	case e.Stage != "":
		return fmt.Sprintf("config: stage %q: %v", e.Stage, e.Cause)
	case e.Field != "":
		return fmt.Sprintf("config: field %q: %v", e.Field, e.Cause)
	default:
		return fmt.Sprintf("config: %v", e.Cause)
	}
}

func (e *ConfigError) Unwrap() error { return e.Cause }
```

Note: `%v` on `e.Cause` is nil-safe (prints `<nil>`), so `Error()` never panics on a hand-built nil-`Cause` literal.

- [ ] **Step 5: Write `config/expr_def.go`**

```go
package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kartaladev/rlng/expr"
	"gopkg.in/yaml.v3"
)

// ExprDef is an expression with optional compile options. It decodes from
// either a scalar string (shorthand: the string is Expr) or a mapping with
// explicit fields.
type ExprDef struct {
	Expr     string         `yaml:"expr" json:"expr"`
	Fallback string         `yaml:"fallback" json:"fallback"`
	Globals  map[string]any `yaml:"globals" json:"globals"`
	Coerce   bool           `yaml:"coerce" json:"coerce"`
}

// UnmarshalYAML accepts a scalar (the expression) or a mapping (explicit fields).
func (e *ExprDef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		e.Expr = value.Value
		return nil
	case yaml.MappingNode:
		type raw ExprDef // alias breaks the UnmarshalYAML recursion
		var r raw
		if err := value.Decode(&r); err != nil {
			return err
		}
		*e = ExprDef(r)
		return nil
	default:
		return &ConfigError{Field: "expr", Cause: fmt.Errorf("expected a scalar or mapping, got yaml kind %d", value.Kind)}
	}
}

// UnmarshalJSON accepts a JSON string (the expression) or an object.
func (e *ExprDef) UnmarshalJSON(data []byte) error {
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		e.Expr = s
		return nil
	}
	type raw ExprDef // alias breaks the UnmarshalJSON recursion
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*e = ExprDef(r)
	return nil
}

// options maps the object form to expr.Option values.
func (e ExprDef) options() []expr.Option {
	var opts []expr.Option
	if e.Fallback != "" {
		opts = append(opts, expr.WithFallback(e.Fallback))
	}
	if len(e.Globals) > 0 {
		opts = append(opts, expr.WithGlobals(e.Globals))
	}
	if e.Coerce {
		opts = append(opts, expr.WithCoerce())
	}
	return opts
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./config/ -race -v 2>&1 | tail -30`
Expected: PASS (all subtests in the four test functions).

- [ ] **Step 7: Gates + commit**

```bash
go test ./config/ -race -cover
go vet ./config/ && gofmt -l config/
git add config/errors.go config/expr_def.go config/expr_def_test.go go.mod go.sum \
  docs/adrs/0007-config-package-and-schema.md docs/adrs/0008-config-dependencies.md \
  docs/plans/004-declarative-config.md
git commit -m "$(cat <<'MSG'
feat(config): ExprDef scalar shorthand and ConfigError

Add the config package foundations: a typed ConfigError that names the
stage/field and unwraps to the underlying cause, and the reusable ExprDef
that decodes from either a scalar string (shorthand) or a full mapping in
both YAML and JSON, mapping its object form to expr.Option values. Adds
gopkg.in/yaml.v3 as the one new consumer dependency (ADR-0008).

Spec: 004
Plan: 004
ADR: 0007
ADR: 0008
MSG
)"
```

---

### Task 2: Definition types + parsers

**Files:**
- Create: `config/def.go`, `config/parse.go`
- Test: `config/parse_test.go`

**Interfaces:**
- Consumes: `ExprDef` (Task 1); `gopkg.in/yaml.v3`; stdlib `encoding/json`, `os`, `path/filepath`, `strings`.
- Produces:
  - `type PipelineDef struct{ Stages []StageDef }`
  - `type StageDef struct{ Name, Type string; DependsOn []string; Expr, Condition *ExprDef; Output string; Exprs []NamedExprDef; HitPolicy string; Rules []RuleDef }`
  - `type NamedExprDef struct{ Name string; Priority int; Expr ExprDef }`
  - `type RuleDef struct{ Condition ExprDef; Decisions map[string]ExprDef }`
  - `func ParseYAML([]byte) (*PipelineDef, error)`, `func ParseJSON([]byte) (*PipelineDef, error)`, `func LoadFile(string) (*PipelineDef, error)`

**Hot-path branches to cover:** `ParseYAML` valid + malformed; `ParseJSON` valid + malformed; `LoadFile` `.yaml`/`.yml`/`.json` dispatch, unknown extension → `ConfigError`, unreadable path → `ConfigError`; a parsed def preserves stage list order and the scalar shorthand round-trips into `StageDef`.

- [ ] **Step 1: Write the failing tests**

Create `config/parse_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`

func TestParseYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name: "valid preserves order and shorthand",
			yaml: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
				assert.Equal(t, "base", d.Stages[0].Name)
				assert.Equal(t, "price * qty", d.Stages[0].Expr.Expr)
				assert.Equal(t, []string{"base"}, d.Stages[1].DependsOn)
			},
		},
		{
			name: "malformed yaml errors",
			yaml: "stages: [unclosed",
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := ParseYAML([]byte(tc.yaml))
			tc.assert(t, d, err)
		})
	}
}

func TestParseJSON(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		json   string
		assert func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name: "valid",
			json: `{"stages":[{"name":"base","type":"single-expr","expr":"price * qty"}]}`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
				assert.Equal(t, "price * qty", d.Stages[0].Expr.Expr)
			},
		},
		{
			name: "malformed",
			json: `{"stages": [`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := ParseJSON([]byte(tc.json))
			tc.assert(t, d, err)
		})
	}
}

func TestLoadFile(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		file    string // basename; contents chosen by ext
		content string
		assert  func(t *testing.T, d *PipelineDef, err error)
	}

	cases := []testCase{
		{
			name:    "yaml extension",
			file:    "p.yaml",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
			},
		},
		{
			name:    "yml extension",
			file:    "p.yml",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 2)
			},
		},
		{
			name:    "json extension",
			file:    "p.json",
			content: `{"stages":[{"name":"a","type":"single-expr","expr":"1"}]}`,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				require.NoError(t, err)
				require.Len(t, d.Stages, 1)
			},
		},
		{
			name:    "unknown extension",
			file:    "p.txt",
			content: sampleYAML,
			assert: func(t *testing.T, d *PipelineDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))
			d, err := LoadFile(path)
			tc.assert(t, d, err)
		})
	}
}

func TestLoadFileMissing(t *testing.T) {
	t.Parallel()
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	var ce *ConfigError
	require.ErrorAs(t, err, &ce)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run 'Parse|LoadFile'`
Expected: FAIL — `PipelineDef`/parsers undefined.

- [ ] **Step 3: Write `config/def.go`**

```go
package config

// PipelineDef is a declarative pipeline: an ordered list of stages. List order
// is authoring order, which Build preserves into stage.NewPipeline so the
// pipeline's deterministic tie-break ordering is reproducible.
type PipelineDef struct {
	Stages []StageDef `yaml:"stages" json:"stages"`
}

// StageDef is a flat union over the three stage types, selected by Type. Fields
// not relevant to Type are ignored by Build.
type StageDef struct {
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"` // single-expr | multi-expr | decision-table
	DependsOn []string `yaml:"depends_on" json:"depends_on"`

	// single-expr
	Expr      *ExprDef `yaml:"expr" json:"expr"`
	Condition *ExprDef `yaml:"condition" json:"condition"`
	Output    string   `yaml:"output" json:"output"`

	// multi-expr
	Exprs []NamedExprDef `yaml:"exprs" json:"exprs"`

	// decision-table
	HitPolicy string    `yaml:"hit_policy" json:"hit_policy"` // single (default) | collect
	Rules     []RuleDef `yaml:"rules" json:"rules"`
}

// NamedExprDef is one entry of a multi-expr stage.
type NamedExprDef struct {
	Name     string  `yaml:"name" json:"name"`
	Priority int     `yaml:"priority" json:"priority"`
	Expr     ExprDef `yaml:"expr" json:"expr"`
}

// RuleDef is one rule of a decision-table stage.
type RuleDef struct {
	Condition ExprDef            `yaml:"condition" json:"condition"`
	Decisions map[string]ExprDef `yaml:"decisions" json:"decisions"`
}
```

- [ ] **Step 4: Write `config/parse.go`**

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAML decodes a PipelineDef from YAML.
func ParseYAML(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// ParseJSON decodes a PipelineDef from JSON.
func ParseJSON(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// LoadFile reads a config file and decodes it by extension: .yaml/.yml as YAML,
// .json as JSON. Any other extension is a ConfigError.
func LoadFile(path string) (*PipelineDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return ParseYAML(data)
	case ".json":
		return ParseJSON(data)
	default:
		return nil, &ConfigError{Cause: fmt.Errorf("unsupported config extension %q", filepath.Ext(path))}
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./config/ -race -v 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Gates + commit**

```bash
go vet ./config/ && gofmt -l config/
git add config/def.go config/parse.go config/parse_test.go
git commit -m "$(cat <<'MSG'
feat(config): definition types and YAML/JSON parsers

Add PipelineDef/StageDef/NamedExprDef/RuleDef and the ParseYAML,
ParseJSON, and LoadFile (extension-dispatched) parsers. Stage lists
preserve authoring order for deterministic pipeline ordering.

Spec: 004
Plan: 004
ADR: 0007
MSG
)"
```

---

### Task 3: Builder — `(*PipelineDef).Build`

**Files:**
- Create: `config/build.go`
- Test: `config/build_test.go`

**Interfaces:**
- Consumes: `PipelineDef`/`StageDef`/`NamedExprDef`/`RuleDef`/`ExprDef` (Tasks 1–2); `github.com/kartaladev/rlng/stage` (`NewSingleExpr`, `NewMultiExpr`, `NewDecisionTable`, `NamedExpr`, `Rule`, `HitPolicySingle`/`HitPolicyCollect`, `WithDependsOn`, `WithOutput`, `WithCondition`, `WithExprOptions`, `WithHitPolicy`, `NewPipeline`, `Pipeline`, `Stage`, type constants).
- Produces: `func (d *PipelineDef) Build() (*stage.Pipeline, error)`.

**Hot-path branches to cover:** each of single-expr/multi-expr/decision-table built and runnable; unknown/empty `type` → `ConfigError`; single-expr missing `expr` → `ConfigError`; multi-expr empty `exprs` → `ConfigError`; decision-table empty `rules` → `ConfigError`; invalid `hit_policy` → `ConfigError`; `hit_policy` `""`/`single`/`collect` accepted; per-decision options rejected → `ConfigError`; a bad expression surfaces a `ConfigError` unwrapping to `*stage.StageError`; a cycle/duplicate/unknown-dep from `NewPipeline` surfaces as `ConfigError`; condition + output options applied.

- [ ] **Step 1: Write the failing tests**

Create `config/build_test.go`:

```go
package config

import (
	"testing"

	"github.com/kartaladev/rlng/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		def    PipelineDef
		assert func(t *testing.T, p *stage.Pipeline, err error)
	}

	sd := func(s StageDef) PipelineDef { return PipelineDef{Stages: []StageDef{s}} }

	cases := []testCase{
		{
			name: "single-expr builds and runs",
			def: PipelineDef{Stages: []StageDef{
				{Name: "base", Type: "single-expr", Expr: &ExprDef{Expr: "price * qty"}},
				{Name: "taxed", Type: "single-expr", Expr: &ExprDef{Expr: "base * 1.1"}, DependsOn: []string{"base"}},
			}},
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("taxed")
				require.True(t, ok)
				assert.InDelta(t, 22.0, v, 1e-9)
			},
		},
		{
			name: "decision-table collect builds and runs",
			def: sd(StageDef{
				Name: "tiers", Type: "decision-table", HitPolicy: "collect",
				Rules: []RuleDef{
					{Condition: ExprDef{Expr: "amount > 100"}, Decisions: map[string]ExprDef{"tag": {Expr: `"big"`}}},
					{Condition: ExprDef{Expr: "amount > 0"}, Decisions: map[string]ExprDef{"tag": {Expr: `"pos"`}}},
				},
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(map[string]any{"amount": 150})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("tiers.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "pos"}, v)
			},
		},
		{
			name: "multi-expr builds and runs",
			def: sd(StageDef{
				Name: "calc", Type: "multi-expr",
				Exprs: []NamedExprDef{
					{Name: "a", Priority: 0, Expr: ExprDef{Expr: "2"}},
					{Name: "b", Priority: 1, Expr: ExprDef{Expr: "a * 3"}},
				},
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("calc.b")
				require.True(t, ok)
				assert.Equal(t, 6, v)
			},
		},
		{
			name: "condition and output applied",
			def: sd(StageDef{
				Name: "gated", Type: "single-expr", Expr: &ExprDef{Expr: "99"},
				Condition: &ExprDef{Expr: "false"}, Output: "result",
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				_, ok := sc.Get("result")
				assert.False(t, ok) // condition false => no write
			},
		},
		{
			name:   "unknown type",
			def:    sd(StageDef{Name: "x", Type: "bogus"}),
			assert: assertConfigErr,
		},
		{
			name:   "single-expr missing expr",
			def:    sd(StageDef{Name: "x", Type: "single-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "multi-expr empty exprs",
			def:    sd(StageDef{Name: "x", Type: "multi-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "decision-table empty rules",
			def:    sd(StageDef{Name: "x", Type: "decision-table"}),
			assert: assertConfigErr,
		},
		{
			name: "invalid hit policy",
			def: sd(StageDef{Name: "x", Type: "decision-table", HitPolicy: "weird",
				Rules: []RuleDef{{Condition: ExprDef{Expr: "true"}, Decisions: map[string]ExprDef{"k": {Expr: "1"}}}}}),
			assert: assertConfigErr,
		},
		{
			name: "per-decision options rejected",
			def: sd(StageDef{Name: "x", Type: "decision-table",
				Rules: []RuleDef{{Condition: ExprDef{Expr: "true"}, Decisions: map[string]ExprDef{"k": {Expr: "1", Fallback: "0"}}}}}),
			assert: assertConfigErr,
		},
		{
			name: "bad expression surfaces StageError",
			def:  sd(StageDef{Name: "x", Type: "single-expr", Expr: &ExprDef{Expr: "1 +"}}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "cycle surfaces pipeline error",
			def: PipelineDef{Stages: []StageDef{
				{Name: "a", Type: "single-expr", Expr: &ExprDef{Expr: "1"}, DependsOn: []string{"b"}},
				{Name: "b", Type: "single-expr", Expr: &ExprDef{Expr: "1"}, DependsOn: []string{"a"}},
			}},
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *stage.CycleError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := tc.def.Build()
			tc.assert(t, p, err)
		})
	}
}

func assertConfigErr(t *testing.T, p *stage.Pipeline, err error) {
	t.Helper()
	assert.Nil(t, p)
	var ce *ConfigError
	require.ErrorAs(t, err, &ce)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./config/ -run TestBuild`
Expected: FAIL — `Build` undefined.

- [ ] **Step 3: Write `config/build.go`**

```go
package config

import (
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

// Build compiles the definition into a *stage.Pipeline, mapping each StageDef to
// the matching stage constructor in list order. Expression and name validation
// is delegated to the stage/expr constructors; Build adds config-shape checks
// and wraps failures in a ConfigError naming the stage.
func (d *PipelineDef) Build() (*stage.Pipeline, error) {
	stages := make([]stage.Stage, 0, len(d.Stages))
	for _, sd := range d.Stages {
		st, err := sd.build()
		if err != nil {
			return nil, err
		}
		stages = append(stages, st)
	}
	p, err := stage.NewPipeline(stages...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return p, nil
}

func (sd StageDef) build() (stage.Stage, error) {
	var base []stage.Option
	if len(sd.DependsOn) > 0 {
		base = append(base, stage.WithDependsOn(sd.DependsOn...))
	}
	switch sd.Type {
	case stage.TypeSingleExpr:
		return sd.buildSingle(base)
	case stage.TypeMultiExpr:
		return sd.buildMulti(base)
	case stage.TypeDecisionTable:
		return sd.buildTable(base)
	default:
		return nil, &ConfigError{Stage: sd.Name, Field: "type", Cause: fmt.Errorf("unknown stage type %q", sd.Type)}
	}
}

func (sd StageDef) buildSingle(base []stage.Option) (stage.Stage, error) {
	if sd.Expr == nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "expr", Cause: errors.New("single-expr requires an expr")}
	}
	opts := append([]stage.Option{}, base...)
	opts = append(opts, stage.WithExprOptions(sd.Expr.options()...))
	if sd.Condition != nil {
		opts = append(opts, stage.WithCondition(sd.Condition.Expr, sd.Condition.options()...))
	}
	if sd.Output != "" {
		opts = append(opts, stage.WithOutput(sd.Output))
	}
	s, err := stage.NewSingleExpr(sd.Name, sd.Expr.Expr, opts...)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Cause: err}
	}
	return s, nil
}

func (sd StageDef) buildMulti(base []stage.Option) (stage.Stage, error) {
	if len(sd.Exprs) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "exprs", Cause: errors.New("multi-expr requires at least one expr")}
	}
	named := make([]stage.NamedExpr, 0, len(sd.Exprs))
	for _, e := range sd.Exprs {
		named = append(named, stage.NamedExpr{
			Name:       e.Name,
			Expression: e.Expr.Expr,
			Priority:   e.Priority,
			Options:    e.Expr.options(),
		})
	}
	s, err := stage.NewMultiExpr(sd.Name, named, base...)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Cause: err}
	}
	return s, nil
}

func (sd StageDef) buildTable(base []stage.Option) (stage.Stage, error) {
	if len(sd.Rules) == 0 {
		return nil, &ConfigError{Stage: sd.Name, Field: "rules", Cause: errors.New("decision-table requires at least one rule")}
	}
	hp, err := parseHitPolicy(sd.HitPolicy)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Field: "hit_policy", Cause: err}
	}
	rules := make([]stage.Rule, 0, len(sd.Rules))
	for i, r := range sd.Rules {
		decisions := make(map[string]string, len(r.Decisions))
		for key, ed := range r.Decisions {
			if len(ed.options()) > 0 {
				return nil, &ConfigError{
					Stage: sd.Name,
					Field: fmt.Sprintf("rules[%d].decisions.%s", i, key),
					Cause: errors.New("per-decision options are not supported; use a bare expression"),
				}
			}
			decisions[key] = ed.Expr
		}
		rules = append(rules, stage.Rule{
			Condition:        r.Condition.Expr,
			ConditionOptions: r.Condition.options(),
			Decisions:        decisions,
		})
	}
	opts := append([]stage.Option{}, base...)
	opts = append(opts, stage.WithHitPolicy(hp))
	s, err := stage.NewDecisionTable(sd.Name, rules, opts...)
	if err != nil {
		return nil, &ConfigError{Stage: sd.Name, Cause: err}
	}
	return s, nil
}

func parseHitPolicy(s string) (stage.HitPolicy, error) {
	switch s {
	case "", "single":
		return stage.HitPolicySingle, nil
	case "collect":
		return stage.HitPolicyCollect, nil
	default:
		return 0, fmt.Errorf("unknown hit policy %q", s)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./config/ -run TestBuild -race -v 2>&1 | tail -40`
Expected: PASS (all subtests).

- [ ] **Step 5: Coverage + gates + commit**

```bash
go test ./config/ -race -cover        # expect >= 85%; confirm every branch above is hit
go vet ./config/ && gofmt -l config/
git add config/build.go config/build_test.go
git commit -m "$(cat <<'MSG'
feat(config): Build a stage.Pipeline from a PipelineDef

Map each StageDef to its stage constructor (single/multi/decision-table)
in list order and assemble via stage.NewPipeline. Config-shape checks
(unknown type, missing required field, invalid hit_policy, per-decision
options) return a ConfigError naming the stage/field; expression and
pipeline errors surface unwrapped through it.

Spec: 004
Plan: 004
ADR: 0007
MSG
)"
```

---

### Task 4: Runnable `Example…` tests + package doc

**Files:**
- Create: `config/example_test.go`, `config/doc.go`

**Interfaces:**
- Consumes: `ParseYAML`, `(*PipelineDef).Build` (Tasks 2–3); `stage.NewScope`, `(*Scope).Get`, `(*Pipeline).Run`.
- Produces: nothing (documentation).

**Hot-path branches to cover:** the example exercises parse → build → run end to end (success path) via `// Output:` assertions.

- [ ] **Step 1: Write the failing example + doc**

Create `config/doc.go`:

```go
// Package config parses declarative YAML/JSON pipeline definitions and builds
// stage.Pipeline values from them.
//
// A definition is an ordered list of stages; each stage names its type
// (single-expr, multi-expr, or decision-table), its dependencies, and its
// type-specific fields. Expression fields accept either a bare string (the
// expression) or an object with compile options (expr, fallback, globals,
// coerce). Parse with ParseYAML/ParseJSON/LoadFile, then call Build to compile
// the definition into a *stage.Pipeline.
package config
```

Create `config/example_test.go`:

```go
package config_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/stage"
)

func ExampleParseYAML() {
	const src = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`
	def, err := config.ParseYAML([]byte(src))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	p, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}
	v, _ := sc.Get("taxed")
	fmt.Printf("%.1f\n", v)
	// Output: 22.0
}
```

- [ ] **Step 2: Run to verify pass**

Run: `go test ./config/ -run Example -v`
Expected: PASS (`ExampleParseYAML` output `22.0`).

- [ ] **Step 3: Full gate + commit**

```bash
go test ./... -race
go vet ./... && gofmt -l . && go mod tidy && git diff --exit-code go.mod go.sum
git add config/example_test.go config/doc.go
git commit -m "$(cat <<'MSG'
docs(config): package doc and runnable parse->build->run example

Spec: 004
Plan: 004
MSG
)"
```

---

## Post-implementation (increment delivery — outside the task loop)

1. **Whole-branch gate:** `/code-review` over `main..HEAD`, then `/security-review` on the branch diff (pay attention to `LoadFile`/`os.ReadFile` path handling and YAML decode of untrusted input). Resolve/triage every finding. Confirm the coverage gate (≥85% on `config`; every hot-path/typed-error branch tested).
2. **Re-run** `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` (clean), `go mod tidy` (no-op after the yaml add), `go mod verify`, `govulncheck ./...` (if installed).
3. **Update** `docs/HANDOVER.md` at the increment boundary (state, next = Increment 5).
4. **Merge** `feat/config-loaders` → `main` (fast-forward), **push**, and **delete** the branch.
