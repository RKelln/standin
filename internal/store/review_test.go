package store

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// macOS commonly spells its temp root through /var -> /private/var. The
// production CLI canonicalizes roots before invoking the store API, so direct
// store tests must use the same real-path contract.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestReviewPrivateAliasCannotLeadIntoProject(t *testing.T) {
	dir := canonicalTempDir(t)
	project := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(project, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(filepath.Join(project, "cache"), alias); err != nil {
		t.Skip("no symlinks: ", err)
	}
	for _, tc := range []struct{ override, home string }{{alias, ""}, {"", alias}, {filepath.Join(alias, "not-yet"), ""}} {
		if _, err := PrivateDir(project, tc.override, tc.home); err == nil {
			t.Errorf("accepted private alias inside project: %+v", tc)
		}
	}
}

func TestReviewWriteBoundaryRejectsNestedLink(t *testing.T) {
	dir := canonicalTempDir(t)
	root := filepath.Join(dir, "restored")
	other := filepath.Join(dir, "scrubbed")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(root, "jobs")); err != nil {
		t.Skip("no symlinks: ", err)
	}
	err := WriteAll([]Out{{Path: filepath.Join(root, "jobs", "answer.md"), Data: []byte("Invented Secret"), Perm: 0600, Root: root}})
	if err == nil {
		t.Fatal("accepted nested link out of restored")
	}
	if _, err := os.Stat(filepath.Join(other, "answer.md")); !os.IsNotExist(err) {
		t.Fatal("wrote outside restored")
	}
	if err := WriteAll([]Out{{Path: filepath.Join(root, "plain", "answer.md"), Data: []byte("Invented Secret"), Perm: 0600, Root: root}}); err != nil {
		t.Fatal(err)
	}
}

func TestReviewWriteBoundaryRejectsRootLink(t *testing.T) {
	dir := canonicalTempDir(t)
	other := filepath.Join(dir, "scrubbed")
	alias := filepath.Join(dir, "restored")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Skip("no symlinks: ", err)
	}
	if err := WriteAll([]Out{{Path: filepath.Join(alias, "answer.md"), Data: []byte("Invented Secret"), Perm: 0600, Root: alias}}); err == nil {
		t.Fatal("accepted symlink boundary")
	}
}

func TestReviewPendingSaveBlocksReadersAndWriters(t *testing.T) {
	dir := canonicalTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, PendingFile), []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckReady(dir); err == nil {
		t.Fatal("reader ignored pending save")
	}
	if l, err := Lock(dir); err == nil {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("writer ignored pending save")
	}
}

