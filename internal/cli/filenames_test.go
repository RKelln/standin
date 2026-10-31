package cli

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// anonShape finds the anonymous shape file (file_<hash>.shape.md) that shape
// writes for a file whose name it will not print (decision 71).
func anonShape(t *testing.T, p *tp) string {
	t.Helper()
	ms, err := filepath.Glob(filepath.Join(p.dir, "shape", "file_*.shape.md"))
	if err != nil || len(ms) != 1 {
		t.Fatalf("want one anonymous shape file, got %v (%v)", ms, err)
	}
	return strings.TrimPrefix(ms[0], p.dir+string(filepath.Separator))
}

func TestReviewSingleNameFilenameHidden(t *testing.T) {
	p := newTP(t)
	p.write("raw/Oksana.csv", "Customer\nOksana\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "shape")
	if r.code != 1 {
		t.Fatalf("shape should hide the filename: %+v", r)
	}
	anon := anonShape(t, p)
	if strings.Contains(r.stdout+r.stderr+p.read(anon), "Oksana") {
		t.Fatal("name exposed by shape")
	}
	r = p.run("", false, "scrub")
	if r.code != 2 || p.exists("scrubbed/Oksana.csv") || strings.Contains(r.stdout+r.stderr, "Oksana") {
		t.Fatalf("scrub should refuse without showing the name: %+v", r)
	}
}

func TestReviewKnownValueInFilenameRefused(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.csv", "Customer\nGrace\n")
	p.write("raw/grace.csv", "Amount\n12.50\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	shape := p.run("", false, "shape")
	if shape.code != 1 || strings.Contains(shape.stdout+shape.stderr, "grace.csv") {
		t.Fatalf("shape should hide the known name: %+v", shape)
	}
	r := p.run("", false, "scrub")
	if r.code != 2 || p.exists("scrubbed") || strings.Contains(r.stdout+r.stderr, "grace.csv") {
		t.Fatalf("known name should refuse even when it is an English word: %+v", r)
	}
}

func TestReviewKnownFilenameNotPrintedOnParseFailure(t *testing.T) {
	for _, command := range []string{"shape", "scrub"} {
		t.Run(command, func(t *testing.T) {
			p := newTP(t)
			p.write("raw/a.csv", "Customer\nGrace\n")
			p.write("raw/grace.csv", "Amount,Note\n10\n")
			p.write("standin.config.json", `{"version":1,"files":[]}`)
			r := p.run("", false, command)
			if r.code != 2 || strings.Contains(strings.ToLower(r.stdout+r.stderr), "grace") {
				t.Fatalf("parse refusal printed a private filename: %+v", r)
			}
		})
	}
}

func TestReviewKnownFilenameNotPrintedOnStandinRefusal(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.csv", "Customer\nGrace\n")
	p.write("raw/grace.csv", "Notes\n[PERSON_1]\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "scrub")
	if r.code != 2 || strings.Contains(strings.ToLower(r.stdout+r.stderr), "grace") {
		t.Fatalf("early refusal printed private filename: %+v", r)
	}
}

// A hyphen or underscore separates words in a file name as much as a space
// does in a header (decision 69). Before this, "sam-notes.md" passed the name
// check, and the name reached the shape file and the first scrub.
func TestReviewHyphenatedNameInFilename(t *testing.T) {
	p := newTP(t)
	p.write("raw/sam-notes.md", "The owner says hello.\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "shape")
	if r.code != 1 {
		t.Fatalf("shape should hide a hyphenated name: %+v", r)
	}
	if regexp.MustCompile(`(?i)\bsam\b`).MatchString(r.stdout + r.stderr + p.read(anonShape(t, p))) {
		t.Fatal("name exposed by shape")
	}
	r = p.run("", false, "scrub")
	if r.code != 2 || regexp.MustCompile(`(?i)\bsam\b`).MatchString(r.stdout+r.stderr) {
		t.Fatalf("scrub should refuse a hyphenated name: %+v", r)
	}
}

// The same rule must not fire on ordinary file names: a bare number is not a
// name, and a hyphen does not make two English words into one.
func TestFileNamePartsThatAreNotData(t *testing.T) {
	for _, name := range []string{
		"invoices-q3.csv", "bank-export.csv", "price-sheet.md", "report-2026.csv",
		"wide-names-as-headers.csv", "customer-emails.md", "R-01.txt", "top-100.csv",
	} {
		if dataLikeName(name) {
			t.Errorf("%q: ordinary file name read as data", name)
		}
	}
	for _, name := range []string{"Oksana.csv", "sam-notes.md", "receipts/leila-102.csv", "sam profile draft.md"} {
		if !dataLikeName(name) {
			t.Errorf("%q: name in file name not noticed", name)
		}
	}
}

// A file named on the command line is named again in the refusal: the person
// typed it, so nothing is disclosed, and a folder of files stops being a
// guessing game (decision 70). A batch run still hides it, but now says what
// kind of file it was.
func TestRefusalNamesTheFile(t *testing.T) {
	p := newTP(t)
	p.write("raw/ledger.xlsx", "PK\x03\x04 not really")
	p.write("raw/notes.md", "hello\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "shape", "raw/ledger.xlsx")
	if r.code != 2 || !strings.Contains(r.stderr, "raw/ledger.xlsx") {
		t.Fatalf("a named file should be named back: %+v", r)
	}
	r = p.run("", false, "shape")
	if r.code != 2 || !strings.Contains(r.stderr, "(name hidden, .xlsx)") {
		t.Fatalf("a batch run should name the extension: %+v", r)
	}
	if strings.Contains(r.stderr, "ledger") {
		t.Fatalf("a batch run must not print the name: %+v", r)
	}
}

// The extension is echoed only when it is a plain one: a file called
// ".Zelda Quorn" must not leak its name through the refusal.
func TestSafeExt(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"ledger.xlsx", ".xlsx"},
		{"a.csv", ".csv"},
		{".Zelda Quorn", ""},
		{"notes", ""},
		{"x.y", ""},
		{"odd.XLSX", ""},
	} {
		if got := safeExt(c.in); got != c.want {
			t.Errorf("safeExt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Running shape twice writes each anonymous shape file once, under the same
// name: a hash of the path, not the next free number (decision 71).
func TestAnonymousShapeNameIsStable(t *testing.T) {
	p := newTP(t)
	p.write("raw/sam-notes.md", "The owner says hello.\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.run("", false, "shape")
	first := anonShape(t, p)
	p.run("", false, "shape")
	if second := anonShape(t, p); second != first {
		t.Fatalf("second run renamed the shape file: %q then %q", first, second)
	}
	ms, _ := filepath.Glob(filepath.Join(p.dir, "shape", "file_*.shape.md"))
	if len(ms) != 1 {
		t.Fatalf("want one anonymous shape file, got %v", ms)
	}
}

func TestCountLabel(t *testing.T) {
	for _, c := range []struct {
		fields, cols int
		want         string
	}{
		{1, 0, "1 field"},
		{2, 0, "2 fields"},
		{0, 3, "3 table columns"},
		{0, 1, "1 table column"},
		{2, 3, "2 fields and 3 table columns"},
		{1, 1, "1 field and 1 table column"},
		{0, 0, "no fields or table columns"},
	} {
		if got := countLabel(c.fields, c.cols); got != c.want {
			t.Errorf("countLabel(%d, %d) = %q, want %q", c.fields, c.cols, got, c.want)
		}
	}
}
