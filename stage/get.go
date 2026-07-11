package stage

import (
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

// GetInt returns the value at path as an int, or an error.
func (s *Scope) GetInt(path string) (int, error) { return GetAs[int](s, path) }

// GetInt64 returns the value at path as an int64, or an error.
func (s *Scope) GetInt64(path string) (int64, error) { return GetAs[int64](s, path) }

// GetFloat64 returns the value at path as a float64, or an error.
func (s *Scope) GetFloat64(path string) (float64, error) { return GetAs[float64](s, path) }

// GetString returns the value at path as a string, or an error.
func (s *Scope) GetString(path string) (string, error) { return GetAs[string](s, path) }

// GetBool returns the value at path as a bool, or an error.
func (s *Scope) GetBool(path string) (bool, error) { return GetAs[bool](s, path) }

// GetSlice returns the value at path as a []any, or an error.
func (s *Scope) GetSlice(path string) ([]any, error) { return GetAs[[]any](s, path) }

// GetMap returns the value at path as a map[string]any, or an error.
func (s *Scope) GetMap(path string) (map[string]any, error) { return GetAs[map[string]any](s, path) }
