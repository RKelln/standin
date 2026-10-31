package initcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	agentsBegin = "<!-- standin:begin (standin init writes this block; edit outside it) -->"
	agentsEnd   = "<!-- standin:end -->"
	agentsMark  = "<!-- standin:begin"
)

const agentsBlock = agentsBegin + `
## Private data: standin

This project uses standin to keep private details away from AI agents.

- Read files in ` + "`scrubbed/`" + ` only. In them, real values are swapped for stand-ins like ` + "`[PERSON_1]`" + `.
  A stand-in is a made-up label that takes the place of a real value.
- Never read, search, list or change ` + "`raw/`" + ` or ` + "`restored/`" + `. They hold real private details.
- The mapping is the secret: the list that turns stand-ins back into real values. It lives outside this
  folder. Never look for it, read it or ask for it.
- Keep every stand-in exactly as it is in anything you write. The person puts real values back with
  ` + "`standin restore`" + `.
- Never run ` + "`standin approve`" + `, ` + "`standin restore`" + `, ` + "`standin show`" + ` or ` + "`standin init`" + `, not even
  through another program. Only the person runs them. Never pass ` + "`--private`" + ` or ` + "`--project`" + ` to standin.
- Do not use wildcards or whole-folder searches outside ` + "`scrubbed/`" + `. Name files in ` + "`scrubbed/`" + `,
  like ` + "`grep -rn word scrubbed/`" + `.
- New file? Read its shape file in ` + "`shape/`" + ` and propose rules in ` + "`standin.config.json`" + `.
  The person runs ` + "`standin review`" + ` and ` + "`standin approve`" + `, then ` + "`standin scrub`" + `.
- Problem (a real value got through, a file will not scrub)? Stop. Ask the person to run
  ` + "`standin report`" + `, and describe what happened in words. Put no names, numbers or other data
  from the files in the description.
` + agentsEnd + "\n"

// planAgents adds or refreshes the standin block in AGENTS.md.
func planAgents(proj string) ([]change, error) {
	rel := "AGENTS.md"
	p := filepath.Join(proj, rel)
	old, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read AGENTS.md: " + err.Error())
	}
	s := string(old)
	var next string
	switch n := strings.Count(s, agentsMark); {
	case err != nil:
		next = agentsBlock
	case n == 0:
		next = s
		if next != "" && !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		if next != "" {
			next += "\n"
		}
		next += agentsBlock
	case n == 1 && strings.Count(s, agentsEnd) == 1:
		i := strings.Index(s, agentsMark)
		j := strings.Index(s, agentsEnd)
		if j < i {
			return nil, errors.New("AGENTS.md has the standin end mark before the start mark. Fix it by hand.")
		}
		end := j + len(agentsEnd)
		if end < len(s) && s[end] == '\n' {
			end++
		}
		next = s[:i] + agentsBlock + s[end:]
	default:
		return nil, errors.New("AGENTS.md has a broken or doubled standin block. Remove it by hand, then run standin init again.")
	}
	if next == s {
		return nil, nil
	}
	what := changeUpdate
	if err != nil {
		what = changeMade
	}
	return []change{{rel: rel, path: p, data: []byte(next), perm: 0o644, what: what, note: " (standin block)"}}, nil
}

func codexNote(proj string) string {
	return `# Codex: standin settings (UNTESTED)

Nobody has tried these with Codex yet. They come from the Codex docs, read on 2026-10-30.
Check them before you trust them.

## What works and what does not

- standin has no Codex hook yet. ` + "`standin hook claude`" + ` reads Claude Code's input, not Codex's.
- Codex reads files with shell commands, so a hook can only guess from the command text.
- Codex hooks let a call through when the hook fails. There is no switch to make them block.
- So for Codex, the folder rule below matters most. Codex enforces it in its sandbox, for every
  program it runs (when the sandbox is on).

## Folder rule (permission profile, beta)

Add this to your Codex config (` + "`~/.codex/config.toml`" + ` or the project's ` + "`.codex/config.toml`" + `).
If ` + "`sandbox_mode`" + ` is set anywhere, Codex ignores ` + "`default_permissions`" + `, so remove it.

` + "```toml" + `
default_permissions = "standin"

[permissions.standin]
extends = ":workspace"

[permissions.standin.filesystem.":workspace_roots"]
"raw" = "deny"
"restored" = "deny"
"**/mapping.json" = "deny"
` + "```" + `

The secret mapping lives outside the project, so the workspace rule above does not cover it. Codex
should not reach it unless you give it access to your home folder.

On Linux, WSL and Windows, a file added to raw/ after a program starts may not be blocked for that
program (Codex docs).

Project folder: ` + proj + `
`
}

func opencodeNote(exe string) string {
	return `# OpenCode: standin settings (UNTESTED)

Nobody has tried these with OpenCode yet. They come from the OpenCode docs and its plugin types
(@opencode-ai/plugin 1.19.5), read on 2026-10-30. Check them before you trust them.

## Permission rules

Put this in ` + "`opencode.json`" + ` in the project. The last rule that matches wins, so keep
the "*" rule first. These rules only cover OpenCode's own tools. A shell command or script can
still read raw/. OpenCode has no sandbox that we found.

` + "```json" + `
{
  "permission": {
    "read": { "*": "allow", "raw/**": "deny", "restored/**": "deny", "**/mapping.json": "deny" },
    "edit": { "*": "allow", "raw/**": "deny", "restored/**": "deny", "**/mapping.json": "deny" },
    "grep": "ask",
    "glob": "ask",
    "bash": { "*": "ask" }
  }
}
` + "```" + `

## Plugin sketch (not built, not tested)

An OpenCode plugin could call standin before each tool, the same way the Claude Code hook does.
This is a sketch only. How a throwing or slow plugin behaves is not documented.

` + "```js" + `
// .opencode/plugin/standin.js  (SKETCH, UNTESTED)
import { spawnSync } from "node:child_process";
const STANDIN = ` + jsString(exe) + `;
const NAMES = { read: "Read", edit: "Edit", write: "Write", grep: "Grep", glob: "Glob", bash: "Bash" };
export const Standin = async ({ directory }) => ({
  "tool.execute.before": async (input, output) => {
    const tool = NAMES[input.tool];
    if (!tool) return;
    const a = output.args || {};
    const ti = tool === "Bash" ? { command: a.command } :
               tool === "Grep" || tool === "Glob" ? { pattern: a.pattern, path: a.path } :
               { file_path: a.filePath };
    const r = spawnSync(STANDIN, ["hook", "claude", "PreToolUse", "--project", directory], {
      input: JSON.stringify({ hook_event_name: "PreToolUse", cwd: directory, tool_name: tool, tool_input: ti }),
      encoding: "utf8", timeout: 30000,
    });
    if (r.status === 0 && r.stdout) {
      const out = JSON.parse(r.stdout).hookSpecificOutput || {};
      if (out.updatedInput && out.updatedInput.file_path) output.args.filePath = out.updatedInput.file_path;
      return;
    }
    if (r.status !== 0) throw new Error(r.stderr || "standin blocked this (no reason given)");
  },
});
` + "```" + `
`
}

func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}
