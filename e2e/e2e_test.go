package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

const emptyConfig = `{"version": 1, "files": []}` + "\n"

// approve runs `standin approve` on a pseudo-terminal, answering yes to every keep.
// Skips the test if no pseudo-terminal can be made here.
func (p *project) approve() {
	p.t.Helper()
	r, err := p.runWithTTY(strings.Repeat("yes\n", 40), "approve")
	if err != nil {
		p.t.Skipf("cannot run approve on a terminal here: %v", err)
	}
	if r.code != 0 {
		p.t.Fatalf("approve on a terminal: want exit 0\n%s", r)
	}
	if _, err := os.Stat(filepath.Join(p.findPrivateDir(), "approval.json")); err != nil {
		p.t.Fatalf("approve: no approval.json: %v", err)
	}
}

// tokenMap pairs each stand-in in scrubbed[col] with the first raw spelling in raw[col].
// It fails if one normalised value got two stand-ins or one stand-in two values.
func tokenMap(t *testing.T, raw, scr []map[string]string, col, cat string, into map[string]string) {
	t.Helper()
	if len(raw) != len(scr) {
		t.Fatalf("row count: raw %d, scrubbed %d", len(raw), len(scr))
	}
	byNorm := map[string]string{}
	for tok, v := range into {
		byNorm[normKey(v)] = tok
	}
	for i := range raw {
		rv, sv := raw[i][col], scr[i][col]
		if strings.TrimSpace(rv) == "" {
			if sv != "" && strings.TrimSpace(sv) != "" {
				t.Errorf("row %d %s: empty value became %q", i+2, col, sv)
			}
			continue
		}
		if !wholeToken.MatchString(sv) || !strings.HasPrefix(sv, "["+cat+"_") {
			t.Errorf("row %d %s: want a [%s_n] stand-in, got %q", i+2, col, cat, sv)
			continue
		}
		if prev, ok := byNorm[normKey(rv)]; ok && prev != sv {
			t.Errorf("row %d %s: same value got two stand-ins %s and %s", i+2, col, prev, sv)
		}
		if first, ok := into[sv]; ok && normKey(first) != normKey(rv) {
			t.Errorf("row %d %s: stand-in %s used for two different values", i+2, col, sv)
		}
		if _, ok := into[sv]; !ok {
			into[sv] = rv
		}
		byNorm[normKey(rv)] = sv
	}
}

// variants returns the normalised values that appear with more than one spelling.
func variants(rows []map[string]string, col string) map[string]bool {
	seen := map[string]map[string]bool{}
	for _, r := range rows {
		k := normKey(r[col])
		if seen[k] == nil {
			seen[k] = map[string]bool{}
		}
		seen[k][r[col]] = true
	}
	out := map[string]bool{}
	for k, s := range seen {
		if len(s) > 1 {
			out[k] = true
		}
	}
	return out
}

// setupPair makes a project with invoices + bank export, config, shape, approve and scrub
// (invoices first, so the bank scan already knows the names).
func setupPair(t *testing.T, bankFixture string) *project {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.addRaw(bankFixture, "bank-export.csv")
	p.useConfig()
	r := p.run(nil, "shape", "raw/invoices.csv", "raw/bank-export.csv")
	p.wantCode(r, 0, 1)
	p.approve()
	p.wantCode(p.run(nil, "scrub", "raw/invoices.csv"), 0, 1)
	r = p.run(nil, "scrub", "raw/bank-export.csv")
	p.wantCode(r, 0, 1)
	return p
}

