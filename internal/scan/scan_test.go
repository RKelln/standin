package scan

import (
	"testing"

	"github.com/RKelln/standin/internal/mapping"
)

func kinds(s string) []string {
	var out []string
	for _, h := range FindPatterns(s, nil) {
		out = append(out, h.Kind+"="+s[h.Start:h.End])
	}
	return out
}

func TestPatterns(t *testing.T) {
	cases := []struct {
		in   string
		want string // kind=text of the first hit, or "" for none
	}{
		{"write to fake.person@example.com today", "email=fake.person@example.com"},
		{"call 416-555-0199 now", "phone number=416-555-0199"},
		{"call (604) 555 0123", "phone number=(604) 555 0123"},
		{"intl +44 20 7946 0958", "phone number=+44 20 7946 0958"},
		{"card 4111 1111 1111 1111 on file", "card number=4111 1111 1111 1111"},
		{"card 4111 1111 1111 1112 bad luhn", "long number (account?)="},
		{"SIN 130 692 544", "SIN (social insurance number)=130 692 544"},
		{"SIN 130-692-544", "SIN (social insurance number)=130-692-544"},
		{"SIN 130692544", "SIN (social insurance number)=130692544"},
		{"IBAN GB82 WEST 1234 5698 7654 32", "bank account (IBAN)=GB82 WEST 1234 5698 7654 32"},
		{"ref 123456789012", "long number (account?)=123456789012"},
		{"amount 12345678.90", ""},
		{"Toronto M5V 2T6", "postal code=M5V 2T6"},
		{"Albany NY 12207", "ZIP code=NY 12207"},
		{"London SW1A 1AA", "postcode=SW1A 1AA"},
		{"at 45 Queen St W", "street address=45 Queen St W"},
		{"nothing here, 2026-01-05, 1399.70", ""},
		{"[PERSON_12345678] is a stand-in", ""},
	}
	for _, c := range cases {
		got := kinds(c.in)
		switch {
		case c.want == "" && len(got) != 0:
			t.Errorf("%q: want no hit, got %v", c.in, got)
		case c.want == "long number (account?)=":
			// Bad Luhn: not a card. Spaced digits are not one long run either.
			for _, g := range got {
				if g[:4] == "card" {
					t.Errorf("%q: bad Luhn reported as card", c.in)
				}
			}
		case c.want != "" && (len(got) == 0 || got[0] != c.want):
			t.Errorf("%q: want %q, got %v", c.in, c.want, got)
		}
	}
}

func TestSINMixedSeparatorsNotSIN(t *testing.T) {
	for _, h := range FindPatterns("id 130 692-544", nil) {
		if h.Kind == "SIN (social insurance number)" {
			t.Fatal("mixed separators should not be a SIN (the original pattern needs the same separator)")
		}
	}
}

func TestLuhn(t *testing.T) {
	if !LuhnOK("4111111111111111") || LuhnOK("4111111111111112") || LuhnOK("") {
		t.Fatal("Luhn wrong")
	}
}

func TestKnownValuesUnicodeAndLongestFirst(t *testing.T) {
	m := mapping.New()
	renee := m.Get("PERSON", "Renée Fakeface")
	ren := m.Get("PERSON", "Renée")
	org := m.Get("ORG", "Fake Org")
	orgLong := m.Get("ORG", "Fake Org Holdings")
	m.Get("PERSON", "Al") // too short: skipped
	k := NewKnown(m)
	out, _ := Replace("RENÉE FAKEFACE paid. Renée too. Renéex not. Fake Org Holdings and fake  org. Al stays.", k, m)
	want := renee + " paid. " + ren + " too. Renéex not. " + orgLong + " and " + org + ". Al stays."
	if out != want {
		t.Fatalf("got  %q\nwant %q", out, want)
	}
}

func TestKnownBoundaryFallback(t *testing.T) {
	m := mapping.New()
	short := m.Get("PERSON", "Ann Fake")
	m.Get("PERSON", "Ann Fakeson")
	k := NewKnown(m)
	// The longer value does not fit at a word end here; the shorter one does.
	out, _ := Replace("Ann Fakesonx and Ann Fake.", k, m)
	if out != "Ann Fakesonx and "+short+"." {
		t.Fatalf("got %q", out)
	}
}

func TestReplacePatternsAddStandins(t *testing.T) {
	m := mapping.New()
	out, n := Replace("Mail fake.person@example.com or call 416-555-0199.", NewKnown(m), m)
	if out != "Mail [EMAIL_1] or call [PHONE_1]." || n != 2 {
		t.Fatalf("got %q %d", out, n)
	}
}

func TestLooksLikeData(t *testing.T) {
	yes := map[string]string{
		"Amara Fakeperson":        "name",
		"Lucía Fernándezfake":     "name",
		"fake.person@example.com": "email",
		"604-555-0120":            "phone",
		"10023":                   "digits",
		"Client 12345678":         "digits",
		"ZELDA QUORN":             "name",
		"Sean O'Brien":            "name",
		"Ann McNab":               "name",
		"Bill Gatesby":            "name",
		"lee_pip":                 "name",
		"Balance.Zelda Quorn":     "name",
	}
	for in, want := range yes {
		if got, ok := LooksLikeData(in); !ok || got != want {
			t.Errorf("LooksLikeData(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"Month", "Total", "Customer Name", "Invoice Date", "address.city", "2026-01", "2026", "Amount", "invoices", "bank-export", "Unit Price", "TOTAL AMOUNT", "last_order", "card_on_file", "invoices.csv"} {
		if kind, ok := LooksLikeData(in); ok {
			t.Errorf("LooksLikeData(%q) = %q; want not data", in, kind)
		}
	}
}

func TestHasStandin(t *testing.T) {
	if !HasStandin("Same as [PERSON_2], ask") || HasStandin("[person_2]") || HasStandin("[PERSON_0]") {
		t.Fatal("HasStandin wrong")
	}
}

func TestKnownNoBreakSpace(t *testing.T) {
	m := mapping.New()
	st := m.Get("PERSON", "Zelda Quorn")
	out, _ := Replace("call Zelda\u00a0Quorn and Zelda\u2003 Quorn", NewKnown(m), m)
	if out != "call "+st+" and "+st {
		t.Fatalf("got %q", out)
	}
}

// A NAME word the person chose to keep is no longer looked for, but its
// stand-in still restores (the row stays in the mapping).
func TestKeptWordsAreNotKnown(t *testing.T) {
	m := mapping.New()
	m.Get("NAME", "Shimano")
	m.Get("NAME", "Oksana")
	m.SetKept(func(k string) bool { return k == "shimano" })
	k := NewKnown(m)
	if got, _ := Replace("Shimano pads for Oksana", k, m); got != "Shimano pads for [NAME_2]" {
		t.Errorf("got %q", got)
	}
	if _, ok := m.Lookup("[NAME_1]"); !ok {
		t.Error("the kept word's stand-in must stay in the mapping")
	}
	b := m.Marshal()
	m2, err := mapping.Parse(b)
	if err != nil || !m2.Entries[0].Kept || m2.Entries[1].Kept {
		t.Errorf("kept flag lost: %v %+v", err, m2)
	}
}

// A known value whose first spelling ends in a dot keeps the dot inside the
// hit, so restore does not write it twice.
func TestKnownValueTakesItsDot(t *testing.T) {
	m := mapping.New()
	m.Get("ORG", "Quillfeather Courier Co.")
	got, _ := Replace("One courier, Quillfeather Courier Co., pays late.", NewKnown(m), m)
	if got != "One courier, [ORG_1], pays late." {
		t.Errorf("got %q", got)
	}
}
