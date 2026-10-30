package initcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/config"
)

type res struct {
	code     int
	out, err string
}

func runInit(t *testing.T, args ...string) res {
	t.Helper()
	var o, e bytes.Buffer
	c := Run(args, strings.NewReader(""), &o, &e)
	return res{c, o.String(), e.String()}
}

func fakeClaude(t *testing.T, v string, found bool, err error) {
	t.Helper()
	old := ClaudeVersion
	ClaudeVersion = func() (string, bool, error) { return v, found, err }
	t.Cleanup(func() { ClaudeVersion = old })
}

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("STANDIN_HOME", filepath.Join(root, "home"))
	p := filepath.Join(root, "proj")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func readJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return m
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if info.IsDir() {
			m[rel] = "dir"
			return nil
		}
		b, _ := os.ReadFile(p)
		m[rel] = string(b)
		return nil
	})
	return m
}

func TestInitFreshAndIdempotent(t *testing.T) {
	fakeClaude(t, "2.1.300 (Claude Code)", true, nil)
	p := newProject(t)
	r := runInit(t, "--project", p, "--bin", "/opt/standin/standin", "--harness", "claude,codex,opencode")
	if r.code != 0 {
		t.Fatalf("code %d: %s %s", r.code, r.out, r.err)
	}
	for _, d := range []string{"raw", "shape", "scrubbed", "restored"} {
		if st, err := os.Stat(filepath.Join(p, d)); err != nil || !st.IsDir() {
			t.Errorf("no %s/", d)
		}
	}
	for _, f := range []string{"standin.config.json", "AGENTS.md", ".claude/settings.json",
		"standin-harness/codex-UNTESTED.md", "standin-harness/opencode-UNTESTED.md"} {
		if _, err := os.Stat(filepath.Join(p, f)); err != nil {
			t.Errorf("no %s", f)
		}
	}
	s := readJSON(t, filepath.Join(p, ".claude", "settings.json"))
	hooks := s["hooks"].(map[string]any)
	for _, ev := range []string{"PreToolUse", "PostToolUse", "UserPromptSubmit"} {
		groups := hooks[ev].([]any)
		if len(groups) != 1 {
			t.Fatalf("%s: %d groups", ev, len(groups))
		}
		h := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
		if h["onFailure"] != "block" || h["command"] != "/opt/standin/standin" || h["type"] != "command" {
			t.Errorf("%s hook wrong: %v", ev, h)
		}
		args := h["args"].([]any)
		if args[0] != "hook" || args[1] != "claude" || args[2] != ev || args[4] != p {
			t.Errorf("%s args wrong: %v", ev, args)
		}
	}
	deny := s["permissions"].(map[string]any)["deny"].([]any)
	for _, want := range []string{"Read(./raw/**)", "Edit(./raw/**)", "Read(./restored/**)", "Edit(./restored/**)"} {
		found := false
		for _, d := range deny {
			found = found || d == want
		}
		if !found {
			t.Errorf("deny lacks %s", want)
		}
	}
	agents, _ := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	for _, want := range []string{"scrubbed/", "raw/", "restored/", "secret", "standin report"} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if !strings.Contains(r.out, "Private store") {
		t.Errorf("does not say where the secret lives: %s", r.out)
	}

	before := snapshot(t, p)
	r2 := runInit(t, "--project", p, "--bin", "/opt/standin/standin", "--harness", "claude,codex,opencode")
	if r2.code != 0 || !strings.Contains(r2.out, "Nothing to change") {
		t.Fatalf("second run: %d %s %s", r2.code, r2.out, r2.err)
	}
	after := snapshot(t, p)
	if len(before) != len(after) {
		t.Fatalf("files changed on re-run: %v vs %v", before, after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed on re-run", k)
		}
	}
}

