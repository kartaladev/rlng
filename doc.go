// Package rlng is a pure-Go rule and calculation engine built on expr-lang/expr.
//
// An Engine[I, R] seeds a Scope from a typed input I, runs a stage.Pipeline
// (build one programmatically or from the config package), and projects the
// final Scope into a typed result R with a Mapper[R]. A MappingTemplate maps
// each output dot-path to an expression evaluated against the final Scope.
// Errors are typed and unwrap to the offending expression or field.
package rlng
