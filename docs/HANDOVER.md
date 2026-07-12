# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the SDD
> ledger `.superpowers/sdd/progress.md` (durable per-task record with commit SHAs), then the active
> spec/ADRs. Trust the ledger + `git log` over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Executing the post-010 audit remediation as focused specs (011–015).

- **Increment 014 (value serde consistency): COMPLETE, merged to `main`, pushed.**
- **Increment 015 (foreach line-item stage): COMPLETE — merged to `main` and pushed per explicit
  user authorization (2026-07-13: "Do this to completion, commit then merge to main, push to
  remote").** Branch `claude/foreach-015` deleted after merge.

## Increment 015 — DONE (foreach line-item stage)

Adds a `foreach` stage that adjudicates collections: per element it runs an inner sub-`Pipeline`
against a fresh per-element `Scope` (element bound as `item`, outer scope readable), writes structured
per-element output at `<stage>.items`, optionally rolls a per-element key up to a header value
(`<stage>.<As>`, reusing the 014-hardened int64/decimal-faithful `aggregate`), and records each
element's firing under `<stage>[i]` (`FiringRulesFor("<stage>[i]")` answers "line i denied by rule X").
Config surface: `type: foreach` with nested inner `StageDef`s; nested foreach rejected at build
(`ErrNestedForEach`). Governing artifacts: `docs/specs/015-foreach-line-item-stage.md` (Accepted, D1–D9),
`docs/plans/015-foreach-line-item-stage.md`, `docs/adrs/0040-foreach-stage.md`.

SDD execution (4 tasks, each task-reviewed clean): `aec9e63` T1 ForEach core · `d98d435` T2 roll-up +
per-element firing · `7051ae0` T3 config surface + nested guard · `0736291` T4 acceptance example +
README · `f3a1ec8` whole-branch gate fixes. Base `main@45c2d81`.

### Whole-branch gate (main..HEAD)
`/code-review` (high) — 2 Important findings fixed: (1) the four new `StageDef` foreach fields gained
`json:",omitempty"` so `Hash()` is unchanged for pre-015 rulesets → `MatchesRuleset` replay-safety holds
across the upgrade (golden-hash test pins the pre-015 value, verified against `main`); (2) `NewForEach`
now rejects an empty-`Key`/`As` `Rollup` (`ErrForEachEmptyRollup`) fail-loud at construction.
`/security-review` — clean. `go test ./... -race` green; `go vet`/`gofmt`/`CGO_ENABLED=0 build` clean;
`go mod tidy` no-op, `go mod verify` passes. No new dependencies.

### New exported API shipped in 015
`pipe.TypeForEach`, `pipe.ForEach`, `pipe.NewForEach` + `WithForEachAs`/`WithForEachOutput`/
`WithForEachDependsOn`/`WithRollups`, `pipe.Rollup{Key,Agg,As}`; sentinels `pipe.ErrForEachNotList`,
`pipe.ErrForEachNoCollection`, `pipe.ErrForEachEmptyRollup`. Config: `StageDef` foreach fields
(`Collection`/`As`/`Stages`/`Rollups`, all `omitempty`), `config.RollupDef`, `config.ErrNestedForEach`.

### Carried-Minor backlog (triaged, non-blocking — address in a future increment)
Recorded in ADR-0040 "Whole-branch gate outcome":
- **Dot-path roll-up keys.** `Rollup.Key` is a flat top-level lookup in each element's result map, so
  rolling up a decision-table output (namespaced `<table>.<key>`) needs a companion `single-expr` to
  surface the value top-level (the acceptance example shows this). Make `Key` dot-path-aware
  (backward-compatible) in a future increment.
- **Per-element lineage beyond firing (D5).** Each element's full derivation graph (built when the
  outer scope tracks provenance) is discarded — only the data `Snapshot()` survives in `items`. Firing
  is retrievable; deeper per-element lineage is not yet surfaced.
- **Per-element `Snapshot()`+`NewScope` cost.** O(elements × outer-scope size) map-spine clone per
  element; benchmark before large collections.
- Pre-existing (Plan-013 leftover): a `golangci-lint`/`staticcheck` finding in
  `config/ruleset_example_test.go:58` — unrelated; clean up when convenient.
- 014 Carried-Minor items (see git history of this file) remain open.

## Next increment
Post-010 audit remediation specs 011–015 are all shipped. No 016 is planned yet — pick the next audit
gap or a backlog item above; brainstorm → spec(Accepted) → plan → SDD → gate, per CLAUDE.md.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI (`.github/workflows/ci.yml`) runs them
  across a Go 1.25/1.26 matrix. Let CI go green after the push.
- **Verified expr/decimal facts (do not re-derive):** operator overloading resolves at COMPILE time by
  static type → `decimal(...)` wrapping is universal, bare arithmetic needs strict-env (`expr.WithEnv`);
  YAML `!decimal` tag collapses to string in `map[string]any` → use object form `{"$dec":"…"}`.
- **Verified foreach facts (do not re-derive):** `Hash()` fingerprints the canonical JSON of the parsed
  `PipelineDef`, so any new always-emitted `StageDef` field changes every ruleset's hash — new fields
  MUST carry `json:",omitempty"` to preserve cross-version replay-matching. A decision-table writes its
  outputs namespaced under the table name, so a flat roll-up key can't reach them (see backlog).
- SDD hand-off files (`.superpowers/sdd/*`) are gitignored scratch; the ledger `progress.md` is the
  durable record. `git clean -fdx` would destroy it — recover from `git log`.