// Success checks 1, 2 and the cross-file stand-in check.
func TestRoundTripInvoicesAndBank(t *testing.T) {
	p := setupPair(t, "bank-export.csv")
	kv := knownValues(t)

	p.find("shape", "invoices.csv.shape.md")
	p.find("shape", "bank-export.csv.shape.md")

	rawInv := parseCSV(t, string(fixture(t, "invoices.csv")))
	scrInv := parseCSV(t, readFile(t, p.find("scrubbed", "invoices.csv")))
	rawBank := parseCSV(t, string(fixture(t, "bank-export.csv")))
	scrBank := parseCSV(t, readFile(t, p.find("scrubbed", "bank-export.csv")))

	// Kept columns are unchanged.
	for i := range rawInv {
		for _, c := range []string{"Invoice", "Date", "Amount", "Status"} {
			if rawInv[i][c] != scrInv[i][c] {
				t.Errorf("invoices row %d %s: kept value changed to %q", i+2, c, scrInv[i][c])
			}
		}
	}
	for i := range rawBank {
		for _, c := range []string{"Date", "Amount", "Balance"} {
			if rawBank[i][c] != scrBank[i][c] {
				t.Errorf("bank row %d %s: kept value changed to %q", i+2, c, scrBank[i][c])
			}
		}
	}

	// Tokenized columns hold stand-ins, consistently.
	persons, emails, phones, accts := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	tokenMap(t, rawInv, scrInv, "Customer", "PERSON", persons)
	tokenMap(t, rawInv, scrInv, "Email", "EMAIL", emails)
	tokenMap(t, rawInv, scrInv, "Phone", "PHONE", phones)
	tokenMap(t, rawBank, scrBank, "Account", "ACCOUNT", accts)

	// Same name, same stand-in, inside the bank description (scan pass).
	for i := range rawBank {
		for tok, name := range persons {
			if strings.Contains(norm(rawBank[i]["Description"]), norm(name)) &&
				!strings.Contains(scrBank[i]["Description"], tok) {
				t.Errorf("bank row %d: description should use %s (the invoices stand-in for that name), got %q",
					i+2, tok, scrBank[i]["Description"])
			}
		}
	}
	// Notes in invoices: known names replaced by the Customer stand-in.
	for i := range rawInv {
		for tok, name := range persons {
			if strings.Contains(norm(rawInv[i]["Notes"]), norm(name)) && !strings.Contains(scrInv[i]["Notes"], tok) {
				t.Errorf("invoices row %d Notes: want %s, got %q", i+2, tok, scrInv[i]["Notes"])
			}
		}
	}

	// Success check 2: no known value in shape/ or scrubbed/, or in command output.
	p.noLeaksInDirs(kv, "shape", "scrubbed")
	rc := p.run(nil, "check", "scrubbed/invoices.csv", "scrubbed/bank-export.csv")
	if rc.code != 0 {
		t.Errorf("check on scrubbed files: want exit 0 (nothing found)\n%s", rc)
	}
	rc = p.run(nil, "check", "raw/invoices.csv")
	if rc.code != 1 {
		t.Errorf("check on a raw file: want exit 1 (real values found)\n%s", rc)
	}
	// Off a terminal (an agent), check gives one bit: no stand-in names, so guesses cannot be tested.
	if tokenRE.MatchString(rc.stdout + rc.stderr) {
		t.Errorf("check off a terminal must not name stand-ins\n%s", rc)
	}
	rc = p.run([]byte("Please call "+rawInv[0]["Customer"]+" today.\n"), "check", "-")
	if rc.code != 1 {
		t.Errorf("check - with a real name: want exit 1\n%s", rc)
	}
	p.wantCode(p.run(nil, "review"), 0, 1)
	p.must(0, nil, "report")
	p.noLeaksInOutput(kv)

	// Known gaps: names not in the mapping stay. Logged, not failed.
	for _, g := range readLines(t, "known-gaps.txt") {
		if l := leaks(readFile(t, p.find("scrubbed", "invoices.csv"))+readFile(t, p.find("scrubbed", "bank-export.csv")),
			[]string{g}); len(l) > 0 {
			t.Logf("known gap (allowed by contract): %q is still in scrubbed/", g)
		}
	}

	// Success check 1: restore gives back the originals.
	rr := p.run(nil, "restore", "scrubbed/invoices.csv")
	if rr.code != 1 {
		t.Errorf("restore invoices: want exit 1 (the fixture has two spellings of some names)\n%s", rr)
	}
	if !strings.Contains(strings.ToLower(rr.stdout+rr.stderr), "spelling") {
		t.Errorf("restore invoices: want a 'more than one spelling' note\n%s", rr)
	}
	noLeaks(t, "restore report", rr.stdout+rr.stderr, kv)
	resInv := parseCSV(t, readFile(t, p.find("restored", "invoices.csv")))
	varCust := variants(rawInv, "Customer")
	if len(varCust) == 0 {
		t.Fatal("fixture should have variant spellings")
	}
	if len(resInv) != len(rawInv) {
		t.Fatalf("restored invoices: %d rows, want %d", len(resInv), len(rawInv))
	}
	for i := range rawInv {
		hasVariant := varCust[normKey(rawInv[i]["Customer"])]
		for c, want := range rawInv[i] {
			got := resInv[i][c]
			if hasVariant && (c == "Customer" || c == "Notes") {
				if normKey(got) != normKey(want) {
					t.Errorf("restored invoices row %d %s: got %q, want %q (after normalising)", i+2, c, got, want)
				}
				if c == "Customer" {
					// First spelling seen.
					for _, r := range rawInv {
						if normKey(r["Customer"]) == normKey(want) {
							if got != r["Customer"] {
								t.Errorf("restored row %d: got %q, want first spelling %q", i+2, got, r["Customer"])
							}
							break
						}
					}
				}
				continue
			}
			if got != want {
				t.Errorf("restored invoices row %d %s: got %q, want %q", i+2, c, got, want)
			}
		}
	}
	rr = p.run(nil, "restore", "scrubbed/bank-export.csv")
	p.wantCode(rr, 0, 1)
	resBank := parseCSV(t, readFile(t, p.find("restored", "bank-export.csv")))
	for i := range rawBank {
		for c, want := range rawBank[i] {
			got := resBank[i][c]
			if c == "Description" { // bank writes names in capitals: compare normalised
				if normKey(got) != normKey(want) {
					t.Errorf("restored bank row %d: got %q, want %q (after normalising)", i+2, got, want)
				}
			} else if got != want {
				t.Errorf("restored bank row %d %s: got %q, want %q", i+2, c, got, want)
			}
		}
	}

	// The secret: outside the project, private permissions.
	priv := p.findPrivateDir()
	if strings.HasPrefix(priv, p.dir+string(filepath.Separator)) {
		t.Errorf("private dir %s is inside the project", priv)
	}
	for _, f := range p.files(".") {
		if filepath.Base(f) == "mapping.json" || filepath.Base(f) == "approval.json" {
			t.Errorf("secret file inside project: %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(p.userHom, ".standin")); err == nil {
		t.Errorf("STANDIN_HOME was set, but ~/.standin was made too")
	}
	if runtime.GOOS != "windows" {
		checkMode(t, priv, 0o700)
		checkMode(t, filepath.Join(priv, "mapping.json"), 0o600)
		checkMode(t, filepath.Join(priv, "approval.json"), 0o600)
	}
}

func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Errorf("stat %s: %v", path, err)
		return
	}
	if got := st.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", path, got, want)
	}
}

