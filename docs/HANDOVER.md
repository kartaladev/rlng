# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then
> `git log` on `main`, then the active spec/plan/ADR if you pick up new work. Trust the repo
> (code + git history + `docs/`) over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Post-010 audit-remediation increments 011–015 are all merged & pushed to `main`.
**Increment 016 (config source Provider abstraction) is COMPLETE — gated green, merged & pushed to
`main`, and its branch deleted.** No increment 017 is planned yet.

## Increment 016 — what shipped (governing artifacts)
- Spec: `docs/specs/016-config-source-provider.md` (decisions D1–D10).
- Plan: `docs/plans/016-config-source-provider.md` (5 tasks, all checked off).
- ADR: `docs/adrs/0041-config-source-provider.md` (Accepted).

**Outcome:** a single `config.Parse(ctx, Provider)` entry point replaces the removed
`ParseYAML`/`ParseJSON`/`LoadFile` (breaking, pre-1.0). Providers: `FromYAMLBytes`/`FromJSONBytes`,
`FromYAMLString`/`FromJSONString`, `FromReader(r, kind)`, `FromFile`(ext-inferred) /
`FromYAMLFile`/`FromJSONFile`, and hardened `FromYAMLURL`/`FromJSONURL` (`WithHTTPClient`/`WithMaxBytes`;
http/https-only checked before dialing, default 10s timeout + 5 MiB cap, ctx-cancellable). Typed
`*ConfigError` sentinels throughout: `ErrUnknownSourceKind`, `ErrNilSource`, `ErrUnsupportedExtension`,
`ErrUnsupportedScheme`, `ErrUnexpectedStatus`, `ErrMaxBytesExceeded`. No new module dependency
(`net/http` is stdlib). `config` package at 99.5% coverage; every hot-path/typed-error branch covered.

**Commits (all now on `main`):** `02c1bdc` (Task 1 core) · `4495512` (Task 2 file) · `05b7225`
(Task 3 URL) · `5317a77` (Task 4 breaking removal + call-site migration) · `3be4ec3` (Task 5 gate fix:
`Parse` nil-`Source` guard) · handover.

**Gate (Task 5):** `/code-review high main..HEAD` found one finding (Parse nil-`Source` panic) — fixed
in `3be4ec3`. `/security-review` (+ independent adversarial audit) found no vulnerability; the increment
adds hardening. Full green: `build`/`vet`/`gofmt`/`CGO_ENABLED=0`/`-race`/`go mod tidy`(no-op)/`verify`.

## Next actions
- No pending 016 work. Start any new increment from a fresh branch off `main` per CLAUDE.md
  (brainstorm → spec → plan/ADR → TDD → gate).

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- SDD hand-off files (`.superpowers/sdd/*`) are gitignored scratch, not part of the repo.
