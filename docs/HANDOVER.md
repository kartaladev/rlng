# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 (incr 017, `f10a8be`); B2 (incr 018, `934f1b5`); B3 (incr 019, `2af21f2`); B4 (incr 020,
`4b3089e`); B5 (incr 021 — this branch, feat commit; merge SHA in `git log` after merge).** B1–B5 complete.
**B6 is next** (a design-checkpoint item — pause for the user's design approval before implementing).

## Standing decisions for this program (do NOT re-ask)
- **Scope = all 12**, including the two deliberate non-goals **B11** (parallel exec; write a superseding
  ADR for ADR-0006 first) and **B12** (YAML env/host functions; superseding ADR for ADR-0028 first).
- **Cadence = AUTO merge+push each green increment** (standing authorization). After each increment's full
  gate passes, merge to `main`, push, delete branch, start the next — no per-increment merge/push approval.
  Does NOT extend to release tags (still need explicit approval).
- **Design-gating = checkpoint only the risky ones.** Design + implement **B2, B3, B4, B10** autonomously
  (surface design via the committed spec, no live approval pause). **PAUSE for the user's design approval**
  before implementing **B5, B6, B7, B8, B9, B11, B12**. Code-review + security-review gates run on every
  increment regardless.
- Per-increment workflow: brainstorm → spec `docs/specs/NNN` → plan `docs/plans/NNN` + ADR `docs/adrs/NNNN`
  → TDD (red/green) → `/code-review high main..HEAD` + `/security-review` → resolve findings → merge+push.
  (See memory `rlng-backlog-execution-program` for the full authorization record.)

## What's DONE this session
- **B5 / increment 021 (this branch `claude/per-decision-options-021`):** per-decision options in decision
  tables (Option A). `pipe.Rule.Decisions` is now `map[string]pipe.Decision` (new exported
  `Decision{Expr, Options}`); the shared `Rule.DecisionOptions` field is **removed**; `WithDefault` takes
  `map[string]pipe.Decision`; `compileDecisions` compiles each decision with its own options. `config`'s
  `bareDecisions` (which rejected per-decision options) is replaced by `decisionsFrom`, threading each
  `ExprDef`'s options + the shared constants/strict env. **Breaking pre-1.0 `pipe` API change** (flagged
  `feat(pipe)!`, `apidiff` note in the commit body); **no config-schema or `Hash()` change** — parsed
  `PipelineDef` shape untouched, `TestPipelineDefHash` golden test byte-identical. All ~8 pipe/config/example
  test files migrated; new behavior tests `pipe/table_options_test.go` + `config/table_options_test.go`;
  README updated. Spec/plan 021, ADR-0046 (supersedes ADR-0007 §5 in part). BACKLOG B5 → resolved.
  Full gate green; code-review: 1 finding (stale README API) fixed; security-review: no findings.

## Next action — B6 (increment 022): DESIGN CHECKPOINT (pause for user approval)
**B6 — Precise member-path references in provenance** (ADR-0011; Spec 006 non-goal; P2). Provenance `Inputs`
records top-level identifiers only (`a` for `a.b.c`); precise member-path lineage is the recorded
refinement. **This is a design-checkpoint item** — brainstorm → write spec `docs/specs/022-*` → **PAUSE and
get the user's design approval before implementing** (per the standing design-gating decision above). Then
plan 022 + ADR-0047 ride with the feat commit; full gate → auto merge+push.

Start: `git checkout -b <branch> main` (after this B5 branch is merged), brainstorm, author the spec.

## Exact state
- On branch `claude/per-decision-options-021` off `main@32978f7`. **B5 implementation complete and green**,
  about to commit the single feat increment (this handover rides in that commit). After commit: whole-branch
  gate already run (clean) → auto merge+push + delete branch (standing authorization).
- Full gate green: `go build ./...`, `go test ./... -race` (all packages ok), `go vet`, `gofmt -l .`
  (empty), `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `pipe` coverage 99.3%,
  `config` coverage 99.7%. `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: **spec 021 DONE**, plans at 020 done, ADRs at 0045 done. **B5 = plan 021 + ADR-0046 next (spec 021 already on main).**
