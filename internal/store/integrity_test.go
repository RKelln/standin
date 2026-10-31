package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewApprovalIntegrity(t *testing.T) {
	d := t.TempDir()
	good := string(MarshalApproval(Approval{Version: 1, ConfigSHA256: strings.Repeat("a", 64)}))
	for _, bad := range []string{good + `{}`, strings.Replace(good, `"version": 1`, `"version": 1, "version": 1`, 1), strings.Replace(good, `"version": 1`, `"version": 1, "extra": true`, 1), strings.Replace(good, strings.Repeat("a", 64), "not-a-hash", 1)} {
		if err := os.WriteFile(filepath.Join(d, ApprovalFile), []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadApproval(d); err == nil {
			t.Fatal("damaged approval accepted")
		}
	}
}

func TestReviewErrorLogCannotExportArbitraryText(t *testing.T) {
	d := t.TempDir()
	for _, log := range []string{
		`[{"time":"2026-10-31T12:00:00Z","command":"Zelda Quorn","code":"ragged"}]`,
		`[{"time":"Zelda Quorn","command":"scrub","code":"ragged"}]`,
		`[{"time":"2026-10-31T12:00:00Z","command":"scrub","code":"Zelda Quorn"}]`,
	} {
		os.WriteFile(filepath.Join(d, ErrorsFile), []byte(log), 0600)
		if len(LoadErrors(d)) != 0 {
			t.Fatal("private log text exposed to report")
		}
	}
}
