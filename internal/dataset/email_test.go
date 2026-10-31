package dataset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/fail"
)

// Invented data only.

const emlPlain = "From: Oksana Drozdenko <oksana.drozdenko@example.com>\r\n" +
	"To: shop@example.com\r\n" +
	"Cc: Tavish Okorafor <tavish@example.com>, Quillfeather Bikes <hello@example.org>\r\n" +
	"Date: Wed, 02 Sep 2026 10:14:00 -0400\r\n" +
	"Subject: =?UTF-8?Q?Re=3A_tune=2Dup?=\r\n" +
	"Message-ID: <1@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"Hi Sam,\r\n" +
	"\r\n" +
	"The bike is ready. Call 416-555-0143 before Friday.\r\n" +
	"\r\n" +
	"Thanks,\r\n" +
	"Oksana Drozdenko\r\n"

func TestRenderEmailPlain(t *testing.T) {
	text, info, err := RenderEmail([]byte(emlPlain))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"From: Oksana Drozdenko <oksana.drozdenko@example.com>\n",
		"To: shop@example.com\n",
		"Cc: Tavish Okorafor <tavish@example.com>\n",
		"Cc: Quillfeather Bikes <hello@example.org>\n",
		"Date: Wed, 02 Sep 2026 10:14:00 -0400\n",
		"Subject: Re: tune-up\n", // the encoded word is decoded
		"Hi Sam,",
		"416-555-0143",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered message lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Message-ID") || strings.Contains(text, "MIME-Version") {
		t.Errorf("rendered message should not carry transport headers:\n%s", text)
	}
	if strings.Contains(text, "\r") {
		t.Error("rendered text should use line feeds only")
	}
	if info.FromHTML || info.Attachments != 0 || info.Parts != 1 {
		t.Errorf("info = %+v, want one plain part and nothing else", info)
	}
}

