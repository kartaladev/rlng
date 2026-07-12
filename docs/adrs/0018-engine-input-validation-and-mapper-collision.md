# ADR-0018 — Engine input validation and mapper path collision

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 008 (docs/specs/008-production-hardening.md), audit findings M3, M6, R5.

## Context

- **M6** — `flatten` accepts a nil input (nil pointer type parameter, or a nil
  interface). `Engine.Evaluate(ctx, nil)` then runs over an empty Scope and,
  because expressions allow undefined variables, returns a zero/garbage result
  with `err == nil` — a bogus success.
- **M3** — `Mapper.setNested` silently overwrites a mapped field when output
  dot-paths collide (a leaf key and a deeper key sharing its prefix, e.g.
  `{"a": …, "a.b": …}`), dropping a value with no error — unlike `Scope.Set`,
  which guards the same situation with `ErrPathNotMap`.
- **R5** — `MappingError` with `Field == ""` renders identically for a real
  final-decode failure and for a (mis)configured empty-string field key.

## Decision

- **Reject nil input (M6).** `flatten` returns a typed error (`rlng: nil input`)
  when the input is a nil pointer or a nil interface value. A non-nil empty map
  remains a valid (empty) seed.
- **Report collisions (M3).** `setNested` returns an error when a path segment is
  already occupied by a non-map (or a leaf collides with an existing map); `Map`
  surfaces it as a `*MappingError` naming the field, matching `Scope.Set`'s
  guard.
- **Disambiguate the decode error (R5).** The final-decode `MappingError` is
  distinguished from a field error (e.g. a dedicated marker/sentence), so the two
  are not confused. `NewMapper` also rejects an empty-string template key up
  front.

## Consequences

- `Evaluate(nil)` and colliding mapping templates now fail with precise typed
  errors instead of silent wrong results.
- Callers passing an intentional nil (expecting empty behavior) must pass an
  explicit empty map instead.
