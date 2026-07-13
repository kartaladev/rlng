# Plan 020 — `Hash()` rejects non-marshalable hand-built defs (B4)

- **Implements:** Spec 020 (`docs/specs/020-hash-nonmarshalable-defs.md`).
- **Records:** ADR-0045 (rides in Task 1's commit); refines ADR-0037.
- **Backlog:** graduates B4 (`docs/BACKLOG.md`).

One task, TDD red→green. A contained edge-case fix: fail loud at `Build` for a hand-built non-marshalable
def, no `Hash()`/`MatchesRuleset` signature change. Standing program authorization: commit the green task,
run the whole-branch gate, auto merge+push.

## Task 1 — Reject non-marshalable defs at `Build` (feature, TDD)

- [x] **Step 1 (red):** In `config/hash_test.go` (or `build_test.go`), add cases through the public API
  (blackbox `package config_test`, hand-building a `config.PipelineDef`). **Hot-path branches from the
  success criteria:** (1) `Build` on a def whose `Constants` holds an unreferenced `func` (reaches the
  stamp point — a bare single-expr `"1 + 1"` references no constant, so stage build succeeds) →
  `errors.Is(err, config.ErrUnhashableDef)` and the error is a `*ConfigError`; (3) `Hash()` on that same
  def → returns a stable 64-char placeholder, no panic. Keep (2)/(4): existing marshalable `Build`/`Hash`
  cases stay green.
- [x] **Step 2 (green):** In `config/hash.go`:
  - Extract `func (d *PipelineDef) canonicalJSON() ([]byte, error)` — the Version-cleared `json.Marshal`.
  - `Hash()` calls it and keeps the documented `"{}"` placeholder fallback on error (behavior byte-identical
    for all existing inputs).
  - Add `var ErrUnhashableDef = errors.New("config: definition is not canonically hashable (contains a
    non-JSON-marshalable value)")` (in `config/errors.go` or `hash.go`, wherever the config sentinels live).

  In `config/build.go:72`, replace `d.Hash()` in the stamp with the error-returning path:
  compute `b, err := d.canonicalJSON()`; on error return `&ConfigError{Field: "constants", Cause:
  fmt.Errorf("%w: %v", ErrUnhashableDef, err)}` (Field best-effort — the marshal error names the type);
  otherwise stamp `hashBytes(b)` (factor the sha256+hex so Hash and Build share it, or recompute from `b`).
- [x] **Step 3 — docs & ADR:** Update `Hash()`'s godoc (the fallback remains for direct callers; `Build`
  now rejects a non-marshalable def). Author **ADR-0045** (Nygard: context = ADR-0037's silent fallback
  loses change-detection for hand-built defs; decision = fail loud at `Build` via `ErrUnhashableDef`, no
  `Hash()` signature break, shared `canonicalJSON`; consequences = additive error surface on `Build`,
  strictly-safer, residual direct-`Hash()` fallback documented). Move **B4** to `docs/BACKLOG.md`'s Resolved
  section (closing increment 020 / ADR-0045).
- [x] **Step 4 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) / `go mod
  verify`; `config` coverage ≥ 85% and the new reject branch + the unchanged fallback branch both covered.
- [x] **Step 5 — commit:** `fix(config): reject non-marshalable defs at Build (B4)` (Backlog B4, Spec 020,
  Plan 020, ADR 0045).

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item (B5,
which is a design-checkpoint item — pause for user approval before implementing).
