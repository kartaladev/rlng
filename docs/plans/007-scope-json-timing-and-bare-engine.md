# Scope JSON codec + timing, and BareEngine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `stage.Scope` round-trippable JSON (for jsonb persistence) and always-on evaluation timing, add a mapper-less `rlng.BareEngine`, and demonstrate every capability in a dedicated `examples/` package.

**Architecture:** Timing is stamped once in `Pipeline.Run` (the choke point every engine flows through) and stored on the `Scope`. A `Scope` JSON codec marshals an envelope `{data, timing, derivations?}` and restores it losslessly. `BareEngine` reuses the existing `flatten`/`WithScopeOptions` machinery and returns the raw `map[string]any`. Examples are `Example…` tests with deterministic output via an injected clock.

**Tech Stack:** Go 1.25+, `encoding/json`, `time`, existing `github.com/kartaladev/rlng/{stage,config}`; test deps `stretchr/testify`.

**Traceability:** Implements Spec 007 (`docs/specs/007-scope-json-timing-and-bare-engine.md`); introduces ADR-0013 (Scope JSON envelope + always-on timing) and ADR-0014 (`BareEngine`).

## Global Constraints

- Module path `github.com/kartaladev/rlng`; pure Go, `CGO_ENABLED=0`, Go 1.25+.
- Library only — no `os.Exit`/`log.Fatal`/`panic` on caller input; return typed, wrapping errors.
- Every exported symbol has a godoc comment. Prefer a small, stable exported surface.
- Tests: `table-test` skill rules — `assert` closure form, `ctx` modifier where context-sensitive, `t.Context()` over `context.Background()`; fold ≥2 cases exercising the same call into one table.
- Coverage gate: target ≥ 85% on changed packages; **every hot-path logic branch and every typed-error branch has a covering test**.
- Per-commit: each task is a green unit (`go test ./... -race` passes) before its commit. ADR/plan edits ride in the **same commit** as the code that realizes them (specs are the only standalone artifact, already committed).
- Conventional Commits with trailers `Spec: 007`, `Plan: 007`, and `ADR: NNNN` on feat commits.

---

### Task 1: Scope evaluation timing (always-on)

**Files:**
- Modify: `stage/scope.go` (add timing fields; default `clock` in `NewScope`)
- Create: `stage/timing.go` (`WithClock`, `markStarted`/`markFinished`, `StartedAt`/`Duration`)
- Modify: `stage/pipeline.go:155` (`Run` stamps timing)
- Create: `docs/adrs/0013-scope-json-and-timing.md` (timing section; JSON section added in Task 2)
- Test: `stage/timing_test.go`

**Interfaces:**
- Consumes: `Scope` struct, `ScopeOption`, `NewScope`, `Pipeline.Run(ctx, *Scope) error`.
- Produces:
  - `func WithClock(clock func() time.Time) ScopeOption`
  - `func (s *Scope) StartedAt() (time.Time, bool)`
  - `func (s *Scope) Duration() (time.Duration, bool)`
  - unexported `func (s *Scope) markStarted()`, `func (s *Scope) markFinished()`

- [ ] **Step 1: Write the failing test** — `stage/timing_test.go`

```go
package stage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeTiming(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		run    bool // whether to stamp via markStarted/markFinished
		assert func(t *testing.T, s *Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []testCase{
		{
			name:  "unstamped scope reports not-run",
			build: func() *Scope { return NewScope(nil) },
			run:   false,
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.StartedAt()
				assert.False(t, ok)
				_, ok = s.Duration()
				assert.False(t, ok)
			},
		},
		{
			name: "injected clock yields deterministic duration",
			build: func() *Scope {
				times := []time.Time{start, start.Add(5 * time.Millisecond)}
				i := 0
				return NewScope(nil, WithClock(func() time.Time {
					t := times[i]
					if i < len(times)-1 {
						i++
					}
					return t
				}))
			},
			run: true,
			assert: func(t *testing.T, s *Scope) {
				at, ok := s.StartedAt()
				require.True(t, ok)
				assert.Equal(t, start, at)
				d, ok := s.Duration()
				require.True(t, ok)
				assert.Equal(t, 5*time.Millisecond, d)
			},
		},
		{
			name:  "nil clock in WithClock is ignored (keeps default)",
			build: func() *Scope { return NewScope(nil, WithClock(nil)) },
			run:   true,
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Duration()
				assert.True(t, ok, "default time.Now clock must still stamp timing")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.build()
			if tc.run {
				s.markStarted()
				s.markFinished()
			}
			tc.assert(t, s)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./stage/ -run TestScopeTiming`
