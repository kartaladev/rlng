# Plan 026 — convenience constructors (B10)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add one-call constructors to the root `rlng` package that compose `config.Parse → Build → New`
(and the typed equivalent), so building an engine from a YAML/JSON config source is a single call.

- **Implements:** Spec 026 (`docs/specs/026-convenience-constructors.md`) — committed `400b27d`.
- **Records:** ADR-0051 (created in this task's commit); extends ADR-0009 (rlng→config additive convenience),
  records the deliberate re-deferral of Pipeline-as-Stage (ADR-0005).
- **Backlog:** graduates the constructors half of B10 (`docs/BACKLOG.md`); Pipeline-as-Stage re-deferred.

**Architecture:** One new file `fromconfig.go` in the root `rlng` package with four thin constructors that
thread `config.Parse` + `PipelineDef.Build` + `New`/`NewTypedEngine`, returning the first error unwrapped.
Purely additive; introduces an in-module `rlng → config` import (no new external dependency, acyclic).

**Tech Stack:** Go 1.25+; no new dependencies.

## Global Constraints

- Pure Go, no cgo; **no new external dependency** — `config` is same-module and already in `go.mod`, so
  `go mod tidy` must remain a no-op. Verify.
- Blackbox tests only (external `rlng_test` package); assert-closure table style (each case has an
  `assert func(...)` closure — never `want`/`wantErr` fields); tests use `t.Context()`.
- `ctx`-first on every constructor; default `Build()` (no `BuildOption`s); errors passed through unwrapped
  (no new sentinel).
- Purely additive: no change to `New`, `NewTypedEngine`, `Evaluate`, `config.Parse`, or `Build`; no
  `Hash()`/eval/config-schema change.
- Every exported symbol carries a godoc comment (library API contract).
- Test-coverage gate: target ≥ 85% on the root `rlng` package; every error-passthrough branch covered.

---

### Task 1: Convenience constructors + tests + ADR

**Files:**
- Create: `fromconfig.go`
- Create: `fromconfig_test.go`
- Create: `fromconfig_example_test.go`
- Create: `docs/adrs/0051-convenience-constructors.md`
- Modify: `docs/BACKLOG.md` (B10 → resolved for constructors; Pipeline-as-Stage re-deferred note)
- Modify: `docs/HANDOVER.md` (increment 026 done; B11 next)
- (plan `docs/plans/026-convenience-constructors.md` already exists — staged in this commit)

**Interfaces:**
- Consumes: `config.Parse(ctx, config.Provider) (*config.PipelineDef, error)`, `(*config.PipelineDef).Build() (*pipe.Pipeline, error)`, `config.FromYAMLString(string) config.Provider`, `New(*pipe.Pipeline, ...Option) (*Engine, error)`, `NewTypedEngine[I,R](*pipe.Pipeline, *Mapper[R], ...Option) (*TypedEngine[I,R], error)`, `ErrNilMapper`.
- Produces: `NewFromProvider`, `NewFromYAML`, `NewTypedFromProvider[I,R]`, `NewTypedFromYAML[I,R]` (all in package `rlng`).

- [ ] **Step 1: Write the failing tests**

Create `fromconfig_test.go`:

```go
package rlng_test

import (
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

The root package is imported as `"github.com/kartaladev/rlng"` and referenced as `rlng.X` (confirmed against
`engine_test.go` / `example_test.go`, both `package rlng_test`). `errors` is NOT imported — the tests use
`assert.ErrorAs`/`assert.ErrorIs`, not `errors.As` directly. The test body:

```go
const validYAML = `
stages:
  - name: grade
    type: single-expr
    expr: input.score * 2
`

func TestNewFromProviderAndYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		build  func(t *testing.T) (*rlng.Engine, error)
		assert func(t *testing.T, eng *rlng.Engine, err error)
	}{
		{
			name: "NewFromYAML builds an engine that evaluates",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), validYAML)
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				require.NotNil(t, eng)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(21)}})
				require.NoError(t, err)
				assert.Equal(t, int64(42), out["grade"])
			},
		},
		{
			name: "NewFromProvider accepts a non-YAML provider (JSON)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				const j = `{"stages":[{"name":"grade","type":"single-expr","expr":"input.score * 2"}]}`
				return rlng.NewFromProvider(t.Context(), config.FromJSONString(j))
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(5)}})
				require.NoError(t, err)
				assert.Equal(t, int64(10), out["grade"])
			},
		},
		{
			name: "parse error passes through unwrapped",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), "this: is: not: valid: yaml: [")
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "build error passes through unwrapped (unknown stage type)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				const bad = `
stages:
  - name: x
    type: no-such-type
    expr: input.score
