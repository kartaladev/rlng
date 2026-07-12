package expr

import (
	"fmt"
	"reflect"
)

// maxEnvDepth bounds struct/map/slice reflection in toEnv against a cyclic or
// pathologically deep env, which would otherwise recurse to an unrecoverable
// fatal stack overflow. It mirrors provenance's lineage-depth guard.
const maxEnvDepth = 1000

// ErrEnvTooDeep is returned (wrapped in an EvalError) when an env's struct/map/
// slice nesting exceeds maxEnvDepth, e.g. a struct with a self-referential
// pointer. It is a bounded error rather than a process-crashing stack overflow.
var ErrEnvTooDeep = fmt.Errorf("env exceeds max nesting depth %d (possible cyclic reference)", maxEnvDepth)

// toEnv normalizes an evaluation environment to a map[string]any. A nil env
// becomes an empty map; a map[string]any is returned unchanged; a struct or
// pointer-to-struct is converted field-by-field. Any other kind is an error, as
// is an env whose nesting exceeds maxEnvDepth (ErrEnvTooDeep).
func toEnv(env any) (map[string]any, error) {
	if env == nil {
		return map[string]any{}, nil
	}
	if m, ok := env.(map[string]any); ok {
		return m, nil
	}

	rv := reflect.ValueOf(env)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return map[string]any{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("env must be map[string]any or struct, got %T", env)
	}
	return structToMap(rv, 0)
}

// structToMap converts a struct's exported fields. It does not self-check depth:
// every field is routed through convertValue (which does), so convertValue is
// always the deeper call and bounds the recursion before structToMap recurses.
func structToMap(v reflect.Value, depth int) (map[string]any, error) {
	out := make(map[string]any, v.NumField())
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}
		cv, err := convertValue(v.Field(i), depth+1)
		if err != nil {
			return nil, err
		}
		out[t.Field(i).Name] = cv
	}
	return out, nil
}

func convertValue(v reflect.Value, depth int) (any, error) {
	if depth > maxEnvDepth {
		return nil, ErrEnvTooDeep
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		// A struct with no exported fields (e.g. time.Time) can never become a
		// useful map, so pass the real value through instead of flattening it
		// to map[string]any{} and silently losing it. This lets expr call its
		// methods (e.g. CreatedAt.Year(), CreatedAt.Before(x)).
		if !hasExportedFields(v.Type()) {
			return v.Interface(), nil
		}
		return structToMap(v, depth+1)
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			cv, err := convertValue(v.Index(i), depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return out, nil
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			cv, err := convertValue(iter.Value(), depth+1)
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(iter.Key().Interface())] = cv
		}
		return out, nil
	default:
		return v.Interface(), nil
	}
}

// hasExportedFields reports whether t (which must be a struct type) declares
// at least one exported field.
func hasExportedFields(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() {
			return true
		}
	}
	return false
}
