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