func TestRenderEmailAlternativePrefersPlain(t *testing.T) {
	msg := "From: Ines Vantwerp <ines@example.com>\n" +
		"To: shop@example.com\n" +
		"Subject: chain\n" +
		"Content-Type: multipart/alternative; boundary=\"BOUND\"\n" +
		"\n" +
		"--BOUND\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"\n" +
		"Plain text about Kofi Mensavi.\n" +
		"--BOUND\n" +
		"Content-Type: text/html; charset=utf-8\n" +
		"\n" +
		"<p>HTML copy about Kofi Mensavi</p>\n" +
		"--BOUND--\n"
	text, info, err := RenderEmail([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Plain text about Kofi Mensavi.") {
		t.Errorf("plain part missing:\n%s", text)
	}
	if strings.Contains(text, "HTML copy") {
		t.Errorf("HTML part should be ignored when a plain part exists:\n%s", text)
	}
	if info.FromHTML {
		t.Error("FromHTML should be false when the plain part was used")
	}
	if info.Parts != 3 {
		t.Errorf("Parts = %d, want the container and its two children", info.Parts)
	}
}

func TestRenderEmailHTMLOnly(t *testing.T) {
	msg := "From: Quillfeather Bikes <hello@example.org>\n" +
		"Subject: sale\n" +
		"Content-Type: text/html; charset=utf-8\n" +
		"\n" +
		"<html><head><title>T</title><style>.x{font-family:Arial}</style>" +
		"<script>var leak=1;</script></head><body>" +
		"<p>Hi Tavish,</p><p>Pads are 15&amp; off.<br>Ask for Yaretzi.</p></body></html>\n"
	text, info, err := RenderEmail([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hi Tavish,", "Pads are 15& off.", "Ask for Yaretzi."} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered text lacks %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"font-family", "var leak", "<p>", "<br>"} {
		if strings.Contains(text, bad) {
			t.Errorf("rendered text still holds %q:\n%s", bad, text)
		}
	}
	if !info.FromHTML {
		t.Error("FromHTML should be true when the text came from an HTML part")
	}
}

func TestRenderEmailBase64(t *testing.T) {
	body := "Bonjour,\n\nDemandez Farida Okorafor.\n"
	b64 := "Qm9uam91ciwKCkRlbWFuZGV6IEZhcmlkYSBPa29yYWZvci4K"
	msg := "From: Noemie Cote <noemie@example.com>\n" +
		"Subject: mardi\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"Content-Transfer-Encoding: base64\n" +
		"\n" + b64 + "\n"
	text, _, err := RenderEmail([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, strings.TrimSpace(body)) {
		t.Errorf("base64 body not decoded:\n%s", text)
	}
}

func TestRenderEmailAttachmentNotRead(t *testing.T) {
	msg := "From: Saoirse Brannagh <saoirse@example.com>\n" +
		"Subject: receipt attached\n" +
		"Content-Type: multipart/mixed; boundary=\"MIX\"\n" +
		"\n" +
		"--MIX\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"\n" +
		"Here is the receipt.\n" +
		"--MIX\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"Content-Disposition: attachment; filename=\"r.txt\"\n" +
		"\n" +
		"ATTACHMENT Zelda Quorn 416-555-0144\n" +
		"--MIX--\n"
	text, info, err := RenderEmail([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "ATTACHMENT") || strings.Contains(text, "Zelda") {
		t.Errorf("attachment body must not be read:\n%s", text)
	}
	if info.Attachments != 1 {
		t.Errorf("Attachments = %d, want 1", info.Attachments)
	}
}

func TestRenderEmailForwarded(t *testing.T) {
	msg := "From: shop@example.com\n" +
		"Subject: Fwd: late payment\n" +
		"Content-Type: multipart/mixed; boundary=\"F\"\n" +
		"\n" +
		"--F\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"\n" +
		"Can you look at this?\n" +
		"--F\n" +
		"Content-Type: message/rfc822\n" +
		"\n" +
		"From: Nkemdi Ellingworth <nkemdi@example.com>\n" +
		"Subject: late payment\n" +
		"\n" +
		"Sorry, the cheque goes out Monday. Bolat Zarnowski has the details.\n" +
		"--F--\n"
	text, _, err := RenderEmail([]byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- Forwarded message ---", "From: Nkemdi Ellingworth", "Subject: late payment", "Bolat Zarnowski"} {
		if !strings.Contains(text, want) {
			t.Errorf("forwarded message lacks %q:\n%s", want, text)
		}
	}
}

func TestRenderEmailRefusals(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		code string
	}{
		{"mbox", "From shop@example.com Wed Sep  2 10:14:00 2026\nFrom: a@example.com\n\nBody\n", "email-structure"},
		{"no blank line", "From: a@example.com\nSubject: x\n", "email-structure"},
		{"no headers", "just some words\nand more\n", "email-structure"},
		{"unknown charset", "From: a@example.com\nContent-Type: text/plain; charset=x-not-a-charset\n\n\x82\xa0\n", "bad-charset"},
		{"broken base64", "From: a@example.com\nContent-Transfer-Encoding: base64\n\n!!!!\n", "email-encoding"},
		{"unknown encoding", "From: a@example.com\nContent-Transfer-Encoding: x-uuencode\n\nhello\n", "email-encoding"},
		{"no boundary", "From: a@example.com\nContent-Type: multipart/mixed\n\ntext\n", "email-structure"},
		{"attachment only", "From: a@example.com\nContent-Type: application/pdf\n\n%PDF-1.4\n", "email-empty"},
		{"empty", "", "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := RenderEmail([]byte(c.msg))
			if err == nil {
				t.Fatalf("expected a refusal with code %q", c.code)
			}
			if got := fail.Code(err); got != c.code {
				t.Errorf("code = %q, want %q (%s)", got, c.code, err)
			}
		})
	}
}

func TestRenderEmailBinaryRefused(t *testing.T) {
	if _, _, err := RenderEmail([]byte("%PDF-1.4 fake")); err == nil {
		t.Error("a PDF renamed .eml should be refused")
	}
}

// A real character set decodes: a Japanese message is read, not refused.
func TestRenderEmailShiftJIS(t *testing.T) {
	msg := append([]byte("From: Tavish Okorafor <tavish@example.com>\nSubject: bike\n"+
		"Content-Type: text/plain; charset=shift_jis\n\n"),
		[]byte("\x82\xb1\x82\xf1\x82\xc9\x82\xbf\x82\xcd")...) // こんにちは
	text, _, err := RenderEmail(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "こんにちは") {
		t.Errorf("Shift-JIS body not decoded:\n%s", text)
	}
}

func TestSplitAddresses(t *testing.T) {
	got := splitAddresses(`"Quorn, Zelda" <z@example.com>, plain@example.com, Oskar <o@example.com>`)
	want := []string{`"Quorn, Zelda" <z@example.com>`, "plain@example.com", "Oskar <o@example.com>"}
	if len(got) != len(want) {
		t.Fatalf("got %d addresses %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("address %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHTMLToText(t *testing.T) {
	in := "<div>One</div><div>Two&nbsp;words</div><script>x()</script><!-- note --><p>A &amp; B</p>"
	got := HTMLToText(in)
	for _, bad := range []string{"<div>", "script", "note", "&amp;"} {
		if strings.Contains(got, bad) {
			t.Errorf("HTMLToText left %q in %q", bad, got)
		}
	}
	for _, want := range []string{"One", "Two words", "A & B"} {
		if !strings.Contains(got, want) {
			t.Errorf("HTMLToText lacks %q in %q", want, got)
		}
	}
}

// A message loaded from a .eml file is a text dataset whose header lines are
// fields, so the config, shape, scrub and restore code work unchanged.
func TestLoadEmailIsTextWithFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "simple.eml")
	if err := os.WriteFile(path, []byte(emlPlain), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Text {
		t.Errorf("Kind = %q, want text", d.Kind)
	}
	if d.Email == nil {
		t.Fatal("Email info missing")
	}
	labels := map[string]bool{}
	for i, c := range d.Columns {
		if d.Source[i] == FromField {
			labels[c] = true
		}
	}
	for _, want := range []string{"From", "To", "Cc", "Date", "Subject"} {
		if !labels[want] {
			t.Errorf("header %q did not become a field; fields are %v", want, labels)
		}
	}
	if d.FreeTextLines() == 0 {
		t.Error("body should be free text")
	}
}
