package e2e

// Email (.eml) end to end, on the invented fixtures in testdata/mail/
// (made by tools/make_fixtures.py). One message per file, as a mail program
// exports it: real header lines, MIME parts, transfer encodings.

import (
	"encoding/json"
	"strings"
	"testing"
)

var mailConfig = []byte(`{
  "version": 1,
  "files": [
    {
      "match": "raw/**/*.eml",
      "fields": {
        "From":    {"action": "tokenize", "category": "PERSON", "reason": "who wrote"},
        "To":      {"action": "tokenize", "category": "EMAIL", "reason": "the shop address"},
        "Cc":      {"action": "tokenize", "category": "EMAIL", "reason": "copied people"},
        "Subject": {"action": "scan", "reason": "short text; swap known names"},
        "Date":    {"action": "keep", "reason": "needed to sort"}
      },
      "text": {"action": "words", "reason": "message body; swap names"}
    }
  ]
}`)

// Every private value in the mail fixtures. Invented.
var mailValues = []string{
	"Oksana Drozdenko", "oksana.drozdenko@example.com",
	"Ines Vantwerp", "ines.vantwerp@example.com",
	"Kofi Mensavi", "Tavish",
	"Quillfeather Bikes", "hello@quillfeather.example.org",
	"Noémie Côté", "Noemie Cote", "noemie.cote@example.com",
	"Farida Okorafor", "Yaretzi",
	"Saoirse Brannagh", "saoirse.brannagh@example.com",
	"Nkemdi Ellingworth", "nkemdi.ellingworth@example.com",
	"Bolat Zarnowski",
	"416-555-0143",
}

func mailProject(t *testing.T) *project {
	p := newProject(t)
	for _, f := range []string{"simple.eml", "alternative.eml", "html-only.eml", "base64.eml", "attachment.eml", "forwarded.eml", "koi8r.eml"} {
		p.addRaw("mail/"+f, "mail/"+f)
	}
	p.addRaw("mail/simple.eml", "inbox/nested.eml") // a message one folder down
	p.write("standin.config.json", mailConfig)
	return p
}

func TestMailFixturesEndToEnd(t *testing.T) {
	p := mailProject(t)
	p.must(0, nil, "shape")

	shape := readFile(t, p.find("shape", "simple.eml.shape.md"))
	for _, want := range []string{"Kind: email message", "MIME parts read", "plain-text view"} {
		if !strings.Contains(shape, want) {
			t.Errorf("shape file lacks %q:\n%s", want, shape)
		}
	}
	// The shape files describe the messages; no value from one may appear.
	for _, f := range p.files("shape") {
		noLeaks(t, "shape "+f, readFile(t, f), mailValues)
	}
	// The HTML copy of a multipart/alternative message is not read at all.
	if sh := readFile(t, p.find("shape", "alternative.eml.shape.md")); strings.Contains(sh, "track/abc") {
		t.Error("shape file should not hold a link from the HTML part")
	}

	p.must(2, nil, "scrub") // nothing is approved yet
	approveKeepNone(t, p)
	p.must(0, nil, "scrub")

	for _, f := range p.files("scrubbed") {
		noLeaks(t, "scrubbed "+f, readFile(t, f), mailValues)
	}
	ru := readFile(t, p.find("scrubbed", "koi8r.eml"))
	// The rendered message reads like a message: fields, then the text.
	body := readFile(t, p.find("scrubbed", "simple.eml"))
	for _, want := range []string{"From: ", "To: ", "Date: ", "Subject: Re: tune-up for the blue bike"} {
		if !strings.Contains(body, want) {
			t.Errorf("scrubbed message lacks %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "[PHONE_") {
		t.Errorf("the phone number should be a stand-in:\n%s", body)
	}
	// An attachment is not read, so its words never reach the copy.
	if a := readFile(t, p.find("scrubbed", "attachment.eml")); strings.Contains(a, "ATTACHMENT BODY") {
		t.Errorf("attachment was read:\n%s", a)
	}
	// An HTML-only message becomes text.
	html := readFile(t, p.find("scrubbed", "html-only.eml"))
	if strings.Contains(html, "<p>") || strings.Contains(html, "font-family") {
		t.Errorf("HTML markup reached the scrubbed copy:\n%s", html)
	}
	if !strings.Contains(html, "15& off") {
		t.Errorf("HTML entities should be decoded:\n%s", html)
	}
	// A Japanese message is decoded to UTF-8, not refused: the words become
	// stand-ins (they have no capital letters to tell a name by), so the proof
	// is that the body is readable text rather than mojibake or an encoding name.
	// Every capitalised Russian word is not in the English word list, so the
	// word check takes them all as stand-ins: the proof of decoding is that
	// the copy is readable UTF-8 with no encoded words left, and that restore
	// gives the message back whole.
	if strings.Contains(ru, "?B?") || strings.Contains(ru, "\ufffd") || !strings.Contains(ru, "[NAME_") {
		t.Errorf("KOI8-R message not turned into readable UTF-8 text:\n%s", ru)
	}
	// A forwarded message keeps the inner message's fields.
	if fw := readFile(t, p.find("scrubbed", "forwarded.eml")); !strings.Contains(fw, "--- Forwarded message ---") {
		t.Errorf("forwarded message lost its inner headers:\n%s", fw)
	}

	// Putting the values back gives the readable message, with real values.
	p.must(0, nil, "restore", "scrubbed/mail/simple.eml")
	restored := readFile(t, p.find("restored", "simple.eml"))
	if !strings.Contains(restored, "Oksana Drozdenko") || !strings.Contains(restored, "416-555-0143") {
		t.Errorf("restore did not put the real values back:\n%s", restored)
	}
	if !strings.Contains(restored, "Hi Sam,") {
		t.Errorf("restore lost the message text:\n%s", restored)
	}
	// The Japanese message comes back whole, header included.
	p.must(0, nil, "restore", "scrubbed/mail/koi8r.eml")
	ruRestored := readFile(t, p.find("restored", "koi8r.eml"))
	if !strings.Contains(ruRestored, "Иван Петров") || !strings.Contains(ruRestored, "четверг") {
		t.Errorf("Russian message did not restore:\n%s", ruRestored)
	}
}

// A message Standin cannot read safely stops the run, and writes nothing.
func TestMailRefusals(t *testing.T) {
	for _, c := range []struct {
		fixture string
		want    string
	}{
		{"bad-charset.eml", "character set"},
		{"mbox.eml", "mbox"},
	} {
		p := newProject(t)
		p.addRaw("mail/"+c.fixture, "mail/"+c.fixture)
		p.write("standin.config.json", mailConfig)
		r := p.run(nil, "shape")
		p.wantCode(r, 2)
		if !strings.Contains(r.stderr, c.want) {
			t.Errorf("%s: refusal should mention %q:\n%s", c.fixture, c.want, r.stderr)
		}
		p.noFilesIn("shape")
	}
}

// The config example in the shape file names the email fields, so an agent
// that has never seen this file can still write the rule.
func TestMailShapeSuggestsRules(t *testing.T) {
	p := mailProject(t)
	p.must(0, nil, "shape")
	sh := readFile(t, p.find("shape", "base64.eml.shape.md"))
	if !strings.Contains(sh, "Usual rules for a message") {
		t.Fatalf("shape file should suggest the usual rules:\n%s", sh)
	}
	var cfg map[string]any
	if err := json.Unmarshal(mailConfig, &cfg); err != nil {
		t.Fatal(err)
	}
}
