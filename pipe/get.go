package pipe

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPathNotFound is returned by the typed getters when a path is absent.
var ErrPathNotFound = errors.New("scope: path not found")

// ScopeTypeError reports a typed getter finding a value of the wrong type.
type ScopeTypeError struct{ Path, Expected, Actual string }

func (e *ScopeTypeError) Error() string {
	return fmt.Sprintf("scope: path %q: expected %s, got %s", e.Path, e.Expected, e.Actual)
}

// GetAs returns the value at path as T. It returns ErrPathNotFound if the path is
// absent and a *ScopeTypeError if the value is not a T. Strict: no coercion.
func GetAs[T any](s *Scope, path string) (T, error) {
	var zero T
	v, ok := s.Get(path)
	if !ok {
		return zero, ErrPathNotFound
	}
	t, ok := v.(T)
	if !ok {
		return zero, &ScopeTypeError{Path: path, Expected: fmt.Sprintf("%T", zero), Actual: fmt.Sprintf("%T", v)}
	}
	return t, nil
}

// GetInt returns the value at path as an int, or an error. A json.Number
// (produced by a legacy, pre-014 JSON round-trip) or an int64 (produced by a
// v2 Scope JSON round-trip — see ADR-0038, which canonicalizes every sized
// integer to the int64 kind) is read losslessly; a non-integer or one that
// overflows int is a *ScopeTypeError.
func (s *Scope) GetInt(path string) (int, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		// Unreachable on 64-bit platforms (int == int64); guards 32-bit builds.
		if int64(int(n)) != n {
			return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: fmt.Sprintf("int64(%d) overflows int", n)}
		}
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: "json.Number(" + n.String() + ")"}
		}
		// Unreachable on 64-bit platforms (int == int64); guards 32-bit builds.
		if int64(int(i)) != i {
			return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: "json.Number(" + n.String() + ") overflows int"}
		}
		return int(i), nil
	default:
		return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: fmt.Sprintf("%T", v)}
	}
}

// GetInt64 returns the value at path as an int64, or an error. A json.Number is
// read losslessly; a non-integer json.Number is a *ScopeTypeError.
func (s *Scope) GetInt64(path string) (int64, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	switch n := v.(type) {
	case int64:
		return n, nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, &ScopeTypeError{Path: path, Expected: "int64", Actual: "json.Number(" + n.String() + ")"}
		}
		return i, nil
	default:
		return 0, &ScopeTypeError{Path: path, Expected: "int64", Actual: fmt.Sprintf("%T", v)}
	}
}

// GetFloat64 returns the value at path as a float64, or an error. A json.Number
// is read losslessly.
func (s *Scope) GetFloat64(path string) (float64, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, &ScopeTypeError{Path: path, Expected: "float64", Actual: "json.Number(" + n.String() + ")"}
		}
		return f, nil
	default:
		return 0, &ScopeTypeError{Path: path, Expected: "float64", Actual: fmt.Sprintf("%T", v)}
	}
}

// GetString returns the value at path as a string, or an error.
func (s *Scope) GetString(path string) (string, error) { return GetAs[string](s, path) }

// GetBool returns the value at path as a bool, or an error.
func (s *Scope) GetBool(path string) (bool, error) { return GetAs[bool](s, path) }

// GetSlice returns the value at path as a []any, or an error.
func (s *Scope) GetSlice(path string) ([]any, error) { return GetAs[[]any](s, path) }

// GetMap returns the value at path as a map[string]any, or an error.
func (s *Scope) GetMap(path string) (map[string]any, error) { return GetAs[map[string]any](s, path) }
