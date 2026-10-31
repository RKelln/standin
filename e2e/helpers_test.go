// Package e2e holds black-box end-to-end tests of the standin binary.
//
// They build ./cmd/standin into a temp dir and run it with exec in a temp project
// folder, with STANDIN_HOME and HOME pointed at temp dirs. They use only the
// invented fixtures in testdata/ (made by tools/make_fixtures.py).
//
// If the binary cannot be built, the suite fails rather than silently skipping.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var (
	repoRoot string
	binPath  string
)

func TestMain(m *testing.M) {
	code := func() int {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e: cannot find working dir:", err)
			return 1
		}
		repoRoot = filepath.Dir(wd)
		if _, err := os.Stat(filepath.Join(repoRoot, "cmd", "standin")); err != nil {
			fmt.Fprintln(os.Stderr, "e2e: cmd/standin does not exist")
			return 1
		}
		dir, err := os.MkdirTemp("", "standin-e2e-bin-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e: cannot make temp dir:", err)
			return 1
		}
		defer os.RemoveAll(dir)
		name := "standin"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binPath = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/standin")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: cannot build standin: %v\n%s", err, out)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}

func needBinary(t *testing.T) {
	t.Helper()
	if binPath == "" {
		t.Fatal("standin test binary is missing")
	}
}

// ---- fixtures -------------------------------------------------------------

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v (run: uv run tools/make_fixtures.py)", name, err)
	}
	return b
}

