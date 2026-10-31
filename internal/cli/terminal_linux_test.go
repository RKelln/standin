package cli

import (
	"os"
	"testing"
)

func TestReviewNonTerminalCharacterDevicesRefused(t *testing.T) {
	for _, path := range []string{"/dev/null", "/dev/zero", "/dev/random"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if isTerminal(f) {
			t.Errorf("%s is a device, not a terminal", path)
		}
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}
}
