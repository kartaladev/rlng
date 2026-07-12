# Ruleset identity & decision stamping — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the engine a deterministic ruleset fingerprint, an optional author version, a decision stamp on the Scope, and a fully replayable Scope JSON record (inputs + firing + provenance + ruleset), so a persisted decision is self-describing and a replay against the wrong ruleset is detectable.

**Architecture:** Add a `pipe.RulesetIdentity{Hash, Version}` value carried on `pipe.Pipeline` (set via a chainable `Pipeline.WithRuleset`) and stamped onto the `Scope` in `Pipeline.Run`, exposed by `Scope.Ruleset()`. Add `(*config.PipelineDef).Hash()` (SHA-256 over the canonical JSON of the parsed definition, with the `version` label excluded) plus a `version` field/`WithRulesetVersion` BuildOption wired by `Build`, and a `MatchesRuleset` replay helper. Extend the Scope JSON codec to round-trip the ruleset stamp and the firing rules. Changes are confined to `pipe/ruleset.go` (new), `pipe/scope.go`, `pipe/pipeline.go`, `pipe/firing.go`, `pipe/json.go`, `config/def.go`, `config/hash.go` (new), `config/build.go`, `config/build_options.go`.

**Tech Stack:** Go 1.25, `github.com/expr-lang/expr`, `crypto/sha256` + `encoding/json` (stdlib), `stretchr/testify`.

## Global Constraints

- Go 1.25+; pure Go, no cgo. Library must not panic/os.Exit/log.Fatal on caller input; return typed errors; no global logger.
- Add **no new dependencies** — `crypto/sha256`, `encoding/hex`, `encoding/json` are stdlib.
- Blackbox tests only: every `_test.go` uses `package <pkg>_test` and drives the exported API. Mandatory `table-test` assert-closure form (`assert func(t, ...)` closures, NOT want/wantErr) for ≥2 same-SUT cases. `t.Context()` over `context.Background()`.
- Every exported symbol has a godoc comment. Target ≥85% coverage on changed packages; every hot-path and typed-error branch has a covering test. The hot path here is `Pipeline.Run` stamping, `Scope.Ruleset()`, `PipelineDef.Hash()`, and the Scope `MarshalJSON`/`UnmarshalJSON` round-trip.
- Additive & backward-compatible: `NewPipeline` keeps its variadic-over-`Stage` signature; `Build()` stays callable with no options; existing decisions without a stamp remain valid. No breaking change → no `!` commit; a normal `feat`.
- Traceability: commits carry `Spec: 013`, `Plan: 013`, and the `ADR: 0037` trailer. End every commit message with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Implements Spec 013; see `docs/specs/013-ruleset-identity.md` (resolved decisions D1–D7).

---

### Task 1: Ruleset identity core in `pipe`

Add the `RulesetIdentity` value, carry it on the `Pipeline`, stamp it onto the `Scope` in `Run`, and expose it via `Scope.Ruleset()`.

**Files:**
- Create: `pipe/ruleset.go`
- Modify: `pipe/scope.go` (add one field to the `Scope` struct, ~line 34), `pipe/pipeline.go` (`Pipeline` struct ~line 41-43; `Run` ~line 155-167)
- Test: `pipe/ruleset_test.go`

**Interfaces:**
- Produces:
  - `type RulesetIdentity struct { Hash string \`json:"hash,omitempty"\`; Version string \`json:"version,omitempty"\` }` — a comparable value naming which ruleset produced a decision.
  - `func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline` — sets the pipeline's identity and returns the pipeline (chainable; configure once before Run, not safe to call concurrently with Run).
  - `func (s *Scope) Ruleset() (RulesetIdentity, bool)` — the stamp recorded during Run; `false` if the Scope was never stamped.
  - `func (s *Scope) stampRuleset(id RulesetIdentity)` — unexported; records `id` on the Scope (no-op for the zero identity), called by `Run`.

- [ ] **Step 1: Write the failing tests**

Create `pipe/ruleset_test.go` (blackbox `package pipe_test`):

