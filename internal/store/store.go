// Package store finds and guards the private folder (where the mapping
// lives), and writes files so that a failure leaves no half-written output.
package store

import (
	"bytes"
	"crypto/rand"
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
	"github.com/RKelln/standin/internal/jsonsafe"
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
	dir, err = canonical(dir)
	if err != nil {
		return "", fail.New("private", "I could not check where the private folder leads. Use a real folder outside the project.")
	}
	if Inside(dir, absProject) {
		return "", fail.New("private-inside", "The private folder leads inside the project folder. Choose a real folder outside it, so the AI cannot read the mapping.")
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
	rel, err := filepath.Rel(folded(dir), folded(path))
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
	r, err := openBoundary(dir)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }() // Read-only directory handle; no buffered writes.
	if err := securePath(dir, true); err != nil {
		return fail.New("private", "I could not lock the private folder %s: %s", dir, fail.Reason(err))
	}
	// Older Windows files may have inherited wider ACLs. Tighten every direct
	// private entry too; a link is refused rather than changing its target.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fail.New("private", "I could not check the private folder permissions.")
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			return fail.New("private", "The private folder holds a link. Remove the link before trying again.")
		}
		if err := securePath(filepath.Join(dir, e.Name()), e.IsDir()); err != nil {
			if _, checkErr := r.Lstat(e.Name()); errors.Is(checkErr, fs.ErrNotExist) {
				continue
			}
			return fail.New("private-permissions", "I could not limit access to an old private file. I stopped to keep the mapping safe.")
		}
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
	// Root pins the privacy boundary for this output (for example restored/).
	// Every component is checked for links and every operation stays beneath it.
	Root string
}

// beforePublish is a fault-injection seam used only by package tests.
var beforePublish func(int)

