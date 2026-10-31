package hook

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	corestore "github.com/RKelln/standin/internal/store"
)

// ConfigName is the project config file. Its folder is the project root.
const ConfigName = "standin.config.json"

// Folder names inside a project (see docs/PLAN.md "Project folder").
const (
	RawDir      = "raw"
	ShapeDir    = "shape"
	ScrubbedDir = "scrubbed"
	RestoredDir = "restored"
)

// Secret file names. They live in the private store, but a copy anywhere is still the secret.
var secretNames = []string{"mapping.json", "approval.json", "words.json"}

// StoreDir returns the private store folder for a project, using the core
// rule (internal/store.PrivateDir): $STANDIN_HOME/projects/<name>-<hash>/,
// or private if set. It refuses a store inside the project.
func StoreDir(project, private string) (string, error) {
	return corestore.PrivateDir(project, private, os.Getenv("STANDIN_HOME"))
}

// HomeDir returns the folder that holds every private folder ($STANDIN_HOME,
// or ~/.standin), or "" if it cannot be found.
func HomeDir() string {
	r, err := corestore.Root(os.Getenv("STANDIN_HOME"))
	if err != nil {
		return ""
	}
	return r
}

// FindProject returns the project root (decision 34, refined in 47).
// CLAUDE_PROJECT_DIR counts only if it holds standin.config.json; else the
// walk starts there. Without it, an old --project flag (from standin 0.1.0)
// counts if that folder still holds the config. Then the walk up from cwd.
// A walk skips a config inside raw/, shape/, scrubbed/ or restored/ of an
// outer project (a copy of a project there must not become the project). If
// CLAUDE_PROJECT_DIR and cwd lead to two different projects, it is an error:
// the hook cannot tell which rules to use.
func FindProject(flagDir, cwd string) (string, error) {
	var fromEnv string
	if env := os.Getenv("CLAUDE_PROJECT_DIR"); env != "" {
		if d, err := checkDir(env); err == nil {
			fromEnv = walkUp(d)
		}
	}
	if fromEnv == "" && flagDir != "" {
		if d, err := checkDir(flagDir); err == nil && hasConfig(d) && !inOuterZone(d) {
			fromEnv = d
		}
	}
	var fromCwd string
	if cwd != "" {
		if d, err := filepath.Abs(cwd); err == nil {
			fromCwd = walkUp(filepath.Clean(d))
		}
	}
	switch {
	case fromEnv != "" && fromCwd != "" && !sameDir(fromEnv, fromCwd):
		return "", errors.New("found two different project folders (one from CLAUDE_PROJECT_DIR, one from the working folder), so it cannot tell which rules to use. Start Claude Code in the project folder, and work inside it")
	case fromEnv != "":
		return fromEnv, nil
	case fromCwd != "":
		return fromCwd, nil
	case cwd == "" && os.Getenv("CLAUDE_PROJECT_DIR") == "":
		return "", errors.New("cannot find the project folder, and the hook input has no cwd")
	}
	return "", errors.New("cannot find the project folder: no " + ConfigName + " in this folder or above it")
}

func hasConfig(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ConfigName))
	return err == nil && !st.IsDir()
}

func sameDir(a, b string) bool {
	if norm(a) == norm(b) {
		return true
	}
	ra, rb := resolve(a), resolve(b)
	return ra != "" && norm(ra) == norm(rb)
}

