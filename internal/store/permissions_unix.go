//go:build !windows

package store

import (
	"errors"
	"os"
)

func securePath(path string, dir bool) error {
	mode := os.FileMode(0600)
	if dir {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

func secureFile(_ string, f *os.File) error { return f.Chmod(0600) }

func syncRoot(r *os.Root, dirs ...string) error {
	dir := "."
	if len(dirs) > 0 {
		dir = dirs[0]
	}
	f, err := r.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
