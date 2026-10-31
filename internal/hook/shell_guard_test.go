package hook

// Shell guard cases, many from the fresh review (2026-10-30, decisions 25-27).
// Invented data only.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func bashPre(t *testing.T, f fixture, cmd string) result {
	t.Helper()
	r := run([]string{"claude", "PreToolUse", "--project", f.project},
		hookJSON("PreToolUse", f.project, "Bash", map[string]any{"command": cmd}, nil))
	record(r)
	return r
}

// The agent must not choose where the mapping goes (decision 25): with
// --private it could build a full mapping in a folder the hook does not know.
func TestBashPlaceFlagsDenied(t *testing.T) {
	f := setup(t)
	for _, cmd := range []string{
		"standin scrub --private /tmp/agent-owned",
		"standin scrub --private=/tmp/agent-owned",
		"standin scrub -private /tmp/agent-owned",
		"standin check --private /tmp/agent-owned scrubbed/fresh.csv",
		"standin scrub --project /tmp/other",
		"standin shape --project=. raw/fresh.csv",
		"standin scrub -project .",
	} {
		r := bashPre(t, f, cmd)
		if r.code != 2 || !strings.Contains(r.stderr, "--private or --project") {
			t.Errorf("%q: got %d %q, want a --private/--project refusal", cmd, r.code, r.stderr)
		}
	}
}

// Reading a folder the agent picked, with a wildcard and no secret name in it.
func TestBashCatOutsideFolderWithWildcardDenied(t *testing.T) {
	f := setup(t)
	if r := bashPre(t, f, "cat /tmp/agent-owned/*"); r.code != 2 {
		t.Errorf("allowed: cat of an agent-chosen folder with a wildcard")
	}
}

// Habits that read raw/ or restored/ without naming them (decision 27).
func TestBashSweepingReadsDenied(t *testing.T) {
	f := setup(t)
	for _, cmd := range []string{
		"grep -rn invoice .",
		"grep -R invoice",
		"grep --recursive invoice .",
		"cat */*.csv",
		"head -5 r*/fresh.csv",
		"head r?w/fresh.csv",
		"cat [r]aw/fresh.csv",
		"cat {raw,scrubbed}/fresh.csv",
		"find . -name '*.csv' -exec head {} +",
		"rg invoice",
		"ag invoice .",
		"ack invoice",
		"tree",
		"du -a .",
		"tar czf all.tgz .",
		"zip -r all.zip .",
		"rsync -a . /tmp/copy",
		"cp -r . /tmp/copy",
		"ls -R",
		"rg invoice scrubbed/ notes/",
		"cat scrubbed*/fresh.csv",
		"Get-ChildItem -Recurse",
	} {
		r := bashPre(t, f, cmd)
		if r.code != 2 {
			t.Errorf("%q: allowed, but it could read raw/ or restored/", cmd)
			continue
		}
		if !strings.Contains(r.stderr, "scrubbed/") || !strings.Contains(r.stderr, "guess") {
			t.Errorf("%q: message should say it is a guess and point to scrubbed/: %q", cmd, r.stderr)
		}
	}
}

func TestBashNormalWorkAllowed(t *testing.T) {
	f := setup(t)
	for _, cmd := range []string{
		"grep -rn invoice scrubbed/",
		"grep -rn invoice scrubbed/ shape/",
		"rg invoice scrubbed",
		"find scrubbed -name '*.csv'",
		"ls",
		"ls -ltr",
		"ls -la scrubbed",
		"cat scrubbed/fresh.csv",
		"cat scrubbed/*.csv",
		"head -5 shape/*.md",
		`grep "[PERSON_1]" scrubbed/fresh.csv`,
		`grep 'a.*b' scrubbed/fresh.csv`,
		"standin scrub",
		"standin scrub raw/fresh.csv",
		"standin shape raw/*.csv",
		"wc -l scrubbed/fresh.csv && echo done",
	} {
		if r := bashPre(t, f, cmd); r.code != 0 {
			t.Errorf("%q: denied (%q), but it only uses scrubbed/ or shape/", cmd, r.stderr)
		}
	}
}

