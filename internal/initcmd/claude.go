package initcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RKelln/standin/internal/hook"
)

// MinClaude is the first Claude Code version with "onFailure": "block" on hooks
// (docs/research/prior-art.md B1, from the hooks docs; untested).
const MinClaude = "2.1.295"

// ClaudeVersion runs `claude --version`. found is false if claude is not on PATH.
// Tests replace it.
var ClaudeVersion = func() (version string, found bool, err error) {
	p, err := exec.LookPath("claude")
	if err != nil {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, p, "--version").Output()
	if err != nil {
		return "", true, errors.New("claude --version did not work")
	}
	v, ok := parseVersion(string(out))
	if !ok {
		return "", true, errors.New("could not find a version number in its output")
	}
	return v, true, nil
}

var verRe = regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)\b`)

func parseVersion(s string) (string, bool) {
	m := verRe.FindString(s)
	return m, m != ""
}

func atLeast(v, min string) bool {
	a, b := verRe.FindStringSubmatch(v), verRe.FindStringSubmatch(min)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return true
}

// DenyRules are added to permissions.deny in the settings file init writes.
// store is this project's private folder; home is the folder that holds every
// private folder ($STANDIN_HOME or ~/.standin), guarded whole.
func DenyRules(store, home string) []string {
	rules := []string{
		"Read(./raw/**)", "Edit(./raw/**)",
		"Read(./restored/**)", "Edit(./restored/**)",
		"Read(**/mapping.json)", "Edit(**/mapping.json)",
		"Read(**/words.json)", "Edit(**/words.json)",
	}
	for _, d := range []string{store, home} {
		if d != "" && filepath.IsAbs(d) && !strings.Contains(d, `\`) {
			// "//path" is an absolute path in Claude Code rules. Skipped on Windows paths (form unverified).
			s := "/" + filepath.ToSlash(d) + "/**"
			rules = append(rules, "Read("+s+")", "Edit("+s+")")
		}
	}
	return rules
}

// hookEntry is one standin hook. It names no project folder: the hook finds
// the project from CLAUDE_PROJECT_DIR or the hook input's cwd, so a moved
// folder keeps working (decision 34).
func hookEntry(exe, event, private string) map[string]any {
	args := []any{"hook", "claude", event}
	if private != "" {
		args = append(args, "--private", private)
	}
	return map[string]any{
		"type":      "command",
		"command":   exe,
		"args":      args,
		"onFailure": "block",
		"timeout":   json.Number("30"),
	}
}

// isOurs spots a hook that standin init wrote before: args "hook claude",
// and a program called standin (or standin.exe) in any folder, or exe, the
// program init writes now. Someone else's hook with the same args is kept.
func isOurs(h any, exe string) bool {
	m, ok := h.(map[string]any)
	if !ok {
		return false
	}
	args, ok := m["args"].([]any)
	if !ok || len(args) < 2 || args[0] != "hook" || args[1] != "claude" {
		return false
	}
	cmd, ok := m["command"].(string)
	if !ok {
		return false
	}
	if exe != "" && cmd == exe {
		return true
	}
	base := cmd[strings.LastIndexAny(cmd, `/\`)+1:]
	return base == "standin" || strings.EqualFold(base, "standin.exe")
}

// Settings files standin may write (decision 36).
var (
	sharedSettings = filepath.Join(".claude", "settings.json")
	localSettings  = filepath.Join(".claude", "settings.local.json")
)

// claudeOpts says how to set up Claude Code.
type claudeOpts struct {
	exe, private, store string
	home                string // the folder of all private folders, guarded whole
	shared              bool   // write .claude/settings.json, not settings.local.json
	promptGuard         bool   // add the UserPromptSubmit hook
}

// settingsFile is one Claude Code settings file, read as JSON.
type settingsFile struct {
	rel    string
	path   string
	old    []byte
	exists bool
	m      map[string]any
}

func readSettings(proj, rel string) (*settingsFile, error) {
	f := &settingsFile{rel: rel, path: filepath.Join(proj, rel), m: map[string]any{}}
	old, err := os.ReadFile(f.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read " + rel + ": " + err.Error())
	}
	f.old, f.exists = old, err == nil
	if f.exists && len(bytes.TrimSpace(old)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(old))
		dec.UseNumber()
		if err := dec.Decode(&f.m); err != nil || f.m == nil {
			return nil, errors.New(rel + " is not a JSON object standin can read. Fix it by hand (or move it away), then run standin init again.")
		}
		if dec.More() {
			return nil, errors.New(rel + " has extra text after the JSON. Fix it by hand, then run standin init again.")
		}
	}
	return f, nil
}

// change returns the write for f, or nil if its meaning did not change.
func (f *settingsFile) change(note string) ([]change, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f.m); err != nil {
		return nil, errors.New("cannot write the settings: " + err.Error())
	}
	if f.exists {
		if bytes.Equal(buf.Bytes(), f.old) {
			return nil, nil
		}
		// Compare meaning, not bytes, so a re-run on a hand-formatted file changes nothing.
		var a, b any
		if json.Unmarshal(f.old, &a) == nil && json.Unmarshal(buf.Bytes(), &b) == nil && jsonEqual(a, b) {
			return nil, nil
		}
	}
	if !f.exists {
		return []change{{rel: f.rel, path: f.path, data: buf.Bytes(), perm: 0o644, what: "made", note: note}}, nil
	}
	// Keep the old file as <name>.bak, written first, so a person can go back.
	return []change{
		{rel: f.rel + ".bak", path: f.path + ".bak", data: f.old, perm: 0o600, what: "saved the old copy as"},
		{rel: f.rel, path: f.path, data: buf.Bytes(), perm: 0o644, what: "updated", note: note},
	}, nil
}

// stripOurs removes every hook standin wrote, from every event, and drops
// groups and events left empty by that. Other hooks are kept as they were.
// It reports whether it removed any.
func stripOurs(f *settingsFile, exe string) (bool, error) {
	v, ok := f.m["hooks"]
	if !ok || v == nil {
		return false, nil
	}
	hooks, ok := v.(map[string]any)
	if !ok {
		return false, errors.New(f.rel + ": hooks is not an object. Fix it by hand, then run standin init again.")
	}
	removed := false
	for name, ev := range hooks {
		groups, ok := ev.([]any)
		if !ok {
			return false, errors.New(f.rel + ": hooks." + name + " is not a list. Fix it by hand, then run standin init again.")
		}
		var kept []any
		changed := false
		for _, g := range groups {
			gm, ok := g.(map[string]any)
			if !ok {
				kept = append(kept, g)
				continue
			}
			hs, ok := gm["hooks"].([]any)
			if !ok {
				kept = append(kept, g)
				continue
			}
			var rest []any
			for _, h := range hs {
				if !isOurs(h, exe) {
					rest = append(rest, h)
				}
			}
			if len(rest) == len(hs) {
				kept = append(kept, gm)
				continue
			}
			changed = true
			if len(rest) == 0 {
				continue // a group that held only standin's hook
			}
			gm["hooks"] = rest
			kept = append(kept, gm)
		}
		if !changed {
			continue
		}
		removed = true
		if len(kept) == 0 {
			delete(hooks, name)
		} else {
			hooks[name] = kept
		}
	}
	if removed && len(hooks) == 0 {
		delete(f.m, "hooks")
	}
	return removed, nil
}

// planClaude merges standin's hooks and deny rules into the chosen settings
// file, and removes standin's hooks from the other one (so an upgrade from
// v0.1.0, which wrote settings.json, does not keep an old copy).
func planClaude(proj string, o claudeOpts) ([]change, error) {
	rel, otherRel := localSettings, sharedSettings
	if o.shared {
		rel, otherRel = sharedSettings, localSettings
	}
	f, err := readSettings(proj, rel)
	if err != nil {
		return nil, err
	}
	other, err := readSettings(proj, otherRel)
	if err != nil {
		return nil, err
	}

	if _, err := stripOurs(f, o.exe); err != nil {
		return nil, err
	}
	hooks, err := objAt(f.m, "hooks", rel)
	if err != nil {
		return nil, err
	}
	events := []struct {
		name, matcher string
	}{{"PreToolUse", hook.ToolMatcher()}, {"PostToolUse", ""}}
	if o.promptGuard {
		events = append(events, struct{ name, matcher string }{"UserPromptSubmit", ""})
	}
	for _, ev := range events {
		var groups []any
		if v, ok := hooks[ev.name]; ok {
			g, ok := v.([]any)
			if !ok {
				return nil, errors.New(rel + ": hooks." + ev.name + " is not a list. Fix it by hand, then run standin init again.")
			}
			groups = g
		}
		group := map[string]any{"hooks": []any{hookEntry(o.exe, ev.name, o.private)}}
		if ev.matcher != "" {
			group["matcher"] = ev.matcher
		}
		hooks[ev.name] = append(groups, group)
	}

	perms, err := objAt(f.m, "permissions", rel)
	if err != nil {
		return nil, err
	}
	var deny []any
	if v, ok := perms["deny"]; ok {
		d, ok := v.([]any)
		if !ok {
			return nil, errors.New(rel + ": permissions.deny is not a list. Fix it by hand, then run standin init again.")
		}
		deny = d
	}
	have := map[string]bool{}
	for _, d := range deny {
		if s, ok := d.(string); ok {
			have[s] = true
		}
	}
	for _, r := range DenyRules(o.store, o.home) {
		if !have[r] {
			deny = append(deny, r)
		}
	}
	perms["deny"] = deny

	plan, err := f.change(" (hooks and deny rules)")
	if err != nil {
		return nil, err
	}
	if other.exists {
		removed, err := stripOurs(other, o.exe)
		if err != nil {
			return nil, err
		}
		if removed {
			note := " (took out standin's old hooks; they are now in " + filepath.ToSlash(rel) + ". Other settings kept)"
			if otherRel == sharedSettings {
				note += ". Others who share this folder now have no standin hooks: each must run standin init on their own computer"
			}
			c, err := other.change(note)
			if err != nil {
				return nil, err
			}
			plan = append(plan, c...)
		}
	}
	return plan, nil
}

func objAt(m map[string]any, key, rel string) (map[string]any, error) {
	v, ok := m[key]
	if !ok || v == nil {
		o := map[string]any{}
		m[key] = o
		return o, nil
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New(rel + ": " + key + " is not an object. Fix it by hand, then run standin init again.")
	}
	return o, nil
}

func jsonEqual(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}
