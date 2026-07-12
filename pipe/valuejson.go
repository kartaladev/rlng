package pipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// ErrMalformedScopeValue is returned by UnmarshalJSON when a v2 Scope JSON
// envelope contains a type-tagged data value that cannot be rehydrated: an
// unrecognized "$k" kind, a tagged object missing its "v" payload, or a
// payload that does not parse for its declared kind (e.g. a "decimal" tag
// whose "v" is not a valid decimal string). It wraps the underlying parse
// error (via errors.Is/errors.As) so the offending cause is inspectable.
var ErrMalformedScopeValue = errors.New("pipe: malformed scope value")

// The wire kind tags for a Scope JSON v2 tagged value. These are the eight
// canonical value kinds of ADR-0038 (D1), plus "null" for an absent value.
const (
	kindTagBool    = "bool"
	kindTagString  = "string"
	kindTagInt64   = "int64"
	kindTagFloat64 = "float64"
	kindTagDecimal = "decimal"
	kindTagTime    = "time"
	kindTagList    = "list"
	kindTagMap     = "map"
	kindTagNull    = "null"
)

// taggedValue is the canonical on-wire form of a single scope value: its kind
// and payload. Full type tagging preserves kind across a JSON round-trip so a
// reloaded decision reproduces the same result (Spec 014 / ADR-0038).
type taggedValue struct {
	Kind string          `json:"$k"`
	V    json.RawMessage `json:"v"`
}

// encodeValue converts a scope value into its type-tagged wire form
// {"$k":<kind>,"v":<payload>}, recursing through maps and slices so every
// nested scalar carries its own kind tag. It covers the eight canonical
// kinds of ADR-0038 (D1: bool, string, int64, float64, decimal, time, list,
// map) plus nil ("null"). A json.Number (e.g. a value already round-tripped
// once through a legacy blob) is classified as int64 if it parses as an
// integer, else float64, so re-encoding a once-loaded value is stable. An
// integer-format json.Number that overflows int64 (e.g. a legacy large ID)
// is out of the int64 contract and returns an error rather than silently
// downgrading to a lossy float64 — this is a fail-closed, not fail-lossy,
// path. A uint value exceeding math.MaxInt64 is likewise out of the int64
// contract and returns an error rather than silently truncating/wrapping.
func encodeValue(v any) (any, error) {
	switch val := v.(type) {
	case nil:
		return taggedValue{Kind: kindTagNull, V: json.RawMessage("null")}, nil

	case bool:
		return encodeTagged(kindTagBool, val)

	case string:
		return encodeTagged(kindTagString, val)

	case int:
		return encodeTagged(kindTagInt64, int64(val))
	case int8:
		return encodeTagged(kindTagInt64, int64(val))
	case int16:
		return encodeTagged(kindTagInt64, int64(val))
	case int32:
		return encodeTagged(kindTagInt64, int64(val))
	case int64:
		return encodeTagged(kindTagInt64, val)
	case uint:
		if uint64(val) > math.MaxInt64 {
			return nil, fmt.Errorf("pipe: encode scope value: uint %d exceeds int64 range: %w", val, ErrMalformedScopeValue)
		}
		return encodeTagged(kindTagInt64, int64(val))
	case uint8:
		return encodeTagged(kindTagInt64, int64(val))
	case uint16:
		return encodeTagged(kindTagInt64, int64(val))
	case uint32:
		return encodeTagged(kindTagInt64, int64(val))
	case uint64:
		if val > math.MaxInt64 {
			return nil, fmt.Errorf("pipe: encode scope value: uint64 %d exceeds int64 range: %w", val, ErrMalformedScopeValue)
		}
		return encodeTagged(kindTagInt64, int64(val))

	case float32:
		return encodeTagged(kindTagFloat64, float64(val))
	case float64:
		return encodeTagged(kindTagFloat64, val)

	case decimal.Decimal:
		// Decimal.String trims trailing zeros, dropping scale (18125.00 ->
		// "18125"). StringFixed with the decimal's own fractional digit count
		// preserves the exponent so the round-trip is lossless including scale.
		text := val.String()
		if e := val.Exponent(); e < 0 {
			text = val.StringFixed(-e)
		}
		return encodeTagged(kindTagDecimal, text)

	case time.Time:
		return encodeTagged(kindTagTime, val.Format(time.RFC3339Nano))

	case json.Number:
		if i, err := val.Int64(); err == nil {
			return encodeTagged(kindTagInt64, i)
		} else if isIntegerRangeErr(err) {
			// Int64() failed because the number IS integer-format but
			// overflows int64 (e.g. a legacy large ID surviving a v0
			// blob) — not because it's genuinely non-integral. Fail
			// closed instead of silently downgrading to a lossy
			// float64 approximation.
			return nil, fmt.Errorf("pipe: encode scope value: json.Number %q exceeds int64 range: %w", val.String(), ErrMalformedScopeValue)
		}
		f, err := val.Float64()
		if err != nil {
			return nil, fmt.Errorf("pipe: encode scope value: json.Number %q: %w", val.String(), ErrMalformedScopeValue)
		}
		return encodeTagged(kindTagFloat64, f)

	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			enc, err := encodeValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = enc
		}
		return encodeTagged(kindTagList, out)

	case map[string]any:
		// encoding/json already sorts map[string]T keys when marshaling, which
		// is what makes the top-level `data` map (and this nested payload)
		// deterministic; the explicit sort here just makes that guarantee
		// visible rather than relying on it implicitly.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(val))
		for _, k := range keys {
			enc, err := encodeValue(val[k])
			if err != nil {
				return nil, err
			}
			out[k] = enc
		}
		return encodeTagged(kindTagMap, out)

	default:
		return nil, fmt.Errorf("pipe: encode scope value: unsupported type %T: %w", v, ErrMalformedScopeValue)
	}
}