// The scrub command says where the private store is.
func TestScrubPrintsPrivatePath(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.useConfig()
	r := p.run(nil, "scrub", "raw/invoices.csv")
	p.wantCode(r, 1)
	priv := p.findPrivateDir()
	if !strings.Contains(r.stdout+r.stderr, filepath.Base(priv)) {
		t.Errorf("scrub should print the private store path %s\n%s", priv, r)
	}
	checkMode(t, filepath.Join(priv, "mapping.json"), 0o600)
}

// One scrub over raw/** must give the same result as scrubbing invoices first.
// bank-export.csv sorts before invoices.csv, so a one-pass scrub would not yet know the names.
// The contract does not say which; this test records the behaviour.
func TestScrubAllAtOnceFindsNamesInBank(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.addRaw("bank-export.csv", "bank-export.csv")
	p.useConfig()
	p.approve()
	p.wantCode(p.run(nil, "scrub"), 0, 1)
	p.noLeaksInDirs(knownValues(t), "scrubbed")
	p.noLeaksInOutput(knownValues(t))
}

// Success check 4 and the restore rules.
func TestRestoreStandins(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.useConfig()
	p.wantCode(p.run(nil, "scrub", "raw/invoices.csv"), 1) // unapproved: fine for this test
	rawInv := parseCSV(t, string(fixture(t, "invoices.csv")))
	scrInv := parseCSV(t, readFile(t, p.find("scrubbed", "invoices.csv")))
	persons := map[string]string{}
	tokenMap(t, rawInv, scrInv, "Customer", "PERSON", persons)
	for _, tok := range []string{"[PERSON_1]", "[PERSON_7]", "[PERSON_12]"} {
		if _, ok := persons[tok]; !ok {
			t.Fatalf("scrub did not make %s (have %d people)", tok, len(persons))
		}
	}
	vars := variants(rawInv, "Customer")

	// restore - refuses a piped stdout, so real values never land in an agent transcript.
	r := p.run([]byte("[PERSON_1]\n"), "restore", "-")
	if r.code != 2 || r.stdout != "" {
		t.Errorf("restore - with piped stdout: want exit 2 and no output\n%s", r)
	}

	in := "Paid: [PERSON_1], [PERSON_12] and [PERSON_7].\n[PERSON_12][PERSON_1]\n"
	r = p.restoreText("known.md", in)
	want := strings.NewReplacer("[PERSON_12]", persons["[PERSON_12]"], "[PERSON_1]", persons["[PERSON_1]"],
		"[PERSON_7]", persons["[PERSON_7]"]).Replace(in)
	if r.stdout != want {
		t.Errorf("restore -:\n got %q\nwant %q", r.stdout, want)
	}
	anyVar := vars[normKey(persons["[PERSON_1]"])] || vars[normKey(persons["[PERSON_7]"])] ||
		vars[normKey(persons["[PERSON_12]"])]
	if !anyVar && r.code != 0 {
		t.Errorf("restore of known stand-ins only: want exit 0\n%s", r)
	}
	if persons["[PERSON_1]"] == persons["[PERSON_7]"] {
		t.Errorf("[PERSON_1] and [PERSON_7] should be different people")
	}

	// Unknown stand-ins and near-misses: unchanged, reported by stand-in, exit 1.
	in = "Ask [PERSON_99]. Also [person_1], PERSON_1, [PERSON 1] and Person 1.\n"
	r = p.restoreText("unknown.md", in)
	if r.code != 1 {
		t.Errorf("restore with unknown and near-miss stand-ins: want exit 1\n%s", r)
	}
	if r.stdout != in {
		t.Errorf("restore changed text it should leave alone:\n got %q\nwant %q", r.stdout, in)
	}
	if !strings.Contains(r.stderr, "[PERSON_99]") {
		t.Errorf("restore should report unknown [PERSON_99] on stderr\n%s", r)
	}
	if !strings.Contains(r.stderr, "[person_1]") {
		t.Errorf("restore should report the near-miss [person_1] on stderr\n%s", r)
	}
	noLeaks(t, "restore report on stderr", r.stderr, knownValues(t))

	// Restore to a file: output lands in restored/.
	p.write("out/reply.md", []byte("Dear [PERSON_1],\n"))
	r = p.run(nil, "restore", "out/reply.md")
	p.wantCode(r, 0, 1)
	got := readFile(t, p.find("restored", "reply.md"))
	if got != "Dear "+persons["[PERSON_1]"]+",\n" {
		t.Errorf("restored/…/reply.md = %q", got)
	}
}

