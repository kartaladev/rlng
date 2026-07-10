# ADR-0008 — Config dependency choices: `yaml.v3` only, no `mimetype`/`validator`

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 004 (docs/specs/004-declarative-config.md)

## Context

The `pkg/calc` reference blueprint (CLAUDE.md §Likely dependencies) loads config
with `gopkg.in/yaml.v3`, sniffs content type with `gabriel-vasile/mimetype`, and
validates decoded structs with `go-playground/validator/v10`. `rlng` is a
library, so every direct dependency becomes a transitive dependency for every
consumer; the Library quality gates require justifying each addition and keeping
the set minimal. Increment 4 is the increment that first needs to read config,
so the dependency line-up is settled here.

## Decision

Add **exactly one** new consumer-visible dependency — `gopkg.in/yaml.v3` — and
use the standard library for everything else.

- **YAML: `gopkg.in/yaml.v3`.** Required — the standard library has no YAML
  decoder, and its `yaml.Node` API is what makes the `ExprDef` scalar-shorthand
  `UnmarshalYAML` clean. This is the sole justified addition.
- **JSON: `encoding/json` (stdlib).** No dependency needed.
- **No `gabriel-vasile/mimetype`.** Content sniffing to tell YAML from JSON is
  unsound: every JSON document is also valid YAML, so a sniffer cannot reliably
  distinguish them, and byte-signature detection is fragile for text formats.
  `LoadFile` instead dispatches on the **file extension** (`.yaml`/`.yml` →
  YAML, `.json` → JSON), which is deterministic and needs no dependency. Callers
  with in-memory bytes pick the format explicitly via `ParseYAML`/`ParseJSON`.
- **No `go-playground/validator/v10`.** The decoded definition needs only a
  handful of shape checks (unknown `type`, required-field presence, valid
  `hit_policy`), and the real validation — expression compilation, empty-name
  rejection, non-empty rule/expr sets — is already done by the `stage`/`expr`
  constructors with typed, field-naming errors. Inline checks that produce a
  `ConfigError` naming the exact stage/field give *better*-located, more
  debuggable errors than reflection-driven struct-tag validation, and avoid the
  dependency (and its transitive `reflect`-tag ecosystem) entirely. This aligns
  with the project's debuggability-first criterion.

## Consequences

- Consumer-visible dependency set after this increment: `expr-lang/expr` and
  `gopkg.in/yaml.v3` (two), plus test-only `testify`. `go mod tidy` legitimately
  updates `go.mod`/`go.sum` once (adding `yaml.v3`); thereafter it is a no-op,
  and `go mod verify` must pass.
- Format selection is explicit (bytes) or extension-based (files) — no "magic"
  detection to mis-fire. A file with an unrecognized extension is a clear
  `ConfigError`, not a wrong-decoder guess.
- Validation logic lives in `config`'s `Build` (and the reused constructors),
  not in struct tags; adding a new shape rule is Go code with a typed error, not
  a tag incantation. The trade-off is that shape checks are hand-written rather
  than declarative — acceptable given how few there are.
- If a future increment genuinely needs richer declarative validation or true
  content negotiation, adding a dependency then is an ADR that supersedes the
  relevant point here — deferred under YAGNI until a concrete need exists.