// isIntegerRangeErr reports whether err is the *strconv.NumError produced by
// json.Number.Int64() (which delegates to strconv.ParseInt) for a value that
// IS integer-format but overflows int64 — as opposed to a syntax error for a
// genuinely non-integral number (e.g. "1.5"). Distinguishing the two lets
// encodeValue fail closed on overflow rather than silently falling through
// to a lossy float64 approximation.
func isIntegerRangeErr(err error) bool {
	var numErr *strconv.NumError
	return errors.As(err, &numErr) && errors.Is(numErr.Err, strconv.ErrRange)
}

// encodeTagged marshals payload and wraps it as a taggedValue for kind. A
// marshal failure (e.g. a non-finite float64 — encoding/json rejects
// NaN/+Inf/-Inf) is wrapped as ErrMalformedScopeValue, consistent with every
// other encode-time failure in this file.
func encodeTagged(kind string, payload any) (any, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pipe: encode scope value: kind %s: %w: %w", kind, err, ErrMalformedScopeValue)
	}
	return taggedValue{Kind: kind, V: b}, nil
}

// decodeValue inverts encodeValue for a v2 blob: a value shaped like
// {"$k":<kind>,"v":<payload>} is rehydrated to its Go kind (decimal via
// decimal.NewFromString, time via time.Parse(time.RFC3339Nano, …), int64/
// float64 from the JSON number, list/map recursively). A value that is NOT a
// tagged object (no "$k") is returned unchanged — the legacy/untagged read
// path — so a v1 field embedded in an otherwise-v2 structure still loads. A
// malformed tag (unknown kind, missing payload, unparseable payload) returns
// ErrMalformedScopeValue.
func decodeValue(v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	kindRaw, hasKind := m["$k"]
	if !hasKind {
		return v, nil
	}
	kind, ok := kindRaw.(string)
	if !ok {
		return nil, fmt.Errorf("pipe: decode scope value: \"$k\" is not a string: %w", ErrMalformedScopeValue)
	}
	payload, hasPayload := m["v"]
	if !hasPayload {
		return nil, fmt.Errorf("pipe: decode scope value: kind %s missing \"v\" payload: %w", kind, ErrMalformedScopeValue)
	}

	switch kind {
	case kindTagNull:
		return nil, nil

	case kindTagBool:
		b, ok := payload.(bool)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind bool payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		return b, nil

	case kindTagString:
		s, ok := payload.(string)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind string payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		return s, nil

	case kindTagInt64:
		n, ok := payload.(json.Number)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind int64 payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("pipe: decode scope value: kind int64 payload %q: %w: %w", n.String(), err, ErrMalformedScopeValue)
		}
		return i, nil

	case kindTagFloat64:
		n, ok := payload.(json.Number)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind float64 payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		f, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("pipe: decode scope value: kind float64 payload %q: %w: %w", n.String(), err, ErrMalformedScopeValue)
		}
		return f, nil

	case kindTagDecimal:
		s, ok := payload.(string)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind decimal payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		d, err := decimal.NewFromString(s)
		if err != nil {
			return nil, fmt.Errorf("pipe: decode scope value: kind decimal payload %q: %w: %w", s, err, ErrMalformedScopeValue)
		}
		return d, nil

	case kindTagTime:
		s, ok := payload.(string)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind time payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil, fmt.Errorf("pipe: decode scope value: kind time payload %q: %w: %w", s, err, ErrMalformedScopeValue)
		}
		return tm, nil

	case kindTagList:
		list, ok := payload.([]any)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind list payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		out := make([]any, len(list))
		for i, elem := range list {
			dec, err := decodeValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = dec
		}
		return out, nil

	case kindTagMap:
		obj, ok := payload.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("pipe: decode scope value: kind map payload is %T: %w", payload, ErrMalformedScopeValue)
		}
		out := make(map[string]any, len(obj))
		for k, elem := range obj {
			dec, err := decodeValue(elem)
			if err != nil {
				return nil, err
			}
			out[k] = dec
		}
		return out, nil

	default:
		return nil, fmt.Errorf("pipe: decode scope value: unknown kind %q: %w", kind, ErrMalformedScopeValue)
	}
}
