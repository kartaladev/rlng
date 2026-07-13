package pipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
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

// GetIntCoerce returns the value at path as an int, coercing from a wider set of
// stored types than the strict GetInt: any integer kind (overflow-checked), an
// integral finite float, an integer json.Number, or a base-10 numeric string
// (surrounding whitespace trimmed). It never silently truncates or wraps: a
// non-integral or non-finite float, a value that overflows int, an unparseable
// string, a non-integer json.Number, or a non-numeric type (bool, nil, slice,
// map, …) is a *ScopeTypeError; a missing path is ErrPathNotFound. Coercion is
// opt-in — the strict GetInt is unchanged. See ADR-0044.
func (s *Scope) GetIntCoerce(path string) (int, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	i, err := coerceToInt64(v)
	if err != nil {
		return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: err.Error()}
	}
	// Unreachable on 64-bit platforms (int == int64); guards 32-bit builds.
	if int64(int(i)) != i {
		return 0, &ScopeTypeError{Path: path, Expected: "int", Actual: fmt.Sprintf("int64(%d) overflows int", i)}
	}
	return int(i), nil
}

// GetInt64Coerce returns the value at path as an int64, coercing per the same
// rules as GetIntCoerce (integer kinds overflow-checked, integral finite floats,
// integer json.Number, base-10 numeric strings). An unconvertible value is a
// *ScopeTypeError; a missing path is ErrPathNotFound. The strict GetInt64 is
// unchanged. See ADR-0044.
func (s *Scope) GetInt64Coerce(path string) (int64, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	i, err := coerceToInt64(v)
	if err != nil {
		return 0, &ScopeTypeError{Path: path, Expected: "int64", Actual: err.Error()}
	}
	return i, nil
}

// GetFloat64Coerce returns the value at path as a float64, coercing from a wider
// set of stored types than the strict GetFloat64: a float is returned as-is
// (a value already stored as a non-finite float passes through — coercion widens
// types, it does not re-validate a caller-stored value); any integer kind widens
// to float64 (magnitudes above 2^53 may lose precision, inherent to float64); a
// json.Number reads via its Float64; and a numeric string parses via
// strconv.ParseFloat (whitespace trimmed). An unparseable string, a string that
// names a non-finite value ("NaN"/"Inf" — coercion never manufactures a
// non-finite float from text, per ADR-0035), or a non-numeric type is a
// *ScopeTypeError; a missing path is ErrPathNotFound. The strict GetFloat64 is
// unchanged. See ADR-0044.
func (s *Scope) GetFloat64Coerce(path string) (float64, error) {
	v, ok := s.Get(path)
	if !ok {
		return 0, ErrPathNotFound
	}
	f, err := coerceToFloat64(v)
	if err != nil {
		return 0, &ScopeTypeError{Path: path, Expected: "float64", Actual: err.Error()}
	}
	return f, nil
}

// coerceToInt64 converts v to an int64 under the GetIntCoerce/GetInt64Coerce
// rules, returning a plain error describing the reason on failure (the caller
// wraps it in a *ScopeTypeError naming the path and target). Integer kinds are
// overflow-checked, floats must be finite and integral, strings parse base-10,
// and a json.Number must name an integer.
func coerceToInt64(v any) (int64, error) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("json.Number(%s) is not an integer", n.String())
		}
		return i, nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("string(%q) is not an integer", n)
		}
		return i, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u > math.MaxInt64 {
			return 0, fmt.Errorf("uint64(%d) overflows int64", u)
		}
		return int64(u), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("float64(%v) is not finite", f)
		}
		if f != math.Trunc(f) {
			return 0, fmt.Errorf("float64(%v) is not integral", f)
		}
		if f >= float64(math.MaxInt64) || f < float64(math.MinInt64) {
			return 0, fmt.Errorf("float64(%v) overflows int64", f)
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("%T is not numeric", v)
	}
}

// coerceToFloat64 converts v to a float64 under the GetFloat64Coerce rules,
// returning a plain error describing the reason on failure. A float passes
// through as-is (a caller-stored non-finite value is preserved); integer kinds
// widen; a json.Number reads via Float64; a string parses via ParseFloat and is
// rejected if it names a non-finite value.
func coerceToFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, fmt.Errorf("json.Number(%s) is not a number", n.String())
		}
		// A json.Number is a text token like a string, so it is held to the same
		// rule: coercion never manufactures a non-finite float from text.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("json.Number(%s) is not finite", n.String())
		}
		return f, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, fmt.Errorf("string(%q) is not a number", n)
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("string(%q) is not finite", n)
		}
		return f, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), nil
	default:
		return 0, fmt.Errorf("%T is not numeric", v)
	}
}
