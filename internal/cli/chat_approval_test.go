package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/config"
)

func TestChatApprovalOutputFailureSavesNothing(t *testing.T) {
	for _, short := range []bool{false, true} {
		p := newTP(t)
		p.write("standin.config.json", cfg)
		input := &countReads{}
		var stderr bytes.Buffer
		env := outputEnv(p, input, brokenOutput{short: short}, &stderr)
		env.StdinIsTerminal = func() bool { return false }
		env.StdoutIsTerminal = func() bool { return false }
		code := Run([]string{"approve-columns", config.HashBytes([]byte(cfg)), "--project", p.dir}, env)
		if code != ExitRefused || input.reads != 0 || strings.Contains(stderr.String(), "Zelda Quorn") {
			t.Fatalf("output failure mishandled: status %d, reads %d, stderr %s", code, input.reads, stderr.String())
		}
		for _, name := range []string{"approval.json", "words.json", "mapping.json"} {
			matches, err := filepath.Glob(filepath.Join(p.home, "projects", "*", name))
			if err != nil || len(matches) != 0 {
				t.Fatalf("saved private state after output failure: %v, %v", matches, err)
			}
		}
	}
}

func TestChatColumnApproval(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", cfg)
	p.write("raw/invoices.csv", invoices)
	p.write("raw/bank.csv", bank)
	hash := config.HashBytes([]byte(cfg))
	review := p.run("", false, "review")
	if !strings.Contains(review.stdout, "standin approve-columns "+hash) {
		t.Fatalf("missing approval command: %+v", review)
	}
	noValues(t, "review", review.stdout+review.stderr)
	for _, args := range [][]string{{"approve-columns"}, {"approve-columns", "stale"}, {"approve-columns", hash, "extra"}} {
		if r := p.run("", false, args...); r.code != 2 {
			t.Fatalf("accepted bad approval: %+v", r)
		}
	}
	r := p.run("", false, "approve-columns", hash)
	if r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	noValues(t, "approval", r.stdout+r.stderr)
	if _, err := os.Stat(filepath.Join(p.privDir(), "mapping.json")); !os.IsNotExist(err) {
		t.Fatal("approval created mapping")
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	p.write("standin.config.json", cfg+"\n")
	if r := p.run("", false, "scrub"); r.code != 2 {
		t.Fatalf("changed config accepted: %+v", r)
	}
}

func TestChatApprovalRefusesWordReview(t *testing.T) {
	for _, rules := range []string{
		`"text":{"action":"words"}`,
		`"fields":{"Notes":{"action":"scan"}}`,
	} {
		t.Run(rules, func(t *testing.T) {
			p := newTP(t)
			body := `{"version":1,"files":[{"match":"raw/notes.md",` + rules + `}]}`
			p.write("standin.config.json", body)
			p.write("raw/notes.md", "Notes: Ann Fakelee called\n\nBo Fakeson replied.\n")
			r := p.run("yes\n", false, "approve-columns", config.HashBytes([]byte(body)))
			if r.code != 2 || !strings.Contains(r.stderr, "private word review") {
				t.Fatalf("word check accepted: %+v", r)
			}
			noValues(t, "refusal", r.stdout+r.stderr)
			matches, _ := filepath.Glob(filepath.Join(p.home, "projects", "*", "approval.json"))
			if len(matches) != 0 {
				t.Fatal("saved word approval")
			}
		})
	}
}
