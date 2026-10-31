package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/store"
)

func TestReviewConcurrentScrubsKeepEveryMapping(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	for i := 0; i < 16; i++ {
		p.write(fmt.Sprintf("raw/file%d.csv", i), fmt.Sprintf("Customer\nInvented Customer %d\n", i))
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := p.run("", false, "scrub", fmt.Sprintf("raw/file%d.csv", i))
			if r.code == 2 {
				t.Errorf("scrub failed: %+v", r)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	m, _, err := mapping.Load(filepath.Join(p.privDir(), store.MappingFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 16 {
		t.Fatalf("got %d mapping entries, want 16", len(m.Entries))
	}
	for i := 0; i < 16; i++ {
		file := fmt.Sprintf("scrubbed/file%d.csv", i)
		if r := p.run("", false, "restore", file); r.code == 2 {
			t.Fatal(r)
		}
		want := fmt.Sprintf("Customer\nInvented Customer %d\n", i)
		if got := p.read(fmt.Sprintf("restored/file%d.csv", i)); got != want {
			t.Errorf("file %d restored wrong person: %q", i, got)
		}
	}
}

func TestReviewMissingMappingDoesNotReuseIDs(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.write("raw/a.csv", "Customer\nZelda Quorn\n")
	if r := p.run("", false, "scrub"); r.code == 2 {
		t.Fatal(r)
	}
	if err := os.Remove(filepath.Join(p.privDir(), store.MappingFile)); err != nil {
		t.Fatal(err)
	}
	p.write("raw/a.csv", "Customer\nOksana Fairweather\n")
	if r := p.run("", false, "scrub"); r.code != 2 {
		t.Fatalf("missing mapping recreated: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(p.privDir(), store.MappingFile)); !os.IsNotExist(err) {
		t.Fatal("made replacement mapping")
	}
}

func TestReviewRestoreRejectsNestedOutputLink(t *testing.T) {
	p := newTP(t)
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	p.write("raw/a.csv", "Customer\nZelda Quorn\n")
	p.run("", false, "scrub")
	p.write("scrubbed/jobs/answer.md", "Dear [VALUE_1]")
	if err := os.Mkdir(filepath.Join(p.dir, "restored"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.dir, "scrubbed"), filepath.Join(p.dir, "restored", "jobs")); err != nil {
		t.Skip("no symlinks: ", err)
	}
	if r := p.run("", false, "restore", "scrubbed/jobs/answer.md"); r.code != 2 {
		t.Fatalf("restored through link: %+v", r)
	}
	if p.exists("scrubbed/answer.md") {
		t.Fatal("real name written into scrubbed")
	}
}
