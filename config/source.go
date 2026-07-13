package config

import (
	"context"
	"errors"
	"io"
)

// SourceKind is the wire format of a config Source. The zero value
// (KindUnspecified) is invalid: Parse rejects it, so a Source that forgets to
// declare a kind fails loud rather than silently defaulting to a format.
type SourceKind int

const (
	KindUnspecified SourceKind = iota // invalid; zero-value guard
	KindYAML
	KindJSON
)

// String renders the kind as "yaml"/"json", or "unspecified" for the zero
// value (and any other unrecognized value).
func (k SourceKind) String() string {
	switch k {
	case KindYAML:
		return "yaml"
	case KindJSON:
		return "json"
	default:
		return "unspecified"
	}
}

// Provider yields a Source to parse. A preloaded provider (bytes/string)
// returns immediately; a deferred provider (file/URL) performs I/O in Source
// and may fail. ctx cancels/deadlines that deferred I/O.
type Provider interface {
	Source(ctx context.Context) (Source, error)
}

// Source is one config document to decode: a reader over its bytes and the
// declared format. If Reader returns a value that also implements io.Closer
// (e.g. an *os.File), Parse closes it after decoding.
type Source interface {
	Reader() io.Reader
	Kind() SourceKind
}

// ErrUnknownSourceKind is the Cause of the ConfigError Parse returns when a
// Source declares a kind it cannot decode (including the KindUnspecified
// zero value).
var ErrUnknownSourceKind = errors.New("config: unknown source kind")

// ErrNilSource is the Cause of the ConfigError Parse returns when a Provider
// reports success (nil error) but yields a nil Source — a Provider-contract
// violation surfaced as a debuggable typed error rather than a panic.
var ErrNilSource = errors.New("config: provider returned a nil Source")
