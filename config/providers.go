package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// bytesSource is a Source over an in-memory document; Reader returns a fresh
// *bytes.Reader each call so a bytesSource value can be reused.
type bytesSource struct {
	data []byte
	kind SourceKind
}

func (s bytesSource) Reader() io.Reader { return bytes.NewReader(s.data) }
func (s bytesSource) Kind() SourceKind  { return s.kind }

// staticProvider wraps a Source that is already known — no I/O, no error.
type staticProvider struct{ src Source }

func (p staticProvider) Source(context.Context) (Source, error) { return p.src, nil }

// FromYAMLBytes returns a Provider for an in-memory YAML document. The bytes
// are read on each Parse; the provider itself performs no I/O.
func FromYAMLBytes(data []byte) Provider { return staticProvider{bytesSource{data, KindYAML}} }

// FromJSONBytes returns a Provider for an in-memory JSON document. The bytes
// are read on each Parse; the provider itself performs no I/O.
func FromJSONBytes(data []byte) Provider { return staticProvider{bytesSource{data, KindJSON}} }

// FromYAMLString is FromYAMLBytes for a string.
func FromYAMLString(s string) Provider { return FromYAMLBytes([]byte(s)) }

// FromJSONString is FromJSONBytes for a string.
func FromJSONString(s string) Provider { return FromJSONBytes([]byte(s)) }

// nopReader hides any io.Closer a wrapped reader implements, by embedding
// only the io.Reader interface. Used by FromReader so Parse's
// `r.(io.Closer)` assertion never sees the caller's Close method.
type nopReader struct{ io.Reader }

// readerSource is a Source over a caller-supplied reader.
type readerSource struct {
	r    io.Reader
	kind SourceKind
}

func (s readerSource) Reader() io.Reader { return s.r }
func (s readerSource) Kind() SourceKind  { return s.kind }

// FromReader returns a Provider that decodes from a caller-supplied reader as
// the given kind. The caller owns the reader's lifecycle: Parse does not
// close it even if it implements io.Closer.
func FromReader(r io.Reader, kind SourceKind) Provider {
	return staticProvider{readerSource{nopReader{r}, kind}}
}

// ErrUnsupportedExtension is the Cause of the ConfigError FromFile's Source
// returns when a path's extension is neither .yaml/.yml nor .json.
var ErrUnsupportedExtension = errors.New("config: unsupported file extension")

// fileProvider is a deferred provider that opens path at Parse time.
type fileProvider struct {
	path string
	kind SourceKind // KindUnspecified => infer by extension
}

// fileSource is a Source backed by an open file; Reader exposes the *os.File
// itself (rather than hiding it behind nopReader) so Parse's io.Closer
// assertion succeeds and the handle is released after decoding.
type fileSource struct {
	f    *os.File
	kind SourceKind
}

func (s fileSource) Reader() io.Reader { return s.f }
func (s fileSource) Kind() SourceKind  { return s.kind }

func (p fileProvider) Source(context.Context) (Source, error) {
	kind := p.kind
	if kind == KindUnspecified {
		switch strings.ToLower(filepath.Ext(p.path)) {
		case ".yaml", ".yml":
			kind = KindYAML
		case ".json":
			kind = KindJSON
		default:
			return nil, &ConfigError{Cause: fmt.Errorf("%w: %q", ErrUnsupportedExtension, filepath.Ext(p.path))}
		}
	}
	f, err := os.Open(p.path)
	if err != nil {
		return nil, &ConfigError{Cause: err}
	}
	return fileSource{f, kind}, nil
}

// FromFile returns a Provider that opens path at Parse time and decodes it by
// extension: .yaml/.yml as YAML, .json as JSON, else ErrUnsupportedExtension.
//
// Trust boundary: path is passed to os.Open as-is (no base-directory
// confinement, symlink check, or size limit). Pipeline definitions are meant
// to be developer/operator-authored (trusted); do not pass a path derived
// from untrusted input.
func FromFile(path string) Provider { return fileProvider{path, KindUnspecified} }

// FromYAMLFile returns a Provider that opens path at Parse time and decodes
// it as YAML regardless of extension. Same trust boundary as FromFile.
func FromYAMLFile(path string) Provider { return fileProvider{path, KindYAML} }

// FromJSONFile returns a Provider that opens path at Parse time and decodes
// it as JSON regardless of extension. Same trust boundary as FromFile.
func FromJSONFile(path string) Provider { return fileProvider{path, KindJSON} }
