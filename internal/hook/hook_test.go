package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Invented values only. Every one of these must never appear in any output.
var secretValues = []string{
	"Zelda Quixote", "zq@example.test", "4417123456", "Brindlewood Ferments",
}

const testMapping = `{
  "version": 1,
  "note": "test mapping, invented values",
  "standins": [
    {"standin": "[PERSON_1]", "category": "PERSON", "key": "zelda quixote", "spellings": ["Zelda Quixote", "zelda  quixote"]},
    {"standin": "[EMAIL_1]", "category": "EMAIL", "key": "zq@example.test", "spellings": ["zq@example.test"]},
    {"standin": "[ACCOUNT_1]", "category": "ACCOUNT", "key": "4417123456", "spellings": ["4417123456"]},
    {"standin": "[ORG_1]", "category": "ORG", "key": "brindlewood ferments", "spellings": ["Brindlewood Ferments"]}
  ]
}`

type fixture struct {
	project, store, home string
}

func setup(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	// Resolve symlinks in the temp path (macOS /var -> /private/var) so messages match.
	root, _ = filepath.EvalSymlinks(root)
	f := fixture{project: filepath.Join(root, "proj"), home: filepath.Join(root, "home")}
	t.Setenv("STANDIN_HOME", f.home)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	for _, d := range []string{RawDir, ScrubbedDir, RestoredDir, ShapeDir, "notes"} {
		must(t, os.MkdirAll(filepath.Join(f.project, d), 0o755))
	}
	write(t, filepath.Join(f.project, ConfigName), `{"version":1,"files":[]}`)
	old := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	// fresh: scrubbed copy is newer than raw.
	write(t, filepath.Join(f.project, RawDir, "fresh.csv"), "Customer\nZelda Quixote\n")
	write(t, filepath.Join(f.project, ScrubbedDir, "fresh.csv"), "Customer\n[PERSON_1]\n")
	must(t, os.Chtimes(filepath.Join(f.project, RawDir, "fresh.csv"), old, old))
	must(t, os.Chtimes(filepath.Join(f.project, ScrubbedDir, "fresh.csv"), newer, newer))
	// stale: raw changed after the scrub.
	write(t, filepath.Join(f.project, RawDir, "stale.csv"), "x\n")
	write(t, filepath.Join(f.project, ScrubbedDir, "stale.csv"), "x\n")
	must(t, os.Chtimes(filepath.Join(f.project, RawDir, "stale.csv"), newer, newer))
	must(t, os.Chtimes(filepath.Join(f.project, ScrubbedDir, "stale.csv"), old, old))
	// none: never scrubbed.
	write(t, filepath.Join(f.project, RawDir, "none.csv"), "x\n")
	must(t, os.MkdirAll(filepath.Join(f.project, RawDir, "sub"), 0o755))
	write(t, filepath.Join(f.project, RawDir, "sub", "deep.json"), "[]")
	write(t, filepath.Join(f.project, ScrubbedDir, "sub", "deep.json"), "[]")
	must(t, os.Chtimes(filepath.Join(f.project, RawDir, "sub", "deep.json"), old, old))

	store, err := StoreDir(f.project, "")
	must(t, err)
	f.store = store
	must(t, os.MkdirAll(store, 0o700))
	write(t, filepath.Join(store, "mapping.json"), testMapping)
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte(s), 0o644))
}

func hookJSON(event, cwd, tool string, input any, extra map[string]any) string {
	m := map[string]any{"session_id": "s1", "hook_event_name": event, "cwd": cwd}
	if tool != "" {
		m["tool_name"] = tool
		m["tool_input"] = input
	}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

type result struct {
	code           int
	stdout, stderr string
}

func run(args []string, stdin string) result {
	var o, e bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &o, &e)
	return result{code, o.String(), e.String()}
}

// allOutput collects every output the tests make, for the leak check.
var allOutput []string

func record(r result) { allOutput = append(allOutput, r.stdout, r.stderr) }