// Success check 5, and the file-name check.
func TestNamesAsHeadersFlagged(t *testing.T) {
	p := newProject(t)
	p.addRaw("wide-names-as-headers.csv", "wide-names-as-headers.csv")
	r := p.run(nil, "shape", "raw/wide-names-as-headers.csv")
	p.wantCode(r, 0, 1)
	shape := readFile(t, p.find("shape", "wide-names-as-headers.csv.shape.md"))
	for _, h := range []string{"[COLUMN_2]", "[COLUMN_3]", "[COLUMN_4]", "[COLUMN_5]", "[COLUMN_6]"} {
		if !strings.Contains(shape, h) {
			t.Errorf("shape file should replace a data-like header with %s", h)
		}
	}
	for _, h := range []string{"Month", "Total"} {
		if !strings.Contains(shape, h) {
			t.Errorf("shape file should keep the plain header %q", h)
		}
	}
	noLeaks(t, "shape file", shape, knownValues(t))
	p.noLeaksInOutput(knownValues(t))
}

func TestFileNameLooksLikeData(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "Amara Okafor.csv")
	r := p.run(nil, "shape", "raw/Amara Okafor.csv")
	p.wantCode(r, 0, 1)
	files := p.files("shape")
	if len(files) != 1 {
		t.Fatalf("want one shape file, got %v\n%s", files, r)
	}
	base := filepath.Base(files[0])
	if strings.Contains(strings.ToLower(base), "okafor") || !strings.HasPrefix(base, "file_") {
		t.Errorf("shape file for a data-like file name should be file_n…, got %q", base)
	}
	noLeaks(t, "shape file", readFile(t, files[0]), knownValues(t))
	// Not checking stdout: the person needs to be told which file to rename (contract is silent).
}

