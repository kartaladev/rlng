package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAML decodes a PipelineDef from YAML. Unknown keys are rejected
// (KnownFields), so a misspelled field such as `hitpolicy:` is a clear error
// rather than a silently-dropped key that changes semantics. An empty document
// decodes to an empty PipelineDef; Build then rejects the zero-stage case.
func ParseYAML(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		if errors.Is(err, io.EOF) { // empty document
			return &d, nil
		}
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// ParseJSON decodes a PipelineDef from JSON. Unknown keys are rejected
// (DisallowUnknownFields), matching ParseYAML. An empty document decodes to an
// empty PipelineDef; Build then rejects the zero-stage case.
func ParseJSON(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		if errors.Is(err, io.EOF) { // empty document
			return &d, nil
		}
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
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
