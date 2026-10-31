package dataset

import (
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/fail"
)

// Invented data only.

const note = "# Visit notes\r\n" +
	"\r\n" +
	"- **From:** Tavish Okorafor <tavish@example.com>\r\n" +
	"Date: 2026-03-04\r\n" +
	"Leila: see you then\r\n" +
	"See: https://example.com/x\r\n" +
	"\r\n" +
	"| Service | Price |\r\n" +
	"|---|---:|\r\n" +
	"| Tune-up | $89 |\r\n" +
	"|Flat \\| tube|$25|\r\n" +
	"\r\n" +
	"Hi Oksana, the bike is ready.\r\n"

func TestParseText(t *testing.T) {
	d, err := Parse(Text, []byte(note))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"From", "Date", "See", "Service", "Price"}
	if strings.Join(d.Columns, ",") != strings.Join(want, ",") {
		t.Fatalf("columns %q, want %q", d.Columns, want)
	}
	if d.Source[0] != FromField || d.Source[3] != "table 1" || d.Tables != 1 {
		t.Errorf("sources %q, tables %d", d.Source, d.Tables)
	}
	if d.Values[0][0] != "Tavish Okorafor <tavish@example.com>" || d.Values[3][1] != `Flat \| tube` {
		t.Errorf("values %q", d.Values)
	}
	lines := d.TextLines()
	if lines[4] != "Leila: see you then" || lines[2] != "" || lines[12] != "Hi Oksana, the bike is ready." {
		t.Errorf("a chat line with a name for a label stays text; fields are blank: %q", lines)
	}
	if d.Where(1, 0) != "line 4" {
		t.Errorf("where: %s", d.Where(1, 0))
	}
	// Unchanged, the file comes back byte for byte (line endings and table layout too).
	same, _ := d.Write(Rewriter{Cell: func(int, string) (string, bool) { return "", false }, Name: func(s string) string { return s }})
	if string(same) != note {
		t.Errorf("round trip changed the file:\n%q", same)
	}
	out, _ := d.Write(Rewriter{
		Cell: func(c int, v string) (string, bool) { return "<" + d.Columns[c] + ">", true },
		Name: func(s string) string { return strings.ToUpper(s) },
		Text: func(_ int, s string) string { return strings.ReplaceAll(s, "Oksana", "[NAME_1]") },
	})
	for _, w := range []string{"- **From:** <From>\r\n", "| SERVICE | PRICE |\r\n", "|<Service>|<Price>|\r\n", "Hi [NAME_1], the", "Leila: see you then"} {
		if !strings.Contains(string(out), w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func TestTextRefusals(t *testing.T) {
	cases := map[string]struct{ text, code string }{
		"ragged table": {"| A | B |\n|---|---|\n| 1 | 2 |\n| 3 |\n", "ragged"},
		"lone CR":      {"Hi\rthere\n", "lone-cr"},
		"uncased":      {"# Notes\n" + strings.Repeat("諕齋蟪狳躶傍孰沨麮榽", 3) + " ok\n", "uncased-script"},
	}
	for name, c := range cases {
		_, err := Parse(Text, []byte(c.text))
		if err == nil || fail.Code(err) != c.code {
			t.Errorf("%s: got %v, want code %s", name, err, c.code)
		}
	}
	// A few uncased letters (a name) are fine: those words are swapped.
	if _, err := Parse(Text, []byte("The note says 王伟 called about the bike on Monday.\n")); err != nil {
		t.Errorf("a short uncased name should not refuse: %v", err)
	}
}

func TestTxtThatIsATable(t *testing.T) {
	if !looksLikeTable("Date,Item,Amount\n2026-01-02,Tubes,12.00\n2026-01-03,\"Pads, front\",28.00\n") {
		t.Error("a CSV with a quoted comma is still a table")
	}
	// Known: three short lines with one comma each also read as a table, so
	// such a .txt is refused (the safe side). This test records it.
	if !looksLikeTable("Hi Sam, the bike is ready.\nThanks, see you.\nBye, now.\n") {
		t.Error("the table check changed; update DESIGN decision 39")
	}
	if looksLikeTable("NORTHGATE CYCLE\nReceipt R-01\nTOTAL   558.22\n") {
		t.Error("a receipt is not a table")
	}
}