`
				return rlng.NewFromYAML(t.Context(), bad)
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "engine Option is threaded (provenance visible on the scope)",
			build: func(t *testing.T) (*rlng.Engine, error) {
				return rlng.NewFromYAML(t.Context(), validYAML, rlng.WithScopeOptions(pipe.WithProvenance()))
			},
			assert: func(t *testing.T, eng *rlng.Engine, err error) {
				require.NoError(t, err)
				sc, err := eng.EvaluateScope(t.Context(), map[string]any{"input": map[string]any{"score": int64(3)}})
				require.NoError(t, err)
				assert.True(t, sc.TracksProvenance(), "WithScopeOptions(WithProvenance) must reach the evaluated scope")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eng, err := tc.build(t)
			tc.assert(t, eng, err)
		})
	}
}

func TestNewTypedFromProviderAndYAML(t *testing.T) {
	t.Parallel()

	type result struct {
		Grade int64 `mapstructure:"grade"`
	}
	mapper, err := rlng.NewMapper[result](rlng.MappingTemplate{"grade": "grade"})
	require.NoError(t, err)

	cases := []struct {
		name   string
		build  func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error)
		assert func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error)
	}{
		{
			name: "NewTypedFromYAML maps into the typed result",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromYAML[map[string]any, result](t.Context(), validYAML, mapper)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.NoError(t, err)
				out, err := eng.Evaluate(t.Context(), map[string]any{"input": map[string]any{"score": int64(4)}})
				require.NoError(t, err)
				assert.Equal(t, int64(8), out.Grade)
			},
		},
		{
			name: "NewTypedFromProvider with a nil mapper returns ErrNilMapper",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromProvider[map[string]any, result](t.Context(), config.FromYAMLString(validYAML), nil)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				assert.ErrorIs(t, err, rlng.ErrNilMapper)
			},
		},
		{
			name: "typed parse error passes through unwrapped",
			build: func(t *testing.T) (*rlng.TypedEngine[map[string]any, result], error) {
				return rlng.NewTypedFromYAML[map[string]any, result](t.Context(), "not: valid: [", mapper)
			},
			assert: func(t *testing.T, eng *rlng.TypedEngine[map[string]any, result], err error) {
				require.Error(t, err)
				assert.Nil(t, eng)
				var ce *config.ConfigError
				assert.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eng, err := tc.build(t)
			tc.assert(t, eng, err)
		})
	}
}
```

Note: confirm `config.FromJSONString` exists (it does: `config/providers.go`), that `eng.EvaluateScope`
returns a `*pipe.Scope` with `TracksProvenance()` (it does), and that the `single-expr` reads `input.score`
correctly against the seed shape used (`{"input": {"score": …}}`). If the expression form or seed nesting
needs adjusting to actually compute, fix the ruleset/seed so the happy-path value is genuinely produced —
do not weaken the assertion.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestNewFromProviderAndYAML|TestNewTypedFromProviderAndYAML'`
Expected: FAIL — `undefined: rlng.NewFromYAML` (and the other three constructors).

- [ ] **Step 3: Implement the constructors**

Create `fromconfig.go`:

```go
package rlng

import (
	"context"

	"github.com/kartaladev/rlng/config"
)

// NewFromProvider builds an Engine directly from a config source: it parses the
// provider, compiles the definition with the default build options, and wraps
// the resulting pipeline. It is the one-call form of
// config.Parse -> PipelineDef.Build -> New. opts configure the per-Evaluate
// Scope (e.g. WithScopeOptions(pipe.WithProvenance())). A parse or build failure
// is returned unwrapped (a *config.ConfigError). For build-time options
// (strict schema, lint-as-error, version override) use the explicit
// config.Parse -> Build -> New path.
func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	pipeline, err := def.Build()
	if err != nil {
		return nil, err
	}
	return New(pipeline, opts...)
}

// NewFromYAML builds an Engine from an in-memory YAML ruleset. It is shorthand
// for NewFromProvider(ctx, config.FromYAMLString(yaml), opts...); for JSON, a
// file, or a URL, call NewFromProvider with the matching config.From* provider.
func NewFromYAML(ctx context.Context, yaml string, opts ...Option) (*Engine, error) {
	return NewFromProvider(ctx, config.FromYAMLString(yaml), opts...)
}

// NewTypedFromProvider builds a TypedEngine[I, R] directly from a config source:
// it parses the provider, compiles with the default build options, and wraps the
// pipeline with mapper. It is the one-call form of
// config.Parse -> PipelineDef.Build -> NewTypedEngine. A parse or build failure
// is returned unwrapped; a nil mapper returns ErrNilMapper. Build-time options
// require the explicit config.Parse -> Build -> NewTypedEngine path.
func NewTypedFromProvider[I, R any](ctx context.Context, p config.Provider, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	pipeline, err := def.Build()
	if err != nil {
		return nil, err
	}
	return NewTypedEngine[I, R](pipeline, mapper, opts...)
}

// NewTypedFromYAML builds a TypedEngine[I, R] from an in-memory YAML ruleset. It
// is shorthand for NewTypedFromProvider(ctx, config.FromYAMLString(yaml), mapper,
// opts...); for JSON, a file, or a URL, call NewTypedFromProvider with the
// matching config.From* provider.
func NewTypedFromYAML[I, R any](ctx context.Context, yaml string, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	return NewTypedFromProvider[I, R](ctx, config.FromYAMLString(yaml), mapper, opts...)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run 'TestNewFromProviderAndYAML|TestNewTypedFromProviderAndYAML' -v`
Expected: PASS (all cases). If a happy-path value assertion fails because the sample ruleset/seed doesn't
compute as written, correct the ruleset/seed in the test (not the assertion) until the real computed value
matches, then re-run.