Expected: FAIL — `WithClock`, `markStarted`, `StartedAt`, `Duration` undefined.

- [ ] **Step 3: Add timing fields to `Scope` and default the clock** — `stage/scope.go`

In the `Scope` struct add:

```go
	startedAt time.Time
	duration  time.Duration
	clock     func() time.Time
```

Add `"time"` to the imports. In `NewScope`, change the construction line to default the clock **before** options apply (so `WithClock` can override):

```go
	s := &Scope{data: data, clock: time.Now}
```

- [ ] **Step 4: Create `stage/timing.go`**

```go
package stage

import "time"

// WithClock sets the time source used to stamp evaluation timing. It defaults to
// time.Now; tests inject a deterministic clock for stable output. A nil clock is
// ignored (the default is kept).
func WithClock(clock func() time.Time) ScopeOption {
	return func(s *Scope) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// markStarted records the start of an evaluation. Called by Pipeline.Run.
func (s *Scope) markStarted() {
	s.mu.Lock()
	s.startedAt = s.clock()
	s.mu.Unlock()
}

// markFinished records the elapsed evaluation time. Called by Pipeline.Run,
// including when a stage errors — a partial run still took time.
func (s *Scope) markFinished() {
	s.mu.Lock()
	s.duration = s.clock().Sub(s.startedAt)
	s.mu.Unlock()
}

// StartedAt reports when the pipeline run began, and false if no run has stamped
// the Scope.
func (s *Scope) StartedAt() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt, !s.startedAt.IsZero()
}

// Duration reports how long the pipeline run took, and false if no run has
// stamped the Scope.
func (s *Scope) Duration() (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.duration, !s.startedAt.IsZero()
}
```

- [ ] **Step 5: Stamp timing in `Pipeline.Run`** — `stage/pipeline.go`

```go
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error {
	sc.markStarted()
	defer sc.markFinished()
	for _, s := range p.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Execute(ctx, sc); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 6: Run tests + verify a run stamps timing**

Run: `go test ./stage/ -run 'TestScopeTiming|TestPipeline' -race`
Expected: PASS.

- [ ] **Step 7: Add a Pipeline-integration timing assertion**

Append to `stage/pipeline_test.go` (fold into its table if one exists; otherwise a focused test):

```go
func TestPipelineRunStampsTiming(t *testing.T) {
	t.Parallel()

	s1, err := NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := NewPipeline(s1)
	require.NoError(t, err)

	sc := NewScope(map[string]any{"price": 10, "qty": 2})
	require.NoError(t, p.Run(t.Context(), sc))

	_, ok := sc.StartedAt()
	assert.True(t, ok)
	d, ok := sc.Duration()
	require.True(t, ok)
	assert.GreaterOrEqual(t, d, time.Duration(0))
}
```

Run: `go test ./stage/ -run TestPipelineRunStampsTiming -race` → PASS. (Add `"time"` to the test file imports if missing.)

- [ ] **Step 8: Write ADR-0013 (timing section)** — `docs/adrs/0013-scope-json-and-timing.md`

```markdown
# ADR-0013 — Scope JSON envelope and always-on evaluation timing

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 007 (docs/specs/007-scope-json-timing-and-bare-engine.md)

## Context

A Scope is the carrier of a computation's result. Consumers must move it across
process boundaries (web responses, jsonb columns) and want to know when a
calculation ran and how long it took. The Scope had neither serialization nor
timing.

## Decision

1. **Always-on timing.** Pipeline.Run stamps `startedAt`/`duration` on the Scope
   (start before stages, elapsed after — via `defer`, so it is set even when a
   stage errors). Cost is two clock reads per run — too cheap to gate, so timing
   is not opt-in (unlike provenance). `WithClock` injects a deterministic clock
   for tests. Accessors `StartedAt()`/`Duration()` return `(_, false)` until a
   run stamps the Scope.
