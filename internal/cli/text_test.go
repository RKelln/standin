package cli

// Free text (.md, .txt): DESIGN decision 38 and docs/research/free-text.md.
// Invented names only.

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const textCustomers = "Name\nTavish Okorafor\n"

const textNotes = "# Shop notes\n" +
	"\n" +
	"From: Daisy Quellbrook <daisy@example.com>\n" +
	"Date: 2026-03-04\n" +
	"\n" +
	"Hi Oksana,\n" +
	"The new Shimano pads came in. Tavish Okorafor paid on Monday.\n" +
	"Daisy said the wheel wobbles.\n" +
	"thx oksana\n" +
	"\n" +
	"Thanks,\n" +
	"Grace\n" +
	"\n" +
	"| Service | Price |\n" +
	"|---|---|\n" +
	"| Tune-up | $89 |\n"

const textCfg = `{"version":1,"files":[
 {"match":"raw/customers.csv","columns":{"Name":{"action":"tokenize","category":"PERSON"}}},
 {"match":"raw/*.md","fields":{"Date":{"action":"keep"}},"tables":{"Service":{"action":"keep"},"Price":{"action":"keep"}},
  "text":{"action":"words","keep_groups":["calendar"],"reason":"shop notes"}}]}`

// Five yes questions (keep Date, Service, Price; the word check; calendar),
// then the unusual-words page, the name-like page, and the final yes.
func textAnswers(unusual, nameLike string) string {
	return strings.Repeat("yes\n", 5) + unusual + "\n" + nameLike + "\n" + "yes\n"
}

func newTextTP(t *testing.T) *tp {
	p := newTP(t)
	p.write("raw/customers.csv", textCustomers)
	p.write("raw/notes.md", textNotes)
	p.write("standin.config.json", textCfg)
	return p
}

// Words in the free text of notes.md that the AI must never see in shape or
// review output (names and other candidates).
var textSecrets = []string{"Daisy", "Quellbrook", "Oksana", "Shimano", "Tavish", "Okorafor", "Grace", "wobbles"}

func noneOf(t *testing.T, what, s string, words []string) {
	t.Helper()
	for _, w := range words {
		if strings.Contains(strings.ToLower(s), strings.ToLower(w)) {
			t.Errorf("%s holds a word from the text (%q)", what, w)
		}
	}
}

func TestTextFlow(t *testing.T) {
	p := newTextTP(t)
	r := p.run("", false, "shape")
	if r.code != 0 {
		t.Fatalf("shape: %+v", r)
	}
	sh := p.read("shape/notes.md.shape.md")
	noneOf(t, "the shape file", sh+r.stdout+r.stderr, textSecrets)
	for _, want := range []string{"Field `From`", "Field `Date`", "Table 1: `Service`, `Price`", "Unusual words", "Name-like words", "under 5"} {
		if !strings.Contains(sh, want) {
			t.Errorf("shape file lacks %q", want)
		}
	}

	r = p.run("", false, "review")
	noneOf(t, "review", r.stdout+r.stderr, textSecrets)
	if !strings.Contains(r.stdout, "Free text, word check: unusual words under 5, name-like words under 5") || r.code != 1 {
		t.Errorf("review should give counts in bands only: %+v", r)
	}

	if r := p.run("", false, "scrub"); r.code != 2 || !strings.Contains(r.stderr, "standin approve") {
		t.Fatalf("unapproved scrub must refuse: %+v", r)
	}

	r = p.run(textAnswers("1", ""), true, "approve")
	if r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	for _, want := range []string{"[Shimano]", "Oksana", "Daisy", "Grace", "Unusual words", "These look like names"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("approve should show %q to the person:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Monday ") && strings.Contains(r.stdout, "x1    \"...") {
		t.Error("calendar words are a group, not asked one by one")
	}
	wj := filepath.Join(p.privDir(), "words.json")
	st, err := os.Stat(wj)
	if err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("words.json must exist with mode 0600: %v %v", err, st)
	}
	wb, _ := os.ReadFile(wj)
	if !strings.Contains(string(wb), `"keep": [
    "shimano"
  ]`) || !strings.Contains(string(wb), `"oksana"`) {
		t.Errorf("words.json:\n%s", wb)
	}
	ab, _ := os.ReadFile(filepath.Join(p.privDir(), "approval.json"))
	if !regexp.MustCompile(`"words_sha256": "[0-9a-f]{64}"`).Match(ab) {
		t.Errorf("approval.json has no words_sha256:\n%s", ab)
	}

	r = p.run("", false, "scrub")
	if r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	noneOf(t, "scrub messages", r.stdout+r.stderr, textSecrets)
	got := p.read("scrubbed/notes.md")
	want := "# Shop notes\n" +
		"\n" +
		"From: [VALUE_1]\n" +
		"Date: 2026-03-04\n" +
		"\n" +
		"Hi [NAME_1],\n" +
		"The new Shimano pads came in. [PERSON_1] paid on Monday.\n" +
		"[NAME_2] said the wheel wobbles.\n" +
		"thx [NAME_1]\n" +
		"\n" +
		"Thanks,\n" +
		"[NAME_3]\n" +
		"\n" +
		"| Service | Price |\n" +
		"|---|---|\n" +
		"| Tune-up | $89 |\n"
	if got != want {
		t.Errorf("scrubbed notes:\n%s\nwant:\n%s", got, want)
	}

	// Restore puts the text back (the lower-case "oksana" comes back as the
	// first spelling, and restore says so).
	r = p.run("", false, "restore", "scrubbed/notes.md")
	back := p.read("restored/notes.md")
	if back != strings.Replace(textNotes, "thx oksana", "thx Oksana", 1) || r.code != 1 || !strings.Contains(r.stderr, "more than one spelling") {
		t.Errorf("restore: %+v\n%s", r, back)
	}
}

