package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/RKelln/standin/internal/store"
)

const (
	toolInputFilePath = "file_path"
	preToolUseEvent   = "PreToolUse"
	postToolUseEvent  = "PostToolUse"
)

// Tools the PreToolUse hook knows. init uses ToolMatcher for the hook matcher.
var (
	readTools  = map[string]string{"Read": toolInputFilePath, "NotebookRead": "notebook_path"}
	writeTools = map[string]string{"Edit": toolInputFilePath, "MultiEdit": toolInputFilePath, "Write": toolInputFilePath, "NotebookEdit": "notebook_path"}
	searchTool = map[string]bool{"Grep": true, "Glob": true, "LS": true}
	shellTools = map[string]bool{"Bash": true, "PowerShell": true}
)

// ToolMatcher sends every tool to the guard. Unknown tools must reach the
// refusal branch rather than silently bypassing the guard's matcher.
func ToolMatcher() string {
	return ".*"
}

type claude struct {
	in     Input
	g      Guard
	stdout io.Writer
	stderr io.Writer
}

func (c *claude) cwd() string {
	if c.in.Cwd != "" {
		return c.in.Cwd
	}
	return c.g.Project
}

// show gives a path relative to the project, with forward slashes, for messages.
func (c *claude) show(p string) string {
	a := c.g.abs(p, c.cwd())
	if rel, err := filepath.Rel(c.g.Project, a); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return a
}

func (c *claude) deny(msg string) int { return block(c.stderr, "[standin] "+msg) }

func (c *claude) emit(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return c.deny("could not write the hook answer, so it blocks to be safe.")
	}
	if _, err := c.stdout.Write(append(b, '\n')); err != nil {
		return exitBlock
	}
	return exitOK
}

const why = "Real private details are there. "

func zoneAdvice(z Zone) string {
	switch z {
	case ZoneRestored:
		return why + "restored/ is for the person only. Work on files in scrubbed/."
	case ZoneStore, ZoneSecret:
		return "That is the mapping: the secret list that turns stand-ins back into real values. The AI must never read it."
	}
	return why + "Work on files in scrubbed/."
}

func (c *claude) preToolUse() int {
	if err := store.CheckReady(c.g.Store); err != nil {
		return c.deny(err.Error())
	}
	var input map[string]any
	if len(c.in.ToolInput) == 0 || json.Unmarshal(c.in.ToolInput, &input) != nil || input == nil {
		return c.deny("the tool input is missing or not a JSON object, so this is blocked to be safe.")
	}
	str := func(k string) (string, bool, bool) { // value, present, ok type
		v, ok := input[k]
		if !ok || v == nil {
			return "", false, true
		}
		s, ok := v.(string)
		return s, true, ok
	}
	tool := c.in.ToolName
	switch {
	case tool == "AskUserQuestion" || tool == "TodoWrite" || tool == "TaskCreate" || tool == "TaskUpdate" || tool == "TaskGet" || tool == "TaskList" || tool == "EnterPlanMode" || tool == "ExitPlanMode":
		// These tools manage the conversation or task list, not local files.
		return exitOK
	case readTools[tool] != "":
		p, present, ok := str(readTools[tool])
		if !present || !ok || p == "" {
			return c.deny(tool + " has no file path, so this is blocked to be safe.")
		}
		z := c.g.ZoneOf(p, c.cwd())
		switch z {
		case ZoneNone:
			return exitOK
		case ZoneRaw:
			if tool == "Read" {
				return c.redirect(p, input)
			}
		}
		return c.deny("Do not read " + c.show(p) + ". " + zoneAdvice(z))

	case writeTools[tool] != "":
		p, present, ok := str(writeTools[tool])
		if !present || !ok || p == "" {
			return c.deny(tool + " has no file path, so this is blocked to be safe.")
		}
		if z := c.g.ZoneOf(p, c.cwd()); z != ZoneNone {
			return c.deny("Do not change " + c.show(p) + ". Standin keeps " + z.String() + " safe from edits. Write your work to a new file outside raw/ and restored/.")
		}
		return exitOK

	case searchTool[tool]:
		root, present, ok := str("path")
		if !ok {
			return c.deny(tool + " has a path that is not text, so this is blocked to be safe.")
		}
		if !present || root == "" {
			root = c.cwd()
		}
		if tool == "Glob" {
			pat, _, ok := str("pattern")
			if !ok {
				return c.deny("Glob has a pattern that is not text, so this is blocked to be safe.")
			}
			if strings.Contains(filepath.ToSlash(pat), "..") {
				return c.deny("Glob patterns with .. are blocked. Search inside scrubbed/ instead, like path: \"scrubbed\".")
			}
			if filepath.IsAbs(pat) || strings.HasPrefix(pat, "/") || strings.HasPrefix(pat, "~") {
				return c.deny("Glob patterns that start at the top of the disk are blocked. Use a relative pattern with path: \"scrubbed\".")
			}
		}
		if tool == "Grep" {
			if gl, _, ok := str("glob"); !ok || strings.Contains(gl, "..") || filepath.IsAbs(gl) {
				return c.deny("Grep has a glob that leaves the search folder, so this is blocked. Use path: \"scrubbed\".")
			}
		}
		if z := c.g.Covers(root, c.cwd()); z != ZoneNone {
			return c.deny(tool + " here would also search " + z.String() + ". Search only scrubbed/: set path to \"" +
				filepath.ToSlash(filepath.Join(c.show(c.g.Project), ScrubbedDir)) + "\" (or a file inside it).")
		}
		return exitOK

	case shellTools[tool]:
		cmd, present, ok := str("command")
		if !present || !ok {
			return c.deny(tool + " has no command, so this is blocked to be safe.")
		}
		if reason := c.shellRisk(cmd); reason != "" {
			return c.deny(reason + " Standin guesses this from the command text; it is a safety net, not a wall. Work on files in scrubbed/ instead.")
		}
		return exitOK
	}
	return c.deny("standin does not know the tool " + fmt.Sprintf("%q", tool) + ", so it blocks it to be safe. Change the hook matcher if this tool is safe.")
}