2. **JSON envelope** — see the JSON section added alongside the codec.

## Consequences

- Every run pays two clock reads; negligible against µs-scale evaluation.
- Timing is stamped in exactly one place (Pipeline.Run), so every engine
  (`Engine`, `BareEngine`, direct `Run`) gets it for free.
```

- [ ] **Step 9: Verify + commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./stage/ -race`
Expected: clean, PASS.

```bash
git add stage/scope.go stage/timing.go stage/timing_test.go stage/pipeline.go stage/pipeline_test.go docs/adrs/0013-scope-json-and-timing.md docs/plans/007-scope-json-timing-and-bare-engine.md
git commit -m "feat(stage): always-on evaluation timing on Scope

Pipeline.Run stamps startedAt/duration on the Scope (set even on stage
error); StartedAt()/Duration() accessors; WithClock injects a deterministic
clock for tests.

Spec: 007
Plan: 007
ADR: 0013"
```

---

### Task 2: Derivation JSON tags + Scope JSON codec

> **AMENDED (value preservation, per user):** JSON serde must preserve all
> values exactly to avoid money-calculation disputes. `UnmarshalJSON` decodes
> `data`/`derivations` with a `json.Decoder` + `UseNumber()` so numbers restore
> as `json.Number` (exact digits — no float64 rounding of integers above 2^53).
> The numeric getters `GetInt`/`GetInt64`/`GetFloat64` (in `stage/get.go`) are
> extended to read `json.Number` losslessly. See the updated spec §"Value
> preservation" and ADR-0013.

**Files:**
- Modify: `stage/provenance.go` (add `json:` tags to `Derivation`)
- Create: `stage/json.go` (`MarshalJSON`/`UnmarshalJSON` + envelope types; `UseNumber` decode)
- Modify: `stage/get.go` (`GetInt`/`GetInt64`/`GetFloat64` accept `json.Number`)
- Modify: `docs/adrs/0013-scope-json-and-timing.md` (JSON section + value-preservation)
- Test: `stage/json_test.go`, `stage/get_test.go` (json.Number cases)

**Interfaces:**
- Consumes: `Scope` (data/startedAt/duration/provenance/derivations fields), `Derivation`.
- Produces:
  - `func (s *Scope) MarshalJSON() ([]byte, error)`
  - `func (s *Scope) UnmarshalJSON(b []byte) error`

- [ ] **Step 1: Write the failing test** — `stage/json_test.go`

