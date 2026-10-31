package dataset

import (
	"encoding/json"
	"strings"
	"testing"
)

func same(_ int, v string) (string, bool) { return v, false }
func name(n string) string                { return n }

func TestCSV(t *testing.T) {
	d, err := Parse(CSV, []byte("\xEF\xBB\xBFName,Note\nAnn Fake,\"has, comma\"\nBo Fake,\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Columns[0] != "Name" || d.Rows != 2 || d.Values[1][0] != "has, comma" {
		t.Fatalf("got %+v", d)
	}
	if d.Where(0, 1) != "row 2" {
		t.Fatalf("Where = %s", d.Where(0, 1))
	}
	out, err := d.Write(Rewriter{Cell: func(c int, v string) (string, bool) {
		if c == 0 {
			return "X", true
		}
		return v, false
	}, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "Name,Note\nX,\"has, comma\"\nX,\n" {
		t.Fatalf("got %q", out)
	}
}

func TestTSV(t *testing.T) {
	d, err := Parse(TSV, []byte("A\tB\n1\t2\n"))
	if err != nil || len(d.Columns) != 2 {
		t.Fatalf("%v %v", d, err)
	}
}

func TestRaggedRefusedWithLine(t *testing.T) {
	_, err := Parse(CSV, []byte("A,B\n1,2\n3,4\n5,6,7\n"))
	if err == nil || !strings.Contains(err.Error(), "Line 4") {
		t.Fatalf("got %v", err)
	}
}

func TestBadQuoteRefused(t *testing.T) {
	if _, err := Parse(CSV, []byte("A,B\n1,x\"y\n")); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestUnsupportedRefused(t *testing.T) {
	for _, n := range []string{"a.pdf", "a.xlsx", "a.docx", "a.png", "a", "a.xml"} {
		if _, err := KindFor(n); err == nil {
			t.Errorf("%s: want a refusal", n)
		}
	}
	// A PDF or a zipped spreadsheet renamed to .csv is still refused.
	for _, b := range []string{"%PDF-1.4 fake", "PK\x03\x04 fake xlsx"} {
		if _, err := Parse(CSV, []byte(b)); err == nil {
			t.Errorf("%q: want a refusal", b[:4])
		}
	}
	if _, err := Parse(CSV, []byte("  \n")); err == nil {
		t.Error("empty: want a refusal")
	}
}

const records = `[
  {"id": "C-1", "name": "Ann Fake", "address": {"city": "Faketown"}, "tags": ["a", "b"],
   "items": [{"sku": "X1", "qty": 2}], "n": 1.50, "ok": true, "none": null},
  {"id": "C-2", "name": "Bo Fake", "extra": "only here"}
]`

func TestJSONArray(t *testing.T) {
	d, err := Parse(JSON, []byte(records))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(d.Columns, ",")
	want := "id,name,address.city,tags[],items[].sku,items[].qty,n,ok,none,extra"
	if got != want || d.Rows != 2 || d.JSONShape != "array of records" {
		t.Fatalf("columns %s", got)
	}
	out, err := d.Write(Rewriter{Cell: same, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal([]byte(records), &a)
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("unchanged write differs:\n%s\n%s", ja, jb)
	}
	if !strings.Contains(string(out), "1.50") {
		t.Fatal("number text not kept as written")
	}
}

func TestJSONObjectOfRecords(t *testing.T) {
	d, err := Parse(JSON, []byte(`{"Ann Fake": {"city": "X"}, "Bo Fake": {"city": "Y"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Columns[0] != KeyColumn || d.Values[0][1] != "Bo Fake" {
		t.Fatalf("got %v %v", d.Columns, d.Values)
	}
	out, err := d.Write(Rewriter{Cell: func(c int, v string) (string, bool) {
		if c == 0 {
			return "[KEY_" + v[:1] + "]", true
		}
		return v, false
	}, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"[KEY_A]"`) || strings.Contains(string(out), "Ann Fake") {
		t.Fatalf("got %s", out)
	}
	// Two keys that would get the same stand-in: refuse.
	if _, err := d.Write(Rewriter{Cell: func(c int, v string) (string, bool) { return "same", true }, Name: name}); err == nil {
		t.Fatal("want a refusal for clashing keys")
	}
}

func TestJSONRefusals(t *testing.T) {
	bad := []string{
		`"just a string"`,
		`[1, 2]`,
		`[{"a": [[1]]}]`,
		`[{"a": 1, "a": 2}]`,
		`[{"a": 1}] trailing`,
		`{"x": 1}`,
		`[{"a": 1}`,
		`[]`,
	}
	for _, b := range bad {
		if _, err := Parse(JSON, []byte(b)); err == nil {
			t.Errorf("%s: want a refusal", b)
		}
	}
}

func TestErrorsHoldNoValues(t *testing.T) {
	_, err := Parse(JSON, []byte(`[{"Secret Fakename": [["x"]]}]`))
	if err == nil || strings.Contains(err.Error(), "Secret") {
		t.Fatalf("got %v", err)
	}
}

func TestLoneCRRefused(t *testing.T) {
	if _, err := Parse(CSV, []byte("a,b\r1,2\r")); err == nil || !strings.Contains(err.Error(), "normal line endings") {
		t.Fatalf("got %v", err)
	}
	if _, err := Parse(CSV, []byte("a,b\r\n1,2\r\n")); err != nil {
		t.Fatalf("CRLF refused: %v", err)
	}
}