```go
package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineWithRulesetStampsScope(t *testing.T) {
	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(t, err)
	p, err := pipe.NewPipeline(base)
	require.NoError(t, err)

	id := pipe.RulesetIdentity{Hash: "abc123", Version: "v1.2.0"}
	require.Same(t, p, p.WithRuleset(id), "WithRuleset returns the same pipeline for chaining")

	sc := pipe.NewScope(map[string]any{"price": 10, "qty": 2})
	require.NoError(t, p.Run(t.Context(), sc))

	got, ok := sc.Ruleset()
	require.True(t, ok)
	assert.Equal(t, id, got)
}

func TestScopeRulesetUnstamped(t *testing.T) {
	base, err := pipe.NewSingleExpr("base", "1 + 1")
	require.NoError(t, err)
	p, err := pipe.NewPipeline(base) // no WithRuleset
	require.NoError(t, err)

	sc := pipe.NewScope(nil)
	require.NoError(t, p.Run(t.Context(), sc))

	_, ok := sc.Ruleset()
	assert.False(t, ok, "an un-stamped Scope reports no ruleset identity")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestPipelineWithRuleset|TestScopeRulesetUnstamped' -v`
Expected: compile failure — `RulesetIdentity`, `WithRuleset`, `Ruleset` undefined.

- [ ] **Step 3: Add the `ruleset` field to `Scope`**

In `pipe/scope.go`, in the `Scope` struct (after the `firing` field, ~line 34), add:

```go
	ruleset     RulesetIdentity         // which ruleset produced this decision (stamped by Pipeline.Run)
```

- [ ] **Step 4: Add the `ruleset` field to `Pipeline`**

In `pipe/pipeline.go`, change the `Pipeline` struct (~line 41-43) to:

```go
type Pipeline struct {
	ordered []Stage
	ruleset RulesetIdentity
}
```

- [ ] **Step 5: Create `pipe/ruleset.go`**

```go
package pipe

// RulesetIdentity names which ruleset produced a decision. Hash is a
// deterministic content fingerprint (the config path fills it from
// (*config.PipelineDef).Hash()); Version is an optional author-declared release
// label. The two are orthogonal: Hash proves what ran, Version names which
// release it was. The zero value means "no identity".
type RulesetIdentity struct {
	Hash    string `json:"hash,omitempty"`
	Version string `json:"version,omitempty"`
}

// WithRuleset records which ruleset this Pipeline evaluates, so Run can stamp
// the identity onto each Scope. It returns the Pipeline for chaining. Configure
// it once, before Run — it is not safe to call concurrently with Run.
func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline {
	p.ruleset = id
	return p
}

// stampRuleset records id on the Scope. The zero identity is a no-op, so an
// un-configured Pipeline leaves Ruleset reporting absent.
func (s *Scope) stampRuleset(id RulesetIdentity) {
	if id == (RulesetIdentity{}) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ruleset = id
}

// Ruleset returns the ruleset identity stamped onto this Scope during Run, and
// false if the Scope was never stamped (the pipeline carried no identity, or it
// has not run). A stamped Scope is self-describing: its inputs, firing rules,
// provenance, and the ruleset that produced them travel together.
func (s *Scope) Ruleset() (RulesetIdentity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ruleset, s.ruleset != RulesetIdentity{}
}
```

- [ ] **Step 6: Stamp the Scope in `Run`**

In `pipe/pipeline.go`, in `Run` (~line 155), add the stamp immediately after `sc.markStarted()`:

```go
func (p *Pipeline) Run(ctx context.Context, sc *Scope) error {
	sc.markStarted()
	defer sc.markFinished()
	sc.stampRuleset(p.ruleset)
	for _, st := range p.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sc.timeStage(st.Name(), func() error { return st.Execute(ctx, sc) }); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 7: Run to verify pass**

Run: `go test ./pipe/ -run 'TestPipelineWithRuleset|TestScopeRulesetUnstamped' -v && go test ./pipe/ -race`
Expected: PASS. Existing pipe tests still green (identity is additive; the zero identity never stamps).

- [ ] **Step 8: Commit**

```bash
git add pipe/ruleset.go pipe/scope.go pipe/pipeline.go pipe/ruleset_test.go docs/plans/013-ruleset-identity.md docs/specs/013-ruleset-identity.md
git commit -m "$(cat <<'EOF'
feat(pipe): ruleset identity stamped onto the Scope

