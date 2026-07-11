package rlng

import "fmt"

// MappingError reports a failure compiling or evaluating a result-mapping field,
// or decoding the assembled result. Field is the output dot-path ("" for the
// final decode). It unwraps to the underlying expr or mapstructure error.
type MappingError struct {
	Field string
	Cause error
}

// Error renders `rlng: mapping field "f": <cause>`, or `rlng: mapping: <cause>`
// for a final-decode failure (Field == "").
func (e *MappingError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("rlng: mapping field %q: %v", e.Field, e.Cause)
	}
	return fmt.Sprintf("rlng: mapping: %v", e.Cause)
}

// Unwrap returns the underlying expr or mapstructure cause for errors.Is/As.
func (e *MappingError) Unwrap() error { return e.Cause }
