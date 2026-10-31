package dataset

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
)

// Invented values only. A config path must identify exactly one structural field.
func TestJSONReservedFieldNamesRefused(t *testing.T) {
	for _, key := range []string{"meta.city", "items[]", "[field]", "(key)", "", "   "} {
		t.Run(key, func(t *testing.T) {
			quoted, _ := json.Marshal(key)
			for _, suffix := range []string{`"Zelda Quorn"`, `{}`, `[]`} {
				input := `[{` + string(quoted) + `:` + suffix + `,"meta":{"city":"public"}}]`
				d, err := Parse(JSON, []byte(input))
				if d != nil || fail.Code(err) != "json-key" || strings.Contains(err.Error(), "Zelda") {
					t.Fatalf("input %s: got dataset=%v, err=%v", input, d, err)
				}
			}
		})
	}
	// Record names are values, not config paths: an email may contain dots.
	if _, err := Parse(JSON, []byte(`{"zelda.quorn@example.com":{"city":"public"},"(key)":{"city":"public"}}`)); err != nil {
		t.Fatalf("record keys must stay supported as data: %v", err)
	}
}

func TestJSONEveryKeyValidated(t *testing.T) {
	for _, value := range []string{`{}`, `[]`, `{"safe":"value"}`} {
		for _, key := range []string{`[PERSON_1]`, `prefix [NAME_3] suffix`} {
			quoted, _ := json.Marshal(key)
			for _, input := range []string{
				`[{` + string(quoted) + `:` + value + `}]`,
				`[{"meta":{` + string(quoted) + `:` + value + `}}]`,
				`{` + string(quoted) + `:{"safe":` + value + `}}`,
			} {
				d, err := Parse(JSON, []byte(input))
				if d != nil || fail.Code(err) != "standin-in-input" {
					t.Errorf("%s: got dataset=%v, err=%v", input, d, err)
				}
			}
		}
		input := `[{"Zel\u200bda":` + value + `}]`
		d, err := Parse(JSON, []byte(input))
		if d != nil || fail.Code(err) != "hidden-char" || strings.Contains(err.Error(), "Zel") {
			t.Errorf("%s: got dataset=%v, err=%v", input, d, err)
		}
	}
}

func TestJSONBrokenUnicodeRefused(t *testing.T) {
	for _, input := range []string{
		`[{"name":"Zel\ud800da"}]`,
		`[{"Zel\udc00da":{}}]`,
	} {
		if d, err := Parse(JSON, []byte(input)); d != nil || fail.Code(err) != "bad-json" {
			t.Errorf("%s: got dataset=%v, err=%v", input, d, err)
		}
	}
	if _, err := Parse(JSON, []byte(`[{"symbol":"\ud83d\ude00"}]`)); err != nil {
		t.Fatalf("valid surrogate pair refused: %v", err)
	}
}

func TestJSONEmptyContainerNamesInInventory(t *testing.T) {
	const input = `[{"EmptyObject":{},"EmptyArray":[],"Group":{"Child":{}},"Rows":[{}]}]`
	d, err := Parse(JSON, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(d.Columns, ","), "EmptyObject,EmptyArray[],Group.Child,Rows[]"; got != want {
		t.Fatalf("inventory=%s, want %s", got, want)
	}
	out, err := d.Write(Rewriter{Cell: same, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	var before, after any
	if err := json.Unmarshal([]byte(input), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &after); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatalf("empty containers changed: %s", out)
	}
}

func TestDuplicateHeadersRefused(t *testing.T) {
	for _, tc := range []struct{ kind, input string }{
		{CSV, "Status,Status\npaid,Zelda Quorn\n"},
		{TSV, "Status\tStatus\npaid\tZelda Quorn\n"},
		{CSV, ",\na,b\n"},
		{Text, "| Status | Status |\n|---|---|\n|paid|Zelda Quorn|\n"},
	} {
		d, err := Parse(tc.kind, []byte(tc.input))
		if d != nil || fail.Code(err) != "duplicate-header" {
			t.Fatalf("%s: got dataset=%v, err=%v", tc.kind, d, err)
		}
		if strings.Contains(err.Error(), "Status") || strings.Contains(err.Error(), "Zelda") {
			t.Fatalf("error repeats private data: %v", err)
		}
	}
	// Equal names in separate tables intentionally share the same table rule.
	const separate = "| Status |\n|---|\n|paid|\n\n| Status |\n|---|\n|paid|\n"
	if _, err := Parse(Text, []byte(separate)); err != nil {
		t.Fatalf("same header in separate tables should work: %v", err)
	}
}

func TestRewrittenHeaderCollisionRefused(t *testing.T) {
	for _, tc := range []struct{ kind, input string }{
		{CSV, "Zelda Quorn,zelda quorn\n1,2\n"},
		{TSV, "Zelda Quorn\tzelda quorn\n1\t2\n"},
		{Text, "| Zelda Quorn | zelda quorn |\n|---|---|\n|1|2|\n"},
	} {
		d, err := Parse(tc.kind, []byte(tc.input))
		if err != nil {
			t.Fatalf("%s: distinct input headers refused: %v", tc.kind, err)
		}
		m := mapping.New()
		out, err := d.Write(Rewriter{Cell: same, Name: func(s string) string { return m.Get("PERSON", s) }})
		if out != nil || fail.Code(err) != "duplicate-header" {
			t.Fatalf("%s: got output %q, err=%v", tc.kind, out, err)
		}
		if strings.Contains(err.Error(), "Zelda") || strings.Contains(err.Error(), "Quorn") {
			t.Fatalf("%s: collision message holds a private name: %v", tc.kind, err)
		}
	}
	// Separate tables intentionally share the configured rule, even after rewrite.
	const separate = "| Zelda Quorn |\n|---|\n|1|\n\n| zelda quorn |\n|---|\n|2|\n"
	d, err := Parse(Text, []byte(separate))
	if err != nil {
		t.Fatal(err)
	}
	m := mapping.New()
	if out, err := d.Write(Rewriter{Cell: same, Name: func(s string) string { return m.Get("PERSON", s) }}); err != nil || len(out) == 0 {
		t.Fatalf("separate tables should work: output=%q, err=%v", out, err)
	}
}