Add RulesetIdentity{Hash, Version}, carried on the Pipeline via a chainable
WithRuleset and stamped onto the Scope in Run, exposed by Scope.Ruleset(). The
zero identity never stamps, so an un-configured pipeline is unchanged.

Spec: 013
Plan: 013
ADR: 0037
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

(The spec update D1–D7 and this plan file ride with Task 1's commit, per CLAUDE.md plan+code atomicity; the spec was already reviewed but is committed here alongside its first realizing code.)

---

### Task 2: Config fingerprint, author version & replay helper

Add `(*PipelineDef).Hash()` (canonical JSON + SHA-256, version excluded), an optional `Version` field and `WithRulesetVersion` BuildOption, wire identity into `Build`, and add the `MatchesRuleset` replay helper.

**Files:**
- Modify: `config/def.go` (add `Version` field to `PipelineDef`, ~line 15-25), `config/build.go` (`Build` ~line 54-58), `config/build_options.go` (`buildConfig` ~line 9-13; add `WithRulesetVersion`)
- Create: `config/hash.go`
- Test: `config/hash_test.go`; extend `config/build_test.go` for version wiring

**Interfaces:**
- Consumes: `pipe.RulesetIdentity`, `(*pipe.Pipeline).WithRuleset` (Task 1).
- Produces:
  - `func (d *PipelineDef) Hash() string` — hex SHA-256 of the canonical JSON of the parsed definition, with `Version` excluded.
  - `func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool` — `d.Hash() == id.Hash`.
  - `func WithRulesetVersion(v string) BuildOption` — sets the author version on the built pipeline's identity (overrides `PipelineDef.Version`).
  - `PipelineDef.Version string` — optional author-declared release label parsed from `version:`.
  - `Build` now attaches `RulesetIdentity{Hash: d.Hash(), Version: <resolved>}` to the returned pipeline.

- [ ] **Step 1: Write the failing tests**

Create `config/hash_test.go` (blackbox `package config_test`):

```go
package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hashYAML = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
`

// The same logical ruleset as equivalent JSON (object-form expr, reordered keys).
const hashJSON = `{"stages":[{"expr":{"expr":"price * qty"},"type":"single-expr","name":"base"}]}`

func TestPipelineDefHash(t *testing.T) {
	tests := []struct {
		name   string
		build  func(t *testing.T) (string, string) // returns two hashes to compare
		assert func(t *testing.T, a, b string)
	}{
		{
			name: "YAML and equivalent JSON hash identically",
			build: func(t *testing.T) (string, string) {
				y, err := config.ParseYAML([]byte(hashYAML))
				require.NoError(t, err)
				j, err := config.ParseJSON([]byte(hashJSON))
				require.NoError(t, err)
				return y.Hash(), j.Hash()
			},
			assert: func(t *testing.T, a, b string) {
				assert.Equal(t, a, b)
				assert.Len(t, a, 64, "hex sha256 is 64 chars")
			},
		},
		{
			name: "version does not affect the content hash",
			build: func(t *testing.T) (string, string) {
				d1, err := config.ParseYAML([]byte(hashYAML))
				require.NoError(t, err)
				d2, err := config.ParseYAML([]byte(hashYAML + "version: v9.9.9\n"))
				require.NoError(t, err)
				return d1.Hash(), d2.Hash()
			},
			assert: func(t *testing.T, a, b string) { assert.Equal(t, a, b) },
		},
		{
			name: "a changed expression changes the hash",
			build: func(t *testing.T) (string, string) {
				d1, err := config.ParseYAML([]byte(hashYAML))
				require.NoError(t, err)
				d2, err := config.ParseYAML([]byte("stages:\n  - name: base\n    type: single-expr\n    expr: price * qty * 2\n"))
				require.NoError(t, err)
				return d1.Hash(), d2.Hash()
			},
			assert: func(t *testing.T, a, b string) { assert.NotEqual(t, a, b) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := tt.build(t)
			tt.assert(t, a, b)
		})
	}
}

func TestPipelineDefMatchesRuleset(t *testing.T) {
	d, err := config.ParseYAML([]byte(hashYAML))
	require.NoError(t, err)
	assert.True(t, d.MatchesRuleset(pipe.RulesetIdentity{Hash: d.Hash()}))
	assert.False(t, d.MatchesRuleset(pipe.RulesetIdentity{Hash: "nope"}))
}
```

(If `config.ParseYAML`/`ParseJSON` are not the exact parser names, the implementer uses the actual exported parser in `config/parse.go` — the object under test is `(*PipelineDef).Hash()`, reached through whatever parses a definition.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./config/ -run 'TestPipelineDefHash|TestPipelineDefMatchesRuleset' -v`
Expected: compile failure — `Hash`, `MatchesRuleset` undefined (and `Version` unknown in the YAML if strict — it will parse once the field exists).

- [ ] **Step 3: Add the `Version` field**

In `config/def.go`, in the `PipelineDef` struct, add after `Schema`:

```go
	// Version is an optional author-declared release label (e.g. "v2.3.1"). It
	// names WHICH release a ruleset is, distinct from Hash() which fingerprints
	// WHAT it contains — so Version is deliberately excluded from the content
	// hash. Empty is valid; it is not an error.
	Version string `yaml:"version" json:"version"`
```

- [ ] **Step 4: Create `config/hash.go`**

```go
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/kartaladev/rlng/pipe"
)

// Hash returns a deterministic content fingerprint of the ruleset: the hex
// SHA-256 of the canonical JSON encoding of the parsed definition. Because it
// hashes the parsed structure (not the source bytes), the same logical ruleset
// hashes identically regardless of YAML vs JSON, formatting, comments, or map-key
// order (encoding/json sorts map keys); a changed rule or expression changes the
// hash. The author Version label is excluded, so re-labelling a release does not
// change what the hash proves. This is a plain fingerprint, not a tamper-proof
// signature (see ADR-0037).
func (d *PipelineDef) Hash() string {
	// Hash a copy with Version cleared so the label never affects the content
	// fingerprint. The copy shares the definition's maps/slices but does not
	// mutate them (marshal is read-only).
	canonical := *d
	canonical.Version = ""
	b, err := json.Marshal(canonical)
	if err != nil {
		// PipelineDef is composed of JSON-marshalable types, so marshal cannot
		// realistically fail; fall back to a stable hash of empty content rather
		// than panic on caller input.
		b = []byte("{}")
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// MatchesRuleset reports whether id's content hash equals this definition's
// Hash() — the replay-safety check: reload a candidate ruleset and compare it to
// a persisted decision's stamped identity so a replay against the wrong ruleset
// is detected rather than silent. It compares content (Hash) only; the Version
// label is not part of the match.
func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool {
	return d.Hash() == id.Hash
}
```

- [ ] **Step 5: Add `WithRulesetVersion` and wire `Build`**

In `config/build_options.go`, add `version` to `buildConfig`:

```go
type buildConfig struct {
	lintErrors bool
	schema     map[string]any
	strict     bool
	version    string // set by WithRulesetVersion; overrides PipelineDef.Version
}
```

and add the option:

```go
// WithRulesetVersion sets the author-declared release label stamped onto the
// built pipeline's ruleset identity, overriding any version: field in the
// document. The content Hash() is always computed regardless; this only names
// the release.
func WithRulesetVersion(v string) BuildOption {
	return func(c *buildConfig) { c.version = v }
}
```

In `config/build.go`, replace the tail of `Build` (the `p, err := pipe.NewPipeline(...)` block, ~line 54-58) with:

```go
	p, err := pipe.NewPipeline(stages...)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	version := cfg.version
	if version == "" {
		version = d.Version
	}
	return p.WithRuleset(pipe.RulesetIdentity{Hash: d.Hash(), Version: version}), nil
```

- [ ] **Step 6: Add the Build-wiring test**

Add to `config/build_test.go` (blackbox; if a table already exercises `Build`, fold these in as assert-closure cases per the `table-test` skill):

```go
func TestBuildStampsRulesetIdentity(t *testing.T) {
	d, err := config.ParseYAML([]byte(hashYAML + "version: v1.0.0\n"))
	require.NoError(t, err)

	tests := []struct {
		name   string
		opts   []config.BuildOption
		assert func(t *testing.T, id pipe.RulesetIdentity, want string)
	}{
		{
			name: "version from the document",
			assert: func(t *testing.T, id pipe.RulesetIdentity, wantHash string) {
				assert.Equal(t, wantHash, id.Hash)
				assert.Equal(t, "v1.0.0", id.Version)
			},
		},
		{
			name: "WithRulesetVersion overrides the document",
			opts: []config.BuildOption{config.WithRulesetVersion("v2.0.0")},
			assert: func(t *testing.T, id pipe.RulesetIdentity, wantHash string) {
				assert.Equal(t, "v2.0.0", id.Version)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := d.Build(tt.opts...)
			require.NoError(t, err)
			sc := pipe.NewScope(map[string]any{"price": 1, "qty": 1})
			require.NoError(t, p.Run(t.Context(), sc))
			id, ok := sc.Ruleset()
			require.True(t, ok)
			tt.assert(t, id, d.Hash())
		})
	}
}
```

- [ ] **Step 7: Run to verify pass**

Run: `go test ./config/ -run 'TestPipelineDefHash|TestPipelineDefMatchesRuleset|TestBuildStampsRulesetIdentity' -v && go test ./config/ ./pipe/ -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add config/hash.go config/def.go config/build.go config/build_options.go config/hash_test.go config/build_test.go
git commit -m "$(cat <<'EOF'
feat(config): ruleset fingerprint, author version, and replay helper

PipelineDef.Hash() fingerprints a ruleset as the hex SHA-256 of the canonical
JSON of the parsed definition (version excluded), so YAML and equivalent JSON
hash identically and a changed rule changes the hash. A version: field and
WithRulesetVersion set the release label; Build stamps RulesetIdentity{Hash,
Version} onto the pipeline. MatchesRuleset is the replay-safety check.

Spec: 013
Plan: 013
ADR: 0037
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Scope JSON round-trips ruleset identity and firing rules

Extend the Scope JSON envelope so a persisted decision carries its ruleset stamp and its firing rules, making the record fully replayable. Add json tags to `FiringRule` for a clean wire format.

**Files:**
- Modify: `pipe/firing.go` (`FiringRule` struct, ~line 8-13 — add json tags), `pipe/json.go` (`scopeJSON` ~line 11-15; `MarshalJSON` ~line 26-38; `UnmarshalJSON` ~line 52-79)
- Test: `pipe/json_test.go`

**Interfaces:**
- Consumes: `RulesetIdentity`, `s.ruleset` (Task 1), `s.firing` (`map[string][]FiringRule`, already present), `FiringRule`.
- Produces: the Scope JSON envelope gains optional `ruleset` and `firing` members; a Scope reloaded via `UnmarshalJSON` returns the stamp from `Ruleset()` and the firing rules from `FiringRule`/`FiringRules`/`FiringRulesFor`.

- [ ] **Step 1: Write the failing test**

Add to `pipe/json_test.go` (blackbox `package pipe_test`):

```go
func TestScopeJSONRoundTripsRulesetAndFiring(t *testing.T) {
	tbl, err := pipe.NewDecisionTable("denial", []pipe.Rule{
		{ID: "R1", Message: "too low", Condition: "score < 650", Decisions: map[string]string{"deny": "true"}},
	}, pipe.WithHitPolicy(pipe.HitPolicySingle))
	require.NoError(t, err)
	p, err := pipe.NewPipeline(tbl)
	require.NoError(t, err)
	p = p.WithRuleset(pipe.RulesetIdentity{Hash: "h123", Version: "v1"})

	sc := pipe.NewScope(map[string]any{"score": 600})
	require.NoError(t, p.Run(t.Context(), sc))

	b, err := json.Marshal(sc)
	require.NoError(t, err)

	var reloaded pipe.Scope
	require.NoError(t, json.Unmarshal(b, &reloaded))

	id, ok := reloaded.Ruleset()
	require.True(t, ok)
	assert.Equal(t, pipe.RulesetIdentity{Hash: "h123", Version: "v1"}, id)

	fired := reloaded.FiringRulesFor("denial")
	require.Len(t, fired, 1)
	assert.Equal(t, "R1", fired[0].RuleID)
	assert.Equal(t, "too low", fired[0].Message)
}

func TestScopeJSONOmitsAbsentRulesetAndFiring(t *testing.T) {
	sc := pipe.NewScope(map[string]any{"x": 1})
	b, err := json.Marshal(sc)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "ruleset")
	assert.NotContains(t, string(b), "firing")
}
```

(Ensure `encoding/json` is imported in the test file.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestScopeJSONRoundTripsRulesetAndFiring|TestScopeJSONOmitsAbsent' -v`
Expected: FAIL — reloaded `Ruleset()` is absent and `FiringRulesFor` is empty (the envelope drops both today).

