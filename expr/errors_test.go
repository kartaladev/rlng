package expr

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	inner := errors.New("boom")

	assert := func(t *testing.T, got error, wantMsg string, wantUnwrap error) {
		t.Helper()
		if got.Error() != wantMsg {
			t.Fatalf("message = %q, want %q", got.Error(), wantMsg)
		}
		if !errors.Is(got, wantUnwrap) {
			t.Fatalf("errors.Is(%v, %v) = false, want true", got, wantUnwrap)
		}
	}

	tests := []struct {
		name      string
		err       error
		wantMsg   string
		wantChain error
	}{
		{
			name:      "compile error names field and expression",
			err:       &CompileError{Name: "discount", Expression: "x >", Cause: inner},
			wantMsg:   `compile "discount" (x >): boom`,
			wantChain: inner,
		},
		{
			name:      "eval error names field and expression",
			err:       &EvalError{Name: "discount", Expression: "x + y", Cause: inner},
			wantMsg:   `eval "discount" (x + y): boom`,
			wantChain: inner,
		},
		{
			name:      "eval error wraps ErrNotBool",
			err:       &EvalError{Expression: "x + 1", Cause: ErrNotBool},
			wantMsg:   `eval (x + 1): expression did not evaluate to bool`,
			wantChain: ErrNotBool,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert(t, tc.err, tc.wantMsg, tc.wantChain)
		})
	}
}
