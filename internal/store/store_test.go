package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateDirOutsideProject(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "shop")
	os.MkdirAll(proj, 0o755)
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
	d := filepath.Join(t.TempDir(), "a", "b")
	if err := Ensure(d); err != nil {
		t.Fatal(err)
	}
	os.Chmod(d, 0o755)
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
	dir := t.TempDir()
	good := filepath.Join(dir, "good.csv")
	os.WriteFile(good, []byte("old"), 0o644)
	// A path under a file cannot be made: the whole write must fail and change nothing.
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte(""), 0o644)
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

func TestRenameFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("old a"), 0o644)
	// The second target is a folder, so the rename onto it fails after a is renamed.
	b := filepath.Join(dir, "b")
	os.MkdirAll(filepath.Join(b, "inside"), 0o755)
	err := WriteAll([]Out{{Path: a, Data: []byte("new a"), Perm: 0o644}, {Path: b, Data: []byte("b"), Perm: 0o644}})
	if err == nil {
		t.Fatal("want an error")
	}
	if got, _ := os.ReadFile(a); string(got) != "old a" {
		t.Fatalf("a not rolled back: %q", got)
	}
}

func TestErrorsRecorded(t *testing.T) {
	d := t.TempDir()
	for i := 0; i < 12; i++ {
		RecordError(d, "scrub", "ragged")
	}
	if es := LoadErrors(d); len(es) != 10 || es[0].Code != "ragged" {
		t.Fatalf("got %v", es)
	}
	RecordError(filepath.Join(d, "missing"), "scrub", "x") // no folder: no file, no panic
}

func TestPrivateDirFollowsSymlinks(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "shop")
	os.MkdirAll(real, 0o755)
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
