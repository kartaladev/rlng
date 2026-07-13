package rlng

import (
	"errors"
	"fmt"
)

// ErrEmptyMappingKey is the Cause of a MappingError returned by NewMapper when a
// MappingTemplate contains an empty output-path key.
var ErrEmptyMappingKey = errors.New("mapping template key must not be empty")

// ErrNilInput is returned by Evaluate/flatten when the input is a nil pointer or
// an untyped nil, which would otherwise seed an empty Scope and return a bogus
// zero result. A non-nil empty map remains a valid (empty) seed.
var ErrNilInput = errors.New("rlng: nil input")

// ErrNilPipeline is returned by New/NewTypedEngine when the required pipeline
// argument is nil (fail-fast at construction rather than a nil deref on the
// first Evaluate).
var ErrNilPipeline = errors.New("rlng: pipeline must not be nil")

// ErrNilMapper is returned by NewTypedEngine when the required mapper argument
// is nil.
var ErrNilMapper = errors.New("rlng: mapper must not be nil")

// ErrConcurrencyRequiresConfig is returned by New/NewTypedEngine when a
// concurrency Option (WithConcurrency/WithMaxParallel) is passed to a
// constructor that wraps an already-built Pipeline. Concurrency is a property of
// pipeline construction: set it on the pipeline via pipe.WithConcurrency, or use
// NewFromYAML/NewFromProvider, which build the pipeline for you.
var ErrConcurrencyRequiresConfig = errors.New("rlng: concurrency options apply only to the config constructors (NewFromYAML/NewFromProvider); set concurrency on the pipeline via pipe.WithConcurrency")

// ErrLossyResultNarrowing is returned by Mapper.Map when a decimal.Decimal
// scope value would be narrowed into a result field in a way that loses
// precision — specifically, a decimal carrying a fractional part decoded into
// an integer-kind field. Decimal->float and decimal->string are always exact
// enough to represent (a float trades exactness for range, which the caller
// has to opt into via the field's own type; a string is lossless), so only
// the integer-kind narrowing is guarded.
var ErrLossyResultNarrowing = errors.New("rlng: mapping would lose precision narrowing a decimal")

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
