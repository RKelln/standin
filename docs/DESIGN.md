# Standin: design

Status: design history and decisions, updated 2026-10-31. `docs/PLAN.md` is the current implementation
contract. The initial proposal below supplies background; later numbered decisions record changes.
The numbered decisions below record the review findings and resulting design changes.

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

- A non-technical owner on a locked-down laptop (no admin, no Python or Node). Runs a supplied binary.
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

## 4. Original browser flow (historical; replaced by CLI decisions below)

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

## 7. MVP and current scope

One supplied Go CLI binary, no runtime network client or language runtime, one pinned and vendored
module (`golang.org/x/text`, character sets).
Includes CSV, TSV and JSON, shape output, config review and approval, scrub, private mapping storage,
restore and refusal of unsupported files. Later work added Markdown/plain text word checks, Claude
Code hooks and setup. The mapping is saved by an explicit scrub command, outside the project.

Excludes name recognition models, PDF, images, Word, format-preserving stand-ins, accounts and hosting.

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
7. No network-capable Go package is linked into the CLI (dependency checks). Installers download the
   release; setup's external `claude --version` command is outside this package check.
8. Builds without cgo for macOS, Windows and Linux, arm64 and amd64. Native execution and installer
   checks on each platform are separate evidence; a cross-build alone does not establish them.

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
2. **Go, one static binary.** Easy cross-builds for macOS, Windows and Linux; no
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
    it out. Its block message names that flag. The author confirmed on 2026-10-31 that the guard stays:
    configurable is enough.
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

### From the second fresh review (2026-10-31)

46. **One-word names in headers are data.** Every header (CSV/TSV, Markdown table, JSON key path) also
    gets the word check: a word that is not an English word or a common header word, and is
    capitalised, in the first-name list, or in a script with no capitals, makes the header data-like
    (`scan.LooksLikeDataHeader`; file names keep the old check). Such a header is `[COLUMN_n]` in shape
    and messages, swapped in the output, and the config may not name it by its real name (scrub
    refuses, naming `[COLUMN_n]`; a text-table header must not be in `tables` at all). Departures from
    the brief: all-caps words of 2-4 letters (SKU, UTC, CRM) are short forms unless they are first
    names; day and month names count as header words; some export and French header words joined the
    common list (Clr, Lineitem, Etransfer, Courriel, Montant ...) so common exports still work. Cost:
    a brand header (Shopify) can now only be tokenized or dropped. Miss: a header that is a first name
    and also a word (Grace).
47. **A known value is swapped everywhere, and scrub checks before it writes.** In the word check a
    one-word `NAME` value is now swapped as a known value in any case and spot (also inside a link),
    except a day or month name in a file whose rule keeps `calendar`. (Kept words and English words
    were already left out of the known list.) The finder still lists such a word as a candidate
    (reason "known name"), so a word swapped before approve is still asked about. Scrub's pass 2 runs
    again if the mapping grew (a word swapped in a later file), then every scanned cell and
    word-checked line is checked with the same finder as `check`; a hit refuses (exit 2, nothing
    written), naming the file, the line or column and the stand-in, never the value. Departure:
    kept columns are not checked. A keep is an approved value, and amounts or dates kept in one column
    and tokenized in another are common: checking them refused a normal config in the tests.
48. **Joined words and lower-case surnames.** Replace also swaps a part of a hyphen-joined word whose
    key is on the swap or candidate list (`oksana-vantreeck` gives `[NAME_1]-vantreeck`); words joined
    by `.` and `_` were already separate. A lower-case word that is not an English word, directly
    after a caught name with only spaces between, is a candidate ("zelda quorn"). Miss: a part that is
    on no list stays (`vantreeck` above).
49. **Sentence starts.** The finder looks past `* _ ( [` and quotes to the text before a word; a colon is
    no longer a sentence end ("Notes: Grace has" catches Grace; "Note: Will be late" now swaps "Will",
    the safe side). Looking back: a capitalised first-name-list word right before a caught name is a
    candidate even at a sentence start ("Rose Fairweather").
