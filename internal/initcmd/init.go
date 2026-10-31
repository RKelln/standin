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
	"strings"

	"github.com/RKelln/standin/internal/hook"
)

const (
	exitOK    = 0
	exitWarn  = 1
	exitBlock = 2
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
		case "claude", "codex", "opencode":
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
		plan = append(plan, change{rel: hook.ConfigName, path: cfg, data: []byte(starterConfig), perm: 0o644, what: "made"})
	} else if err != nil {
		return fail("cannot check " + hook.ConfigName + ": " + err.Error())
	}

	if harnesses["codex"] {
		plan = append(plan, planFixed(proj, filepath.Join("standin-harness", "codex-UNTESTED.md"), codexNote(proj))...)
	}
	if harnesses["opencode"] {
		plan = append(plan, planFixed(proj, filepath.Join("standin-harness", "opencode-UNTESTED.md"), opencodeNote(exe))...)
	}

	// Apply: folders, then files.
	var did []string
	for _, d := range []struct {
		name string
		perm os.FileMode
	}{{hook.RawDir, 0o700}, {hook.ShapeDir, 0o755}, {hook.ScrubbedDir, 0o755}, {hook.RestoredDir, 0o700}} {
		p := filepath.Join(proj, d.name)
		if st, err := os.Stat(p); err == nil {
			if !st.IsDir() {
				return fail(d.name + " exists but is not a folder.")
			}
			continue
		}
		if err := os.Mkdir(p, d.perm); err != nil {
			return fail("cannot make " + d.name + "/: " + err.Error())
		}
		did = append(did, "made "+d.name+"/")
	}
	for _, c := range plan {
		if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
			return fail("cannot make the folder for " + c.rel + ": " + err.Error())
		}
		if err := atomicWrite(c.path, c.data, c.perm); err != nil {
			return fail("cannot write " + c.rel + ": " + err.Error())
		}
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
		fmt.Fprintln(stdout, "We recommend turning on Claude Code's sandbox, which blocks every program from raw/ and restored/:")
		fmt.Fprintln(stdout, `  add "sandbox": {"enabled": true, "allowUnsandboxedCommands": false} to .claude/settings.local.json.`)
		fmt.Fprintln(stdout, "  It works on macOS, Linux (needs bubblewrap and socat) and WSL2, not on plain Windows.")
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
		what = "updated"
	}
	return []change{{rel: rel, path: p, data: []byte(content), perm: 0o644, what: what}}
}

// atomicWrite writes to a temp file in the same folder, then renames it.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".standin-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
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
