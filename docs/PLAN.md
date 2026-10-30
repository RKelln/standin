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
- Default-deny: a value is tokenized unless the approved config says `keep`.
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
`$STANDIN_HOME/projects/<folder-name>-<first 8 hex of sha256(abs path)>/` with `mapping.json` and
`approval.json`. `STANDIN_HOME` defaults to `~/.standin`. `--private DIR` overrides. Created `0700`,
files `0600`. Every command that uses it prints its path once.

### Commands

```
standin shape  [--project DIR] FILE...        raw/x.csv -> shape/x.csv.shape.md
standin review [--project DIR]                print the config as a plain table (keeps first)
standin approve [--project DIR]               needs a terminal (stdin is a TTY); asks yes per `keep`;
                                              stores sha256 of the config in approval.json
standin scrub  [--project DIR] [FILE...]      raw/** (or FILEs) -> scrubbed/<same path>; updates mapping
standin restore [--project DIR] FILE... | -   scrubbed-style text -> restored/<same path>, or stdout for -
standin check  [--project DIR] FILE... | -    finds real values (from mapping) and PII patterns; prints
                                              token names and pattern kinds, never the value; exit 1 if any
standin report [--project DIR]                a data-free issue report (version, OS, file kinds, shapes,
                                              last errors) to stdout
standin version
```

`scrub` without an approval whose hash matches the current config: every column is tokenized (keeps are
ignored) and it warns, exit 1. Changing the config voids the approval.

### Input formats (MVP)

- CSV and TSV: delimiter by extension (`.csv` comma, `.tsv` tab); `.txt` refused. Strip UTF-8 BOM. Not
  valid UTF-8: decode as Windows-1252 only if the bytes fit it, and say so; UTF-16 with BOM decoded.
  Ragged rows (field count differs from header): refuse with the line number.
- JSON: an array of objects, or an object whose values are objects ("object of records": its keys are
  data, category `KEY`). Nested objects flatten to dotted paths (`address.city`); arrays of scalars or
  objects inside a record use `[]` (`items[].name`). Anything else: refuse.
- Anything else (PDF, images, xlsx, docx, unknown): refuse, "Save it as CSV, or remove private details
  by hand." Later: OFX/QFX, QuickBooks exports, xlsx.

### Stand-ins

- Format `[CATEGORY_N]`: `CATEGORY` is `[A-Z]+`, `N` starts at 1 per category. Regex
  `\[[A-Z]+_[1-9][0-9]*\]`. Brackets make `[PERSON_1]` and `[PERSON_12]` unambiguous.
- Categories: `PERSON ORG EMAIL PHONE ACCOUNT ID ADDRESS POSTAL KEY VALUE` (unknown category in config:
  refuse). Unmentioned columns use `VALUE`.
- Key: category + normalised value (lowercase, trim, collapse inner whitespace, strip trailing `.,;:`).
  The same normalised value always gets the same stand-in, across all files in a project.
- Restore puts back the first spelling seen.
- Empty values stay empty (not tokenized).

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
- Actions: `tokenize` (needs `category`), `keep`, `drop` (value becomes empty), `scan` (free text: pattern
  pass plus known-values pass, everything else stays). A column absent from the config is `tokenize VALUE`.
  Free-text-looking columns (avg > 40 chars or many spaces) absent from the config: `drop`, and warn.
- Unknown keys or actions: refuse.

### Scan pass (`scan` columns and `check`)

- Known values: every real value in the mapping, matched case-insensitive on word boundaries, longest first,
  replaced by its stand-in. Values shorter than 3 chars are skipped (too many false hits).
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

Replaces every exact stand-in it knows. Reports (by stand-in, not value): unknown stand-ins
(`[PERSON_99]`), and near-misses it did not change (`[person_1]`, `PERSON_1`, `[PERSON 1]`, `Person 1`).
Exit 1 if any.

## Order

1. Now: R (running), A, C, D in parallel.
2. When R lands: fold findings into `DESIGN.md` Decisions; brief B (hooks: Claude Code first, then Codex,
   OpenCode; `standin init`; skill text including "report issues with `standin report`").
3. V: fresh review, then fixes. Then success checks run and recorded in `HANDOFF.md`.
4. Release v0.1.0 for the workshop; Ryan tests the curl install on the Mac mini.
