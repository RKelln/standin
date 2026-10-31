package cli

// Tests for the v0.2 fixes from the trial notes (DESIGN.md decisions 33-37).
// Invented data only.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTPAt makes a test project at <temp>/<rel>, where rel ends in "shop".
func newTPAt(t *testing.T, rel string) *tp {
	base := t.TempDir()
	p := &tp{t: t, dir: filepath.Join(base, filepath.FromSlash(rel)), home: filepath.Join(base, "home")}
	if err := os.MkdirAll(filepath.Join(p.dir, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *tp) mappingExists() bool {
	m, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "mapping.json"))
	return len(m) > 0
}

// No keep and no scan, so no approval is needed.
const plainCfg = `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"VALUE"}}}]}`

const bankScan = "Date,Description,Amount\n" +
	"2026-02-01,PRE-AUTH DEBIT LANDLORD RENT,-1200.00\n" +
	"2026-02-03,E-TRANSFER FROM ZELDA QUORN,85.00\n"

const bankScanCfg = `{"version":1,"files":[
 {"match":"raw/customers.csv","columns":{"Name":{"action":"tokenize","category":"PERSON"}}},
 {"match":"raw/bank.csv","columns":{"Date":{"action":"keep"},"Description":{"action":"scan"},"Amount":{"action":"keep"}}}]}`