50. **A day or month name next to a name joins the name-like group** ("April Okonkwo", "Ana June").
    Choices are per word, so after that the word is name-like in the whole project: "in April" is
    swapped too, unless the person keeps "april" in approve (which also keeps it in the name).
51. **Hidden characters refuse.** A soft hyphen, zero-width space, non-joiner, joiner (except between two
    emoji), word joiner or a byte order mark after the start, in any CSV, TSV, JSON (also as a `\u`
    escape) or text file, refuses with the line (or record) number; so does an HTML character code
    touching a letter in `.md`/`.txt` (`Quo&#114;n`, also `Q&amp;A`). The message says to remove it.
52. **Guards.** The hooks guard the whole private-folders root (`$STANDIN_HOME` or `~/.standin`, and the
    `--private` folder) for Read, Edit, Grep, Glob, LS and the Bash text check, and `init` adds deny
    rules for it and for `words.json`. If the project sits inside that root, only its `projects/`
    folder is guarded. The moved-project hint names the old and new private folders only when stderr
    is a terminal; else it says to ask the person to run `standin review` themselves. Project
    discovery: `CLAUDE_PROJECT_DIR` counts only if it holds the config (else the walk starts there),
    then an old `--project` that still holds the config, then the walk from `cwd`; a walk skips a
    config inside `raw/ shape/ scrubbed/ restored/` of an outer project; if `CLAUDE_PROJECT_DIR` and
    `cwd` lead to two different projects, the hook blocks. `init` takes out only hooks whose program
    is named `standin`/`standin.exe` (or is the binary it writes), saves `<file>.bak` before changing
    a settings file, and when it takes hooks out of a shared `settings.json` it says others must run
    `standin init` themselves. Terminal checks also refuse the null device. The text shape file says
    it shows field labels and table headers (a surname that is also a word would show).

### From the second trial (2026-10-31)

Six suggestions from a trial on a made-up shop folder. Weighed for general use, not one deployment.

53. **Calendar words are kept by default** in the word check. A missing `keep_groups` means
    `["calendar"]`; `"keep_groups": []` turns it off. Why: nearly every text has days and months, and
    forgetting the line made "Friday 10am" into stand-ins. A name spot still beats it ("Hi April"),
    and approve still asks the person about the calendar keep.
54. **No link from a first name in text to a full name in a column.** "Leila" in text stays `[NAME_n]`;
    "Leila Haddad" in a column stays `[PERSON_n]`. Why: linking merges two different people with the
    same first name, and a wrong merge is worse for the AI's reading than a split. Documented in the
    README. Reconsider if a person asks for an opt-in at approve.
55. **A top-level `keep_groups` says it belongs inside `text`.**
56. **Approve asks one question per file rule for keeps, and one for scans**, naming each column,
    not one per column. The word pages are unchanged. Why: twenty "type yes" prompts get typed
    without reading. Cost: one yes covers several columns, so the question lists every name.
57. **The scrub summary says "this run" and that kept words are not counted.** The second scrub's
    smaller count was right: the person had kept words at approve. Shape still writes one file per
    raw file; grouping files that look alike is not done (the per-file counts differ, and a merged
    file hides which file a count came from).
58. **A day or month name that is also a first name (Friday, May, June) stays in the calendar group**
    unless it is in a name spot (after Hi/Dear, a title, `name is`, a sign-off, next to a caught
    name). Before, the English-word test sent it to the first-name rule. "It's", "this is" and "c'est"
    no longer count as a name spot for these words ("It's Friday."). Cost: "It's May" about a person
    is missed.

### From the thorough review (2026-10-31)

59. **Pin file boundaries instead of trusting path strings (R1/R2).** Private paths resolve the longest
    existing ancestor before checking project overlap. Output files use `os.Root` handles; linked
    output roots and nested links are refused. Why: a nested restored-folder link wrote real names
    into scrubbed/. This preserves the standard-library-only binary.
60. **One private-state writer at a time (R3).** A cross-process directory lease covers mapping/approval
    reads through publication. Stale leases are never stolen automatically. Interrupted private saves
    retain a pending mark and previous private files; readers and hooks then refuse until a person
    recovers a matching private/output generation. A missing mapping over existing scrubbed files
    refuses. Why: concurrent runs assigned the same stand-in to different people. This is not a claim
    of all-files atomicity or proven power-loss durability.