- [ ] **Step 3: Add json tags to `FiringRule`**

In `pipe/firing.go`, change the `FiringRule` struct (~line 8-13) to carry json tags for a clean wire format:

```go
type FiringRule struct {
	Stage     string `json:"stage"`                // the decision-table stage name
	RuleID    string `json:"rule_id,omitempty"`    // the matched rule's ID ("" when the default fired)
	Message   string `json:"message,omitempty"`    // the matched rule's message
	IsDefault bool   `json:"is_default,omitempty"` // true when the table's default decisions fired
}
```

- [ ] **Step 4: Extend the JSON envelope**

In `pipe/json.go`, extend `scopeJSON` (~line 11-15):

```go
type scopeJSON struct {
	Data        map[string]any          `json:"data"`
	Timing      *scopeTimingJSON        `json:"timing,omitempty"`
	Derivations map[string]Derivation   `json:"derivations,omitempty"`
	Ruleset     *RulesetIdentity        `json:"ruleset,omitempty"`
	Firing      map[string][]FiringRule `json:"firing,omitempty"`
}
```

In `MarshalJSON` (~line 30-36), after the `provenance` block, add the ruleset and firing members:

```go
	env := scopeJSON{Data: s.data}
	if !s.startedAt.IsZero() {
		env.Timing = &scopeTimingJSON{StartedAt: s.startedAt, DurationNS: s.duration.Nanoseconds()}
	}
	if s.provenance {
		env.Derivations = s.derivations
	}
	if s.ruleset != (RulesetIdentity{}) {
		id := s.ruleset
		env.Ruleset = &id
	}
	if len(s.firing) > 0 {
		env.Firing = s.firing
	}
	return json.Marshal(env)
```

