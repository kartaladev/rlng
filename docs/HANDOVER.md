# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 is DONE (increment 017, `main@f10a8be`).** Next up: **B2.**

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
- Increment 016 gate finished + merged & pushed (`546d54b`); branch deleted.
- Cross-increment backlog swept → `docs/BACKLOG.md` (B1–B12 + resolved appendix).
- **B1 / increment 017 shipped** (`main@f10a8be`): `foreach` `Rollup.Key` is now dot-path-aware —
  `applyRollup` resolves it via a new shared `lookupPath` helper (extracted from `Scope.Get`) instead of
  a flat `m[r.Key]`; a dot-free key is unchanged; no `Hash()`/schema change. Gate fix: no-dot fast path in
  `lookupPath`. Spec/plan 017, ADR-0042. Commits `8e8be9b`(spec)→`3a33e19`(backlog)→`73132da`(refactor)→
  `a545297`(feat)→`f10a8be`(perf). B1 moved to `docs/BACKLOG.md` Resolved.

## Next action — B2 (increment 018)
**B2 = `foreach` per-element scope-copy benchmark** (`docs/BACKLOG.md` B2; source ADR-0040). Autonomous
(no design checkpoint). Each `foreach` element deep-copies the outer scope's map spine via
`Snapshot()`+`NewScope` — O(elements × outer-scope size). Add a Go benchmark (`cc-skills-golang:golang-benchmark`)
measuring this cost across collection sizes; document the result. It is a measurement increment — optimize
only if the benchmark shows a real problem (if it does, that becomes its own follow-up with an ADR). Start:
`git checkout -b claude/foreach-scopecopy-bench-018 main`, brainstorm, write spec/plan 018, add the
benchmark under `pipe/`, verify, gate, merge+push.

## Exact state
- On `main`, clean, synced with `origin/main` at `f10a8be`. `git status --short` = empty.
- Full gate green as of B1 merge: `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .`,
  `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `pipe` 99.4%, `config` 99.5%.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at 017, ADRs at 0042. B2 = spec/plan 018, next ADR 0043.
