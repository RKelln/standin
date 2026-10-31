# Standin: build plan (2026-10-30)

Goal: a Go CLI, good enough on synthetic data by the Get2Know workshop (2026-11-02), and useful to an
owner at home. Hooks call it; a person runs setup once with help from their agent.

## Shape of the work

| Track | Owner | Depends on | Output |
|---|---|---|---|
| R. Research | research agent | none | `docs/research/prior-art.md` |
| A. Core CLI | builder agent | this contract | `cmd/standin`, `internal/...`, unit tests |
| C. Fixtures and end-to-end tests | test agent | this contract | `testdata/`, `e2e/` |
| D. Release pipeline | release agent | none (cribbed from agentmap) | `.goreleaser.yaml`, `install.sh`, `install.ps1`, CI |
| B. Hooks, `init`, harness snippets, skill | builder agent | R | `internal/hook`, `skill/` |
| V. Fresh review | review agent | A, B, C | findings, then fixes |

Agents do not commit; the orchestrator reviews and commits.

## Contract (A and C both build to this; change it only through the orchestrator)

### Rules

- Go, standard library only. Module `github.com/RKelln/standin`, main in `./cmd/standin`,
  `var version, commit string` in `main` (set by ldflags).
- No package that can open a network connection is linked (`net`, `net/http`, `net/rpc`, `crypto/tls`...).
  A test checks `go list -deps ./cmd/standin`.
- Fail closed: on any parse error or doubt, write no output file and say why. Exit 2.
- Default-deny: a value is tokenized unless the approved config says `keep` (or `scan`, for free text;
  or the word check, for `.md`/`.txt` text, where a candidate word stays only if the person keeps it).
- No command ever prints a real value from a tokenized column, except `restore` (which writes for the
  person, into `restored/` or stdout when asked).
- Messages near grade 6. Errors say what happened and what to do next.

### Exit codes

`0` done. `1` done, with warnings or findings (see output). `2` refused or error; nothing written.

### Project folder

```
<project>/
  standin.config.json    the config: no secrets, the AI may read and write it
  raw/                   real files; the agent must not read this
  shape/                 shape files for the AI (setup only)
  scrubbed/              what the agent reads
  restored/              real values put back; for the person, the agent must not read this
```

Private store (the secret), outside the project so tools working in the project do not see it:
`$STANDIN_HOME/projects/<folder-name>-<first 8 hex of sha256(abs path)>/` with `mapping.json`,
`approval.json` and (v0.2, free text) `words.json`. `STANDIN_HOME` defaults to `~/.standin`. `--private DIR` overrides. Created `0700`,
files `0600`. Every command that uses it prints its path once.

### Commands

```
standin shape  [--project DIR] FILE...        raw/x.csv -> shape/x.csv.shape.md
standin review [--project DIR]                print the config as a plain table (keeps first)
standin approve [--project DIR]               needs a terminal (stdin is a TTY); asks yes per `keep`
                                              and per `scan` (and per word check and kept group);
                                              stores sha256 of the config in approval.json. With a
                                              word check: stdout must be a TTY too; shows new words
                                              to keep (v0.2, see "Free text")
standin scrub  [--project DIR] [FILE...]      raw/** (or FILEs) -> scrubbed/<same path>; updates mapping
standin restore [--project DIR] FILE... | -   scrubbed-style text -> restored/<same path>, or stdout for -
standin check  [--project DIR] FILE... | -    finds real values (from mapping) and PII patterns; on a
                                              terminal prints token names, pattern kinds and lines, never
                                              the value; off a terminal only "found something" or "found
                                              nothing"; exit 1 if any
standin show [--project DIR] STANDIN...      real value of each stand-in (PERSON_9 or [PERSON_9]); only
                                              when stdout is a terminal, else exit 2; unknown: exit 1 (v0.2)
standin report [--project DIR]                a data-free issue report (version, OS, file kinds, shapes,
                                              last errors) to stdout
standin version
standin licences                              notices of the built-in word lists (v0.2)
```

`scrub` without an approval whose hash matches the current config, when the config has any `keep` or
`scan` (or a word check, v0.2): it refuses (exit 2), writes nothing (not even the mapping) and says to run
`standin approve` in the person's own terminal (v0.2, decision 33; v0.1.0 tokenized them as `VALUE` and
warned). Changing the config voids the approval; so does a changed or missing `words.json` when the config
has a word check. A config with no `keep`, no `scan` and no word check needs no approval.

### Input formats (MVP)