func TestReviewPrivateSaveKeepsPreviousFile(t *testing.T) {
	dir := canonicalTempDir(t)
	p := filepath.Join(dir, MappingFile)
	if err := os.WriteFile(p, []byte("old invented mapping"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAll([]Out{{Path: p, Data: []byte("new invented mapping"), Perm: 0600, Root: dir}}); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(filepath.Join(dir, ".standin-previous-"+MappingFile))
	if err != nil || string(old) != "old invented mapping" {
		t.Fatalf("previous mapping unavailable: %q %v", old, err)
	}
	if err := CheckReady(dir); err != nil {
		t.Fatalf("finished save left marker: %v", err)
	}
}

func TestReviewLeaseAcrossProcesses(t *testing.T) {
	if os.Getenv("STANDIN_LOCK_TEST_CHILD") == "1" {
		dir := os.Getenv("STANDIN_LOCK_TEST_DIR")
		l, err := Lock(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := l.Close(); err != nil {
				t.Error(err)
			}
		}()
		if err := os.WriteFile(filepath.Join(dir, "child-held"), []byte("held"), 0600); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(filepath.Join(dir, "child-release")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("parent did not release child")
	}
	dir := canonicalTempDir(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestReviewLeaseAcrossProcesses$")
	cmd.Env = append(os.Environ(), "STANDIN_LOCK_TEST_CHILD=1", "STANDIN_LOCK_TEST_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			if err := os.WriteFile(filepath.Join(dir, "child-release"), nil, 0600); err != nil {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 500; i++ {
		if _, err := os.Stat(filepath.Join(dir, "child-held")); err == nil {
			break
		}
		if i == 499 {
			t.Fatal("child did not acquire lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := CheckReady(dir); err == nil {
		t.Fatal("reader ignored another process lease")
	}
	if l, err := lockFor(dir, 20*time.Millisecond); err == nil {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("second process acquired existing lease")
	}
	if err := os.WriteFile(filepath.Join(dir, "child-release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	waited = true
	l, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewAbandonedLeaseNeverStolen(t *testing.T) {
	dir := canonicalTempDir(t)
	if err := os.Mkdir(filepath.Join(dir, LockFile), 0700); err != nil {
		t.Fatal(err)
	}
	if l, err := lockFor(dir, 10*time.Millisecond); err == nil {
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("stole abandoned lease")
	}
	if err := CheckReady(dir); err == nil {
		t.Fatal("reader accepted abandoned lease")
	}
}

func TestReviewKilledPublicationLeavesRecoveryMarker(t *testing.T) {
	if os.Getenv("STANDIN_KILL_TEST_CHILD") == "1" {
		dir := os.Getenv("STANDIN_KILL_TEST_DIR")
		l, err := Lock(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := l.Close(); err != nil {
				t.Error(err)
			}
		}()
		beforePublish = func(int) {
			if err := os.WriteFile(filepath.Join(dir, "child-publishing"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			for {
				time.Sleep(time.Second)
			}
		}
		if err := WriteAll([]Out{{Path: filepath.Join(dir, MappingFile), Data: []byte("new invented mapping"), Perm: 0600, Root: dir}}); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := canonicalTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, MappingFile), []byte("old invented mapping"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestReviewKilledPublicationLeavesRecoveryMarker$")
	cmd.Env = append(os.Environ(), "STANDIN_KILL_TEST_CHILD=1", "STANDIN_KILL_TEST_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Error(err)
				}
			}
		}
	}()
	for i := 0; i < 500; i++ {
		if _, err := os.Stat(filepath.Join(dir, "child-publishing")); err == nil {
			break
		}
		if i == 499 {
			t.Fatal("child did not begin publication")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatal(err)
		}
	}
	waited = true
	if err := CheckReady(dir); err == nil {
		t.Fatal("reader accepted a killed publication")
	}
	if _, err := os.Stat(filepath.Join(dir, PendingFile)); err != nil {
		t.Fatal("missing recovery marker: ", err)
	}
	previous, err := os.ReadFile(filepath.Join(dir, ".standin-previous-"+MappingFile))
	if err != nil || string(previous) != "old invented mapping" {
		t.Fatalf("previous mapping lost: %q %v", previous, err)
	}
}

func TestReviewPublicationFailureRestoresOldFiles(t *testing.T) {
	dir := canonicalTempDir(t)
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("old a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("old b"), 0600); err != nil {
		t.Fatal(err)
	}
	beforePublish = func(i int) {
		if i == 1 {
			if err := os.Remove(b); err != nil {
				t.Errorf("remove target: %v", err)
			}
			if err := os.Mkdir(b, 0700); err != nil {
				t.Errorf("replace target with directory: %v", err)
			}
		}
	}
	defer func() { beforePublish = nil }()
	err := WriteAll([]Out{{Path: a, Data: []byte("new a"), Perm: 0600, Root: dir}, {Path: b, Data: []byte("new b"), Perm: 0600, Root: dir}})
	if err == nil {
		t.Fatal("expected publication failure")
	}
	got, err := os.ReadFile(a)
	if err != nil || string(got) != "old a" {
		t.Fatalf("old output not restored: %q %v", got, err)
	}
}

func TestReviewRollbackFailureLeavesMarker(t *testing.T) {
	dir := canonicalTempDir(t)
	a, b := filepath.Join(dir, MappingFile), filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("old invented mapping"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("old b"), 0600); err != nil {
		t.Fatal(err)
	}
	beforePublish = func(i int) {
		if i == 1 {
			if err := os.Remove(a); err != nil {
				t.Errorf("remove mapping: %v", err)
			}
			if err := os.Mkdir(a, 0700); err != nil {
				t.Errorf("replace mapping with directory: %v", err)
			}
			if err := os.Remove(b); err != nil {
				t.Errorf("remove target: %v", err)
			}
			if err := os.Mkdir(b, 0700); err != nil {
				t.Errorf("replace target with directory: %v", err)
			}
		}
	}
	defer func() { beforePublish = nil }()
	err := WriteAll([]Out{{Path: a, Data: []byte("new invented mapping"), Perm: 0600, Root: dir}, {Path: b, Data: []byte("new b"), Perm: 0600, Root: dir}})
	if err == nil {
		t.Fatal("expected failed rollback")
	}
	if err := CheckReady(dir); err == nil {
		t.Fatal("failed rollback did not block readers")
	}
}
