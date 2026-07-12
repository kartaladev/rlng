# ADR-0032 — Strict decoding of the expression object form

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 011 (docs/specs/011-config-path-safety-parity.md) / Plan 011, deeper-audit finding (config HIGH).

## Context

The config parsers advertise strict decoding: `ParseYAML` sets `KnownFields(true)`
and `ParseJSON` sets `DisallowUnknownFields()`, so a misspelled field is a clear
error rather than a silently-dropped key. The deeper audit found this held for
`PipelineDef`/`StageDef` but **not** for `ExprDef` (an expression's object form).
`ExprDef.UnmarshalYAML` called `value.Decode`, which starts a fresh yaml decoder
that does not inherit `KnownFields`; `ExprDef.UnmarshalJSON` called plain
`json.Unmarshal` with no `DisallowUnknownFields`. A typo'd `fallbck:`/`globals:`/
`coerc:` on any expression, condition, or decision was silently ignored — and the
object form is the *only* place a mistyped compile-option can occur.

## Decision

Make the `ExprDef` object form reject unknown keys in both formats. The pinned
`gopkg.in/yaml.v3` (v3.0.1) lacks a `Node`-level `KnownFields` option, so the
YAML path validates the mapping node's keys against the known set
(`expr`, `fallback`, `globals`, `coerce`) before decoding; the JSON path uses a
`json.Decoder` with `DisallowUnknownFields()` plus a pre-decode key probe so an
unknown key yields a typed `*ConfigError{Field: "expr"}` rather than a bare
stdlib error.

Additionally, `ParseYAML`/`ParseJSON` no longer re-wrap an error that is already
a `*ConfigError`: previously the outer `&ConfigError{Cause: err}` wrap meant
`errors.As` matched the outer error (`Field == ""`), masking the inner field
attribution. They now return an existing `*ConfigError` as-is, so the offending
field stays reachable — the debuggability property this spec exists to deliver.

## Consequences

- The "unknown keys are rejected" contract now holds uniformly, including for
  compile-option typos — the exact class of bug strict decoding is meant to catch.
- `ConfigError.Field` attribution survives the parse layer, so callers can
  `errors.As` to the offending field, not just read it from the message string.
- The YAML known-key set must be kept in sync with `ExprDef`'s fields; a
  test asserts the set matches the struct so a future field addition is caught.
- The JSON pre-decode probe double-parses a valid object once (config-time only,
  negligible); an acceptable cost for a typed, attributed error.
