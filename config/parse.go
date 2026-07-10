package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAML decodes a PipelineDef from YAML.
func ParseYAML(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// ParseJSON decodes a PipelineDef from JSON.
func ParseJSON(data []byte) (*PipelineDef, error) {
	var d PipelineDef
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return &d, nil
}

// LoadFile reads a config file and decodes it by extension: .yaml/.yml as YAML,
// .json as JSON. Any other extension is a ConfigError.
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