- CSV and TSV: delimiter by extension (`.csv` comma, `.tsv` tab). Strip UTF-8 BOM. Not
  valid UTF-8: decode as Windows-1252 only if the bytes fit it, and say so; UTF-16 with BOM decoded.
  Ragged rows (field count differs from header): refuse with the line number.
- JSON: an array of objects, or an object whose values are objects ("object of records": its keys are
  data, category `KEY`). Nested objects flatten to dotted paths (`address.city`); arrays of scalars or
  objects inside a record use `[]` (`items[].name`). Anything else: refuse.
- Free text (v0.2): `.md`, `.markdown` and `.txt`, same encodings. See "Free text" below. A `.txt` that
  parses as a CSV or TSV table (3+ rows, 2+ columns, all the same width) is refused: "rename it to .csv
  or .tsv".
- Anything else (PDF, images, xlsx, docx, unknown): refuse, "Save it as CSV, or remove private details
  by hand." Later: OFX/QFX, QuickBooks exports, xlsx, `.eml`/`.mbox` (hand-parsed: `net/mail` links `net`).

### Stand-ins

- Format `[CATEGORY_N]`: `CATEGORY` is `[A-Z]+`, `N` starts at 1 per category. Regex
  `\[[A-Z]+_[1-9][0-9]*\]`. Brackets make `[PERSON_1]` and `[PERSON_12]` unambiguous.
- Categories: `PERSON ORG EMAIL PHONE ACCOUNT ID ADDRESS POSTAL KEY VALUE NAME` (unknown category in
  config: refuse). Unmentioned columns use `VALUE`. `NAME` (v0.2) is a word the word check swapped.
- Key: category + normalised value (lowercase, trim, collapse inner whitespace, strip trailing `.,;:`).
  The same normalised value always gets the same stand-in, across all files in a project.
- The mapping keeps every original spelling per stand-in (first one first). Restore puts back the first
  spelling and reports, by stand-in name, how many had more than one (exit 1).
- Empty values stay empty (not tokenized).
- `scrub` refuses (exit 2) any raw cell, header or JSON key that already contains stand-in-shaped text;
  the message names file, row and column, never the value. (Otherwise restore would turn it into a real
  value.)

### Config `standin.config.json`

```json
{
  "version": 1,
  "files": [
    {
      "match": "raw/invoices*.csv",
      "columns": {
        "Customer":  {"action": "tokenize", "category": "PERSON", "reason": "customer names"},
        "Amount":    {"action": "keep", "reason": "needed to match payments"},
        "Notes":     {"action": "scan", "reason": "free text; replace known names and patterns"},
        "Internal":  {"action": "drop", "reason": "not needed"}
      }
    }
  ]
}
```

- `match`: a glob relative to the project, `path.Match` rules plus `**`. First match wins.
- Actions: `tokenize` (needs `category`), `keep`, `drop` (value becomes empty; an optional `category`),
  `scan` (free text: pattern pass plus known-values pass, everything else stays). A column absent from the
  config is `tokenize VALUE`. Dropped values still get stand-ins in the mapping (their `category`, or
  `VALUE`), so scan swaps them where they show up in free text.
- `[COLUMN_n]` (a column named by position) may only be `tokenize` or `drop`: a position-based keep would
  follow a re-ordered export.
  Free-text-looking columns (avg > 40 chars or many spaces) absent from the config: `drop`, and warn.
- Unknown keys or actions: refuse.
- Text files (v0.2): a rule may also have `fields` and `tables` (maps like `columns`, same actions and
  checks, no `[COLUMN_n]`) and `text`: `{"action": "words"|"drop", "keep_groups": ["calendar"], "reason": ...}`.
  `keep_groups` only with `words`, only built-in groups (now just `calendar`): the config never names a
  word. No `text` rule: the text is dropped (warn). `fields`/`tables`/`text` on a CSV/JSON file, or
  `columns` on a text file: ignored, with a warning.

### Scan pass (`scan` columns and `check`)

- Known values: every real value in the mapping, matched case-insensitive on Unicode word boundaries,
  longest first, any run of spaces (including no-break and other Unicode spaces) between words, replaced by
  its stand-in. Values shorter than 3 chars are skipped (too many false hits). Known misses: Unicode normal
  forms (NFC vs NFD) and Turkish dotted I.
- Patterns (port from Get2Know `check_share.py`, same author, MIT): email, phone, card (Luhn), Canadian SIN
  (Luhn), IBAN, runs of 8+ digits, CA/US/UK postal codes. Each match becomes a new stand-in in its category.
- Names not in the mapping stay. The output says so.

### Shape file `shape/<name>.shape.md`

