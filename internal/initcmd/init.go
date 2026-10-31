// Package initcmd implements `standin init`: it makes the project folders, a
// starter config, the harness hook settings and an AGENTS.md block.
//
// It plans every change first and refuses (exit 2, nothing written) if any part
// is unsafe: a settings file it cannot read, a Claude Code that is too old, or a
// broken AGENTS.md block. Running it again changes nothing.
package initcmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	failure "github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/hook"
	corestore "github.com/RKelln/standin/internal/store"
)

const (
	exitOK        = 0
	exitWarn      = 1
	exitBlock     = 2
	changeMade    = "made"
	changeUpdate  = "updated"
	claudeHarness = "claude"
)

// change is one planned file write.
type change struct {
	rel  string // shown to the person
	path string
	data []byte
	perm os.FileMode
	what string // "made" or "updated"
	note string // shown after the name
}

// Replaced in tests to exercise setup failures after folders were created.
var writeSetup = corestore.WriteAll

// Run is the entry point for `standin init`. args excludes "init".
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("standin init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", ".", "the project folder")
	harness := fs.String("harness", "claude", "which agent tools to set up: claude, codex, opencode (comma list)")
	bin := fs.String("bin", "", "full path of the standin program for hooks to run (default: this program)")
	private := fs.String("private", "", "private store folder, if not the default")
	noVersion := fs.Bool("no-version-check", false, "set up Claude Code hooks even if its version cannot be checked (not safe)")
	shared := fs.Bool("shared", false, "write the Claude Code hooks to .claude/settings.json (shared) instead of .claude/settings.local.json (this computer only)")
	noPromptGuard := fs.Bool("no-prompt-guard", false, "leave out the hook that stops known real values being sent in your chat messages")
	if err := fs.Parse(args); err != nil {
		return exitBlock
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "standin init: unexpected words:", strings.Join(fs.Args(), " "))
		return exitBlock
	}
	fail := func(msg string) int {
		fmt.Fprintln(stderr, "standin init: "+msg+"\nNothing was changed.")
		return exitBlock
	}

	harnesses := map[string]bool{}
	for _, h := range strings.Split(*harness, ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		switch h {
		case claudeHarness, "codex", "opencode":
			harnesses[h] = true
		case "":
		default:
			return fail(fmt.Sprintf("%q is not a harness standin knows. Use claude, codex or opencode.", h))
		}
	}

	proj, err := filepath.Abs(*project)
	if err != nil {
		return fail("cannot find the project folder: " + err.Error())
	}
	proj = filepath.Clean(proj)
	if st, err := os.Stat(proj); err != nil || !st.IsDir() {
		return fail("the project folder does not exist: " + proj)
	}
	if real, err := filepath.EvalSymlinks(proj); err == nil {
		proj = real
	}
	// Check existing destinations before reading or changing setup files.
	if err := preflight(proj, nil); err != nil {
		return fail(err.Error())
	}
	store, err := hook.Core.StoreDir(proj, *private)
	if err != nil {
		return fail("cannot work out the private store: " + err.Error())
	}
	if within(store, proj) {
		return fail("the private store " + store + " is inside the project. It must be outside, so the AI cannot see it.")
	}

	exe := *bin
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return fail("cannot find where standin is installed. Use --bin PATH.")
		}
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return fail("cannot use --bin: " + err.Error())
	}

	var warnings []string
	var plan []change

	// Claude Code: version check first, so a refusal writes nothing.
	if harnesses["claude"] {
		v, found, err := ClaudeVersion()
		switch {
		case !found:
			warnings = append(warnings, "Claude Code (claude) was not found, so its version was not checked. "+
				"The hooks need version "+MinClaude+" or newer. Check with: claude --version")
		case err != nil && !*noVersion:
			return fail("cannot read the Claude Code version (" + err.Error() + "). Standin needs " + MinClaude +
				" or newer, so its hooks block when they fail. Update Claude Code, or use --no-version-check if you are sure.")
		case err != nil:
			warnings = append(warnings, "Claude Code version not checked (--no-version-check). If it is older than "+MinClaude+", a broken hook lets data through.")
		case !atLeast(v, MinClaude) && !*noVersion:
			return fail("Claude Code " + v + " is too old. Standin needs " + MinClaude +
				" or newer, so its hooks block when they fail. Update Claude Code, then run standin init again.")
		case !atLeast(v, MinClaude):
			warnings = append(warnings, "Claude Code "+v+" is older than "+MinClaude+" (--no-version-check). A broken hook may let data through.")
		}
		c, err := planClaude(proj, claudeOpts{exe: exe, private: *private, store: store, home: guardHome(proj), shared: *shared, promptGuard: !*noPromptGuard})
		if err != nil {
			return fail(err.Error())
		}
		plan = append(plan, c...)
	}

	agents, err := planAgents(proj)
	if err != nil {
		return fail(err.Error())
	}
	plan = append(plan, agents...)

	cfg := filepath.Join(proj, hook.ConfigName)
	if _, err := os.Stat(cfg); errors.Is(err, os.ErrNotExist) {
		plan = append(plan, change{rel: hook.ConfigName, path: cfg, data: []byte(starterConfig), perm: 0o644, what: changeMade})
	} else if err != nil {
		return fail("cannot check " + hook.ConfigName + ": " + err.Error())
	}

	if harnesses["codex"] {
		plan = append(plan, planFixed(proj, filepath.Join("standin-harness", "codex-UNTESTED.md"), codexNote(proj))...)
	}
	if harnesses["opencode"] {
		plan = append(plan, planFixed(proj, filepath.Join("standin-harness", "opencode-UNTESTED.md"), opencodeNote(exe))...)
	}

	if err := preflight(proj, plan); err != nil {
		return fail(err.Error())
	}
	// Apply folders, then publish all setup files as one staged write. If a
	// normal error occurs, remove only the empty folders this run created.
	var did []string
	var created []string
	applyFail := func(msg string) int {
		var left []string
		for i := len(created) - 1; i >= 0; i-- {
			if err := os.Remove(created[i]); err != nil {
				rel, _ := filepath.Rel(proj, created[i])
				left = append(left, filepath.ToSlash(rel))
			}
		}
		fmt.Fprintln(stderr, "standin init: "+msg)
		if len(left) > 0 {
			fmt.Fprintln(stderr, "Some new folders remain: "+strings.Join(left, ", ")+".")
		}
		fmt.Fprintln(stderr, "Setup did not finish. Check the files above, then run standin init again before using the AI.")
		return exitBlock
	}
	for _, d := range []struct {
		name string
		perm os.FileMode
	}{{hook.RawDir, 0o700}, {hook.ShapeDir, 0o755}, {hook.ScrubbedDir, 0o755}, {hook.RestoredDir, 0o700}} {
		p := filepath.Join(proj, d.name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.Mkdir(p, d.perm); err != nil {
			return applyFail("cannot make " + d.name + "/: " + corestoreReason(err))
		}
		created = append(created, p)
		did = append(did, "made "+d.name+"/")
	}
	var outs []corestore.Out
	for _, c := range plan {
		if err := makeParents(proj, filepath.Dir(c.path), &created); err != nil {
			return applyFail("cannot make the folder for " + c.rel + ": " + corestoreReason(err))
		}
		perm := c.perm
		if st, err := os.Stat(c.path); err == nil {
			perm = st.Mode().Perm()
		}
		outs = append(outs, corestore.Out{Path: c.path, Root: proj, Data: c.data, Perm: perm})
	}
	if err := writeSetup(outs); err != nil {
		return applyFail(err.Error())
	}
	for _, c := range plan {
		did = append(did, c.what+" "+filepath.ToSlash(c.rel)+c.note)
	}

	fmt.Fprintln(stdout, "standin init: project "+proj)
	if len(did) == 0 {
		fmt.Fprintln(stdout, "Nothing to change. It was already set up.")
	}
	for _, d := range did {
		fmt.Fprintln(stdout, "  "+d)
	}
	fmt.Fprintln(stdout, "Private store (the secret list of real values) will be kept here, outside the project:")
	fmt.Fprintln(stdout, "  "+store)
	fmt.Fprintln(stdout, "Never give that folder, or the mapping.json in it, to an AI.")
	fmt.Fprintln(stdout, "Next: put your real files in raw/, then run: standin shape raw/<file>")
	if harnesses["claude"] {
		fmt.Fprintln(stdout, "Claude Code: restart it in this folder so it loads the hooks.")
		if *shared {
			fmt.Fprintln(stdout, "The hooks are in .claude/settings.json. It holds this computer's path to standin, so it may not work for others who share it.")
		} else {
			fmt.Fprintln(stdout, "The hooks are in .claude/settings.local.json, for this computer only. If this folder is in git, add that file to .gitignore.")
		}
		if *noPromptGuard {
			fmt.Fprintln(stdout, "The prompt guard is off (--no-prompt-guard): your own chat messages are not checked for real values.")
		} else {
			fmt.Fprintln(stdout, "The prompt guard is on: a chat message that holds a real value standin knows is not sent.")
			fmt.Fprintln(stdout, "  If you talk about your own records on purpose, run: standin init --no-prompt-guard")
		}
		fmt.Fprintln(stdout, "Be aware: the shell check is only a guess from the command text. A script or a clever command")
		fmt.Fprintln(stdout, "  can still read raw/. The hooks are a safety net, not a wall.")
		fmt.Fprintln(stdout, "We recommend turning on Claude Code's shell sandbox. With these deny rules, it blocks shell commands from raw/ and restored/:")
		fmt.Fprintln(stdout, `  add "sandbox": {"enabled": true, "allowUnsandboxedCommands": false} to .claude/settings.local.json.`)
		fmt.Fprintln(stdout, "  It works on macOS, Linux (needs bubblewrap and socat) and WSL2, not on plain Windows.")
		fmt.Fprintln(stdout, "  Check /sandbox to make sure it started. File tools, MCP servers and hooks run outside the shell sandbox.")
	}
	if harnesses["codex"] || harnesses["opencode"] {
		fmt.Fprintln(stdout, "Codex and OpenCode notes are in standin-harness/. They are untested; read them before you use them.")
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "Warning: "+w)
	}
	if len(warnings) > 0 {
		return exitWarn
	}
	return exitOK
}