func readLines(t *testing.T, name string) []string {
	var out []string
	for _, l := range strings.Split(string(fixture(t, name)), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}

func knownValues(t *testing.T) []string { return readLines(t, "known-values.txt") }

// ---- project --------------------------------------------------------------

type project struct {
	t       *testing.T
	dir     string // the project folder
	home    string // STANDIN_HOME
	userHom string // HOME (so nothing lands in the real ~)
	cwd     string // working dir for commands; default dir
	outputs []string
}

func newProject(t *testing.T) *project {
	t.Helper()
	needBinary(t)
	base := t.TempDir()
	if real, err := filepath.EvalSymlinks(base); err == nil {
		base = real
	}
	p := &project{t: t, dir: filepath.Join(base, "shop"), home: filepath.Join(base, "standin-home"),
		userHom: filepath.Join(base, "home")}
	for _, d := range []string{p.dir, filepath.Join(p.dir, "raw"), p.userHom} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// addRaw copies a fixture to raw/<as>.
func (p *project) addRaw(name, as string) {
	p.t.Helper()
	p.write(filepath.Join("raw", as), fixture(p.t, name))
}

func (p *project) write(rel string, b []byte) {
	p.t.Helper()
	full := filepath.Join(p.dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(full, b, 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) useConfig() { p.write("standin.config.json", fixture(p.t, "standin.config.json")) }

type result struct {
	args           []string
	code           int
	stdout, stderr string
}

func (r result) String() string {
	return fmt.Sprintf("standin %s -> exit %d\nstdout:\n%s\nstderr:\n%s", strings.Join(r.args, " "), r.code,
		r.stdout, r.stderr)
}

func (p *project) env() []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "STANDIN_HOME=") || strings.HasPrefix(e, "HOME=") ||
			strings.HasPrefix(e, "USERPROFILE=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "STANDIN_HOME="+p.home, "HOME="+p.userHom, "USERPROFILE="+p.userHom)
}

// run runs standin in the project folder with stdin from in (nil: empty pipe).
// Output of every command except restore is kept for the stdout/stderr leak check.
func (p *project) run(in []byte, args ...string) result {
	p.t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = p.dir
	if p.cwd != "" {
		cmd.Dir = p.cwd
	}
	cmd.Env = p.env()
	if in == nil {
		in = []byte{}
	}
	cmd.Stdin = bytes.NewReader(in)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			p.t.Fatalf("run standin %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	r := result{args: args, code: code, stdout: so.String(), stderr: se.String()}
	if len(args) > 0 && args[0] != "restore" {
		p.outputs = append(p.outputs, r.stdout, r.stderr)
	}
	return r
}

func (p *project) must(want int, in []byte, args ...string) result {
	p.t.Helper()
	r := p.run(in, args...)
	if r.code != want {
		p.t.Fatalf("want exit %d\n%s", want, r)
	}
	return r
}

// wantCode accepts any of the given exit codes.
func (p *project) wantCode(r result, codes ...int) {
	p.t.Helper()
	for _, c := range codes {
		if r.code == c {
			return
		}
	}
	p.t.Errorf("want exit in %v\n%s", codes, r)
}

// privateDir is where the contract says the secret store lives.
func (p *project) privateDir() string {
	abs, _ := filepath.Abs(p.dir)
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(p.home, "projects", filepath.Base(p.dir)+"-"+hex.EncodeToString(sum[:])[:8])
}

// findPrivateDir finds the store even if the CLI hashed a symlink-resolved path.
func (p *project) findPrivateDir() string {
	p.t.Helper()
	want := p.privateDir()
	if _, err := os.Stat(want); err == nil {
		return want
	}
	m, _ := filepath.Glob(filepath.Join(p.home, "projects", filepath.Base(p.dir)+"-*"))
	if len(m) == 1 {
		p.t.Logf("private dir is %s, contract formula gives %s (symlinks in temp path?)", m[0], want)
		return m[0]
	}
	p.t.Fatalf("no private dir at %s (found %v)", want, m)
	return ""
}

// find returns the one file under dir (relative to project) whose base name is base.
// The contract says "<same path>" without saying whether raw/ or scrubbed/ is kept.
func (p *project) find(dir, base string) string {
	p.t.Helper()
	var hits []string
	filepath.WalkDir(filepath.Join(p.dir, dir), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == base {
			hits = append(hits, path)
		}
		return nil
	})
	if len(hits) != 1 {
		p.t.Fatalf("want one %s under %s/, found %v", base, dir, hits)
	}
	return hits[0]
}

func (p *project) files(dir string) []string {
	var out []string
	filepath.WalkDir(filepath.Join(p.dir, dir), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func (p *project) noFilesIn(dirs ...string) {
	p.t.Helper()
	for _, d := range dirs {
		if f := p.files(d); len(f) > 0 {
			p.t.Errorf("want no output in %s/, found %v", d, f)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---- leak checks ----------------------------------------------------------

var spaceRun = regexp.MustCompile(`\s+`)

func norm(s string) string { return spaceRun.ReplaceAllString(strings.ToLower(s), " ") }

// leaks returns the known values found in text (case-insensitive, whitespace collapsed).
func leaks(text string, values []string) []string {
	h := norm(text)
	var out []string
	for _, v := range values {
		if strings.Contains(h, norm(v)) {
			out = append(out, v)
		}
	}
	return out
}

func noLeaks(t *testing.T, what, text string, values []string) {
	t.Helper()
	if l := leaks(text, values); len(l) > 0 {
		t.Errorf("%s contains real values: %q", what, l)
	}
}

// noLeaksInDirs checks every file under the given project folders.
func (p *project) noLeaksInDirs(values []string, dirs ...string) {
	p.t.Helper()
	for _, d := range dirs {
		for _, f := range p.files(d) {
			noLeaks(p.t, f, readFile(p.t, f), values)
		}
	}
}

// noLeaksInOutput checks the stdout and stderr of every command run so far (not restore).
func (p *project) noLeaksInOutput(values []string) {
	p.t.Helper()
	// Random test-directory digits or the path hash can coincidentally contain
	// a short fixture amount. Exclude these exact generated metadata paths,
	// while continuing to check every filename, message and data value.
	out := strings.Join(p.outputs, "\n")
	out = strings.ReplaceAll(out, p.privateDir(), "(test private directory)")
	out = strings.ReplaceAll(out, filepath.Dir(p.dir), "(test directory)")
	noLeaks(p.t, "stdout/stderr of standin commands", out, values)
}

// ---- CSV ------------------------------------------------------------------

func parseCSV(t *testing.T, text string) []map[string]string {
	t.Helper()
	text = strings.TrimPrefix(text, "\ufeff")
	r := csv.NewReader(strings.NewReader(text))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("empty CSV")
	}
	var out []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, h := range rows[0] {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		out = append(out, m)
	}
	return out
}

var tokenRE = regexp.MustCompile(`\[[A-Z]+_[1-9][0-9]*\]`)
var wholeToken = regexp.MustCompile(`^\[[A-Z]+_[1-9][0-9]*\]$`)

// normKey is the contract's normalisation: lowercase, trim, collapse whitespace,
// strip trailing .,;:
func normKey(s string) string {
	s = strings.TrimSpace(spaceRun.ReplaceAllString(strings.ToLower(s), " "))
	return strings.TrimRight(s, ".,;:")
}
