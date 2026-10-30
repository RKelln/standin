package cli

// Tests for the fixes from the fresh review (DESIGN.md decisions 24-31).
// Each began as a failing repro. Invented data only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scrubOut(t *testing.T, raw, cfg string) (res, string) {
	t.Helper()
	p := newTP(t)
	p.write("raw/inv.csv", raw)
	p.write("standin.config.json", cfg)
	r := p.run("", false, "scrub")
	if !p.exists("scrubbed/inv.csv") {
		return r, ""
	}
	return r, p.read("scrubbed/inv.csv")
}

// 24: scan leaves unknown words in, so it needs approval like keep.
func TestUnapprovedScanIsTokenized(t *testing.T) {
	r, out := scrubOut(t, "Customer\nZelda Quorn\n",
		`{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"scan"}}}]}`)
	if strings.Contains(out, "Zelda Quorn") || out != "Customer\n[VALUE_1]\n" {
		t.Errorf("unapproved scan column: got\n%s", out)
	}
	if r.code != 1 || !strings.Contains(r.stderr, "keep or scan is tokenized") {
		t.Errorf("want a warning, exit 1: %+v", r)
	}
}

func TestApproveAsksForEachScan(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer,Notes\nZelda Quorn,call Zelda Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"},"Notes":{"action":"scan"}}}]}`)
	r := p.run("no\n", true, "approve")
	if r.code != 2 || !strings.Contains(r.stdout, "Scan Notes in raw/*.csv?") || !strings.Contains(r.stdout, "names Standin does not know stay in") {
		t.Fatalf("%+v", r)
	}
	if r := p.run("yes\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(p.privDir(), "approval.json"))
	if !strings.Contains(string(b), `"raw/*.csv:Notes"`) {
		t.Errorf("approval.json should list the scan: %s", b)
	}
	if r := p.run("", false, "scrub"); r.code != 0 || p.read("scrubbed/inv.csv") != "Customer,Notes\n[PERSON_1],call [PERSON_1]\n" {
		t.Fatalf("%+v\n%s", r, p.read("scrubbed/inv.csv"))
	}
}

// 31: a CSV with old-Mac line endings (CR only) would read as one header row.
func TestLoneCRRefused(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Name,Notes\rZelda Quorn,owes money\rPip Varga,paid late again\r")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "shape")
	if r.code != 2 || !strings.Contains(r.stderr, "normal line endings") {
		t.Errorf("shape: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 2 {
		t.Errorf("scrub: %+v", r)
	}
	if p.exists("shape") || p.exists("scrubbed") {
		t.Error("wrote output")
	}
}

// 31: a no-break space inside a known name is still matched in scan text.
func TestKnownScanNoBreakSpace(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer,Notes\nZelda\u00a0Quorn,call Zelda\u00a0Quorn back\nPip Varga,ask Pip\u202fVarga\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"},"Notes":{"action":"scan"}}}]}`)
	p.approve()
	p.run("", false, "scrub")
	if out := p.read("scrubbed/inv.csv"); strings.Contains(out, "Quorn") || strings.Contains(out, "Varga") {
		t.Errorf("name with a Unicode space kept in a scan column:\n%s", out)
	}
}

// Known misses, kept on purpose (DESIGN.md 31): the standard library has no
// Unicode normal forms, and lower-casing Turkish İ changes its length. This
// test logs what happens; it does not fail.
func TestKnownMissUnicodeForms(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer,Notes\nİlker Yaman,met İlker Yaman\nRene\u0301e Quorn,met Ren\u00e9e Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"},"Notes":{"action":"scan"}}}]}`)
	p.approve()
	p.run("", false, "scrub")
	out := p.read("scrubbed/inv.csv")
	for _, v := range []string{"Yaman", "Quorn"} {
		if strings.Contains(out, v) {
			t.Logf("known miss: %s is still in a scan column", v)
		}
	}
}

// 31: shape and scrub use the same header check, including dotted parts.
func TestDottedHeaderHiddenInBoth(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Balance.Zelda Quorn\n12.50\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.run("", false, "shape")
	if strings.Contains(p.read("shape/inv.csv.shape.md"), "Zelda") {
		t.Error("shape keeps the header")
	}
	p.run("", false, "scrub")
	if out := p.read("scrubbed/inv.csv"); strings.Contains(out, "Zelda Quorn") {
		t.Errorf("scrubbed keeps the header:\n%s", out)
	}
}

// 31: names as headers in more forms.
func TestNameHeadersFlagged(t *testing.T) {
	_, out := scrubOut(t, "Month,Bill Gatesby,ZELDA QUORN,Sean O'Brien,Ann McNab,lee_pip,Unit Price,Customer Name\nJan,1,2,3,4,5,6,7\n", `{"version":1,"files":[]}`)
	head := strings.SplitN(out, "\n", 2)[0]
	for _, v := range []string{"Gatesby", "QUORN", "O'Brien", "McNab", "lee_pip"} {
		if strings.Contains(head, v) {
			t.Errorf("name header %q kept in scrubbed header", v)
		}
	}
	for _, v := range []string{"Month", "Unit Price", "Customer Name"} {
		if !strings.Contains(head, v) {
			t.Errorf("plain header %q should stay", v)
		}
	}
}

// 31: a header equal to a value already known is swapped.
func TestHeaderEqualToKnownValueSwapped(t *testing.T) {
	p := newTP(t)
	p.write("raw/a.csv", "Customer\nPip\n")
	p.write("raw/b.csv", "Month,Pip\nJan,4\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/a.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"}}}]}`)
	p.run("", false, "scrub")
	if head := strings.SplitN(p.read("scrubbed/b.csv"), "\n", 2)[0]; head != "Month,[PERSON_1]" {
		t.Errorf("got %q", head)
	}
}

// 30: a dropped name column still teaches scan the name.
func TestDroppedNamesKnownToScan(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Contact,Notes\nZelda Quorn,asked Zelda Quorn to pay\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Contact":{"action":"drop","category":"PERSON"},"Notes":{"action":"scan"}}}]}`)
	p.approve()
	p.run("", false, "scrub")
	if out := p.read("scrubbed/inv.csv"); out != "Contact,Notes\n,asked [PERSON_1] to pay\n" {
		t.Errorf("got\n%s", out)
	}
}

