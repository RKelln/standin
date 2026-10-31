package e2e

// Free text (.md, .txt) end to end, on the invented fixtures inbox.md,
// notes.md and receipt.txt (made by tools/make_fixtures.py).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var wordRe = regexp.MustCompile(`\pL[\pL\pM]*`)

func wordsOf(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(s, -1) {
		out[strings.ToLower(w)] = true
	}
	return out
}

// textConfig is the fixture config plus rules for the three text files.
func textConfig(t *testing.T) []byte {
	var cfg map[string]any
	if err := json.Unmarshal(fixture(t, "standin.config.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	keep := map[string]any{"action": "keep"}
	person := map[string]any{"action": "tokenize", "category": "PERSON"}
	words := func(groups ...string) map[string]any {
		r := map[string]any{"action": "words"}
		if len(groups) > 0 {
			r["keep_groups"] = groups
		}
		return r
	}
	cfg["files"] = append(cfg["files"].([]any),
		map[string]any{"match": "raw/inbox.md", "fields": map[string]any{"From": person, "Date": keep, "Subject": map[string]any{"action": "scan"}}, "text": words("calendar")},
		map[string]any{"match": "raw/notes.md", "fields": map[string]any{"Attendees": person, "Regrets": person, "Next meeting": keep},
			"tables": map[string]any{"Service": keep, "Price": keep}, "text": words()},
		map[string]any{"match": "raw/*.txt", "fields": map[string]any{"Date": keep, "Paid": keep}, "text": words()},
	)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

func textProject(t *testing.T) *project {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	for _, f := range []string{"inbox.md", "notes.md", "receipt.txt"} {
		p.addRaw(f, f)
	}
	p.write("standin.config.json", textConfig(t))
	return p
}

var freeTextLine = regexp.MustCompile(`^Free text: (\d+) unusual words, (\d+) name-like words`)

// approveKeepNone approves the config with a pseudo-terminal, keeping no
// word. A first run (answers run out on the first word page) tells how many
// yes questions and word pages there are.
func approveKeepNone(t *testing.T, p *project) result {
	t.Helper()
	probe, err := p.runWithTTY(strings.Repeat("yes\n", 60), "approve")
	if err != nil {
		t.Skip("needs a pseudo-terminal: ", err)
	}
	yes := strings.Count(strings.SplitN(probe.stdout, "\nFree text:", 2)[0], "Type yes:")
	var a, b int
	for _, l := range strings.Split(probe.stdout, "\n") {
		if m := freeTextLine.FindStringSubmatch(l); m != nil {
			a, b = atoi(m[1]), atoi(m[2])
		}
	}
	if yes == 0 || a+b == 0 {
		t.Fatalf("could not read the approve screen:\n%s", probe.stdout)
	}
	pages := (a+19)/20 + (b+19)/20
	r, err := p.runWithTTY(strings.Repeat("yes\n", yes)+strings.Repeat("\n", pages)+"yes\n", "approve")
	if err != nil || r.code != 0 {
		t.Fatalf("approve: %v\n%s", err, r)
	}
	return r
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestTextFixturesEndToEnd(t *testing.T) {
	p := textProject(t)
	p.must(0, nil, "shape")
	r := p.run(nil, "review")
	p.wantCode(r, 1) // not approved yet
	review := r.stdout + r.stderr

	// No word from a text file may appear in a shape file or in review, other
	// than the labels and headers the shape file names on purpose, and words
	// Standin's own text uses anyway (taken from a dummy file's shape).
	allowed := map[string]bool{}
	var shapes strings.Builder
	for _, f := range []string{"inbox.md", "notes.md", "receipt.txt"} {
		sh := readFile(t, filepath.Join(p.dir, "shape", f+".shape.md"))
		shapes.WriteString(sh)
		for _, m := range regexp.MustCompile("### (?:Field|Table column) `([^`]+)`").FindAllStringSubmatch(sh, -1) {
			for w := range wordsOf(m[1]) {
				allowed[w] = true
			}
		}
	}
	dummy := newProject(t)
	dummy.write("raw/x.md", []byte("Note: hello\n\n| Alpha | Beta |\n|---|---|\n| 1 | 2 |\n\nHi Zed, write to zed@example.com or 416-555-0100 on Monday. Ok.\n"))
	dummy.write("standin.config.json", []byte(`{"version":1,"files":[{"match":"raw/*.md","fields":{"Note":{"action":"keep"}},"tables":{"Alpha":{"action":"keep"}},"text":{"action":"words","keep_groups":["calendar"]}}]}`))
	dummy.must(0, nil, "shape")
	dr := dummy.run(nil, "review")
	// File names are shown on purpose too (a data-like one is hidden).
	for w := range wordsOf(readFile(t, filepath.Join(dummy.dir, "shape", "x.md.shape.md")) + dr.stdout + dr.stderr + string(textConfig(t)) + "inbox notes receipt") {
		allowed[w] = true
	}
	shown := wordsOf(shapes.String() + review)
	for _, f := range []string{"inbox.md", "notes.md", "receipt.txt"} {
		for w := range wordsOf(string(fixture(t, f))) {
			if len([]rune(w)) >= 3 && !allowed[w] && shown[w] {
				t.Errorf("shape or review shows %q, a word from %s", w, f)
			}
		}
	}
	if !strings.Contains(review, "Free text, word check:") {
		t.Errorf("review should count the words:\n%s", review)
	}

	p.must(2, nil, "scrub") // not approved
	approveKeepNone(t, p)
	r = p.run(nil, "scrub")
	if r.code != 0 {
		t.Fatalf("scrub:\n%s", r)
	}

	// Every invented name is swapped wherever it is capitalised.
	names := readLines(t, "text-names.txt")
	for _, f := range []string{"inbox.md", "notes.md", "receipt.txt"} {
		out := readFile(t, filepath.Join(p.dir, "scrubbed", f))
		for _, n := range names {
			if regexp.MustCompile(`(^|[^\pL])` + regexp.QuoteMeta(n) + `($|[^\pL])`).MatchString(out) {
				t.Errorf("scrubbed/%s still has the name %q", f, n)
			}
		}
		if strings.Contains(out, "@example.com") || strings.Contains(out, "555-01") {
			t.Errorf("scrubbed/%s still has an email or phone", f)
		}
	}
	p.noLeaksInDirs(knownValues(t), "scrubbed", "shape")
	notes := readFile(t, filepath.Join(p.dir, "scrubbed", "notes.md"))
	for _, want := range []string{"| Tune-up | $89 |", "|---|---:|", "## Actions", "Next meeting: Tuesday, April 7.", "Attendees: [PERSON_"} {
		if !strings.Contains(notes, want) {
			t.Errorf("scrubbed/notes.md lacks %q:\n%s", want, notes)
		}
	}
	if inbox := readFile(t, filepath.Join(p.dir, "scrubbed", "inbox.md")); !strings.Contains(inbox, "Date: ") || strings.Contains(inbox, "From: Y") {
		t.Errorf("inbox fields:\n%s", inbox[:400])
	}

	// Restore puts it all back (a word seen as QUILLFEATHER and Quillfeather
	// comes back in its first spelling, so compare ignoring case).
	for _, f := range []string{"inbox.md", "notes.md", "receipt.txt"} {
		p.run(nil, "restore", filepath.Join("scrubbed", f))
		got, err := os.ReadFile(filepath.Join(p.dir, "restored", f))
		if err != nil || !strings.EqualFold(string(got), string(fixture(t, f))) {
			t.Errorf("restore %s does not match the raw file (%v)", f, err)
		}
	}

	// The private folder holds words.json, 0600, and the approval names its hash.
	priv := p.privateDir()
	if st, err := os.Stat(filepath.Join(priv, "words.json")); err != nil || (st.Mode().Perm() != 0o600 && os.PathSeparator == '/') {
		t.Errorf("words.json: %v %v", err, st)
	}
	if !strings.Contains(readFile(t, filepath.Join(priv, "approval.json")), `"words_sha256"`) {
		t.Error("approval.json has no words_sha256")
	}
}

func TestTextFixturesRefused(t *testing.T) {
	for _, f := range []string{"table.txt", "uncased.md"} {
		t.Run(f, func(t *testing.T) {
			p := newProject(t)
			p.addRaw(f, f)
			p.write("standin.config.json", []byte(`{"version":1,"files":[{"match":"raw/*","text":{"action":"drop"}}]}`))
			for _, cmd := range []string{"shape", "scrub"} {
				if r := p.run(nil, cmd); r.code != 2 || strings.TrimSpace(r.stderr) == "" {
					t.Errorf("%s %s: want a refusal that says why\n%s", cmd, f, r)
				}
			}
			p.noFilesIn("shape", "scrubbed")
		})
	}
}
