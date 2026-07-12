# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the SDD
> ledger `.superpowers/sdd/progress.md` (durable per-task record with commit SHAs), then the active
> spec. Trust the ledger + `git log` over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Executing the post-010 audit remediation as focused specs (011–015).

- **Increment 014 (value serde consistency): COMPLETE, gated green, merged to `main`, and pushed.**
- **Increment 015 (foreach line-item stage): PLANNED (spec Accepted + Plan written & committed);
  implementation NOT started — to begin in a FRESH session per the user's directive.**

## ⏸ PAUSE POINT (this session, 2026-07-13)

Stopped cleanly after planning 015, per the user: *"Pause after plan, we will start implementation on
a clean new session."* Tree builds green (only docs changed on the 015 branch). **To resume 015:**
on branch `claude/foreach-015` (at `31b902d`, base `main@45c2d81`), read `CLAUDE.md`, then
`.superpowers/sdd/progress.md` (fresh 015 ledger), `docs/specs/015-foreach-line-item-stage.md`
(Accepted, D1–D9), and `docs/plans/015-foreach-line-item-stage.md`, then execute via
`superpowers:subagent-driven-development` starting at Task 1. **015 is GATED: do NOT push — merge/push
awaits explicit user approval.**

## Increment 014 — DONE

Delivered exact-decimal money + value fidelity across all six serde boundaries. Merged to `main`
(fast-forward) and pushed per explicit user authorization. Branch `claude/value-serde-014` deleted.

Commits (base `main@98e59d1`): `6cc30b2` spec · `3f4a5cf` T1 expr decimal type · `1207939` T2
aggregation fidelity · `30dbcb6` T3 type-tagged Scope JSON (v2) · `96d3cac` T4 config/seed/mapping ·
`eed5962` T5 acceptance example + README · `11a6f1a` fix-4 · `4e42c45` whole-branch gate fixes.

Gate: `/code-review` (high) — 4 findings, 2 fixed (mapper int64-overflow narrowing → `ErrLossyResultNarrowing`;
Scope JSON decimal scale preservation), 2 triaged (below). `/security-review` — clean. `go test ./... -race`
green; `go vet`/`gofmt`/`CGO_ENABLED=0 build` clean; `go mod tidy` no-op, `go mod verify` passes.
`shopspring/decimal v1.4.0` is the only new dep. New exported API: `decimal()`/`round`/`roundBank`
builtins; `pipe.ErrAggregateOverflow`, `pipe.ErrMalformedScopeValue`, `config.ErrDecimalLiteral`,
`rlng.ErrLossyResultNarrowing`; Scope JSON `"v":2` type-tagged codec; config `{"$dec":"…"}` literals.

### Observable API changes shipped in 014 (note for any SemVer/apidiff work)
- Aggregation sum/min/max now returns `int64` (was `int(acc)`) — the concrete type inside the `any`
  result changed `int`→`int64` (spec G2).
- Scope JSON write format is now `v2` (type-tagged): a pre-014 reader cannot parse a v2 blob; new
  code still reads legacy v0 blobs (one-way format evolution, ADR-0038).
- Integers reload from v2 JSON as `int64`; `GetInt` gained an `int64` case; a reloaded integer read
  via `GetFloat64` now errors (strict, no coercion) instead of widening.

### Carried-Minor backlog (triaged, non-blocking — address in a future increment)
- `expr/variables.go`: a decimal-constant default via `String()` drops scale for trailing-zero
  constants (`{"$dec":"1.50"}` → `decimal("1.5")`); numerically exact, rare.
- `expr/options.go`: `decimalExprOptions()` rebuilt per expression compile (one-time cost, not hot path).
- `pipe/table.go`: `classify` + `toInt64`/`toFloat64`/`asDecimal` are 3 parallel type-lists (drift risk).
- `pipe/valuejson.go`: `derivations` values remain untagged (only `data` is type-tagged); redundant
  (harmless) `sort.Strings` in the map encode.
- `mapper_test.go`: `TestMapperDecimalFidelity` uses a `run`-closure, not the canonical assert-closure shape.
- Pre-existing (Plan-013 leftover): a `golangci-lint`/`staticcheck` finding in
  `config/ruleset_example_test.go:58` — unrelated to 014; clean up when convenient.

## Increment 015 — NEXT (foreach line-item stage)

Read `docs/specs/015-foreach-line-item-stage.md` first (Draft; goals G1–G5, ADR-0040 anticipated).
Per the user's AFK directive (2026-07-13): take 015 through the full brainstorm → spec(Accepted) →
Plan 015 → SDD execution → whole-branch gate cycle **autonomously**, documenting decisions +
alternatives (the user is unavailable to brainstorm live). **LEAVE 015 GATED — do NOT push 015; its
merge/push awaits the user's explicit morning approval.** (The "merge then push" authorization
attached to increment 014 only.)

Key 015 design decisions to settle in the autonomous brainstorm (record in ADR-0040 + the spec):
per-element scope binding (element as `item`, outer scope readable); indexed/namespaced per-element
output path + optional Spec-010 roll-up; per-element firing/provenance (reuse Spec-012 model keyed by
index); YAML/JSON config surface (Spec-011 strict-decoding honored); nested-foreach → defer with a
clear error (spec non-goal); empty collection = defined no-op; non-list path / per-element error =
typed `StageError` naming the index; context-cancellable at element boundaries. 015 per-element
numeric outputs must honor the 014 value contract (decimal/int64 fidelity).

## Gotchas / environment

- **Session limit was hit ~014 Task-4-fix (reset 5am Asia/Jakarta).** The remaining 014 work
  (Task-4-fix completion, Task 5, the whole-branch gate, and both fixes) was completed by the
  controller in-context (the `/code-review` and `/security-review` skills run in the main context, not
  via subagents). If resuming 015 hits the same limit, prefer in-context work over subagent dispatch,
  or wait for reset. Task-4's fixes were controller-self-verified (not subagent-re-reviewed).
- `govulncheck`/`golangci-lint` are NOT installed locally — CI (`.github/workflows/ci.yml`) runs them
  across a Go 1.25/1.26 matrix. Let CI go green.
- **Verified expr/decimal facts (do not re-derive):** operator overloading resolves at COMPILE time by
  static type → `decimal(...)` wrapping is universal, bare arithmetic needs strict-env (`expr.WithEnv`);
  YAML `!decimal` tag collapses to string in `map[string]any` → use object form `{"$dec":"…"}`;
  mapstructure decomposes struct-nested `decimal.Decimal` to an empty map and hooks don't fire → the
  map-seed path preserves decimals, struct-seed needs the `flatten` reflection restore, struct-nested
  slices stay typed slices (no restore needed).
- SDD hand-off files (`.superpowers/sdd/*`) are gitignored scratch; the ledger `progress.md` is the
  durable record. `git clean -fdx` would destroy it — recover from `git log`.