// 26: off a screen, check gives one bit: found something, or not.
func TestCheckOffScreenGivesOneBit(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Customer":{"action":"tokenize","category":"PERSON"}}}]}`)
	p.run("", false, "scrub")
	p.write("guesses.txt", "Ann Other\nZelda Quorn\nfake.person@example.com\n")
	r := p.run("", false, "check", "guesses.txt")
	if r.code != 1 || !strings.Contains(r.stderr, "run `standin check` in their own terminal") {
		t.Fatalf("%+v", r)
	}
	for _, bad := range []string{"[PERSON_1]", "line", "email", "1 thing"} {
		if strings.Contains(r.stdout+r.stderr, bad) {
			t.Errorf("off-screen check says %q:\n%s", bad, r.stderr)
		}
	}
	p.write("clean.txt", "nothing to see\n")
	if r := p.run("", false, "check", "clean.txt"); r.code != 0 || !strings.Contains(r.stderr, "Found nothing") {
		t.Errorf("%+v", r)
	}
}

// 31: a name hidden in the extension is never printed.
func TestDataInExtensionNotPrinted(t *testing.T) {
	p := newTP(t)
	p.write("raw/statement.Zelda Quorn", "a\n1\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	r := p.run("", false, "scrub")
	sh := p.run("", false, "shape", "raw/statement.Zelda Quorn")
	rep := p.run("", false, "report")
	all := strings.ToLower(r.stderr + sh.stderr + rep.stdout)
	if r.code != 2 || strings.Contains(all, "zelda") || strings.Contains(all, "quorn") {
		t.Errorf("name in the extension printed:\nscrub: %s\nshape: %s\nreport: %s", r.stderr, sh.stderr, rep.stdout)
	}
}

// 31: two restore inputs with the same output name are refused.
func TestRestoreSameOutputRefused(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.run("", false, "scrub")
	p.write("out/a/reply.md", "Hi [VALUE_1]\n")
	p.write("out/b/reply.md", "Bye [VALUE_1]\n")
	if r := p.run("", false, "restore", "out/a/reply.md", "out/b/reply.md"); r.code != 2 || p.exists("restored") {
		t.Fatalf("%+v", r)
	}
}

// 29: stand-ins in links and formulas are left alone and reported.
func TestRestoreLeavesLinksAndFormulas(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.run("", false, "scrub")
	p.write("out/r.md", "Dear [VALUE_1],\n[pay](https://pay.example.com/?n=[VALUE_1])\n")
	p.write("out/r.csv", "Who,Link\n[VALUE_1],\"=HYPERLINK(\"\"x?[VALUE_1]\"\")\"\n")
	r := p.run("", false, "restore", "out/r.md", "out/r.csv")
	if r.code != 1 || !strings.Contains(r.stderr, "web link or a spreadsheet formula") {
		t.Fatalf("%+v", r)
	}
	if got := p.read("restored/r.md"); got != "Dear Zelda Quorn,\n[pay](https://pay.example.com/?n=[VALUE_1])\n" {
		t.Errorf("md: %q", got)
	}
	if got := p.read("restored/r.csv"); got != "Who,Link\nZelda Quorn,\"=HYPERLINK(\"\"x?[VALUE_1]\"\")\"\n" {
		t.Errorf("csv: %q", got)
	}
	if strings.Contains(r.stderr, "Zelda") {
		t.Error("report printed a value")
	}
}
