package restore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
)

func twelvePeople() *mapping.Mapping {
	m := mapping.New()
	for i := 1; i <= 12; i++ {
		m.Get("PERSON", fmt.Sprintf("Person Fake%d", i))
	}
	return m
}

func TestOneVersusTwelve(t *testing.T) {
	m := twelvePeople()
	r, err := Restore("[PERSON_1] [PERSON_12] [PERSON_12][PERSON_1]", m, Text)
	if err != nil {
		t.Fatal(err)
	}
	want := "Person Fake1 Person Fake12 Person Fake12Person Fake1"
	if r.Out != want || r.Issues() {
		t.Fatalf("got %q issues=%v", r.Out, r.Issues())
	}
}

func TestUnknownAndNearMissLeftAlone(t *testing.T) {
	m := twelvePeople()
	in := "Ask [PERSON_99]. Also [person_1], PERSON_1, [PERSON 1], Person 1 and [COLUMN_2].\n"
	r, _ := Restore(in, m, Text)
	if r.Out != in {
		t.Fatalf("changed text: %q", r.Out)
	}
	if len(r.Unknown) != 1 || r.Unknown[0].Text != "[PERSON_99]" {
		t.Fatalf("unknown: %+v", r.Unknown)
	}
	var near []string
	for _, f := range r.NearMiss {
		near = append(near, f.Text)
	}
	got := strings.Join(near, "|")
	if got != "[person_1]|[PERSON 1]|PERSON_1|Person 1" {
		t.Fatalf("near misses: %q", near)
	}
	for _, f := range r.NearMiss {
		if f.Like != "[PERSON_1]" {
			t.Errorf("%q: like %q", f.Text, f.Like)
		}
	}
}

func TestSpellingsReported(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Ann Testlee")
	m.Get("PERSON", "ANN TESTLEE")
	r, _ := Restore("Hi [PERSON_1]", m, Text)
	if r.Out != "Hi Ann Testlee" || len(r.Spellings) != 1 || r.Spellings[0] != "[PERSON_1]" {
		t.Fatalf("got %q %v", r.Out, r.Spellings)
	}
}

func TestCSVQuotesValues(t *testing.T) {
	m := mapping.New()
	m.Get("ORG", `Fakeco, "Ltd"`)
	r, err := Restore("Org,Note\n[ORG_1],ok\n", m, CSV)
	if err != nil {
		t.Fatal(err)
	}
	if r.Out != "Org,Note\n\"Fakeco, \"\"Ltd\"\"\",ok\n" {
		t.Fatalf("got %q", r.Out)
	}
}

func TestJSONEscapes(t *testing.T) {
	m := mapping.New()
	m.Get("ORG", `Say "hi" \ now`)
	r, _ := Restore(`{"org": "[ORG_1]"}`, m, JSON)
	if r.Out != `{"org": "Say \"hi\" \\ now"}` {
		t.Fatalf("got %s", r.Out)
	}
}

func TestLineNumbers(t *testing.T) {
	m := twelvePeople()
	r, _ := Restore("a\nb\n[PERSON_50]\n", m, Text)
	if len(r.Unknown) != 1 || r.Unknown[0].Line != 3 {
		t.Fatalf("got %+v", r.Unknown)
	}
}
