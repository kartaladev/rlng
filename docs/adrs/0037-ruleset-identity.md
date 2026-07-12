# ADR-0037 — Ruleset identity & decision stamping

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 013 (docs/specs/013-ruleset-identity.md) / Plan 013, deeper-audit finding "zero ruleset identity".

## Context

The engine has strong per-decision provenance — firing rules, lineage, input
echo, per-stage timing — and a `Scope` JSON round-trip for persisting a decision.
But the audit found the engine records *what happened* with nothing binding a
decision to **which ruleset produced it**: no hash, version, or fingerprint. Two
regulated-lending requirements were therefore unanswerable from the engine alone
— "reproduce decision #48213" (which ruleset revision?) and "which policy version
denied this applicant?" (no version on the record). Determinism itself was
already sound, so replay is *possible* if the caller separately hashes and
version-controls the config, but the engine gave no help and a mismatched ruleset
replayed silently against the wrong rules.

## Decision

Add opt-in, additive ruleset identity and stamp it onto the decision record.

- **D1 — Fingerprint = canonical JSON + SHA-256.** `(*config.PipelineDef).Hash()`
  returns the hex SHA-256 of the canonical JSON encoding of the *parsed*
  definition. Hashing the parsed struct (not source bytes) means YAML and
  equivalent JSON hash identically, reordered map keys hash identically
  (`encoding/json` sorts map keys deterministically), and a changed rule or
  expression changes the hash. `ExprDef` has no custom marshaler, so the encoding
  is deterministic without a bespoke canonicalizer. stdlib only
  (`crypto/sha256`), no new dependency.
- **D2 — Version is excluded from the hash.** The content hash proves *what* ran;
  the author `version` names *which release*. They are orthogonal, so `Hash()`
  clears the `Version` field on a copy before marshaling — re-labelling a release
  never changes the content fingerprint. All other fields (stages, constants,
  schema, mapping) are logic and are included.
- **D3 — The programmatic path is caller-supplied.** A compiled `pipe.Pipeline`
  has no stable canonical source to hash across `expr-lang` versions, so only the
  config path computes a `Hash()`. The programmatic path carries caller-supplied
  identity (a version label and/or an externally-computed hash). It is *not* an
  automatically-derived, config-compatible hash — a config-built and code-built
  equivalent ruleset are not expected to match.
- **D4/D5 — Identity lives on `pipe.Pipeline`; `Run` stamps the Scope.** Both
  paths converge on `*pipe.Pipeline` (which `rlng.Engine` holds), and
  `config.Build` (a different package) needs an exported hook, so a comparable
  value `pipe.RulesetIdentity{Hash, Version}` attaches via a chainable
  `func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline` (configure once
  before Run; not safe to call concurrently with Run). `NewPipeline` keeps its
  variadic-over-`Stage` signature. `Pipeline.Run` stamps the identity onto the
  Scope; `Scope.Ruleset() (RulesetIdentity, bool)` exposes it (the zero identity
  never stamps, so an un-configured pipeline is unchanged). `config.Build` always
  sets `Hash` from `def.Hash()`; a `version:` field and the `WithRulesetVersion`
  BuildOption set the label (option beats field).
- **D6 — Scope JSON round-trips ruleset AND firing.** To make the record
  self-describing, the Scope JSON envelope gains `ruleset` and `firing` members
  (both `omitempty`), and `FiringRule` gains snake_case json tags. A reloaded
  Scope then exposes `Ruleset()`, `FiringRule(s)`, and `FiringRulesFor` — inputs
  + firing + provenance + the ruleset that produced them travel together. (This
  closes the firing-serialization gap noted after Spec 012.)
- **D7 — Replay primitive.** `RulesetIdentity` is directly comparable; the
  documented replay-safety check is `candidateDef.Hash() == persisted.Ruleset().Hash`,
  wrapped as `func (d *PipelineDef) MatchesRuleset(id pipe.RulesetIdentity) bool`
  so a replay against the wrong ruleset is a one-call check rather than silent.

## Consequences

- A persisted decision is self-describing and replayable: a caller can detect a
  ruleset mismatch (`MatchesRuleset`) instead of silently replaying against the
  wrong rules, answering the "reproduce decision / which version" questions from
  the engine alone.
- **Additive & backward-compatible (no SemVer break):** `NewPipeline` and
  `Build()` are unchanged; the two new JSON members are `omitempty`, so an
  un-stamped, non-firing Scope serializes byte-identically to before; existing
  decisions without a stamp remain valid.
- **Threat model:** `Hash()` is a plain content **fingerprint**, not a
  tamper-proof signature — it detects accidental drift and mismatched replays,
  not a malicious actor who re-hashes a tampered ruleset. Signing/tamper-proofing
  is explicitly out of scope (a consumer BRMS concern). `Hash()` also assumes
  JSON-marshalable field values; a hand-built `PipelineDef` carrying a non-
  marshalable value (`chan`/`func`) falls back to a stable placeholder hash and
  loses the change-detection guarantee — the parse paths (`ParseYAML`/`ParseJSON`)
  can never produce such values, so this only affects definitions built by hand.
- Ruleset storage, a version registry, effective-dating, and hot-reload remain
  the consumer's concern; the engine provides the identity and the comparison
  primitive only.
