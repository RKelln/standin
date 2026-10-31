# Prior art, agent hooks and file formats (research, 2026-10-30)

Historical research, with checks dated where recorded. This is not the current implementation
contract; use [`../PLAN.md`](../PLAN.md) and [`../DESIGN.md`](../DESIGN.md).

**Scope note.** This started as research for a single offline HTML page. Partway through, the project
owner changed the scope: Standin is now a **command-line tool (Go, one static binary), called mainly by
agent hooks**. Part A (prior art) applies to both. Part B covers the new scope: agent hooks, running an
unsigned binary on a Mac or Windows laptop, and the file formats small businesses export. The browser
facts I had already tested are kept in the appendix, in case a browser page comes back (for example as
the restore screen, or as a fallback on laptops that block downloaded programs).

**Questions.** Does a tool already do what Standin plans? What can we learn from the tools that come
close? Can an agent hook quietly send the agent to a scrubbed copy when it tries to read a raw file? Will
a non-technical person be able to run the binary at all? What do the input files look like?

**Short answers.**

- One tool comes very close to the old browser plan: **data-masker** (MIT). For the hook-based CLI, the
  closest are **mloda-pii** (proprietary), **noirdoc** (MIT, Python) and **PrivAiTe** (BSD, a proxy).
  None is a small static binary with Standin's shape-first, default-deny flow.
- **Yes, in Claude Code a `PreToolUse` hook can rewrite a `Read` call's `file_path` to the scrubbed copy**
  (`updatedInput`), and permission rules are then checked against the new path. A `PostToolUse` hook can
  also **replace** the output of built-in tools, Bash included (`updatedToolOutput`). But a crashed or
  timed-out hook lets the call through unless the hook sets `"onFailure": "block"`, and hooks never see
  `@file` mentions, `CLAUDE.md`, or text the person pastes.
- In **Codex**, there is no file-read tool to redirect; reads happen through shell commands, which a
  hook can rewrite but not reliably parse. Its sandbox deny rules are the stronger tool.
