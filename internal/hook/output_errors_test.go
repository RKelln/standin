package hook

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("Zelda Quorn private cause") }

func TestRedirectBrokenOutputBlocksWithoutEchoingCause(t *testing.T) {
	f := setup(t)
	input := hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": "raw/fresh.csv"}, nil)
	var stderr bytes.Buffer
	code := Run([]string{"claude", "PreToolUse", "--project", f.project}, strings.NewReader(input), failedOutput{}, &stderr)
	if code != exitBlock || strings.Contains(stderr.String(), "Zelda Quorn") {
		t.Fatalf("broken redirect output was allowed or leaked: %d, %s", code, stderr.String())
	}
}
