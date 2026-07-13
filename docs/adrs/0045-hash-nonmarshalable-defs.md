# ADR-0045 — `Build` rejects non-canonically-hashable definitions

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 020 / Plan 020, graduating backlog item **B4** — the `Hash()` non-marshalable
  silent-fallback limitation recorded in ADR-0037.

## Context

`(*config.PipelineDef).Hash()` fingerprints a ruleset as the SHA-256 of the canonical JSON of the parsed
definition, and on a `json.Marshal` failure silently fell back to hashing `"{}"`. A hand-built
`PipelineDef` can carry a non-marshalable value (`chan`/`func`/`complex`) in any of its `any`-typed fields
(`Constants`, `Schema`, or an `ExprDef.Globals`); every such def then hashed to the **same** placeholder,
so `MatchesRuleset` could false-match and a changed rule went undetected — the exact opposite of `Hash()`'s
change-detection guarantee, and a silent failure in a debuggability-first engine. ADR-0037 recorded this as
an accepted limitation (parse paths can never produce such values, so it is a hand-built-only concern). B4
closes it.

## Decision

**Fail loud at `Build` — the point where ruleset identity is established — without a breaking signature
change to `Hash()`.**

- **`Build` rejects an unhashable def (Spec 020 D1).** `Build` stamps `pipe.RulesetIdentity{Hash: …}` onto
  the pipeline (`config/build.go`). It now computes that hash via an error-returning path and, on a marshal
  failure, returns a `*ConfigError` wrapping the new sentinel `ErrUnhashableDef` instead of stamping a
  placeholder. Because identity is derived from a single whole-struct marshal, every `any`-typed nesting
  point is covered at once — no per-field enumeration.
- **Shared canonicalization (Spec 020 D2).** The Version-cleared marshal is factored into unexported
  `canonicalJSON() ([]byte, error)` and `hashCanonical() (string, error)`, used by both `Hash()` and
  `Build` — one canonicalization, no drift between the fingerprint and the validation.
- **`Hash() string` unchanged (Spec 020 D3).** `Hash()` keeps its signature and its documented placeholder
  fallback for direct callers (it cannot return an error or panic on caller input). Its godoc now states
  that the fail-loud check lives at `Build`. Existing hashes are byte-identical — the pinned golden-hash
  test stays green.
- **New exported sentinel `ErrUnhashableDef` (Spec 020 D4).** `errors.Is`-inspectable, wrapping the
  underlying `json.Marshal` error (which names the offending type). The `ConfigError` is deliberately **not**
  field-scoped: the non-marshalable value can live in any `any`-typed field, so a `Field:` attribution would
  risk misdirecting; the wrapped marshal error carries the concrete type.

## Consequences

- **Fail-loud where it matters.** A hand-built non-marshalable def is now rejected at construction with a
  typed, `errors.Is`-inspectable error, instead of silently acquiring a meaningless identity that could
  false-match on replay. This restores `Hash()`'s change-detection guarantee for every def that goes
  through `Build`.
- **Behavior change, strictly safer, effectively additive.** `Build` previously *succeeded* (stamping the
  placeholder) for such a def and now returns an error. This affects only hand-built defs carrying a
  `chan`/`func`/`complex` — already-nonsensical rulesets that no `Provider` parse path can produce — so it
  turns a silent-wrong into a loud-error with no impact on any real (parsed) ruleset. `Hash()` and
  `MatchesRuleset` signatures are unchanged; no SemVer break to the exported surface.
- **Documented residual.** A caller who calls `Hash()`/`MatchesRuleset` **directly** on a hand-built
  non-marshalable def (bypassing `Build`) still gets the placeholder fallback — `Hash()` cannot fail loud
  without a breaking signature change. This is the narrow, documented boundary: `Build` is where identity
  is established, and a caller who hand-hashes owns supplying a marshalable def.
- **Small surface.** Two unexported helpers and one exported sentinel; no new dependency (stdlib
  `crypto/sha256`, `encoding/json`, `errors`). Every new branch — the reject path and the retained fallback
  — is covered.

## Traceability

Spec: 020 (docs/specs/020-hash-nonmarshalable-defs.md)
Plan: 020 (docs/plans/020-hash-nonmarshalable-defs.md)
Backlog: B4 (docs/BACKLOG.md → Resolved)
Related: ADR-0037 (ruleset identity & the original accepted fallback — refined here), ADR-0040 / Spec 015
(`omitempty` foreach fields — why the whole-struct hash must stay stable).
