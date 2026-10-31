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

One program, no other software needed. Standin does not open network connections. The installer
downloads the program. During setup, `init` runs your local `claude --version`; that other program is
outside Standin's control.

## How it works

```sh
standin init                 # makes raw/ shape/ scrubbed/ restored/, sets up Claude Code's guard hooks
# put your real files in raw/
standin shape                # describes each file for the AI, with no real values
# ask the AI to read shape/ and write standin.config.json
standin review               # read the rules as a table
standin approve              # you say yes to each column the AI will see for real, and pick words to keep
standin scrub                # writes scrubbed/ for the AI; the mapping goes to ~/.standin
# the AI works on scrubbed/ and writes its answer with stand-ins
standin restore answer.md    # puts the real values back, into restored/, for you
standin show PERSON_9        # prints who a stand-in is, on your screen only
```

- **Default-deny:** every value becomes a stand-in unless you approved `keep` for its column. If the
  config has a `keep` or `scan` that you have not approved, `scrub` stops and writes nothing.
- **Fail closed:** if Standin cannot parse a file, or is unsure about its rules, it writes no output
  files and says why. A failed or interrupted save may need recovery; see below.
- **The mapping is the secret.** It lives outside the project folder. Never give it to an AI.
- Use plain file names such as `invoices.csv`. If a name looks private, `shape` stops and asks you
  to rename it before writing any shape files.
  If an older version made anonymous shape files, clear those old files before sharing the folder.
- `standin check FILE` looks for real values and private patterns, and names the stand-in, never the value.
- `standin report` prints an issue report with no data in it. Agents should use it to report problems.
- `standin show PERSON_9` (or `[PERSON_9]`) prints the real value behind a stand-in. It prints only to your
  screen, never to a pipe or an AI tool, and the guard hooks stop an AI from running it.

Files in: CSV, TSV and JSON (UTF-8, UTF-16, Windows-1252), free text in `.md` and `.txt`, and one
email message per `.eml` file. Not yet: Excel, OFX/QFX, PDF, mailbox files that hold several messages
(`.mbox`, `.msg`). Save as CSV, or paste the messages into a `.md` file, first.

CSV and TSV headers must be different. So must the headers within each Markdown table. JSON field
names cannot be blank or contain dots or square brackets, or be `(key)`: those marks name nested
fields in the config. Rename such fields before using Standin. Top-level record names in an object
of records are data and may still contain dots, such as in an email address.

Restore also checks JSON and tables before writing. A broken file is refused; save an unfinished
fragment as `.txt` to restore it as plain text. If putting a value back would create a formula,
web link or HTML tag, Standin leaves the stand-in and warns you. Read every restored file before
opening it in a spreadsheet or browser. This is not a general check for safe HTML or formulas. A value
that is itself a web link (a URL pasted into notes, say) is left as a stand-in in every restored copy:
fill it in by hand where you need it.

### Free text (`.md` and `.txt`)

- `Label: value` lines (`From:`, `Date:`, `Attendees:`) are **fields**, and Markdown tables are
  **columns**. They follow the same rules as CSV columns.
- The rest gets the **word check**: Standin picks out words that may be private (a *candidate* is
  such a word): words that are not English words, words after Hi, Dear or Thanks, sign-offs, first
  names, emails, phone numbers and values it already knows. Each one becomes a stand-in like
  `[NAME_1]`, unless you keep it. Other words stay. Markdown marks stay as they are.
- The AI never sees a candidate word. The shape file and `standin review` give counts only.
- `standin approve` shows you the new words on your screen, 20 at a time, each with a short piece of
  the text: first the unusual words (brands, streets, short forms: most words worth keeping are
  here), then the words that look like names. You type the numbers of the words to keep. Your
  choices go in `words.json` in the private folder. It holds names, so it is as secret as the
  mapping. If it changes, approve again.
- A word that is new since you last approved becomes a stand-in, and scrub says how many (exit 1).
- A `.txt` that is really a table is refused: rename it to `.csv`. A file mostly in a script with no
  capital letters (Chinese, Arabic ...) is refused: Standin cannot find names in it.

### Email (`.eml`)

- **One message per file.** Standin reads the message and shows the AI a plain-text view: `From`, `To`,
  `Cc`, `Date` and `Subject` at the top, then the message text. Those five take the same rules as any
  other field.
- When a message offers plain text and HTML as alternatives, Standin uses the plain text. Separate
  body parts and forwarded messages are kept in order. HTML is turned into text: fonts, styles,
  scripts, links and pictures are dropped.
- **Attachments are never opened.** The shape file says how many were there.
- `.mbox` and `.msg` files hold several messages, so they are refused. Save one message per `.eml`, or
  paste the messages into a `.md` file.
