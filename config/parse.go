package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Parse decodes a PipelineDef from a Provider's Source. The Source's reader
// is closed after decoding if it implements io.Closer (a deferred provider's
// *os.File or HTTP response body; a preloaded provider's reader typically
// does not). Unknown fields are rejected (matching the underlying YAML/JSON
// strict decoders); an empty document decodes to an empty PipelineDef (Build
// then rejects the zero-stage case). Failures are a *ConfigError; a Source
// with an unrecognized Kind (including the KindUnspecified zero value) is
// ErrUnknownSourceKind, and a Provider that reports success but yields a nil
// Source is ErrNilSource (rather than a panic).
func Parse(ctx context.Context, p Provider) (*PipelineDef, error) {
	src, err := p.Source(ctx)
	if err != nil {
		if ce := asConfigError(err); ce != nil {
			// Preserve a nested ConfigError's Field attribution rather than
			// shadowing it behind an outer wrap.
			return nil, ce
		}
		return nil, &ConfigError{Cause: err}
	}
	if src == nil {
		return nil, &ConfigError{Cause: ErrNilSource}
	}
	r := src.Reader()
	if c, ok := r.(io.Closer); ok {
		defer c.Close()
	}
	switch src.Kind() {
	case KindYAML:
		return decodeYAML(r)
	case KindJSON:
		return decodeJSON(r)
	default:
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %s", ErrUnknownSourceKind, src.Kind())}
	}
}

// decodeYAML decodes a PipelineDef from r using strict (KnownFields) YAML
// decoding. An empty document decodes to an empty PipelineDef.
func decodeYAML(r io.Reader) (*PipelineDef, error) {
	var d PipelineDef
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		if errors.Is(err, io.EOF) { // empty document
			return &d, nil
		}
		if ce := asConfigError(err); ce != nil {
			// A nested Unmarshaler (e.g. ExprDef) already returned a
			// *ConfigError naming the offending Field; re-wrapping here
			// would shadow it (errors.As matches the first *ConfigError
			// in the chain), masking the Field attribution. Return as-is.
			return nil, ce
		}
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// decodeJSON decodes a PipelineDef from r using strict (DisallowUnknownFields)
// JSON decoding. An empty document decodes to an empty PipelineDef.
func decodeJSON(r io.Reader) (*PipelineDef, error) {
	var d PipelineDef
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		if errors.Is(err, io.EOF) { // empty document
			return &d, nil
		}
		if ce := asConfigError(err); ce != nil {
			// See the matching comment in decodeYAML: don't shadow a nested
			// ConfigError's Field attribution behind an outer wrap.
			return nil, ce
		}
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// asConfigError returns err's first *ConfigError in its chain, or nil if none
// is present.
func asConfigError(err error) *ConfigError {
	var ce *ConfigError
	if errors.As(err, &ce) {
		return ce
	}
	return nil
}
