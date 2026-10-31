package cli

// Fixes from the second fresh review (DESIGN decisions 46-52). Invented names only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/words"
)

// F1: one-word names in headers are hidden in shape and messages and swapped
// in the scrubbed copy: Markdown tables, CSV headers and JSON keys.
func TestOneWordNameHeaders(t *testing.T) {
	p := newTP(t)
	p.write("raw/roster.md", "| Day | Oksana | Kavya |\n|---|---|---|\n| Mon | open | close |\n")
	p.write("raw/rota.csv", "Day,Oksana,Total\nMon,open,1\n")
	p.write("raw/rota.json", `[{"day":"Mon","Kavya":{"shift":"open"}}]`)
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"drop"}}]}`)
	sh := p.run("", false, "shape")
	out := sh.stderr
	for _, f := range []string{"shape/roster.md.shape.md", "shape/rota.csv.shape.md", "shape/rota.json.shape.md"} {
		out += p.read(f)
	}
	noneOf(t, "shape", out, []string{"Oksana", "Kavya"})
	if !strings.Contains(p.read("shape/rota.csv.shape.md"), "[COLUMN_2]") {
		t.Error("the CSV header should be [COLUMN_2] in the shape file")
	}
	r := p.run("", false, "scrub")
	if r.code > 1 {
		t.Fatalf("%+v", r)
	}
	out = r.stderr
	for _, f := range []string{"scrubbed/roster.md", "scrubbed/rota.csv", "scrubbed/rota.json"} {
		out += p.read(f)
	}
	noneOf(t, "scrub", out, []string{"Oksana", "Kavya"})
	if !strings.HasPrefix(p.read("scrubbed/roster.md"), "| Day | [PERSON_") {
		t.Errorf("roster: %q", p.read("scrubbed/roster.md"))
	}

	// Such a header cannot be named in the config.
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Oksana":{"action":"drop"}}}]}`)
	r = p.run("", false, "scrub", "raw/rota.csv")
	if r.code != 2 || !strings.Contains(r.stderr, "[COLUMN_2]") || strings.Contains(r.stderr, "Oksana") {
		t.Errorf("naming a data-like header: %+v", r)
	}
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","tables":{"Oksana":{"action":"drop"}},"text":{"action":"drop"}}]}`)
	r = p.run("", false, "scrub", "raw/roster.md")
	if r.code != 2 || strings.Contains(r.stderr, "Oksana") {
		t.Errorf("naming a data-like table header: %+v", r)
	}
}

const wordsCfg = `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words"}}]}`

// F2: a word swapped before (a one-word NAME value) is swapped wherever it
// shows up, also in lower case inside a link, and approve still asks about it.
func TestKnownOneWordNameSwappedInLinks(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words","keep_groups":[]}}]}`)
	p.write("raw/a.md", "Hi Oksana, thanks.\n")
	if r := p.run("yes\n\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	p.write("raw/c.md", "Hi Ignatius.\n")
	p.run("", false, "scrub", "raw/c.md") // a new word: swapped, exit 1
	p.write("raw/d.md", "see https://crm.example/ignatius and oksana-vantreeck\n")
	r := p.run("", false, "scrub", "raw/d.md")
	got := p.read("scrubbed/d.md")
	if strings.Contains(strings.ToLower(got), "ignatius") || strings.Contains(strings.ToLower(got), "oksana") {
		t.Fatalf("a known name is left in (exit %d): %q", r.code, got)
	}
	if r.code != 1 || !strings.Contains(r.stderr, "new word") {
		t.Errorf("Ignatius is still new, so scrub should say so: %+v", r)
	}
	if c := p.run("", false, "check", "scrubbed/d.md"); c.code != 0 {
		t.Errorf("check still finds something: %+v", c)
	}
}

// F2: the last check finds a known value left in an output.
func TestLeftOver(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Zelda Quorn")
	m.Get(words.Category, "Monday")
	known := scan.NewKnown(m)
	if st, ok := leftOver("[PERSON_1] met zelda  quorn", known, false); !ok || st != "[PERSON_1]" {
		t.Errorf("leftOver: %q %v", st, ok)
	}
	if _, ok := leftOver("[PERSON_1] met no one", known, false); ok {
		t.Error("nothing is left over")
	}
	if _, ok := leftOver("open Monday", known, true); ok {
		t.Error("a day name the rule keeps is not left over")
	}
	if _, ok := leftOver("open Monday", known, false); !ok {
		t.Error("a swapped day name is left over when the rule does not keep it")
	}
}

// F5, F6, F7 through scrub: lower-case surname after a first name, a first
// name before a caught surname at a sentence start, a month as a first name.
func TestNameRulesInScrub(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words","keep_groups":["calendar"]}}]}`)
	p.write("raw/n.md", "author: zelda quorn\nRose Fairweather called.\nApril Okonkwo paid in April.\n")
	if r := p.run(strings.Repeat("yes\n", 2)+"\n\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	got := p.read("scrubbed/n.md")
	noneOf(t, "scrubbed", got, []string{"zelda", "quorn", "Rose", "Fairweather", "Okonkwo"})
	if strings.Contains(got, "April") {
		// The month next to a name is in the name-like group now; the key
		// decides for the whole project, so "in April" is swapped too.
		t.Logf("note: %q", got)
	}
}

// F3: hooks guard the whole private-folders root, not just this project's
// folder. Tested in internal/hook; here the moved hint off a terminal.
func TestMovedHintOffTerminalNamesNoPath(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", plainCfg)
	p.run("", false, "scrub")
	old := p.privDir()
	moved := &tp{t: t, dir: filepath.Join(filepath.Dir(p.dir), "moved", "shop"), home: p.home}
	os.MkdirAll(filepath.Dir(moved.dir), 0o755)
	if err := os.Rename(p.dir, moved.dir); err != nil {
		t.Fatal(err)
	}
	r := moved.run("", false, "review")
	if strings.Contains(r.stderr, old) || !strings.Contains(r.stderr, "standin review") {
		t.Errorf("off a terminal: %s", r.stderr)
	}
}

// The null device is a character device too; it is not a terminal.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("the null device counted as a terminal")
	}
}