In `UnmarshalJSON` (~line 74-77), after the `Derivations` restore, add:

```go
	if env.Derivations != nil {
		s.provenance = true
		s.derivations = env.Derivations
	}
	if env.Ruleset != nil {
		s.ruleset = *env.Ruleset
	}
	if env.Firing != nil {
		s.firing = env.Firing
	}
	return nil
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./pipe/ -run 'TestScopeJSON' -v && go test ./pipe/ -race`
Expected: PASS. Existing Scope JSON tests still green (both new members are `omitempty`, so an unstamped, non-firing Scope serializes exactly as before).

- [ ] **Step 6: Commit**

```bash
git add pipe/json.go pipe/firing.go pipe/json_test.go
git commit -m "$(cat <<'EOF'
feat(pipe): round-trip ruleset identity and firing in Scope JSON

The Scope JSON envelope now carries the ruleset stamp and the firing rules
(both omitempty), so a persisted decision is self-describing and replayable:
inputs + firing + provenance + the ruleset that produced them survive reload.
FiringRule gains snake_case json tags for a clean wire format.

Spec: 013
Plan: 013
ADR: 0037
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: ADR-0037, runnable example, docs & whole-branch gate

**Files:**
- Create (controller-authored, added here): `docs/adrs/0037-ruleset-identity.md`
- Create/modify: a runnable `Example` demonstrating the identity + replay flow (`config/ruleset_example_test.go` or extend `config/example_test.go`)
- Modify: `README.md`, `config/doc.go`, and/or `pipe/doc.go` where they summarize identity/serialization
- Test: the example's `// Output:` block

