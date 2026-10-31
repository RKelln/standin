package mapping

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Ann Testlee":       "ann testlee",
		"  ann   TESTLEE ":  "ann testlee",
		"Ann\tTestlee.":     "ann testlee",
		"Acme Testco Ltd.,": "acme testco ltd",
		"x;:":               "x",
		"Zoë":               "zoë",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameValueSameStandin(t *testing.T) {
	m := New()
	a := m.Get("PERSON", "Ann Testlee")
	b := m.Get("PERSON", "ann testlee ")
	c := m.Get("PERSON", "Bo Fakename")
	d := m.Get("EMAIL", "Ann Testlee")
	if a != "[PERSON_1]" || b != a || c != "[PERSON_2]" || d != "[EMAIL_1]" {
		t.Fatalf("got %s %s %s %s", a, b, c, d)
	}
	e, _ := m.Lookup(a)
	if len(e.Spellings) != 2 || e.Value() != "Ann Testlee" {
		t.Fatalf("spellings: %q", e.Spellings)
	}
}

func TestRoundTripAndCounters(t *testing.T) {
	m := New()
	for _, v := range []string{"One Fake", "Two Fake", "Three Fake"} {
		m.Get("PERSON", v)
	}
	m2, err := Parse(m.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Get("PERSON", "Four Fake"); got != "[PERSON_4]" {
		t.Fatalf("next after reload = %s", got)
	}
	if got := m2.Get("PERSON", "two fake"); got != "[PERSON_2]" {
		t.Fatalf("reload lost a key: %s", got)
	}
	if m2.Added() != 1 {
		t.Fatalf("Added = %d", m2.Added())
	}
}

func TestDamagedRefused(t *testing.T) {
	bad := []string{
		`not json`,
		`{"version":2,"standins":[]}`,
		`{"version":1,"standins":[{"standin":"[PERSON_1]","category":"ORG","key":"a","spellings":["A"]}]}`,
		`{"version":1,"standins":[{"standin":"[NICK_1]","category":"NICK","key":"a","spellings":["A"]}]}`,
		`{"version":1,"standins":[{"standin":"[PERSON_1]","category":"PERSON","key":"a","spellings":[]}]}`,
		`{"version":1,"standins":[{"standin":"[PERSON_1]","category":"PERSON","key":"a","spellings":["A"]},{"standin":"[PERSON_1]","category":"PERSON","key":"b","spellings":["B"]}]}`,
		`{"version":1,"extra":1,"standins":[]}`,
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("want refusal for %s", b)
		}
	}
}