61. **Reject ambiguous input names (R4–R6).** Literal JSON field keys containing dots/brackets, blank
    keys and `(key)` refuse; outer object-of-records names remain data. Empty-container keys are
    inventoried and validated too. Duplicate CSV/TSV headers and duplicate headers within one Markdown
    table refuse. Same headers in separate tables remain supported. Why: rejecting ambiguity is a
    smaller, safer MVP than inventing escaping or guessing which keep rule the person intended.
62. **Check filenames like headers, and against known values (R7).** Shape collects batch values only
    in memory to check metadata; scrub checks tokenized/dropped values before releasing diagnostic
    messages. Names flagged by either check are hidden by shape and refused by scrub. Why: swapping a
    cell did not help when its name was still in the filename. Ordinary-word and unknown-name limits
    remain documented.
63. **Strict structured restore and checks after substitution (R8/R9).** CSV/TSV quoting and row widths
    must parse; JSON must be one complete, duplicate-free value before and after restoration. Expanded
    formulas, links and HTML tag contexts leave the source stand-ins with warnings, including hazards
    assembled from multiple stand-ins. Why: valid-looking tokens can expand into active content.
    This is exposure reduction, not an HTML sanitizer. Plain-text fragments use `.txt` explicitly.
64. **Preflight setup and tell the truth on save failures (R10).** Init checks destinations before
    creating folders, stages its file writes, rolls back new empty folders after failure, and does not
    claim nothing changed after an incomplete save. Why: a setup failure previously left changes
    while saying the opposite.
65. **Strengthen the small safety checks.** JSON readers reject duplicate fields, trailing content,
    invalid Unicode, counter gaps/overflow and mapping keys inconsistent with their spellings.
    Terminal checks query the OS console/terminal, but a fake
    terminal is still not proof of a person. PreToolUse matches all tools so unknown/MCP tools reach
    the refusal; a short list of conversation/task-list tools is allowed. Why: a guard must not depend
    on unverified decoder defaults, device types or an incomplete matcher.
66. **Windows needs native file permissions.** Protected access lists grant the current Windows user
    access to private folders and files; ACL failure refuses the write. Unix mode bits alone are not
    Windows isolation. Native ACL behavior remains to be verified on Windows; OS administrators can
    still take ownership. The mapping and recovery copies remain secrets.
67. **Validation must show the guarded action happened.** Native CI and archive/release workflows are
    checked into the main tree, with pinned actions and a manual snapshot path. Live smoke checks need
    audit evidence of the requested hook/tool/path/exit code, not just absence of a private word.
    E2e setup fails when the binary cannot build. Generated temporary paths are excluded from fixture
    value searches so random digits do not masquerade as leaked amounts. Why: security checks must
    distinguish an actual protected action from unrelated setup/authentication failure or test noise.
    Hosted/native/live tests remain unrun until performed; no publication is implied by source fixes.
68. **Email is read as text, not as a mail file (v0.3).** One message per `.eml`; `From`, `To`, `Cc`,
    `Date` and `Subject` become fields and the body becomes free text, so config, shape, approve, scrub,
    restore and the hooks all work unchanged and there is no second privacy path to keep correct.
    Why: a workshop participant's inbox is the most private thing they own, and the agent needs the
    words in it, not the MIME. Cost, stated in the shape file and the README: the scrubbed copy is a
    plain-text view, so `restore` gives that view back, not the original message; HTML formatting,
    links and attachments are not kept. `raw/` is never changed. The mail libraries are not usable
    here: `net/mail` and `mime/multipart` both link `net`, which the no-network rule forbids. `mime`
    and `mime/quotedprintable` are used; the multipart split and the HTML-to-text step are ours.
69. **A hyphen or underscore separates words in a file name.** The name check ran the header-word test
    on the whole stem, so `sam-notes.md` passed as one word: the name reached the shape title and the
    first scrub, and the second scrub then refused the whole batch without saying which file. Each part
    is now checked with the word check alone; the digit and pattern checks are not applied per part,
    because a bare number is ordinary ("report-2026.csv", "top-100.csv"). Cost: a name that is also a
    common word still passes (documented).
