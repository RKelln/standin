# Standin

Let an AI work on your files without seeing your private data.

Standin swaps the private values in a file for **stand-ins**: made-up labels like `[PERSON_1]` or
`[EMAIL_2]`. It keeps a private list, the **mapping**, that links each stand-in to its real value, and later
puts the real values back into whatever the AI wrote. The same value always gets the same stand-in, so the
AI can still match a payment to an invoice. The real values stay on your computer.

It is made for small businesses and non-profits who want to use an AI coding agent (Claude Code and
similar) on their own records: invoices, bank exports, customer lists. It is the tool behind the "share
safely" step of [Get2Know](https://github.com/RKelln/get2know).

**Status:** v0.1.0, early. Tested on invented data and in Claude Code 2.1.314 on Linux. Not yet tested by
real users or on real exports. See "What it misses" below.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/RKelln/standin/main/install.ps1 | iex
```

One program, no other software needed. It never uses the network.

## How it works

```sh
standin init                 # makes raw/ shape/ scrubbed/ restored/, sets up Claude Code's guard hooks
# put your real files in raw/
standin shape                # describes each file for the AI, with no real values
# ask the AI to read shape/ and write standin.config.json
standin review               # read the rules as a table
standin approve              # you say yes to each column the AI will see for real
standin scrub                # writes scrubbed/ for the AI; the mapping goes to ~/.standin
# the AI works on scrubbed/ and writes its answer with stand-ins
standin restore answer.md    # puts the real values back, into restored/, for you
standin show PERSON_9        # prints who a stand-in is, on your screen only
```

- **Default-deny:** every value becomes a stand-in unless you approved `keep` for its column. If the
  config has a `keep` or `scan` that you have not approved, `scrub` stops and writes nothing.
- **Fail closed:** if Standin cannot read a file, or is unsure, it writes nothing and says why.
- **The mapping is the secret.** It lives outside the project folder. Never give it to an AI.
- `standin check FILE` looks for real values and private patterns, and names the stand-in, never the value.
- `standin report` prints an issue report with no data in it. Agents should use it to report problems.
- `standin show PERSON_9` (or `[PERSON_9]`) prints the real value behind a stand-in. It prints only to your
  screen, never to a pipe or an AI tool, and the guard hooks stop an AI from running it.

Files in: CSV, TSV and JSON (UTF-8, UTF-16, Windows-1252). Not yet: Excel, OFX/QFX, PDF. Save as CSV first.

### The guard hooks (Claude Code)

`standin init` adds hooks and deny rules to `.claude/settings.local.json`: settings for this computer
only. They hold this computer's path to the `standin` program. If the folder is in git, add that file to
`.gitignore`. Use `standin init --shared` to write `.claude/settings.json` instead (shared with anyone
who gets the folder). Running `init` again moves standin's own hooks out of the other file, and leaves
the rest of it alone.

- **Before each tool call**, the hook blocks reads of `raw/`, `restored/` and the mapping, and hands the
  AI the scrubbed copy of a raw file when there is an up-to-date one.
- **After each tool call**, it warns the AI if the output may hold a real value.
- **The prompt guard** stops a chat message that holds a real value Standin knows (a name from a
  tokenized column, say) from being typed or pasted into the chat. If you talk about your own records
  with the AI on purpose, turn it off with `standin init --no-prompt-guard`. It only catches whole
  values it knows: a first name alone gets through.
- The hooks find the project folder themselves, so a moved folder keeps working. The mapping is kept
  by the folder's full path, so after a move Standin tells you where the old private folder is and
  where to move it. It does not move it for you.

## What it misses

- **Names inside free text** (notes, bank descriptions) are swapped only if Standin already knows them
  from a tokenized column. Other names stay. Read scan columns before you share them.
- **Patterns:** stand-ins still show which rows share a value, and how often.
- **Combinations** (a postal code plus a job plus a date) can still point to one person.
- **The guard hooks are a safety net, not a wall.** A shell script can still read `raw/`. Hooks do not see
  files you mention with `@`, pasted text, or `CLAUDE.md`. Codex and OpenCode notes are untested.
- The AI provider still gets everything you type, and the scrubbed files.

Read `docs/DESIGN.md` for the design and its decisions, and `skill/standin/SKILL.md` for agent instructions.

MIT licence, for code and docs.
