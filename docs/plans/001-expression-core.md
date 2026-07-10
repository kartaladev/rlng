# Expression Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `rlng/expr` — the two atomic evaluators (`Predicate`, `Function`) that every higher layer of the rule + calculation engine composes from.

**Architecture:** Compile once from Go strings on top of `expr-lang/expr`; evaluate many over a `map[string]any` (or a struct converted to one). Config-declared variables are injected as `??` defaults at compile time via an AST patcher. All failures are typed, `Unwrap`-able errors that name the field and expression.

**Tech Stack:** Go 1.25+, `github.com/expr-lang/expr` (only direct dependency). Std `reflect`, `errors`, `strconv`, `strings`, `testing`.

**Traceability:** Implements **Spec 001** (`docs/specs/001-expression-core.md`). Records **ADR-0001** (module path + naming) in Task 1. Every implementation commit carries a `Spec: 001` trailer (and Task 1 also `ADR: 0001`).

## Global Constraints

- Module path: `github.com/kartaladev/rlng`. Go **1.25+**.
- **Pure Go, no cgo:** `CGO_ENABLED=0 go build ./...` must pass.
- **One direct dependency** this increment: `github.com/expr-lang/expr` — introduced in **Task 3** (the first task that imports it), not Task 1. Do **not** add `lestrrat-go/option` or `mapstructure`.
- **No global logger:** library code must not log to a global logger. Non-scalar variables that cannot be patched are skipped **silently** (defined behavior) — this refines Spec 001's "logged warning" wording to satisfy the no-global-logger rule.
- Library code must not call `os.Exit`/`log.Fatal`/`panic` on caller input — return typed errors.
- Package `expr` imports the dependency aliased as `exprlang "github.com/expr-lang/expr"`.
- Tests use the `table-test` skill form (`assert` closure, `t.Context()` when a context is present). Add runnable `Example…` tests as godoc.
- **Pre-commit gate (CLAUDE.md §Development workflow):** before the *final* commit of this increment, run `/code-review`, then `/security-review`, then `go test ./... -race` — all clean. Per-step commits below at minimum run `go test ./... -race`.

## File Structure

```
go.mod                          # module + expr dependency
docs/adrs/0001-module-path-and-naming.md
expr/
  errors.go        # CompileError, EvalError, ErrNotBool, errEmptyExpression
  errors_test.go
  env.go           # toEnv: map passthrough / struct→map conversion
  env_test.go
  variables.go     # patcher: globals/locals → `??` default literals
  variables_test.go
  options.go       # Option, config, newConfig, With* options
  predicate.go     # Predicate, NewPredicate, Test
  predicate_test.go
  predicate_example_test.go
  function.go      # Function, NewFunction, Apply
  function_test.go
  function_example_test.go
  doc.go           # package doc
```

---

### Task 1: Bootstrap module + typed errors + ADR-0001

**Files:**
- Create: `go.mod`, `expr/errors.go`, `expr/errors_test.go`, `docs/adrs/0001-module-path-and-naming.md`

**Interfaces:**
- Produces: `CompileError{Name, Expression string; Cause error}`, `EvalError{Name, Expression string; Cause error}` (both implement `error` + `Unwrap`); sentinels `ErrNotBool`, `errEmptyExpression` (unexported).

- [ ] **Step 1: Initialize the module**

```bash
cd /Users/zakyalvan/Documents/RND/rlng
go mod init github.com/kartaladev/rlng
```
Expected: `go.mod` created with a `go 1.25`+ line and the module path, and **no `require` block yet** — Task 1 code imports only the standard library. Do **not** `go get expr` here: an unimported dependency is pruned immediately by `go mod tidy`. The `expr-lang/expr` dependency is introduced in Task 3 (the first task that imports it), pinned to `v1.17.8`.

- [ ] **Step 2: Write the failing test** — `expr/errors_test.go`

