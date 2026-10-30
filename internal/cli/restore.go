package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/restore"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/store"
	"github.com/RKelln/standin/internal/textenc"
)

// loadMapping reads the mapping. need=true refuses if there is none.
func (c *ctx) loadMapping(need bool) (*mapping.Mapping, bool, error) {
	privDir, err := c.privateDir(false)
	if err != nil {
		return nil, false, err
	}
	m, exists, err := mapping.Load(filepath.Join(privDir, store.MappingFile))
	if err != nil {
		return nil, false, err
	}
	if !exists && need {
		return nil, false, fail.New("no-mapping", "There is no mapping for this project yet. Run `standin scrub` first.")
	}
	return m, exists, nil
}

// readText reads a file (or stdin for "-") as text.
func (c *ctx) readText(arg string) (string, error) {
	var b []byte
	var err error
	if arg == "-" {
		b, err = io.ReadAll(c.env.Stdin)
	} else {
		var abs string
		abs, err = c.resolve(arg)
		if err != nil {
			return "", err
		}
		b, err = os.ReadFile(abs)
	}
	if err != nil {
		return "", fail.New("read", "I could not read %s.", c.showArg(arg))
	}
	if len(b) == 0 {
		return "", nil
	}
	text, _, err := textenc.Decode(b)
	if err != nil {
		return "", fail.New(fail.Code(err), "%s: %s", c.showArg(arg), err.Error())
	}
	return text, nil
}

// restoredPath: scrubbed/a/b.md -> restored/a/b.md; anything else -> restored/<base name>.
func (c *ctx) restoredPath(abs string) string {
	if r, ok := c.rel(abs); ok && strings.HasPrefix(r, "scrubbed/") {
		return filepath.Join(c.project, "restored", filepath.FromSlash(strings.TrimPrefix(r, "scrubbed/")))
	}
	return filepath.Join(c.project, "restored", filepath.Base(abs))
}

func modeFor(name string) restore.Mode {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		return restore.CSV
	case ".tsv":
		return restore.TSV
	case ".json":
		return restore.JSON
	}
	return restore.Text
}

func runRestore(c *ctx) error {
	if len(c.args) == 0 {
		return fail.New("usage", "Name the files to restore, or use - to read typed text. For example: standin restore scrubbed/report.md")
	}
	for _, a := range c.args {
		if a == "-" && !c.env.StdoutIsTerminal() {
			return fail.New("stdout-not-terminal", "restore - prints real values, so it only prints to a screen, not to a pipe, a file or an AI tool. Run it yourself in a terminal, or restore a file instead: its output goes to restored/.")
		}
	}
	m, _, err := c.loadMapping(true)
	if err != nil {
		return err
	}
	type job struct {
		arg, label string
		res        restore.Result
		out        string // "" for stdout
	}
	var jobs []job
	var outs []store.Out
	dashSeen := false
	for _, a := range c.args {
		text, err := c.readText(a)
		if err != nil {
			return err
		}
		mode, label, out := restore.Text, "typed text", ""
		if a == "-" {
			if dashSeen {
				return fail.New("usage", "Use - only once.")
			}
			dashSeen = true
		} else {
			abs, _ := c.resolve(a)
			if r, ok := c.rel(abs); ok && (strings.HasPrefix(r, "raw/") || strings.HasPrefix(r, "restored/")) {
				return fail.New("usage", "%s already has real values. Restore files the AI wrote, like those in scrubbed/.", c.showArg(a))
			}
			mode, label, out = modeFor(abs), c.showArg(a), c.restoredPath(abs)
		}
		res, err := restore.Restore(text, m, mode)
		if err != nil {
			return fail.New(fail.Code(err), "%s: %s", label, err.Error())
		}
		if mode == restore.JSON && json.Valid([]byte(text)) && !json.Valid([]byte(res.Out)) {
			return fail.New("write", "%s: the restored JSON would not be valid, so I wrote nothing.", label)
		}
		if out != "" {
			outs = append(outs, store.Out{Path: out, Data: []byte(res.Out), Perm: 0o600})
		}
		jobs = append(jobs, job{a, label, res, out})
	}
	if err := store.WriteAll(outs); err != nil {
		return err
	}
	for _, j := range jobs {
		if j.out == "" {
			io.WriteString(c.env.Stdout, j.res.Out)
		} else {
			r, _ := c.rel(j.out)
			c.msg("Wrote %s (%d stand-ins put back).", show(r), j.res.Replaced)
		}
		c.reportRestore(j.label, j.res)
	}
	if len(outs) > 0 {
		c.msg("restored/ holds real values. It is for you. Do not let the AI read it.")
	}
	return nil
}

func (c *ctx) reportRestore(label string, r restore.Result) {
	for _, f := range r.Unknown {
		c.warn("%s line %d: %s is not in the mapping, so I left it. The AI may have made it up.", label, f.Line, f.Text)
	}
	for _, f := range r.NearMiss {
		like := ""
		if f.Like != "" {
			like = " It looks like " + f.Like + "."
		}
		c.warn("%s line %d: %q looks almost like a stand-in, so I did not change it.%s Fix it by hand if needed.", label, f.Line, f.Text, like)
	}
	if n := len(r.Spellings); n > 0 {
		c.warn("%s: %d stand-in(s) had more than one spelling; the first one was used: %s", label, n, strings.Join(r.Spellings, ", "))
	}
}

func runCheck(c *ctx) error {
	if len(c.args) == 0 {
		return fail.New("usage", "Name the files to check, or use - to read typed text. For example: standin check scrubbed/invoices.csv")
	}
	m, exists, err := c.loadMapping(false)
	if err != nil {
		return err
	}
	if !exists {
		c.msg("There is no mapping yet, so I only look for patterns (emails, phone numbers and so on).")
	}
	known := scan.NewKnown(m)
	type finding struct {
		label string
		line  int
		what  string
	}
	var found []finding
	for _, a := range c.args {
		text, err := c.readText(a)
		if err != nil {
			return err
		}
		label := "typed text"
		if a != "-" {
			label = c.showArg(a)
		}
		for i, line := range strings.Split(text, "\n") {
			seen := map[string]bool{}
			for _, h := range scan.FindAll(line, known) {
				what := h.Kind
				if h.Standin != "" {
					what = "a real value from the mapping (" + h.Standin + ")"
				}
				if !seen[what] {
					seen[what] = true
					found = append(found, finding{label, i + 1, what})
				}
			}
		}
	}
	for _, f := range found {
		c.warn("%s line %d: %s", f.label, f.line, f.what)
	}
	if len(found) == 0 {
		c.msg("Found nothing. This check only knows the mapping and some patterns; read the files yourself too.")
	} else {
		c.msg("Found %d thing(s). I do not print the values. Open the file and look at those lines.", len(found))
	}
	return nil
}

func runVersion(c *ctx) error {
	commit := c.env.Commit
	if commit == "" {
		commit = "unknown"
	}
	fmt.Fprintf(c.env.Stdout, "standin %s (commit %s)\n", c.version(), commit)
	return nil
}
