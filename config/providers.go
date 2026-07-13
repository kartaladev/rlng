package config

import (
	"bytes"
	"context"
	"io"
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