**Interfaces:** none new — documentation + an `Example` test doubling as godoc.

- [ ] **Step 1: Controller authors the ADR** (`docs/adrs/0037-ruleset-identity.md`, Nygard format: Status/Date/Prompted-by/Context/Decision/Consequences), citing Spec 013 / Plan 013 and recording: canonical-JSON+SHA-256 (D1), version-excluded-from-hash (D2), programmatic-path-is-caller-supplied (D3), identity-on-Pipeline + `WithRuleset` + Run-stamps (D4/D5), Scope-JSON-round-trips-ruleset-and-firing (D6), the `MatchesRuleset` replay primitive (D7), and the threat-model note (plain fingerprint, not a tamper-proof signature). The implementer `git add`s it.

- [ ] **Step 2: Add a runnable example**

Add an `Example` (blackbox) that: parses a small YAML ruleset with a `version:`; prints `def.Hash() == def.Hash()` (stability, `true`) and the fact that a re-labelled version hashes equal; `Build`s and `Run`s it; prints `sc.Ruleset().Version` and a `FiringRulesFor` rule ID; marshals the Scope to JSON, unmarshals into a fresh Scope, and prints that the reloaded `Ruleset().Version` and firing survive plus `def.MatchesRuleset(reloaded.Ruleset())` is `true`. Print only **stable** values (booleans, the version string, rule IDs) — do NOT pin the 64-char hash in `// Output:`. Verify: `go test ./config/ -run '^Example' -v` PASS.

