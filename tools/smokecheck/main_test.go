package main

import (
	"encoding/json"
	"testing"
)

func TestAuditRequiresRealMatchingHookEvidence(t *testing.T) {
	want := audit{Event: "PreToolUse", Tool: "Read", Subject: "raw/invoices.csv", Status: 0, Redirected: true}
	for _, fake := range []string{"", "authentication failed", `{"event":"PreToolUse","tool":"Read","subject":"standin.config.json","status":0,"redirected":true}`, `{"event":"PreToolUse","tool":"Read","subject":"raw/invoices.csv","status":1,"redirected":true}`} {
		if contains([]byte(fake), want) {
			t.Fatalf("unrelated failure or different tool passed: %q", fake)
		}
	}
	b, _ := json.Marshal(want)
	if !contains(b, want) {
		t.Fatal("matching event did not pass")
	}
}

func TestAuditChecksRedirectOutputAndExactCommand(t *testing.T) {
	input := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"/invented/demo/raw/invoices.csv"}}`)
	output := []byte(`{"hookSpecificOutput":{"updatedInput":{"file_path":"/invented/demo/scrubbed/invoices.csv"}}}`)
	got, err := record(input, output, 0)
	if err != nil || !got.Redirected || got.Subject != "raw/invoices.csv" {
		t.Fatalf("redirect: %+v, %v", got, err)
	}
	got, err = record(input, []byte("unrelated failure"), 1)
	if err != nil || got.Redirected || got.Status != 1 {
		t.Fatalf("failure: %+v, %v", got, err)
	}
	if subject("Bash", map[string]any{"command": "echo cat raw/invoices.csv | head -3"}) != "other" {
		t.Fatal("a different command must not prove the requested cat was blocked")
	}
	if _, err := record([]byte("authentication failed"), nil, 0); err == nil {
		t.Fatal("invalid hook input accepted")
	}
}
