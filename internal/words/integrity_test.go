package words

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewChoicesIntegrity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "words.json")
	for _, damaged := range []string{
		`{"version":1,"keep":[],"swap":[]} {}`,
		`{"version":1,"version":1,"keep":[],"swap":[]}`,
		`{"version":1,"keep":[],"swap":["zelda","zelda"]}`,
		`{"version":1,"keep":[""],"swap":[]}`,
		`{"version":1,"keep":["\ud800"],"swap":[]}`,
	} {
		if err := os.WriteFile(p, []byte(damaged), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := LoadChoices(p); err == nil {
			t.Fatal("damaged choices accepted")
		}
	}
}