- In **OpenCode**, a plugin can change tool arguments before a call and block it by throwing.
- An **unsigned Go binary on a Mac** runs on Apple silicon (Go's linker adds an ad-hoc signature), but a
  browser download is quarantined, and since macOS Sequoia the person must go to System Settings to allow
  it. On Windows, **Smart App Control blocks unsigned code outright** when it is on.

All claims were checked on **2026-10-30** unless a row gives another date. Licences come from the GitHub
API (`gh api repos/<owner>/<repo> --jq .license.spdx_id`). "Unverified" means I could not confirm it at a
primary source.

## Words used here

- **PII** (personally identifiable information): any detail that can point to a real person, such as a
  name, email, phone number or account number.
- **Pseudonymisation**: swapping a real value for a stand-in, while someone keeps a key that can swap it
  back. The data is still personal data, because it can be reversed.
- **Anonymisation**: changing data so that no one can work back to the person. Much harder. Standin does
  not do this.
- **Vault** or **mapping table**: the private list of stand-in to real value. Whoever has it can undo
  everything.
- **Restore** (also "deanonymise", "reveal", "rehydrate"): putting real values back into text.
- **Quasi-identifier**: a detail that is harmless alone but can point to one person when combined with
  others (postal code + birth date + job).
- **k-anonymity**: a table is k-anonymous if every row looks the same as at least k-1 other rows on the
  quasi-identifiers ([ARX](https://arx.deidentifier.org/overview/privacy-criteria/)).
- **Content security policy (CSP)**: a rule set a web page gives the browser, saying what the page may
  load or connect to.
- **Secure context**: a page the browser trusts enough to offer some features, such as `crypto.subtle`.
- **Hook**: a small program that an AI agent tool runs before or after each action, which can block or
  change it.

## Part A. Prior art

### A1. The tools in DESIGN.md section 8, checked

| Tool | Licence (GitHub API) | Status | What it does | Reversible? Vault? Restore? | Fits Standin? |
|---|---|---|---|---|---|
| [Presidio](https://github.com/data-privacy-stack/presidio) (repo moved from `microsoft/presidio` to `data-privacy-stack/presidio`; docs moved to [presidio.dataprivacystack.org](https://presidio.dataprivacystack.org/anonymizer/)) | MIT | Active. Release 2.2.366 on 2026-10-27. | Python. Finds PII (regex plus spaCy names), then applies an operator. Built-in operators: replace, redact, hash, mask, encrypt, custom, surrogate_ahds, keep. One deanonymise operator: decrypt. | **Only `encrypt`/`decrypt` is reversible.** It is AES in CBC mode with a random IV (`presidio_anonymizer/operators/aes_cipher.py`), key 128, 192 or 256 bits. A random IV means the same name encrypts differently each time, so two rows cannot be matched. No numbered placeholders and no vault are built in. Numbered placeholders exist only as a **sample**: `InstanceCounterAnonymizer`, format `<{entity_type}_{index}>`, in `docs/samples/deployments/openai-anonymaztion-and-deanonymaztion-best-practices/`. | Ideas only. Needs Python. |
| [presidio-structured](https://github.com/data-privacy-stack/presidio/tree/main/presidio-structured) | MIT (part of Presidio) | v0.0.10 on PyPI, 2026-10-27. | Guesses which columns or JSON keys hold PII, then runs Presidio operators on every value in them. Uses pandas. | `StructuredEngine` has `anonymize` only. **No deanonymise.** | Confirms "column rules" is a known approach. Needs Python. |
| [LLM Guard](https://github.com/protectai/llm-guard) | MIT | **Archived: yes.** Last commit "Archiving Project (#355)" on 2026-07-08. README: "THIS PROJECT HAS BEEN ARCHIVED." Last release 0.3.16 on PyPI, 2025-05-19. | Python prompt-security kit. `Anonymize` scanner puts `[REDACTED_PERSON_1]` style placeholders in a `Vault`; `Deanonymize` puts them back. | Yes, in memory. Restore strategies: exact, case-insensitive, fuzzy (Levenshtein distance up to 3), exact-then-fuzzy (`output_scanners/deanonymize.py`). | Shape only. DESIGN.md's July 2026 claim is **correct**. |
| [mloda-pii](https://pypi.org/project/mloda-pii/) | `LicenseRef-Proprietary` (PyPI). "Proprietary and source-available". No GitHub repo found. | 0.7.0 on PyPI, 2026-10-30. | Claude Code hook. Rewrites tool results so the model sees tokens. Detection with Presidio, names with spaCy. Python 3.10 to 3.14. | Tokens are keyed HMAC-SHA256 (`PII_EMAIL_ADDRESS_<32 hex>`, or `PII_EMAIL_ADDRESS_1` in "readable" mode). Vault in `~/.local/state/mloda-pii/`. Restores on the user's screen. Fails closed by default. Sends anonymous counts to PostHog unless turned off. | Do not depend on it. Its "Limits" list is the best test checklist found (quoted below). |
| noirdoc: [mask-engine](https://github.com/noirdoc-ai/mask-engine) and [mask-claude-plugin](https://github.com/noirdoc-ai/mask-claude-plugin) (old names `nextaim-de/noirdoc`, `nextaim-de/noirdoc-claude-plugin` redirect here) | MIT (both) | Engine 0.2.0 (2026-09-04), plugin 0.2.1 (2026-04-27). README says alpha. | German-first PII redaction for PDF, DOCX, XLSX, text. Presidio plus optional GLiNER and Flair (about 560 MB of weights). Claude Code plugin blocks raw reads of protected paths and routes them through redaction. | Yes. Placeholders `<<PERSON_1>>`. Mapping in `~/.noirdoc/namespaces/<ns>/`. `noirdoc reveal` is run **by the user in their own terminal, outside Claude Code**, so real values never enter the transcript. Lenient restore (see A3). Also sells a hosted proxy, "Noirdoc Cloud". | Best model for the "restore happens outside the agent" rule. Its threat model is quoted below. Needs Python 3.12+. |
| shutapp | Not found | **Unverified.** No GitHub repo, PyPI package or product page found under that name (GitHub repo search and two web searches). | | | Drop it from the table, or ask where the name came from. |
| [PrivAiTe](https://github.com/crp4222/PrivAiTe) | BSD-3-Clause | Active. v0.8.0 on 2026-10-30. 58 stars. | Local proxy for LLM APIs and agent CLIs (Claude Code, Codex). Scrubs requests, restores replies, including streaming and tool-call arguments. Presidio plus a local ONNX model. Docker image about 3 GB. | Yes, mapping in memory per request. Placeholders `<EMAIL_ADDRESS_1>`. Fails closed if detection errors. Its README says the agent's own prompt (where Claude Code puts `CLAUDE.md`) "pass[es] through as-is", and the agent keeps real values in its local transcripts. | Not a fit for the user (needs Docker or Python). Its restore code has the best lessons (A3). |
| [RemoveMyID](https://github.com/Sinaini/removemyid) | MIT | 1 star. Last push 2026-10-16. | React + TypeScript + Vite. Browser-only redaction of text, CSV, JSON, PDF, images (Tesseract OCR, compromise.js for names). | Offers consistent pseudonyms `[PERSON_1]`. **No mapping export or restore** documented in the README. | Needs a Node build. No round trip. |
| [scrubadub](https://github.com/LeapBeyond/scrubadub) | GitHub API: **Apache-2.0**. PyPI metadata says MIT. The two disagree. | Not archived, but no push since 2023-09-01 (v2.0.1). | Python. Removes PII from free text with tags like `{{EMAIL}}`. | No. Tags are not numbered or reversible. | No. |
| [PresidioJS](https://github.com/ergesmema/presidioJS) | GitHub API: **none** (no LICENSE file). README says MIT. | 1 star. Last push 2026-03-26. | Browser Presidio-like library. NER model about 67 MB via Transformers.js. Swiss/German recognisers. | Yes: `<PERSON_1>` with a deanonymise mode. Encrypt operator uses AES-GCM via WebCrypto. | Needs a build and a model download. Licence unclear. |
| [Rampart](https://github.com/nationaldesignstudio/rampart) | GitHub API: NOASSERTION. README: **CC BY 4.0**. | Last push 2026-06-30. 238 stars. | 14.7 MB ONNX model plus fixed rules, for text typed in a browser. | Session table maps values to `[GIVEN_NAME_1]`; restored locally. | Model is a download. CC BY 4.0 for code is unusual; check before reuse. |
| [Drawbridge](https://github.com/sprice/drawbridge) | MIT | 0 stars. | Chrome extension using Rampart on claude.ai and chatgpt.com. | Restores placeholders in replies. Its README says it only wraps `window.fetch`, and attachments are not scanned. The model "downloads from HuggingFace once at install time". | No. |
| "Cloak" | Not found | **Unverified.** No matching browser PII tool found. | | | Drop or find a link. |
| [privatiser-engine](https://github.com/XionDot/privatiser-engine) | MIT | 14 stars. PyPI `privatiser` 0.6.0. | Python CLI, web UI and browser extension. Aimed at logs and config (IPs, keys, AWS IDs). | Yes: `-m mapping.json`, `privatiser deanonymize`. Uses **format-preserving** fakes (`user-1@redacted.example.net`, `(555) 000-0001`). | Not for business records. |

The size claims in DESIGN.md ("15 to 67 MB models") hold: Rampart 14.7 MB, PresidioJS about 67 MB.

### A2. What the first pass missed

| Tool | Licence | Status | What it does | Round trip? | Notes |
|---|---|---|---|---|---|
| [**data-masker**](https://github.com/farhan-syah/data-masker) | MIT | Created 2026-10-20. Release v1.0.9 on 2026-10-21 (one `index.html`, 1.1 MB). 0 stars. | **One offline HTML file.** CSV, TSV, TXT, xlsx, xls, ods (bundles SheetJS 0.20.3). Per column: Mask, Remove or Keep. Guesses column types from headers in 14 languages and from values. Codes like `PERSON_001`. Date shifting, rounding, ranges. "Hide names and numbers in notes". A "Risk check" panel. Plain-language README. | **Yes.** A "match file" (CSV: `code,original_value,column`) and a "Restore names" step that turns codes in an AI answer back into real values. | The closest thing to Standin. See below. |
| [deident](https://github.com/ikuV/deident-wasm) | Apache-2.0 | 155 stars. Last push 2026-08-23. | Rust CLI and WASM for CSV, JSONL, Parquet, DICOM, driven by a YAML policy. | Yes: an encrypted vault and `deident reverse`. Refuses to reverse an ambiguous value. | Not a single HTML file. Its anonymise report is the k-anonymity check we want (see A2b). |
| [DataPermit](https://github.com/honraosaloni-prog/DataPermit) | None (no licence file) | 0 stars. | Single-file HTML prototype: inspect, per-column rules (tokenize, pseudonymise, generalise), verify, export. | No restore. | **Loads SheetJS from cdnjs at runtime**, so it is not offline. No licence, so we cannot reuse it. |
| [csv-anonymizer](https://github.com/fabriziosalmi/csv-anonymizer) | AGPL-3.0 | 10 stars. | Static in-browser CSV fuzzer and redactor. | No restore. | AGPL. |
| [csv-data-anonymizer](https://github.com/ddv1982/csv-data-anonymizer) | MIT | 0 stars. | Desktop app with per-column actions. | Tokens are opaque; no restore found. | Not a browser file. |
| [PasteGuard](https://github.com/sgasser/pasteguard) | Apache-2.0 | 792 stars. | Local proxy and browser extension for chat, APIs and coding agents. | Restores placeholders in replies, streaming included. | Proxy or extension: needs installs. |
| [maskit](https://github.com/xiaYuTian11/maskit) | AGPL-3.0 | 320 stars. | Local gateway for coding tools; Chinese-language focus. | Streaming restore. | Proxy. |
| [CosyRedactGateway](https://github.com/CassiopeiaCode/CosyRedactGateway) | Apache-2.0 | 105 stars. | Credential-focused gateway. | Restores, including streamed parts. | Proxy. |
| [nopii](https://github.com/olafrv/nopii) | Apache-2.0 | 1 star. | Proxy for Claude Code. | Restores. Placeholders are `<TYPE>_<sha256(value)[:8]>`. | **Lesson:** an unsalted hash of the value can be reversed by guessing (hash every name in a phone book and compare). Do not build tokens from a plain hash. |
| [sensitive-canary](https://github.com/coo-quack/sensitive-canary) | MIT | 29 stars. v0.16.0 on 2026-10-24. | Claude Code hooks that **block** prompts, reads and commands holding secrets or PII. | No; it blocks, it does not tokenise. | Possible companion, not a substitute. |
| [ARX](https://github.com/arx-deidentifier/arx) | Apache-2.0 | v3.9.2, 2025-09-21. | Java desktop tool for k-anonymity, l-diversity, risk analysis. | No (anonymisation, not pseudonymisation). | Too heavy; useful definitions. |
| [sdcMicro](https://github.com/sdcTools/sdcMicro) | GPL-2.0 | v5.8.2, 2026-07-17. | R package for disclosure-risk checks. | No. | Needs R. |
| [Amnesia](https://github.com/dTsitsigkos/Amnesia) | BSD-3-Clause (in LICENSE file; GitHub API says NOASSERTION) | Last push 2025-12-25. | Java server for k-anonymity and km-anonymity. | No. | Needs Java. |

#### A2a. Does anything make Standin unnecessary?

Not quite, but **data-masker is close enough that Ryan should look at it before we build.** It is MIT, so
we could borrow ideas or code with credit, or offer changes upstream. This is a scope choice for Ryan.

What data-masker already has that Standin planned: one HTML file, opens offline, per-column choice, a
mapping file the user keeps, restore of codes in an AI answer, a browser test that checks "No network
request. No original value in any download. The match file fits the masked data. Restore returns the
real values." It also has ideas Standin lacks: a formula guard on restore (cells starting with `=`, `@`,
or `+`/`-` before a letter get a leading apostrophe, so a spreadsheet does not run them); a warning not to
open the match file in Excel (it changes long numbers and leading zeros); a four-character run ID in both
file names so the match file always fits the masked file; a warning before closing the page without saving
the match file.

Where it differs from Standin's rules:

1. **It makes one network request.** Its CSP has `connect-src https://api.github.com`, and the code
   fetches the repo's star count (`src/app.js`, "The only network request of the page"). Standin's rule
   is no network request at all.
2. **It is not default-deny.** It keeps dates, salaries and categories by default, and masks only what it
   guesses is a direct identifier.
3. **No shape-first step.** There is no file the AI can read to propose the config. A person chooses
   column by column. (It does say "Column names and types only is safest" when sharing.)
4. **No agent settings.** Nothing for Claude Code or Codex.
5. **No JSON input.**
6. **Untested beyond Chromium.** Its README: "Firefox and Safari are not tested."
7. **Its "Risk check" is not a k-anonymity check.** It flags columns with personal data set to Keep,
   partial dates and free text. For combinations it only advises: "Count how many people share each mix.
   Merge groups of fewer than 5."
8. **Codes ignore capital letters** (README), so restore returns one spelling for all variants (see A3).

#### A2b. The lightest useful re-identification check

ARX, sdcMicro and Amnesia all start from the same count. For a few thousand rows it is cheap in a
browser:

1. The person ticks the columns that stay readable and could point to someone (role, postal code, birth
   year). These are the quasi-identifiers.
2. For each row, join those values into one key. Count how many rows share each key.
3. Report: the smallest group size (this is **k**), how many rows are alone in their group (unique),
   and the share of rows in groups smaller than 5.
4. Show the keys only as counts ("12 rows are the only one with their mix"), never the values.

deident prints exactly this: `quasi-identifiers [age, zip, admission_date]: 5 equivalence class(es), min
size 1, 1 unique row(s) (8.3%)` ([README](https://github.com/ikuV/deident-wasm)). A JavaScript `Map`
over 5,000 rows takes milliseconds. This is not a full risk model (ARX also models "prosecutor",
"journalist" and "marketer" attackers), but it catches the common case.

### A3. Lessons for restore

How the tools handle an AI that changes a placeholder:

| Tool | Placeholder | Restore rule | Source |
|---|---|---|---|
| LLM Guard | `[REDACTED_PERSON_1]` | Exact `str.replace`, or case-insensitive, or fuzzy (edit distance up to 3). | `llm_guard/output_scanners/deanonymize.py` |
| noirdoc | `<<PERSON_1>>` | Pass 1 exact `<<[A-Z_]+_\d+>>`. Pass 2 "lenient": any case, spaces inside, `«»`, `《》`, `＜＞` brackets; upper-case it and look it up. An unknown placeholder is left alone and logged. It also tells the model, in a system prompt, to treat `<<TYPE_N>>` as real values and "Never comment on" the format. | `src/noirdoc/reidentification/engine.py`, `pseudonymization/system_prompt.py` |
| PrivAiTe | `<EMAIL_ADDRESS_1>` | One regex pass, **longest placeholder first**, so a replacement is never replaced again. Optional fuzzy pass that trims edge punctuation, joins whitespace (so a placeholder split over a line break still matches), and **never fuzzy-matches an intact placeholder it does not know** ("Fuzzy-matching it to a KNOWN placeholder would inject someone else's PII"). | `privaite/pii/deanonymizer.py` |
| data-masker | `PERSON_001` | Regex with word guards on both sides `(?<![A-Za-z0-9_])PREFIX[_-]ID(?![A-Za-z0-9_])`, any case. Accepts `_` or `-`. Counts codes it does not know and reports them. | `src/logic.js`, `buildRestoreRe`, `restoreText` |
| mloda-pii | `PII_EMAIL_ADDRESS_<32 hex>` or `..._1` | Keyed HMAC, so the same value gives the same token across runs. | PyPI page |

What this means for Standin:

1. **Never fuzzy-match.** LLM Guard's fuzzy mode allows 3 edits. `Customer_1` and `Customer_7` are 1
   edit apart, so fuzzy restore can put the wrong person's name in a letter. PrivAiTe guards against this
   case explicitly.
2. **Be lenient only in ways that cannot change which stand-in it is:** case (`customer_1`), the joiner
   (`Customer 1`, `Customer-1`, `Customer_1`), and a possessive or punctuation after it (`Customer_1's`,
   `Customer_1,`). Word guards on both sides stop `Customer_1` matching inside `Customer_12`. Nobody
   handles a placeholder split across a line except PrivAiTe's fuzzy pass; Standin could allow a line break
   as a joiner.
3. **Report every unknown stand-in** (`Customer_99` when there are only 40) and every near-miss. Do not
   guess. This fits "fail closed".
4. **Check the input for stand-in look-alikes before scrubbing.** PrivAiTe found that a literal
   `<PERSON_2>` already in the data was turned into a real name on the way back. If a source value already
   matches Standin's stand-in pattern, refuse or pick another prefix.
5. **One pass, longest first** (or a single regex with word guards), never one `replace` per entry.
6. **Normalised keys lose spellings.** noirdoc keys on `strip().lower()` and keeps the first spelling;
   data-masker's "Codes ignore capital letters". So `Ann Lee` and `ann lee ` become one stand-in, and
   restore gives back one spelling. DESIGN.md success check 1 ("restored text equals the original values")
   will fail on such data unless the mapping keeps every original spelling and restore of a scrubbed
   *file* works cell by cell.
7. **Restore outside the agent.** noirdoc's reveal runs in the user's terminal, not in the agent's session,
   so real values never reach the transcript. Standin restores in the browser, which already meets this.
   Keep it so: never offer a restore the agent can call.
8. **Formats.** Bracketed forms (`[EMAIL_1]`, `<<PERSON_1>>`) are easy to find and a person sees at a
   glance they are stand-ins. Bare words (`Customer_1`) read more naturally to the model but are harder to
   find with no false matches. Most tools use brackets plus an upper-case type. Format-preserving fakes
   (privatiser) keep tools working but are easy to mistake for real data.
9. **Numbers in random order.** data-masker numbers codes "in random order", so `PERSON_001` does not
   mean "first row". Counting by first appearance leaks row order. Small cost, worth noting.
10. **Do not hash values into tokens without a secret key** (nopii uses plain `sha256(value)[:8]`; mloda
    uses a keyed HMAC). Standin's counters avoid this entirely.

### A4. Leak paths that agent hooks state they do not cover

Copied word for word, as asked.

**mloda-pii, "Limits"** ([PyPI page](https://pypi.org/project/mloda-pii/), version 0.7.0, read through the
PyPI JSON API on 2026-10-30):

> - The hook guards against accidental exposure, not a model that tries to recover values: a Bash command gets the real
>   value and can return it disguised, which detection misses.
> - Hooks never see `@file` mentions, CLAUDE.md and memory files, the client's own reminders and context (such as its
>   git status), images or compaction summaries; those reach the model unchanged, as do results of Claude Code's own
>   account memory MCP tools (the client drops their rewrites).
> - Untested: whether tools that `claude mcp serve` runs for another MCP client go through the hooks, so do not serve a
>   project that holds personal data that way; and whether `/resume` or compaction sends the original structured MCP
>   result the local transcript keeps next to the rewrite.
> - A tool result reaches the model as it is when the hook process itself crashes or hangs past the 60 s timeout `init`
>   sets, and so does a failed tool other than Bash or a failed Bash command the wrapper cannot reach (`exec`, an
>   `exit` after the command sets its own EXIT trap, most syntax errors, and outside `bypassPermissions` a sourced file
>   that exits, a trailing `\` or a failing open heredoc without an `exit`), or a timed-out one that Claude Code kills
>   instead of moving it to the background (one that starts with `sleep`, or any with
>   `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1`). The verifier then ends the turn, but the result stays in the
>   conversation and the model reads it on the next turn.
> - A Claude Code below 2.1.152 may skip every hook (untested), and one below 2.1.121 ignores the rewritten output of
>   built-in tools in any case. `init` and `status` warn only about the `claude` on `PATH`, not an IDE or desktop
>   client. A Claude Code release that changes the hook interface can let values through unnoticed until mloda-pii
>   follows. Supported platforms are listed under Setup.
> - Claude Code checks an Edit's `old_string` before any hook runs, so an Edit whose `old_string` holds a token fails.
>   The hook tells the model to use a Bash command or Write instead; a Bash command may ask for permission, and a Write
>   from a partial Read cuts the file.
> - Tokens are still personal data under GDPR; whoever holds the key can recompute them.

(The full list also covers detection gaps, IPv4/IPv6 edge cases, timeouts and the licence gate.)

**noirdoc plugin, "What the guard hook does NOT block"** ([README](https://github.com/noirdoc-ai/mask-claude-plugin)):

> | User pastes raw PII into the chat | `PreToolUse` cannot see user prompts. [...] | Redact the source file with `/noirdoc-redact` and paste from the clean copy. |
> | `Grep` / `Glob` with no explicit `path` (defaults to workspace root) | Can surface snippets from protected files in tool output. | Add narrow `path=` arguments to searches on sensitive workspaces, or disable the tool for sensitive sessions. |
> | MCP server tool outputs | MCP servers run in their own processes and return arbitrary text. | Only use MCP servers you trust on sensitive workspaces. |
> | Subagent (`Agent` tool) results | Sub-agents can read content whose bytes then come back as a tool result to the parent. Plugin hooks should apply inside sub-agents, but assume nothing until verified. | Don't dispatch sub-agents onto protected paths without checking the subagent's config. |
> | Content that's already in the session transcript | Every turn is replayed to the API on every subsequent request. There is no Claude Code hook to rewrite outbound API payloads. | Start a fresh session for a new sensitive context. `/compact` summarizes but does not redact. |
> | WebFetch on `http(s)://` URLs that return PII | The plugin has no visibility into remote response bodies. | Treat remote fetches as just another source of user-provided text. |

> **Once content is in any turn of the conversation, it will be sent to the Anthropic API for every subsequent request in that session.**

> **Claude Code's UserPromptSubmit hook can block or append additional context, but cannot rewrite the prompt text.**

It also says its `Bash` check is "Best-effort. Simple cases only. Pipes, process substitution, `$(...)`,
globbing by the shell — **not parsed**", and "The guard fails open: workspaces without
`.noirdoc/config.toml` experience no blocks."

**PrivAiTe, threat model** ([README](https://github.com/crp4222/PrivAiTe)): "The agent's own prompt is not
scanned. The `system` and `instructions` fields pass through as-is, and Claude Code puts your `CLAUDE.md`
there." and "A model inventing a value instead of copying a placeholder: nothing can be restored then."


## Part B. The CLI scope: hooks, running the binary, input formats

### B1. Claude Code hooks and permissions

Sources: [hooks reference](https://code.claude.com/docs/en/hooks) (read as Markdown from
`https://code.claude.com/docs/en/hooks.md`), [permissions](https://code.claude.com/docs/en/permissions),
[settings reference](https://code.claude.com/docs/en/settings-reference),
[sandboxing](https://code.claude.com/docs/en/sandboxing). All read 2026-10-30. Quotes are exact.

#### What each hook can do

| Hook | Block? | Rewrite? | Notes |
|---|---|---|---|
| `PreToolUse` | Yes: `permissionDecision: "deny"` with `permissionDecisionReason` (shown to Claude), or exit 2 with the reason on stderr. | **Yes, the tool's input.** `updatedInput` "Modifies the tool's input parameters before execution. Replaces the entire input object, so include unchanged fields alongside modified ones. Claude Code evaluates permission rules ... against the input your hook returns, not the input Claude sent. Combine with `"allow"` to auto-approve, or `"ask"` to show the modified input to the user." | Fires for built-in tools; the reference documents input fields for Bash, PowerShell, Write, Edit, **Read** (`file_path`, `offset`, `limit`), **Glob** (`pattern`, `path`), **Grep** (`pattern`, `path`, `glob`, ...), WebFetch, WebSearch, Agent and others. "Deny and ask rules are still evaluated regardless of what the hook returns." Precedence when several hooks answer: "`deny` > `defer` > `ask` > `allow`". |
| `PostToolUse` | Not the tool (it already ran). `decision: "block"` "adds the `reason` next to the tool result. Claude still sees the original output". | **Yes, the output, for all tools.** `updatedToolOutput` "Replaces the tool's output with the provided value before it is sent to Claude. The value must match the tool's output shape." `updatedMCPToolOutput` is for MCP tools only; "Prefer `updatedToolOutput`, which works for all tools". | Warning in the docs: "`updatedToolOutput` only changes what Claude sees. ... Telemetry such as OpenTelemetry tool spans and analytics events also captures the original output before the hook runs." And: "For built-in tools, a value that doesn't match the tool's output schema is ignored and the original output is used." The docs give Bash's shape (`stdout`, `stderr`, `interrupted`, `isImage`). **Read's output shape is not documented on that page (unverified).** |
| `UserPromptSubmit` | Yes: `decision: "block"` or exit 2. | **No.** "`UserPromptSubmit`: can't replace the prompt; it only injects `additionalContext` alongside it". | Default timeout 30 s. "A blocked prompt never reaches Claude, but its text isn't removed everywhere": it is written to the local transcript unless `suppressOriginalPrompt` is set, and even then "can still appear in local files such as the session transcript and your prompt history". |
| `MessageDisplay` | No. | Display only. `displayContent` "replaces the displayed text on screen. Display-only: the transcript and what Claude sees keep the original". | **Useful for restore.** It runs as Claude's reply streams to the screen, so a hook could show real values to the person while Claude and the transcript keep stand-ins. "If the hook fails or times out, Claude Code displays the original text" (the stand-ins), which is the safe direction. Default timeout 10 s. |
| `PreCompact` | Yes (blocks compaction). | No. | |

The docs sum it up: "For redaction or transformation use cases, intercept at `PreToolUse` for outbound tool
inputs and `PostToolUse` for inbound tool results."

#### Exit codes, crashes and timeouts

- Exit 0: success; JSON on stdout is applied.
- Exit 2: "Blocking error"; on `PreToolUse` it blocks the call and Claude sees stderr.
- Any other exit code, a hook that cannot start, or invalid JSON: "Non-blocking error ... The action goes
  ahead". The docs warn: "Exit code 1 without valid JSON output is a non-blocking error ... If your hook is
  meant to enforce a policy, use `exit 2`."
- Timeouts: "On `PreToolUse`, a timed-out `command`, `http`, or `mcp_tool` hook doesn't block the tool
  call ... so don't count on a stalled hook to act as a gate." Default timeout on most events is 600 s.
- **So hooks fail open by default.** "On most events, when a hook fails or times out, Claude Code still
  carries out the action. A policy hook with a wrong path or a crashing script then lets everything
  through. To block the action instead, set `"onFailure": "block"` on a `command` or `http` hook. Requires
  Claude Code v2.1.295 or later." With it set, can't start, other exit codes, timeouts and invalid output
  all block.
- Windows: without Git Bash, a shell-form hook runs in PowerShell, and the failure rules differ (see
  "Failed hooks on Windows without Git Bash"). Use exec form (`command` plus `args`) for a `.exe`.

#### Can a PreToolUse hook transparently send raw reads to scrubbed copies?

**For the `Read` tool: yes, by the docs.** A hook matching `Read` can return:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "updatedInput": { "file_path": "/abs/path/scrubbed/invoices.csv" }
  }
}
```

Because permission rules are checked against the rewritten input, a `Read(./raw/**)` deny rule and the
redirect can live together: the redirected path is not under `raw/`, so it is allowed. (I have not run
this; it is the documented behaviour. **Untested.**) Things to know:

- **Claude is not told** unless the hook says so. `permissionDecisionReason` for `"allow"` goes only to
  the debug log. Use `additionalContext` to tell Claude "you were given the scrubbed copy". Being open
  about it is better than silent: otherwise Claude may "fix" stand-ins it thinks are corrupt data.
- `offset` and `limit` must be passed through (the input is replaced whole). Line numbers stay the same
  only if scrubbing keeps one row per line.
- **Grep and Glob** take an optional `path`. A Grep with no `path` searches the whole working directory,
  raw files included. The hook must rewrite or deny it. (noirdoc lists exactly this as a gap.)
- **Edit and Write** on a raw path should be denied, not redirected: an edit made against stand-ins and
  applied to the real file would be wrong. mloda-pii also notes "Claude Code checks an Edit's `old_string`
  before any hook runs".
- **Bash** input is one `command` string. A hook can rewrite it, but cannot reliably see which files a
  command reads (`grep -r`, `python script.py`, `$(...)`). The permissions page says the same of deny
  rules. Deny shell reads of the raw folder with the sandbox instead.
- **Not covered by any hook:** `@file` mentions, `CLAUDE.md` and memory files, text the person pastes,
  images, compaction summaries, and anything already in the transcript (mloda-pii and noirdoc lists in
  A4). `Read` deny rules do reach @-mentions on a best-effort basis (below).
- **Version drift.** mloda-pii says "A Claude Code release that changes the hook interface can let values
  through unnoticed". Standin should check the Claude Code version and fail closed if it is older than
  the features it relies on (`onFailure` needs v2.1.295).

The safest design is still to **keep raw files outside the folder the agent works in**. Then a redirect
hook is a convenience, not the only wall.

#### Permission deny rules

Paste-ready shape (from the settings reference example, folder names changed):

```json
{
  "permissions": {
    "deny": ["Read(./raw/**)", "Edit(./raw/**)", "Read(./private/**)", "Edit(./private/**)"]
  },
  "sandbox": { "enabled": true, "allowUnsandboxedCommands": false }
}
```

- Path forms: `//path` absolute; `~/path` from home; `/path` "relative to the settings source" (not the
  file-system root); `path` or `./path` relative to the current directory. Gitignore syntax. As a deny
  rule, `Read(secrets/**)` "matches a directory named `secrets` at any depth". `.claudeignore` "has no
  effect". `Write(...)`, `Glob(...)` and `NotebookEdit(...)` path rules "never match".
- Coverage, quoted: "Claude makes a best-effort attempt to apply `Read` rules to all built-in tools that
  read files like Grep and Glob, to `@file` mentions in your prompts, and to the selection and open-file
  context that a connected IDE shares with Claude."
- Limits, quoted: "Read and Edit deny rules apply to Claude's built-in file tools, to file commands Claude
  Code recognizes in Bash, such as `cat`, `head`, `tail`, `sed`, and `tee`, and to the targets of Bash
  redirections such as `> file` and `< file`. They don't apply to a command that reads files without
  naming them, such as `grep -r pattern .` run from the directory that holds the file, or to arbitrary
  subprocesses that read or write files indirectly, like a Python or Node script that opens files itself.
  For OS-level enforcement that blocks all processes from accessing a path, enable the sandbox."
- The sandbox's `denyRead` "adds the paths from your `Read(...)` deny permission rules", so with the
  sandbox on, the same rules bind every shell command. But: "The sandbox runs on macOS, Linux, and WSL2.
  On native Windows, Claude Code runs commands unsandboxed." Linux needs `bubblewrap` and `socat`.
- "Permission rules are enforced by Claude Code, not by the model."

### B2. OpenAI Codex CLI and OpenCode

#### Codex

Sources: [hooks](https://learn.chatgpt.com/docs/hooks) and
[permissions](https://learn.chatgpt.com/docs/permissions) (the old `developers.openai.com/codex/...`
URLs redirect there). Read 2026-10-30.

- **Events:** `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`,
  `UserPromptSubmit`, `SubagentStop`, `Stop` and others. "Hooks are enabled by default." Default timeout
  600 s.
- **Which tools:** shell commands and unified exec (matched as `Bash`), `apply_patch` (matched as
  `apply_patch`, `Edit` or `Write`), MCP tools and other local function tools. Hosted tools such as web
  search are not hooked. **Codex has no separate file-read tool in this list; it reads files through shell
  commands.** The docs warn: "Some specialized tool paths can opt out of the default hook path. Treat
  tool hooks as a useful guardrail, not a complete enforcement boundary."
- **PreToolUse:** can deny (`permissionDecision: "deny"`, the old `decision: "block"`, or exit 2), add
  context, or rewrite: "To rewrite a supported tool call without blocking, return `permissionDecision:
  "allow"` with `updatedInput`". "For Bash commands and apply_patch, `updatedInput` must include a string
  `command` field." `"ask"`, `continue: false` and others "are parsed but not supported yet. Codex marks
  the hook run as failed, reports the error, and continues the tool call."
- **PostToolUse:** cannot rewrite output to a value of your choice. "`decision: "block"` doesn't undo the
  completed Bash command. Instead, Codex records the feedback, replaces the tool result with that
  feedback". "`updatedMCPToolOutput` and `suppressOutput` are parsed but not supported yet."
- **UserPromptSubmit:** can block (`decision: "block"` or exit 2) or add context. No rewrite.
- **Failure:** for managed MCP hooks, "a PreToolUse callback error, timeout, or malformed response can
  fail the hook without blocking the tool". For command hooks, unsupported output fields fail the hook and
  "continue the tool call". So **Codex hooks fail open** too. I found no `onFailure`-style switch.
- **Folder deny (stronger):** permission profiles (beta) enforce in the sandbox, for every process. From
  the docs: `deny` "denies access and wins over equally specific write or read entries." Supported "on
  macOS, Linux, WSL, and native Windows". Example adapted from the docs (folder entries are my reading;
  **untested**):

```toml
default_permissions = "standin"

[permissions.standin]
extends = ":workspace"

[permissions.standin.filesystem.":workspace_roots"]
"raw" = "deny"
"**/*.mapping.json" = "deny"
```

  Caveats from the docs: if `sandbox_mode` is set anywhere, Codex uses the older settings "instead of
  `default_permissions`"; on Linux, WSL and Windows, deny globs are listed when each sandboxed process
  starts, and "Matching files created afterward are not added to that process's deny list".

#### OpenCode

Sources: [plugins](https://opencode.ai/docs/plugins/), [permissions](https://opencode.ai/docs/permissions/),
and the published type file `@opencode-ai/plugin` 1.19.5 (`dist/index.d.ts`, from the npm registry). Repo
[anomalyco/opencode](https://github.com/anomalyco/opencode) (MIT; `sst/opencode` redirects there). Read
2026-10-30.

- **Permissions:** keyed by tool: `read` ("matches the file path"), `edit`, `glob`, `grep`, `bash`
  ("matches parsed commands"), `external_directory` and others; values `allow`, `ask`, `deny`. "Rules are
  evaluated by pattern match, with the last matching rule winning." `read` defaults to allow, with `.env`
  files set to ask. These are tool-level checks; I found **no OS sandbox** in the permissions page, so a
  shell command or script can still read a denied folder (**inferred, not stated**).
- **Plugins** (JavaScript or TypeScript, run inside OpenCode, so Standin would need a small JS shim that
  calls the binary):
  - `tool.execute.before(input, output)`: the docs show changing `output.args.command` for bash and
    `output.args.filePath` checks for `read`, and blocking by throwing: `throw new Error("Do not read .env
    files")`. So **redirecting a `read` to a scrubbed copy by changing `output.args.filePath` looks
    possible**, by the same pattern. **Untested.**
  - `tool.execute.after(input, output)` gets `output: { title, output: string, metadata }`. The type
    suggests the output string can be changed in place; the docs do not say so. **Unverified.**
  - `experimental.chat.messages.transform` and `experimental.chat.system.transform` can change the
    messages and system prompt before they go to the model. Marked experimental. This would let Standin
    scrub the whole outgoing request, the gap Claude Code hooks cannot close. **Untested.**
  - Failure behaviour of a throwing or hanging plugin, other than the documented throw-to-block: not
    documented (**unverified**).

### B3. Running an unsigned Go binary

#### macOS on Apple silicon

| Fact | Finding | Source |
|---|---|---|
| Apple silicon needs a signature to run arm64 code | Yes. Go binaries were "killed at start" until the linker signed them. | [golang/go#42684](https://github.com/golang/go/issues/42684) (closed 2020-12-01) |
| Go's linker ad-hoc signs darwin/arm64 | **Yes, with the internal linker**, which is what a `CGO_ENABLED=0` cross-build from Linux uses. `NeedCodeSign()` returns `ctxt.IsDarwin() && ctxt.IsARM64()`, and the linker writes an `LC_CODE_SIGNATURE` via `cmd/internal/codesign` when `ctxt.IsInternal() && ctxt.NeedCodeSign()`. Added by CL 272256 "cmd/link: code-sign on darwin/arm64" (Go 1.16). A Homebrew maintainer in the issue: "a linker or other ad hoc signature is sufficient to run the binary ... on any machine." | `src/cmd/link/internal/ld/lib.go`, `macho.go` on `master`, read 2026-10-30 |
| An ad-hoc signature is not a Developer ID | Right. It lets the code run, but Gatekeeper still treats a quarantined download as from an unidentified developer. | Apple: downloaded software should be "signed by a registered developer and notarized by Apple" ([Gatekeeper and runtime protection](https://support.apple.com/guide/security/gatekeeper-and-runtime-protection-sec5599b66df/web)) |
| Who sets the quarantine flag | The app that writes the file, if it opts in. Apple's `LSFileQuarantineEnabled`: "A Boolean value indicating whether the files this app creates are quarantined by default." Browsers set it. `curl` is a command-line tool with no app bundle, so a file fetched with `curl` in Terminal is **not** quarantined. (Widely known; the curl part is inferred from Apple's wording, **not stated by Apple**.) Files unpacked from a quarantined zip by Archive Utility are also quarantined (**unverified at a primary source**). | [LSFileQuarantineEnabled](https://developer.apple.com/documentation/bundleresources/information-property-list/lsfilequarantineenabled) |
| Sequoia removed the Control-click bypass | "In macOS Sequoia, users will no longer be able to Control-click to override Gatekeeper when opening software that isn't signed correctly or notarized. They'll need to visit System Settings > Privacy & Security". | [Apple developer news, 2024-08-06](https://developer.apple.com/news/?id=saqachfa) |
| The steps a person must follow | Try to open it; then "Open System Settings." "Click Privacy & Security, scroll down, and click the Open Anyway button"; then confirm with Open. | [support.apple.com/102445](https://support.apple.com/en-us/102445) |
| Notarizing a bare binary | Possible with a paid Apple Developer ID, but "Although tickets are created for standalone binaries, it's not currently possible to staple tickets to them." Gatekeeper then looks the ticket up online. You can notarize a zip but "can't staple to it directly". | [Customizing the notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow) |

Least scary path for a non-technical person, in order:

1. **Best: sign with a Developer ID and notarize** (costs a yearly Apple developer membership; Ryan's
   call). Ship inside a notarized, stapled `.pkg` or `.dmg` so it also works offline. Then double-click
   just works.
2. **No Apple account:** give one line to paste in Terminal that fetches the file with `curl` (no
   quarantine flag) and runs it. Short, but asking a layperson to paste a command is its own risk, and it
   needs network access.
3. **Browser download:** expect the "cannot be opened" dialog, then System Settings > Privacy & Security
   > Open Anyway. Write these steps with screenshots. Avoid telling people to run `xattr -d
   com.apple.quarantine`; it teaches them to switch off a safety check.
4. Managed (locked-down) Macs may block all of this through device management (**unverified**; depends on
   the employer's settings).

#### Windows

- SmartScreen checks a downloaded file "against a list of files that are well known and downloaded
  frequently. If the file isn't on that list, Microsoft Defender SmartScreen shows a warning". Reputation
  attaches to the file and the signing certificate: "If there's no reputation, the item is marked as a
  higher risk and presents a warning". ([SmartScreen overview](https://learn.microsoft.com/en-us/windows/security/operating-system-security/virus-and-threat-protection/microsoft-defender-smartscreen/), updated 2026-04-23.)
  The person can usually choose "More info" then "Run anyway" (**common knowledge; not stated on that
  page**).
- **Smart App Control (Windows 11) blocks it outright when on:** "Malware, Potentially Unwanted Apps
  (PUA), and unknown, unsigned code are blocked by default." In enforcement mode "Apps cannot be run
  unless they are recognized by Microsoft's app intelligence services, or they are signed with a trusted
  certificate." ([Smart App Control](https://learn.microsoft.com/en-us/windows/apps/develop/smart-app-control/overview), updated 2026-09-30.)
  So **on Windows, signing with a trusted code-signing certificate is close to required**.
- Company laptops often add AppLocker or App Control policies that block any program run from Downloads
  (**unverified; varies by employer**). AGENTS.md describes a laptop where `python3`, `node` and `git` are
  installer stubs. That person may not be able to run a downloaded binary at all. The browser page did not
  have this problem.

### B4. Export formats from accounting tools and banks

| Format | What it looks like | Fields that matter | Quirks for a parser | Source |
|---|---|---|---|---|
| **OFX 1.x** (1.0.2, 1.6) | A plain-text header block, then SGML. Header: `OFXHEADER:100`, `DATA:OFXSGML`, `VERSION:102`, `SECURITY:NONE`, `ENCODING:USASCII`, `CHARSET:NONE`, `COMPRESSION:NONE`, `OLDFILEUID:NONE`, `NEWFILEUID:NONE`. | Same as 2.x below. | Not XML. End tags on leaf elements were optional before 2.x (the 2.3 spec says "OFX 2.2 requires the use of end tags", implying 1.x did not). `CHARSET` is often `1252` in real files (**common; not checked against the 1.x spec**). Do not use an XML parser; write a small tag reader. | [OFX 2.3 spec](https://financialdataexchange.org/wp-content/uploads/2025/12/OFX-Banking-Specification-v2.3.pdf) §2.8 (dated 10/16/2020); 1.0.2 and 1.6 zips on the [FDX OFX page](https://financialdataexchange.org/about-fdx/ofx-work-group/) |
| **OFX 2.x** (2.0.3 to 2.3) | XML, with `<?OFX OFXHEADER="200" VERSION="220" SECURITY="NONE" OLDFILEUID="NONE" NEWFILEUID="NONE"?>`. | In `<STMTTRN>`: `TRNTYPE`, `DTPOSTED`, `DTUSER`, `TRNAMT` ("signed from the perspective of the customer"), `FITID` (bank's transaction ID, "Used to detect duplicate downloads"), `CHECKNUM` (A-12), `REFNUM` (A-32), `PAYEEID`, `NAME` ("Name of payee or description of transaction, A-32") or a `PAYEE` aggregate, `EXTDNAME` (A-100), `MEMO`. Accounts in `BANKACCTFROM` (`BANKID`, `ACCTID`). | Dates are `YYYYMMDDHHMMSS.XXX [gmt offset[:tz name]]` with shorter forms allowed. `NAME` is cut at 32 characters, so the same payee can appear truncated. `ACCTID`, `BANKID`, `NAME`, `MEMO`, `PAYEE` are the sensitive fields; `FITID` may embed account data. | OFX 2.3 spec §3.2.8.1, §11.4.4.1 |
| **QFX** (Quicken) and **QBO** (QuickBooks Web Connect) | OFX with Intuit additions, mainly an `INTU.BID` bank ID. | As OFX. | Treat as OFX; keep the unknown `INTU.*` tags. **No primary Intuit spec found (unverified).** | Secondary sources only |
| **QuickBooks Online bank CSV** | 3 columns: "Date, Description, and Amount", or 4: "Date, Description, Credit, and Debit". | | "Make sure the dates are in one format. We recommend you use: dd/mm/yyyy" (Canada page; US pages may differ). "Remove any zeroes (0) from the file. Leave those cells blank." "Mac users must save the file as a Windows CSV file." Credit card files may have signs reversed. Also accepts QFX, QBO and OFX uploads. | [Intuit help (en-CA)](https://quickbooks.intuit.com/learn-support/en-ca/help-article/bank-transactions/format-csv-files-excel-get-bank-transactions/L4BjLWckq_CA_en_CA) |
| **QuickBooks Desktop IIF** | "ASCII text, TSV (Tab-Separated Value) files"; "The file headers lines begin with !TRNS, !SPL, and !ENDTRNS." Each transaction is a `TRNS` line, `SPL` lines, then `ENDTRNS`. | Names, accounts, memo, amount per split. | Tab-separated, header rows start with `!`, several record types in one file, so not a simple table. Desktop only. Full field list is in QuickBooks Help, not online (**not read**). | Intuit "IIF Overview" article (via search; page moved, the old URL returns 404) |
| **QuickBooks reports** (both) | Excel or CSV export of reports (customer lists, invoices). | Report-dependent. | Report exports often have title rows above the header and total rows at the end (**common; unverified**). Standin must let the person choose the header row, or refuse. | Unverified |
| **Xero bank statement CSV** | One signed amount column plus payee, description, reference. | `Date`, `Amount`, `Payee`, `Description`, `Reference`, `Cheque Number` (per third-party guides). | Dates follow the organisation's region settings. **Official Xero page could not be fetched (unverified).** | Secondary sources |
| **Wave** | Bank statement upload by CSV, mapping date, description and amount during upload; Wave says OFX, QBO and QFX import more easily than CSV. | Date, description, amount. | Official page returned 403 to the fetch; facts from the search summary of [Wave's help article](https://support.waveapps.com/hc/en-us/articles/208621626-Upload-a-bank-or-credit-card-statement-in-csv-format) (**partly unverified**). | |
| **FreshBooks** | Invoice export as CSV ("Export Invoices", active and archived); reports as CSV or PDF. | Columns not documented publicly. | **Unverified**; export a sample from a test account. | [FreshBooks support](https://support.freshbooks.com/hc/en-us/articles/360032628152) (via search) |

Which are most common for a small business: I found no primary data on market share. From what the
tools accept, **bank CSV and OFX/QFX/QBO** are the formats every accounting tool imports, so they are the
safest first targets after plain CSV. IIF is Desktop-only and declining (**unverified**).

Parsing points that matter for Standin:

1. **Encodings.** Bank CSVs from Windows tools are often Windows-1252, and some exports are UTF-16 with a
   BOM. In Go, `unicode/utf8.Valid` is the strict check; `golang.org/x/text/encoding/charmap` has
   Windows-1252 (an extra module, not the standard library). Fail closed and ask (see the encoding rows in the appendix).
2. **Header row is not always row 1.** Report exports and some bank files put a title, account number
   and date range above the header. The account number in that preamble is sensitive. Default-deny must
   cover preamble rows, not only data rows.
3. **Credit/debit split and sign flips** (QuickBooks credit card note). Keep amounts as text unless the
   config says keep; never "fix" signs.
4. **Truncated names** (OFX `NAME` A-32) mean the same payee may appear as two strings. Tokenising them
   as two stand-ins is safe; merging them is a later feature.
5. **Formula injection on output.** A restored CSV opened in Excel can run cells starting with `=`, `+`,
   `-`, `@` (data-masker guards this; see A2a).
6. **OFX is two formats.** SGML 1.x and XML 2.x need different readers; detect by the first line.

## Findings that should change the design

1. **A Claude Code `PreToolUse` hook can redirect `Read` to the scrubbed copy.** `updatedInput` replaces
   `file_path`, and permission rules are checked against the new path, so a `Read(./raw/**)` deny rule
   still works as a backstop. Tell Claude it got the scrubbed copy (`additionalContext`); pass `offset`
   and `limit` through. Documented, **not yet tested**: build a fixture test first.
2. **Hooks fail open unless told not to.** A crash, a non-0/2 exit, bad JSON or a timeout lets the call
   through. Every Standin hook entry must set `"onFailure": "block"` (Claude Code v2.1.295+), use exec
   form, and exit 2 (not 1) to block. Standin should check the Claude Code version and refuse to install
   on older ones. Codex has no such switch that I found, so in Codex treat hooks as a guide and rely on
   the sandbox profile.
3. **`PostToolUse` can replace built-in tool output** (`updatedToolOutput`), Bash included. This allows a
   second layer: scrub any raw value that slips into a tool result. But a value that does not match the
   tool's output shape is silently ignored, and telemetry sees the original. Read's output shape is
   undocumented, so test it.
4. **Use `MessageDisplay` for restore on screen.** It changes only what the person sees; Claude and the
   transcript keep stand-ins, and a failure shows stand-ins. That is exactly Standin's restore rule.
   Restoring into *files* the agent wrote should still be a separate, explicit command the person runs,
   in the way noirdoc's `reveal` runs outside the agent.
5. **Hooks cannot cover prompts, `@file`, `CLAUDE.md`, pastes or compaction.** `UserPromptSubmit` can
   block but not rewrite. So the hook can only refuse a prompt that contains a known real value (from the
   mapping) and ask the person to use the stand-in. The `@file` gap is partly closed by `Read` deny rules
   ("best-effort").
6. **Grep, Glob and Bash need deny, not redirect.** Grep or Glob with no `path` searches the raw folder;
   Bash commands cannot be parsed reliably. Deny Grep/Glob calls that would reach the raw folder, and turn
   on the sandbox so shell commands cannot read it (not available on native Windows). Best of all: keep
   raw files and the mapping outside the agent's folder.
7. **Distribution is the hardest part for the target user.** An unsigned binary from a browser is
   quarantined on a Mac (Sequoia: System Settings > Open Anyway), and **Windows Smart App Control blocks
   unsigned code**. The AGENTS.md user (locked-down laptop, installer stubs) may not be able to run the
   binary at all. This conflicts with the original requirement; Ryan should decide on code signing (Apple
   Developer ID and a Windows certificate cost money) and whether to keep a browser fallback.
8. **data-masker** (MIT, one HTML file, CSV/Excel, match file, restore of codes in an AI answer) is the
   closest prior art to the old browser plan. Borrow its formula guard, run ID in both file names and "do
   not open the match file in Excel" warning, with credit.
9. **Restore must be strict.** Case and joiner leniency only, word guards, one pass, report unknown
   stand-ins, no fuzzy matching. Before scrubbing, refuse input that already contains stand-in-shaped text.
10. **Keep every original spelling in the mapping.** Normalising before keying merges `Ann Lee` and
    `ann lee`; restore then gives one spelling. Restore a scrubbed file by cell position to get exact
    values back.
11. **Input formats:** after CSV/TSV/JSON, OFX/QFX/QBO is the format every accounting tool accepts.
    It is two formats (SGML 1.x, XML 2.x). Header rows are not always row 1, and the preamble often holds
    the account number. Bank CSVs are often Windows-1252: fail closed and ask.
12. **A cheap k-anonymity count** (smallest group, unique rows, rows in groups under 5) over the kept
    columns turns DESIGN.md 6.6's "Not addressed" into a useful warning.

## Things that contradict DESIGN.md

- **DESIGN.md 6.5** says a hook is "a later phase" and that agent settings enforce the folder rule.
  Under the new scope the hook is the main entry point. And settings alone do not enforce the rule
  against shell commands or scripts; only the sandbox does, and not on native Windows.
- **DESIGN.md 6.5, "a crashed hook can let the original through"**: true by default, but Claude Code now
  has `"onFailure": "block"`, which closes that path.
- **DESIGN.md 2 and AGENTS.md** (no installs on a locked-down laptop) conflict with a downloaded binary.
- **Presidio**: the `encrypt`/`decrypt` operators exist but are AES-CBC with a random IV, so the same value
  encrypts differently each time and rows cannot be matched. Numbered placeholders are a sample, not a
  feature. `presidio-structured` has no restore.
- **scrubadub**: DESIGN.md says MIT. The GitHub API says Apache-2.0; PyPI says MIT.
- **noirdoc**: MIT and open source; repos moved to `noirdoc-ai/`. Python 3.12+, not spaCy by default.
- **shutapp** and **Cloak**: not found. The claim that shutapp sends text to a hosted API is unverified.
- **"No single-file browser tool with a round-trip exists"**: no longer true (data-masker).
- **DESIGN.md 6.6**: re-identification by combinations can be partly checked cheaply.

## Not checked

- Any hook actually running. Every hook claim above is from the documentation, not a test.
- The `Read` tool's output shape for `updatedToolOutput`.
- OpenCode `tool.execute.after` output mutation and plugin failure behaviour.
- Codex permission profiles in practice.
- Gatekeeper behaviour on a real Mac, including quarantine from Archive Utility and from `curl`.
- Windows SmartScreen "Run anyway" wording, and company App Control policies.
- Official Xero, Wave and FreshBooks export column lists; the QFX/QBO `INTU.*` tags; OFX 1.x
  `CHARSET` values in real files.
- From the browser appendix: real Safari, Edge, drag and drop, DNS prefetch under CSP.
- The full mloda-pii licence text, and whether PresidioJS and Rampart can be reused.

## Appendix. Browser facts (earlier single-HTML scope)

These were tested before the scope changed to a CLI. They still matter if Standin keeps a browser page
(for example a restore screen, or a fallback for laptops that block downloaded programs).

### How I tested

I wrote a test page (`probe.html`) with the CSP below, opened it from `file://` in three engines through
Playwright 1.64.1 on Linux, and ran a small local web server that logged every request that reached it.
The page tried 20 kinds of request to that server. The harness also loaded a file through the file input,
clicked a Blob download link, and clicked an ordinary link.

- Chromium 156.0.8078.4
- WebKit 27.2 (Playwright's Linux build. **This is the Safari engine, not Safari.** Safari's own download
  handling, sandbox and settings differ. Safari on a Mac is untested.)
- Firefox 157.0

The test files are in the scratchpad, not the repo. They could become the repo's "no network" check.

The CSP used:

```html
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline';
  style-src 'unsafe-inline'; img-src data: blob:; form-action 'none'; base-uri 'none';
  object-src 'none'; frame-src 'none'; worker-src 'none'; connect-src 'none'; font-src 'none';
  media-src 'none'; manifest-src 'none'">
```

### Results

| Fact | Chrome / Chromium | Safari / WebKit | Firefox | Sources |
|---|---|---|---|---|
| **W1. `file://` page is a secure context** | Works. `isSecureContext` true. | Works in WebKit 27.2. Safari itself unverified. | Works. `isSecureContext` true. Note `location.origin` is the string `"null"` in Firefox, `"file://"` in the others. | Spec: "the user agent SHOULD treat `file` URLs as potentially trustworthy", but "MAY choose to more strictly assign trust in a way which excludes `file`" ([Secure Contexts ED, 10 Nov 2023, §3.1](https://w3c.github.io/webappsec-secure-contexts/)). [MDN](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts) (modified 2026-09-14) lists `file:///path/to/resource.html` as secure. Test 2026-10-30. |
| `crypto.getRandomValues` | Works. | Works. | Works. | Does not need a secure context. BCD: Chrome 11, Firefox 21, Safari 5. |
| `crypto.subtle` (SHA-256 digest, AES-GCM key) | Works. | Works. | Works. | Needs a secure context. BCD: Chrome 37, Firefox 34, Safari 11. Test 2026-10-30. |
| `crypto.randomUUID` | Works. | Works. | Works. | BCD: Chrome 92, Firefox 95, Safari 15.4. |
| **W2. Meta CSP applies on `file://`** | Yes. All 18 subresource probes blocked; 20 violation events. | Yes. 19 violation events. | Yes. 19 violation events. | BCD `meta-element-support`: Chrome 25, Firefox 45, Safari 7. Test 2026-10-30. |
| Blocked by the CSP above (none reached the server) | fetch, XHR, sendBeacon, WebSocket, EventSource, img, CSS background, `@font-face`, `<link rel=stylesheet>`, `rel=icon`, `rel=preload`, `rel=prefetch`, `rel=preconnect`, `<script src>`, `<iframe>`, `<object>`, `<video>` src and poster, remote `Worker`, form submit | Same | Same | Test 2026-10-30. Note `connect-src 'none'` also blocks `fetch('data:...')`. |
| **Not blocked: navigation** | `window.open(url)` and a link click both reached the server. | Same. | Same; the new window also fetched `/favicon.ico`. | CSP does not cover top-level navigation. `form-action` is the only navigation directive and it is not a fetch directive, so `default-src` does not cover it: it must be set by itself ([CSP3 ED, 27 Oct 2026](https://w3c.github.io/webappsec-csp/), §6.4.1 and the fetch-directive fallback list). Test 2026-10-30. |
| Prefetch, preconnect | Blocked in the test. | Blocked. | Blocked. | CSP3: resource hints "are instead governed by the union of servers allowed in all of a policy's directives' source lists. If default-src is not specified, these requests will always be allowed." `prefetch-src` is deprecated: not in Chrome or Firefox, Safari 16.3 (BCD). So always set `default-src 'none'`. |
| DNS prefetch | **Unverified** (the test cannot see DNS lookups). | Unverified. | Unverified. | CSP3 does not mention DNS prefetch. Add `<meta http-equiv="x-dns-prefetch-control" content="off">` and, more important, never put a user's value in an `href`. |
| Meta CSP limits | | | | `report-uri`, `frame-ancestors`, `sandbox` and Report-Only do not work in `<meta>`; "policies in `meta` elements are not applied to content which precedes them" (CSP3). So the meta tag must come first in `<head>`. |
| **W3. File input: `File.text()`, `FileReader`** | Works. | Works in WebKit. | Works. | BCD `Blob.text`: Chrome 76, Firefox 69, Safari 14. **Caveat:** the spec says `text()` runs "UTF-8 decode" ([File API](https://w3c.github.io/FileAPI/)), which never fails: bad bytes become `�`. The test file with Windows-1252 quotes came back as `�Ann�`. So read with `arrayBuffer()` and decode with `TextDecoder` yourself. |
| Drag and drop | **Untested** (Playwright did not drive it). | Untested. | Untested. | Same `File` object as the input, so expected to work. Test by hand. |
| Save with Blob + `<a download>` | Works: file saved with the right name and content, under the CSP. | Works in WebKit. **Safari unverified.** | Works. | BCD `a.download`: Chrome 14, Firefox 20, Safari 10.1. WebKit bugs: [190351](https://bugs.webkit.org/show_bug.cgi?id=190351) (Safari 12 regression, fixed 2018; `target="_blank"` triggers the popup blocker and then Safari loads the blob instead of saving it) and [211234](https://bugs.webkit.org/show_bug.cgi?id=211234) (revoking the object URL right after the click broke downloads on iOS; duplicate of 212279). So: no `target`, keep the URL alive a while before `revokeObjectURL`. No bug found that is specific to `file://` pages. |
| **W4. `TextDecoder` encodings** | `windows-1252`, `utf-16le`, `utf-16be`, `macintosh`, `iso-8859-15`, `windows-1250`, `shift_jis`, `gbk` all work. | Same. | Same. | Per the [Encoding Standard](https://encoding.spec.whatwg.org/encodings.json), `latin1`, `iso-8859-1`, `ascii` and `us-ascii` are **labels for windows-1252**, and `utf-16` means `utf-16le`. The test confirmed this in all three. |
| Detect non-UTF-8 with `{fatal:true}` | Works: bytes `93 41 94` throw `TypeError`. | Same. | Same. | BCD `TextDecoder.fatal`: Chrome 38, Firefox 36, Safari 10.1. A UTF-8 BOM is stripped by default. **Caveat:** pure ASCII and some short Windows-1252 text is also valid UTF-8, so passing the check does not prove the encoding. UTF-16 without a BOM will not throw reliably; check for BOMs (`EF BB BF`, `FF FE`, `FE FF`) first, then `fatal` UTF-8, then ask the person (Windows-1252 is the common answer for bank exports). |
| **W5. `DecompressionStream('deflate-raw')`** | Works. Since Chrome 103 (the stream itself since 80). | Works in WebKit. Since Safari 16.4. | Works. Since Firefox 113. | BCD `api/DecompressionStream.json`. Test 2026-10-30 (round trip with `CompressionStream`). Enough to unzip an `.xlsx` with no library (you still need to read the zip directory and the sheet XML yourself). |

