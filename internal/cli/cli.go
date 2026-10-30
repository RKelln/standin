// Package cli runs the standin commands.
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/hook"
	"github.com/RKelln/standin/internal/initcmd"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/store"
)

// Exit codes.
const (
	ExitOK      = 0 // done
	ExitWarn    = 1 // done, with warnings or findings
	ExitRefused = 2 // refused or error; nothing written
)

// Env is what a command can touch. Tests fill it in.
type Env struct {
	Stdin           io.Reader
	Stdout, Stderr  io.Writer
	StdinIsTerminal func() bool
	// StdoutIsTerminal reports whether stdout goes to a screen (not a pipe,
	// a file or an AI tool).
	StdoutIsTerminal func() bool
	Getenv           func(string) string
	Version, Commit  string
}

// ctx is one run of a command.
type ctx struct {
	env      Env
	cmd      string
	project  string // absolute, symlinks resolved
	private  string // --private, as given
	args     []string
	warnings int
	privDir  string
	printed  bool // private path printed already
}

// msg prints a line for the person (to stderr, so stdout stays clean).
func (c *ctx) msg(format string, args ...any) {
	fmt.Fprintf(c.env.Stderr, format+"\n", args...)
}

// warn prints a warning and makes the exit code 1.
func (c *ctx) warn(format string, args ...any) {
	c.warnings++
	fmt.Fprintf(c.env.Stderr, "Warning: "+format+"\n", args...)
}

type command struct {
	name    string
	usage   string
	summary string
	run     func(c *ctx) error
	// direct commands parse their own flags and set their own exit code.
	direct func(args []string, stdin io.Reader, stdout, stderr io.Writer) int
}

var commands []command

func init() {
	commands = []command{
		{"shape", "standin shape [--project DIR] [FILE...]", "Describe each raw file for the AI, with no real values.", runShape, nil},
		{"review", "standin review [--project DIR]", "Show the config as a plain table, keeps first.", runReview, nil},
		{"approve", "standin approve [--project DIR]", "Approve the config. You must type the answers yourself.", runApprove, nil},
		{"scrub", "standin scrub [--project DIR] [FILE...]", "Make the scrubbed copy the AI reads, and update the mapping.", runScrub, nil},
		{"restore", "standin restore [--project DIR] FILE... | -", "Put real values back in text the AI wrote.", runRestore, nil},
		{"check", "standin check [--project DIR] FILE... | -", "Look for real values and private patterns. Never prints them.", runCheck, nil},
		{"report", "standin report [--project DIR]", "Print an issue report with no data from your files.", runReport, nil},
		{"version", "standin version", "Print the version.", runVersion, nil},
		{"init", "standin init [--project DIR] [--harness claude,codex,opencode]", "Make the folders and set up your AI tool's guard rules.", nil, initcmd.Run},
		{"hook", "standin hook claude EVENT", "Called by your AI tool, not by you. Blocks reads of private files.", nil, hook.Run},
	}
}

const intro = `Standin lets an AI work on your files without seeing your private data.

Words used here:
  stand-in   a label like [PERSON_1] that takes the place of a real value.
  tokenize   swap each real value for a stand-in.
  mapping    the private list that links each stand-in to its real value.
             It is the secret. It lives outside the project folder. Never give it to an AI.

Steps: put real files in raw/. Run shape. The AI reads shape/ and writes
standin.config.json. You run review and approve. Run scrub. The AI reads
only scrubbed/. Run restore on what the AI writes to put real values back.

Commands:
`

func usage(w io.Writer) {
	fmt.Fprint(w, intro)
	for _, cmd := range commands {
		fmt.Fprintf(w, "  %-46s %s\n", cmd.usage, cmd.summary)
	}
	fmt.Fprint(w, `
Options:
  --project DIR   the project folder (default: this folder)
  --private DIR   where the mapping lives (default: $STANDIN_HOME, or ~/.standin)

Exit codes: 0 done, 1 done with warnings or findings, 2 stopped (nothing written).
`)
}

// Run runs standin with args (without the program name) and returns the exit code.
func Run(args []string, env Env) int {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.StdinIsTerminal == nil {
		env.StdinIsTerminal = func() bool { return false }
	}
	if env.StdoutIsTerminal == nil {
		env.StdoutIsTerminal = func() bool { return false }
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(env.Stdout)
		return ExitOK
	}
	if args[0] == "--version" {
		args[0] = "version"
	}
	var cmd *command
	for i := range commands {
		if commands[i].name == args[0] {
			cmd = &commands[i]
		}
	}
	if cmd == nil {
		fmt.Fprintf(env.Stderr, "I do not know the command %q. Run `standin help` to see the commands.\n", args[0])
		return ExitRefused
	}
	if cmd.direct != nil {
		return cmd.direct(args[1:], env.Stdin, env.Stdout, env.Stderr)
	}
	c := &ctx{env: env, cmd: cmd.name}
	project, help, err := c.parseFlags(args[1:])
	if help {
		fmt.Fprintf(env.Stdout, "Usage: %s\n\n%s\n", cmd.usage, cmd.summary)
		return ExitOK
	}
	if err == nil {
		err = c.setProject(project)
	}
	if err == nil {
		err = cmd.run(c)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "Stopped: %s\n", err.Error())
		if c.privDir == "" && c.project != "" {
			c.privDir, _ = store.PrivateDir(c.project, c.private, env.Getenv("STANDIN_HOME"))
		}
		store.RecordError(c.privDir, c.cmd, fail.Code(err))
		return ExitRefused
	}
	if c.warnings > 0 {
		return ExitWarn
	}
	return ExitOK
}

