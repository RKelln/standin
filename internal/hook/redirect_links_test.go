package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedirectRefusesLinkedScrubbedTwin(t *testing.T) {
	for _, where := range []string{"file into raw", "file outside project", "nested folder", "scrubbed root", "safe target link"} {
		t.Run(where, func(t *testing.T) {
			f := setup(t)
			raw := "raw/fresh.csv"
			target := filepath.Join(f.project, "raw", "other.csv")
			link := filepath.Join(f.project, "scrubbed", "fresh.csv")
			write(t, target, "Customer\nZelda Quixote\n")
			switch where {
			case "file outside project":
				target = filepath.Join(t.TempDir(), "other.csv")
				write(t, target, "Customer\nZelda Quixote\n")
			case "nested folder":
				raw = "raw/sub/deep.json"
				link = filepath.Join(f.project, "scrubbed", "sub")
				target = filepath.Join(f.project, "raw", "sub")
			case "scrubbed root":
				link = filepath.Join(f.project, "scrubbed")
				target = filepath.Join(f.project, "raw")
			case "safe target link":
				target = filepath.Join(f.project, "notes", "safe.csv")
				write(t, target, "Customer\n[PERSON_1]\n")
			}
			must(t, os.Chtimes(target, time.Now(), time.Now()))
			must(t, os.RemoveAll(link))
			if err := os.Symlink(target, link); err != nil {
				t.Skip("symlinks unavailable: ", err)
			}
			r := run([]string{"claude", "PreToolUse", "--project", f.project}, hookJSON("PreToolUse", f.project, "Read", map[string]any{"file_path": raw}, nil))
			if r.code != 2 || r.stdout != "" {
				t.Fatalf("unsafe redirect was not refused: %+v", r)
			}
			for _, secret := range secretValues {
				if contains := strings.Contains(r.stderr, secret); contains {
					t.Fatal("refusal leaked a private value")
				}
			}
		})
	}
}