func TestPreToolUse(t *testing.T) {
	f := setup(t)
	p := f.project
	pre := []string{"claude", "PreToolUse", "--project", p}
	j := func(tool string, in any) string { return hookJSON("PreToolUse", p, tool, in, nil) }
	cases := []struct {
		name     string
		stdin    string
		code     int
		redirect string // expected scrubbed path on redirect
		inErr    string // substring of stderr
	}{
		{"read scrubbed ok", j("Read", map[string]any{"file_path": filepath.Join(p, "scrubbed/fresh.csv")}), 0, "", ""},
		{"read notes ok", j("Read", map[string]any{"file_path": "notes/a.md"}), 0, "", ""},
		{"read raw redirect", j("Read", map[string]any{"file_path": filepath.Join(p, "raw/fresh.csv"), "offset": 10, "limit": 5}), 0, filepath.Join(p, "scrubbed/fresh.csv"), ""},
		{"read raw relative redirect", j("Read", map[string]any{"file_path": "raw/fresh.csv"}), 0, filepath.Join(p, "scrubbed/fresh.csv"), ""},
		{"read raw nested redirect", j("Read", map[string]any{"file_path": "raw/sub/deep.json"}), 0, filepath.Join(p, "scrubbed/sub/deep.json"), ""},
		{"read raw stale", j("Read", map[string]any{"file_path": "raw/stale.csv"}), 2, "", "standin scrub"},
		{"read raw none", j("Read", map[string]any{"file_path": "raw/none.csv"}), 2, "", "standin scrub"},
		{"read raw dir", j("Read", map[string]any{"file_path": "raw"}), 2, "", "scrubbed/"},
		{"read raw dotdot", j("Read", map[string]any{"file_path": "scrubbed/../raw/none.csv"}), 2, "", "standin scrub"},
		{"read restored", j("Read", map[string]any{"file_path": "restored/out.md"}), 2, "", "restored/"},
		{"read store", j("Read", map[string]any{"file_path": filepath.Join(f.store, "mapping.json")}), 2, "", "secret"},
		{"read mapping copy", j("Read", map[string]any{"file_path": "notes/mapping.json"}), 2, "", "secret"},
		{"read no path", j("Read", map[string]any{}), 2, "", "no file path"},
		{"read path not string", j("Read", map[string]any{"file_path": 3}), 2, "", "no file path"},
		{"notebookread raw", j("NotebookRead", map[string]any{"notebook_path": "raw/n.ipynb"}), 2, "", "scrubbed/"},
		{"grep no path", j("Grep", map[string]any{"pattern": "x"}), 2, "", "scrubbed"},
		{"grep root", j("Grep", map[string]any{"pattern": "x", "path": p}), 2, "", "raw/"},
		{"grep raw", j("Grep", map[string]any{"pattern": "x", "path": "raw"}), 2, "", "raw/"},
		{"grep restored", j("Grep", map[string]any{"pattern": "x", "path": "restored"}), 2, "", "restored/"},
		{"grep home covers store", j("Grep", map[string]any{"pattern": "x", "path": f.home}), 2, "", "private store"},
		{"grep scrubbed ok", j("Grep", map[string]any{"pattern": "x", "path": "scrubbed"}), 0, "", ""},
		{"grep glob escapes", j("Grep", map[string]any{"pattern": "x", "path": "scrubbed", "glob": "../raw/*"}), 2, "", "glob"},
		{"glob no path", j("Glob", map[string]any{"pattern": "**/*.csv"}), 2, "", "scrubbed"},
		{"glob scrubbed ok", j("Glob", map[string]any{"pattern": "**/*.csv", "path": "scrubbed"}), 0, "", ""},
		{"glob dotdot", j("Glob", map[string]any{"pattern": "../raw/*.csv", "path": "scrubbed"}), 2, "", ".."},
		{"glob absolute", j("Glob", map[string]any{"pattern": filepath.Join(p, "raw/*"), "path": "scrubbed"}), 2, "", "top of the disk"},
		{"glob path not string", j("Glob", map[string]any{"pattern": "*", "path": []int{1}}), 2, "", "not text"},
		{"ls raw", j("LS", map[string]any{"path": "raw"}), 2, "", "raw/"},
		{"edit raw", j("Edit", map[string]any{"file_path": "raw/fresh.csv", "old_string": "a", "new_string": "b"}), 2, "", "Do not change"},
		{"write restored", j("Write", map[string]any{"file_path": "restored/x.md", "content": "hi"}), 2, "", "Do not change"},
		{"write store", j("Write", map[string]any{"file_path": filepath.Join(f.store, "mapping.json"), "content": "{}"}), 2, "", "Do not change"},
		{"multiedit raw", j("MultiEdit", map[string]any{"file_path": "raw/a.csv", "edits": []any{}}), 2, "", "Do not change"},
		{"notebookedit raw", j("NotebookEdit", map[string]any{"notebook_path": "raw/a.ipynb", "new_source": ""}), 2, "", "Do not change"},
		{"write scrubbed ok", j("Write", map[string]any{"file_path": "scrubbed/report.md", "content": "[PERSON_1]"}), 0, "", ""},
		{"write no path", j("Write", map[string]any{"content": "x"}), 2, "", "no file path"},
		{"bash cat raw", j("Bash", map[string]any{"command": "cat raw/fresh.csv"}), 2, "", "not a wall"},
		{"bash ls raw", j("Bash", map[string]any{"command": "ls raw"}), 2, "", "raw/"},
		{"bash abs raw", j("Bash", map[string]any{"command": "head " + filepath.Join(p, "raw", "x.csv")}), 2, "", "raw/"},
		{"bash quoted raw", j("Bash", map[string]any{"command": `python3 -c "open('raw/x.csv')"`}), 2, "", "raw/"},
		{"bash restored", j("Bash", map[string]any{"command": "grep -r x ./restored"}), 2, "", "restored/"},
		{"bash mapping", j("Bash", map[string]any{"command": "find / -name mapping.json"}), 2, "", "mapping.json"},
		{"bash store", j("Bash", map[string]any{"command": "ls " + f.store}), 2, "", "private store"},
		{"bash dot standin", j("Bash", map[string]any{"command": "ls ~/.standin/projects"}), 2, "", "private store"},
		{"bash restore", j("Bash", map[string]any{"command": "standin restore -"}), 2, "", "Only the person"},
		{"bash scrubbed ok", j("Bash", map[string]any{"command": "wc -l scrubbed/fresh.csv && echo drawing"}), 0, "", ""},
		{"bash standin check ok", j("Bash", map[string]any{"command": "standin check scrubbed/out.md"}), 0, "", ""},
		{"bash standin scrub raw ok", j("Bash", map[string]any{"command": "standin scrub raw/fresh.csv"}), 0, "", ""},
		{"bash standin shape raw ok", j("Bash", map[string]any{"command": "standin shape raw/a.csv raw/b.csv"}), 0, "", ""},
		{"bash standin shape project flag", j("Bash", map[string]any{"command": "standin shape --project . raw/a.csv"}), 2, "", "--project"},
		{"bash standin then cat", j("Bash", map[string]any{"command": "standin scrub raw/a.csv; cat raw/a.csv"}), 2, "", "raw/"},
		{"bash standin pipe", j("Bash", map[string]any{"command": "standin check raw/a.csv | tee x"}), 2, "", "raw/"},
		{"bash standin subst", j("Bash", map[string]any{"command": "standin scrub $(cat raw/a.csv)"}), 2, "", "raw/"},
		{"bash fake standin path", j("Bash", map[string]any{"command": "./standin scrub raw/a.csv"}), 2, "", "raw/"},
		{"bash standin restore raw", j("Bash", map[string]any{"command": "standin restore raw/a.csv"}), 2, "", ""},
		{"bash standin mapping", j("Bash", map[string]any{"command": "standin check mapping.json"}), 2, "", "mapping.json"},
		{"bash no command", j("Bash", map[string]any{}), 2, "", "no command"},
		{"powershell raw", j("PowerShell", map[string]any{"command": `Get-Content raw\x.csv`}), 2, "", "raw/"},
		{"unknown tool", j("WebFetch", map[string]any{"url": "x"}), 2, "", "does not know the tool"},
		{"tool input not object", hookJSON("PreToolUse", p, "Read", "raw/x.csv", nil), 2, "", "not a JSON object"},
		{"tool input missing", `{"hook_event_name":"PreToolUse","cwd":"` + jsonEsc(p) + `","tool_name":"Read"}`, 2, "", "not a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(pre, tc.stdin)
			record(r)
			if r.code != tc.code {
				t.Fatalf("code %d, want %d; stderr %q stdout %q", r.code, tc.code, r.stderr, r.stdout)
			}
			if tc.inErr != "" && !strings.Contains(r.stderr, tc.inErr) {
				t.Errorf("stderr %q lacks %q", r.stderr, tc.inErr)
			}
			if tc.code == 2 && r.stdout != "" {
				t.Errorf("block wrote stdout %q", r.stdout)
			}
			if tc.redirect == "" {
				if tc.code == 0 && r.stdout != "" {
					t.Errorf("unexpected stdout %q", r.stdout)
				}
				return
			}
			var out struct {
				H struct {
					Event   string         `json:"hookEventName"`
					Dec     string         `json:"permissionDecision"`
					Updated map[string]any `json:"updatedInput"`
					Ctx     string         `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			must(t, json.Unmarshal([]byte(r.stdout), &out))
			if out.H.Event != "PreToolUse" || out.H.Dec != "allow" || out.H.Updated["file_path"] != tc.redirect {
				t.Fatalf("bad redirect %s", r.stdout)
			}
			if !strings.Contains(out.H.Ctx, "scrubbed") {
				t.Errorf("no context note: %q", out.H.Ctx)
			}
			var in map[string]any
			var whole map[string]json.RawMessage
			must(t, json.Unmarshal([]byte(tc.stdin), &whole))
			must(t, json.Unmarshal(whole["tool_input"], &in))
			for k, v := range in {
				if k == "file_path" {
					continue
				}
				if got := out.H.Updated[k]; got != v {
					t.Errorf("field %s not passed through: %v vs %v", k, got, v)
				}
			}
		})
	}
}

func jsonEsc(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func TestMalformedAndArgs(t *testing.T) {
	f := setup(t)
	p := f.project
	good := hookJSON("PreToolUse", p, "Read", map[string]any{"file_path": "scrubbed/fresh.csv"}, nil)
	cases := []struct {
		name  string
		args  []string
		stdin string
	}{
		{"no args", nil, good},
		{"one arg", []string{"claude"}, good},
		{"three args", []string{"claude", "PreToolUse", "extra"}, good},
		{"bad flag", []string{"claude", "PreToolUse", "--nope"}, good},
		{"unknown harness", []string{"codex", "PreToolUse"}, good},
		{"unknown event", []string{"claude", "Stop"}, good},
		{"empty stdin", []string{"claude", "PreToolUse"}, ""},
		{"not json", []string{"claude", "PreToolUse"}, "hello"},
		{"truncated", []string{"claude", "PreToolUse"}, good[:len(good)-5]},
		{"two objects", []string{"claude", "PreToolUse"}, good + good},
		{"array", []string{"claude", "PreToolUse"}, "[1,2]"},
		{"event mismatch", []string{"claude", "PostToolUse"}, good},
		{"event missing", []string{"claude", "PreToolUse"}, `{"cwd":"` + jsonEsc(p) + `","tool_name":"Read","tool_input":{"file_path":"a"}}`},
		{"no project found", []string{"claude", "PreToolUse"}, hookJSON("PreToolUse", t.TempDir(), "Read", map[string]any{"file_path": "a"}, nil)},
		{"project missing and none found", []string{"claude", "PreToolUse", "--project", filepath.Join(p, "nope")}, hookJSON("PreToolUse", t.TempDir(), "Read", map[string]any{"file_path": "a"}, nil)},
		{"prompt missing", []string{"claude", "UserPromptSubmit"}, hookJSON("UserPromptSubmit", p, "", nil, nil)},
		{"prompt not string", []string{"claude", "UserPromptSubmit"}, hookJSON("UserPromptSubmit", p, "", nil, map[string]any{"prompt": 5})},
		{"post no response", []string{"claude", "PostToolUse"}, hookJSON("PostToolUse", p, "Read", map[string]any{"file_path": "a"}, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(tc.args, tc.stdin)
			record(r)
			if r.code != 2 {
				t.Fatalf("code %d, want 2; stderr %q", r.code, r.stderr)
			}
			if strings.TrimSpace(r.stderr) == "" {
				t.Error("no reason on stderr")
			}
			if r.stdout != "" {
				t.Errorf("stdout should be empty, got %q", r.stdout)
			}
		})
	}
}

func TestProjectFromCwd(t *testing.T) {
	f := setup(t)
	// No --project: found by walking up from cwd.
	r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", filepath.Join(f.project, "notes"), "Read", map[string]any{"file_path": "../raw/none.csv"}, nil))
	record(r)
	if r.code != 2 || !strings.Contains(r.stderr, "standin scrub") {
		t.Fatalf("got %d %q", r.code, r.stderr)
	}
	// CLAUDE_PROJECT_DIR is used when set.
	t.Setenv("CLAUDE_PROJECT_DIR", f.project)
	r = run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", "", "Read", map[string]any{"file_path": filepath.Join(f.project, "raw/fresh.csv")}, nil))
	record(r)
	if r.code != 0 || !strings.Contains(r.stdout, "updatedInput") {
		t.Fatalf("got %d %q %q", r.code, r.stdout, r.stderr)
	}
}

// 34: settings from 0.1.0 pass --project with the old place. After a move
// the hook finds the project from CLAUDE_PROJECT_DIR or cwd and still guards
// raw/; with neither, it blocks and names the fix.
func TestMovedProject(t *testing.T) {
	f := setup(t)
	gone := filepath.Join(t.TempDir(), "old-place", "shop")
	args := []string{"claude", "PreToolUse", "--project", gone}
	read := func(cwd, path string) result {
		r := run(args, hookJSON("PreToolUse", cwd, "Read", map[string]any{"file_path": path}, nil))
		record(r)
		return r
	}
	// Found from cwd: raw/ is still guarded, scrubbed/ still readable.
	if r := read(f.project, "raw/none.csv"); r.code != 2 || !strings.Contains(r.stderr, "standin scrub") {
		t.Errorf("raw read after a move: %d %q", r.code, r.stderr)
	}
	if r := read(f.project, "scrubbed/fresh.csv"); r.code != 0 {
		t.Errorf("scrubbed read after a move: %d %q", r.code, r.stderr)
	}
	// Found from CLAUDE_PROJECT_DIR, even when cwd is elsewhere.
	t.Setenv("CLAUDE_PROJECT_DIR", f.project)
	if r := read(t.TempDir(), filepath.Join(f.project, "raw", "fresh.csv")); r.code != 0 || !strings.Contains(r.stdout, "updatedInput") {
		t.Errorf("env: %d %q %q", r.code, r.stdout, r.stderr)
	}
	// An env that does not exist is skipped too; nothing found: block, name the fix.
	t.Setenv("CLAUDE_PROJECT_DIR", gone)
	r := read(t.TempDir(), "a.txt")
	if r.code != 2 || !strings.Contains(r.stderr, "run standin init again in this folder") {
		t.Errorf("nothing found: %d %q", r.code, r.stderr)
	}
}

func TestSymlinkIntoRaw(t *testing.T) {
	f := setup(t)
	link := filepath.Join(f.project, "notes", "sneaky.csv")
	if err := os.Symlink(filepath.Join(f.project, RawDir, "none.csv"), link); err != nil {
		t.Skip("no symlinks here")
	}
	r := run([]string{"claude", "PreToolUse", "--project", f.project}, hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": link}, nil))
	record(r)
	if r.code != 2 {
		t.Fatalf("symlink into raw was allowed: %d", r.code)
	}
}

func TestUserPrompt(t *testing.T) {
	f := setup(t)
	args := []string{"claude", "UserPromptSubmit", "--project", f.project}
	cases := []struct {
		name   string
		prompt string
		code   int
		want   []string
	}{
		{"clean", "Please list overdue invoices for [PERSON_1] in scrubbed/fresh.csv.", 0, nil},
		{"tokens only", "Match [ACCOUNT_1] to [EMAIL_1] and [ORG_1].", 0, nil},
		{"known name", "Did Zelda Quixote pay?", 2, []string{"[PERSON_1]", "typed or pasted into the chat", "standin init --no-prompt-guard"}},
		{"known name other case and spaces", "did ZELDA   quixote pay", 2, []string{"[PERSON_1]"}},
		{"known org", "Email brindlewood ferments today", 2, []string{"[ORG_1]"}},
		{"known account", "acct 4417123456 is late", 2, []string{"[ACCOUNT_1]"}},
		{"name inside word no hit", "Zelda Quixoteville is a town", 0, nil},
		{"unknown email pattern", "write to someone@invented.test", 2, []string{"email"}},
		{"phone pattern", "call 416-555-0199 now", 2, []string{"phone"}},
		{"card pattern", "card 4111 1111 1111 1111", 2, []string{"card"}},
		{"long digits", "ref 98765432101", 2, []string{"long number"}},
		{"postal", "lives at M5V 2T6", 2, []string{"postal"}},
		{"date and amount fine", "On 2026-10-30 we got 1,234.56", 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(args, hookJSON("UserPromptSubmit", f.project, "", nil, map[string]any{"prompt": tc.prompt}))
			record(r)
			if r.code != tc.code {
				t.Fatalf("code %d want %d: %q", r.code, tc.code, r.stderr)
			}
			for _, w := range tc.want {
				if !strings.Contains(r.stderr, w) {
					t.Errorf("stderr %q lacks %q", r.stderr, w)
				}
			}
		})
	}
}

func TestPostToolUse(t *testing.T) {
	f := setup(t)
	args := []string{"claude", "PostToolUse", "--project", f.project}
	post := func(resp any) string {
		return hookJSON("PostToolUse", f.project, "Bash", map[string]any{"command": "x"}, map[string]any{"tool_response": resp})
	}
	cases := []struct {
		name  string
		stdin string
		warn  bool
		code  int
	}{
		{"clean", post(map[string]any{"stdout": "[PERSON_1] owes 10.00", "stderr": ""}), false, 0},
		{"leak in stdout", post(map[string]any{"stdout": "Zelda Quixote owes 10.00"}), true, 0},
		{"leak in nested", post(map[string]any{"file": map[string]any{"content": "zq@example.test"}}), true, 0},
		{"leak as number", post(map[string]any{"n": 4417123456}), true, 0},
		{"plain string", post("ok"), false, 0},
		{"pattern only does not warn", post("call 416-555-0199"), false, 0},
		{"bad response json", strings.Replace(post("ok"), `"tool_response":"ok"`, `"tool_response":`, 1), false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(args, tc.stdin)
			record(r)
			if r.code != tc.code {
				t.Fatalf("code %d want %d: %q", r.code, tc.code, r.stderr)
			}
			if tc.warn != strings.Contains(r.stdout, "additionalContext") {
				t.Fatalf("warn=%v, stdout %q", tc.warn, r.stdout)
			}
			if tc.warn {
				var out map[string]map[string]string
				must(t, json.Unmarshal([]byte(r.stdout), &out))
				if out["hookSpecificOutput"]["hookEventName"] != "PostToolUse" {
					t.Errorf("bad output %q", r.stdout)
				}
			}
		})
	}
}

func TestBadMapping(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      "{nope",
		"wrong version": `{"version": 9, "standins": []}`,
		"unknown field": `{"version": 1, "standins": [], "extra": 1}`,
		"bad row":       `{"version": 1, "standins": [{"standin": "PERSON_1", "category": "PERSON", "key": "a", "spellings": ["a"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			write(t, filepath.Join(f.store, "mapping.json"), body)
			for _, ev := range []string{"UserPromptSubmit", "PostToolUse"} {
				extra := map[string]any{"prompt": "hello", "tool_response": "ok"}
				tool, in := "", any(nil)
				if ev == "PostToolUse" {
					tool, in = "Bash", map[string]any{"command": "ls"}
				}
				r := run([]string{"claude", ev, "--project", f.project}, hookJSON(ev, f.project, tool, in, extra))
				record(r)
				if r.code != 2 || !strings.Contains(r.stderr, "cannot check") {
					t.Fatalf("%s: got %d %q", ev, r.code, r.stderr)
				}
			}
		})
	}
}

func TestNoMappingYet(t *testing.T) {
	f := setup(t)
	if err := os.Remove(filepath.Join(f.store, "mapping.json")); err != nil {
		t.Fatal(err)
	}
	args := []string{"claude", "UserPromptSubmit", "--project", f.project}
	r := run(args, hookJSON("UserPromptSubmit", f.project, "", nil, map[string]any{"prompt": "Did Zelda Quixote pay?"}))
	record(r)
	if r.code != 0 {
		t.Fatalf("no mapping: names cannot be known, want 0, got %d %q", r.code, r.stderr)
	}
	r = run(args, hookJSON("UserPromptSubmit", f.project, "", nil, map[string]any{"prompt": "mail zq@example.test"}))
	record(r)
	if r.code != 2 || !strings.Contains(r.stderr, "email") {
		t.Fatalf("no mapping: patterns still apply, got %d %q", r.code, r.stderr)
	}
}

// Runs last by name order within this file is not guaranteed, so it runs every
// other test itself first, then checks every output it saw.
func TestNoValueEverPrinted(t *testing.T) {
	for _, fn := range []func(*testing.T){TestPreToolUse, TestMalformedAndArgs, TestProjectFromCwd, TestMovedProject, TestSymlinkIntoRaw, TestUserPrompt, TestPostToolUse, TestBadMapping, TestNoMappingYet, TestBashPlaceFlagsDenied, TestBashCatOutsideFolderWithWildcardDenied, TestBashSweepingReadsDenied, TestBashNormalWorkAllowed, TestBashPersonOnlyCommandsDenied, TestPowerShellPersonOnlyDenied, TestHomePathsIntoRaw, TestPostToolUseWarningNamesNothing} {
		fn(t)
	}
	if len(allOutput) < 50 {
		t.Fatalf("too few outputs recorded: %d", len(allOutput))
	}
	for _, out := range allOutput {
		low := squash(out)
		for _, v := range secretValues {
			if strings.Contains(low, squash(v)) {
				t.Errorf("output leaks a test value: %q", out)
			}
		}
	}
}

func squash(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func TestToolMatcher(t *testing.T) {
	m := ToolMatcher()
	for _, want := range []string{"Read", "Grep", "Glob", "Edit", "Write", "NotebookEdit", "Bash", "mcp__files__read", "FutureFileTool"} {
		if !regexp.MustCompile(m).MatchString(want) {
			t.Errorf("matcher %q lacks %s", m, want)
		}
	}
}