// Success check 6 and fail-closed input handling.
func TestRefusesUnsupportedAndBroken(t *testing.T) {
	cases := []struct {
		fixture, as string
		msg         *regexp.Regexp // must appear in stdout+stderr
	}{
		{"statement.pdf", "statement.pdf", regexp.MustCompile(`(?i)csv`)},
		{"ledger.xlsx", "ledger.xlsx", regexp.MustCompile(`(?i)csv`)},
		{"ledger.xlsx", "ledger.csv", nil}, // a zip renamed .csv
		{"empty.csv", "empty.csv", nil},
		{"ragged.csv", "ragged.csv", regexp.MustCompile(`\b4\b`)}, // the line number
		{"invoices.csv", "invoices.txt", nil},                     // .txt is refused by extension
	}
	for _, c := range cases {
		t.Run(c.as, func(t *testing.T) {
			p := newProject(t)
			p.addRaw(c.fixture, c.as)
			p.write("standin.config.json", []byte(emptyConfig))
			for _, cmd := range []string{"shape", "scrub"} {
				r := p.run(nil, cmd, "raw/"+c.as)
				if r.code != 2 {
					t.Errorf("%s %s: want exit 2 (refused)\n%s", cmd, c.as, r)
				}
				if strings.TrimSpace(r.stdout+r.stderr) == "" {
					t.Errorf("%s %s: refused without saying why", cmd, c.as)
				}
				if c.msg != nil && !c.msg.MatchString(r.stdout+r.stderr) {
					t.Errorf("%s %s: message should match %s\n%s", cmd, c.as, c.msg, r)
				}
			}
			p.noFilesIn("shape", "scrubbed")
			p.must(0, nil, "report") // "last errors" must not carry file contents
			p.noLeaksInOutput(knownValues(t))
		})
	}
}

// Contract update: raw input that already holds stand-in-shaped text is refused.
func TestRefusesStandinShapedInput(t *testing.T) {
	for _, name := range []string{"standin-shaped-cell.csv", "standin-shaped-header.csv", "standin-shaped-key.json"} {
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			p.addRaw(name, name)
			p.write("standin.config.json", []byte(emptyConfig))
			r := p.run(nil, "scrub", "raw/"+name)
			if r.code != 2 {
				t.Errorf("want exit 2\n%s", r)
			}
			p.noFilesIn("scrubbed")
			out := r.stdout + r.stderr
			if strings.Contains(out, "Same as [PERSON_2]") || strings.Contains(out, ", ask ") {
				t.Errorf("message prints the cell value\n%s", r)
			}
			p.noLeaksInOutput(knownValues(t))
		})
	}
}

// Default-deny: keeps count only with an approval that matches the config.
func TestUnapprovedKeepIsTokenized(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.useConfig()
	r := p.run(nil, "scrub", "raw/invoices.csv")
	if r.code != 1 {
		t.Errorf("scrub without approval: want exit 1 (warning)\n%s", r)
	}
	assertTokenized(t, readFile(t, p.find("scrubbed", "invoices.csv")), "Invoice", "Date", "Amount", "Status")
}

func TestConfigChangeVoidsApproval(t *testing.T) {
	p := newProject(t)
	p.addRaw("invoices.csv", "invoices.csv")
	p.useConfig()
	p.approve()
	cfg := strings.Replace(string(fixture(t, "standin.config.json")), "customer names", "customer full names", 1)
	p.write("standin.config.json", []byte(cfg))
	r := p.run(nil, "scrub", "raw/invoices.csv")
	if r.code != 1 {
		t.Errorf("scrub after the config changed: want exit 1\n%s", r)
	}
	assertTokenized(t, readFile(t, p.find("scrubbed", "invoices.csv")), "Amount", "Date")
}

func assertTokenized(t *testing.T, text string, cols ...string) {
	t.Helper()
	for i, row := range parseCSV(t, text) {
		for _, c := range cols {
			if v := row[c]; v != "" && !wholeToken.MatchString(v) {
				t.Errorf("row %d %s: unapproved keep should be a stand-in, got %q", i+2, c, v)
			}
		}
	}
}

// restoreText writes in to out/name, restores it, and returns the result with
// stdout replaced by the restored file's text.
func (p *project) restoreText(name, in string) result {
	p.t.Helper()
	p.write("out/"+name, []byte(in))
	r := p.run(nil, "restore", "out/"+name)
	b, err := os.ReadFile(p.find("restored", name))
	if err != nil {
		p.t.Fatal(err)
	}
	r.stdout = string(b)
	return r
}

