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
var secretNames = []string{"mapping.json", "approval.json"}

// StoreDir returns the private store folder for a project, using the core
// rule (internal/store.PrivateDir): $STANDIN_HOME/projects/<name>-<hash>/,
// or private if set. It refuses a store inside the project.
func StoreDir(project, private string) (string, error) {
	return corestore.PrivateDir(project, private, os.Getenv("STANDIN_HOME"))
}

// FindProject returns the project root. Order: CLAUDE_PROJECT_DIR (Claude
// Code sets it to the folder whose settings it loaded), then the --project
// flag (written by standin 0.1.0), then the nearest folder at or above cwd
// that holds standin.config.json. A place that does not exist is skipped, so
// a moved folder still works; if none is found it is an error (decision 34).
func FindProject(flagDir, cwd string) (string, error) {
	if env := os.Getenv("CLAUDE_PROJECT_DIR"); env != "" {
		if d, err := checkDir(env); err == nil {
			return d, nil
		}
	}
	if flagDir != "" {
		if d, err := checkDir(flagDir); err == nil {
			return d, nil
		}
	}
	if cwd == "" {
		return "", errors.New("cannot find the project folder, and the hook input has no cwd")
	}
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ConfigName)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("cannot find the project folder: no " + ConfigName + " in this folder or above it")
		}
		dir = parent
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
	}{{ZoneRaw, g.raw()}, {ZoneRestored, g.restored()}, {ZoneStore, g.Store}}
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
