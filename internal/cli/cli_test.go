package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// All data here is invented.

type tp struct {
	t    *testing.T
	dir  string
	home string
}

func newTP(t *testing.T) *tp {
	base := t.TempDir()
	p := &tp{t: t, dir: filepath.Join(base, "shop"), home: filepath.Join(base, "home")}
	os.MkdirAll(filepath.Join(p.dir, "raw"), 0o755)
	return p
}

func (p *tp) write(rel, s string) {
	full := filepath.Join(p.dir, rel)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p *tp) read(rel string) string {
	b, err := os.ReadFile(filepath.Join(p.dir, rel))
	if err != nil {
		p.t.Fatal(err)
	}
	return string(b)
}

func (p *tp) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(p.dir, rel))
	return err == nil
}

type res struct {
	code           int
	stdout, stderr string
}

func (p *tp) run(stdin string, tty bool, args ...string) res {
	var o, e bytes.Buffer
	code := Run(append(args, "--project", p.dir), Env{
		Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
		StdinIsTerminal: func() bool { return tty },
		// tty means a person at a screen: stdin and stdout are terminals.
		StdoutIsTerminal: func() bool { return tty },
		Getenv: func(k string) string {
			if k == "STANDIN_HOME" {
				return p.home
			}
			return ""
		},
		Version: "test",
	})
	return res{code, o.String(), e.String()}
}

func (p *tp) privDir() string {
	m, _ := filepath.Glob(filepath.Join(p.home, "projects", "shop-*"))
	if len(m) != 1 {
		p.t.Fatalf("private dirs: %v", m)
	}
	return m[0]
}

const invoices = "Invoice,Customer,Amount,Notes\n" +
	"INV-1,Ann Fakelee,10.00,Ann Fakelee will pay late\n" +
	"INV-2,Bo Fakeson,20.00,\n" +
	"INV-3,ann fakelee ,30.00,Call 416-555-0199\n"

// "bank" sorts before "invoices": the two-pass scrub must still know the names.
const bank = "Date,Description\n2026-01-05,E-TRANSFER FROM BO FAKESON\n2026-01-06,PAYMENT ANN FAKELEE\n"

const cfg = `{"version": 1, "files": [
 {"match": "raw/invoices.csv", "columns": {
   "Invoice": {"action": "keep", "reason": "ours"},
   "Customer": {"action": "tokenize", "category": "PERSON", "reason": "names"},
   "Amount": {"action": "keep", "reason": "match"},
   "Notes": {"action": "scan", "reason": "free text"}}},
 {"match": "raw/bank.csv", "columns": {
   "Date": {"action": "keep", "reason": "match"},
   "Description": {"action": "scan", "reason": "names inside"}}}]}`

var realValues = []string{"Ann Fakelee", "ann fakelee", "Bo Fakeson", "BO FAKESON", "ANN FAKELEE", "416-555-0199"}

func noValues(t *testing.T, what, s string) {
	t.Helper()
	low := strings.ToLower(s)
	for _, v := range realValues {
		if strings.Contains(low, strings.ToLower(v)) {
			t.Errorf("%s holds the real value %q:\n%s", what, v, s)
		}
	}
}

func (p *tp) approve() {
	r := p.run(strings.Repeat("yes\n", 10), true, "approve")
	if r.code != 0 {
		p.t.Fatalf("approve: %+v", r)
	}
}

func TestTwoFilesSameStandinAndTwoPassScan(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("raw/bank.csv", bank)
	p.write("standin.config.json", cfg)
	p.approve()
	r := p.run("", false, "scrub")
	if r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	inv, bk := p.read("scrubbed/invoices.csv"), p.read("scrubbed/bank.csv")
	noValues(t, "scrubbed invoices", inv)
	noValues(t, "scrubbed bank", bk)
	noValues(t, "scrub output", r.stdout+r.stderr)
	want := "Invoice,Customer,Amount,Notes\nINV-1,[PERSON_1],10.00,[PERSON_1] will pay late\nINV-2,[PERSON_2],20.00,\nINV-3,[PERSON_1],30.00,Call [PHONE_1]\n"
	if inv != want {
		t.Errorf("invoices:\n%s\nwant\n%s", inv, want)
	}
	if bk != "Date,Description\n2026-01-05,E-TRANSFER FROM [PERSON_2]\n2026-01-06,PAYMENT [PERSON_1]\n" {
		t.Errorf("bank:\n%s", bk)
	}
	// Restore puts back the first spelling, and says there were two.
	r = p.run("", false, "restore", "scrubbed/bank.csv")
	if r.code != 1 || !strings.Contains(r.stderr, "more than one spelling") {
		t.Errorf("restore: %+v", r)
	}
	if got := p.read("restored/bank.csv"); !strings.Contains(got, "PAYMENT Ann Fakelee") {
		t.Errorf("restored: %s", got)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{p.privDir(): 0o700, filepath.Join(p.privDir(), "mapping.json"): 0o600,
			filepath.Join(p.privDir(), "approval.json"): 0o600} {
			if st, err := os.Stat(path); err != nil || st.Mode().Perm() != want {
				t.Errorf("%s: mode wrong (%v)", path, err)
			}
		}
	}
}