func TestApproveRefusesWithoutTTY(t *testing.T) {
	p := newProject(t)
	p.useConfig()
	r := p.run([]byte(strings.Repeat("yes\n", 40)), "approve")
	if r.code != 2 {
		t.Errorf("approve with piped stdin: want exit 2\n%s", r)
	}
	m, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "approval.json"))
	if len(m) > 0 {
		t.Errorf("approve without a terminal wrote %v", m)
	}
}

// A free-text column the config does not mention is dropped, with a warning.
func TestUnmentionedFreeTextDropped(t *testing.T) {
	p := newProject(t)
	rows := parseCSV(t, string(fixture(t, "invoices.csv")))
	var b strings.Builder
	b.WriteString("Invoice,Customer,Followup\n")
	for _, r := range rows[:10] {
		b.WriteString(r["Invoice"] + "," + r["Customer"] + ",\"Talked with " + r["Customer"] +
			" about the late invoice and they will pay after the long weekend\"\n")
	}
	p.write("raw/followups.csv", []byte(b.String()))
	p.write("standin.config.json", []byte(`{"version": 1, "files": [{"match": "raw/followups.csv", "columns": {
  "Invoice": {"action": "keep", "reason": "ours"},
  "Customer": {"action": "tokenize", "category": "PERSON", "reason": "names"}}}]}`+"\n"))
	p.approve()
	r := p.run(nil, "scrub", "raw/followups.csv")
	if r.code != 1 {
		t.Errorf("want exit 1 (warned about dropping Followup)\n%s", r)
	}
	for i, row := range parseCSV(t, readFile(t, p.find("scrubbed", "followups.csv"))) {
		if row["Followup"] != "" {
			t.Errorf("row %d Followup: want dropped (empty), got %q", i+2, row["Followup"])
		}
	}
}

func TestBadConfigRefused(t *testing.T) {
	cases := map[string]string{
		"unknown action":   `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"hide"}}}]}`,
		"unknown category": `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"NAME"}}}]}`,
		"no category":      `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize"}}}]}`,
		"unknown key":      `{"version":1,"files":[{"match":"raw/*.csv","colums":{}}]}`,
		"not json":         `{"version":1,"files":[`,
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			p.addRaw("invoices.csv", "invoices.csv")
			p.write("standin.config.json", []byte(cfg))
			r := p.run(nil, "scrub", "raw/invoices.csv")
			if r.code != 2 {
				t.Errorf("want exit 2\n%s", r)
			}
			p.noFilesIn("scrubbed")
		})
	}
}

func TestJSONInputs(t *testing.T) {
	p := newProject(t)
	p.addRaw("customers.json", "customers.json")
	p.addRaw("customers-by-name.json", "customers-by-name.json")
	p.write("standin.config.json", []byte(emptyConfig))
	kv := knownValues(t)

	// Run from outside the project with --project.
	p.cwd = p.userHom
	r := p.run(nil, "shape", "--project", p.dir, filepath.Join(p.dir, "raw", "customers.json"),
		filepath.Join(p.dir, "raw", "customers-by-name.json"))
	p.cwd = ""
	p.wantCode(r, 0, 1)
	shape := readFile(t, p.find("shape", "customers.json.shape.md"))
	for _, path := range []string{"address.city", "address.postal", "tags[]"} {
		if !strings.Contains(shape, path) {
			t.Errorf("customers shape should list %q", path)
		}
	}
	p.find("shape", "customers-by-name.json.shape.md")
	p.noLeaksInDirs(kv, "shape")

	r = p.run(nil, "scrub", "raw/customers.json", "raw/customers-by-name.json")
	p.wantCode(r, 0, 1) // no keeps in the config, so an approval may not be needed
	p.noLeaksInDirs(kv, "scrubbed")
	var arr []map[string]any
	if err := json.Unmarshal([]byte(readFile(t, p.find("scrubbed", "customers.json"))), &arr); err != nil {
		t.Fatalf("scrubbed customers.json is not a JSON array: %v", err)
	}
	if len(arr) != 24 {
		t.Errorf("scrubbed customers.json has %d records, want 24", len(arr))
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(readFile(t, p.find("scrubbed", "customers-by-name.json"))), &obj); err != nil {
		t.Fatalf("scrubbed customers-by-name.json is not a JSON object: %v", err)
	}
	for k := range obj {
		if !regexp.MustCompile(`^\[KEY_[1-9][0-9]*\]$`).MatchString(k) {
			t.Errorf("object-of-records key should be a [KEY_n] stand-in, got %q", k)
		}
	}

	p.wantCode(p.run(nil, "restore", "scrubbed/customers.json"), 0, 1)
	restored := readFile(t, p.find("restored", "customers.json"))
	var orig []map[string]any
	json.Unmarshal(fixture(t, "customers.json"), &orig)
	for _, rec := range orig {
		if name := rec["name"].(string); !strings.Contains(restored, name) {
			t.Errorf("restored customers.json is missing %q", name)
		}
	}
	p.noLeaksInOutput(kv)
}

