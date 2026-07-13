# Spec 020 — `Hash()` rejects non-marshalable hand-built defs

- **Status:** Draft
- **Backlog item:** B4 (`docs/BACKLOG.md`) — the `Hash()` non-marshalable-fallback tech-debt recorded in
  ADR-0037.
- **Realized by:** Plan 020; ADR-0045.

## Problem

`(*config.PipelineDef).Hash()` (`config/hash.go`) fingerprints a ruleset as the SHA-256 of the canonical
JSON of the parsed definition. It **assumes every field value is JSON-marshalable** and, on a marshal
failure, **silently falls back** to hashing `"{}"`:

```go
b, err := json.Marshal(canonical)
if err != nil {
    b = []byte("{}") // silent fallback — change-detection lost
}
```

A hand-built `PipelineDef` can carry a non-marshalable value in any of its `any`-typed fields —
`Constants`, `Schema` (both `map[string]any`), or any `ExprDef.Globals` — e.g. a `chan`, `func`, or
`complex`. Every such def then hashes to the **same** placeholder, so `MatchesRuleset` can false-match and
a changed rule goes undetected — the opposite of `Hash()`'s change-detection guarantee, and a silent
failure in a debuggability-first engine. The Parse path (any `Provider`) decodes only JSON/YAML scalars,
maps, and slices, so it can never produce such a value — this affects **only** definitions built by hand in
Go. ADR-0037 recorded this as an accepted limitation; B4 closes it.

## Goal

Turn the silent change-detection loss into a **fail-loud, typed error at the point ruleset identity is
established** — `Build`, which stamps `Hash()` onto the pipeline (`config/build.go:72`). A def that cannot
be canonically hashed is rejected with an inspectable error rather than built and stamped with a
meaningless placeholder identity. Do this **without a breaking signature change** to `Hash()` or
`MatchesRuleset`.

## Decisions

- **D1 — Fail loud in `Build`, at the identity-stamp boundary.** `Build` currently stamps
  `pipe.RulesetIdentity{Hash: d.Hash(), …}` at `build.go:72`, swallowing any marshal error. Change it to
  compute the hash via an **error-returning** internal path and, on a marshal failure, return a
  `*ConfigError` wrapping a new sentinel `ErrUnhashableDef` — so a hand-built non-marshalable def is
  rejected at construction instead of silently mis-stamped. This is the exact point identity is created, so
  every `any`-typed nesting point (`Constants`/`Schema`/`Globals`) is covered by the one whole-struct
  marshal.
- **D2 — Extract the canonical marshal once.** Factor the Version-cleared `json.Marshal` into an unexported
  `(*PipelineDef).canonicalJSON() ([]byte, error)` used by **both** `Hash()` and `Build`. One
  canonicalization, no drift between the fingerprint and the validation.
- **D3 — `Hash() string` signature unchanged (no SemVer break).** `Hash()` keeps its exact behavior for
  direct callers — it still returns the `"{}"` placeholder on a marshal error (it cannot return an error or
  panic on caller input). Its godoc is updated to state that `Build` **rejects** a non-marshalable def, so
  the fail-loud guarantee lives at the construction boundary while `Hash()`/`MatchesRuleset` stay
  source-compatible. Existing hashes are byte-identical (the golden-hash test stays green).
- **D4 — New exported sentinel `ErrUnhashableDef`.** `var ErrUnhashableDef = errors.New("config:
  definition is not canonically hashable (contains a non-JSON-marshalable value)")`, wrapped with the
  underlying `json.Marshal` error so the offending type shows in the message. Exported and
  `errors.Is`-inspectable, per the project's export-sentinels rule — it is the debuggability surface for
  this branch.

## Non-goals

- Changing `Hash()` or `MatchesRuleset` to return an error (breaking) — D3 keeps them compatible; the
  fail-loud check is at `Build`.
- Field-by-field validation or a bespoke "why unmarshalable" report — the wrapped `json.Marshal` error
  already names the failing type; a whole-struct marshal covers every `any` nesting point.
- Rejecting non-marshalable values anywhere other than the identity boundary (e.g. mid-evaluation).
- Making the direct-`Hash()`-without-`Build` path fail loud — it keeps the documented placeholder fallback
  (a caller who bypasses `Build` and hand-hashes owns supplying a marshalable def); this residual is
  recorded in ADR-0045.

## Success criteria / hot-path branches to cover

1. `Build` on a hand-built def with a non-marshalable `Constants` value (an unreferenced `func`/`chan`,
   which reaches the stamp point) → `*ConfigError` wrapping `ErrUnhashableDef` (`errors.Is`).
2. `Build` on a normal (marshalable) def → unchanged: succeeds and stamps the real hash (existing
   `build_test.go` cases stay green).
3. `Hash()` on a non-marshalable hand-built def → still returns the documented placeholder (no panic), and
   is stable across calls.
4. `Hash()`/`MatchesRuleset` on parsed (always-marshalable) defs → unchanged; the pinned golden hash and
   all `hash_test.go` cases stay green.

## Traceability

Backlog: B4. Plan: 020. ADR: 0045 (records the fail-loud-at-Build decision; refines ADR-0037's accepted
limitation). Related: ADR-0037 (ruleset identity & the original fallback), ADR-0040/0015 (`omitempty`
foreach fields — why the whole-struct hash must stay stable).
