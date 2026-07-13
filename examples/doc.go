// Package examples is a guided tour of rlng, not a grab-bag of demos. The 14
// numbered `_test.go` files are meant to be read in order: each starts at the
// simplest concept the file before it needed and builds on it, ending with a
// full typed engine wired from a YAML document. Every file is `package
// examples_test` (blackbox — only the public API is imported) and exports
// nothing; run the whole tour with `go test ./examples/` or read it straight
// through with `go doc -all ./examples`.
//
// The tour climbs one layer at a time:
//
//   - expr (01-04)  — compile and evaluate a single expression, no pipeline.
//   - pipe (05-10)  — wire expressions into stages, decision tables, and a
//     dependency-ordered pipeline; add per-element iteration, explainability,
//     and deterministic time.
//   - config (11-12) — author the same pipelines as declarative YAML/JSON,
//     and give a decision a replayable identity.
//   - rlng (13-14)  — the engine facade that turns a pipeline into a single
//     Evaluate call, untyped and then typed.
//
// Reading order (file → topic):
//
//	01  01_expr_predicates_test.go           expr.Predicate: strict vs. lenient (WithCoerce) truthiness
//	02  02_expr_functions_test.go            expr.Function: value expressions and fallback-on-error
//	03  03_expr_variables_env_test.go        variable defaults (WithGlobals/WithLocals), strict WithEnv, host functions
//	04  04_expr_decimal_test.go              exact-decimal money: decimal(), round vs. roundBank, WithEnv for bare vars
//	05  05_pipe_scope_getters_test.go        pipe.Scope: dot-path Set/Get, strict vs. coercing typed getters
//	06  06_pipe_stages_test.go               SingleExpr (condition gate) and MultiExpr (priority + intra-stage aliasing)
//	07  07_pipe_decision_table_test.go       decision tables: hit policies single/collect/unique/any, WithDefault
//	08  08_pipe_pipeline_concurrency_test.go Pipeline DAG ordering, cycle detection, WithConcurrency determinism
//	09  09_pipe_foreach_test.go              ForEach per-element iteration, Rollup aggregation, nested foreach
//	10  10_pipe_provenance_clock_test.go     WithProvenance explainability (Explain/Lineage), WithClock deterministic time
//	11  11_config_rulesets_test.go           declarative YAML/JSON rulesets, constants, strict schema, Lint
//	12  12_config_replay_test.go             ruleset Hash/identity, Scope JSON persistence, replay-safety checks
//	13  13_engine_untyped_test.go            rlng.Engine: Evaluate/EvaluateScope, NewFromYAML one-call construction
//	14  14_engine_typed_test.go              rlng.TypedEngine + Mapper: typed input/result, decimal narrowing, concurrency options
//
// Each Example function's doc comment states what the feature is, why you'd
// reach for it, and the specific quirk or edge case it demonstrates — the
// code is a real-world scenario (lending, pricing, invoicing, adjudication),
// not foo/bar. It is a test/documentation package: it exports nothing.
package examples