```go
package expr

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	inner := errors.New("boom")

	assert := func(t *testing.T, got error, wantMsg string, wantUnwrap error) {
		t.Helper()
		if got.Error() != wantMsg {
			t.Fatalf("message = %q, want %q", got.Error(), wantMsg)
		}
		if !errors.Is(got, wantUnwrap) {
			t.Fatalf("errors.Is(%v, %v) = false, want true", got, wantUnwrap)
		}
	}

	tests := []struct {
		name      string
		err       error
		wantMsg   string
		wantChain error
	}{
		{
			name:      "compile error names field and expression",
			err:       &CompileError{Name: "discount", Expression: "x >", Cause: inner},
			wantMsg:   `compile "discount" (x >): boom`,
			wantChain: inner,
		},
		{
			name:      "eval error names field and expression",
			err:       &EvalError{Name: "discount", Expression: "x + y", Cause: inner},
			wantMsg:   `eval "discount" (x + y): boom`,
			wantChain: inner,
		},
		{
			name:      "eval error wraps ErrNotBool",
			err:       &EvalError{Expression: "x + 1", Cause: ErrNotBool},
			wantMsg:   `eval (x + 1): expression did not evaluate to bool`,
			wantChain: ErrNotBool,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert(t, tc.err, tc.wantMsg, tc.wantChain)
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./expr/ -run TestErrors -v`
Expected: FAIL — `undefined: CompileError` / `undefined: EvalError` / `undefined: ErrNotBool`.

- [ ] **Step 4: Write minimal implementation** — `expr/errors.go`

```go
// Package expr provides the atomic expression evaluators — Predicate and
// Function — that the rest of rlng composes from.
package expr

import (
	"errors"
	"fmt"
)

// ErrNotBool is returned (wrapped in an EvalError) when a strict Predicate's
// expression evaluates to a non-boolean value.
var ErrNotBool = errors.New("expression did not evaluate to bool")

// errEmptyExpression is returned (wrapped in a CompileError) when an empty or
// whitespace-only expression is supplied to NewPredicate/NewFunction.
var errEmptyExpression = errors.New("expression must not be empty")

// CompileError reports a failure to compile an expression. It names the field
// (if any) and the offending expression, and unwraps to the underlying cause.
type CompileError struct {
	Name       string
	Expression string
	Cause      error
}

func (e *CompileError) Error() string {
	return "compile " + label(e.Name, e.Expression) + ": " + e.Cause.Error()
}

func (e *CompileError) Unwrap() error { return e.Cause }

// EvalError reports a failure while evaluating a compiled expression. It names
// the field (if any) and the expression, and unwraps to the underlying cause.
type EvalError struct {
	Name       string
	Expression string
	Cause      error
}

func (e *EvalError) Error() string {
	return "eval " + label(e.Name, e.Expression) + ": " + e.Cause.Error()
}

func (e *EvalError) Unwrap() error { return e.Cause }

// label renders `"name" (expression)` when a name is present, else `(expression)`.
func label(name, expression string) string {
	if name != "" {
		return fmt.Sprintf("%q (%s)", name, expression)
	}
	return "(" + expression + ")"
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./expr/ -run TestErrors -v`
Expected: PASS.

- [ ] **Step 6: Write ADR-0001** — `docs/adrs/0001-module-path-and-naming.md`

```markdown
# ADR-0001 — Module path and rule-vs-calc naming

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 001 (docs/specs/001-expression-core.md)

## Context

rlng is a general-purpose engine spanning two tracks — rule evaluation (decision
tables) and calculation pipelines. The git remote is github.com/kartaladev/rlng.
Two naming questions were open: the Go module path, and whether the top-level
API should read as calculation (`Calculator`/`Calculate`, per the seed reference)
or something neutral.

## Decision

- Module path is `github.com/kartaladev/rlng` (ratifies the existing git remote).
- Top-level facade (increment 5) will be named neutrally: `Engine` / `Evaluate`,
  not `Calculator` / `Calculate`, because the library serves calculations *and*
  rules.
- The atomic evaluator layer uses rule/DMN vocabulary: `Predicate.Test` and
  `Function.Apply`.

## Consequences

- Consumers import `github.com/kartaladev/rlng`; a future major version is the
  only way to change the path.
- Naming is consistent across both tracks; the calc-reference names are adapted,
  not copied.
- Supersede this ADR rather than editing it if the naming changes.
```

- [ ] **Step 7: Verify build + module hygiene**

Run: `go build ./expr/ && go mod tidy && go vet ./expr/`
Expected: builds clean; `go mod tidy` is a no-op — `errors.go` imports only the standard library, so `go.mod` stays module + go line and **no `go.sum` is created** (delete an empty `go.sum` if one lingers from an earlier `go get`).

- [ ] **Step 8: Commit**

