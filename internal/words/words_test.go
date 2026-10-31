package words

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
)

// All names here are invented.

func TestMain(m *testing.M) {
	if err := Load(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestListsLoad(t *testing.T) {
	if len(english) < 70000 || len(first) < 20000 {
		t.Fatalf("lists too small: %d words, %d names", len(english), len(first))
	}
	for _, w := range []string{"grace", "brown", "rich", "bike", "tune-up", "Shop's", "I'm"} {
		if !InDict(w) {
			t.Errorf("%q should be an English word", w)
		}
	}
	// Capitalised SCOWL entries (names) are left out on purpose.
	for _, w := range []string{"Nguyen", "Okafor", "Quellbrook", "Monday", "Toronto"} {
		if InDict(w) {
			t.Errorf("%q should not be an English word here", w)
		}
	}
	for _, w := range []string{"Kofi", "Oksana", "Renée", "grace"} {
		if !IsFirstName(w) {
			t.Errorf("%q should be in the first-name list", w)
		}
	}
	if Key("Leila’s") != "leila" || Key("O'Brien") != "o'brien" {
		t.Errorf("Key: %q %q", Key("Leila’s"), Key("O'Brien"))
	}
}

func find(text string) *Finder {
	f := NewFinder(nil)
	f.AddLines(strings.Split(text, "\n"))
	return f
}

func reasons(f *Finder, w string) Reason {
	if c := f.Get(Key(w)); c != nil {
		return c.Reasons
	}
	return 0
}

func TestFinderRules(t *testing.T) {
	cases := []struct {
		text, word string
		want       Reason // 0: not a candidate
		group      string
	}{
		{"Hi Grace, the bike is ready.", "Grace", NameSpot, GroupNameLike},
		{"Dear Hope,", "Hope", NameSpot, GroupNameLike},
		{"I spoke to Dr. Brannagh today.", "Brannagh", NameSpot | NotInDict, GroupNameLike},
		{"My name is Rose and I ride.", "Rose", NameSpot, GroupNameLike},
		{"Thanks,\nRose", "Rose", SignOff, GroupNameLike},
		{"Bring it in.\n-- Rose", "Rose", SignOff, GroupNameLike},
		{"The new Shimano pads are in.", "Shimano", NotInDict, GroupUnusual},
		{"Rose called about the bike.", "Rose", 0, ""}, // a word, at a sentence start: a known miss
		{"I met with Grace and her dad.", "Grace", NameSpot, GroupNameLike},
		{"Then Grace came by.", "Grace", FirstName, GroupNameLike},
		{"Ask Kofi Brown about it.", "Brown", NextToName, GroupNameLike},
		{"Open on Monday and Tuesday.", "Monday", Calendar, GroupCalendar},
		{"Hi April, see you soon.", "April", NameSpot | Calendar, GroupNameLike},
		{"thx, it's oksana again", "oksana", FirstName, GroupNameLike},
		{"a grace period", "grace", 0, ""},
		{"See price-sheet.md for more.", "md", 0, ""},
		{"Will you open on Sunday?", "Will", 0, ""},
		{"Hi there, Hi team, Thanks all", "there", 0, ""},
		{"The note says 王伟 called.", "王伟", Uncased, GroupUnusual},
		// Review fixes (decisions 47-49).
		{"author: zelda quorn", "quorn", NextToName, GroupNameLike},
		{"Ask (Grace) about it.", "Grace", FirstName, GroupNameLike},
		{"Then **Grace** came by.", "Grace", FirstName, GroupNameLike},
		{"Notes: Grace has the keys.", "Grace", FirstName, GroupNameLike},
		{"**Grace** came by.", "Grace", 0, ""}, // a sentence start, past the marks
		{"Rose Fairweather called.", "Rose", NextToName, GroupNameLike},
		{"April Okonkwo called.", "April", NextToName | Calendar, GroupNameLike},
		{"Meet June Fairbanks-Ndlovu at noon.", "June", NextToName | Calendar, GroupNameLike},
		{"Ana Okonkwo June", "June", NextToName | Calendar, GroupNameLike},
		{"Open on Monday at noon.", "Monday", Calendar, GroupCalendar},
		// Days and months that are also first names stay in the calendar group
		// unless they sit in a name spot.
		{"Tue, Wed, Friday, Saturday 10am to 6pm.", "Friday", Calendar, GroupCalendar},
		{"You said Thursday. It's Friday. Is it done?", "Friday", Calendar, GroupCalendar},
		{"We open in April and May.", "May", Calendar, GroupCalendar},
		{"Hi May, see you soon.", "May", NameSpot | Calendar, GroupNameLike},
	}
	for _, c := range cases {
		f := find(c.text)
		got := reasons(f, c.word)
		if got&c.want != c.want || (c.want == 0) != (got == 0) {
			t.Errorf("%q in %q: reasons %b, want %b", c.word, c.text, got, c.want)
			continue
		}
		if c.want != 0 {
			if g := f.Get(Key(c.word)).Group(); g != c.group {
				t.Errorf("%q in %q: group %s, want %s", c.word, c.text, g, c.group)
			}
		}
	}
}

func TestFieldPartsCatchWordNames(t *testing.T) {
	f := NewFinder(nil)
	f.AddFieldParts("Daisy Quellbrook <daisy.q@example.com>")
	f.AddLines([]string{"Daisy said the wheel wobbles."})
	if reasons(f, "Daisy")&FieldPart == 0 {
		t.Error("a field part at a sentence start should be caught")
	}
}

func TestKnownValuesAreNotCandidates(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Tavish Okorafor")
	f := NewFinder(scan.NewKnown(m))
	f.AddLines([]string{"Hi Tavish Okorafor, email me at t.o@example.com"})
	if len(f.Candidates()) != 0 {
		t.Errorf("known values and patterns should be left to the scan: %v", f.Candidates()[0].Word)
	}
}

func TestReplace(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Tavish Okorafor")
	known := scan.NewKnown(m)
	swap := func(k string) bool { return k == "oksana" || k == "rose" || k == "shimano" }
	cases := []struct{ in, want string }{
		{"Hi Oksana, Rose's bike is done.", "Hi [NAME_1], [NAME_2]'s bike is done."},
		{"thx oksana", "thx [NAME_1]"},
		{"a rose garden", "a rose garden"}, // lower case and a word: stays
		{"OKSANA called", "[NAME_1] called"},
		{"Tavish Okorafor at 416-555-0142", "[PERSON_1] at [PHONE_1]"},
		{"**Rose** - [Shimano](https://example.com)", "**[NAME_2]** - [[NAME_3]](https://example.com)"},
	}
	for _, c := range cases {
		if got := Replace(c.in, known, m, swap, false); got != c.want {
			t.Errorf("Replace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Parts of a word joined by - . or _ are swapped when on the list.
func TestReplaceJoinedWords(t *testing.T) {
	m := mapping.New()
	swap := func(k string) bool { return k == "oksana" || k == "kavya" || k == "ramaswamy" }
	cases := []struct{ in, want string }{
		{"see /people/oksana-vantreeck", "see /people/[NAME_1]-vantreeck"},
		{"mail kavya.ramaswamy@ or kavya_ramaswamy", "mail [NAME_2].[NAME_3]@ or [NAME_2]_[NAME_3]"},
		{"a well-known shop", "a well-known shop"},
	}
	for _, c := range cases {
		if got := Replace(c.in, nil, m, swap, false); got != c.want {
			t.Errorf("Replace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A one-word NAME value from the mapping is swapped wherever it is, also in
// lower case inside a link, unless it is a day name the rule keeps.
func TestHitsKnownOneWordNames(t *testing.T) {
	m := mapping.New()
	m.Get(Category, "Ignatius")
	m.Get(Category, "Monday")
	known := scan.NewKnown(m)
	no := func(string) bool { return false }
	if got := Replace("see https://crm.example/ignatius", known, m, no, false); got != "see https://crm.example/[NAME_1]" {
		t.Errorf("got %q", got)
	}
	if got := Replace("Open Monday", known, m, no, true); got != "Open Monday" {
		t.Errorf("a kept day name was swapped: %q", got)
	}
	if got := Replace("Open Monday", known, m, no, false); got != "Open [NAME_2]" {
		t.Errorf("got %q", got)
	}
	f := NewFinder(known)
	f.AddLines([]string{"see https://crm.example/ignatius"})
	if c := f.Get("ignatius"); c == nil || c.Reasons&KnownName == 0 {
		t.Error("a known one-word name should stay a candidate, so approve can ask about it")
	}
}

// A one-word name in a header is caught; common headers are not.
func TestHeaderNamed(t *testing.T) {
	for _, h := range []string{"Oksana", "Kavya", "kavya", "Day Oksana", "Shopify"} {
		if !headerNamed(h) {
			t.Errorf("%q should look like a name", h)
		}
	}
	for _, h := range []string{"Day", "Amount", "Customer Email", "SKU", "Created (UTC)", "Montant", "Courriel",
		"items[].name", "address.city", "customerId", "Mon", "Lundi", "Clr", "Grace"} {
		if headerNamed(h) {
			t.Errorf("%q should not look like a name", h)
		}
	}
}

func TestContextMasksOtherCandidates(t *testing.T) {
	f := find("My partner Oksana Drozdenko will bring the Shimano bike to Tavish.")
	c := f.Get("shimano")
	ctx := f.Context(c)
	for _, w := range []string{"Oksana", "Drozdenko", "Tavish"} {
		if strings.Contains(ctx, w) {
			t.Errorf("context shows another candidate %q: %s", w, ctx)
		}
	}
	if !strings.Contains(ctx, "[Shimano]") || !strings.Contains(ctx, Mask) {
		t.Errorf("context: %s", ctx)
	}
}

func TestChoicesRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "words.json")
	ch := NewChoices()
	ch.Keep["shimano"], ch.Swap["oksana"] = true, true
	if err := os.WriteFile(p, ch.Marshal(), 0o600); err != nil {
		t.Fatal(err)
	}
	got, raw, exists, err := LoadChoices(p)
	if err != nil || !exists || !got.Keep["shimano"] || !got.Swap["oksana"] || len(raw) == 0 {
		t.Fatalf("%v %v %+v", err, exists, got)
	}
	os.WriteFile(p, []byte(`{"version":1,"note":"","keep":[],"swap":[],"groups":["calendar"]}`), 0o600)
	if _, _, _, err := LoadChoices(p); err == nil {
		t.Error("an unknown key should be refused")
	}
}

// A swapped word that is also an English word is not a known value, so a CSV
// scan, check or hook does not fire on "may" or "supply".
func TestCommonNameWordsAreNotKnown(t *testing.T) {
	m := mapping.New()
	m.Get(Category, "May")
	m.Get(Category, "Oksana")
	got, _ := scan.Replace("may I pay Oksana in May?", scan.NewKnown(m), m)
	if got != "may I pay [NAME_2] in May?" {
		t.Errorf("got %q", got)
	}
}

// A long encoded run is one candidate, not several meaningless ones: a
// tracking id used to arrive as "AQH", "JzLm" and "Pq", three decisions for
// one id (decision 72).
func TestEncodedRunIsOneCandidate(t *testing.T) {
	f := NewFinder(nil)
	f.AddLines([]string{"https://mail.example.org/t/c/AQH3k9JzLm2Pq7Rt?utm_source=newsletter"})
	c := f.Get(Key("AQH3k9JzLm2Pq7Rt"))
	if c == nil || c.Reasons&MachineText == 0 {
		t.Fatalf("the id should be one candidate, got %v", f.Candidates())
	}
	for _, part := range []string{"AQH", "JzLm", "Pq"} {
		if f.Get(Key(part)) != nil {
			t.Errorf("%q should not be a candidate of its own", part)
		}
	}
}

// Ordinary text is not an encoded run: a path, a slug and a query part keep
// their words, and a long word without a digit stays a word.
func TestOrdinaryTextIsNotEncoded(t *testing.T) {
	for _, c := range []struct{ line, word string }{
		{"see /people/oksana-vantreeck", "oksana"},
		{"the spring-2026-newsletter is out", "newsletter"},
		{"?utm_campaign=spring&utm_medium=email", "campaign"},
		{"Donaudampfschifffahrtsgesellschaft", "Donaudampfschifffahrtsgesellschaft"},
	} {
		if got := encodedRuns(c.line); len(got) != 0 {
			t.Errorf("%q: runs = %v, want none", c.line, got)
		}
		if !strings.Contains(c.line, c.word) {
			t.Fatalf("bad case: %q lacks %q", c.line, c.word)
		}
	}
}

func TestTokensKeepWordsAroundARun(t *testing.T) {
	line := "id AQH3k9JzLm2Pq7Rt then more words"
	var got []string
	for _, sp := range Tokens(line) {
		got = append(got, line[sp[0]:sp[1]])
	}
	want := []string{"id", "AQH3k9JzLm2Pq7Rt", "then", "more", "words"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("tokens = %q, want %q", got, want)
	}
}
