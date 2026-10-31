package cli

import (
	"strings"
	"testing"
)

func TestRestoreMalformedFileWritesNoOtherOutputs(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"answer.json", `{"Who":"[VALUE_1]"`},
		{"answer.csv", "Who,Status\n[VALUE_1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newTP(t)
			p.write("raw/inv.csv", "Customer\nZelda Quorn\n")
			p.write("standin.config.json", `{"version":1,"files":[]}`)
			if r := p.run("", false, "scrub"); r.code == 2 {
				t.Fatal(r)
			}
			p.write("good.md", "Dear [VALUE_1]")
			p.write(tc.name, tc.body)
			r := p.run("", false, "restore", "good.md", tc.name)
			if r.code != 2 || p.exists("restored") || strings.Contains(r.stderr, "Zelda") {
				t.Fatalf("malformed restore published data: %+v", r)
			}
		})
	}
}

func TestRestoreNewFormulaStaysTokenized(t *testing.T) {
	p := newTP(t)
	p.write("raw/inv.csv", "Notes\n=1+1\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	if r := p.run("", false, "scrub"); r.code == 2 {
		t.Fatal(r)
	}
	r := p.run("", false, "restore", "scrubbed/inv.csv")
	if r.code != 1 || p.read("restored/inv.csv") != "Notes\n[VALUE_1]\n" || !strings.Contains(r.stderr, "would make") {
		t.Fatalf("formula was restored without a warning: %+v", r)
	}
}
