// Package rlng is a pure-Go rule and calculation engine built on expr-lang/expr.
//
// An Engine runs a pipe.Pipeline (built programmatically or from the config
// package) against arbitrary input and returns the accumulated map[string]any.
// A TypedEngine[I, R] adds typed I/O: it seeds a Scope from a typed input I and
// projects the final Scope into a typed result R with a Mapper[R], whose
// MappingTemplate maps each output dot-path to an expression evaluated against
// the final Scope. Errors are typed and unwrap to the offending expression or
// field.
package rlng