```go
package stage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeJSONRoundTrip(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		assert func(t *testing.T, blob []byte, reloaded *Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return start }

	cases := []testCase{
		{
			name: "data only, no run, no provenance",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": "x"})
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.NotContains(t, string(blob), "timing")
				assert.NotContains(t, string(blob), "derivations")
				v, ok := reloaded.Get("a")
				require.True(t, ok)
				assert.Equal(t, "x", v)
			},
		},
		{
			name: "timing present after a run",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": 1}, WithClock(fixedClock))
				sc.markStarted()
				sc.markFinished()
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.Contains(t, string(blob), "\"timing\"")
				at, ok := reloaded.StartedAt()
				require.True(t, ok)
				assert.Equal(t, start.UTC(), at.UTC())
				d, ok := reloaded.Duration()
				require.True(t, ok)
				assert.Equal(t, time.Duration(0), d)
			},
		},
		{
			name: "provenance derivations round-trip and restore inspection",
			build: func() *Scope {
				sc := NewScope(map[string]any{"price": 10}, WithProvenance())
				require.NoError(t, sc.Derive("base", 20, Derivation{
					Stage: "base", StageType: TypeSingleExpr, Operation: "eval",
					Expression: "price * 2", Inputs: map[string]any{"price": 10},
				}))
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.Contains(t, string(blob), "\"derivations\"")
				assert.True(t, reloaded.TracksProvenance())
				d, ok := reloaded.Derivation("base")
				require.True(t, ok)
				assert.Equal(t, "price * 2", d.Expression)
				assert.NotEmpty(t, reloaded.Explain("base"))
			},
		},
		{
			name: "byte-stable round-trip (marshal->unmarshal->marshal)",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": 1.5, "b": "y"}, WithClock(fixedClock))
				sc.markStarted()
				sc.markFinished()
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				again, err := json.Marshal(reloaded)
				require.NoError(t, err)
				assert.JSONEq(t, string(blob), string(again))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := tc.build()
			blob, err := json.Marshal(sc)
			require.NoError(t, err)

			var reloaded Scope
			require.NoError(t, json.Unmarshal(blob, &reloaded))
			tc.assert(t, blob, &reloaded)
		})
	}
}

func TestScopeUnmarshalErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		input  string
		assert func(t *testing.T, s *Scope, err error)
	}

	cases := []testCase{
		{
			name:  "malformed json is an error",
			input: `{bad`,
			assert: func(t *testing.T, s *Scope, err error) {
				require.Error(t, err)
			},
		},
		{
			name:  "absent data yields empty (not nil) map",
			input: `{"timing":{"started_at":"2026-01-01T00:00:00Z","duration_ns":0}}`,
			assert: func(t *testing.T, s *Scope, err error) {
				require.NoError(t, err)
				assert.NotNil(t, s.Snapshot())
				assert.Empty(t, s.Snapshot())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var s Scope
			err := json.Unmarshal([]byte(tc.input), &s)
			tc.assert(t, &s, err)
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./stage/ -run 'TestScopeJSON|TestScopeUnmarshal'`
Expected: FAIL — `Scope` has no `MarshalJSON`/`UnmarshalJSON` (default struct marshal, or errors).

- [ ] **Step 3: Add `json:` tags to `Derivation`** — `stage/provenance.go`

```go
type Derivation struct {
	Path       string         `json:"path"`
	Stage      string         `json:"stage,omitempty"`
	StageType  string         `json:"stage_type"`
	Operation  string         `json:"operation"`
	Expression string         `json:"expression,omitempty"`
	Inputs     map[string]any `json:"inputs,omitempty"`
	Value      any            `json:"value"`
}
```

- [ ] **Step 4: Create `stage/json.go`**

```go
package stage

import (
	"encoding/json"
	"time"
)

// scopeJSON is the on-wire envelope for a Scope: the accumulated result data,
// evaluation timing, and (when provenance is enabled) the derivations.
type scopeJSON struct {
	Data        map[string]any        `json:"data"`
	Timing      *scopeTimingJSON      `json:"timing,omitempty"`
	Derivations map[string]Derivation `json:"derivations,omitempty"`
}

type scopeTimingJSON struct {
	StartedAt  time.Time `json:"started_at"`
	DurationNS int64     `json:"duration_ns"`
}

// MarshalJSON serializes the Scope as a round-trippable envelope
// {data, timing?, derivations?} suitable for a jsonb column. `timing` appears
// after a run; `derivations` only when provenance is enabled. For just the
// result data (e.g. a web response) marshal Snapshot() instead.
func (s *Scope) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	env := scopeJSON{Data: s.data}
	if !s.startedAt.IsZero() {
		env.Timing = &scopeTimingJSON{StartedAt: s.startedAt, DurationNS: s.duration.Nanoseconds()}
	}
	if s.provenance {
		env.Derivations = s.derivations
	}
	return json.Marshal(env)
}

// UnmarshalJSON restores a Scope from the envelope produced by MarshalJSON:
// result data, timing, and — when derivations are present — provenance state so
// Derivation/Lineage/Explain work on the restored Scope. A restored Scope is
// for inspection, not re-execution. Numbers in `data` decode as float64 per
// encoding/json.
func (s *Scope) UnmarshalJSON(b []byte) error {
	var env scopeJSON
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = env.Data
	if s.data == nil {
		s.data = map[string]any{}
	}
	if s.clock == nil {
		s.clock = time.Now
	}
	if env.Timing != nil {
		s.startedAt = env.Timing.StartedAt
		s.duration = time.Duration(env.Timing.DurationNS)
	}
	if env.Derivations != nil {
		s.provenance = true
		s.derivations = env.Derivations
	}
	return nil
}
```

- [ ] **Step 5: Run tests + race**

Run: `go test ./stage/ -run 'TestScopeJSON|TestScopeUnmarshal' -race`
Expected: PASS.

- [ ] **Step 6: Append the JSON section to ADR-0013**

Replace the placeholder line `2. **JSON envelope** — see the JSON section added alongside the codec.` with:

```markdown
2. **JSON envelope, round-trippable.** `Scope.MarshalJSON` emits
   `{data, timing?, derivations?}`; `UnmarshalJSON` restores all three and,
   when derivations are present, marks the Scope provenance-enabled for
   inspection. `data` is always present; `timing`/`derivations` are conditional.
   Raw result data for a web response is `json.Marshal(Snapshot())` (just the
   map) — the envelope is deliberately the persistence form. Numbers in `data`
   reload as float64 (encoding/json), which the strict typed getters reflect.
   `Derivation` carries snake_case json tags for a stable schema.
```

And add to the Consequences list:

```markdown
- The jsonb blob carries the audit trail only when provenance was enabled; the
  off path serializes just data (+ timing).
```

- [ ] **Step 7: Verify + commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./stage/ -race -cover`
Expected: clean, PASS, stage coverage ≥ 85%.

```bash
git add stage/provenance.go stage/json.go stage/json_test.go docs/adrs/0013-scope-json-and-timing.md
git commit -m "feat(stage): round-trippable Scope JSON codec for jsonb persistence

MarshalJSON/UnmarshalJSON serialize an envelope {data, timing?, derivations?}
and restore it losslessly (restored provenance answers Derivation/Explain).
Derivation gains snake_case json tags. Snapshot() stays the raw-data path.

Spec: 007
Plan: 007
ADR: 0013"
```

---

### Task 3: `rlng.BareEngine`

**Files:**
- Create: `bare_engine.go`
- Create: `docs/adrs/0014-bare-engine.md`
- Test: `bare_engine_test.go`

**Interfaces:**
- Consumes: `engineConfig`, `Option`, `WithScopeOptions`, `flatten[I any](I) (map[string]any, error)` (all in `engine.go`); `stage.Pipeline`, `stage.NewScope`, `stage.Scope`.
- Produces:
  - `func NewBareEngine(pipeline *stage.Pipeline, opts ...Option) *BareEngine`
  - `func (e *BareEngine) Evaluate(ctx context.Context, input any) (map[string]any, error)`
  - `func (e *BareEngine) EvaluateScope(ctx context.Context, input any) (*stage.Scope, error)`

- [ ] **Step 1: Write the failing test** — `bare_engine_test.go`

```go
package rlng

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bareInput struct {
	Price int `mapstructure:"price"`
	Qty   int `mapstructure:"qty"`
}

func buildBareEngine(tb testing.TB, opts ...Option) *BareEngine {
	tb.Helper()
	base, err := stage.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := stage.NewPipeline(base, taxed)
	require.NoError(tb, err)
	return NewBareEngine(p, opts...)
}

func TestBareEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *BareEngine
		input  any
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, out map[string]any, err error)
	}

	cases := []testCase{
		{
			name:   "struct input returns accumulated map",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  bareInput{Price: 10, Qty: 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 20, out["base"])
				assert.InDelta(t, 22.0, out["taxed"], 1e-9)
			},
		},
		{
			name:   "map input passes through",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 3},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, out["base"])
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *BareEngine {
				boom, err := stage.NewSingleExpr("x", "qty % 0")
				require.NoError(tb, err)
				p, err := stage.NewPipeline(boom)
				require.NoError(tb, err)
				return NewBareEngine(p)
			},
			input: map[string]any{"qty": 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: func(tb testing.TB) *BareEngine { return buildBareEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 2},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := tc.engine(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			out, err := e.Evaluate(ctx, tc.input)
			tc.assert(t, out, err)
		})
	}
}

