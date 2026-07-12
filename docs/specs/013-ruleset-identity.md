# Spec 013 — Ruleset identity & decision stamping

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-010 audit remediation.** Findings embedded inline (no standalone audit
  record).
- **Builds on:** Spec 003 (`Pipeline`), Spec 004 (`config`), Spec 006/007
  (provenance, Scope JSON serialization), Spec 010 (rule identity & firing).
- **Realized by:** Plan 013.
- **Anticipated ADRs:** ADR-0037 (ruleset identity & decision stamping).
- **Relates to:** Spec 014 (a decision record is only replayable if both its
  ruleset identity *and* its values round-trip losslessly).

## Context

The engine has strong per-decision provenance — firing rules, lineage, input
echo, per-stage timing — and a Scope JSON round-trip for persisting a decision.
But the audit found **zero ruleset identity** anywhere (no hash, version, or
fingerprint). A persisted decision records *what happened* but nothing binds it
to **which ruleset produced it**. That leaves two regulated-lending requirements
unanswerable from the engine alone:

- *"Reproduce decision #48213"* — you cannot know which ruleset revision to load.
- *"Which policy version denied this applicant?"* — the decision carries no
  version.

Determinism itself is sound (Spec 010 / audit: sorted-key decisions, ordered
rules, opt-in `now()`, IEEE-754-deterministic float), so replay is *possible* if
the caller separately version-controls and hashes the config — but the engine
gives no help, and a mismatched ruleset replays silently against the wrong rules.

## Goals

1. **G1 — Deterministic ruleset fingerprint.** Add a stable content hash over a
   ruleset's canonical form. For the config path this is a `Definition.Hash()`
   (or `PipelineDef.Hash()`) computed from a canonical serialization of the
   definition (stable key ordering, normalized whitespace) so the same logical
   ruleset always hashes identically regardless of YAML/JSON formatting. Decide
   and record (ADR-0037) whether the programmatic-`Pipeline` path can offer an
   equivalent identity or is documented as caller-supplied, since a compiled
   `Pipeline` has no source text to hash.
2. **G2 — Optional author version.** Allow a ruleset to carry an author-declared
   `version`/`id` (a config field and/or a `Pipeline`/`Engine` construction
   option), distinct from the content hash: the hash proves *what* ran, the
   version names *which release* it was.
3. **G3 — Stamp identity onto the decision record.** Record the ruleset hash (and
   version, when set) on the `Scope`, exposed via an accessor (e.g.
   `Scope.Ruleset()` → `{Hash, Version}`) and included in the Scope JSON
   serialization, so a persisted decision is self-describing: inputs + firing
   rules + provenance + **the ruleset that produced them**.
4. **G4 — Replay safety hook.** Provide the means for a caller to verify a
   reloaded ruleset matches a persisted decision's stamp (compare hashes) so a
   replay against the wrong ruleset is detectable rather than silent. The engine
   provides the identity and the comparison primitive; the caller owns storage
   and the version→artifact mapping (BRMS concern, out of scope).

## Non-goals

- Ruleset storage, a version registry, effective-dating, or hot-reload — BRMS
  concerns the consumer builds around `rlng`.
- Signing/tamper-proofing the hash (a plain content fingerprint, not a security
  signature; note the threat model in the ADR).
- Making identity mandatory — it is opt-in and additive; existing decisions
  without a stamp remain valid.

## Hot-path branches (test targets)

- Hash stability: the same definition in YAML and in equivalent JSON hashes
  identically; reordered but semantically identical keys hash identically (or the
  chosen canonicalization is asserted); a changed rule/expression changes the hash.
- Version: author `version` parsed from config / set via option; absent version
  is an empty/optional field, not an error.
- Stamping: `Scope.Ruleset()` returns the hash (+version) after a run; it appears
  in the Scope JSON round-trip and survives reload.
- Replay: a reloaded matching ruleset compares equal; a mismatched ruleset
  compares unequal (detectable), across the JSON round-trip.