// planFixed writes a file standin owns, if its content differs.
func planFixed(proj, rel, content string) []change {
	p := filepath.Join(proj, rel)
	old, err := os.ReadFile(p)
	if err == nil && bytes.Equal(old, []byte(content)) {
		return nil
	}
	what := "made"
	if err == nil {
		what = changeUpdate
	}
	return []change{{rel: rel, path: p, data: []byte(content), perm: 0o644, what: what}}
}

func corestoreReason(err error) string { return failure.Reason(err) }

// preflight refuses ambiguous or blocked destinations before setup changes
// anything. Checking every component also keeps existing setup symlinks from
// leading reads and writes to another folder.
func preflight(proj string, plan []change) error {
	dirs := map[string]bool{"raw": true, "shape": true, "scrubbed": true, "restored": true, ".claude": true, "standin-harness": true}
	files := map[string]bool{"AGENTS.md": true, hook.ConfigName: true,
		filepath.Join(".claude", "settings.json"): true, filepath.Join(".claude", "settings.local.json"): true}
	for _, c := range plan {
		rel, err := filepath.Rel(proj, c.path)
		if err != nil || !within(c.path, proj) {
			return errors.New("a setup path is outside the project folder")
		}
		files[rel] = true
	}
	var paths []string
	for p := range dirs {
		paths = append(paths, p)
	}
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		parts := strings.Split(rel, string(filepath.Separator))
		cur := proj
		for i, part := range parts {
			cur = filepath.Join(cur, part)
			st, err := os.Lstat(cur)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return fmt.Errorf("cannot check %s: %s", filepath.ToSlash(rel), failure.Reason(err))
			}
			if st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%s uses a link to another path. Replace it with a plain file or folder before setup", filepath.ToSlash(rel))
			}
			wantDir := i < len(parts)-1 || dirs[rel]
			if wantDir && !st.IsDir() {
				return fmt.Errorf("%s exists but is not a folder", filepath.ToSlash(rel))
			}
			if !wantDir && !st.Mode().IsRegular() {
				return fmt.Errorf("%s exists but is not a plain file", filepath.ToSlash(rel))
			}
		}
	}
	return nil
}

func makeParents(proj, dir string, created *[]string) error {
	rel, err := filepath.Rel(proj, dir)
	if err != nil || !within(dir, proj) {
		return errors.New("setup folder is outside the project")
	}
	cur := proj
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(cur, 0755); err != nil {
				return err
			}
			*created = append(*created, cur)
			continue
		}
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("setup path is not a plain folder")
		}
	}
	return nil
}

// guardHome is the private-folders root for the deny rules, or its
// projects/ folder if the project is inside the root (an odd STANDIN_HOME),
// so the rules never cover the project itself.
func guardHome(proj string) string {
	h := hook.HomeDir()
	if h == "" {
		return ""
	}
	for _, b := range []string{h, filepath.Join(h, "projects")} {
		if !within(proj, b) {
			return b
		}
	}
	return ""
}

func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

const starterConfig = `{
  "version": 1,
  "files": []
}
`
