// Package store finds and guards the private folder (where the mapping
// lives), and writes files so that a failure leaves no half-written output.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RKelln/standin/internal/fail"
)

// File names in the private folder.
const (
	MappingFile  = "mapping.json"
	ApprovalFile = "approval.json"
	WordsFile    = "words.json" // word choices for free text; as secret as the mapping
	ErrorsFile   = "errors.json"
)

// PrivateDir works out the private folder for a project.
// override is --private; home is $STANDIN_HOME (may be empty).
func PrivateDir(project, override, home string) (string, error) {
	absProject, err := filepath.Abs(project)
	if err != nil {
		return "", fail.New("private", "I could not work out the full path of the project folder.")
	}
	// The same folder reached through a symlink must find the same private folder.
	if real, err := filepath.EvalSymlinks(absProject); err == nil {
		absProject = real
	}
	var dir string
	if override != "" {
		dir, err = filepath.Abs(override)
		if err != nil {
			return "", fail.New("private", "I could not work out the full path of --private.")
		}
	} else {
		root, err := projectsRoot(home)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256([]byte(absProject))
		dir = filepath.Join(root, folderName(absProject)+"-"+hex.EncodeToString(sum[:])[:8])
	}
	if Inside(dir, absProject) {
		return "", fail.New("private-inside", "The private folder must not be inside the project folder, because the AI can read the project folder. Choose a folder outside it.")
	}
	return dir, nil
}

// Root is the folder that holds every private folder: $STANDIN_HOME, or
// ~/.standin. The hooks guard all of it, not only this project's folder.
func Root(home string) (string, error) {
	root, err := projectsRoot(home)
	if err != nil {
		return "", err
	}
	return filepath.Dir(root), nil
}

// projectsRoot is $STANDIN_HOME/projects (home defaults to ~/.standin).
func projectsRoot(home string) (string, error) {
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fail.New("private", "I could not find your home folder. Set STANDIN_HOME to a private folder, then try again.")
		}
		home = filepath.Join(h, ".standin")
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return "", fail.New("private", "I could not work out the full path of STANDIN_HOME.")
	}
	return filepath.Join(home, "projects"), nil
}

func folderName(absProject string) string {
	name := filepath.Base(absProject)
	if name == "" || name == string(filepath.Separator) || name == "." {
		name = "project"
	}
	return name
}

// SameNameDirs lists private folders (other than dir) made for a project
// folder with the same name as project: what is left behind when a project
// folder is moved, since the private folder is keyed by the full path.
func SameNameDirs(project, dir, home string) []string {
	root, err := projectsRoot(home)
	if err != nil {
		return nil
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return nil
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	prefix := folderName(abs) + "-"
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if !e.IsDir() || !strings.HasPrefix(n, prefix) || len(n) != len(prefix)+8 {
			continue
		}
		if _, err := hex.DecodeString(n[len(prefix):]); err != nil {
			continue
		}
		p := filepath.Join(root, n)
		if p != dir {
			out = append(out, p)
		}
	}
	return out
}

// Inside reports whether path is dir or inside dir.
func Inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Ensure makes the private folder (0700) and its parents if needed, and tightens it if it is open.
func Ensure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail.New("private", "I could not make the private folder %s: %s", dir, fail.Reason(err))
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fail.New("private", "I could not lock the private folder %s: %s", dir, fail.Reason(err))
	}
	return nil
}

// Exists reports whether dir exists.
func Exists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// Out is one file to write.
type Out struct {
	Path string
	Data []byte
	Perm os.FileMode
}

