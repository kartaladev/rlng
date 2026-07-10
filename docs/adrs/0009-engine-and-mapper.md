# ADR-0009 — `Engine[I, R]` facade, `Mapper[R]`, and root-package placement

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 005 (docs/specs/005-engine-facade.md)

## Context

Increment 5 is the final increment: the public facade a consumer imports. It
ties the evaluation stack (Increments 1–4) into a single generic entry point
that takes a typed input, runs a pipeline, and returns a typed result. Several
shape decisions needed recording: where the facade lives, its generic signature,
how the result is projected, and how it relates to `config`. (The new
dependency is isolated in ADR-0010; the `Engine`/`Evaluate` name was already
settled in ADR-0001 and is merely realized here.)

## Decision

1. **Root `rlng` package.** The facade lives in the module-root package `rlng`
   (`engine.go`, `mapper.go`, `errors.go`) — the package a consumer gets from
   `import "github.com/kartaladev/rlng"`. This is why the root was kept empty
   through Increments 1–4.

2. **`Engine[I any, R any]`, constructed from a built pipeline.** `New[I, R](
   pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option) *Engine[I, R]`;
   `Evaluate(ctx, input I) (R, error)`. The engine is composed from an
   **already-built `*stage.Pipeline`**, not from config bytes — so the root
   package does **not** import `config`. `config` and `rlng` are siblings over
   `stage`; a consumer wires `config.ParseYAML(data).Build()` and passes the
   result to `rlng.New`. This keeps the dependency graph acyclic and lets a
   consumer build a pipeline programmatically or declaratively with the same
   facade. The `Engine`/`Evaluate` naming realizes ADR-0001 (neutral, serves
   calculations *and* rules).

3. **`Mapper[R]` + `MappingTemplate` — output fields are expressions.** The
   result projection is not a plain struct copy: a `MappingTemplate`
   (`map[string]string`) maps an **output dot-path** to a **leaf expression**
   evaluated against the final `Scope`, so the result can compute over pipeline
   outputs (`{"total": "line.net + line.tax"}`). `NewMapper` compiles each
   expression once (reusing `expr.Function`); `Map` evaluates them, assembles a
   nested `map[string]any`, and decodes it into `R`. This mirrors the calc
   blueprint's `MappingTemplate` and keeps evaluation on the hot path cheap.

4. **Typed `MappingError{Field, Cause}`** for compile/eval/decode failures in the
   mapper, naming the output field and unwrapping to the underlying `expr`/decode
   error — extending the debuggability chain to the last layer. Pipeline errors
   pass through `Evaluate` unwrapped (already typed).

5. **Concurrency-safe after construction.** `Engine` holds no mutable state once
   built (the pipeline and mapper are immutable; each `Evaluate` creates its own
   `Scope`), so distinct `Evaluate` calls run concurrently. Execution remains
   sequential within a call (ADR-0006).

## Consequences

- The public surface is small and stable: `Engine`, `New`, `Evaluate`,
  `Mapper`, `NewMapper`, `MappingTemplate`, `Option`/`WithScopeOptions`,
  `MappingError`. "Accept a concrete pipeline, return structs."
- `rlng` does not depend on `config`; the declarative and programmatic paths
  converge at `*stage.Pipeline`. A future convenience (e.g. `rlng.NewFromYAML`)
  can be added additively if desired — deliberately deferred (YAGNI).
- Output mapping is programmatic (Go `MappingTemplate`), not yet config-declared;
  loading a mapping from YAML/JSON is a backlog item. The declarative story is
  therefore complete for the pipeline but not yet for the result projection.
- Generic `Engine[I, R]` gives compile-time-typed inputs/outputs; the input and
  result decoding is handled by mapstructure (ADR-0010).
