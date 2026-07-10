// Package config parses declarative YAML/JSON pipeline definitions and builds
// stage.Pipeline values from them.
//
// A definition is an ordered list of stages; each stage names its type
// (single-expr, multi-expr, or decision-table), its dependencies, and its
// type-specific fields. Expression fields accept either a bare string (the
// expression) or an object with compile options (expr, fallback, globals,
// coerce). Parse with ParseYAML, ParseJSON, or LoadFile, then call Build to
// compile the definition into a *stage.Pipeline.
package config