func TestInitMergesExisting(t *testing.T) {
	fakeClaude(t, "claude 2.1.295", true, nil)
	p := newProject(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(p, ".claude"), 0o755))
	existing := `{
  "model": "some-model",
  "env": {"A": "1"},
  "permissions": {"allow": ["Bash(ls:*)"], "deny": ["Read(./secrets/**)", "Read(./raw/**)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "other-tool"}]},
      {"matcher": "Read", "hooks": [{"type": "command", "command": "/old/standin", "args": ["hook", "claude", "PreToolUse"]}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "notify"}]}]
  }
}`
	must(os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte(existing), 0o600))
	must(os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte("# My rules\n\nBe kind."), 0o644))
	must(os.WriteFile(filepath.Join(p, "standin.config.json"), []byte(`{"version":1,"files":[{"match":"raw/a.csv","columns":{}}]}`), 0o644))

	r := runInit(t, "--project", p, "--bin", "/new/standin")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.err)
	}
	s := readJSON(t, filepath.Join(p, ".claude", "settings.json"))
	if s["model"] != "some-model" || s["env"].(map[string]any)["A"] != "1" {
		t.Error("lost existing keys")
	}
	perms := s["permissions"].(map[string]any)
	if len(perms["allow"].([]any)) != 1 {
		t.Error("lost allow rules")
	}
	deny := perms["deny"].([]any)
	count := 0
	for _, d := range deny {
		if d == "Read(./raw/**)" {
			count++
		}
		if d == "Read(./secrets/**)" {
			count += 10
		}
	}
	if count != 11 {
		t.Errorf("deny merge wrong: %v", deny)
	}
	hooks := s["hooks"].(map[string]any)
	if len(hooks["Stop"].([]any)) != 1 {
		t.Error("lost Stop hook")
	}
	pre := hooks["PreToolUse"].([]any)
	if len(pre) != 2 {
		t.Fatalf("want other-tool group plus one standin group, got %v", pre)
	}
	if pre[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] != "other-tool" {
		t.Error("other hook lost or moved")
	}
	if pre[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] != "/new/standin" {
		t.Error("old standin hook not replaced")
	}
	if st, _ := os.Stat(filepath.Join(p, ".claude", "settings.json")); st.Mode().Perm() != 0o600 {
		t.Errorf("file mode changed to %v", st.Mode().Perm())
	}
	ag, _ := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if !strings.HasPrefix(string(ag), "# My rules\n\nBe kind.\n\n"+agentsBegin) {
		t.Errorf("AGENTS.md not appended cleanly: %q", ag)
	}
	cfg, _ := os.ReadFile(filepath.Join(p, "standin.config.json"))
	if !strings.Contains(string(cfg), "raw/a.csv") {
		t.Error("existing config overwritten")
	}

	// An edited block is refreshed in place, text around it kept.
	edited := strings.Replace(string(ag), "Never read", "Maybe read", 1) + "\nAfter.\n"
	must(os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte(edited), 0o644))
	if r := runInit(t, "--project", p, "--bin", "/new/standin"); r.code != 0 {
		t.Fatal(r.err)
	}
	ag2, _ := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if strings.Contains(string(ag2), "Maybe read") || !strings.HasSuffix(string(ag2), "\nAfter.\n") || strings.Count(string(ag2), agentsMark) != 1 {
		t.Errorf("block not refreshed: %q", ag2)
	}
}