```bash
git add go.mod expr/errors.go expr/errors_test.go docs/adrs/0001-module-path-and-naming.md docs/plans/001-expression-core.md
git commit -m "feat(expr): scaffold module and typed error model" \
  -m "Adds the go module, expr-lang/expr dependency, and CompileError/EvalError/ErrNotBool. Records ADR-0001 (module path + naming) alongside the first code." \
  -m "Spec: 001" -m "ADR: 0001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Struct/map env conversion

**Files:**
- Create: `expr/env.go`, `expr/env_test.go`

**Interfaces:**
- Produces: `toEnv(env any) (map[string]any, error)` — `nil` → empty map; `map[string]any` → passthrough; `struct`/`*struct` → converted map (exported fields by Go field name, nested structs → nested maps, slices/arrays and maps converted element-wise, nil pointers → `nil`); any other kind → error.

- [ ] **Step 1: Write the failing test** — `expr/env_test.go`

```go
package expr

import (
	"reflect"
	"testing"
)

func TestToEnv(t *testing.T) {
	type Inner struct{ Ratio float64 }
	type Outer struct {
		Name  string
		Inner Inner
		Tags  []string
		Ptr   *Inner
	}

	assert := func(t *testing.T, got, want map[string]any, wantErr bool, err error) {
		t.Helper()
		if wantErr {
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}

	tests := []struct {
		name    string
		in      any
		want    map[string]any
		wantErr bool
	}{
		{"nil is empty env", nil, map[string]any{}, false},
		{"map passthrough", map[string]any{"a": 1}, map[string]any{"a": 1}, false},
		{
			name: "struct with nested, slice, nil pointer",
			in: Outer{
				Name:  "x",
				Inner: Inner{Ratio: 0.5},
				Tags:  []string{"a", "b"},
				Ptr:   nil,
			},
			want: map[string]any{
				"Name":  "x",
				"Inner": map[string]any{"Ratio": 0.5},
				"Tags":  []any{"a", "b"},
				"Ptr":   nil,
			},
		},
		{"unsupported kind errors", 42, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toEnv(tc.in)
			assert(t, got, tc.want, tc.wantErr, err)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./expr/ -run TestToEnv -v`
Expected: FAIL — `undefined: toEnv`.

- [ ] **Step 3: Write minimal implementation** — `expr/env.go`

```go
package expr

import (
	"fmt"
	"reflect"
)

// toEnv normalizes an evaluation environment to a map[string]any. A nil env
// becomes an empty map; a map[string]any is returned unchanged; a struct or
// pointer-to-struct is converted field-by-field. Any other kind is an error.
func toEnv(env any) (map[string]any, error) {
	if env == nil {
		return map[string]any{}, nil
	}
	if m, ok := env.(map[string]any); ok {
		return m, nil
	}

	rv := reflect.ValueOf(env)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return map[string]any{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("env must be map[string]any or struct, got %T", env)
	}
	return structToMap(rv), nil
}

func structToMap(v reflect.Value) map[string]any {
	out := make(map[string]any, v.NumField())
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}
		out[t.Field(i).Name] = convertValue(v.Field(i))
	}
	return out
}

func convertValue(v reflect.Value) any {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		return structToMap(v)
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = convertValue(v.Index(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = convertValue(iter.Value())
		}
		return out
	default:
		return v.Interface()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./expr/ -run TestToEnv -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add expr/env.go expr/env_test.go
git commit -m "feat(expr): struct and map environment conversion" \
  -m "toEnv normalizes nil/map/struct inputs into map[string]any for evaluation." \
  -m "Spec: 001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Variable patcher (`??` defaults)

**Files:**
- Create: `expr/variables.go`, `expr/variables_test.go`

**Interfaces:**
- Consumes: `exprlang "github.com/expr-lang/expr"` and its `ast` subpackage.
- Produces: `newPatcher(globals, locals map[string]any) *variablePatcher` (returns `nil` when both maps are empty so callers can skip `exprlang.Patch`); `variablePatcher` implements `ast.Visitor`. Lookup precedence: locals → globals. Only scalar kinds (bool/string/int*/uint*/float*) are patched into `ident ?? literal`; non-scalars and nil pointers are skipped silently.

**Dependency (first importer):** this is the first task that imports `github.com/expr-lang/expr`. Add it pinned before writing the test so the module resolves:

```bash
go get github.com/expr-lang/expr@v1.17.8
```
Expected: `go.mod` gains `require github.com/expr-lang/expr v1.17.8`; `go.sum` is written. The commit below therefore includes `go.mod` and `go.sum`.

- [ ] **Step 1: Write the failing test** — `expr/variables_test.go`

```go
package expr

import (
	"testing"

	exprlang "github.com/expr-lang/expr"
)

func TestVariablePatcher(t *testing.T) {
	assert := func(t *testing.T, src string, globals, locals, env map[string]any, want any) {
		t.Helper()
		opts := []exprlang.Option{exprlang.AllowUndefinedVariables()}
		if p := newPatcher(globals, locals); p != nil {
			opts = append(opts, exprlang.Patch(p))
		}
		program, err := exprlang.Compile(src, opts...)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		got, err := exprlang.Run(program, env)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	tests := []struct {
		name             string
		src              string
		globals, locals  map[string]any
		env              map[string]any
		want             any
	}{
		{
			name:    "global default applies when env omits the key",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			env:     map[string]any{},
			want:    0.15,
		},
		{
			name:    "runtime env overrides the default",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			env:     map[string]any{"rate": 0.2},
			want:    0.2,
		},
		{
			name:    "local takes precedence over global",
			src:     "rate",
			globals: map[string]any{"rate": 0.15},
			locals:  map[string]any{"rate": 0.99},
			env:     map[string]any{},
			want:    0.99,
		},
		{
			name:    "nil patcher when no variables declared",
			src:     "1 + 1",
			env:     map[string]any{},
			want:    2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert(t, tc.src, tc.globals, tc.locals, tc.env, tc.want)
		})
	}
}

func TestNewPatcherNilWhenEmpty(t *testing.T) {
	if newPatcher(nil, nil) != nil {
		t.Fatal("expected nil patcher when no variables declared")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./expr/ -run 'TestVariablePatcher|TestNewPatcherNilWhenEmpty' -v`
Expected: FAIL — `undefined: newPatcher`.

- [ ] **Step 3: Write minimal implementation** — `expr/variables.go`

```go
package expr

import (
	"reflect"

	"github.com/expr-lang/expr/ast"
)

// variablePatcher rewrites each identifier that matches a declared variable
// into `identifier ?? <literal>`, so the variable acts as a default overridable
// by the runtime environment. Lookup precedence is locals, then globals. Only
// scalar values become literals; anything else is skipped (the identifier is
// then a normal, undefined-allowed lookup).
type variablePatcher struct {
	globals map[string]any
	locals  map[string]any
}

// newPatcher returns a patcher, or nil when no variables are declared so the
// caller can omit exprlang.Patch entirely.
func newPatcher(globals, locals map[string]any) *variablePatcher {
	if len(globals) == 0 && len(locals) == 0 {
		return nil
	}
	return &variablePatcher{globals: globals, locals: locals}
}

func (v *variablePatcher) lookup(name string) (any, bool) {
	if val, ok := v.locals[name]; ok {
		return val, true
	}
	val, ok := v.globals[name]
	return val, ok
}

// Visit implements ast.Visitor.
func (v *variablePatcher) Visit(node *ast.Node) {
	ident, ok := (*node).(*ast.IdentifierNode)
	if !ok {
		return
	}
	value, found := v.lookup(ident.Value)
	if !found {
		return
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return
	}

	var literal ast.Node
	switch rv.Kind() {
	case reflect.Bool:
		literal = &ast.BoolNode{Value: rv.Bool()}
	case reflect.String:
		literal = &ast.StringNode{Value: rv.String()}
	case reflect.Float32, reflect.Float64:
		literal = &ast.FloatNode{Value: rv.Float()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		literal = &ast.IntegerNode{Value: int(rv.Int())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		literal = &ast.IntegerNode{Value: int(rv.Uint())}
	default:
		return // non-scalar: skip silently (no global logging)
	}
	literal.SetType(rv.Type())

	ast.Patch(node, &ast.BinaryNode{Operator: "??", Left: ident, Right: literal})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./expr/ -run 'TestVariablePatcher|TestNewPatcherNilWhenEmpty' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum expr/variables.go expr/variables_test.go
git commit -m "feat(expr): compile-time variable defaults via ?? patcher" \
  -m "newPatcher injects scalar globals/locals as `ident ?? literal` defaults; locals win over globals; non-scalars skipped silently." \
  -m "Spec: 001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Predicate + options

**Files:**
- Create: `expr/options.go`, `expr/predicate.go`, `expr/predicate_test.go`, `expr/predicate_example_test.go`

**Interfaces:**
- Consumes: `toEnv`, `newPatcher`, `CompileError`, `EvalError`, `ErrNotBool`, `errEmptyExpression`.
- Produces:
  - `type Option func(*config)`; `config{globals, locals map[string]any; coerce bool; fallback string; returnKind reflect.Kind; hasReturnKind bool}`; `newConfig(opts []Option) *config`.
  - `WithGlobals(map[string]any) Option`, `WithLocals(map[string]any) Option`, `WithCoerce() Option` (Task 4), `WithFallback(string) Option` + `WithReturnKind(reflect.Kind) Option` (used in Task 5).
  - `type Predicate`; `NewPredicate(expression string, opts ...Option) (*Predicate, error)`; `(*Predicate) Test(env any) (bool, error)`.

- [ ] **Step 1: Write the failing test** — `expr/predicate_test.go`

```go
package expr

import (
	"errors"
	"testing"
)

func TestPredicate(t *testing.T) {
	assert := func(t *testing.T, p *Predicate, newErr error, env any, want bool, wantEvalErr error) {
		t.Helper()
		if newErr != nil {
			t.Fatalf("NewPredicate: %v", newErr)
		}
		got, err := p.Test(env)
		if wantEvalErr != nil {
			if !errors.Is(err, wantEvalErr) {
				t.Fatalf("Test err = %v, want Is %v", err, wantEvalErr)
			}
			return
		}
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		if got != want {
			t.Fatalf("Test = %v, want %v", got, want)
		}
	}

	t.Run("empty expression is a compile error", func(t *testing.T) {
		_, err := NewPredicate("   ")
		if !errors.Is(err, errEmptyExpression) {
			t.Fatalf("err = %v, want Is errEmptyExpression", err)
		}
	})

	t.Run("cases", func(t *testing.T) {
		tests := []struct {
			name        string
			expr        string
			opts        []Option
			env         any
			want        bool
			wantEvalErr error
		}{
			{name: "true condition", expr: "amount > 100", env: map[string]any{"amount": 150}, want: true},
			{name: "false condition", expr: "amount > 100", env: map[string]any{"amount": 50}, want: false},
			{name: "struct env", expr: "Amount > 100", env: struct{ Amount int }{Amount: 150}, want: true},
			{
				name: "global default used",
				expr: "amount > threshold",
				opts: []Option{WithGlobals(map[string]any{"threshold": 100})},
				env:  map[string]any{"amount": 150},
				want: true,
			},
			{
				name:        "strict: non-bool result errors",
				expr:        "amount + 1",
				env:         map[string]any{"amount": 1},
				wantEvalErr: ErrNotBool,
			},
			{
				name: "coerce: non-empty string is true",
				expr: "name",
				opts: []Option{WithCoerce()},
				env:  map[string]any{"name": "x"},
				want: true,
			},
			{
				name: "coerce: zero number is false",
				expr: "count",
				opts: []Option{WithCoerce()},
				env:  map[string]any{"count": 0},
				want: false,
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				p, err := NewPredicate(tc.expr, tc.opts...)
				assert(t, p, err, tc.env, tc.want, tc.wantEvalErr)
			})
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./expr/ -run TestPredicate -v`
Expected: FAIL — `undefined: NewPredicate` / `undefined: WithGlobals` / `undefined: WithCoerce`.

- [ ] **Step 3: Write the options implementation** — `expr/options.go`

```go
package expr

import "reflect"

type config struct {
	globals       map[string]any
	locals        map[string]any
	coerce        bool
	fallback      string
	returnKind    reflect.Kind
	hasReturnKind bool
}

// Option configures a Predicate or Function. Options that do not apply to a
// given evaluator are ignored: WithCoerce applies only to Predicate;
// WithFallback and WithReturnKind only to Function; WithGlobals/WithLocals to
// both.
type Option func(*config)

func newConfig(opts []Option) *config {
	c := &config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithGlobals sets engine-wide default variables, injected as `??` defaults.
func WithGlobals(vars map[string]any) Option { return func(c *config) { c.globals = vars } }

// WithLocals sets per-evaluator default variables; they take precedence over globals.
func WithLocals(vars map[string]any) Option { return func(c *config) { c.locals = vars } }

// WithCoerce makes a Predicate use lenient truthiness instead of the default
// strict (bool-only) mode.
func WithCoerce() Option { return func(c *config) { c.coerce = true } }

// WithFallback sets a Function's fallback expression, evaluated (over an empty
// env) when the main expression errors or yields nil.
func WithFallback(expression string) Option { return func(c *config) { c.fallback = expression } }

// WithReturnKind compiles a Function to coerce its result to the given kind.
func WithReturnKind(k reflect.Kind) Option {
	return func(c *config) { c.returnKind = k; c.hasReturnKind = true }
}
```

- [ ] **Step 4: Write the predicate implementation** — `expr/predicate.go`

```go
package expr

import (
	"fmt"
	"strconv"
	"strings"

	exprlang "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Predicate is a compiled boolean expression. It is safe for concurrent use.
type Predicate struct {
	expression string
	program    *vm.Program
	coerce     bool
}

// NewPredicate compiles expression into a Predicate. By default the expression
// must evaluate to a bool (strict); pass WithCoerce for lenient truthiness.
func NewPredicate(expression string, opts ...Option) (*Predicate, error) {
	src := strings.TrimSpace(expression)
	if src == "" {
		return nil, &CompileError{Expression: expression, Cause: errEmptyExpression}
	}

	cfg := newConfig(opts)
	exprOpts := []exprlang.Option{exprlang.AllowUndefinedVariables()}
	if p := newPatcher(cfg.globals, cfg.locals); p != nil {
		exprOpts = append(exprOpts, exprlang.Patch(p))
	}

	program, err := exprlang.Compile(src, exprOpts...)
	if err != nil {
		return nil, &CompileError{Expression: expression, Cause: err}
	}
	return &Predicate{expression: expression, program: program, coerce: cfg.coerce}, nil
}

// Test evaluates the predicate against env (a map[string]any or a struct).
func (p *Predicate) Test(env any) (bool, error) {
	m, err := toEnv(env)
	if err != nil {
		return false, &EvalError{Expression: p.expression, Cause: err}
	}

	result, err := exprlang.Run(p.program, m)
	if err != nil {
		return false, &EvalError{Expression: p.expression, Cause: err}
	}

	if p.coerce {
		return truthy(result), nil
	}

	b, ok := result.(bool)
	if !ok {
		return false, &EvalError{
			Expression: p.expression,
			Cause:      fmt.Errorf("%w: got %T", ErrNotBool, result),
		}
	}
	return b, nil
}

// truthy implements lenient truthiness for WithCoerce predicates.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(x)); err == nil {
			return b
		}
		return strings.TrimSpace(x) != ""
	case int:
		return x != 0
	case int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", x) != "0"
	case float32:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return false
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./expr/ -run TestPredicate -v`
Expected: PASS (all subtests).

- [ ] **Step 6: Add a runnable example** — `expr/predicate_example_test.go`

```go
package expr_test

import (
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

func ExamplePredicate() {
	p, err := expr.NewPredicate("amount > threshold",
		expr.WithGlobals(map[string]any{"threshold": 100}))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	ok, _ := p.Test(map[string]any{"amount": 150})
	fmt.Println(ok)
	// Output: true
}
```

- [ ] **Step 7: Run the example**

Run: `go test ./expr/ -run ExamplePredicate -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add expr/options.go expr/predicate.go expr/predicate_test.go expr/predicate_example_test.go
git commit -m "feat(expr): Predicate evaluator with functional options" \
  -m "Strict-by-default boolean evaluation with opt-in WithCoerce truthiness, variable defaults, and typed errors. Adds the shared Option/config plumbing." \
  -m "Spec: 001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Function + fallback

**Files:**
- Create: `expr/function.go`, `expr/function_test.go`, `expr/function_example_test.go`

**Interfaces:**
- Consumes: `toEnv`, `newPatcher`, `newConfig`, `WithFallback`, `WithReturnKind`, `CompileError`, `EvalError`, `errEmptyExpression`.
- Produces: `type Function`; `NewFunction(name, expression string, opts ...Option) (*Function, error)`; `(*Function) Apply(env any) (any, error)`.

- [ ] **Step 1: Write the failing test** — `expr/function_test.go`

```go
package expr

import (
	"errors"
	"testing"
)

func TestFunction(t *testing.T) {
	assert := func(t *testing.T, name, expr string, opts []Option, env any, want any, wantNewErr, wantApplyErr error) {
		t.Helper()
		f, err := NewFunction(name, expr, opts...)
		if wantNewErr != nil {
			if !errors.Is(err, wantNewErr) {
				t.Fatalf("NewFunction err = %v, want Is %v", err, wantNewErr)
			}
			return
		}
		if err != nil {
			t.Fatalf("NewFunction: %v", err)
		}
		got, err := f.Apply(env)
		if wantApplyErr != nil {
			if !errors.Is(err, wantApplyErr) {
				t.Fatalf("Apply err = %v, want Is %v", err, wantApplyErr)
			}
			return
		}
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if got != want {
			t.Fatalf("Apply = %v, want %v", got, want)
		}
	}

	tests := []struct {
		name         string
		fnName       string
		expr         string
		opts         []Option
		env          any
		want         any
		wantNewErr   error
		wantApplyErr error
	}{
		{name: "computes value", fnName: "total", expr: "price * qty", env: map[string]any{"price": 10, "qty": 3}, want: 30},
		{name: "empty expression errors", fnName: "x", expr: "  ", wantNewErr: errEmptyExpression},
		{
			name:   "fallback used on eval error",
			fnName: "ratio",
			expr:   "a / b",
			opts:   []Option{WithFallback("0.0")},
			env:    map[string]any{"a": 1, "b": 0}, // integer division by zero -> runtime error
			want:   0.0,
		},
		{
			name:         "no fallback surfaces eval error",
			fnName:       "ratio",
			expr:         "a / b",
			env:          map[string]any{"a": 1, "b": 0},
			wantApplyErr: nil, // replaced below
		},
	}

	// The "no fallback" case asserts an error occurs (any EvalError).
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "no fallback surfaces eval error" {
				f, err := NewFunction(tc.fnName, tc.expr)
				if err != nil {
					t.Fatalf("NewFunction: %v", err)
				}
				if _, err := f.Apply(tc.env); err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			assert(t, tc.fnName, tc.expr, tc.opts, tc.env, tc.want, tc.wantNewErr, tc.wantApplyErr)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./expr/ -run TestFunction -v`
Expected: FAIL — `undefined: NewFunction`.

- [ ] **Step 3: Write minimal implementation** — `expr/function.go`

```go
package expr

import (
	"strings"

	exprlang "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Function is a compiled value-producing expression with an optional fallback.
// It is safe for concurrent use.
type Function struct {
	name       string
	expression string
	program    *vm.Program
	fallback   *vm.Program
}

// NewFunction compiles expression into a named Function. A WithFallback
// expression, if given, is compiled now and evaluated when Apply's main
// expression errors or yields nil.
func NewFunction(name, expression string, opts ...Option) (*Function, error) {
	src := strings.TrimSpace(expression)
	if src == "" {
		return nil, &CompileError{Name: name, Expression: expression, Cause: errEmptyExpression}
	}

	cfg := newConfig(opts)
	base := []exprlang.Option{exprlang.AllowUndefinedVariables()}
	if p := newPatcher(cfg.globals, cfg.locals); p != nil {
		base = append(base, exprlang.Patch(p))
	}

	mainOpts := base
	if cfg.hasReturnKind {
		mainOpts = append(append([]exprlang.Option{}, base...), exprlang.AsKind(cfg.returnKind))
	}
	program, err := exprlang.Compile(src, mainOpts...)
	if err != nil {
		return nil, &CompileError{Name: name, Expression: expression, Cause: err}
	}

	fn := &Function{name: name, expression: expression, program: program}

	if fb := strings.TrimSpace(cfg.fallback); fb != "" {
		fbProgram, err := exprlang.Compile(fb, base...)
		if err != nil {
			return nil, &CompileError{Name: name, Expression: cfg.fallback, Cause: err}
		}
		fn.fallback = fbProgram
	}
	return fn, nil
}

// Apply evaluates the function against env (a map[string]any or a struct).
func (f *Function) Apply(env any) (any, error) {
	m, err := toEnv(env)
	if err != nil {
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}

	result, err := exprlang.Run(f.program, m)
	if err != nil {
		if f.fallback != nil {
			return f.runFallback()
		}
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}
	if result == nil && f.fallback != nil {
		return f.runFallback()
	}
	return result, nil
}

func (f *Function) runFallback() (any, error) {
	result, err := exprlang.Run(f.fallback, map[string]any{})
	if err != nil {
		return nil, &EvalError{Name: f.name, Expression: f.expression, Cause: err}
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./expr/ -run TestFunction -v`
Expected: PASS.

- [ ] **Step 5: Add a runnable example** — `expr/function_example_test.go`

```go
package expr_test

import (
	"fmt"

	"github.com/kartaladev/rlng/expr"
)

func ExampleFunction() {
	f, err := expr.NewFunction("total", "price * qty")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	got, _ := f.Apply(map[string]any{"price": 10, "qty": 3})
	fmt.Println(got)
	// Output: 30
}
```

- [ ] **Step 6: Run the example**

Run: `go test ./expr/ -run ExampleFunction -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add expr/function.go expr/function_test.go expr/function_example_test.go
git commit -m "feat(expr): Function evaluator with fallback and return kind" \
  -m "Value-producing expression; fallback runs on eval error or nil result; optional WithReturnKind coercion; typed errors." \
  -m "Spec: 001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Package doc + full quality gate

**Files:**
- Create: `expr/doc.go`

**Interfaces:**
- Consumes: everything above. Produces no new API.

- [ ] **Step 1: Write the package doc** — `expr/doc.go`

```go
// Package expr provides rlng's atomic expression evaluators, built on
// github.com/expr-lang/expr.
//
// Predicate compiles a boolean expression; by default it is strict (the result
// must be a bool) and returns an EvalError wrapping ErrNotBool otherwise. Pass
// WithCoerce for lenient truthiness.
//
// Function compiles a value-producing expression with an optional WithFallback
// expression, evaluated when the main expression errors or yields nil.
//
// Both accept an environment that is either a map[string]any or a struct
// (converted field-by-field), and both support WithGlobals/WithLocals default
// variables injected as `x ?? <default>` at compile time. All failures are
// *CompileError or *EvalError, which name the field and expression and unwrap
// to the underlying cause.
package expr
```

- [ ] **Step 2: Run the whole suite with the race detector**

Run: `go test ./... -race`
Expected: PASS, no data races.

- [ ] **Step 3: Run the library quality gates**

Run:
```bash
CGO_ENABLED=0 go build ./...
go vet ./...
gofmt -l .            # expect no output
go mod tidy           # expect go.mod/go.sum unchanged
```
Expected: all clean. (If `golangci-lint` / `govulncheck` are installed, run `golangci-lint run ./...` and `govulncheck ./...` too.)

- [ ] **Step 4: Pre-commit review gate (CLAUDE.md)**

Run `/code-review` on the diff and address findings; then `/security-review` and resolve anything flagged; then re-run `go test ./... -race`. Only proceed once all pass.

- [ ] **Step 5: Commit**

```bash
git add expr/doc.go
git commit -m "docs(expr): package documentation for the expression core" \
  -m "Spec: 001" \
  -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Link the plan back to the spec**

Update `docs/specs/001-expression-core.md` Traceability: change the Plan line to reference `docs/plans/001-expression-core.md`. Commit as `docs: link spec 001 to its plan` (`Spec: 001` trailer).

---

## Self-Review

**1. Spec coverage:**
- Predicate (strict + WithCoerce) → Task 4. ✓
- Function (+ fallback + return kind) → Task 5. ✓
- Variable defaults (`??`, locals→globals, scalar-only) → Task 3. ✓
- Typed errors (CompileError/EvalError/ErrNotBool, Unwrap) → Task 1. ✓
- Env conversion (map/struct, AllowUndefinedVariables) → Task 2 + compile options in Tasks 4/5. ✓
- Plain functional options, drop lestrrat-go/option → Task 4 options.go. ✓
- Package `rlng/expr`, `expr-lang/expr` only dep → Tasks 1–6; enforced in Task 6 `go mod tidy`. ✓
- ADR-0001 (module path + naming) → Task 1. ✓
- Testing: table-driven + Example tests + `-race` → all tasks + Task 6. ✓

**2. Placeholder scan:** No TBD/TODO; every code step contains complete code. The one intentional deviation (non-scalar variables skipped silently vs the spec's "logged warning") is called out in Global Constraints to satisfy the no-global-logger rule.

**3. Type consistency:** `newConfig`, `newPatcher`, `toEnv`, `Option`, `config` field names (`globals/locals/coerce/fallback/returnKind/hasReturnKind`) are defined in Tasks 1–4 and used consistently in Tasks 4–5. `NewPredicate`/`Test`, `NewFunction`/`Apply`, `CompileError`/`EvalError`/`ErrNotBool` names match across tasks and the spec.

## Notes / deviations

- **Spec refinement:** Spec 001 says non-patchable variables are "skipped with a logged warning"; this plan skips them **silently** (no global logging) per the CLAUDE.md library rule. Consider a follow-up `docs:` tweak to Spec 001's wording, or a later `WithLogger` option if surfacing these becomes valuable.
