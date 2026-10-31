package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/RKelln/standin/internal/fail"
)

// canonical resolves the longest existing ancestor, including a private
// folder that has not been made yet. A broken link is an error, not a path
// that may safely be created elsewhere.
func canonical(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	cur, tail := abs, []string{}
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				real = filepath.Join(real, tail[i])
			}
			return real, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fs.ErrNotExist
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func folded(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// openBoundary pins a real directory. No symlink is accepted as the boundary,
// and the opened handle must be the same directory that was checked.
func openBoundary(dir string) (*os.Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, boundaryError()
	}
	volume := filepath.VolumeName(abs)
	r, err := os.OpenRoot(volume + string(filepath.Separator))
	if err != nil {
		return nil, boundaryError()
	}
	parts := strings.Split(strings.TrimPrefix(abs, volume+string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		info, err := r.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			r.Close()
			return nil, boundaryError()
		}
		next, err := r.OpenRoot(part)
		if err != nil {
			r.Close()
			return nil, boundaryError()
		}
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(info, actual) {
			next.Close()
			r.Close()
			return nil, boundaryError()
		}
		r.Close()
		r = next
	}
	return r, nil
}

func boundaryError() error {
	return fail.New("output-boundary", "An output folder is missing, is a link, or changed while I opened it. Use real folders, then try again.")
}

// noLinks checks existing components beneath a pinned boundary. os.Root also
// enforces the boundary during every later operation, including after a race.
func noLinks(r *os.Root, name string) error {
	parts := strings.Split(filepath.Clean(name), string(filepath.Separator))
	p := ""
	for _, part := range parts {
		if part == "." {
			continue
		}
		p = filepath.Join(p, part)
		st, err := r.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fail.New("output-boundary", "I could not check an output path safely.")
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fail.New("output-boundary", "An output path holds a link to another place. Replace the link with a real folder or file, then try again.")
		}
	}
	return nil
}
