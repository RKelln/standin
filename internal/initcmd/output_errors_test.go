package initcmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("Zelda Quorn private cause") }

func TestInitBrokenInstructionsDoNotReturnSuccess(t *testing.T) {
	fakeClaude(t, "2.1.300 (Claude Code)", true, nil)
	p := newProject(t)
	var stderr bytes.Buffer
	code := Run([]string{"--project", p, "--bin", "/opt/standin/standin"}, strings.NewReader(""), failedOutput{}, &stderr)
	if code != exitBlock || strings.Contains(stderr.String(), "Zelda Quorn") || !strings.Contains(stderr.String(), "Files may already have been saved") {
		t.Fatalf("setup printing failure reported success or false rollback: %d, %s", code, stderr.String())
	}
}
