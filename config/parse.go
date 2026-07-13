package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse decodes a PipelineDef from a Provider's Source. The Source's reader
// is closed after decoding if it implements io.Closer (a deferred provider's
// *os.File or HTTP response body; a preloaded provider's reader typically
// does not). Unknown fields are rejected (matching the underlying YAML/JSON
// strict decoders); an empty document decodes to an empty PipelineDef (Build
// then rejects the zero-stage case). Failures are a *ConfigError; a Source
// with an unrecognized Kind (including the KindUnspecified zero value) is
// ErrUnknownSourceKind.
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

// ParseYAML decodes a PipelineDef from YAML. Unknown keys are rejected
// (KnownFields), so a misspelled field such as `hitpolicy:` is a clear error
// rather than a silently-dropped key that changes semantics. An empty document
// decodes to an empty PipelineDef; Build then rejects the zero-stage case.
func ParseYAML(data []byte) (*PipelineDef, error) {
	return decodeYAML(bytes.NewReader(data))
}

// ParseJSON decodes a PipelineDef from JSON. Unknown keys are rejected
// (DisallowUnknownFields), matching ParseYAML. An empty document decodes to an
// empty PipelineDef; Build then rejects the zero-stage case.
func ParseJSON(data []byte) (*PipelineDef, error) {
	return decodeJSON(bytes.NewReader(data))
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

// LoadFile reads a config file and decodes it by extension: .yaml/.yml as YAML,
// .json as JSON. Any other extension is a ConfigError.
//
// Trust boundary: path is passed to os.ReadFile as-is (no base-directory
// confinement, symlink check, or size limit), and the whole file is read into
// memory. Pipeline definitions are meant to be developer/operator-authored
// (trusted). Do not pass a path or file contents derived from untrusted input;
// if you must, confine the path and cap the size before calling ParseYAML /
// ParseJSON on the bytes yourself.
func LoadFile(path string) (*PipelineDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return ParseYAML(data)
	case ".json":
		return ParseJSON(data)
	default:
		return nil, &ConfigError{Cause: fmt.Errorf("unsupported config extension %q", filepath.Ext(path))}
	}
}