func (c *ctx) parseFlags(args []string) (project string, help bool, err error) {
	project = "."
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			c.args = append(c.args, args[i+1:]...)
			return project, false, nil
		case a == "-h" || a == "--help":
			return project, true, nil
		case a == "--project" || a == "-project" || a == "--private" || a == "-private":
			if i+1 >= len(args) {
				return "", false, fail.New("usage", "%s needs a folder after it.", a)
			}
			if strings.HasSuffix(a, "project") {
				project = args[i+1]
			} else {
				c.private = args[i+1]
			}
			i++
		case strings.HasPrefix(a, "--project=") || strings.HasPrefix(a, "-project="):
			project = a[strings.Index(a, "=")+1:]
		case strings.HasPrefix(a, "--private=") || strings.HasPrefix(a, "-private="):
			c.private = a[strings.Index(a, "=")+1:]
		case len(a) > 1 && a[0] == '-':
			return "", false, fail.New("usage", "I do not know the option %q. Run `standin %s --help`.", a, c.cmd)
		default:
			c.args = append(c.args, a)
		}
	}
	return project, false, nil
}

func (c *ctx) setProject(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fail.New("project", "I could not find the project folder.")
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return fail.New("project", "I could not find the project folder %s.", abs)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	c.project = abs
	return nil
}

// privateDir works out the private folder, makes it if create is true, and
// prints its path once.
func (c *ctx) privateDir(create bool) (string, error) {
	if c.privDir == "" {
		d, err := store.PrivateDir(c.project, c.private, c.env.Getenv("STANDIN_HOME"))
		if err != nil {
			return "", err
		}
		c.privDir = d
	}
	if create {
		if err := store.Ensure(c.privDir); err != nil {
			return "", err
		}
	}
	if !c.printed {
		c.printed = true
		c.msg("Private folder (holds the mapping; keep it away from the AI): %s", c.privDir)
	}
	return c.privDir, nil
}

// resolve finds a file named on the command line: as given (from this
// folder), or else inside the project folder.
func (c *ctx) resolve(arg string) (string, error) {
	if filepath.IsAbs(arg) {
		return clean(arg), nil
	}
	if abs, err := filepath.Abs(arg); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return clean(abs), nil
		}
	}
	p := filepath.Join(c.project, arg)
	if _, err := os.Stat(p); err == nil {
		return clean(p), nil
	}
	return "", fail.New("not-found", "I could not find the file %s.", c.showArg(arg))
}

// clean resolves symlinks in the folder part, so paths compare with the project.
func clean(p string) string {
	p = filepath.Clean(p)
	if dir, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(dir, filepath.Base(p))
	}
	return p
}

// rel gives the project-relative slash path of abs, and whether it is inside.
func (c *ctx) rel(abs string) (string, bool) {
	r, err := filepath.Rel(c.project, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// dataLikeName reports whether any part of a path looks like private data.
// Each folder and the full file name are checked, extension included (a
// name can hide in it: "statement.Zelda Quorn"), with and without the
// extension, and each dot-separated part.
func dataLikeName(rel string) bool {
	for _, p := range strings.Split(rel, "/") {
		for _, s := range []string{p, strings.TrimSuffix(p, filepath.Ext(p))} {
			if _, ok := scan.LooksLikeDataName(s); ok {
				return true
			}
		}
	}
	return false
}

const hiddenName = "a file whose name looks like private data (rename it to something plain, like invoices.csv)"

// show gives a project-relative path for messages, or a note if the name looks like data.
func show(rel string) string {
	if dataLikeName(rel) {
		return hiddenName
	}
	return rel
}

func (c *ctx) showArg(arg string) string {
	if dataLikeName(filepath.ToSlash(arg)) {
		return hiddenName
	}
	return arg
}

// rawFile is a file inside raw/.
type rawFile struct {
	abs   string
	rel   string // project-relative, like raw/a/b.csv
	inRaw string // relative to raw/, like a/b.csv
}

// rawFiles returns the named files (each must be inside raw/), or every file under raw/.
func (c *ctx) rawFiles() ([]rawFile, error) {
	var out []rawFile
	if len(c.args) == 0 {
		root := filepath.Join(c.project, "raw")
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			return nil, fail.New("no-raw", "There is no raw/ folder in the project. Make one and put your files in it.")
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if p != root && strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || skipJunk[strings.ToLower(name)] {
				return nil
			}
			r, _ := c.rel(p)
			out = append(out, rawFile{abs: p, rel: r, inRaw: strings.TrimPrefix(r, "raw/")})
			return nil
		})
		if err != nil {
			return nil, fail.New("read", "I could not read the raw/ folder.")
		}
		if len(out) == 0 {
			return nil, fail.New("no-raw", "The raw/ folder is empty. Put your files in it first.")
		}
		sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
		return out, nil
	}
	seen := map[string]bool{}
	for _, a := range c.args {
		if a == "-" {
			return nil, fail.New("usage", "This command reads files, not typed text.")
		}
		abs, err := c.resolve(a)
		if err != nil {
			return nil, err
		}
		r, ok := c.rel(abs)
		if !ok || !strings.HasPrefix(r, "raw/") {
			return nil, fail.New("not-raw", "%s is not in the raw/ folder of the project. Put real files in raw/ first, then try again.", c.showArg(a))
		}
		if st, err := os.Stat(abs); err != nil || st.IsDir() {
			return nil, fail.New("not-found", "%s is not a file.", show(r))
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, rawFile{abs: abs, rel: r, inRaw: strings.TrimPrefix(r, "raw/")})
		}
	}
	return out, nil
}

// Files a computer adds to folders by itself.
var skipJunk = map[string]bool{"desktop.ini": true, "thumbs.db": true}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
