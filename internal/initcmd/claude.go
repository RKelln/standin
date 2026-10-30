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

// DenyRules are added to permissions.deny in .claude/settings.json.
func DenyRules(store string) []string {
	rules := []string{
		"Read(./raw/**)", "Edit(./raw/**)",
		"Read(./restored/**)", "Edit(./restored/**)",
		"Read(**/mapping.json)", "Edit(**/mapping.json)",
	}
	if store != "" && filepath.IsAbs(store) && !strings.Contains(store, `\`) {
		// "//path" is an absolute path in Claude Code rules. Skipped on Windows paths (form unverified).
		s := "/" + filepath.ToSlash(store) + "/**"
		rules = append(rules, "Read("+s+")", "Edit("+s+")")
	}
	return rules
}

func hookEntry(exe, event, proj, private string) map[string]any {
	args := []any{"hook", "claude", event, "--project", proj}
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

// isOurs spots a hook that standin init wrote before (any binary path).
func isOurs(h any) bool {
	m, ok := h.(map[string]any)
	if !ok {
		return false
	}
	args, ok := m["args"].([]any)
	if !ok || len(args) < 2 {
		return false
	}
	return args[0] == "hook" && args[1] == "claude"
}

// planClaude merges standin's hooks and deny rules into .claude/settings.json.
func planClaude(proj, exe, private, store string) ([]change, error) {
	rel := filepath.Join(".claude", "settings.json")
	p := filepath.Join(proj, rel)
	old, err := os.ReadFile(p)
	settings := map[string]any{}
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read " + rel + ": " + err.Error())
	}
	if exists && len(bytes.TrimSpace(old)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(old))
		dec.UseNumber()
		if err := dec.Decode(&settings); err != nil || settings == nil {
			return nil, errors.New(rel + " is not a JSON object standin can read. Fix it by hand (or move it away), then run standin init again.")
		}
		if dec.More() {
			return nil, errors.New(rel + " has extra text after the JSON. Fix it by hand, then run standin init again.")
		}
	}

	hooks, err := objAt(settings, "hooks", rel)
	if err != nil {
		return nil, err
	}
	events := []struct {
		name, matcher string
	}{{"PreToolUse", hook.ToolMatcher()}, {"PostToolUse", ""}, {"UserPromptSubmit", ""}}
	for _, ev := range events {
		var groups []any
		if v, ok := hooks[ev.name]; ok {
			g, ok := v.([]any)
			if !ok {
				return nil, errors.New(rel + ": hooks." + ev.name + " is not a list. Fix it by hand, then run standin init again.")
			}
			groups = g
		}
		var kept []any
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
				if !isOurs(h) {
					rest = append(rest, h)
				}
			}
			if len(rest) == 0 && len(hs) > 0 {
				continue // a group that held only standin's hook
			}
			if len(rest) != len(hs) {
				gm["hooks"] = rest
			}
			kept = append(kept, gm)
		}
		group := map[string]any{"hooks": []any{hookEntry(exe, ev.name, proj, private)}}
		if ev.matcher != "" {
			group["matcher"] = ev.matcher
		}
		hooks[ev.name] = append(kept, group)
	}

	perms, err := objAt(settings, "permissions", rel)
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
	for _, r := range DenyRules(store) {
		if !have[r] {
			deny = append(deny, r)
		}
	}
	perms["deny"] = deny

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return nil, errors.New("cannot write the settings: " + err.Error())
	}
	if exists && bytes.Equal(buf.Bytes(), old) {
		return nil, nil
	}
	if exists {
		// Compare meaning, not bytes, so a re-run on a hand-formatted file changes nothing.
		var a, b any
		if json.Unmarshal(old, &a) == nil && json.Unmarshal(buf.Bytes(), &b) == nil && jsonEqual(a, b) {
			return nil, nil
		}
	}
	what := "made"
	if exists {
		what = "updated"
	}
	return []change{{rel: rel, path: p, data: buf.Bytes(), perm: 0o644, what: what, note: " (hooks and deny rules)"}}, nil
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