func TestChangedWordsVoidApproval(t *testing.T) {
	p := newTextTP(t)
	if r := p.run(textAnswers("", ""), true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	m := regexp.MustCompile(`The new (\[NAME_\d+\]) pads`).FindStringSubmatch(p.read("scrubbed/notes.md"))
	if m == nil {
		t.Fatal("Shimano should be swapped when not kept")
	}
	shimano := m[1]
	// The person moves shimano to keep by hand: the approval no longer counts.
	wj := filepath.Join(p.privDir(), "words.json")
	b, _ := os.ReadFile(wj)
	b = bytes.Replace(b, []byte(`"keep": []`), []byte(`"keep": ["shimano"]`), 1)
	if err := os.WriteFile(wj, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r := p.run("", false, "scrub")
	if r.code != 2 || !strings.Contains(r.stderr, "words.json") || !strings.Contains(r.stderr, "standin approve") {
		t.Fatalf("a changed words.json must void the approval: %+v", r)
	}
	// Approve again: no new words to ask about; then Shimano stays as it is,
	// though it was swapped before (its old stand-in still restores).
	r = p.run(strings.Repeat("yes\n", 6), true, "approve")
	if r.code != 0 || !strings.Contains(r.stdout, "No new words") {
		t.Fatalf("approve again: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	if !strings.Contains(p.read("scrubbed/notes.md"), "The new Shimano pads") {
		t.Errorf("a kept word must stay, even one swapped before:\n%s", p.read("scrubbed/notes.md"))
	}
	p.write("out/old.md", "Order more "+shimano+" pads.\n")
	p.run("", false, "restore", "out/old.md")
	if p.read("restored/old.md") != "Order more Shimano pads.\n" {
		t.Errorf("the old stand-in must still restore: %q", p.read("restored/old.md"))
	}
}

func TestNewWordsAreSwappedAndAskedNext(t *testing.T) {
	p := newTextTP(t)
	if r := p.run(textAnswers("1", ""), true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	p.write("raw/more.md", "Hi Ravindra,\nThe Shimano part is in.\n")
	r := p.run("", false, "scrub")
	if r.code != 1 || !strings.Contains(r.stderr, "1 new word(s)") || strings.Contains(r.stderr, "Ravindra") {
		t.Fatalf("new words: exit 1 and a count only: %+v", r)
	}
	if got := p.read("scrubbed/more.md"); !regexp.MustCompile(`^Hi \[NAME_\d+\],\nThe Shimano part is in.\n$`).MatchString(got) {
		t.Errorf("more.md: %q", got)
	}
	// review gives only a band; approve asks only about the new word.
	if r := p.run("", false, "review"); !strings.Contains(r.stdout, "new since the last approve: under 5") {
		t.Errorf("review: %s", r.stdout)
	}
	r = p.run(strings.Repeat("yes\n", 5)+"\n"+"yes\n", true, "approve")
	if r.code != 0 || !strings.Contains(r.stdout, "New since the last approve: 1") || !strings.Contains(r.stdout, "Ravindra") || strings.Contains(r.stdout, "[Oksana]") {
		t.Fatalf("approve should ask about the new word only: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Errorf("scrub after approve: %+v", r)
	}
}

func TestApproveWordsNeedsAScreen(t *testing.T) {
	p := newTextTP(t)
	var o, e bytes.Buffer
	code := Run([]string{"approve", "--project", p.dir}, Env{
		Stdin: strings.NewReader(textAnswers("", "")), Stdout: &o, Stderr: &e,
		StdinIsTerminal:  func() bool { return true },
		StdoutIsTerminal: func() bool { return false },
		Getenv: func(k string) string {
			if k == "STANDIN_HOME" {
				return p.home
			}
			return ""
		},
	})
	if code != 2 || !strings.Contains(e.String(), "screen") {
		t.Errorf("approve must refuse to print words to a pipe: %d %s", code, e.String())
	}
	noneOf(t, "refused approve", o.String()+e.String(), textSecrets)
}

func TestBadPicksRefuse(t *testing.T) {
	p := newTextTP(t)
	r := p.run(strings.Repeat("yes\n", 5)+"9\nx\n0-1\n", true, "approve")
	if r.code != 2 || p.exists("words.json") {
		t.Errorf("three bad answers must refuse: %+v", r)
	}
}

func TestTextDefaults(t *testing.T) {
	p := newTP(t)
	p.write("raw/notes.md", textNotes)
	// No text rule: the text is dropped; the From field is tokenized; no approval needed.
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","fields":{"From":{"action":"tokenize","category":"PERSON"}}}]}`)
	r := p.run("", false, "scrub")
	if r.code != 1 || !strings.Contains(r.stderr, `no "text" rule`) {
		t.Fatalf("%+v", r)
	}
	got := p.read("scrubbed/notes.md")
	noneOf(t, "dropped text", got, textSecrets)
	if !strings.Contains(got, "From: [PERSON_1]\nDate: [VALUE_") || !strings.Contains(got, "| [VALUE_") {
		t.Errorf("fields and table cells not named in the config are tokenized:\n%s", got)
	}
}

func TestCalendarSwappedUnlessKept(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.md", "Open on Monday and Tuesday. Kofi works Monday.\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words","keep_groups":[]}}]}`)
	if r := p.run("yes\n\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	if got := p.read("scrubbed/a.md"); got != "Open on [NAME_1] and [NAME_2]. [NAME_3] works [NAME_1].\n" {
		t.Errorf("got %q", got)
	}
}

func TestTextRefusedFiles(t *testing.T) {
	for name, body := range map[string]string{
		"raw/table.txt": "Date,Item,Amount\n2026-01-02,Tubes,12.00\n2026-01-03,Pads,28.00\n",
		"raw/zh.md":     "# Notes\n" + strings.Repeat("諕齋蟪狳躶傍孰沨麮榽", 4) + "\n",
		"raw/st.md":     "Ask [NAME_1] about it.\n",
	} {
		p := newTP(t)
		p.write(name, body)
		p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*","text":{"action":"drop"}}]}`)
		cmds := []string{"shape", "scrub"}
		if name == "raw/st.md" {
			cmds = cmds[1:] // like CSV: shape does not look for stand-ins; scrub refuses
		}
		for _, cmd := range cmds {
			if r := p.run("", false, cmd); r.code != 2 {
				t.Errorf("%s %s: want a refusal: %+v", cmd, name, r)
			}
		}
		if p.exists("scrubbed") {
			t.Errorf("%s: wrote output", name)
		}
	}
}

// Day and month names are kept unless the rule says "keep_groups": [].
func TestCalendarKeptByDefault(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.md", "Open on Monday and Tuesday.\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words"}}]}`)
	if r := p.run("yes\nyes\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	if got := p.read("scrubbed/a.md"); got != "Open on Monday and Tuesday.\n" {
		t.Errorf("got %q", got)
	}
}

func TestKeepGroupsAtTopLevelSaysWhere(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.md", "Hello.\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","keep_groups":["calendar"],"text":{"action":"words"}}]}`)
	r := p.run("", false, "review")
	if r.code != 2 || !strings.Contains(r.stdout+r.stderr, `inside "text"`) {
		t.Errorf("want a hint: %+v", r)
	}
}