func TestUnapprovedKeepIsTokenized(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("standin.config.json", cfg)
	r := p.run("", false, "scrub", "raw/invoices.csv")
	if r.code != 1 || !strings.Contains(r.stderr, "not been approved") {
		t.Fatalf("%+v", r)
	}
	if out := p.read("scrubbed/invoices.csv"); strings.Contains(out, "INV-1") || strings.Contains(out, "10.00") {
		t.Errorf("unapproved keep was kept:\n%s", out)
	}
}

func TestConfigEditVoidsApproval(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("standin.config.json", cfg)
	p.approve()
	if r := p.run("", false, "scrub", "raw/invoices.csv"); r.code != 0 || !strings.Contains(p.read("scrubbed/invoices.csv"), "INV-1") {
		t.Fatalf("approved scrub: %+v", r)
	}
	p.write("standin.config.json", strings.Replace(cfg, `"ours"`, `"our numbers"`, 1))
	r := p.run("", false, "scrub", "raw/invoices.csv")
	if r.code != 1 || !strings.Contains(r.stderr, "changed after it was approved") {
		t.Fatalf("%+v", r)
	}
	if strings.Contains(p.read("scrubbed/invoices.csv"), "INV-1") {
		t.Error("keep used after the config changed")
	}
}

func TestApproveNeedsTerminal(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", cfg)
	r := p.run(strings.Repeat("yes\n", 10), false, "approve")
	if r.code != 2 {
		t.Fatalf("%+v", r)
	}
	if m, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "approval.json")); len(m) > 0 {
		t.Fatal("approval written without a terminal")
	}
	// On a terminal, a "no" approves nothing.
	r = p.run("yes\nno\n", true, "approve")
	if r.code != 2 {
		t.Fatalf("%+v", r)
	}
	if m, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "approval.json")); len(m) > 0 {
		t.Fatal("approval written after a no")
	}
	if !strings.Contains(r.stdout, "Keep Invoice in raw/invoices.csv?") {
		t.Errorf("question: %s", r.stdout)
	}
}

func TestNoKeepOrScanNeedsNoApproval(t *testing.T) {
	p := newTP(t)
	p.write("raw/bank.csv", bank)
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/bank.csv","columns":{"Date":{"action":"tokenize","category":"VALUE"},"Description":{"action":"drop"}}}]}`)
	r := p.run("", false, "scrub")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestCheckNeverPrintsValues(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("standin.config.json", cfg)
	p.run("", false, "scrub", "raw/invoices.csv")
	text := "Please ask Ann Fakelee, or mail bo.fakeson@example.com, or 416-555-0199 or card 4111 1111 1111 1111.\n"
	r := p.run(text, true, "check", "-") // on a screen: full detail
	if r.code != 1 {
		t.Fatalf("%+v", r)
	}
	out := r.stdout + r.stderr
	noValues(t, "check output", out)
	for _, v := range []string{"bo.fakeson@example.com", "4111"} {
		if strings.Contains(out, v) {
			t.Errorf("check printed %q", v)
		}
	}
	for _, want := range []string{"[PERSON_1]", "email", "card number"} {
		if !strings.Contains(out, want) {
			t.Errorf("check output should name %q:\n%s", want, out)
		}
	}
	if r := p.run("", false, "check", "scrubbed/invoices.csv"); r.code != 0 {
		t.Errorf("check on scrubbed: %+v", r)
	}
}

func TestStandinShapedInputRefused(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", "Customer,Notes\nAnn Fakelee,Same as [PERSON_2] ask\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "scrub")
	if r.code != 2 || strings.Contains(r.stderr, "Same as") || !strings.Contains(r.stderr, "row 1") {
		t.Fatalf("%+v", r)
	}
	if p.exists("scrubbed") {
		t.Error("wrote output")
	}
}