func TestBareEngineEvaluateScope(t *testing.T) {
	t.Parallel()

	e := buildBareEngine(t)
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)

	_, ok := sc.Duration()
	assert.True(t, ok, "EvaluateScope exposes timing")
	v, err := sc.GetFloat64("taxed")
	require.NoError(t, err)
	assert.InDelta(t, 22.0, v, 1e-9)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test . -run TestBareEngine`
Expected: FAIL — `BareEngine`/`NewBareEngine` undefined.

- [ ] **Step 3: Create `bare_engine.go`**

```go
package rlng

import (
	"context"

	"github.com/kartaladev/rlng/stage"
)

// BareEngine runs a compiled pipeline against arbitrary input and returns the
// accumulated map[string]any — no result mapping (cf. Engine[I, R]). It is safe
// for concurrent use after construction: each call builds a fresh Scope.
type BareEngine struct {
	pipeline  *stage.Pipeline
	scopeOpts []stage.ScopeOption
}

// NewBareEngine constructs a BareEngine from a compiled pipeline. Options
// configure the per-Evaluate Scope (e.g. WithScopeOptions(stage.WithProvenance())).
func NewBareEngine(pipeline *stage.Pipeline, opts ...Option) *BareEngine {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return &BareEngine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}
}

// EvaluateScope seeds a Scope from input, runs the pipeline, and returns the
// full Scope — exposing timing, JSON serialization, and provenance. A
// map[string]any input seeds directly; any other value is flattened via
// mapstructure. Pipeline/stage errors pass through unwrapped.
func (e *BareEngine) EvaluateScope(ctx context.Context, input any) (*stage.Scope, error) {
	seed, err := flatten(input)
	if err != nil {
		return nil, err
	}
	sc := stage.NewScope(seed, e.scopeOpts...)
	if err := e.pipeline.Run(ctx, sc); err != nil {
		return nil, err
	}
	return sc, nil
}

