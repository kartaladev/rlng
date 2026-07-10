package expr

import (
	"fmt"
	"reflect"
)

// toEnv normalizes an evaluation environment to a map[string]any. A nil env
// becomes an empty map; a map[string]any is returned unchanged; a struct or
// pointer-to-struct is converted field-by-field. Any other kind is an error.
func toEnv(env any) (map[string]any, error) {
	if env == nil {
		return map[string]any{}, nil
	}
	if m, ok := env.(map[string]any); ok {
		return m, nil
	}

	rv := reflect.ValueOf(env)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return map[string]any{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("env must be map[string]any or struct, got %T", env)
	}
	return structToMap(rv), nil
}

func structToMap(v reflect.Value) map[string]any {
	out := make(map[string]any, v.NumField())
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}
		out[t.Field(i).Name] = convertValue(v.Field(i))
	}
	return out
}

func convertValue(v reflect.Value) any {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		return structToMap(v)
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = convertValue(v.Index(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = convertValue(iter.Value())
		}
		return out
	default:
		return v.Interface()
	}
}
