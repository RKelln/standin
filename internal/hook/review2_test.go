package hook

// Fixes from the second fresh review (DESIGN decisions 46-52). Invented values only.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// F3: the whole private-folders root is guarded, so another project's
// private folder (say, one left behind by a move) cannot be read or searched.
func TestHomeRootGuarded(t *testing.T) {
	f := setup(t)
	other := filepath.Join(f.home, "projects", "shop-0badc0de")
	must(t, os.MkdirAll(other, 0o700))
	write(t, filepath.Join(other, "notes.txt"), "x")
	p := f.project
	j := func(tool string, in any) string { return hookJSON("PreToolUse", p, tool, in, nil) }
	for _, c := range []struct{ name, stdin string }{
		{"read", j("Read", map[string]any{"file_path": filepath.Join(other, "notes.txt")})},
		{"grep", j("Grep", map[string]any{"pattern": "spellings", "path": other})},
		{"grep home", j("Grep", map[string]any{"pattern": "spellings", "path": f.home})},
		{"glob", j("Glob", map[string]any{"pattern": "*", "path": other})},
		{"ls", j("LS", map[string]any{"path": filepath.Join(f.home, "projects")})},
		{"bash", j("Bash", map[string]any{"command": "cat " + filepath.Join(other, "notes.txt")})},
		{"bash ls", j("Bash", map[string]any{"command": "ls " + f.home})},
	} {
		r := run([]string{"claude", "PreToolUse"}, c.stdin)
		record(r)
		if r.code != 2 {
			t.Errorf("%s: not blocked: %d %q", c.name, r.code, r.stderr)
		}
	}
	// The project itself is still usable.
	r := run([]string{"claude", "PreToolUse"}, j("Read", map[string]any{"file_path": filepath.Join(p, "notes", "a.md")}))
	if r.code != 0 {
		t.Errorf("a project read was blocked: %q", r.stderr)
	}
}

// An odd STANDIN_HOME that holds the project: only its projects/ folder is
// guarded, so the project still works.
func TestHomeHoldingProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths")
	}
	g := Guard{Project: "/a/b/proj", Store: "/a/b/projects/proj-1", Home: "/a/b"}
	if g.home() != "/a/b/projects" {
		t.Errorf("home: %q", g.home())
	}
	if z := g.ZoneOf("/a/b/proj/notes.md", "/a/b/proj"); z != ZoneNone {
		t.Errorf("project file in zone %v", z)
	}
	if z := g.ZoneOf("/a/b/projects/other-2/mapping.json", "/a/b/proj"); z != ZoneStore {
		t.Errorf("other store: %v", z)
	}
}

// F4: CLAUDE_PROJECT_DIR counts only with a config; configs inside raw/,
// shape/, scrubbed/ or restored/ of an outer project are skipped; two
// different projects block.
func TestFindProjectRules(t *testing.T) {
	f := setup(t)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	parent := filepath.Dir(f.project)

	// Env without a config: the walk up from it finds nothing, cwd wins.
	t.Setenv("CLAUDE_PROJECT_DIR", parent)
	if d, err := FindProject("", filepath.Join(f.project, "notes")); err != nil || d != f.project {
		t.Errorf("env without config: %q %v", d, err)
	}
	// Env below the project: walks up to it.
	t.Setenv("CLAUDE_PROJECT_DIR", filepath.Join(f.project, "notes"))
	if d, err := FindProject("", ""); err != nil || d != f.project {
		t.Errorf("env below the project: %q %v", d, err)
	}
	// A copy of a project inside scrubbed/ (or raw/) is not a project.
	inner := filepath.Join(f.project, ScrubbedDir, "copy")
	must(t, os.MkdirAll(filepath.Join(inner, "notes"), 0o755))
	write(t, filepath.Join(inner, ConfigName), `{"version":1,"files":[]}`)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	if d, err := FindProject("", filepath.Join(inner, "notes")); err != nil || d != f.project {
		t.Errorf("config in scrubbed/: %q %v", d, err)
	}
	t.Setenv("CLAUDE_PROJECT_DIR", inner)
	if d, err := FindProject("", inner); err != nil || d != f.project {
		t.Errorf("env in scrubbed/: %q %v", d, err)
	}
	// An old --project without a config is skipped.
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	if d, err := FindProject(parent, f.project); err != nil || d != f.project {
		t.Errorf("--project without config: %q %v", d, err)
	}
	// Two different projects: block.
	two := filepath.Join(parent, "two")
	must(t, os.MkdirAll(two, 0o755))
	write(t, filepath.Join(two, ConfigName), `{"version":1,"files":[]}`)
	t.Setenv("CLAUDE_PROJECT_DIR", two)
	if _, err := FindProject("", f.project); err == nil || !strings.Contains(err.Error(), "two different project folders") {
		t.Errorf("two projects: %v", err)
	}
	r := run([]string{"claude", "PreToolUse"}, hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": "notes/a.md"}, nil))
	record(r)
	if r.code != 2 {
		t.Errorf("two projects should block: %d %q", r.code, r.stderr)
	}
}