70. **A refusal names the file when the person named it.** Twenty files in `raw/` and "A raw file (name
    hidden): I cannot read this kind of file" is a dead end. A file typed on the command line is already
    on the person's screen, so echoing it discloses nothing; a batch run keeps it hidden but now prints
    the extension ("a raw file (name hidden, .xlsx)"), which is enough to tell one bad file from another.
    The extension is echoed only when it is plain lowercase alphanumerics: a file called `.Zelda Quorn`
    has "Zelda Quorn" where its extension would be, and the existing test suite caught exactly that.
    Cost: a file named on the command line whose name looks like data still reads "(name hidden)".
71. **An anonymous shape file is named by a hash of the path.** It used to take the next free
    `file_N.shape.md`, so a second `shape` run wrote `file_2` next to a stale `file_1`, and the AI could
    be handed the wrong one. It is now `file_<6 hex>` from the project-relative path: stable for that
    file for ever, overwritten on each run, no state to keep. Cost: the name is opaque; the warning
    still tells the person to rename the raw file to something plain.
72. **A long encoded run is one word candidate.** A 16-character tracking id used to arrive as "AQH",
    "JzLm" and "Pq": three decisions for one id, none of them meaningful, and three chances for the
    person to keep a fragment and lose the rest. A run of 12 or more characters of letters, digits and
    base64 "+" that holds a digit is now one token and one candidate, swapped whole. A slash, hyphen,
    underscore or "=" ends a run, so a path ("/people/oksana"), a slug ("spring-2026-newsletter") and a
    query part ("campaign=spring") keep their ordinary words and their ordinary treatment. Twelve
    because eleven is still prose ("spring-2026"). Cost: the person decides about the id as a whole,
    with no per-fragment choice, which is what an id is anyway.
73. **One pinned, vendored module, and no bespoke dependency checking.** `go.mod` holds the version,
    `go.sum` the hashes, `go mod tidy -diff` in CI fails when the graph drifts from the source, and
    `vendor/` makes `git clone && go build` work offline. Why: "standard library only" was a proxy for
    "nothing unvetted enters", and it blocked the one change that reduced hand-written parsing risk.
    The allowlist is now `go.mod` itself, which is what Go means by an allowlist; a second, hand-written
    list of expected modules would duplicate `go.sum` and rot on the first module rename. The check that
    survives is the one Go cannot express: no package that can open a socket is linked.
74. **Character sets come from `x/text`; the mail structure stays ours.** Every Go MIME parser imports
    `net/textproto` for the header type, and that links `net`: `go-message`, `net/mail` and
    `mime/multipart` all do (measured, not assumed), so "use a mature parser" and "no network stack in
    the binary" are mutually exclusive in Go. Taking `x/text` alone keeps the second: it is net-free,
    and it replaces the refusal list with real decoding for every character set a mail program names
    (Shift-JIS, KOI8-R, GB2312, ISO-2022-JP ...). Cost: +900 KB binary and 4.4 MB vendored. The header
    lines, the multipart split and the forwarding walk stay ours, bounded by fixtures and fail-closed
    refusals. A script with no capital letters (Chinese, Japanese, Arabic) is still refused: the word
    check cannot find names there. Revisit if the network rule is ever dropped.
75. **A mature mail parser checks ours, from the test binary only.** `github.com/emersion/go-message` reads
    the same fixtures in `internal/dataset/oracle_test.go`, and the two readings must agree word for word.
    It is imported nowhere else, so `go list -deps ./cmd/standin` never sees it, the shipped binary still
    links no socket, and `make no-network` still reads zero. Why: this is the only way to have a mature
    implementation check ours without putting `net` in the product. It makes the hand-written parser
    verified rather than merely small, which is the part that was actually bothering the author.
    Cost: the test binary compiles it, `vendor/` carries it (+0.1 MB), and a library upgrade means
    re-reading a diff.

### From the follow-up verification (2026-10-31)

