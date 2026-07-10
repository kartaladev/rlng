package expr

import (
	"reflect"
	"testing"
)

func TestToEnv(t *testing.T) {
	type Inner struct{ Ratio float64 }
	type Outer struct {
		Name  string
		Inner Inner
		Tags  []string
		Ptr   *Inner
	}

	assert := func(t *testing.T, got, want map[string]any, wantErr bool, err error) {
		t.Helper()
		if wantErr {
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}

	tests := []struct {
		name    string
		in      any
		want    map[string]any
		wantErr bool
	}{
		{"nil is empty env", nil, map[string]any{}, false},
		{"map passthrough", map[string]any{"a": 1}, map[string]any{"a": 1}, false},
		{
			name: "struct with nested, slice, nil pointer",
			in: Outer{
				Name:  "x",
				Inner: Inner{Ratio: 0.5},
				Tags:  []string{"a", "b"},
				Ptr:   nil,
			},
			want: map[string]any{
				"Name":  "x",
				"Inner": map[string]any{"Ratio": 0.5},
				"Tags":  []any{"a", "b"},
				"Ptr":   nil,
			},
		},
		{"unsupported kind errors", 42, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toEnv(tc.in)
			assert(t, got, tc.want, tc.wantErr, err)
		})
	}
}