// 33, the trial repro: scrub before approve, then approve, then scrub. In
// v0.1.0 the first scrub put whole descriptions in the mapping as VALUE
// stand-ins, and the approved scrub then swapped whole cells in the scan
// column. Now the first scrub refuses and writes no mapping, so the scan
// column comes out readable.
func TestUnapprovedScrubThenApproveKeepsScanReadable(t *testing.T) {
	p := newTP(t)
	p.write("raw/customers.csv", "Name\nZelda Quorn\n")
	p.write("raw/bank.csv", bankScan)
	p.write("standin.config.json", bankScanCfg)

	r := p.run("", false, "scrub")
	if r.code != 2 || !strings.Contains(r.stderr, "standin approve") {
		t.Fatalf("unapproved scrub should refuse: %+v", r)
	}
	if p.mappingExists() || p.exists("scrubbed") {
		t.Fatal("unapproved scrub wrote the mapping or output")
	}

	p.approve()
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("approved scrub: %+v", r)
	}
	got := p.read("scrubbed/bank.csv")
	want := "Date,Description,Amount\n" +
		"2026-02-01,PRE-AUTH DEBIT LANDLORD RENT,-1200.00\n" +
		"2026-02-03,E-TRANSFER FROM [PERSON_1],85.00\n"
	if got != want {
		t.Errorf("scan column:\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(p.read("scrubbed/bank.csv"), "[VALUE_") {
		t.Error("whole cells were swapped for VALUE stand-ins")
	}
}

// review says scrub will not run until approve.
func TestReviewSaysScrubWaits(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", bankScanCfg)
	r := p.run("", false, "review")
	if r.code != 1 || !strings.Contains(r.stdout, "will not run until you run `standin approve`") {
		t.Errorf("%+v", r)
	}
}

// 34: a moved project has no private folder under its new path. Commands say
// where the old one is and where to move it, and move nothing themselves.
func TestMovedProjectHint(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	oldDir := filepath.Join(base, "Downloads", "shop")
	newDir := filepath.Join(base, "Documents", "shop")
	at := func(dir string) *tp { return &tp{t: t, dir: dir, home: home} }

	old := at(oldDir)
	if err := os.MkdirAll(filepath.Join(oldDir, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	old.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	old.write("standin.config.json", plainCfg)
	if r := old.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	oldPriv := old.privDir()

	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	moved := at(newDir)
	moved.write("out/reply.md", "Hi [VALUE_1]\n")
	// Off a terminal (an agent reading a pipe), the hint names no private path.
	r := moved.run("", false, "restore", "out/reply.md")
	if r.code != 2 || strings.Contains(r.stderr, oldPriv) || strings.Contains(r.stderr, "from:") ||
		!strings.Contains(r.stderr, "standin review") {
		t.Fatalf("off a terminal the hint must not name the old folder: %+v", r)
	}
	r = moved.run("", true, "restore", "out/reply.md")
	if r.code != 2 {
		t.Fatalf("restore with no mapping should stop: %+v", r)
	}
	if !strings.Contains(r.stderr, "move that private folder") || !strings.Contains(r.stderr, oldPriv) {
		t.Errorf("no move hint naming the old private folder:\n%s", r.stderr)
	}
	ents, _ := os.ReadDir(filepath.Join(home, "projects"))
	if len(ents) != 1 {
		t.Errorf("private folders: %v", ents)
	}
	newPriv, err := filepath.Glob(filepath.Join(home, "projects", "shop-*"))
	if err != nil || len(newPriv) != 1 || newPriv[0] != oldPriv {
		t.Fatalf("the old private folder was moved or a new one made: %v", newPriv)
	}
	if strings.Contains(r.stderr, "Zelda") {
		t.Error("printed a value")
	}

	// The person moves it as told; then restore works and the hint is gone.
	var to string
	for _, line := range strings.Split(r.stderr, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "to:") {
			to = strings.TrimSpace(strings.TrimPrefix(s, "to:"))
		}
	}
	if to == "" || to == oldPriv {
		t.Fatalf("no new path in the hint:\n%s", r.stderr)
	}
	if err := os.Rename(oldPriv, to); err != nil {
		t.Fatal(err)
	}
	r = moved.run("", false, "restore", "out/reply.md")
	if r.code != 0 || strings.Contains(r.stderr, "move that private folder") || moved.read("restored/reply.md") != "Hi Zelda Quorn\n" {
		t.Errorf("after the move: %+v", r)
	}
}

// A different project that just shares the name gets the hint only until it
// has its own private folder.
func TestSameNameHintStops(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	for _, d := range []string{"a/shop", "b/shop"} {
		p := &tp{t: t, dir: filepath.Join(base, d), home: home}
		os.MkdirAll(filepath.Join(p.dir, "raw"), 0o755)
		p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
		p.write("standin.config.json", plainCfg)
		r := p.run("", false, "scrub")
		if d == "b/shop" && (r.code != 1 || !strings.Contains(r.stderr, "same name")) {
			t.Errorf("second shop: want the hint: %+v", r)
		}
		if r := p.run("", false, "scrub"); r.code != 0 || strings.Contains(r.stderr, "same name") {
			t.Errorf("%s re-run: %+v", d, r)
		}
	}
}

func runShowCmd(p *tp, tty bool, args ...string) res {
	var o, e bytes.Buffer
	code := Run(append([]string{"show", "--project", p.dir}, args...), Env{
		Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e,
		StdoutIsTerminal: func() bool { return tty },
		Getenv:           func(k string) string { return map[string]string{"STANDIN_HOME": p.home}[k] },
	})
	return res{code, o.String(), e.String()}
}

// 35: show prints a real value on a screen only.
func TestShow(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\nzelda  quorn\nPip Varga\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"}}}]}`)
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	r := runShowCmd(p, true, "PERSON_1", "[PERSON_2]")
	if r.code != 0 || !strings.Contains(r.stdout, "[PERSON_1]  Zelda Quorn") || !strings.Contains(r.stdout, "[PERSON_2]  Pip Varga") {
		t.Fatalf("on a screen: %+v", r)
	}
	if !strings.Contains(r.stdout, "1 other way") {
		t.Errorf("should say PERSON_1 has another spelling: %q", r.stdout)
	}
	// Off a screen: refuse, print nothing.
	r = runShowCmd(p, false, "PERSON_1")
	if r.code != 2 || r.stdout != "" || strings.Contains(r.stderr, "Zelda") || !strings.Contains(r.stderr, "screen") {
		t.Errorf("to a pipe: %+v", r)
	}
	// Unknown: say so, exit 1.
	r = runShowCmd(p, true, "PERSON_99")
	if r.code != 1 || !strings.Contains(r.stderr, "[PERSON_99] is not in the mapping") || r.stdout != "" {
		t.Errorf("unknown: %+v", r)
	}
	// Not a stand-in, or none named: usage, exit 2.
	for _, args := range [][]string{{"Zelda"}, {}, {"PERSON_0"}} {
		if r := runShowCmd(p, true, args...); r.code != 2 || r.stdout != "" {
			t.Errorf("%v: %+v", args, r)
		}
	}
}

// 37: messages name project-relative paths. In v0.1.0 "answer.md" read as
// two lower-case words (a name), so restore said it wrote "a file whose
// name looks like private data". A long hex folder or a home folder above
// the project must not trip the check either.
func TestRestoreMessageNamesPlainPath(t *testing.T) {
	for _, at := range []string{"x/ba8a8b45-ed08-4664-8819-dbc755211cb3/shop", "Users/sam/Documents/shop"} {
		t.Run(at, func(t *testing.T) {
			p := newTPAt(t, at)
			p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
			p.write("standin.config.json", plainCfg)
			if r := p.run("", false, "scrub"); r.code != 0 {
				t.Fatalf("%+v", r)
			}
			p.write("answer.md", "Hi [VALUE_1]\n")
			p.write("scrubbed/notes/answer.md", "Bye [VALUE_1]\n")
			for arg, want := range map[string]string{
				"answer.md":                                      "Wrote restored/answer.md (1 stand-ins put back)",
				filepath.Join(p.dir, "answer.md"):                "Wrote restored/answer.md (1 stand-ins put back)",
				"scrubbed/notes/answer.md":                       "Wrote restored/notes/answer.md (1 stand-ins put back)",
				filepath.Join(p.dir, "scrubbed/notes/answer.md"): "Wrote restored/notes/answer.md",
			} {
				r := p.run("", false, "restore", arg)
				if r.code != 0 || !strings.Contains(r.stderr, want) || strings.Contains(r.stderr, "hidden") {
					t.Errorf("restore %s: want %q\n%+v", arg, want, r)
				}
			}
			// Run from inside the project, as a person would.
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Chdir(wd); err != nil {
					t.Error(err)
				}
			}()
			if err := os.Chdir(p.dir); err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			Run([]string{"restore", "answer.md"}, Env{Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e,
				Getenv: func(k string) string { return map[string]string{"STANDIN_HOME": p.home}[k] }})
			if !strings.Contains(e.String(), "Wrote restored/answer.md") {
				t.Errorf("from the project folder: %s", e.String())
			}
		})
	}
}

// 37: a data-like name reads "(name hidden)", and the rest of the path stays.
func TestHiddenNameReadsPlainly(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", plainCfg)
	p.run("", false, "scrub")
	p.write("scrubbed/Pip Varga.md", "Hi [VALUE_1]\n")
	r := p.run("", false, "restore", "scrubbed/Pip Varga.md")
	if r.code != 0 || !strings.Contains(r.stderr, "Wrote restored/(name hidden) (1 stand-ins put back)") || strings.Contains(r.stderr, "Pip") {
		t.Errorf("%+v", r)
	}
	if got := show("raw/Pip Varga/inv.csv"); got != "raw/(name hidden)/inv.csv" {
		t.Errorf("show: %q", got)
	}
	for _, plain := range []string{"answer.md", "notes/reply.txt", "raw/bank-export.csv", "my notes.md"} {
		if dataLikeName(plain) {
			t.Errorf("%s taken for data", plain)
		}
	}
	for _, data := range []string{"zelda.quorn", "statement.Zelda Quorn", "lee.pip.csv", "Pip Varga.md", "pip@example.com.csv"} {
		if !dataLikeName(data) {
			t.Errorf("%s not taken for data", data)
		}
	}
}
