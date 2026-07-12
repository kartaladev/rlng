package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kartaladev/rlng/expr"
	"gopkg.in/yaml.v3"
)

// ExprDef is an expression with optional compile options. It decodes from
// either a scalar string (shorthand: the string is Expr) or a mapping with
// explicit fields. In the shorthand form the expression must be a string:
// YAML accepts an unquoted scalar (e.g. expr: price * 2), but JSON requires a
// quoted string (e.g. "expr": "price * 2") — a bare JSON number or boolean is
// rejected.
type ExprDef struct {
	Expr     string         `yaml:"expr" json:"expr"`
	Fallback string         `yaml:"fallback" json:"fallback"`
	Globals  map[string]any `yaml:"globals" json:"globals"`
	Coerce   bool           `yaml:"coerce" json:"coerce"`
}

// UnmarshalYAML accepts a scalar (the expression) or a mapping (explicit fields).
func (e *ExprDef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		e.Expr = value.Value
		return nil
	case yaml.MappingNode:
		// Validate known fields before decoding
		known := map[string]bool{"expr": true, "fallback": true, "globals": true, "coerce": true}
		for i := 0; i < len(value.Content); i += 2 {
			if k := value.Content[i].Value; !known[k] {
				return &ConfigError{Field: "expr", Cause: fmt.Errorf("unknown field %q", k)}
			}
		}
		type raw ExprDef // alias breaks the UnmarshalYAML recursion
		var r raw
		if err := value.Decode(&r); err != nil {
			return err
		}
		*e = ExprDef(r)
		return nil
	default:
		return &ConfigError{Field: "expr", Cause: fmt.Errorf("expected a scalar or mapping, got yaml kind %d", value.Kind)}
	}
}

// UnmarshalJSON accepts a JSON string (the expression) or an object. Callers
// reach this only with well-formed JSON (encoding/json validates syntax before
// invoking an Unmarshaler), so a string form is decoded straight into Expr.
func (e *ExprDef) UnmarshalJSON(data []byte) error {
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '"' {
		return json.Unmarshal(data, &e.Expr)
	}
	type raw ExprDef // alias breaks the UnmarshalJSON recursion
	var r raw
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return err
	}
	*e = ExprDef(r)
	return nil
}

// hasOptions reports whether the object form carries any compile option, without
// allocating an option slice.
func (e ExprDef) hasOptions() bool {
	return e.Fallback != "" || len(e.Globals) > 0 || e.Coerce
}

// options maps the object form to expr.Option values.
func (e ExprDef) options() []expr.Option {
	var opts []expr.Option
	if e.Fallback != "" {
		opts = append(opts, expr.WithFallback(e.Fallback))
	}
	if len(e.Globals) > 0 {
		opts = append(opts, expr.WithGlobals(e.Globals))
	}
	if e.Coerce {
		opts = append(opts, expr.WithCoerce())
	}
	return opts
}
