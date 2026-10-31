package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RKelln/standin/internal/fail"
)

func TestLeaseCloseFailureIsSafeAndLeavesLockBlocking(t *testing.T) {
	dir := canonicalTempDir(t)
	lease, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LockFile, "held"), []byte("invented"), 0600); err != nil {
		t.Fatal(err)
	}
	err = lease.Close()
	if err == nil || strings.Contains(err.Error(), dir) {
		t.Fatalf("release failure missing or contains path: %v", err)
	}
	if CheckReady(dir) == nil {
		t.Fatal("failed release stopped blocking readers")
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("second close should be harmless: %v", err)
	}
}

func TestRecordErrorReportsLeaseReleaseFailure(t *testing.T) {
	dir := canonicalTempDir(t)
	beforePublish = func(int) {
		if err := os.WriteFile(filepath.Join(dir, LockFile, "held"), []byte("invented"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforePublish = nil })
	err := RecordError(dir, "scrub", "ragged")
	if err == nil || !strings.Contains(err.Error(), "lock") || strings.Contains(err.Error(), dir) {
		t.Fatalf("logging did not report safe release error: %v", err)
	}
	if CheckReady(dir) == nil {
		t.Fatal("leftover logging lease stopped blocking readers")
	}
}

func TestRollbackCleanupFailureKeepsPendingMarker(t *testing.T) {
	dir := canonicalTempDir(t)
	file := filepath.Join(dir, MappingFile)
	if err := os.WriteFile(file, []byte("old invented mapping"), 0600); err != nil {
		t.Fatal(err)
	}
	beforePublish = func(int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".standin-tmp-") {
				path := filepath.Join(dir, entry.Name())
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "held"), []byte("invented"), 0600); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		t.Fatal("staged output was not present")
	}
	t.Cleanup(func() { beforePublish = nil })
	err := WriteAll([]Out{{Path: file, Data: []byte("new invented mapping"), Perm: 0600, Root: dir}})
	if err == nil || fail.Code(err) != "write-rollback" || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "invented mapping") {
		t.Fatalf("cleanup failure needs safe recovery guidance: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, PendingFile)); err != nil {
		t.Fatalf("unfinished save marker lost: %v", err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old invented mapping" {
		t.Fatalf("old mapping changed: %q %v", got, err)
	}
	if CheckReady(dir) == nil {
		t.Fatal("reader accepted failed cleanup generation")
	}
}

func TestRootAtomicFailedRenameRemovesOnlyTemp(t *testing.T) {
	dir := canonicalTempDir(t)
	r, err := openBoundary(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err := os.Mkdir(filepath.Join(dir, "occupied"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := rootAtomic(r, "occupied", []byte("invented"), 0600); err == nil {
		t.Fatal("rename onto a directory succeeded")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "occupied" {
		t.Fatalf("temp file left or destination removed: %v %v", entries, err)
	}
}