// walkUp finds the nearest folder at or above dir that holds the config and
// is not inside a zone folder of an outer project. "" if none.
func walkUp(dir string) string {
	for {
		if hasConfig(dir) && !inOuterZone(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// inOuterZone reports whether dir is inside raw/, shape/, scrubbed/ or
// restored/ of a project above it.
func inOuterZone(dir string) bool {
	for a := filepath.Dir(dir); ; a = filepath.Dir(a) {
		if rel, err := filepath.Rel(a, dir); err == nil && hasConfig(a) {
			first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
			switch norm(first) {
			case RawDir, ShapeDir, ScrubbedDir, RestoredDir:
				return true
			}
		}
		if filepath.Dir(a) == a {
			return false
		}
	}
}

func checkDir(d string) (string, error) {
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", errors.New("the project folder does not exist: " + abs)
	}
	return filepath.Clean(abs), nil
}

// Zone says which protected area a path is in.
type Zone int

const (
	ZoneNone     Zone = iota
	ZoneRaw           // raw/: real files
	ZoneRestored      // restored/: real values put back
	ZoneStore         // the private store
	ZoneSecret        // a file named like the mapping, anywhere
)

func (z Zone) String() string {
	switch z {
	case ZoneRaw:
		return "raw/"
	case ZoneRestored:
		return "restored/"
	case ZoneStore:
		return "the private store"
	case ZoneSecret:
		return "the mapping (the secret)"
	}
	return ""
}

// Guard knows the protected paths of one project.
type Guard struct {
	Project string
	Store   string
	// Home is the folder that holds every project's private folder
	// ($STANDIN_HOME, or ~/.standin). It is guarded whole, so the agent cannot
	// reach another project's mapping (say, one left behind by a move).
	Home string
}

// home is the private-folders root to guard. If the project is inside it
// (an odd STANDIN_HOME), only its projects/ folder is guarded, so the
// project itself stays usable.
func (g Guard) home() string {
	if g.Home == "" {
		return ""
	}
	h := filepath.Clean(g.Home)
	for _, b := range []string{h, filepath.Join(h, "projects")} {
		inside := false
		for _, pf := range forms(g.Project) {
			for _, bf := range forms(b) {
				if within(pf, bf) {
					inside = true
				}
			}
		}
		if !inside {
			return b
		}
	}
	return ""
}

func (g Guard) raw() string      { return filepath.Join(g.Project, RawDir) }
func (g Guard) restored() string { return filepath.Join(g.Project, RestoredDir) }

// abs makes p absolute against cwd (or the project) and cleans it.
func (g Guard) abs(p, cwd string) string {
	p = expandHome(p)
	if !filepath.IsAbs(p) {
		base := cwd
		if base == "" {
			base = g.Project
		}
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

// forms returns the path as given and, if different, with symlinks resolved
// (for the longest part that exists).
func forms(p string) []string {
	out := []string{p}
	if r := resolve(p); r != "" && r != p {
		out = append(out, r)
	}
	return out
}

func resolve(p string) string {
	rest := ""
	cur := p
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return r
			}
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		if rest == "" {
			rest = filepath.Base(cur)
		} else {
			rest = filepath.Join(filepath.Base(cur), rest)
		}
		cur = parent
	}
}

func foldCase() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

func norm(p string) string {
	if foldCase() {
		return strings.ToLower(p)
	}
	return p
}

// within reports whether p is base or inside it.
func within(p, base string) bool {
	p, base = norm(p), norm(base)
	if p == base {
		return true
	}
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (g Guard) bases() []struct {
	z Zone
	p string
} {
	return []struct {
		z Zone
		p string
	}{{ZoneRaw, g.raw()}, {ZoneRestored, g.restored()}, {ZoneStore, g.Store}, {ZoneStore, g.home()}}
}

// ZoneOf says which protected area the file path p is in (ZoneNone if none).
func (g Guard) ZoneOf(p, cwd string) Zone {
	a := g.abs(p, cwd)
	for _, f := range forms(a) {
		for _, b := range g.bases() {
			if b.p == "" {
				continue
			}
			for _, bf := range forms(b.p) {
				if within(f, bf) {
					return b.z
				}
			}
		}
		base := norm(filepath.Base(f))
		for _, s := range secretNames {
			if base == s {
				return ZoneSecret
			}
		}
	}
	return ZoneNone
}

// Covers says which protected area a search rooted at dir would reach: the
// root is inside one, or one is inside the root.
func (g Guard) Covers(dir, cwd string) Zone {
	if z := g.ZoneOf(dir, cwd); z != ZoneNone {
		return z
	}
	a := g.abs(dir, cwd)
	for _, f := range forms(a) {
		for _, b := range g.bases() {
			if b.p == "" {
				continue
			}
			for _, bf := range forms(b.p) {
				if within(bf, f) {
					return b.z
				}
			}
		}
	}
	return ZoneNone
}

// ScrubbedFor maps a path under raw/ to its scrubbed/ twin.
func (g Guard) ScrubbedFor(p, cwd string) (string, bool) {
	a := g.abs(p, cwd)
	rel, err := filepath.Rel(g.raw(), a)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		// Try the resolved form (raw/ reached through a symlink).
		r := resolve(a)
		rr := resolve(g.raw())
		if r == "" || rr == "" {
			return "", false
		}
		rel, err = filepath.Rel(rr, r)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", false
		}
	}
	return filepath.Join(g.Project, ScrubbedDir, rel), true
}

// expandHome turns a leading ~, $HOME or ${HOME} into the home folder, as a
// shell or a harness might, so a home-relative path is checked like any other.
func expandHome(p string) string {
	for _, pre := range []string{"~", "$HOME", "${HOME}"} {
		if p != pre && !strings.HasPrefix(p, pre+"/") && !strings.HasPrefix(p, pre+`\`) {
			continue
		}
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return p
		}
		return filepath.Join(home, p[len(pre):])
	}
	return p
}