- [ ] **Step 5: Add the runnable example**

Create `fromconfig_example_test.go` (match the import style of the existing `example_test.go`):

```go
package rlng_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
)

// Example_newFromYAML builds an engine from a YAML ruleset in a single call and
// evaluates it.
func Example_newFromYAML() {
	const ruleset = `
stages:
  - name: total
    type: single-expr
    expr: input.qty * input.price
`
	eng, err := rlng.NewFromYAML(context.Background(), ruleset)
	if err != nil {
		panic(err)
	}
	out, err := eng.Evaluate(context.Background(), map[string]any{
		"input": map[string]any{"qty": 3, "price": 4},
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("total = %v\n", out["total"])

	// Output:
	// total = 12
}
```

Run it once and confirm the observed output matches the `// Output:` block exactly (adjust the literal to
the real computed value if `int` vs `int64` printing differs — `%v` on an int64 prints `12`, so this is
stable; verify by running).

- [ ] **Step 6: Author ADR-0051**

Create `docs/adrs/0051-convenience-constructors.md`:

```markdown
# ADR-0051 — convenience constructors: build an engine from a config source

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 026 / Plan 026, graduating backlog B10 (constructors half).

## Context

Building an engine from declarative config was three explicit steps (config.Parse -> PipelineDef.Build ->
rlng.New). ADR-0009 kept `rlng` free of a `config` dependency (the declarative and programmatic paths
converge at *pipe.Pipeline) but explicitly anticipated a `rlng.NewFromYAML` convenience "added additively if
desired (deferred, YAGNI)." B10 is that follow-through. The B10 backlog line also named a second feature —
`Pipeline` implementing `Stage` (nested pipelines) — which ADR-0005 deliberately excluded.

## Decision

- **Four convenience constructors in the root `rlng` package** (`fromconfig.go`): `NewFromProvider`,
  `NewFromYAML`, `NewTypedFromProvider[I,R]`, `NewTypedFromYAML[I,R]`. Each composes
  Parse -> Build -> New/NewTypedEngine, is ctx-first, calls `Build()` with default options, threads engine
  `Option`s, and returns the first error unwrapped. `NewFromProvider` accepts any `config.Provider`, so no
  per-format constructor family is needed.
- **`rlng` now imports `config`** — the additive convenience ADR-0009 anticipated. `config` is same-module,
  its transitive deps are already in `go.mod` (no new external dependency; `go mod tidy` stays a no-op), the
  import is acyclic (`config` does not import `rlng`), and dead-code elimination drops `config` from a
  consumer's binary when the constructors go unused.
- **Build-time options stay on the explicit path.** The constructors use default `Build()`; strict schema /
  lint-as-error / version override use `config.Parse -> Build -> New` directly. No new error type — parse,
  build, and nil-mapper errors are threaded through unchanged.

## Consequences

- The common "engine from YAML/JSON" path is a single call; the three-layer path remains for advanced build
  options. Purely additive public surface (no breaking change; no Hash()/eval/schema change).
- `rlng` gains a `config` package import (in-module, acyclic, no new external dep).
- **Pipeline-as-Stage (the other half of B10) is re-deferred, deliberately.** Reversing ADR-0005 has
  marginal value: `foreach` already owns per-element sub-pipelines, and a flat nested pipeline is ≈ inlining
  its stages (shared scope + DAG). It would add naming, shared-scope bookkeeping, and collision semantics
  for no concrete demand. Kept as a documented backlog note; revisit with a superseding ADR to ADR-0005 if a
  real composition need appears.

## Traceability

Spec: 026. Plan: 026. Extends: ADR-0009 (the anticipated additive `NewFromYAML`). Related: ADR-0019
(TypedEngine naming), ADR-0005 (Pipeline-not-a-Stage, the re-deferred non-goal), increment 016
(config.Provider abstraction consumed here).
```

