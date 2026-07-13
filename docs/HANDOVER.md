# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the SDD
> ledger `.superpowers/sdd/progress.md` (durable per-task record with commit SHAs + the exact Task 4
> migration recipe), then the active spec/plan/ADR. Trust the ledger + `git log` over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Post-010 audit-remediation increments 011–015 are all merged & pushed to `main`
(015 foreach shipped at `main@20465d5`). **Increment 016 (config source Provider abstraction) is IN
PROGRESS on branch `claude/config-provider-016`.**

## ⏸ PAUSE POINT (2026-07-13) — session limit; resume in a fresh session (reset 10:10am Asia/Jakarta)

Stopped at a **clean safepoint**: increment 016 Tasks 1–3 committed & green; tree clean; `HEAD 05b7225`;
`go build ./...` + `go test ./... -race` green. Paused because subagents began failing on the session
limit and Task 4 (a large atomic migration) + Task 5 (gate) are too large to do well on the remaining
budget — per CLAUDE.md, hand over only from a safepoint; don't push a large change under pressure.

## Increment 016 — governing artifacts (read first)
- Spec: `docs/specs/016-config-source-provider.md` (Draft; decisions D1–D10). Committed `8a6e862`.
- Plan: `docs/plans/016-config-source-provider.md` (5 tasks). Rode in Task 1's commit.
- ADR: `docs/adrs/0041-config-source-provider.md` (Accepted). Rode in Task 1's commit.

## What's DONE (016, branch `claude/config-provider-016`, base `main@20465d5`)
- **Task 1 `02c1bdc`** — `config/source.go` (`SourceKind` enum+`String()`, `Provider`/`Source` interfaces,
  `ErrUnknownSourceKind`), `config/parse.go` rewrite (`Parse(ctx, Provider)` + shared
  `decodeYAML`/`decodeJSON(io.Reader)`; `ParseYAML`/`ParseJSON` **kept, delegating**; `LoadFile` kept),
  `config/providers.go` (bytes/string/reader providers; `nopReader` hides a caller's closer). Reviewed clean.
- **Task 2 `4495512`** (amended after a review fix) — file providers `FromFile` (ext-inference),
  `FromYAMLFile`/`FromJSONFile`, `ErrUnsupportedExtension`, trust-boundary godoc; `fileSource` returns
  the `*os.File` so `Parse` closes it. `providers.go` 100%. Reviewed clean (1 fix: `FromJSONFile` test).
- **Task 3 `05b72259`** — `config/urlsource.go` hardened URL providers `FromYAMLURL`/`FromJSONURL` +
  `URLOption` (`WithHTTPClient`/`WithMaxBytes`), sentinels `ErrUnsupportedScheme`/`ErrUnexpectedStatus`/
  `ErrMaxBytesExceeded`. Scheme checked before dialing; `LimitReader(body, max+1)`+`len>max` cap;
  `NewRequestWithContext`; `defer body.Close()`; default client 10s timeout; `bytesSource` reuse.
  8 `httptest` cases. Reviewed **in-context** by the controller (subagent reviewer hit the limit) — Approved.

## What's REMAINING (the ledger `.superpowers/sdd/progress.md` has the full step-by-step)
- **Task 4 — remove `ParseYAML`/`ParseJSON`/`LoadFile`; migrate all call sites** (NOT STARTED, base
  `05b72259`). The atomic breaking cut: delete the 3 funcs from `config/parse.go`; migrate ~45 call
  sites (`grep -rn 'ParseYAML\|ParseJSON\|LoadFile' --include='*.go'` → 60 hits) across 13 test files +
  `examples/` per the mapping in the ledger (`config.ParseYAML([]byte(s))` → `config.Parse(ctx,
  config.FromYAMLString(s))`, etc.; `ctx` = `t.Context()` in tests, `context.Background()` in examples);
  **rewrite the dedicated `TestParseYAML`/`TestParseJSON`/`TestLoadFile`/`ExampleParseYAML`** (they test
  the removed symbols); update `config/doc.go` + `config/hash.go` godoc + `README.md`; **add a synthetic
  fake-`Provider` test** covering `Parse`'s raw-error-wrap fallback (closes the last hot-path branch —
  carried finding). Commit `feat(config)!: remove ParseYAML/ParseJSON/LoadFile for Parse(ctx, Provider)`.
- **Task 5 — whole-branch gate** (NOT STARTED): `/code-review high main..HEAD` + `/security-review`
  (focus the URL provider), fix/triage, full green gate, then **present for merge/push — 016 is GATED,
  do NOT push** (015's merge/push authorization did NOT carry over to 016).

## Carried findings for the Task 5 gate
- `config/parse.go` raw-error-wrap fallback (a `Provider.Source` returning a non-`*ConfigError`) is
  unreachable by all in-repo providers → add the fake-`Provider` test in Task 4 to cover it.
- `config/urlsource.go` `http.NewRequestWithContext` error branch is defensive/untested (unreachable via
  the public API: constant GET, URL already `url.Parse`d) — acknowledge, no fix needed.

## Gotchas / environment
- **Session-limit behavior:** subagents fail with a session-limit error near the cap; the controller's
  own in-context work may still run. Prefer in-context work or wait for the reset. Task 4 is best done
  fresh (subagent capacity restored) as one green unit; commit only when the WHOLE repo is green.
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- SDD hand-off files (`.superpowers/sdd/*`) are gitignored scratch; `progress.md` is the durable record
  (has the full Task 4 recipe). `git clean -fdx` would destroy it — recover from this file + `git log`.
