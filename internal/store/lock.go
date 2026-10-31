package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/RKelln/standin/internal/fail"
)

const LockFile = ".standin-lock"
const PendingFile = "pending.json"

var ownedLocks = struct {
	sync.Mutex
	dirs map[string]bool
}{dirs: map[string]bool{}}

// Lease serializes private-state changes across processes. A crashed process
// leaves its lease behind; it is never guessed stale or taken from another run.
type Lease struct {
	root *os.Root
	dir  string
}

func Lock(dir string) (*Lease, error) { return lockFor(dir, 5*time.Second) }

func lockFor(dir string, wait time.Duration) (*Lease, error) {
	if err := Ensure(dir); err != nil {
		return nil, err
	}
	r, err := openBoundary(dir)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err = r.Mkdir(LockFile, 0700)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			_ = r.Close() // Read-only directory handle; preserve the lock error.
			return nil, fail.New("private-lock", "I could not lock the private folder safely.")
		}
		if time.Now().After(deadline) {
			_ = r.Close() // No lock was acquired and no file data was written.
			return nil, fail.New("private-busy", "Another Standin run is using the private folder, or a run stopped before it finished. Wait for it to finish. If no run is open, ask the person to check the private folder before removing .standin-lock.")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := securePath(filepath.Join(dir, LockFile), true); err != nil {
		removeErr := r.Remove(LockFile)
		_ = r.Close() // The removal result determines whether a stale lock remains.
		if removeErr != nil {
			return nil, fail.New("private-lock", "I could not secure or remove a new private-folder lock. Ask the person to check .standin-lock before trying again.")
		}
		return nil, err
	}
	ownedLocks.Lock()
	ownedLocks.dirs[dir] = true
	ownedLocks.Unlock()
	l := &Lease{root: r, dir: dir}
	if err := CheckReady(dir); err != nil {
		return nil, withCleanup(err, l.Close())
	}
	return l, nil
}

func (l *Lease) Close() error {
	if l == nil || l.root == nil {
		return nil
	}
	ownedLocks.Lock()
	delete(ownedLocks.dirs, l.dir)
	ownedLocks.Unlock()
	err := l.root.Remove(LockFile)
	cerr := l.root.Close()
	l.root = nil
	if err != nil {
		return fail.New("private-lock", "I could not remove the private-folder lock. The command may have saved files. Ask the person to check the outputs and .standin-lock before trying again.")
	}
	if cerr != nil {
		return fail.New("private-lock", "I removed the private-folder lock, but could not close its folder handle. The command may have saved files. Check the outputs before trying again.")
	}
	return nil
}

// CheckReady refuses an unfinished save, or another process's active/stale
// lease. Readers must not use a partly published generation of private state.
func CheckReady(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, PendingFile)); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return fail.New("private-pending", "The last save did not finish. I cannot safely use its mapping. Ask the person to restore the private folder and scrubbed files from the same backup before removing pending.json and .standin-lock. Previous private files are kept as .standin-previous-*.")
	}
	ownedLocks.Lock()
	own := ownedLocks.dirs[dir]
	ownedLocks.Unlock()
	if !own {
		if _, err := os.Lstat(filepath.Join(dir, LockFile)); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return fail.New("private-busy", "The private folder is in use, or its last run stopped early. Wait, or ask the person to check it before removing .standin-lock.")
		}
	}
	return nil
}