Markdown for the AI, built only for setup. Per column: header (or `[COLUMN_n]` plus a flag if the header
itself looks like data: email, digits, phone, or two capitalised words), inferred kind, rows, empty
share, distinct count as a band (`under 5`, `5-20`, `21-100`, `over 100`), length band
(`1-5 6-10 11-20 21-40 41-80 over 80`), and up to 3 shape strings (`email-like`, `date YYYY-MM-DD`,
`amount 2 decimals`, `two capitalised words`, `digits`, `letters and digits`, `free text`) with count
bands. Never a real value, never an exact length or count below 5. Ends with a short "how to write the
config" note and the config schema above. The file name is shape-checked too: if it looks like data, the
shape file uses `file_n` and says to rename the raw file.

### Restore

Strict: replaces every exact stand-in it knows, longest first, never fuzzy (`[PERSON_1]` vs `[PERSON_7]` is one edit). Reports (by stand-in, not value): unknown stand-ins
(`[PERSON_99]`), near-misses it did not change (`[person_1]`, `PERSON_1`, `[PERSON 1]`, `Person 1`), and
known stand-ins it left alone because they sit in a link or a formula: in any text, inside `http://`,
`https://`, `www.`, after `](`, `src=` or `href=`; in CSV/TSV, any cell starting `= + - @` (after
trimming). Two inputs that would land on the same `restored/` name: refuse. Exit 1 if any finding.

### Settled details (2026-10-30, from track C questions)

1. Paths: `raw/a/b.csv` -> `scrubbed/a/b.csv` (path relative to `raw/`); `scrubbed/a/b.md` ->
   `restored/a/b.md`; a file outside `scrubbed/` -> `restored/<base name>`.
2. `scrub` of many files runs in two passes: tokenize columns of every file first, then `scan` columns,
   so a name met in a later file is still known.
3. Exit codes: `shape` 0, or 1 if it flagged a header or file name. `scrub` 0 when approved and clean,
   2 when a needed approval is missing.
   A config with no `keep` and no `scan` needs no approval.
4. Restore and check reports go to stderr; stdout carries only the restored text (for `-`). `restore -`
   refuses (exit 2) when stdout is not a terminal: restore is for the person, not the agent.
5. `approve` asks `Keep <column> in <match>? ... Type yes:` per keep and `Scan <column> in <match>?
   It is free text: names Standin does not know stay in ... Type yes:` per scan, then one final yes.
   `approval.json`: `{"version":1,"config_sha256":"...","approved_at":"RFC3339",
   "keeps":["<match>:<column>"],"scans":["<match>:<column>"]}`.
6. The private folder hash uses the absolute path with symlinks resolved.
7. No `standin.config.json`: `scrub` refuses (exit 2), "run standin shape, then write a config".
8. Free-text rule: average over non-empty cells.
9. A file name that looks like data (any folder, or the full file name with or without its extension, or
   any dot-separated part): `shape` writes `file_n` and says rename; `scrub` refuses it (exit 2).
   Messages, errors (OS errors are reduced to their cause, without the path), logs and the report never
   print such a name; the report names only common extensions, others as `(other)`.
10. Binary content (NUL bytes, `PK`/`%PDF` magic) is refused whatever the extension.
11. JSON records may differ in keys; a missing key stays missing.
12. Word boundaries in the scan are Unicode-aware (Go's `\b` is ASCII-only: `Renée` would be missed).
13. CSV/TSV with lone carriage returns (old Mac line endings) is refused: "save it with normal line
    endings".
14. Headers and JSON keys: shape and scrub use one check (`scan.LooksLikeDataName`: the whole name, then
    each dot-separated part). Names caught: two capitalised words incl. `O'Brien`/`McNab`/hyphens, two
    all-caps words, two lower-case words (`lee_pip`), unless both are common header words; a common word
    followed by an uncommon capitalised word (`Bill Gatesby`). Scrub also swaps a header equal to a value
    already in the mapping.

### Free text (v0.2, track F; DESIGN 38-44, research `docs/research/free-text.md`)

- **Structure first.** A Markdown table (a line with `|`, then a `|---|` line with the same cell count;
  rows follow while lines hold `|`) gives one column per header, `Source` "table N"; a ragged row refuses
  with its line number. A `Label: value` line (optional `- ` and `**`; label 1-3 words, starts with a
  capital, at most 30 characters, every word a common header word or an English word that is not a first
  name, not data-like by `LooksLikeDataName`; value not starting `//`) is a field, one column per label.
  Everything else is free text. Output keeps every byte that is not swapped (line endings, table layout,
  Markdown marks).
- **Refusals.** Lone CR; stand-in-shaped text anywhere (scrub); more than 20% of letters (and 10 or more)
  in a script with no capitals.
- **Candidates** (package `internal/words`): in free text and `scan` fields, a word of 2+ letters outside
  known-value and pattern hits (a one-word `NAME` value is not such a hit: it goes through the rules
  again), when one of: after Hi, Hello, Dear, Thanks, Cheers, from, with, cc, called ... (then nothing or
  a comma); after a title (Mr Mrs Ms Mx Dr Mme M Prof, then nothing or a dot); after "name is", "this
  is", "it's", "Name:", "Client:" ...; a sign-off (1-3 words alone on a line after "Thanks," "Regards,"
  ... or after `--`); capitalised and not in the English list (day and month names here go to
  `calendar`); capitalised, in the first-name list, not at a sentence or line start; lower case, 3+
  letters, in the first-name list, not an English word or a day name, not after `. / @ _`; capitalised,
  right after a word caught as a name with only spaces between; capitalised and a word of a tokenized,
  not-free-text field value ("field part"); any word in a script with no capitals.
