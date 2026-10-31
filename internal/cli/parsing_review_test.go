package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RKelln/standin/internal/store"
)

func TestReviewAmbiguousRawFilesWriteNoOutputs(t *testing.T) {
	for _, tc := range []struct{ file, data string }{
		{"records.json", `[{"meta.city":"public","meta":{"city":"Zelda Quorn"}}]`},
		{"records.json", `[{"Customer":"Zelda Quorn","[PERSON_1]":{}}]`},
		{"inv.csv", "Status,Status\npaid,Zelda Quorn\n"},
		{"notes.md", "| Status | Status |\n|---|---|\n|paid|Zelda Quorn|\n"},
	} {
		t.Run(tc.file+tc.data[:1], func(t *testing.T) {
			for _, command := range []string{"shape", "scrub"} {
				p := newTP(t)
				p.write("raw/"+tc.file, tc.data)
				p.write("standin.config.json", `{"version":1,"files":[]}`)
				r := p.run("", false, command)
				if r.code != 2 || p.exists("shape") || p.exists("scrubbed") {
					t.Fatalf("%s wrote ambiguous input: %+v", command, r)
				}
				priv, err := store.PrivateDir(p.dir, "", p.home)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(priv, store.MappingFile)); !os.IsNotExist(err) {
					t.Fatal("refused input created a mapping")
				}
			}
		})
	}
}