// WriteAll stages every file before publication and rolls back ordinary
// rename failures. A rollback failure is reported truthfully. Private-state
// batches keep previous files and a pending marker until publication finishes;
// interrupted saves must be recovered by the person. This is not a claim of
// an all-files atomic commit or power-loss durability.
func WriteAll(outs []Out) (result error) {
	// Reject aliases before even making folders. Case-folding on every platform
	// makes a batch safe to use on case-insensitive file systems too.
	var destinations []string
	for _, o := range outs {
		abs, err := filepath.Abs(o.Path)
		if err != nil {
			return fail.New("output-path", "I could not check an output path. I wrote no output files.")
		}
		for _, prior := range destinations {
			if strings.EqualFold(abs, prior) {
				return fail.New("output-duplicate", "Two outputs use the same file name. Give them different names, then try again. I wrote no output files.")
			}
		}
		destinations = append(destinations, abs)
	}
	type staged struct {
		out     Out
		root    *os.Root
		name    string
		tmp     string
		backup  []byte
		existed bool
		mode    os.FileMode
	}
	var st []staged
	roots := map[string]*os.Root{}
	defer func() {
		for _, r := range roots {
			_ = r.Close() // Directory pins only; file closes and directory syncs are checked.
		}
	}()
	cleanup := func() bool {
		clean := true
		for _, s := range st {
			if s.tmp != "" {
				if err := s.root.Remove(s.tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
					clean = false
				}
			}
		}
		return clean
	}
	defer func() {
		if !cleanup() {
			result = withCleanup(result, cleanupError())
		}
	}()
	// Preflight every path before staging any output. Privacy boundaries must
	// be real directories; os.Root prevents a later link change from escaping.
	for _, o := range outs {
		dirPerm := os.FileMode(0755)
		if o.Perm.Perm()&0077 == 0 {
			dirPerm = 0700
		}
		dir := o.Root
		if dir == "" {
			dir = filepath.Dir(o.Path)
		}
		if _, ok := roots[dir]; !ok {
			if err := os.MkdirAll(dir, dirPerm); err != nil {
				return fail.New("write", "I could not make an output folder. I wrote no output files.")
			}
			r, err := openBoundary(dir)
			if err != nil {
				return err
			}
			roots[dir] = r
		}
		r := roots[dir]
		name, err := filepath.Rel(dir, o.Path)
		if err != nil || !filepath.IsLocal(name) {
			return fail.New("output-boundary", "An output path leaves its safe folder. I wrote no output files.")
		}
		if err := noLinks(r, name); err != nil {
			return err
		}
		if info, err := r.Stat(name); err == nil && !info.Mode().IsRegular() {
			return fail.New("write", "An output file is not a plain file. I wrote no output files.")
		}
		st = append(st, staged{out: o, root: r, name: name})
	}
	for i := range st {
		s := &st[i]
		o := s.out
		r := s.root
		dir := filepath.Dir(s.name)
		dirPerm := os.FileMode(0755)
		if o.Perm.Perm()&0077 == 0 {
			dirPerm = 0700
		}
		if err := r.MkdirAll(dir, dirPerm); err != nil {
			return fail.New("write", "I could not make a folder for the output (%s). I did not save the final output files.", fail.Reason(err))
		}
		f, tmp, err := rootTemp(r, dir, o.Perm)
		if err != nil {
			return withCleanup(fail.New("write", "I could not write an output file (%s). I did not save the final output files.", fail.Reason(err)), atomicCleanupError(err))
		}
		s.tmp = tmp
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
			return fail.New("write", "I could not write an output file (%s). I did not save the final output files.", fail.Reason(err))
		}
		if old, err := r.ReadFile(s.name); err == nil {
			s.backup, s.existed = old, true
			if info, err := r.Stat(s.name); err == nil {
				s.mode = info.Mode().Perm()
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fail.New("write", "I could not save the old output for rollback. I wrote no output files.")
		}
	}
	// Mark private-state publication, and keep previous private files for a
	// person recovering after interruption. A pending marker is never ignored.
	var pending *os.Root
	for _, s := range st {
		base := filepath.Base(s.name)
		if base != MappingFile && base != WordsFile && base != ApprovalFile {
			continue
		}
		if pending == nil {
			pending = s.root
			if _, err := pending.Lstat(PendingFile); err == nil || !errors.Is(err, fs.ErrNotExist) {
				return fail.New("private-pending", "A previous save did not finish. Ask the person to recover the private folder before trying again.")
			}
		}
		if s.existed {
			if err := rootAtomic(s.root, ".standin-previous-"+base, s.backup, 0600); err != nil {
				return withCleanup(fail.New("write", "I could not save the previous private file. I did not save the final output files."), atomicCleanupError(err))
			}
		}
	}
	if pending != nil {
		if err := rootAtomic(pending, PendingFile, []byte("{\"version\":1,\"note\":\"SECRET. A save is in progress. Do not use the mapping until it finishes. Previous private files are .standin-previous-*.\"}\n"), 0600); err != nil {
			return withCleanup(fail.New("write", "I could not mark the private save safely. I did not save the final output files."), atomicCleanupError(err))
		}
		if err := syncRoot(pending); err != nil {
			return fail.New("private-pending", "I could not confirm the unfinished-save mark was saved. I stopped before publishing files. Ask the person to check the private folder.")
		}
	}
	for i := range st {
		if beforePublish != nil {
			beforePublish(i)
		}
		if err := st[i].root.Rename(st[i].tmp, st[i].name); err != nil {
			// Undo the ones already moved.
			rolledBack := true
			for j := 0; j < i; j++ {
				if st[j].existed {
					if rootAtomic(st[j].root, st[j].name, st[j].backup, st[j].mode) != nil {
						rolledBack = false
					}
				} else {
					if st[j].root.Remove(st[j].name) != nil {
						rolledBack = false
					}
				}
			}
			if !cleanup() {
				rolledBack = false
			}
			if !rolledBack {
				return fail.New("write-rollback", "I could not finish saving or put back every file. Stop using these outputs. Ask the person to recover the private folder and scrubbed files from a matching backup.")
			}
			for _, s := range st {
				if syncParents(s.root, s.name) != nil {
					return fail.New("write-rollback", "I put back the old files, but could not confirm they were saved. Stop using the outputs and ask the person to check them.")
				}
			}
			if pending != nil {
				if err := pending.Remove(PendingFile); err != nil {
					return fail.New("private-pending", "I put back the old files, but could not clear the unfinished-save mark. Ask the person to check the private folder.")
				}
				if syncRoot(pending) != nil {
					return fail.New("private-pending", "I put back the old files, but could not confirm the private folder was saved. Ask the person to check it.")
				}
			}
			return fail.New("write", "I could not save an output file (%s). I put back what I had changed.", fail.Reason(err))
		}
		st[i].tmp = ""
	}
	for _, s := range st {
		if syncParents(s.root, s.name) != nil {
			return fail.New("write-sync", "The files were written, but I could not confirm the folders were saved. Stop using these outputs and ask the person to check them.")
		}
	}
	if pending != nil {
		if err := pending.Remove(PendingFile); err != nil {
			return fail.New("private-pending", "The files were saved, but I could not clear the unfinished-save mark. Stop using them and ask the person to check the private folder.")
		}
		if syncRoot(pending) != nil {
			return fail.New("private-pending", "The files were saved, but I could not confirm the private folder was saved. Ask the person to check it.")
		}
	}
	return nil
}