func TestFailClosedNoPartialOutput(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.csv", "Name\nAnn Fakelee\n")
	p.write("raw/b.csv", "Name,Note\nBo Fakeson,x\nCy Fake\n") // ragged
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "scrub")
	if r.code != 2 || !strings.Contains(r.stderr, "Line 3") {
		t.Fatalf("%+v", r)
	}
	if p.exists("scrubbed") {
		t.Error("partial output written")
	}
	if m, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "mapping.json")); len(m) > 0 {
		t.Error("mapping written")
	}
	if r := p.run("", false, "shape"); r.code != 2 || p.exists("shape") {
		t.Errorf("shape wrote output for a bad batch: %+v", r)
	}
}

func TestDataLikeFileName(t *testing.T) {
	p := newTP(t)
	p.write("raw/Ann Fakelee.csv", "Amount\n1.00\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "shape")
	if r.code != 1 || !p.exists("shape/file_1.shape.md") {
		t.Fatalf("%+v", r)
	}
	noValues(t, "shape output", r.stdout+r.stderr+p.read("shape/file_1.shape.md"))
	r = p.run("", false, "scrub")
	if r.code != 2 || p.exists("scrubbed") {
		t.Fatalf("%+v", r)
	}
	noValues(t, "scrub output", r.stdout+r.stderr)
	r = p.run("", false, "scrub", "raw/Ann Fakelee.csv")
	noValues(t, "scrub output", r.stdout+r.stderr)
}

func TestNoConfigRefused(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	if r := p.run("", false, "scrub"); r.code != 2 || p.exists("scrubbed") {
		t.Fatalf("%+v", r)
	}
}

func TestDataLikeHeaderTokenized(t *testing.T) {
	p := newTP(t)
	p.write("raw/wide.csv", "Month,Ann Fakelee,Total\n2026-01,1.00,1.00\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Month":{"action":"keep"},"[COLUMN_2]":{"action":"drop"},"Total":{"action":"keep"}}}]}`)
	p.approve()
	r := p.run("", false, "scrub")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if got := p.read("scrubbed/wide.csv"); got != "Month,[PERSON_1],Total\n2026-01,,1.00\n" {
		t.Errorf("got %q", got)
	}
}

func TestReportIsDataFree(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("raw/broken.csv", "a,b\n1\n")
	p.write("standin.config.json", cfg)
	p.run("", false, "scrub", "raw/invoices.csv")
	p.run("", false, "scrub", "raw/broken.csv")
	r := p.run("", false, "report")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	noValues(t, "report", r.stdout)
	for _, bad := range []string{"invoices", "broken", "Customer", "INV-"} {
		if strings.Contains(r.stdout, bad) {
			t.Errorf("report names %q:\n%s", bad, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "ragged") {
		t.Errorf("report should list the last error code:\n%s", r.stdout)
	}
}

func TestRestoreNeedsMapping(t *testing.T) {
	p := newTP(t)
	if r := p.run("[PERSON_1]", false, "restore", "-"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestHelpAndUnknown(t *testing.T) {
	p := newTP(t)
	if r := p.run("", false, "help"); r.code != 0 || !strings.Contains(r.stdout, "stand-in") {
		t.Fatalf("%+v", r)
	}
	if r := p.run("", false, "frobnicate"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
	if r := p.run("", false, "scrub", "--bogus"); r.code != 2 {
		t.Fatalf("%+v", r)
	}
	if r := p.run("", false, "version"); r.code != 0 || !strings.Contains(r.stdout, "standin test") {
		t.Fatalf("%+v", r)
	}
}

func TestRestoreDashNeedsScreen(t *testing.T) {
	p := newTP(t)
	p.write("raw/invoices.csv", invoices)
	p.write("standin.config.json", cfg)
	p.run("", false, "scrub", "raw/invoices.csv")
	r := p.run("Hi [PERSON_1]\n", false, "restore", "-")
	if r.code != 2 || r.stdout != "" {
		t.Fatalf("restore - to a pipe should refuse: %+v", r)
	}
	var o, e bytes.Buffer
	code := Run([]string{"restore", "-", "--project", p.dir}, Env{
		Stdin: strings.NewReader("Hi [PERSON_1] and [PERSON_2]\n"), Stdout: &o, Stderr: &e,
		StdoutIsTerminal: func() bool { return true },
		Getenv:           func(k string) string { return map[string]string{"STANDIN_HOME": p.home}[k] },
	})
	// [PERSON_1] has two spellings in the fixture, so exit 1.
	if code != 1 || o.String() != "Hi Ann Fakelee and Bo Fakeson\n" {
		t.Fatalf("restore - on a screen: %d %q %q", code, o.String(), e.String())
	}
}
