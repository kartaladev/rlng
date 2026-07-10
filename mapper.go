package rlng

import (
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/kartaladev/rlng/expr"
)

// MappingTemplate maps an output dot-path to a leaf expression evaluated against
// the final Scope, e.g. {"total": "line.net + line.tax", "info.tag": "tiers.tag"}.
type MappingTemplate map[string]string

// Mapper projects a Scope into a typed R by evaluating each template field and
// decoding the assembled nested map into R.
type Mapper[R any] struct {
	fields []mappedField
}

type mappedField struct {
	path string
	fn   *expr.Function
}

// NewMapper compiles each template field's expression up front. A compile error
// is a *MappingError naming the field. Fields are evaluated in sorted dot-path
// order for determinism.
func NewMapper[R any](tmpl MappingTemplate) (*Mapper[R], error) {
	paths := make([]string, 0, len(tmpl))
	for p := range tmpl {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fields := make([]mappedField, 0, len(paths))
	for _, p := range paths {
		fn, err := expr.NewFunction(p, tmpl[p])
		if err != nil {
			return nil, &MappingError{Field: p, Cause: err}
		}
		fields = append(fields, mappedField{path: p, fn: fn})
	}
	return &Mapper[R]{fields: fields}, nil
}

// Map evaluates each field against scope, assembles a nested map[string]any by
// dot-path, and decodes it into R. Eval and decode errors are *MappingError.
func (m *Mapper[R]) Map(scope map[string]any) (R, error) {
	var zero R
	out := make(map[string]any)
	for _, f := range m.fields {
		v, err := f.fn.Apply(scope)
		if err != nil {
			return zero, &MappingError{Field: f.path, Cause: err}
		}
		setNested(out, f.path, v)
	}

	var r R
	if err := mapstructure.Decode(out, &r); err != nil {
		return zero, &MappingError{Cause: err}
	}
	return r, nil
}

// setNested writes v at a dot-separated path in out, creating intermediate maps.
func setNested(out map[string]any, path string, v any) {
	keys := strings.Split(path, ".")
	m := out
	for _, k := range keys[:len(keys)-1] {
		child, ok := m[k].(map[string]any)
		if !ok {
			child = make(map[string]any)
			m[k] = child
		}
		m = child
	}
	m[keys[len(keys)-1]] = v
}