func rootTemp(r *os.Root, dir string, perm os.FileMode) (*os.File, string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, "", err
	}
	name := filepath.Join(dir, ".standin-tmp-"+hex.EncodeToString(random[:]))
	f, err := r.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm)
	if err == nil && perm.Perm() == 0600 {
		if e := secureFile(filepath.Join(r.Name(), name), f); e != nil {
			_ = f.Close() // Still empty: no private bytes were written before ACL setup.
			if err := r.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, "", cleanupError()
			}
			return nil, "", e
		}
	}
	return f, name, err
}

func syncParents(r *os.Root, name string) error {
	for dir := filepath.Dir(name); ; dir = filepath.Dir(dir) {
		if err := syncRoot(r, dir); err != nil {
			return err
		}
		if dir == "." {
			return nil
		}
	}
}

func rootAtomic(r *os.Root, name string, data []byte, perm os.FileMode) (result error) {
	f, tmp, err := rootTemp(r, filepath.Dir(name), perm)
	if err != nil {
		return err
	}
	defer func() {
		if tmp != "" {
			if err := r.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
				// The system error may hold a private path. Cleanup guidance is
				// data-free and still makes clear that the save did not finish.
				result = cleanupError()
			}
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err := r.Rename(tmp, name); err != nil {
		return err
	}
	tmp = "" // Renamed successfully; do not remove the published output.
	return nil
}

func cleanupError() error {
	return fail.New("write-cleanup", "The save did not finish, and I could not remove every temporary output file. Stop using these outputs. Ask the person to check the output and private folders before trying again.")
}

// Only the data-free cleanup warning may accompany an outer save error.
func atomicCleanupError(err error) error {
	if fail.Code(err) == "write-cleanup" {
		return err
	}
	return nil
}

func withCleanup(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	if primary == nil {
		return cleanup
	}
	return fail.New(fail.Code(primary), "%s %s", primary.Error(), cleanup.Error())
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
	if err := CheckReady(dir); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, ApprovalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail.New("approval-read", "I could not read the approval file (%s).", fail.Reason(err))
	}
	var a Approval
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil || jsonsafe.Validate(b) != nil || a.Version != 1 || !validHash(a.ConfigSHA256) || (a.WordsSHA256 != "" && !validHash(a.WordsSHA256)) {
		return nil, fail.New("approval-bad", "The approval file is damaged. Run `standin approve` again.")
	}
	return &a, nil
}

func validHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
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
// exists. Logging is optional; only failure to release its lease is returned.
func RecordError(dir, command, code string) (result error) {
	if dir == "" || !Exists(dir) {
		return nil
	}
	lease, err := lockFor(dir, 0)
	if err != nil {
		return nil
	}
	defer func() { result = lease.Close() }()
	es := LoadErrors(dir)
	es = append(es, ErrorEntry{Time: time.Now().UTC().Format(time.RFC3339), Command: command, Code: code})
	if len(es) > keepErrors {
		es = es[len(es)-keepErrors:]
	}
	b, _ := json.MarshalIndent(es, "", "  ")
	// The refusal is already reported. An optional log-save failure must not
	// replace it; any private temp stays confined to the private boundary.
	_ = WriteAll([]Out{{Path: filepath.Join(dir, ErrorsFile), Data: append(b, '\n'), Perm: 0o600}})
	return nil
}

// LoadErrors reads the error records, or none.
func LoadErrors(dir string) []ErrorEntry {
	if CheckReady(dir) != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, ErrorsFile))
	if err != nil {
		return nil
	}
	var es []ErrorEntry
	if jsonsafe.Validate(b) != nil || json.Unmarshal(b, &es) != nil || len(es) > keepErrors {
		return nil
	}
	for i := range es {
		stamp, err := time.Parse(time.RFC3339, es[i].Time)
		if err != nil || !safeErrorCommand[es[i].Command] || !safeErrorCode[es[i].Code] {
			return nil
		}
		es[i].Time = stamp.UTC().Format(time.RFC3339)
	}
	return es
}

// Only built-in labels may leave the private error log through report.
// A hand-edited log must not turn this data-free report into a secret reader.
var safeErrorCommand = stringSet("shape review approve scrub restore show check report version licences")
var safeErrorCode = stringSet(`error active-content approval-bad approval-read bad-answer bad-csv bad-encoding
bad-json bad-quote config-bad config-read duplicate-header empty hidden-char json-dup json-key json-shape
left-over lone-cr mapping-bad mapping-missing mapping-read name-is-data no-answer no-config no-mapping
no-raw no-terminal not-approved not-found not-raw not-text output-boundary private private-busy
private-inside private-lock private-pending private-permissions project ragged read same-output
standin-in-input stdout-not-terminal txt-table uncased-script unsupported usage word-lists words-bad
words-read write write-cleanup write-rollback write-sync`)

func stringSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, word := range strings.Fields(s) {
		m[word] = true
	}
	return m
}
