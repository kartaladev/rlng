# Spec 011 — Config-path safety parity

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-010 audit remediation.** Driven by a second, deeper business-rule
  production-readiness audit of the merged library. Findings are embedded inline
  (no standalone audit record).
- **Builds on:** Spec 004 (`config`), Spec 010 (`WithEnv`, lint, config surface),
  Spec 001 (`expr`).
- **Realized by:** Plan 011.
- **Anticipated ADRs:** ADR-0031 (config strict-env schema surface),
  ADR-0032 (strict object-form config decoding), ADR-0033 (lint enforcement in Build).

## Context — the config path is a second-class citizen

Spec 010 added the library's key correctness guards (strict typed env, ruleset
lint, strict decoding), but the audit found they are reachable only from the
**programmatic Go API**, not from the **config/YAML path** — which is the
deployment shape the library actively markets ("a complete decision service
authorable as one YAML document"). A file-authored ruleset is therefore
materially less safe than a hand-built one. Three findings share this root cause:

1. **Strict typed env unreachable from config.** `config/build.go` never wires
   `expr.WithEnv`; every config-built stage compiles with `AllowUndefinedVariables`
   (lenient). A field typo in YAML (`annual_incom`) silently evaluates to `nil`
   and the rule misfires — the exact class of bug ADR-0023 exists to catch.
   *(Verified: no `WithEnv`/`WithSchema`/schema plumbing anywhere in `config/`.)*
2. **Strict decoding silently bypassed inside the expression object form.**
   `PipelineDef`/`StageDef` reject unknown keys (`parse.go` uses
   `KnownFields`/`DisallowUnknownFields`), but `ExprDef.UnmarshalYAML`
   (`config/expr_def.go:34`) calls `value.Decode`, which starts a fresh yaml
   decoder that does **not** inherit `KnownFields(true)`, and
   `ExprDef.UnmarshalJSON` (`config/expr_def.go:53`) calls a plain
   `json.Unmarshal` with no `DisallowUnknownFields`. A typo'd `fallbck:`,
   `globals:`, or `coerc:` on any expression/condition/decision is silently
   dropped — and the object form is the *only* place a mistyped compile-option
   can occur.
3. **Lint is never wired into `Build`.** `missing-default` (an unmatched input
   silently produces no output) and `unreachable-rule` surface only if the
   consumer independently calls `(*PipelineDef).Lint()`. A config with a
   first-match table and no default builds clean. Separately, lint's catch-all
   detection is purely syntactic (`isCatchAll` matches the literal `true` only,
   `config/lint.go:103`), so a semantic catch-all such as `1 == 1` produces a
   false `missing-default` and hides genuinely unreachable later rules.
4. **Config build-error misattribution.** When a single-expr stage's *value*
   expression fails to compile, `buildSingle` (`config/build.go:82-92`) can
   re-report the failure against the `condition` field, pointing the author at
   the wrong expression — a debuggability regression in the layer whose whole
   purpose is precise attribution.

## Goals

1. **G1 — Strict typed env from config (ADR-0031).** Add a config surface that
   lets a ruleset opt into strict compilation: a top-level `schema` (or `env`)
   block declaring the input shape, wired through `Build` into `expr.WithEnv` for
   every stage, with declared `constants`/`globals` and registered functions
   merged into the type-check env so they stay usable. Opt-in (lenient stays the
   default for backward compatibility). A `config.WithStrict()`/`BuildStrict`
   build option toggles it programmatically for callers who cannot edit the
   document. Unknown identifiers and type errors become **build-time** errors
   naming the offending stage/field/expression.
2. **G2 — Honest strict decoding in the expression object form (ADR-0032).**
   `ExprDef` object-form unmarshaling rejects unknown keys in both YAML (decode
   via a `KnownFields(true)` decoder or validate the mapping node's keys) and
   JSON (`json.Decoder` + `DisallowUnknownFields`). The advertised
   "unknown keys are rejected" contract then holds uniformly, including for
   compile-option typos.
3. **G3 — Lint enforcement in Build (ADR-0033).** A `config.WithLint(level)` /
   `BuildStrict` option runs `Lint` during `Build` and promotes findings (or a
   selected severity) to construction errors, so a coverage gap fails fast
   instead of silently. Default remains advisory (non-breaking). Additionally,
   make lint honest about its heuristic: either recognize a small set of
   semantic catch-alls (`1 == 1`, parenthesized `true`) or narrow the godoc and
   finding messages to state the detection is best-effort/syntactic — no false
   authority.
4. **G4 — Correct build-error attribution.** Attribute a single-expr build
   failure to the field that actually failed to compile (thread the failing
   expression through the error rather than guessing `condition`), and avoid the
   redundant double-compile of the predicate on the failure path.

## Non-goals

- Changing the default leniency of the programmatic `expr` API (011 only adds the
  *config* on-ramp to strictness).
- Runtime type validation of input against the schema (this is compile-time
  identifier/type checking of expressions, not input-shape enforcement).
- Content-type sniffing of config files (dispatch stays extension-based; a doc
  note suffices).

## Hot-path branches (test targets)

- Strict-env config: a declared-schema document rejects an unknown identifier at
  `Build` (naming stage/field/expression); accepts declared identifiers; merges
  constants/globals/functions; the lenient (no-schema) path is unchanged;
  `WithStrict()` programmatic toggle both on and off.
- Object-form strict decoding: unknown key inside an `ExprDef` object rejected in
  **both** YAML and JSON; a valid object form still parses; scalar-shorthand
  unaffected.
- Lint-in-Build: `WithLint` promotes `missing-default` and `unreachable-rule` to
  a build error; default `Build` stays advisory (no error); a clean table yields
  no findings; a semantic catch-all (`1 == 1`) no longer false-flags
  `missing-default` (or is documented as best-effort with the test asserting the
  documented behavior).
- Attribution: a broken value expression is reported against the value field, not
  `condition`; a broken condition (value ok) is reported against `condition`;
  both broken reports the first failure deterministically.