// Evaluate runs the pipeline and returns the accumulated map[string]any (the
// Scope snapshot).
func (e *BareEngine) Evaluate(ctx context.Context, input any) (map[string]any, error) {
	sc, err := e.EvaluateScope(ctx, input)
	if err != nil {
		return nil, err
	}
	return sc.Snapshot(), nil
}
```

- [ ] **Step 4: Run tests + race**

Run: `go test . -run TestBareEngine -race`
Expected: PASS.

- [ ] **Step 5: Write ADR-0014** — `docs/adrs/0014-bare-engine.md`

```markdown
# ADR-0014 — Mapper-less BareEngine

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 007 (docs/specs/007-scope-json-timing-and-bare-engine.md)

## Context

Engine[I, R] projects the final Scope into a typed R via a Mapper. Some
consumers want the raw accumulated values and no mapping — e.g. to serialize
the Scope directly, or to work dynamically with map[string]any.

## Decision

Add BareEngine: constructed from a compiled Pipeline, `Evaluate(ctx, input any)
(map[string]any, error)` returns the Scope snapshot; `EvaluateScope` returns the
full *stage.Scope (timing, JSON, provenance). It reuses the existing `flatten`
(map passthrough / struct via mapstructure) and `WithScopeOptions`. `input` is
`any` — a BareEngine is not parameterized on input or result type.

## Consequences

- Two engines share the seeding/scope machinery; only the tail differs (map vs
  Mapper.Map). No new dependency.
- `Evaluate` returning `map[string]any` is the documented shape; `EvaluateScope`
  is the escape hatch for the richer Scope capabilities.
```

- [ ] **Step 6: Verify + commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test . -race -cover`
Expected: clean, PASS, root coverage ≥ 85%.

```bash
git add bare_engine.go bare_engine_test.go docs/adrs/0014-bare-engine.md
git commit -m "feat(rlng): mapper-less BareEngine returning map[string]any

Evaluate(ctx, any) returns the accumulated Scope snapshot; EvaluateScope
returns the full Scope (timing/JSON/provenance). Reuses flatten and
WithScopeOptions.

Spec: 007
Plan: 007
ADR: 0014"
```

---

### Task 4: `examples/` package — runnable Example tests for every capability

**Files:**
- Create: `examples/doc.go`
- Create: `examples/pricing_test.go`
- Create: `examples/eligibility_test.go`
- Create: `examples/config_test.go`

**Interfaces:**
- Consumes: `stage.{NewSingleExpr,NewMultiExpr,NewDecisionTable,NewPipeline,NewScope,WithClock,WithProvenance,WithDependsOn,WithHitPolicy,HitPolicyCollect,NamedExpr,Rule}`, `config.{ParseJSON}` + `(*PipelineDef).Build`, `rlng.NewBareEngine`.
- Produces: package `examples` (test-only; example functions).

