# ADR-0010 — mapstructure for input/result struct↔map decoding

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 005 (docs/specs/005-engine-facade.md)

## Context

The `Engine[I, R]` facade must convert between typed values and the
`map[string]any` world the evaluation stack uses: flatten a typed input `I` into
the `Scope` seed, and decode the assembled result map into a typed `R`. Three
options were considered: hand-rolled reflection, an `encoding/json` round-trip
(`Marshal` then `Unmarshal` into the other shape), and the
`github.com/go-viper/mapstructure/v2` library named in the calc blueprint.

## Decision

Add **`github.com/go-viper/mapstructure/v2`** and use it for both directions
(input struct→map, result map→struct).

- **Over an `encoding/json` round-trip:** a JSON round-trip is zero-dependency
  but **loses numeric type fidelity** — every number decoded from JSON into an
  `any` becomes `float64`. In a rule/calculation engine that matters: `expr`
  integer operations (notably `%`, which errors on non-integers, and integer vs
  float division) behave differently for `int` than `float64`. mapstructure
  preserves the original Go types when flattening a struct into the Scope seed,
  so `Qty int` stays an `int` in the expression environment. Faithful types are
  a correctness/debuggability property, which the project prioritizes over
  shaving one dependency.
- **Over hand-rolled reflection:** decoding an arbitrary result map into an
  arbitrary generic `R` (nested structs, tags, pointers, type coercion) is
  exactly what mapstructure is built for; re-implementing it by reflection would
  be error-prone and is not the library's value-add.
- **Scope of use:** input flattening (`mapstructure.Decode(input, &m)`), and
  result decoding (`mapstructure.Decode(assembledMap, &r)`). A `map[string]any`
  input bypasses mapstructure entirely (used directly as the seed).
- Struct field mapping uses mapstructure's default behavior (the `mapstructure`
  tag, else case-insensitive field name); this is documented on the facade.

## Consequences

- Consumer-visible dependency set after this increment: `expr-lang/expr`,
  `gopkg.in/yaml.v3` (in `config` only), and `go-viper/mapstructure/v2` (in root
  `rlng` only). `go mod tidy` updates `go.mod`/`go.sum` once; `go mod verify`
  must pass. mapstructure is pure Go, widely used, and cgo-free — consistent
  with the no-cgo / cross-compilable gate.
- Input/result structs are annotated with `mapstructure` tags (or rely on field
  names). If json-tag compatibility is later wanted, mapstructure's `TagName`
  can be configured additively without an API break.
- Numeric type fidelity is preserved end to end (input → Scope → expression →
  result), avoiding a class of subtle float-vs-int surprises.
- If a future need arises to drop the dependency, the two `Decode` call sites are
  the only coupling; a superseding ADR would replace them.
