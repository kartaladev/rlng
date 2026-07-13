# Plan 017 — dot-path `foreach` roll-up keys (B1)

- **Implements:** Spec 017 (`docs/specs/017-foreach-rollup-dotpath-keys.md`).
- **Records:** ADR-0042 (rides in Task 2); updates ADR-0040's roll-up contract note.
- **Backlog:** graduates B1 (`docs/BACKLOG.md`).

Two tasks, TDD each, both green units. Task 1 is a behavior-preserving refactor (extract the shared
path-walk); Task 2 adds the dot-path feature on top. Standing program authorization: commit each green
task, then run the whole-branch gate and auto merge+push.

## Task 1 — Extract `lookupPath`, delegate `Scope.Get` to it (refactor, behavior-preserving)

- [ ] **Step 1 (green stays green):** Add unexported `lookupPath(m map[string]any, path string) (any, bool)`
  in `pipe/scope.go` — the exact walk currently inlined in `Scope.Get`: `strings.Split(path, ".")`, walk
  `map[string]any` levels, return `(nil,false)` on a missing key or non-map node. Empty path → `(nil,false)`.
- [ ] **Step 2:** Rewrite `Scope.Get` to acquire its `RLock` and delegate to `lookupPath(s.data, path)`.
  No behavior change.
- [ ] **Step 3 — verify:** existing `Scope.Get` tests stay green (`go test ./pipe/ -race`); `go vet`,
  `gofmt`. **Hot-path branches (already covered by existing scope tests, confirm):** empty path; missing
  segment; non-map intermediate; nested hit. Add a case only if coverage on `lookupPath` shows a gap.
- [ ] **Step 4 — commit:** `refactor(pipe): extract lookupPath shared by Scope.Get` (Spec 017, Plan 017).

## Task 2 — `applyRollup` resolves `Rollup.Key` as a dot-path (feature, TDD red→green)

- [ ] **Step 1 (red):** Add failing table cases in `pipe/foreach_test.go` driving a `foreach` whose
  `Rollup.Key` is a **dot-path** into a decision-table output (`<table>.<key>`), through the public API
  (build a `ForEach`/config and `Run` it). Cases → **hot-path branches from Spec 017 success criteria:**
  (2) nested key resolves; (3) missing leaf → element skipped; (4) non-map intermediate → skipped;
  (5) mixed elements fold over present-only; (6) each `Aggregate*` kind over a dot-path; (1) dot-free key
  regression (existing cases already cover, keep).
- [ ] **Step 2 (green):** In `applyRollup` (`pipe/foreach.go:211`) replace `v, ok := m[r.Key]` with
  `v, ok := lookupPath(m, r.Key)`. Nothing else changes — empty-fold switch, `setRollup`, aggregation all
  untouched.
- [ ] **Step 3 — docs & ADR:** Update `Rollup.Key` godoc (state it is a dot-path; dot-free = top-level).
  Update ADR-0040's roll-up contract note (the flat-key limitation is lifted) and, if the acceptance
  example used a companion `single-expr` solely to surface the value, simplify it. Author **ADR-0042**
  (Nygard: context = flat-key limitation forcing boilerplate; decision = dot-path resolution reusing
  `lookupPath`, backward-compatible, no hash change; consequences). Move B1 to `docs/BACKLOG.md`'s Resolved
  section.
- [ ] **Step 4 — verify:** `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .` (empty),
  `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op)/`verify`; `pipe` coverage ≥ 85% and every new
  branch covered.
- [ ] **Step 5 — commit:** `feat(pipe): resolve foreach Rollup.Key as a dot-path (B1)` (Backlog B1,
  Spec 017, Plan 017, ADR 0042).

## Whole-branch gate (Task 5-equivalent)

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; full green gate; then
auto merge+push + delete branch (standing program authorization), and start B2.