func TestInitRefusals(t *testing.T) {
	cases := []struct {
		name   string
		claude func(t *testing.T)
		prep   func(t *testing.T, p string)
		args   []string
		inErr  string
	}{
		{"old claude", func(t *testing.T) { fakeClaude(t, "2.1.294", true, nil) }, nil, nil, "too old"},
		{"old major", func(t *testing.T) { fakeClaude(t, "1.9.999", true, nil) }, nil, nil, "too old"},
		{"unreadable version", func(t *testing.T) { fakeClaude(t, "", true, errors.New("no number")) }, nil, nil, "cannot read the Claude Code version"},
		{"bad settings json", nil, func(t *testing.T, p string) {
			os.MkdirAll(filepath.Join(p, ".claude"), 0o755)
			os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte("{oops"), 0o644)
		}, nil, "not a JSON object"},
		{"settings is array", nil, func(t *testing.T, p string) {
			os.MkdirAll(filepath.Join(p, ".claude"), 0o755)
			os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte("[]"), 0o644)
		}, nil, "not a JSON object"},
		{"hooks not object", nil, func(t *testing.T, p string) {
			os.MkdirAll(filepath.Join(p, ".claude"), 0o755)
			os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte(`{"hooks": []}`), 0o644)
		}, nil, "hooks is not an object"},
		{"deny not list", nil, func(t *testing.T, p string) {
			os.MkdirAll(filepath.Join(p, ".claude"), 0o755)
			os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte(`{"permissions": {"deny": "x"}}`), 0o644)
		}, nil, "deny is not a list"},
		{"broken agents block", nil, func(t *testing.T, p string) {
			os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte(agentsBegin+"\nhalf"), 0o644)
		}, nil, "broken or doubled"},
		{"unknown harness", nil, nil, []string{"--harness", "claude,vim"}, "not a harness"},
		{"store inside project", nil, nil, []string{"--private", "PROJ/secret"}, "inside the project"},
		{"raw is a file", nil, func(t *testing.T, p string) {
			os.WriteFile(filepath.Join(p, "raw"), []byte("x"), 0o644)
		}, nil, "not a folder"},
		{"extra words", nil, nil, []string{"now"}, "unexpected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.claude != nil {
				tc.claude(t)
			} else {
				fakeClaude(t, "2.1.300", true, nil)
			}
			p := newProject(t)
			if tc.prep != nil {
				tc.prep(t, p)
			}
			before := snapshot(t, p)
			args := []string{"--project", p, "--bin", "/x/standin"}
			for _, a := range tc.args {
				args = append(args, strings.Replace(a, "PROJ", p, 1))
			}
			r := runInit(t, args...)
			if r.code != 2 {
				t.Fatalf("code %d, want 2: %s %s", r.code, r.out, r.err)
			}
			if !strings.Contains(r.err, tc.inErr) {
				t.Errorf("stderr %q lacks %q", r.err, tc.inErr)
			}
			if tc.name == "raw is a file" {
				return // folders before it may be made; files are not
			}
			after := snapshot(t, p)
			if len(after) != len(before) {
				t.Errorf("refusal still wrote files: %v", after)
			}
		})
	}
}

func TestInitClaudeMissingWarns(t *testing.T) {
	fakeClaude(t, "", false, nil)
	p := newProject(t)
	r := runInit(t, "--project", p, "--bin", "/x/standin")
	if r.code != 1 || !strings.Contains(r.err, "not found") {
		t.Fatalf("got %d %q", r.code, r.err)
	}
	if _, err := os.Stat(filepath.Join(p, ".claude", "settings.json")); err != nil {
		t.Error("settings not written when claude is absent")
	}
}

func TestInitNoVersionCheck(t *testing.T) {
	fakeClaude(t, "2.0.1", true, nil)
	p := newProject(t)
	r := runInit(t, "--project", p, "--bin", "/x/standin", "--no-version-check")
	if r.code != 1 || !strings.Contains(r.err, "older") {
		t.Fatalf("got %d %q", r.code, r.err)
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{{"2.1.295", true}, {"2.1.296", true}, {"2.2.0", true}, {"3.0.0", true}, {"2.1.294", false}, {"2.0.999", false}, {"x", false}}
	for _, c := range cases {
		if got := atLeast(c.v, MinClaude); got != c.want {
			t.Errorf("%s: %v", c.v, got)
		}
	}
	if v, ok := parseVersion("2.1.301 (Claude Code)\n"); !ok || v != "2.1.301" {
		t.Errorf("parse: %q %v", v, ok)
	}
}

func TestStarterConfigParses(t *testing.T) {
	if _, err := config.Parse([]byte(starterConfig)); err != nil {
		t.Fatalf("the core config reader refuses the starter config: %v", err)
	}
}
