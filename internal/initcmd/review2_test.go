package initcmd

// Fixes from the second fresh review (DESIGN decision 52). Invented values only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsOursNeedsStandinProgram(t *testing.T) {
	hk := func(cmd string) map[string]any {
		return map[string]any{"command": cmd, "args": []any{"hook", "claude", "PreToolUse"}}
	}
	for cmd, want := range map[string]bool{
		"/usr/local/bin/standin":       true,
		`C:\Tools\standin.exe`:         true,
		"standin":                      true,
		"/opt/other/guard":             false,
		"/opt/other/standin-wrapper":   false,
		"/home/u/bin/my-standin-build": true, // the program init writes now
	} {
		if got := isOurs(hk(cmd), "/home/u/bin/my-standin-build"); got != want {
			t.Errorf("isOurs(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// An existing settings file is copied to <name>.bak before it is changed, and
// taking hooks out of a shared settings.json says teammates must run init.
func TestInitBackupAndTeammateNote(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	if err := os.MkdirAll(filepath.Join(p, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"/old/standin","args":["hook","claude","PostToolUse"]}]}]}}`
	shared := filepath.Join(p, ".claude", "settings.json")
	if err := os.WriteFile(shared, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runInit(t, "--project", p, "--bin", "/new/standin")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if b, err := os.ReadFile(shared + ".bak"); err != nil || string(b) != old {
		t.Errorf("no backup of settings.json: %v %q", err, b)
	}
	if !strings.Contains(r.out, "each must run standin init") {
		t.Errorf("no note for teammates:\n%s", r.out)
	}
	// The deny rules cover the whole private-folders root.
	home := os.Getenv("STANDIN_HOME")
	local := readJSON(t, filepath.Join(p, ".claude", "settings.local.json"))
	deny := local["permissions"].(map[string]any)["deny"].([]any)
	found := false
	for _, d := range deny {
		if d == "Read(/"+filepath.ToSlash(home)+"/**)" {
			found = true
		}
	}
	if !found {
		t.Errorf("no deny rule for %s: %v", home, deny)
	}
}