func (c *claude) redirect(p string, input map[string]any) int {
	rawAbs := c.g.abs(p, c.cwd())
	twin, ok := c.g.ScrubbedFor(p, c.cwd())
	if !ok {
		return c.deny("Do not read " + c.show(p) + ". " + zoneAdvice(ZoneRaw))
	}
	if !c.g.safeScrubbedTwin(twin) {
		return c.deny("The scrubbed copy is not a safe regular file. Replace links with real files and folders, then ask the person to run standin scrub again.")
	}
	rs, rerr := os.Stat(rawAbs)
	ts, terr := os.Stat(twin)
	if rerr != nil || terr != nil || rs.IsDir() || ts.IsDir() || !ts.ModTime().After(rs.ModTime()) {
		return c.deny("Do not read " + c.show(p) + ": it has real private details. There is no up-to-date scrubbed copy. " +
			"Ask the person to run: standin scrub " + c.show(p) + ". Then read " + c.show(twin) + ".")
	}
	out := make(map[string]any, len(input))
	for k, v := range input { // keeps offset, limit and anything else
		out[k] = v
	}
	out["file_path"] = twin
	note := "Standin gave you the scrubbed copy " + c.show(twin) + " instead of " + c.show(p) + ". " +
		"Real names and numbers are swapped for stand-ins like [PERSON_1]. Keep every stand-in exactly as it is; " +
		"the person puts the real values back later. Do not try to read raw/."
	return c.emit(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            preToolUseEvent,
			"permissionDecision":       "allow",
			"permissionDecisionReason": "standin: raw read sent to scrubbed copy",
			"updatedInput":             out,
			"additionalContext":        note,
		},
	})
}

var (
	segRe = regexp.MustCompile("(?i)(^|[\\s\"'=(/\\\\:;|&<>`])(raw|restored)($|[\\s\"'/\\\\*;|&<>)`])")
	// standin named anywhere (bare, by path, or .exe), then a command only the person may run.
	standinWordRe = regexp.MustCompile(`(?i)(^|[^a-z0-9_-])standin(\.exe)?($|[^a-z0-9_-])`)
	personOnlyRe  = regexp.MustCompile("(?i)(^|[\\s;|&(`=])(approve|restore|init|show)($|[\\s;|&)`])")
	// --private, -private, --project, -project, with or without =value.
	placeFlagRe = regexp.MustCompile(`(?i)(^|[\s;|&(=])--?(private|project)($|[\s=])`)
	// One plain standin command that never prints a real value, with no shell
	// joins, pipes, redirects or substitutions. It may name raw/ (to shape or scrub it).
	// The program must be the bare word standin (found on PATH) or this very program.
	safeStandinRe = regexp.MustCompile("^\\s*(standin|STANDIN_SELF)\\s+(shape|scrub|check|review|report|version)\\b[^;|&`$()<>{}\\n\\r]*$")
	// Short flag clusters that mean "recursive" for grep, cp, ls and friends.
	recFlagRe = regexp.MustCompile(`^-[A-Za-z]*[rR][A-Za-z]*$`)
)

// Tools that walk whole folder trees by themselves.
var sweepTools = map[string]bool{
	"rg": true, "ag": true, "ack": true, "find": true, "fd": true, "tree": true, "du": true,
	"tar": true, "zip": true, "rsync": true, "get-childitem": true, "gci": true,
}

// Tools that walk folder trees when given a recursive flag.
var recTools = map[string]bool{
	"grep": true, "egrep": true, "fgrep": true, "zgrep": true, "cp": true, "ls": true,
	"chmod": true, "chown": true, "select-string": true, "copy-item": true, "dir": true,
}

