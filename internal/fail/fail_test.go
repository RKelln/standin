package fail

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodePreservesWrappedRefusal(t *testing.T) {
	refusal := New("private-busy", "Try again.")
	for _, err := range []error{refusal, fmt.Errorf("context: %w", refusal), errors.Join(errors.New("other failure"), refusal)} {
		if got := Code(err); got != "private-busy" {
			t.Errorf("Code(%v) = %q", err, got)
		}
	}
	for _, err := range []error{nil, errors.New("ordinary failure"), (*Error)(nil)} {
		if got := Code(err); got != "error" {
			t.Errorf("Code(%v) = %q", err, got)
		}
	}
}