- **The scrubbed copy is a view, not your original.** Your `.eml` is never changed. `restore` puts the
  real values back into that same view. The formatting, the MIME structure and any attachment stay in
  your `raw/` copy only.

### The guard hooks (Claude Code)

`standin init` adds hooks and deny rules to `.claude/settings.local.json`: settings for this computer
only. They hold this computer's path to the `standin` program. If the folder is in git, add that file to
`.gitignore`. Use `standin init --shared` to write `.claude/settings.json` instead (shared with anyone
who gets the folder). Running `init` again moves standin's own hooks out of the other file, and leaves
the rest of it alone.

- **Before each tool call**, the hook checks known file and shell tools, blocks reads of `raw/`,
  `restored/` and the mapping, and hands the AI a scrubbed copy when one is up to date. Unknown tools,
  including new plug-in tools (MCP), are blocked until Standin knows how to check them. Conversation and task
  list tools may run.
- **After each tool call**, it warns the AI if the output may hold a real value.
- **The prompt guard** stops a chat message that holds a real value Standin knows (a name from a
  tokenized column, say) from being typed or pasted into the chat. If you talk about your own records
  with the AI on purpose, turn it off with `standin init --no-prompt-guard`. It only catches whole
  values it knows: a first name alone gets through.
- The hooks find the project folder themselves, so a moved folder keeps working. The mapping is kept
  by the folder's full path, so after a move Standin tells you where the old private folder is and
  where to move it. It does not move it for you.

## What it misses

- **Names inside CSV and JSON free text** (`scan` columns: notes, bank descriptions) are swapped only if
  Standin already knows them from a tokenized column. Other names stay. Read scan columns before you
  share them.
- **The word check on `.md` and `.txt` misses:** a name that is also a common word, typed in lower
  case (rich, grace, will) or only at the start of a sentence ("Rose called"); a surname that is a
  word, with no first name or title next to it; business names made of words (City Hardware); names
  inside links and handles (`@grace_lee`, `/people/ana-vantreeck`) unless Standin already swapped that
  name elsewhere; and a person told apart by a story or a rare detail. For
  case notes and other text about people's lives, `drop` the text. The built-in word list is English:
  a French message has most of its words offered as candidates, so expect to keep a lot, or `drop` the
  text. Tested on invented text only.
- **Email:** a message that is only HTML loses its links and pictures. An attachment is never opened,
  so a receipt sent only as an attachment is invisible to the agent. `From` holds the name and the
  address as one value, so the same person can be one stand-in there and another in the message text.
- **First names in text and full names in columns are not linked.** "Leila" in an email becomes
  `[NAME_3]`, while "Leila Haddad" in a column is `[PERSON_1]`. The AI cannot tell they are the same
  person. Linking them would also merge two different Leilas, so Standin does not.
- **"It's May" about a person** is read as the month. Names after "Hi", "Dear" or "name is" are still caught.
- **Headers:** a column header that is a first name and also a common word (Grace, Rose) still shows
  in the shape file.
- **File names:** names Standin flags, or names that repeat a private value it knows, must be renamed.
  Other names can still get through. Use plain names such as `invoices.csv` before you start.
- **Patterns:** stand-ins still show which rows share a value, and how often.
- **Combinations** (a postal code plus a job plus a date) can still point to one person.
- **The guard hooks are a safety net, not a wall.** A shell script can still read `raw/`. Hooks do not see
  files you mention with `@`, pasted text, or `CLAUDE.md`. Codex and OpenCode notes are untested.
- The AI provider still gets everything you type, and the scrubbed files.

### If a save stops early

Standin allows one run at a time to change a project's mapping or word choices. A second run waits
briefly, then asks you to try again. If a run crashes, `.standin-lock` may remain in the private folder.
Standin never removes a lock just because it looks old.

If you see `pending.json`, a save did not finish. Stop using the scrubbed files and any answers made
from that run. Restore the private folder and scrubbed files from the **same backup** before clearing
the marks. Previous private files are kept as `.standin-previous-*`; these are secrets too, and are
not a full backup of the project. Ask for help if you do not have a matching backup. If there is only
a lock, first make sure no Standin run is open, then check the folder before removing it yourself.
Do not ask an AI to read the private folder. Copying or sharing the project does not copy its mapping.

The save checks cover ordinary write errors and interrupted processes. They do not promise recovery
from power loss or a broken disk. Back up the private folder securely along with the files it matches.
On Windows, Standin sets private files to allow only your Windows account; native Windows testing is
still needed. A terminal check cannot prove that a person is present: software can make a fake terminal.

Read `docs/DESIGN.md` for the design and its decisions, and `skill/standin/SKILL.md` for agent instructions.

MIT licence, for code and docs. The built-in word lists (SCOWL, Ontario and Quebec baby names) have
their own notices: see `THIRD_PARTY.md`, or run `standin licences`.