const personOnlyMsg = "Only the person runs standin approve, restore, show and init, in their own terminal. The AI must never run them, not even through another program."

// shellRisk returns why a shell command looks like it touches a protected place, or "".
func (c *claude) shellRisk(cmd string) string {
	// Quotes and backslashes can split a word ("stand"in, s\tandin); drop them for the word checks.
	// A backslash may be an escape (s\tandin) or a Windows path (C:\Tools\standin.exe): try both.
	flat := strings.NewReplacer(`"`, "", "'", "", `\`, "").Replace(cmd)
	flatPath := strings.NewReplacer(`"`, "", "'", "", `\`, "/").Replace(cmd)
	for _, x := range []string{flat, flatPath} {
		if standinWordRe.MatchString(x) && personOnlyRe.MatchString(x) {
			return personOnlyMsg
		}
	}
	if placeFlagRe.MatchString(flat) {
		return "Do not pass --private or --project. Standin picks those places itself, so the secret stays where the AI cannot reach it."
	}
	if safeStandin(cmd) {
		return ""
	}
	low := strings.ToLower(cmd)
	if m := segRe.FindStringSubmatch(cmd); m != nil {
		return "This command seems to use " + strings.ToLower(m[2]) + "/, which has real private details."
	}
	for _, s := range secretNames {
		if strings.Contains(low, s) {
			return "This command names " + s + ", the secret list of real values."
		}
	}
	if strings.Contains(low, ".standin") || strings.Contains(low, "standin_home") {
		return "This command seems to use the private store, where the secret list of real values lives."
	}
	for _, f := range append(forms(c.g.Store), forms(c.g.home())...) {
		if f != "" && strings.Contains(norm(cmd), norm(f)) {
			return "This command uses the private store, where the secret list of real values lives."
		}
		if f != "" && strings.Contains(low, strings.ToLower(filepath.ToSlash(f))) {
			return "This command uses the private store, where the secret list of real values lives."
		}
	}
	if reason := c.sweepRisk(cmd); reason != "" {
		return reason
	}
	return ""
}

// shellWord is one word of a shell command, roughly as the shell splits it.
type shellWord struct {
	text string // quotes removed
	glob bool   // has * ? [ or { outside quotes
}

// splitShell splits a command into words. It is rough: it knows quotes,
// backslashes and the common operators, nothing more.
func splitShell(cmd string) []shellWord {
	var out []shellWord
	var cur strings.Builder
	var glob, have bool
	flush := func() {
		if have {
			out = append(out, shellWord{text: cur.String(), glob: glob})
		}
		cur.Reset()
		glob, have = false, false
	}
	var quote rune
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			switch {
			case r == quote:
				quote = 0
			case r == '\\' && quote == '"' && i+1 < len(rs):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, have = r, true
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			have = true
		case strings.ContainsRune(" \t\r\n;|&()<>`", r):
			flush()
		case r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			flush()
		default:
			if strings.ContainsRune("*?[{", r) {
				glob = true
			}
			cur.WriteRune(r)
			have = true
		}
	}
	flush()
	return out
}

