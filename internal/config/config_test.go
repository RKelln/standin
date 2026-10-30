package config

import (
	"strings"
	"testing"
)

const good = `{
  "version": 1,
  "files": [
    {"match": "raw/invoices*.csv", "columns": {
      "Customer": {"action": "tokenize", "category": "PERSON", "reason": "names"},
      "Amount": {"action": "keep", "reason": "needed"},
      "Notes": {"action": "scan"},
      "[COLUMN_5]": {"action": "drop"}
    }},
    {"match": "raw/**/*.json", "columns": {"(key)": {"action": "tokenize", "category": "KEY"}}}
  ]
}`

func TestParseGood(t *testing.T) {
	c, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Hash) != 64 {
		t.Fatalf("hash %q", c.Hash)
	}
	if got := strings.Join(c.Files[0].ColumnNames(), ","); got != "Customer,Amount,Notes,[COLUMN_5]" {
		t.Fatalf("order %s", got)
	}
	r := c.RuleFor("raw/invoices-2026.csv")
	if r == nil || r.Match != "raw/invoices*.csv" {
		t.Fatal("no rule for invoices")
	}
	col, ok, err := r.ColumnFor("Hidden header", 4)
	if err != nil || !ok || col.Action != Drop {
		t.Fatalf("[COLUMN_5] lookup: %v %v %v", col, ok, err)
	}
	if _, ok, _ := r.ColumnFor("Other", 1); ok {
		t.Fatal("unlisted column found")
	}
	if c.RuleFor("raw/a/b/c.json") == nil || c.RuleFor("raw/c.json") == nil {
		t.Fatal("** should match any folders, including none")
	}
	if c.RuleFor("scrubbed/invoices.csv") != nil {
		t.Fatal("matched outside raw/")
	}
}

func TestHashChangesWithAnyEdit(t *testing.T) {
	a, _ := Parse([]byte(good))
	b, _ := Parse([]byte(strings.Replace(good, `"names"`, `"full names"`, 1)))
	if a.Hash == b.Hash {
		t.Fatal("a config edit must change the hash (and so void the approval)")
	}
}

func TestRefusals(t *testing.T) {
	bad := map[string]string{
		"unknown action":     `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"hide"}}}]}`,
		"unknown category":   `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"tokenize","category":"NAME"}}}]}`,
		"no category":        `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"tokenize"}}}]}`,
		"category with keep": `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"keep","category":"PERSON"}}}]}`,
		"unknown key":        `{"version":1,"files":[{"match":"raw/*.csv","colums":{}}]}`,
		"unknown col key":    `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"keep","why":"x"}}}]}`,
		"no action":          `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{}}}]}`,
		"bad version":        `{"version":2,"files":[]}`,
		"no match":           `{"version":1,"files":[{"columns":{}}]}`,
		"bad glob":           `{"version":1,"files":[{"match":"raw/[.csv","columns":{}}]}`,
		"not json":           `{"version":1,"files":[`,
		"trailing":           `{"version":1,"files":[]} {}`,
		"dup column":         `{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"keep"},"A":{"action":"drop"}}}]}`,
		"key drop":           `{"version":1,"files":[{"match":"raw/*.json","columns":{"(key)":{"action":"drop"}}}]}`,
		"key category":       `{"version":1,"files":[{"match":"raw/*.json","columns":{"(key)":{"action":"tokenize","category":"PERSON"}}}]}`,
	}
	for name, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
}

func TestBothNameAndRefRefused(t *testing.T) {
	c, err := Parse([]byte(`{"version":1,"files":[{"match":"raw/*.csv","columns":{"A":{"action":"keep"},"[COLUMN_1]":{"action":"drop"}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Files[0].ColumnFor("A", 0); err == nil {
		t.Fatal("want a refusal when one column is named twice")
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		p, n string
		want bool
	}{
		{"raw/*.csv", "raw/a.csv", true},
		{"raw/*.csv", "raw/x/a.csv", false},
		{"raw/**", "raw/x/y/a.csv", true},
		{"raw/**/a.csv", "raw/a.csv", true},
		{"**/*.csv", "raw/x/a.csv", true},
		{"raw/?.csv", "raw/ab.csv", false},
	}
	for _, c := range cases {
		if got := Match(c.p, c.n); got != c.want {
			t.Errorf("Match(%q, %q) = %v", c.p, c.n, got)
		}
	}
}
