package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateDirOutsideProject(t *testing.T) {
	base := canonicalTempDir(t)
	proj := filepath.Join(base, "shop")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := PrivateDir(proj, "", filepath.Join(base, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(d) != filepath.Join(base, "home", "projects") || filepath.Base(d)[:5] != "shop-" || len(filepath.Base(d)) != 13 {
		t.Fatalf("got %s", d)
	}
	if _, err := PrivateDir(proj, filepath.Join(proj, "secret"), ""); err == nil {
		t.Fatal("--private inside the project: want a refusal")
	}
	if _, err := PrivateDir(proj, "", filepath.Join(proj, ".standin")); err == nil {
		t.Fatal("STANDIN_HOME inside the project: want a refusal")
	}
}

func TestEnsurePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix modes")
	}
	d := filepath.Join(canonicalTempDir(t), "a", "b")
	if err := Ensure(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(d); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(d)
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	p := filepath.Join(d, "m.json")
	if err := WriteAll([]Out{{Path: p, Data: []byte("x"), Perm: 0o600}}); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %o", st.Mode().Perm())
	}
}

func TestWriteAllNoneOnFailure(t *testing.T) {
	dir := canonicalTempDir(t)
	good := filepath.Join(dir, "good.csv")
	if err := os.WriteFile(good, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A path under a file cannot be made: the whole write must fail and change nothing.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	err := WriteAll([]Out{
		{Path: good, Data: []byte("new"), Perm: 0o644},
		{Path: filepath.Join(blocker, "x.csv"), Data: []byte("y"), Perm: 0o644},
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if b, _ := os.ReadFile(good); string(b) != "old" {
		t.Fatalf("good.csv changed to %q", b)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "good.csv" && e.Name() != "blocker" {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

func TestWriteAllRejectsDuplicateDestinationsBeforeCreatingFolders(t *testing.T) {
	for _, pair := range [][2]string{
		{"answer.md", "answer.md"},
		{"answer.md", "Answer.md"},
		{"notes/answer.md", "NOTES/answer.md"},
		{"notes/../answer.md", "answer.md"},
		{"sigma/Σ.txt", "sigma/ς.txt"},
	} {
		t.Run(pair[0]+"_"+pair[1], func(t *testing.T) {
			dir := filepath.Join(canonicalTempDir(t), "new-output")
			err := WriteAll([]Out{
				{Path: filepath.Join(dir, pair[0]), Data: []byte("first"), Perm: 0644, Root: dir},
				{Path: filepath.Join(dir, pair[1]), Data: []byte("second"), Perm: 0644, Root: dir},
			})
			if err == nil {
				t.Fatal("duplicate destination accepted")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("made output folder before duplicate check")
			}
		})
	}
}

func TestWriteAllDuplicateLeavesExistingFileUnchanged(t *testing.T) {
	dir := canonicalTempDir(t)
	file := filepath.Join(dir, "answer.md")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	err := WriteAll([]Out{{Path: file, Data: []byte("first"), Perm: 0644}, {Path: file, Data: []byte("second"), Perm: 0644}})
	if err == nil {
		t.Fatal("duplicate destination accepted")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("old output changed: %q %v", got, err)
	}
}

func TestRenameFailureRollsBack(t *testing.T) {
	dir := canonicalTempDir(t)
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("old a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The second target is a folder, so the rename onto it fails after a is renamed.
	b := filepath.Join(dir, "b")
	if err := os.MkdirAll(filepath.Join(b, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := WriteAll([]Out{{Path: a, Data: []byte("new a"), Perm: 0o644}, {Path: b, Data: []byte("b"), Perm: 0o644}})
	if err == nil {
		t.Fatal("want an error")
	}
	if got, _ := os.ReadFile(a); string(got) != "old a" {
		t.Fatalf("a not rolled back: %q", got)
	}
}

func TestErrorsRecorded(t *testing.T) {
	d := canonicalTempDir(t)
	for i := 0; i < 12; i++ {
		RecordError(d, "scrub", "ragged")
	}
	if es := LoadErrors(d); len(es) != 10 || es[0].Code != "ragged" {
		t.Fatalf("got %v", es)
	}
	RecordError(filepath.Join(d, "missing"), "scrub", "x") // no folder: no file, no panic
}

func TestPrivateDirFollowsSymlinks(t *testing.T) {
	base := canonicalTempDir(t)
	real := filepath.Join(base, "shop")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here")
	}
	a, _ := PrivateDir(real, "", filepath.Join(base, "home"))
	b, _ := PrivateDir(link, "", filepath.Join(base, "home"))
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}