- [ ] **Step 3: Update docs** — README (add ruleset identity / replayable decision record to the explainability feature list), and `config/doc.go` / `pipe/doc.go` where they describe the config surface or Scope serialization, to mention `Hash()`, `version`, `Scope.Ruleset()`, and that the Scope JSON round-trips identity + firing. Accurate to shipped code only.

- [ ] **Step 4: Whole-package gate**

Run: `go test ./... -race && go vet ./... && gofmt -l . && CGO_ENABLED=0 go build ./...` — all clean; `gofmt -l` empty. Coverage: `go test ./pipe/ ./config/ -cover` ≥ 85%.

- [ ] **Step 5: Commit**

```bash
git add docs/adrs/0037-ruleset-identity.md config/ruleset_example_test.go README.md config/doc.go pipe/doc.go
git commit -m "$(cat <<'EOF'
docs(rlng): ADR-0037 + ruleset identity example and docs

Spec: 013
Plan: 013
ADR: 0037
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Only `git add` the doc files that actually exist/were changed.)

---

## Self-review

- **Spec coverage:** G1 (deterministic fingerprint) → Task 2 `Hash()` (D1/D2); G2 (optional author version) → Task 2 `Version` + `WithRulesetVersion`; G3 (stamp identity onto the decision record + JSON) → Task 1 (`Ruleset()` + Run stamping) and Task 3 (JSON round-trip of ruleset + firing, D6); G4 (replay safety hook) → Task 2 `MatchesRuleset` (D7). Programmatic-path caller-supplied (D3) is realized by `WithRuleset` accepting any identity (Task 1) and documented in ADR-0037 (Task 4). ADR/docs/example → Task 4.
- **Placeholder scan:** none — every code step shows complete code; Task 4's prose steps are documentation with a defined, stable Output.
- **Type consistency:** `RulesetIdentity{Hash, Version}` defined in Task 1, consumed in Tasks 2 (config wiring, `MatchesRuleset`) and 3 (JSON); `Pipeline.WithRuleset(RulesetIdentity) *Pipeline` and `Scope.Ruleset() (RulesetIdentity, bool)` consistent across Tasks 1–3; `scopeJSON.Firing map[string][]FiringRule` matches `s.firing`'s type (Task 1's Scope field, unchanged) and `FiringRule`'s json tags (Task 3).
- **Sequencing:** Task 1 is the foundation (type + stamping). Tasks 2 and 3 both depend on Task 1 and are independent of each other. Task 4 last (ADR/docs/gate). No breaking change — all additive.
- **Hot-path/typed-error branches covered:** Run stamps a configured identity; Run leaves an un-configured Scope unstamped (`Ruleset()` false); `Hash()` stability (YAML≡JSON), version-excluded, change-detected; version resolved from field vs option; JSON round-trip restores ruleset + firing; JSON omits both when absent; `MatchesRuleset` true/false.
