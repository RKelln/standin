# Standin: design

Status: first draft, 2026-10-30. A proposal for the working agent, not a decision record. The agent should
check it, correct it, and record decisions in the "Decisions" section at the end.

## 1. Why

Small businesses and non-profits hold the records an AI agent could help with most: invoices, bank
exports, customer lists, email exports. They are also the records they should be most careful about. Two
bad outcomes: people paste real data into a hosted model without thinking, or they refuse to use AI at all.

Standin sits between the two. The AI works with the *structure* of the data and stand-in values; the real
values stay on the person's machine and are put back at the end.

The first user is someone running [Get2Know](https://github.com/RKelln/get2know)'s interview and its
"share safely" step, who then wants an agent to reconcile invoices against a bank export, draft reminders,
or sort emails, without exposing customer names, emails and account numbers.

## 2. Who it is for

- A non-technical owner on a locked-down laptop (no installs, no admin, no Python or Node). Opens a file in
  the browser.
- A person with an AI coding agent in a folder who wants a rule: "the agent reads `scrubbed/`, never
  `raw/`".
- Not for people who need a compliance guarantee. It is an exposure reducer.

## 3. Core idea

Two ideas carry the design.

**Shape first.** The AI never has to see a real value to make a good decision about a column. A
description of each value (its format, length, character classes) plus a summary of the column is enough
to propose "tokenize this, keep that, drop the other". This is safer than asking a detector to find
sensitive spans in real data, and it works well on structured files, where column rules beat
name-recognition models.

**Stand-ins that round-trip.** The same real value always becomes the same stand-in, so the AI can still
match a payment to an invoice. A private table maps stand-ins back. A restore step swaps them in the AI's
output.

## 4. The flow

1. **Load.** Drop a file on the page. Supported in the MVP: CSV, TSV, JSON (array of records, or object of
   records). Anything else: refuse and say "redact by hand".
2. **Shape file.** Standin writes `shape.json` (or `.md`):
   - Per column: header, inferred type, distinct count, empty count, length range, a handful of *shape
     strings* for example values.
   - A shape string describes a value, never contains it: `email-like (22 chars)`, `date YYYY-MM-DD`,
     `amount 2 decimals`, `two capitalized words`, `6 digits`, `free text (avg 80 chars)`.
   - Headers and JSON keys are shape-checked too. A header or key that looks like an email, a name or an ID
     is flagged and replaced in the shape file (see 6.2).
3. **Propose.** The person gives `shape.json` to the AI. The AI replies with a *config*: for each column,
   `tokenize` (with a category such as `person`, `email`, `phone`, `account`, `id`, `address`),
   `keep`, `drop`, or `ask a person`, plus a one-line reason.
4. **Review.** The person pastes or loads the config. Standin shows it as a plain table. Every column that
   the config marks `keep` is highlighted and needs an explicit tick. Columns the config does not mention
   are `tokenize` by default. Free-text columns default to `drop` (or `blank`).
5. **Scrub.** Standin writes:
   - `scrubbed.csv` (or `.json`): same shape, stand-ins in place of real values.
   - `mapping.json`: stand-in to real value. The secret. Saved only on an explicit click.
   - A before-and-after view of three rows for a final eyeball check.
6. **Work.** The AI reads `scrubbed.*` and does its job (reconcile, list overdue, draft reminders). Its
   replies and files contain stand-ins.
7. **Restore.** The person pastes text, or loads a file the AI wrote. Standin swaps stand-ins for real
   values using `mapping.json` and shows the result. Nothing is uploaded.

## 5. Stand-in rules

- One counter per category, keyed on the normalised value: `Customer_1`, `Customer_2`; `[EMAIL_1]`.
  Normalise case, surrounding whitespace and obvious punctuation before keying, so `Ann Lee` and `ann lee `
  are the same stand-in.
- **Delimit or order tokens** so restore cannot break `Customer_1` inside `Customer_12`: use a closing
  delimiter (`Customer_1`, `Customer_12` with word boundaries) or replace longest-first.
- Format-preserving options are out of scope for the MVP (stand-ins are visibly stand-ins, so a person
  sees at a glance what is replaced).
- Keep amounts and dates only if the config says `keep`. Offer coarse options later (round amounts, shift
  dates by a fixed offset) and note that these change results.
- Consistent stand-ins leak structure: frequency and relationships remain visible. State this in the
  interface.

## 6. Safety design

### 6.1 Default-deny and review

A wrong `keep` leaks data; a wrong `tokenize` only costs usefulness. So every value is tokenized unless an
approved config says keep, and the person approves each keep.

### 6.2 Headers, keys and file names are not automatically safe

A wide table can have customer names as columns. JSON keys can be data (`{"Acme Ltd": {...}}`). File names
can be too. Shape-check them like values and flag the ones that look like an email, name or ID. Tell the
person to rename the file before handing it to anything.

### 6.3 Free text

Column rules cannot handle an email body or a notes field. MVP behaviour: default `drop`; the person may
choose `regex pass` (emails, phone numbers, long digit strings, account-number patterns, postal codes) and
`known-values pass` (replace any value that already appears in a tokenized column, such as customer names,
wherever it occurs in the text). State plainly in the interface that names not in the known list will
remain.

### 6.4 Fail closed

Unsupported or unparseable input: no output, a clear message. No silent best effort.

### 6.5 The AI side

- The agent reads `scrubbed/` only. Enforce it with the agent's settings (deny reads on the raw folder),
  not with trust. Provide a ready settings snippet for Claude Code and a line for `AGENTS.md`.
- An optional hook (rewrite tool results, block raw values in prompts, fail closed) is a later phase.
  Prior art lists its own leak paths: a shell command can see real values; `@file` mentions, `CLAUDE.md`,
  memory files and compaction summaries are not covered; a crashed hook can let the original through. Use
  that list as a test checklist before claiming anything about a hook.
- The mapping table must not sit in a folder the agent can read. Tell the user where to keep it.

### 6.6 What it still leaks

- Names inside free text that are not in any known column.
- Re-identification from combinations (role, postal code and date together). Not addressed; say so.
- Structure and frequency, as in section 5.
- The provider still receives everything the person types and the scrubbed data.

The interface should say these in a short "What this does not do" panel.

## 7. MVP

One HTML file, opened from a folder in a browser. No build, no network, no installs.

Includes: CSV, TSV and JSON in; shape file out; config review table with default-deny; scrub; mapping
save; before-and-after view; restore from pasted text or a loaded file; refuse unsupported files; a short
"what this does not do" panel; a settings snippet for Claude Code.

Excludes: name recognition models, PDF, images, Word, an agent hook, format-preserving stand-ins,
accounts, anything hosted.

### Success checks

1. A synthetic invoices file and bank export (provided as fixtures) go through shape, config, scrub,
   restore, and the restored text equals the original values.
2. No real value from a tokenized column appears in `shape.json` or `scrubbed.*` (automated string search
   against the fixtures' known values).
3. An AI given only `shape.json` proposes a config that, after human review, tokenizes every sensitive
   column in the fixtures. Run this with `rehearse` or by hand with a small model; record the result.
4. `Customer_1` and `Customer_12` restore correctly.
5. A file with person names as headers is flagged.
6. A PDF is refused with a clear message.
7. The page makes no network requests (check in browser devtools and with a content security policy that
   forbids them).
8. Works opened from a plain folder in Chrome and Safari, with the browser offline.

## 8. Prior art (short pass, 2026-10-30; unverified claims marked)

| Project | Notes |
|---|---|
| Microsoft Presidio | MIT, Python. Analyzer plus anonymizer; reversible `encrypt`/`decrypt` operators (from memory, confirm). `presidio-structured` for pandas/JSON. Heavy (spaCy model); needs Python. |
| LLM Guard (Anonymize/Deanonymize with a Vault) | MIT, Python. Archived by its owner July 2026. Useful as a shape: analyzer, vault, restore. |
| mloda-pii | Proprietary, source-available, licence key. A Claude Code hook that rewrites tool results with keyed tokens and a local vault. Requires Python and spaCy. Its stated leak paths are a good test checklist. Do not depend on it. |
| noirdoc, shutapp, PrivAiTe | Hooks and proxies for Claude Code and LLMs. shutapp reportedly sends text to a hosted API, which rules it out. Licences and repos unconfirmed. |
| RemoveMyID | MIT, React/TS. Browser redaction for text, CSV, PDF, OCR. Immature (1 star), needs a Node build, no mapping export documented. |
| scrubadub | MIT, Python (from memory). Irreversible tags. Status unconfirmed. |
| Browser NER tools (PresidioJS, Cloak, Rampart, privatiser) | Download 15 to 67 MB models. Claims unconfirmed. |

The agent should verify the top rows at their primary sources before building on them, and check for
anything the pass missed (for example tools aimed at CSV column anonymisation, and k-anonymity checks).

Takeaways: copy the *shape* of Presidio and LLM Guard (finder, applier, vault, restore), not their code;
a header-to-category map beats name recognition on structured files; no single-file browser tool with a
round-trip exists, so this is worth building.

## 9. Open questions

1. Name. "Standin" is a working title; change it freely.
2. Licence. Proposed: code MIT, docs CC BY 4.0, as Get2Know.
3. JSON shapes beyond arrays of flat records (nested objects, arrays inside records). How far in the MVP?
4. Shape strings: how fine? Too fine can fingerprint a value (length plus pattern); too coarse stops the AI
   judging. Decide with a test.
5. Should the AI's config be JSON the page validates, or a table the person ticks by hand? Likely both:
   the AI writes JSON, the page shows it as a table.
6. Where does restore apply: pasted text only in the MVP, or also whole files (Markdown, CSV, HTML the AI
   wrote)?
7. Integration with Get2Know's `share-safely`: does it call Standin for tabular files, or only link to it?
8. Dates and amounts: offer coarsening (round, offset) in the MVP or later?
9. A person-friendly name for "tokenize" and "stand-in" in the interface.

## 10. Phases

1. MVP as in section 7.
2. Free-text helpers, nested JSON, coarsening options.
3. Claude Code hook and settings pack, tested against the leak-path checklist.
4. A `share-safely` hand-off from Get2Know.

## Decisions

(The working agent records decisions and why here. Evidence: `docs/research/prior-art.md`. The contract
is in `docs/PLAN.md`; where this file and PLAN differ, PLAN wins.)

1. **A CLI, not an HTML page** (Ryan, 2026-10-30). Agent harnesses call it from hooks; setup is hands-on.
   Sections 4, 7 and success checks 7-8 describe the old page; the CLI keeps the same flow (shape, config,
   approve, scrub, restore). Check 7 becomes "no network package is linked" (a test on `go list -deps`).
2. **Go, standard library only, one static binary.** Easy cross-builds for macOS, Windows and Linux; no
   runtime to install. Rust needs a toolchain we do not have here; Python may be an installer stub on a
   locked-down laptop. Release pipeline copied from Ryan's agentmap.
3. **Name `standin`, textmark `[standin]`; MIT for code and docs** (Ryan).
4. **Stand-in format `[CATEGORY_N]`.** One shape for every category. The closing bracket means
   `[PERSON_1]` cannot match inside `[PERSON_12]`, and a stand-in is plainly not a real value.
5. **The secret lives outside the project** (`~/.standin/projects/...`, folders 0700, files 0600), so
   tools working in the project folder do not see it. Every command prints where it is.
6. **Normalise for the key, keep every spelling.** `Ann Lee` and `ann lee ` share one stand-in, so files
   still join up. The mapping keeps all spellings; restore writes the first and says how many had more.
   (Research finding 9: a single spelling would make exact round-trips fail.)
7. **Refuse data that already looks like a stand-in.** Otherwise restore turns it into a real value (seen
   in PrivAiTe).
8. **Restore is strict.** Exact stand-ins only, longest first; near-misses are reported, never changed.
   Fuzzy matching (LLM Guard allows 3 edits) would swap people.
9. **Coarse shape bands** (open question 4). Counts and lengths as bands; nothing below 5 exact; no real
   values. Shapes are for setup only; day to day the agent sees scrubbed files only.
10. **Known-values scan in the MVP.** Bank descriptions and notes hold customer names; replacing every
    value already in the mapping catches many. Names not in the mapping stay, and the output says so.
11. **Nested JSON in the MVP** (open question 3): dotted paths and `[]`. Object-of-records keys are data.
12. **Restore works on files and stdin** (open question 6), writing to `restored/` for the person.
13. **The hook is the main entry point, not a later phase** (changes 6.5). Agent deny settings alone do not
    stop shell reads. Claude Code hooks fail open by default, so every Standin hook sets
    `"onFailure": "block"`, uses exec form and exits 2 to block; `standin init` refuses older Claude Code
    versions that lack it. Bash reads stay a known gap except under a sandbox; say so.
14. **Accounting and bank exports before xlsx** (Ryan). After CSV and JSON: OFX/QFX/QBO (SGML 1.x and
    XML 2.x), then QuickBooks exports. Preamble rows above a bank CSV header are data and default-deny.
15. **Installs by `curl`/`irm`, unsigned for now.** A browser download on macOS is quarantined; the curl
    path is not. Windows Smart App Control may block unsigned binaries: open item for Ryan (paid signing).
16. **Agents report issues without data.** `standin report` prints version, OS, file kinds and shapes,
    never values, so a leak or a format gap gets fixed once.
17. **Hook blocks are exit 2 plus a stderr reason, never JSON.** A change in the JSON format cannot then
    turn a block into an allow. Only the Read redirect writes JSON, and only when the scrubbed copy is
    strictly newer than the raw file.
18. **The agent may run `standin shape|scrub|check|review|report|version`; only the person runs `restore`
    and `approve`.** The Bash hook lets a single plain `standin` call name `raw/` (no shell metacharacters).
    `restore -` refuses when stdout is not a terminal, so real values do not land in a transcript.
19. **PostToolUse warns on known values only**; patterns fire too often in tool output.
20. **`init --no-version-check`** is an escape hatch for unreadable `claude --version` output; it exits 1
    with a loud warning. `init` advises, but does not turn on, Claude Code's sandbox (session-wide;
    needs bubblewrap on Linux).
21. **Tested in real Claude Code 2.1.314 (2026-10-30, `tools/claude_smoke.sh`):** `onFailure: "block"`
    blocks a broken hook; the Bash hook blocks `cat raw/...`; a prompt holding a real value is not sent;
    the Read redirect to `scrubbed/` works. But the `Read(./raw/**)` deny rule fires *before* the hook, so
    with it in place a raw read is denied, not redirected. We keep the deny rule: it still holds if hooks
    are turned off, and the agent can read `scrubbed/` directly. `AGENTS.md` alone already made a small
    model refuse to read `raw/`.
22. **Only `init` may run another program** (`claude --version`). A test checks no other package links
    `os/exec`, and that no network package is linked at all.
23. **Success check 3 passed (2026-10-30):** a small model (Haiku) given only the two shape files wrote a
    config that tokenized or dropped every private column (names, emails, phones, accounts), scanned the
    free text, and kept only dates, amounts and status.

### From the fresh review (2026-10-30)

24. **`scan` needs approval, like `keep`.** It leaves unknown words in, so it is a kind of keep. Without a
    matching approval a `scan` column is tokenized. `approve` asks for each `scan` too.
25. **Agent-run `standin` calls take no `--private` or `--project` flags**, and `approve`, `restore` and
    `init` are blocked from the agent's shell wherever they appear in a command. A pseudo-terminal can fake
    the TTY check, so the hook is what stops an agent approving its own config.
26. **Detectors give the agent at most one bit.** When stdout is not a terminal, `check` says only whether it
    found something, with no stand-in names or line numbers, and tells the agent to ask the person to run it.
    The PostToolUse warning names no stand-in either. Otherwise the agent could test guessed names.
27. **Shell habits that sweep the whole project are blocked.** The Bash hook denies glob characters,
    brace lists and recursive tools (`grep -r`, `rg`, `find`, `tree` ...) unless every path in the command
    is under `scrubbed/` or `shape/`. Still a guess; the sandbox is the wall. Docs say so more strongly.
28. **No `keep` or `scan` on `[COLUMN_n]`.** A position-based keep would follow a re-ordered export.
29. **Restore leaves stand-ins inside URLs and formulas alone** (a cell starting `= + - @`, or text inside
    `http://`, `https://`, `](`, `src=`), and reports them (exit 1). Otherwise an AI-built link could send a
    real value out when the file is opened.
30. **Dropped values join the known list**, so a name dropped from one column is still swapped in notes.
31. Smaller fail-closed fixes: refuse lone-CR line endings; no data-like file names in any message or
    report (extension included); refuse two restores to the same output name; header checks match between
    shape and scrub; headers equal to a known value are swapped; all-caps names, `O'`/`Mc` names and
    no-break spaces are caught. Known misses kept: Unicode normal forms (no NFC in the standard library)
    and Turkish dotted I.

### From the Get2Know trial (2026-10-30, `docs/get2know-needs.md`)

32. **The prompt hook stays on, with an opt-out.** In general use it stops a pasted customer list or a
    known value typed into the chat. Where a person talks about their own records on purpose (an
    interview), it gets in the way and catches only full known values, so `init --no-prompt-guard` leaves
    it out. Its block message names that flag.
33. **`scrub` refuses until the config is approved** (exit 2, "run standin approve"), when the config has
    any `keep` or `scan`. Replaces the "tokenize as `VALUE` and warn" fallback: those whole-cell stand-ins
    joined the known list, and a later approved scrub swapped whole descriptions in scan columns.
34. **Hooks find the project themselves.** `init` writes no `--project`; the hook uses
    `CLAUDE_PROJECT_DIR`, then the hook input's `cwd`, walking up to the project, and still fails closed
    if none is found, naming the fix. A moved folder keeps working. The private folder is still keyed by
    path; when it is missing and one for a same-named project exists, commands say how to move it.
    Built order: `CLAUDE_PROJECT_DIR`, then an old `--project` (from 0.1.0 settings) if that folder still
    exists, then `cwd` walking up. A gone `--project` is skipped, not a block. `CLAUDE_PROJECT_DIR` comes
    first because Claude Code sets it to the folder whose settings it loaded (hooks docs), which also
    covers a copied folder whose old `--project` still exists. The move hint is a warning (exit 1), so two
    real projects that share a folder name see it only until the second one has its own private folder.
35. **`standin show STANDIN...`** prints real values for the person, on a terminal only (like `restore -`).
    The Bash guard blocks it for agents.
36. **Hooks and deny rules go in `.claude/settings.local.json` by default.** They hold this machine's
    absolute binary path, and a shared `settings.json` may be shipped and updated by someone else.
    `init --shared` writes `settings.json` instead.
    Checked 2026-10-30 at https://code.claude.com/docs/en/settings and /hooks: `settings.local.json` is the
    "project local" scope, above `settings.json`; list settings such as `permissions.deny` merge across
    files; deny rules apply without the trust step; hooks are read from it ("Hook locations" table) and
    merge across files, and a handler defined in two files runs once. Tested: `tools/claude_smoke.sh`
    passes with the hooks and deny rules only in the local file (Claude Code 2.1.314). When `init` writes
    one file it takes standin's own hooks (any event, any binary path) out of the other and leaves
    everything else; the file is re-encoded, so key order and spacing may change but the JSON means the
    same, and an untouched file is not rewritten. It leaves standin's old deny rules in `settings.json`:
    they only add protection. Claude Code does not git-ignore a local file it did not create, so `init`
    says to add it to `.gitignore`. Docs note: in a git repository Claude Code also reads the local file
    at the repository root; it still reads one in the starting folder, which is where `init` writes it.
37. **Messages name project-relative paths only**, so a long scratch or home path cannot trip the
    data-like-name check. A hidden name reads "restored/(name hidden)".
    The trial's message had another cause: the file-name check read `answer.md` as two lower-case words
    (`answer`, `md`), which looks like a name. A common extension (the report's list) is no longer read
    as a word; an uncommon one is still checked ("statement.Zelda Quorn"). Only the folder or file name
    that looks like data is hidden, so the rest of the path still shows. A file outside the project is
    named by its own name only. Still absolute in messages, on purpose: the project and private folder
    lines (`init`, the "Private folder" line, the move hint).
38. **Free text (`.md`, `.txt`): approve words, not columns** (research and numbers:
    `docs/research/free-text.md`). Tables and `Label: value` lines are pulled out first and use the
    column rules. In the rest, candidate words (known values, patterns, capitalised words not in an
    embedded English list, words in name spots, first-name list hits) become `[NAME_n]` unless the person
    keeps them in `approve`, on a terminal. The agent sees counts only; `review` shows counts for text.
    Word decisions live in the private folder (`words.json`, as secret as the mapping) and are hashed into
    the approval. New words since approval become stand-ins and scrub exits 1. Lists: SCOWL size 60
    (lower-case entries only; its capitalised entries include surnames) and Ontario plus Quebec first
    names, gzipped with `go:embed`, notices in `THIRD_PARTY.md`. `.eml`/`.mbox` later, hand-parsed
    (`net/mail` links `net`). First numbers: 22-27 review words on a small shop folder; about 150 on an
    invented 200-email inbox, of which about 30 are words a person keeps.
    Built 2026-10-30; details and measured numbers in decisions 39-45 below.

### Free text, as built (track F, 2026-10-30; contract: `docs/PLAN.md` "Free text")

Decision 38 is built as the research's "smallest useful slice". Details decided while building:

39. **A `.txt` is free text unless it parses as a table** (CSV or TSV, 3 or more rows, 2 or more
    columns, every row the same width): then it is refused, "rename it to .csv or .tsv", as the research
    advised. A real CSV parse, so quoted commas do not hide a table. Known: three short lines with one
    comma each also read as a table, so such a note is refused (the safe side; a test records it).
40. **Fields and tables.** A `Label: value` line is a field only when every label word is a common
    header word or an English word that is not also a first name. So a chat line ("Leila: see you
    then", "Mark: ok") stays text and gets the word check, instead of hiding the name as a label.
    A table header that looks like data is tokenized and cannot be named in the config (no
    `[COLUMN_n]` for tables: with several tables, n would be unclear). A field `scan` gets the word
    check; a table `scan` is the CSV scan (known values and patterns), as the research said. Field
    parts (decision 38) come only from field values that do not read like free text: a dropped prose
    field made "Is", "What" and "How" candidates on the trial folder.
41. **One home for each choice.** Day and month names are kept by the config (`keep_groups`), which the
    config hash already covers; `words.json` holds only `keep` and `swap` (the research sketch also had
    `groups` there). `words_sha256` is the hash of the whole `words.json` as approve wrote it, so any
    edit voids the approval, as the brief asked. Scrub never writes `words.json`: a new word stays
    "new" (swapped, exit 1) until the person runs approve, which asks only about new words.
42. **A word kept after it was swapped** keeps its mapping row, marked `"kept": true`, so old stand-ins
    still restore, while the known-values pass, `check` and the hooks stop looking for it. A one-word
    `NAME` value is not a known-value hit inside the word check: it goes through the word rules again,
    so a kept word, or a day name the config keeps, is not swapped just because it was swapped
    elsewhere. A v0.1 binary refuses a mapping with `kept` in it (unknown field; fail closed).
    A one-word `NAME` that is also an English word ("May", "Supply", "Rose") is not a known value at
    all: known values match in any case, so a CSV scan, `check`, the PostToolUse warning and the prompt
    guard would fire on everyday words. The word check alone swaps it (in `.md`/`.txt`). Cost: such a
    name in a CSV scan column is not swapped by being known. Other swapped names ("Sam", "Oksana")
    are known values, so the prompt guard now also stops them (decision 32's opt-out still applies).
    The hooks load the word lists (about 15 ms) only when the mapping has `NAME` rows.
43. **Finder rules beyond the prototype:** titles (Mr, Mrs, Ms, Mx, Dr, Mme, M., Prof) are name spots;
    a word right after a caught name counts only with nothing but spaces between; a first name typed in
    lower case is a candidate when it is 3 or more letters, not an English word, not a day name, and
    not right after `. / @ _` ("thx kavya" yes, "price-sheet.md" no); a word in a script with no capitals
    is a candidate (unusual group); a file with more than 20% of its letters (and at least 10) in such
    a script is refused. With no `text` rule the text is dropped (default-deny), with a warning.
44. **Approve shows words only on a screen.** With a word check, `approve` needs stdout to be a terminal
    as well as stdin. Context lines mask other candidates, known values and patterns as `▒▒▒`. Enter
    or `none` keeps nothing; three unreadable answers refuse. `review` (the agent may run it) shows
    bands only, including "new since the last approve". `words.json` joined the hook's secret file
    names. `standin licences` prints the list notices, since `install.sh` keeps only the binary.
45. **Known values are found by a 3-letter index, not one joined regexp.** With about 1,500 values (a
    1000-email inbox), the joined regexp took 7 s per scrub; the index takes 0.2 s, with the same output
    on every sample. Also fixed: a known value whose first spelling ends in `.` ("Harbourline Courier
    Co.") now takes that dot into the hit, so restore no longer writes "Co..".

**Numbers from the build** (Go, same lists as the research; invented data only). Review words (new
words a person is asked about, both groups) on copies of the trial's bike-shop folder: 19 (clean) and
26 (messy), against the research's 22 and 27; none of the 5 people missed. Generated inbox (research
generator, invented names), config `From` tokenize, `Date` keep, `Subject` scan, text words: 200 emails
gave 82 review words (23 unusual, nearly all brands and places, and 59 name-like) and 16 day names;
keeping all 23 unusual words, 98% of name words were swapped (left: a brand that is also a surname, a
hyphenated name the run kept, and two names typed in lower case that are also words). 1000 emails: 87
review words; one name word left ("faith", in lower case). Binary 3.43 MB to 3.92 MB (stripped).
