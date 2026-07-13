# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 (incr 017, `f10a8be`); B2 (incr 018, `934f1b5`); B3 (incr 019, `2af21f2`); B4 DONE
(incr 020, `4b3089e`).** The autonomous set (B1–B4) is complete. Next up: **B5 — a DESIGN-CHECKPOINT item
(needs the user's design approval before implementing).**

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
- **B2 / increment 018** (`934f1b5`): benchmark-only — `BenchmarkForEachScopeCopy`; scope-copy cost linear
  both axes, sub-ms typical → **accepted, no optimization** (ADR-0043). No prod change.
- **B3 / increment 019** (`2af21f2`): opt-in coercing numeric getters `GetIntCoerce`/`GetInt64Coerce`/
  `GetFloat64Coerce` (`pipe/get.go`); safe/honest per ADR-0035; additive, no SemVer break (ADR-0044).
- **B4 / increment 020 shipped** (`main@4b3089e`): `(*config.PipelineDef).Build` now **rejects** a
  hand-built def carrying a non-JSON-marshalable value (in any `any`-typed field: `Constants`/`Schema`/
  `Globals`) with a `*ConfigError` wrapping the new exported `config.ErrUnhashableDef`, instead of silently
  stamping the `{}` placeholder identity. Version-cleared marshal factored into shared
  `canonicalJSON`/`hashCanonical`. **`Hash()`/`MatchesRuleset` signatures unchanged** (direct-`Hash()`
  fallback retained + documented); existing hashes byte-identical (golden test green); parse paths
  unaffected. Spec/plan 020, ADR-0045; BACKLOG B4 → resolved. Commits `311b7db`(spec)→`4b3089e`(fix).
  Code-review: no findings; security-review: clean.

## Next action — B5 (increment 021) — ⚠ DESIGN CHECKPOINT (needs user approval before implementing)
**B5 = Per-decision options in decision-table config** (`docs/BACKLOG.md` B5; source ADR-0007, Spec 004,
`config/build.go:358`). **This is a design-gated item** (B5–B9, B11, B12): brainstorm and write the
spec, then **PAUSE and get the user's design approval before implementing** — do NOT auto-proceed to
code/merge as with B1–B4. Today `stage.Rule` carries one rule-level `DecisionOptions` shared across all
decisions; config **rejects** a decision that declares its own `fallback`/`globals` (`config/build.go`
~line 358, "per-decision options are not supported; use a bare expression"). Bare-string decisions are
unaffected. Extending to per-decision options is a **contract change → needs a new ADR (0046)**. Start:
`git checkout -b claude/per-decision-options-021 main`, read `config/build.go` (the decision-table build +
the ~line-358 rejection), `docs/adrs/0007-*`, `docs/specs/004-*`, and the `stage` decision-table types;
brainstorm → **spec 021 → present design for approval** → (on approval) plan 021 + ADR-0046 → TDD → gate →
merge+push.

## Exact state
- On `main`, clean, synced with `origin/main` at `4b3089e`. `git status --short` = empty.
- Full gate green as of B4 merge: `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .`,
  `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `config` coverage 99.8%.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at 020 done, ADRs at 0045 done. **B5 = spec/plan 021, ADR-0046.**