76. **Private filenames must be renamed before shape (replaces 70–71).** Shape checks the whole
    batch against filename rules and the batch's values before warnings or publication. A flagged
    name refuses the whole run, without printing that name. Why: an encoding warning bypassed the
    filename guard; six-hex anonymous hashes collided and allowed guesses of private names. Neutral
    names avoid a new secret saved by shape and the stale assignments of sequential names. Cost:
    setup needs a rename first. Old anonymous shape files from earlier versions need to be removed
    by the person before sharing; this change does not delete old files. Refusal messages display
    only known file extensions, never arbitrary lowercase suffixes.
77. **Output batches reject duplicate destinations before making folders.** Clean absolute paths
    must be distinct, including Unicode case aliases, on every platform. Why: two staged outputs
    previously silently replaced one another. This conservatively refuses some distinct names on
    case-sensitive filesystems so a batch stays portable. Boundary and link checks still apply.
78. **A redirected read checks the destination too.** A scrubbed twin must be outside protected
    zones, inside the scrubbed tree, regular, and have no linked component below the project root.
    Why: a newer link pointing back into raw passed the timestamp test. Hooks still hand a pathname
    to another program: they cannot stop another process changing it after the check.
79. **Encoded tokens partition the text.** Find ordinary words only in gaps between encoded runs.
    Why: a hyphenated word crossing a run's boundary produced overlapping byte spans and crashed
    scrub after a successful approve. This keeps the surrounding slug punctuation and makes each
    span safe to replace; approve, scrub and restore share the regression.