> The `// Output:` blocks below are computed from the expressions; run
> `go test ./examples/` to confirm and adjust any float formatting if the
> toolchain differs. Each file is `package examples_test`.

- [ ] **Step 1: Create `examples/doc.go`**

```go
// Package examples holds runnable, real-world usage examples for rlng. Each
// Example function is exercised by `go test ./examples/` and doubles as godoc.
// It is a test/documentation package — it exports nothing.
package examples
```

- [ ] **Step 2: Create `examples/pricing_test.go` (single+multi-expr, typed getters, timing, Scope JSON round-trip)**

```go
package examples_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kartaladev/rlng/stage"
)

// deterministicClock returns a clock that yields `start`, then `start+step` on
// every subsequent call — so evaluation duration is stable in example output.
func deterministicClock(start time.Time, step time.Duration) func() time.Time {
	times := []time.Time{start, start.Add(step)}
	i := 0
	return func() time.Time {
		t := times[i]
		if i < len(times)-1 {
			i++
		}
		return t
	}
}

// Example_pricing computes an order total through a single-expr stage feeding a
// multi-expr stage (two named results), reads results with typed getters, shows
// evaluation timing, and round-trips the Scope through JSON (as a jsonb column).
func Example_pricing() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	calc, _ := stage.NewMultiExpr("calc", []stage.NamedExpr{
		{Name: "taxed", Expression: "base * 1.1", Priority: 0},
		{Name: "discounted", Expression: "base * 0.9", Priority: 1},
	}, stage.WithDependsOn("base"))
	p, _ := stage.NewPipeline(base, calc)

	clock := deterministicClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 5*time.Millisecond)
	sc := stage.NewScope(map[string]any{"price": 10, "qty": 2}, stage.WithClock(clock))
	_ = p.Run(context.Background(), sc)

	taxed, _ := sc.GetFloat64("calc.taxed")
	discounted, _ := sc.GetFloat64("calc.discounted")
	dur, _ := sc.Duration()
	fmt.Printf("taxed=%.1f discounted=%.1f took=%s\n", taxed, discounted, dur)

	blob, _ := json.Marshal(sc) // persist to jsonb
	var reloaded stage.Scope
	_ = json.Unmarshal(blob, &reloaded) // read back
	back, _ := reloaded.GetFloat64("calc.taxed")
	fmt.Printf("reloaded taxed=%.1f\n", back)

	// Output:
	// taxed=22.0 discounted=18.0 took=5ms
	// reloaded taxed=22.0
}
```

- [ ] **Step 3: Create `examples/eligibility_test.go` (decision-table single + collect, provenance Explain)**

```go
package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

// Example_eligibility grades a loan application with a decision table (first
// match wins) and explains how the grade was derived.
func Example_eligibility() {
	tier, _ := stage.NewDecisionTable("grade", []stage.Rule{
		{Condition: "score >= 750", Decisions: map[string]string{"tier": `"prime"`}},
		{Condition: "score >= 650", Decisions: map[string]string{"tier": `"near_prime"`}},
		{Condition: "true", Decisions: map[string]string{"tier": `"subprime"`}},
	})
	p, _ := stage.NewPipeline(tier)

	sc := stage.NewScope(map[string]any{"score": 700}, stage.WithProvenance())
	_ = p.Run(context.Background(), sc)

	grade, _ := sc.GetString("grade.tier")
	fmt.Println("tier:", grade)
	fmt.Print(sc.Explain("grade.tier"))

	// Output:
	// tier: near_prime
	// grade.tier = near_prime [grade decision-table] expr: "near_prime"
	//   score = 700 [seed]
}

// Example_eligibility_flags shows a collect-mode decision table: every matching
// rule contributes to a slice of risk flags.
func Example_eligibility_flags() {
	checks, _ := stage.NewDecisionTable("checks", []stage.Rule{
		{Condition: "score < 650", Decisions: map[string]string{"flag": `"low_score"`}},
		{Condition: "dti > 0.4", Decisions: map[string]string{"flag": `"high_dti"`}},
	}, stage.WithHitPolicy(stage.HitPolicyCollect))
	p, _ := stage.NewPipeline(checks)

	sc := stage.NewScope(map[string]any{"score": 600, "dti": 0.5})
	_ = p.Run(context.Background(), sc)

	flags, _ := sc.GetSlice("checks.flag")
	fmt.Println("flags:", flags)

	// Output:
	// flags: [low_score high_dti]
}
```

