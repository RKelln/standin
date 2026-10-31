package shape

import (
	"regexp"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/dataset"
)

var fixture = "Customer,Email,Amount,Joined,Amara Fakeokafor,Notes\n" +
	"Ann Fakelee,ann.fakelee@example.com,1399.70,2026-01-05,12.00,Called about the late invoice and they will pay\n" +
	"Bo Fakeson,bo.fakeson@example.org,25.00,2026-02-11,3.50,\n" +
	"Cy Fakewood,cy.fakewood@example.net,7.25,2026-03-02,1.00,Net 30\n"

var values = []string{"Ann Fakelee", "ann.fakelee@example.com", "1399.70", "2026-01-05", "Bo Fakeson",
	"bo.fakeson@example.org", "25.00", "Cy Fakewood", "7.25", "Amara Fakeokafor", "Called about", "Net 30"}

func TestShapeHasNoValuesAndNoSmallCounts(t *testing.T) {
	d, err := dataset.Parse(dataset.CSV, []byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	md := Markdown(d, "raw/fixture.csv", "test")
	for _, v := range values {
		if strings.Contains(strings.ToLower(md), strings.ToLower(v)) {
			t.Errorf("shape file holds the value %q", v)
		}
	}
	// No exact count or length below 5 outside headings, column numbers and the help text.
	end := strings.Index(md, "## How to write the config")
	if end < 0 {
		t.Fatal("shape file is missing the config instructions")
	}
	body := md[:end]
	small := regexp.MustCompile(`(?m)^- (Rows|Records|Empty|Different values|Length)[^\n]*\b[1-4]\b`)
	for _, line := range strings.Split(body, "\n") {
		if small.MatchString(line) && !strings.Contains(line, "under 5") && !regexp.MustCompile(`1-5|6-10|11-20|21-40|41-80`).MatchString(line) {
			t.Errorf("small exact number: %q", line)
		}
	}
	if !strings.Contains(md, "Rows: under 5") {
		t.Error("3 rows should show as under 5")
	}
	if !strings.Contains(md, "[COLUMN_5]") {
		t.Error("a name as a header should be hidden as [COLUMN_5]")
	}
	for _, want := range []string{"two capitalised words", "email-like", "amount 2 decimals", "date YYYY-MM-DD"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing shape string %q", want)
		}
	}
}

func TestHiddenFileName(t *testing.T) {
	d, _ := dataset.Parse(dataset.CSV, []byte(fixture))
	md := Markdown(d, "", "test")
	if !strings.Contains(md, "file name looks like private data") {
		t.Fatal("no rename note")
	}
}

func TestDescribe(t *testing.T) {
	cases := map[string]string{
		"fake@example.com": "email-like",
		"2026-01-05":       "date YYYY-MM-DD",
		"01/05/2026":       "date NN/NN/YYYY",
		"1,399.70":         "amount 2 decimals",
		"$12":              "amount, no decimals",
		"416-555-0199":     "phone-like",
		"12345":            "digits",
		"M5V 2T6":          "postal-code-like",
		"Ann Fakelee":      "two capitalised words",
		"Zoë Fakemann":     "two capitalised words",
		"INV-1001":         "letters and digits",
		"yes":              "yes/no",
		"this is a long note with many words in it": "free text",
	}
	for in, want := range cases {
		if got := Describe(in); got != want {
			t.Errorf("Describe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBands(t *testing.T) {
	if CountBand(0) != "none" || CountBand(4) != "under 5" || CountBand(5) != "5-20" || CountBand(101) != "over 100" {
		t.Fatal("CountBand")
	}
	if LengthBand(5) != "1-5" || LengthBand(81) != "over 80" {
		t.Fatal("LengthBand")
	}
}

func TestJSONPathLabels(t *testing.T) {
	l, flag := Label("address.city", 0)
	if l != "address.city" || flag != "" {
		t.Fatal("plain path hidden")
	}
	l, flag = Label("contacts.Ann Fakelee.phone", 3)
	if l != "[COLUMN_4]" || flag != "name" {
		t.Fatalf("data-like key not hidden: %s %s", l, flag)
	}
}