80. **Attachment disposition is checked before walking a body.** Skip the entire attachment entity,
    including multipart containers and attached messages; count the skipped container once.
    [RFC 2183 §2.9](https://www.rfc-editor.org/rfc/rfc2183#section-2.9) applies disposition to the
    multipart as a whole. Body selection belongs within each alternative subtree; independent mixed
    parts and forwarded message bodies remain in order, following the distinction in
    [RFC 2046 §5.1.3–4](https://www.rfc-editor.org/rfc/rfc2046#section-5.1.3). Why: global plain/HTML
    selection silently removed forwarded HTML and independent mixed bodies.
81. **Mail refusals must be data-free, and decoded text must be checked for loss.** Charset and
    transfer labels are private input, so errors must not repeat them. Declared encodings must not
    turn euros into another currency. Unknown encoded-header charsets refuse instead of returning
    the raw header. The [x/text decoder contract](https://pkg.go.dev/golang.org/x/text/encoding#Decoder)
    permits malformed bytes to become replacement characters without an error, so checking its
    error alone is insufficient. The tests include malformed bytes and legitimate replacement
    characters, plus complete decoded text with amounts and currency symbols. The mature-parser
    comparison is evidence for these synthetic cases, not proof that all mail is supported; this
    qualifies decision 75's earlier claim of verification.
82. **Run the promised dependency check in hosted verification.** Native CI, archive checks and
    release verification now invoke `go mod tidy -diff` (or `tidy-check`). Why: decision 73 promised
    the check in CI, but only the local Make target actually ran it. Vendored offline builds and the
    separate check for linked network packages remain required.
83. **Keep intended language and complete messages.** Repeated category and shape labels use named
    constants. The spelling check allows the Canadian and French forms used by commands, fixtures
    and input recognition. Punctuation checks have narrow exceptions for complete user-facing
    validation messages. Why: changing `licences`, `cheque` or French field words to satisfy a US
    dictionary would alter the interface or supported inputs, while sentence punctuation helps
    readers. No linter is disabled wholesale.
84. **Printing is part of a command's result.** Stdout and stderr remember write errors and short
    writes; a failure gives exit 2 with a data-free message. It may happen after files were saved,
    so the message does not promise nothing changed. Approval checks printing before reading each
    answer and before saving choices: unseen prompts cannot grant approval. Hook output failures
    also block. Why: ignored writes could claim success for a partial result or accept blind answers.
85. **Cleanup failures are different from harmless resource closes.** Check failed temporary-file
    cleanup, lock release, file sync and publication errors; retain unfinished-save state when
    rollback cleanup fails. Give safe recovery guidance without private paths. Read-only directory
    pins and embedded readers have explicit best-effort closes where no write can be lost. Optional
    error logging preserves the original refusal, but reports failure to release its lock. Why:
    swallowing a failed release can leave the next command blocked, and swallowing rollback cleanup
    can misrepresent recovery. These checks still do not prove power-loss durability.
86. **Best-effort reports and wrapped refusal codes are deliberate.** Report keeps collecting kinds
    and shapes after unreadable entries and skips unsupported shapes, rather than failing the whole
    issue report. Those two callback returns have local lint explanations. Refusal codes survive
    ordinary error wrapping and joins, so reports keep their useful data-free category. Terminal
    tests accept Linux's expected end-of-terminal EIO, check other I/O failures and drain the last
    output before closing the terminal.
87. **Checkout line endings must not change the checks or fixture bytes.** Text source and module
    files use LF on every platform; `testdata/` keeps its exact tracked bytes, including intentional
    encodings and CRLF. Why: the first native Windows CI stopped at `go mod tidy -diff`. A scratch
    checkout with CRLF module files reproduced a failing diff with no dependency changes. Explicit
    Git attributes remove that checkout-dependent difference without rewriting format fixtures.
88. **Windows ACL handles need metadata rights for the identity check.** The first native Windows
    tests could not secure an empty private temp, causing approval, scrub and recovery tests to fail
    afterward. The permission handle now requests Go's `File.Stat` metadata access set as well as
    `WRITE_DAC`; it still requests no file-data read or write rights. The no-follow open and SameFile
    comparison remain mandatory before setting the current-user-only ACL or writing private bytes.
    Go's `internal/syscall/windows/at_windows.go` adds the same metadata rights to allow `File.Stat`;
    [Microsoft's query documentation](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/nf-ntifs-ntqueryinformationfile)
    requires read-attribute access for basic file information. Native regression tests reject a path
    naming a different file and verify a matching empty file's protected ACL. Permission refusals
    now name a fixed failing step and numeric Windows code, never a private path; the output writer
    preserves that safe guidance. Cross-builds and vet do not establish native runtime success:
    Windows CI must confirm the fix before release.
89. **Native tests must trigger the same failure on each platform.** The rollback-cleanup test now
    uses a nonempty destination directory to force publication failure, then checks restoration of
    an earlier output, retained pending state and safe recovery guidance. Windows can rename the
    substituted source directory over a file, so the old injection could succeed. Setup tests also
    check the existing filename fallback on Windows rather than requiring Unix absolute deny rules.
    Windows absolute-path rules in Claude Code remain unverified and are omitted; the runtime hooks
    still check private-folder paths. This limitation is now explicit in the README.

### From the model evaluation (2026-10-31)

90. **A handle is a username, claimed whole like an email.** `@grace_lee` becomes `[USERNAME_1]`, a new
    category added to `mapping.Categories` and to the two shape prompts. Why: the word check cannot see
    one. "grace" and "lee" are English words, and decision 43 leaves a word after `@` alone on purpose,
    so an address local part and `price-sheet.md` are not split into fragments. A pattern now claims
    the run `@[A-Za-z0-9_][A-Za-z0-9_.-]{1,29}[A-Za-z0-9_]` whole, which makes the stand-in one value
    that restores exactly, and shape counts it as "username". Guards: the `@` must follow a non-word
    character (so `user@host` is not a handle), the run must hold a letter (so `@123` is a number), and
    a closed list of CSS at-rules and decorator words (`media`, `import`, `keyframes`, `apply` ...) is
    refused, because a pasted style sheet is the one place `@word` appears in prose. Cost: a handle is
    masked as a unit, so the AI sees `[USERNAME_1]` where the text had a username: the same trade as an
    email. **Miss kept:** a plain path (`/people/ana-vantreeck`) is not a pattern. A path segment gives
    no signal that it is a person, and "the word after `/` that is not an English word" flags every
    filesystem path in a note (`/usr/local/bin`). A name that repeats a known value is still swapped
    there by the known-values pass. Measured on the evaluation's hard set: the shipped check moved from
    77% to 79% of name instances with no new false flags
    (`docs/research/model-based-detection-eval.md`).