// WriteAll writes every file, or none. Each file goes to a temporary file
// in the same folder first; only when all are written are they renamed into
// place. If a rename fails, files already renamed are put back as they were
// (or removed if they were new).
func WriteAll(outs []Out) error {
	type staged struct {
		out     Out
		tmp     string
		backup  []byte
		existed bool
		mode    os.FileMode
	}
	var st []staged
	cleanup := func() {
		for _, s := range st {
			if s.tmp != "" {
				os.Remove(s.tmp)
			}
		}
	}
	for _, o := range outs {
		dir := filepath.Dir(o.Path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			cleanup()
			return fail.New("write", "I could not make a folder for the output (%s). I wrote nothing.", fail.Reason(err))
		}
		f, err := os.CreateTemp(dir, ".standin-tmp-*")
		if err != nil {
			cleanup()
			return fail.New("write", "I could not write an output file (%s). I wrote nothing.", fail.Reason(err))
		}
		s := staged{out: o, tmp: f.Name()}
		st = append(st, s)
		err = f.Chmod(o.Perm)
		if err == nil {
			_, err = f.Write(o.Data)
		}
		if err == nil {
			err = f.Sync()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			cleanup()
			return fail.New("write", "I could not write an output file (%s). I wrote nothing.", fail.Reason(err))
		}
		if old, err := os.ReadFile(o.Path); err == nil {
			st[len(st)-1].backup, st[len(st)-1].existed = old, true
			if info, err := os.Stat(o.Path); err == nil {
				st[len(st)-1].mode = info.Mode().Perm()
			}
		}
	}
	for i := range st {
		if err := os.Rename(st[i].tmp, st[i].out.Path); err != nil {
			// Undo the ones already moved.
			for j := 0; j < i; j++ {
				if st[j].existed {
					_ = os.WriteFile(st[j].out.Path, st[j].backup, st[j].mode)
				} else {
					os.Remove(st[j].out.Path)
				}
			}
			for j := i; j < len(st); j++ {
				os.Remove(st[j].tmp)
			}
			return fail.New("write", "I could not save an output file (%s). I put back what I had changed.", fail.Reason(err))
		}
		st[i].tmp = ""
	}
	return nil
}

// Approval is what approve stores.
type Approval struct {
	Version      int      `json:"version"`
	ConfigSHA256 string   `json:"config_sha256"`
	ApprovedAt   string   `json:"approved_at"`
	Keeps        []string `json:"keeps"`
	Scans        []string `json:"scans,omitempty"`
	// Texts lists "<match>:text" for each approved word check on free text.
	Texts []string `json:"texts,omitempty"`
	// WordsSHA256 is the sha256 of words.json as approve wrote it. A changed
	// words.json voids the approval.
	WordsSHA256 string `json:"words_sha256,omitempty"`
}

// LoadApproval reads approval.json. A missing file gives nil, nil.
func LoadApproval(dir string) (*Approval, error) {
	b, err := os.ReadFile(filepath.Join(dir, ApprovalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail.New("approval-read", "I could not read the approval file (%s).", fail.Reason(err))
	}
	var a Approval
	if err := json.Unmarshal(b, &a); err != nil || a.Version != 1 || a.ConfigSHA256 == "" {
		return nil, fail.New("approval-bad", "The approval file is damaged. Run `standin approve` again.")
	}
	return &a, nil
}

// MarshalApproval writes an approval as JSON.
func MarshalApproval(a Approval) []byte {
	b, _ := json.MarshalIndent(a, "", "  ")
	return append(b, '\n')
}

// ErrorEntry is a data-free record of a refusal, for `standin report`.
type ErrorEntry struct {
	Time    string `json:"time"`
	Command string `json:"command"`
	Code    string `json:"code"`
}

const keepErrors = 10

// RecordError adds a data-free error record to the private folder, if it
// exists. It never fails the command.
func RecordError(dir, command, code string) {
	if dir == "" || !Exists(dir) {
		return
	}
	es := LoadErrors(dir)
	es = append(es, ErrorEntry{Time: time.Now().UTC().Format(time.RFC3339), Command: command, Code: code})
	if len(es) > keepErrors {
		es = es[len(es)-keepErrors:]
	}
	b, _ := json.MarshalIndent(es, "", "  ")
	_ = WriteAll([]Out{{Path: filepath.Join(dir, ErrorsFile), Data: append(b, '\n'), Perm: 0o600}})
}

// LoadErrors reads the error records, or none.
func LoadErrors(dir string) []ErrorEntry {
	b, err := os.ReadFile(filepath.Join(dir, ErrorsFile))
	if err != nil {
		return nil
	}
	var es []ErrorEntry
	if json.Unmarshal(b, &es) != nil {
		return nil
	}
	return es
}