func TestEncodings(t *testing.T) {
	t.Run("windows-1252", func(t *testing.T) {
		p := setupPair(t, "bank-export-cp1252.csv")
		out := strings.Join(p.outputs, "\n")
		if !strings.Contains(out, "1252") {
			t.Errorf("scrub should say it read the file as Windows-1252")
		}
		scr := readFile(t, p.find("scrubbed", "bank-export.csv"))
		if !utf8.ValidString(scr) {
			t.Errorf("scrubbed output is not UTF-8")
		}
		p.noLeaksInDirs(knownValues(t), "scrubbed", "shape")
		p.noLeaksInOutput(knownValues(t))
		p.wantCode(p.run(nil, "restore", "scrubbed/bank-export.csv"), 0, 1)
		res := parseCSV(t, readFile(t, p.find("restored", "bank-export.csv")))
		raw := parseCSV(t, string(fixture(t, "bank-export.csv"))) // the UTF-8 twin
		for i := range raw {
			if normKey(res[i]["Description"]) != normKey(raw[i]["Description"]) {
				t.Errorf("row %d: restored %q, want %q", i+2, res[i]["Description"], raw[i]["Description"])
			}
		}
	})
	t.Run("utf-8 BOM", func(t *testing.T) {
		p := newProject(t)
		p.addRaw("bank-export-bom.csv", "bank-export.csv")
		p.useConfig()
		p.wantCode(p.run(nil, "shape", "raw/bank-export.csv"), 0, 1)
		shape := readFile(t, p.find("shape", "bank-export.csv.shape.md"))
		if strings.Contains(shape, "\ufeff") {
			t.Errorf("shape file has the BOM in it")
		}
		p.approve()
		p.wantCode(p.run(nil, "scrub", "raw/bank-export.csv"), 0, 1)
		raw := parseCSV(t, string(fixture(t, "bank-export.csv")))
		scr := parseCSV(t, readFile(t, p.find("scrubbed", "bank-export.csv")))
		for i := range raw {
			if scr[i]["Date"] != raw[i]["Date"] {
				t.Errorf("row %d Date: %q; the BOM probably stuck to the first header so keep did not apply",
					i+2, scr[i]["Date"])
				break
			}
		}
	})
	t.Run("utf-16 BOM", func(t *testing.T) {
		p := newProject(t)
		p.addRaw("bank-export-utf16.csv", "bank-export.csv")
		p.wantCode(p.run(nil, "shape", "raw/bank-export.csv"), 0, 1)
		shape := readFile(t, p.find("shape", "bank-export.csv.shape.md"))
		if strings.Contains(shape, "\x00") || !strings.Contains(shape, "Description") {
			t.Errorf("UTF-16 file not decoded: shape file has NULs or lacks the headers")
		}
		noLeaks(t, "shape file", shape, knownValues(t))
	})
}

// The binary links no package that can open a network connection.
func TestNoNetworkPackages(t *testing.T) {
	needBinary(t)
	cmd := exec.Command("go", "list", "-deps", "./cmd/standin")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		bad := pkg == "net" || pkg == "crypto/tls" ||
			strings.HasPrefix(pkg, "net/http") || strings.HasPrefix(pkg, "net/rpc") || pkg == "net/smtp" ||
			pkg == "net/textproto" || pkg == "net/mail" || strings.Contains(pkg, "golang.org/x/net")
		if bad {
			t.Errorf("standin links %s, which can open network connections", pkg)
		}
	}
}

// `standin version` works and prints something.
func TestVersion(t *testing.T) {
	p := newProject(t)
	r := p.must(0, nil, "version")
	if strings.TrimSpace(r.stdout) == "" {
		t.Errorf("version printed nothing")
	}
}