// pathLike says whether a word names a file or folder (or a pattern of them).
// A plain word counts if a file or folder by that name exists in cwd.
func pathLike(w shellWord, cwd string) (string, bool) {
	t := w.text
	if strings.HasPrefix(t, "-") {
		i := strings.Index(t, "=")
		if i < 0 {
			return "", false
		}
		t = t[i+1:]
	}
	if t == "" {
		return "", false
	}
	if w.glob || t == "." || t == ".." || strings.HasPrefix(t, "~") || strings.HasPrefix(t, "$HOME") ||
		strings.HasPrefix(t, "${HOME}") || strings.ContainsAny(t, `/\`) {
		return t, true
	}
	if _, err := os.Lstat(filepath.Join(cwd, t)); err == nil {
		return t, true
	}
	return "", false
}

// sweepRisk denies wildcards and folder-walking tools unless every path in the
// command is inside scrubbed/ or shape/ (decision 27). It is a guess from the text.
func (c *claude) sweepRisk(cmd string) string {
	words := splitShell(cmd)
	glob, rec := false, false
	recTool, lowRTool := false, false
	for _, w := range words {
		if w.glob {
			glob = true
		}
		base := strings.ToLower(filepath.Base(filepath.ToSlash(w.text)))
		base = strings.TrimSuffix(base, ".exe")
		if sweepTools[base] {
			rec = true
		}
		if recTools[base] {
			recTool = true
			if base != "ls" && base != "dir" {
				lowRTool = true
			}
		}
	}
	if recTool {
		for _, w := range words {
			lw := strings.ToLower(w.text)
			// -R is recursive everywhere; -r too, except for ls (where it means reverse).
			short := recFlagRe.MatchString(w.text) && (strings.Contains(w.text, "R") || lowRTool)
			if short || lw == "--recursive" || lw == "-recurse" || lw == "--dereference-recursive" || lw == "/s" {
				rec = true
			}
		}
	}
	if !glob && !rec {
		return ""
	}
	safe := []string{filepath.Join(c.g.Project, ScrubbedDir), filepath.Join(c.g.Project, ShapeDir)}
	inSafe := func(p string) bool {
		a := c.g.abs(p, c.cwd())
		for _, f := range forms(a) {
			for _, s := range safe {
				if within(f, s) {
					return true
				}
			}
		}
		return false
	}
	paths := 0
	if len(words) == 0 {
		return c.sweepMsg()
	}
	for _, w := range words[1:] {
		t, ok := pathLike(w, c.cwd())
		if !ok {
			continue
		}
		paths++
		if w.glob {
			// Only the part before the first wildcard is fixed; check its folder.
			i := strings.IndexAny(t, "*?[{")
			t = t[:i]
			if j := strings.LastIndexAny(t, `/\`); j >= 0 {
				t = t[:j+1]
			} else {
				t = ""
			}
			if t == "" {
				t = "."
			}
		}
		if !inSafe(t) {
			return c.sweepMsg()
		}
	}
	if paths == 0 && !inSafe(".") {
		return c.sweepMsg() // a search with no path starts here, which reaches raw/
	}
	return ""
}

func (c *claude) sweepMsg() string {
	return "This command uses wildcards (* ? [ {) or searches whole folders, so it could reach raw/ or restored/. " +
		"Name files inside scrubbed/ or shape/ instead, like: grep -rn word scrubbed/"
}

func (c *claude) scanner() (Scanner, int) {
	s, err := Core.NewScanner(c.g.Project, c.g.Store)
	if err != nil || s == nil {
		msg := "unknown problem"
		if err != nil {
			msg = err.Error()
		}
		return nil, c.deny("cannot check for real values (" + msg + "), so this is blocked to be safe. Run standin report and share it.")
	}
	return s, exitOK
}

func (c *claude) userPrompt() int {
	if c.in.Prompt == nil {
		return c.deny("the hook input has no prompt, so it is blocked to be safe.")
	}
	s, code := c.scanner()
	if s == nil {
		return code
	}
	fs := s.Scan(*c.in.Prompt)
	if len(fs) == 0 {
		return exitOK
	}
	return block(c.stderr, "[standin] Your message was not sent. It has "+describe(fs)+". "+
		"This guard stops private values that standin knows from being typed or pasted into the chat. "+
		"Use the stand-in (like [PERSON_1]) instead of the real value, or leave the detail out. "+
		"A stand-in is a made-up label that takes the place of a real value. "+
		"If you talk about your own records on purpose, you can turn this guard off: run standin init --no-prompt-guard in your own terminal.")
}

func (c *claude) postToolUse() int {
	if len(c.in.ToolResponse) == 0 {
		return c.deny("the hook input has no tool result to check, so it is blocked to be safe.")
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(c.in.ToolResponse))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return c.deny("the tool result is not valid JSON, so it is blocked to be safe.")
	}
	s, code := c.scanner()
	if s == nil {
		return code
	}
	var sb strings.Builder
	flatten(v, &sb)
	if len(knownOnly(s.Scan(sb.String()))) == 0 {
		return exitOK
	}
	// Say nothing about which value or how many (decision 26), so the agent
	// cannot use this warning to test guesses.
	note := "[standin] Warning: a real value may be in that tool output. " +
		"Do not repeat, copy or save anything from it. " +
		"Ask the person to check it, and to run standin report and describe how it happened, with no data in the description."
	return c.emit(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     postToolUseEvent,
			"additionalContext": note,
		},
	})
}

func flatten(v any, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(k)
			sb.WriteString("\n")
			flatten(t[k], sb)
		}
	case []any:
		for _, x := range t {
			flatten(x, sb)
		}
	case string:
		sb.WriteString(t)
		sb.WriteString("\n")
	case json.Number:
		sb.WriteString(t.String())
		sb.WriteString("\n")
	}
}

// safeStandin reports whether cmd is one plain, read-safe standin command.
func safeStandin(cmd string) bool {
	if strings.Contains(strings.ToLower(cmd), "mapping.json") {
		return false
	}
	trimmed := strings.TrimLeft(cmd, " \t")
	if self, err := os.Executable(); err == nil && self != "" && strings.HasPrefix(trimmed, self+" ") {
		trimmed = "STANDIN_SELF" + trimmed[len(self):]
	} else if strings.HasPrefix(trimmed, "STANDIN_SELF") {
		return false
	}
	return safeStandinRe.MatchString(trimmed)
}
