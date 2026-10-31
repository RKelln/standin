package restore

import (
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
)

func TestRestoreDoesNotCreateActiveContent(t *testing.T) {
	for _, tc := range []struct {
		name, value, input string
		mode               Mode
	}{
		{"formula", "=1+1", "Note\n[VALUE_1]\n", CSV},
		{"formula after whitespace", "  +SUM(1)", "Note\n[VALUE_1]\n", CSV},
		{"tab formula", "-2", "Note\n[VALUE_1]\n", TSV},
		{"new link", "https://example.invalid/private", "Dear [VALUE_1]", Text},
		{"joined link", "ps://example.invalid/private", "htt[VALUE_1]", Text},
		{"new tag", "<img src='https://example.invalid/private'>", "Dear [VALUE_1]", Text},
		{"tag attribute", "private", "<img title='[VALUE_1]'>", Text},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mapping.New()
			m.Get("VALUE", tc.value)
			r, err := Restore(tc.input, m, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(r.Out, "[VALUE_1]") || r.Replaced != 0 || len(r.Blocked) != 1 || !r.Issues() {
				t.Fatalf("unsafe substitution was not held back: %+v", r)
			}
		})
	}
	// A formula can be made by two safe-looking values together.
	m := mapping.New()
	m.Get("VALUE", "=")
	m.Get("VALUE", "1+1")
	r, err := Restore("Note\n[VALUE_1][VALUE_2]\n", m, CSV)
	if err != nil || r.Replaced != 0 || len(r.Blocked) != 2 {
		t.Fatalf("joined formula: %+v %v", r, err)
	}
}

func TestRestoreRejectsMalformedStructuredInput(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Zelda Quorn")
	for _, tc := range []struct {
		name, input string
		mode        Mode
	}{
		{"unfinished JSON", `{"who":"[PERSON_1]"`, JSON},
		{"duplicate JSON key", `{"who":"[PERSON_1]","who":0}`, JSON},
		{"escaped duplicate JSON key", `{"who":"[PERSON_1]","\u0077ho":0}`, JSON},
		{"JSON trailing value", `{"who":"[PERSON_1]"} {}`, JSON},
		{"broken JSON surrogate", `{"who":"\ud800[PERSON_1]"}`, JSON},
		{"unclosed CSV quote", "Who\n\"[PERSON_1]\n", CSV},
		{"ragged CSV", "Who,Status\n[PERSON_1]\n", CSV},
		{"bare quote CSV", "Who\n[PERSON_1]\"\n", CSV},
		{"old line endings CSV", "Who\r[PERSON_1]\r", CSV},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Restore(tc.input, m, tc.mode)
			if err == nil || r.Out != "" {
				t.Fatalf("wrote after invalid input: %+v, %v", r, err)
			}
			if strings.Contains(err.Error(), "Zelda") {
				t.Fatal("error exposed a value")
			}
		})
	}
}

func TestRestoreRejectsDuplicateResultKeys(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Zelda Quorn")
	m.Get("ORG", "Zelda Quorn")
	r, err := Restore(`{"[PERSON_1]":1,"[ORG_1]":2}`, m, JSON)
	if err == nil || r.Out != "" {
		t.Fatalf("duplicate output keys accepted: %+v %v", r, err)
	}
}

func TestRestoreRefusesActiveContainerBodies(t *testing.T) {
	m := mapping.New()
	m.Get("PERSON", "Zelda Quorn")
	for _, text := range []string{
		`<script>send("[PERSON_1]")</script>`,
		`<ScRiPt>send("[PERSON_1]")`,
		`<style>body { background: url("https://example.invalid/[PERSON_1]") }</style>`,
		`<style>body:after { content: "[PERSON_1]";`,
		`<iframe srcdoc="[PERSON_1]">`,
		`<svg><text>[PERSON_1]</text></svg>`,
	} {
		r, err := Restore(text, m, Text)
		if err == nil || r.Out != "" {
			t.Fatalf("restored into active content: %+v %v", r, err)
		}
		if strings.Contains(err.Error(), "Zelda") {
			t.Fatal("error exposed a value")
		}
	}
}