// approve, restore, show and init are for the person only, in any wrapper (decision 25).
func TestBashPersonOnlyCommandsDenied(t *testing.T) {
	f := setup(t)
	for _, cmd := range []string{
		`yes yes | script -qc "standin approve" /dev/null`,
		"standin approve",
		"standin restore -",
		"standin restore scrubbed/report.md",
		"standin init",
		`sh -c 'standin approve'`,
		`bash -c "cd . && standin restore -"`,
		`expect -c 'spawn standin approve; send "yes\r"'`,
		"/usr/local/bin/standin approve",
		`"/opt/my tools/standin.exe" restore -`,
		`stand"in" approve`,
		`s\tandin approve`,
		"STANDIN.EXE Approve",
		"echo hi; standin init --harness claude",
		"standin show PERSON_9",
		"standin show [PERSON_9] [ORG_1]",
		`script -qc "standin show PERSON_9" /dev/null`,
	} {
		r := bashPre(t, f, cmd)
		if r.code != 2 || !strings.Contains(r.stderr, "Only the person") {
			t.Errorf("%q: got %d %q, want the person-only refusal", cmd, r.code, r.stderr)
		}
	}
	// Not standin: common commands with the same words still run.
	for _, cmd := range []string{"git restore scrubbed/x.md", "npm init -y", "standin scrub raw/init.csv", "git show HEAD", "standin shape raw/show.csv"} {
		if r := bashPre(t, f, cmd); r.code != 0 {
			t.Errorf("%q: denied (%q)", cmd, r.stderr)
		}
	}
}

func TestPowerShellPersonOnlyDenied(t *testing.T) {
	f := setup(t)
	r := run([]string{"claude", "PreToolUse", "--project", f.project},
		hookJSON("PreToolUse", f.project, "PowerShell", map[string]any{"command": `& "C:\Tools\standin.exe" approve`}, nil))
	record(r)
	if r.code != 2 {
		t.Fatalf("got %d", r.code)
	}
}

// "~" and $HOME are expanded before the path check.
func TestHomePathsIntoRaw(t *testing.T) {
	f := setup(t)
	home := filepath.Dir(f.project)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := os.Stat(filepath.Join(home, filepath.Base(f.project), "raw", "fresh.csv")); err != nil {
		t.Fatal(err)
	}
	g := Guard{Project: f.project, Store: f.store}
	for _, pre := range []string{"~", "$HOME", "${HOME}"} {
		p := pre + "/" + filepath.Base(f.project) + "/raw/fresh.csv"
		if z := g.ZoneOf(p, filepath.Join(f.project, "notes")); z != ZoneRaw {
			t.Errorf("ZoneOf(%q) = %v, want raw/", p, z)
		}
	}
	r := run([]string{"claude", "PreToolUse", "--project", f.project}, hookJSON("PreToolUse", f.project, "Grep",
		map[string]any{"pattern": "x", "path": "~/" + filepath.Base(f.project)}, nil))
	record(r)
	if r.code != 2 {
		t.Errorf("Grep on ~/project allowed")
	}
}

// The PostToolUse warning gives the agent one bit: no stand-in, no count (decision 26).
func TestPostToolUseWarningNamesNothing(t *testing.T) {
	f := setup(t)
	r := run([]string{"claude", "PostToolUse", "--project", f.project},
		hookJSON("PostToolUse", f.project, "Bash", map[string]any{"command": "x"},
			map[string]any{"tool_response": map[string]any{"stdout": "Zelda Quixote and zq@example.test and 4417123456"}}))
	record(r)
	if r.code != 0 {
		t.Fatalf("got %d %q", r.code, r.stderr)
	}
	var out map[string]map[string]string
	must(t, json.Unmarshal([]byte(r.stdout), &out))
	note := out["hookSpecificOutput"]["additionalContext"]
	if !strings.Contains(note, "may be in that") || !strings.Contains(note, "ask the person") && !strings.Contains(note, "Ask the person") {
		t.Errorf("warning wording: %q", note)
	}
	if regexp.MustCompile(`\[[A-Z]+_[0-9]+\]|[0-9]`).MatchString(note) {
		t.Errorf("warning names a stand-in or a number: %q", note)
	}
}