- [ ] **Step 7: Close out B10 in the docs**

`docs/BACKLOG.md`:
- Change the B10 table row to strikethrough with a partial-resolution note, e.g.:
  `| ~~**B10**~~ | ~~Convenience constructors~~ (constructors ✅; Pipeline-as-Stage re-deferred) | ADR-0009; ADR-0005 | ergonomics | additive | ✅ **Done** (incr 026, ADR-0051) — constructors; Pipeline-as-Stage still deferred |`
  (match the existing row format for B1–B9.)
- Rewrite the B10 **Details** paragraph: constructors shipped (incr 026, ADR-0051 — `NewFromProvider`/
  `NewFromYAML` + typed variants, `rlng→config` additive import); Pipeline-as-Stage **remains deferred** with
  rationale (reverses ADR-0005, marginal value, foreach covers per-element sub-pipelines — needs a
  superseding ADR + a concrete use case).
- Add a "Recently resolved" row: `| rlng.NewFromYAML convenience (B10; ADR-0009 deferral) | Increment 026 / ADR-0051 |`.

`docs/HANDOVER.md`:
- Increments through 026 done; artifact numbering specs/plans **026 done**, ADRs **0051 done**.
- Next action → **B11** (parallel execution of independent DAG stages) — a **design-checkpoint** item AND a
  non-goal reversal: needs a superseding ADR to ADR-0006 first, and a PAUSE for the user's design approval
  before implementing (per the standing design-gating decision). Keep the read-first pointers and
  standing-decisions sections intact.

- [ ] **Step 8: Full verification**

Run:
- `go build ./... && go test ./... -race` — green.
- `go vet ./... && gofmt -l .` (empty).
- `CGO_ENABLED=0 go build ./...` — green.
- `go mod tidy && git diff --quiet go.mod go.sum` — **must be a no-op** (no new dependency). If it changes
  go.mod/go.sum, STOP and report — the spec requires no new external dep.
- `go test . -cover` — root package coverage ≥ 85%; the four constructors and every error-passthrough branch
  covered.

- [ ] **Step 9: Commit**

```bash
git add fromconfig.go fromconfig_test.go fromconfig_example_test.go \
        docs/adrs/0051-convenience-constructors.md docs/plans/026-convenience-constructors.md \
        docs/BACKLOG.md docs/HANDOVER.md
git commit -m "$(cat <<'EOF'
feat(rlng): convenience constructors to build an engine from config (B10)

Add NewFromProvider/NewFromYAML and typed NewTypedFromProvider/
NewTypedFromYAML to the root package: one call composing
config.Parse -> Build -> New (ctx-first, default Build, errors threaded).
Introduces an in-module rlng->config import (no new external dep), the
additive convenience ADR-0009 anticipated. Pipeline-as-Stage (the other
half of B10) is re-deferred with rationale (would reverse ADR-0005).

Plan 026 + ADR-0051 ride here.

Spec: 026
Plan: 026
ADR: 0051
EOF
)"
```

---

## Whole-branch delivery gate (after Task 1)

Per CLAUDE.md §5 and the standing program authorization:

- [ ] `go build ./...`, `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) / `go mod verify` all clean.
- [ ] `/code-review high main..HEAD` — resolve/triage every finding; re-run affected review + `-race`.
- [ ] `/security-review` on the branch diff — resolve/triage findings.
- [ ] Confirm coverage: `go test . -cover` (target ≥ 85%; new branches covered).
- [ ] Auto merge to `main` + push + delete branch (standing authorization; does NOT extend to release tags).

## Self-review notes (spec coverage)

- Spec D1 (four constructors) → Task 1 Step 3. D2 (rlng→config, no new dep) → Step 3 + Step 8 mod-tidy
  check. D3 (ctx-first) → signatures in Step 3. D4 (default Build; escape hatch) → Step 3 doc comments.
  D5 (errors unwrapped, no new sentinel) → Step 3 + tests Step 1 (parse/build/nil-mapper passthrough).
- Success criteria 1-2 → happy-path cases; 3-4 → parse/build passthrough; 5 → typed happy path; 6 → nil
  mapper; 7 → opts threading; 8 → runnable example (Step 5).
- Non-goal (Pipeline-as-Stage) → recorded in ADR-0051 (Step 6) + BACKLOG re-deferral note (Step 7).
