package cli

import (
	"strings"
	"testing"
)

func TestEncodedHyphenApproveScrubRestore(t *testing.T) {
	p := newTP(t)
	const original = "order-abcdefghijkl1\n"
	p.write("raw/notes.md", original)
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words","keep_groups":[]}}]}`)
	if r := p.run("yes\nnone\nyes\n", true, "approve"); r.code != 0 {
		t.Fatalf("approve: %+v", r)
	}
	if r := p.run("", false, "scrub"); r.code != 0 {
		t.Fatalf("scrub: %+v", r)
	}
	if got := p.read("scrubbed/notes.md"); !strings.HasPrefix(got, "order-[NAME_") || strings.Contains(got, "abcdefghijkl1") {
		t.Fatalf("scrub changed surrounding text or left the id: %q", got)
	}
	if r := p.run("", false, "restore", "scrubbed/notes.md"); r.code != 0 {
		t.Fatalf("restore: %+v", r)
	}
	if got := p.read("restored/notes.md"); got != original {
		t.Fatalf("restored %q, want %q", got, original)
	}
}