- [ ] **Step 4: Create `examples/config_test.go` (config JSON → BareEngine)**

```go
package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
)

// Example_configBareEngine loads a pipeline from a JSON definition and runs it
// through a BareEngine, which returns the raw accumulated map[string]any.
func Example_configBareEngine() {
	def, err := config.ParseJSON([]byte(`{
		"stages": [
			{"name": "base", "type": "single-expr", "expr": "price * qty"},
			{"name": "taxed", "type": "single-expr", "expr": "base * 1.1", "depends_on": ["base"]}
		]
	}`))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	engine := rlng.NewBareEngine(pipeline)
	out, err := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"])

	// Output:
	// base=20 taxed=22.0
}
```

- [ ] **Step 5: Run the examples**

Run: `go test ./examples/ -race -v`
Expected: PASS for `Example_pricing`, `Example_eligibility`, `Example_eligibility_flags`, `Example_configBareEngine`. If any `// Output:` mismatches (e.g. float formatting), correct the expected block to the actual (verified) output — do not change the assertions to hide a real logic error.

- [ ] **Step 6: Commit**

```bash
git add examples/
git commit -m "docs(examples): runnable Example tests covering every capability

pricing (single+multi-expr, typed getters, timing, Scope JSON round-trip),
eligibility (decision-table + provenance Explain), and config->BareEngine.

Spec: 007
Plan: 007"
```

---

### Task 5: README Examples section + whole-branch delivery gate

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a concise Examples section to `README.md`**

Insert after the existing quick-start/usage section (keep it brief — the code is the documentation):

```markdown
## Examples

Runnable, real-world usage lives in [`examples/`](./examples) — run them with:

```bash
go test ./examples/
```

- **Pricing** — a two-stage expression pipeline, typed getters, evaluation
  timing, and round-tripping the `Scope` through JSON (as a `jsonb` column).
- **Eligibility** — a decision table with provenance: `Explain` renders how a
  result was derived back to the seed inputs.
- **Config → BareEngine** — load a pipeline from a JSON/YAML definition and run
  it through `BareEngine`, which returns the raw `map[string]any`.
```

- [ ] **Step 2: Commit the README**

```bash
git add README.md
git commit -m "docs: point README at the runnable examples package

Spec: 007
Plan: 007"
```

- [ ] **Step 3: Whole-branch delivery gate (per CLAUDE.md §5)**

Run, in order, and resolve/triage every finding before delivery:

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./...
go test ./... -race -cover        # confirm ≥85% on changed pkgs; every hot-path/typed-error branch covered
govulncheck ./...
```

Then `/code-review main..HEAD` and `/security-review` over the whole branch; fix or triage findings; re-run `-race`. Confirm every exported symbol has godoc and `go mod tidy` leaves go.mod/go.sum unchanged. **Do not merge/push without explicit user approval.**

---

## Self-Review

**Spec coverage:**
- Timing (always-on, WithClock, StartedAt/Duration, Pipeline.Run) → Task 1. ✓
- Scope JSON envelope round-trip + Derivation tags → Task 2. ✓
- BareEngine (Evaluate + EvaluateScope) → Task 3. ✓
- examples covering all features → Task 4. ✓
- README + ADR-0013 + ADR-0014 → Tasks 1/2 (ADR-0013), 3 (ADR-0014), 5 (README). ✓
- Web-vs-persistence split (Snapshot vs MarshalJSON) → documented in Task 2 godoc + ADR. ✓
- float64-on-reload nuance → Task 2 godoc/ADR + Task 2 test asserts JSON-layer equality. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; example `// Output:` blocks are concrete (with an explicit verify-and-correct step). ✓

**Type consistency:** `WithClock`, `markStarted`/`markFinished`, `StartedAt`/`Duration`, `scopeJSON`/`scopeTimingJSON`, `MarshalJSON`/`UnmarshalJSON`, `NewBareEngine`/`Evaluate`/`EvaluateScope`, `flatten` (existing) — names/signatures consistent across tasks and match the spec. ✓
