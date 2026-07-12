# Spec 013 — Ruleset identity & decision stamping

- **Status:** Accepted (design resolved 2026-07-12; realized by Plan 013)
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

## Resolved design decisions (brainstormed 2026-07-12)

These concretize the open questions in G1–G4 and set the scope Plan 013 realizes.

- **D1 — Fingerprint = canonical JSON + SHA-256.** `(*config.PipelineDef).Hash()`
  returns the hex SHA-256 of the canonical JSON encoding of the *parsed*
  `PipelineDef`. Because it hashes the parsed struct (not source bytes), YAML and
  equivalent JSON hash identically, reordered map keys hash identically
  (`encoding/json` sorts map keys), and rule-slice order stays semantic. stdlib
  only (`crypto/sha256`), no new dependency. Implementation must confirm `ExprDef`
  marshals deterministically.
- **D2 — `version` is excluded from the hash.** The content hash proves *what*
  ran; the author `version` names *which release*. They are orthogonal, so the
  new `PipelineDef.Version` field is omitted from the hashed canonical form —
  changing the version label must not change the content fingerprint. All other
  fields (stages, constants, schema, mapping) are logic and are included.
- **D3 — Programmatic path is caller-supplied.** A compiled `pipe.Pipeline` has no
  stable canonical source to hash across `expr-lang` versions, so only the config
  path computes a `Hash()`. The programmatic path carries caller-supplied identity
  (version and/or an externally-computed hash). Documented in ADR-0037.
- **D4 — Identity lives on `pipe.Pipeline`; `Pipeline.Run` stamps the Scope.**
  Both paths converge on `*pipe.Pipeline` (which `rlng.Engine` holds), and
  `config.Build` (a different package) needs an exported hook, so identity attaches
  via a chainable `func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline`
  (configure-once-before-Run; not safe to call concurrently with Run). `NewPipeline`
  keeps its variadic-over-`Stage` signature unchanged. `config.WithRulesetVersion`
  is a new `BuildOption`; `Build` always sets `Hash` from `def.Hash()`.
- **D5 — `RulesetIdentity` is a `pipe` value type.** `type RulesetIdentity struct {
  Hash, Version string }`, directly comparable. `Scope.Ruleset() (RulesetIdentity,
  bool)` returns the stamp after a run.
- **D6 — Scope JSON round-trips ruleset AND firing (expanded scope).** To realize
  G3's "self-describing" record, Plan 013 also closes the firing-serialization gap:
  `scopeJSON` gains `ruleset` and `firing` fields, and `FiringRule` gains snake_case
  json tags. A reloaded Scope then exposes `Ruleset()`, `FiringRule(s)`, and
  `FiringRulesFor` — inputs + firing + provenance + identity.
- **D7 — Replay primitive.** `RulesetIdentity` is comparable; the documented check
  is `candidateDef.Hash() == persisted.Ruleset().Hash`, with a thin helper
  `func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool`.

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
