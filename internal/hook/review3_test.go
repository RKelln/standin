package hook

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/store"
)

func TestReviewUnknownToolsReachRefusal(t *testing.T) {
	f := setup(t)
	for _, tool := range []string{"mcp__files__read", "FutureFileTool"} {
		r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", f.project, tool, map[string]any{"path": "raw/fresh.csv"}, nil))
		if r.code != 2 || !strings.Contains(r.stderr, "does not know the tool") {
			t.Fatalf("unknown tool bypassed guard: %+v", r)
		}
	}
	r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", f.project, "AskUserQuestion", map[string]any{}, nil))
	if r.code != 0 {
		t.Fatalf("conversation-only tool blocked: %+v", r)
	}
}

func TestReviewInterruptedSaveBlocksScrubbedReads(t *testing.T) {
	f := setup(t)
	write(t, filepath.Join(f.store, store.PendingFile), `{"version":1}`)
	r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": "scrubbed/fresh.csv"}, nil))
	if r.code != 2 || !strings.Contains(r.stderr, "last save did not finish") {
		t.Fatalf("mixed-generation output allowed: %+v", r)
	}
}

func TestReviewCopiedRecoveryFilesAreSecrets(t *testing.T) {
	f := setup(t)
	for _, file := range []string{".standin-previous-mapping.json", ".standin-previous-words.json", ".standin-previous-approval.json"} {
		write(t, filepath.Join(f.project, "notes", file), `{"invented":"secret"}`)
		r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": filepath.Join("notes", file)}, nil))
		if r.code != 2 {
			t.Fatalf("copied recovery file was readable: %+v", r)
		}
	}
}