- **Groups.** `calendar`: only the calendar reason. `name-like`: a name spot, sign-off, first name,
  next-to-name or field part, or in the first-name list. `unusual`: the rest.
- **Lists:** English = SCOWL size 60, Canadian, variant 1, lower-case entries only, no `'s` forms
  (79,626 words); first names = Ontario + Quebec, single words, given 5+ times (22,409). gzip, `go:embed`,
  `internal/words/lists/`, made by `tools/fetch_wordlists.py`; notices in `THIRD_PARTY.md`.
- **Swap.** In free text and `scan` fields: known values and patterns as in the scan pass, then every word
  whose key (lower case, `’`→`'`, `'s` off) is a candidate or on the swap list, unless kept (keep list, or
  a calendar-only word where the rule keeps `calendar`). Swapped where it is capitalised or uncased, and
  in lower case only when it is not an English word. `'s` stays outside: `[NAME_1]'s`.
- **words.json** (private folder, 0600): `{"version":1,"note":"SECRET ...","keep":[...],"swap":[...]}`,
  keys sorted. Only `approve` writes it. Unknown keys refuse.
- **approval.json** adds `"texts": ["<match>:text"]` and `"words_sha256"` (sha256 of words.json as
  written). A word check needs both to match.
- **approve** with a word check: yes per word check and per kept group, then the new words (candidates in
  neither list, not calendar-only): unusual first, then name-like, most used first, 20 a page, each with
  its count and one context line (about 30 letters each side, the word in `[ ]`, other candidates, known
  values and patterns as `▒▒▒`). The person types numbers to keep (`1 3 5-7`; Enter or `none` for none);
  three bad answers refuse. Not picked: swap list. Then the final yes.
- **scrub**: pass 1 also adds, for tokenized/dropped field values, the part before `<` and pattern hits.
  New words are swapped, never kept; scrub then warns "N new word(s) ... run standin approve" (exit 1).
  A word moved to keep marks its `NAME` row `"kept": true` in the mapping: it still restores, but
  scans, check and the hooks no longer look for it.
- **shape**: fields and table columns as column shapes; free text as count bands only (lines, unusual,
  name-like, calendar, pattern kinds). **review**: count bands only, plus new words since approve.

## Order

1. Now: R (running), A, C, D in parallel.
2. When R lands: fold findings into `DESIGN.md` Decisions; brief B (hooks: Claude Code first, then Codex,
   OpenCode; `standin init`; skill text including "report issues with `standin report`").
3. V: fresh review, then fixes. Then success checks run and recorded in `HANDOFF.md`.
4. Release v0.1.0 for the workshop; Ryan tests the curl install on the Mac mini.

## v0.2 (2026-10-30, from the Get2Know trial)

Source: `docs/get2know-needs.md`. Decisions: `docs/DESIGN.md` 32-38. These replace the contract above
where they differ (unapproved scrub now refuses; hooks take no `--project`; `show` is new; settings go to
`settings.local.json` unless `init --shared`; `init --no-prompt-guard` skips the prompt hook).

| Track | Owner | Output |
|---|---|---|
| G. Fixes 2-7 | builder agent | DESIGN 32-37, tests, smoke test updated |
| F. Free text | research agent, then builder | `docs/research/free-text.md` (design, list licences, candidate counts on the Get2Know fixtures), then `.md`/`.txt` support (built; contract in "Free text" above) |
