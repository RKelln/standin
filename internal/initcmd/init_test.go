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
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
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
	if err != nil {
		t.Fatal(err)
	}
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
	for _, f := range []string{"standin.config.json", "AGENTS.md", ".claude/settings.local.json",
		"standin-harness/codex-UNTESTED.md", "standin-harness/opencode-UNTESTED.md"} {
		if _, err := os.Stat(filepath.Join(p, f)); err != nil {
			t.Errorf("no %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(p, ".claude", "settings.json")); err == nil {
		t.Error("the shared settings.json was written without --shared")
	}
	s := readJSON(t, filepath.Join(p, ".claude", "settings.local.json"))
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
		if len(args) != 3 || args[0] != "hook" || args[1] != "claude" || args[2] != ev {
			t.Errorf("%s args wrong (want no --project): %v", ev, args)
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

	r := runInit(t, "--project", p, "--bin", "/new/standin", "--shared")
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
	if r := runInit(t, "--project", p, "--bin", "/new/standin", "--shared"); r.code != 0 {
		t.Fatal(r.err)
	}
	ag2, _ := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if strings.Contains(string(ag2), "Maybe read") || !strings.HasSuffix(string(ag2), "\nAfter.\n") || strings.Count(string(ag2), agentsMark) != 1 {
		t.Errorf("block not refreshed: %q", ag2)
	}
}

func TestInitRefusals(t *testing.T) {
	writeSettings := func(t *testing.T, p, name, body string) {
		t.Helper()
		dir := filepath.Join(p, ".claude")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
			if err := os.MkdirAll(filepath.Join(p, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte("{oops"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, "not a JSON object"},
		{"settings is array", nil, func(t *testing.T, p string) {
			writeSettings(t, p, "settings.json", "[]")
		}, nil, "not a JSON object"},
		{"hooks not object", nil, func(t *testing.T, p string) {
			writeSettings(t, p, "settings.json", `{"hooks": []}`)
		}, nil, "hooks is not an object"},
		{"deny not list", nil, func(t *testing.T, p string) {
			writeSettings(t, p, "settings.local.json", `{"permissions": {"deny": "x"}}`)
		}, nil, "deny is not a list"},
		{"shared deny not list", nil, func(t *testing.T, p string) {
			writeSettings(t, p, "settings.json", `{"permissions": {"deny": "x"}}`)
		}, []string{"--shared"}, "deny is not a list"},
		{"bad local settings json", nil, func(t *testing.T, p string) {
			if err := os.MkdirAll(filepath.Join(p, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, ".claude", "settings.local.json"), []byte("{oops"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, "not a JSON object"},
		{"broken agents block", nil, func(t *testing.T, p string) {
			if err := os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte(agentsBegin+"\nhalf"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, "broken or doubled"},
		{"unknown harness", nil, nil, []string{"--harness", "claude,vim"}, "not a harness"},
		{"store inside project", nil, nil, []string{"--private", "PROJ/secret"}, "inside the project"},
		{"raw is a file", nil, func(t *testing.T, p string) {
			if err := os.WriteFile(filepath.Join(p, "raw"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
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
	if _, err := os.Stat(filepath.Join(p, ".claude", "settings.local.json")); err != nil {
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

func hookEvents(t *testing.T, path string) map[string]any {
	t.Helper()
	s := readJSON(t, path)
	h, _ := s["hooks"].(map[string]any)
	return h
}

// 36: a v0.1.0 setup wrote standin's hooks (with --project) into settings.json.
// A re-run writes settings.local.json and takes only standin's hooks out of
// settings.json; everything else there stays.
func TestInitMovesOldHooksToLocal(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	if err := os.MkdirAll(filepath.Join(p, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{
  "model": "some-model",
  "permissions": {"deny": ["Read(./secrets/**)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "other-tool"}]},
      {"matcher": "Read", "hooks": [{"type": "command", "command": "/old/standin", "args": ["hook", "claude", "PreToolUse", "--project", "/gone/shop"], "onFailure": "block"}]}
    ],
    "PostToolUse": [{"hooks": [{"type": "command", "command": "/old/standin", "args": ["hook", "claude", "PostToolUse", "--project", "/gone/shop"]}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/old/standin", "args": ["hook", "claude", "UserPromptSubmit", "--project", "/gone/shop"]}]}],
    "Stop": [{"hooks": [{"type": "command", "command": "notify", "timeout": 5}]}]
  }
}`
	shared := filepath.Join(p, ".claude", "settings.json")
	if err := os.WriteFile(shared, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runInit(t, "--project", p, "--bin", "/new/standin")
	if r.code != 0 {
		t.Fatalf("code %d: %s %s", r.code, r.out, r.err)
	}
	if !strings.Contains(r.out, "settings.json (took out standin's old hooks") {
		t.Errorf("does not say it changed settings.json:\n%s", r.out)
	}
	var want map[string]any
	if err := json.Unmarshal([]byte(`{
  "model": "some-model",
  "permissions": {"deny": ["Read(./secrets/**)"]},
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "other-tool"}]}],
    "Stop": [{"hooks": [{"type": "command", "command": "notify", "timeout": 5}]}]
  }
}`), &want); err != nil {
		t.Fatal(err)
	}
	if got := readJSON(t, shared); !jsonEqual(got, want) {
		b, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("settings.json after init:\n%s", b)
	}
	local := hookEvents(t, filepath.Join(p, ".claude", "settings.local.json"))
	for _, ev := range []string{"PreToolUse", "PostToolUse", "UserPromptSubmit"} {
		if g, _ := local[ev].([]any); len(g) != 1 {
			t.Errorf("settings.local.json %s: %v", ev, local[ev])
		}
	}
	// Re-run: nothing to change.
	if r := runInit(t, "--project", p, "--bin", "/new/standin"); !strings.Contains(r.out, "Nothing to change") {
		t.Errorf("second run changed something:\n%s", r.out)
	}
}

// A settings.json with no standin hooks is not touched, not even reformatted.
func TestInitLeavesSharedAlone(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	if err := os.MkdirAll(filepath.Join(p, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"model\":   \"x\", \"hooks\": {\"Stop\": []}}"
	shared := filepath.Join(p, ".claude", "settings.json")
	if err := os.WriteFile(shared, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runInit(t, "--project", p, "--bin", "/x/standin"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if b, _ := os.ReadFile(shared); string(b) != body {
		t.Errorf("settings.json changed: %q", b)
	}
}

// --shared writes settings.json and takes standin's hooks out of the local file.
func TestInitSharedMovesBack(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	if r := runInit(t, "--project", p, "--bin", "/x/standin"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if r := runInit(t, "--project", p, "--bin", "/x/standin", "--shared"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if h := hookEvents(t, filepath.Join(p, ".claude", "settings.json")); len(h) != 3 {
		t.Errorf("settings.json hooks: %v", h)
	}
	if h := hookEvents(t, filepath.Join(p, ".claude", "settings.local.json")); len(h) != 0 {
		t.Errorf("settings.local.json still has standin hooks: %v", h)
	}
}

// --no-prompt-guard leaves out UserPromptSubmit, and takes out one written before.
func TestInitNoPromptGuard(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	local := filepath.Join(p, ".claude", "settings.local.json")
	if r := runInit(t, "--project", p, "--bin", "/x/standin"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if _, ok := hookEvents(t, local)["UserPromptSubmit"]; !ok {
		t.Fatal("the prompt guard should be on by default")
	}
	r := runInit(t, "--project", p, "--bin", "/x/standin", "--no-prompt-guard")
	if r.code != 0 || !strings.Contains(r.out, "prompt guard is off") {
		t.Fatalf("%+v", r)
	}
	h := hookEvents(t, local)
	if _, ok := h["UserPromptSubmit"]; ok {
		t.Errorf("UserPromptSubmit still there: %v", h)
	}
	if _, ok := h["PreToolUse"]; !ok {
		t.Error("PreToolUse lost")
	}
	if r := runInit(t, "--project", p, "--bin", "/x/standin", "--no-prompt-guard"); !strings.Contains(r.out, "Nothing to change") {
		t.Errorf("re-run changed something:\n%s", r.out)
	}
}
