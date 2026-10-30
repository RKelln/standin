# Prior art and browser facts (research, 2026-10-30)

**Question.** Does a tool already do what Standin plans? What can we learn from the tools that come close?
And do the browser features the MVP needs work on a page opened from a folder (`file://`)?

**Short answer.** One tool comes very close: **data-masker** (MIT, one offline HTML file, CSV and Excel,
codes, a "match file", restore from an AI answer). It is ten days old, has no users yet, and differs from
Standin in ways that matter. Read the "Does anything make Standin unnecessary?" section. Every browser
feature we need works in current Chromium, WebKit and Firefox builds, opened from `file://`, with a
strict content security policy. A content security policy does **not** stop a page from opening a link
or a new window. That is the one gap.

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

## Part B. Browser facts

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
| **B1. `file://` page is a secure context** | Works. `isSecureContext` true. | Works in WebKit 27.2. Safari itself unverified. | Works. `isSecureContext` true. Note `location.origin` is the string `"null"` in Firefox, `"file://"` in the others. | Spec: "the user agent SHOULD treat `file` URLs as potentially trustworthy", but "MAY choose to more strictly assign trust in a way which excludes `file`" ([Secure Contexts ED, 10 Nov 2023, §3.1](https://w3c.github.io/webappsec-secure-contexts/)). [MDN](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts) (modified 2026-09-14) lists `file:///path/to/resource.html` as secure. Test 2026-10-30. |
| `crypto.getRandomValues` | Works. | Works. | Works. | Does not need a secure context. BCD: Chrome 11, Firefox 21, Safari 5. |
| `crypto.subtle` (SHA-256 digest, AES-GCM key) | Works. | Works. | Works. | Needs a secure context. BCD: Chrome 37, Firefox 34, Safari 11. Test 2026-10-30. |
| `crypto.randomUUID` | Works. | Works. | Works. | BCD: Chrome 92, Firefox 95, Safari 15.4. |
| **B2. Meta CSP applies on `file://`** | Yes. All 18 subresource probes blocked; 20 violation events. | Yes. 19 violation events. | Yes. 19 violation events. | BCD `meta-element-support`: Chrome 25, Firefox 45, Safari 7. Test 2026-10-30. |
| Blocked by the CSP above (none reached the server) | fetch, XHR, sendBeacon, WebSocket, EventSource, img, CSS background, `@font-face`, `<link rel=stylesheet>`, `rel=icon`, `rel=preload`, `rel=prefetch`, `rel=preconnect`, `<script src>`, `<iframe>`, `<object>`, `<video>` src and poster, remote `Worker`, form submit | Same | Same | Test 2026-10-30. Note `connect-src 'none'` also blocks `fetch('data:...')`. |
| **Not blocked: navigation** | `window.open(url)` and a link click both reached the server. | Same. | Same; the new window also fetched `/favicon.ico`. | CSP does not cover top-level navigation. `form-action` is the only navigation directive and it is not a fetch directive, so `default-src` does not cover it: it must be set by itself ([CSP3 ED, 27 Oct 2026](https://w3c.github.io/webappsec-csp/), §6.4.1 and the fetch-directive fallback list). Test 2026-10-30. |
| Prefetch, preconnect | Blocked in the test. | Blocked. | Blocked. | CSP3: resource hints "are instead governed by the union of servers allowed in all of a policy's directives' source lists. If default-src is not specified, these requests will always be allowed." `prefetch-src` is deprecated: not in Chrome or Firefox, Safari 16.3 (BCD). So always set `default-src 'none'`. |
| DNS prefetch | **Unverified** (the test cannot see DNS lookups). | Unverified. | Unverified. | CSP3 does not mention DNS prefetch. Add `<meta http-equiv="x-dns-prefetch-control" content="off">` and, more important, never put a user's value in an `href`. |
| Meta CSP limits | | | | `report-uri`, `frame-ancestors`, `sandbox` and Report-Only do not work in `<meta>`; "policies in `meta` elements are not applied to content which precedes them" (CSP3). So the meta tag must come first in `<head>`. |
| **B3. File input: `File.text()`, `FileReader`** | Works. | Works in WebKit. | Works. | BCD `Blob.text`: Chrome 76, Firefox 69, Safari 14. **Caveat:** the spec says `text()` runs "UTF-8 decode" ([File API](https://w3c.github.io/FileAPI/)), which never fails: bad bytes become `�`. The test file with Windows-1252 quotes came back as `�Ann�`. So read with `arrayBuffer()` and decode with `TextDecoder` yourself. |
| Drag and drop | **Untested** (Playwright did not drive it). | Untested. | Untested. | Same `File` object as the input, so expected to work. Test by hand. |
| Save with Blob + `<a download>` | Works: file saved with the right name and content, under the CSP. | Works in WebKit. **Safari unverified.** | Works. | BCD `a.download`: Chrome 14, Firefox 20, Safari 10.1. WebKit bugs: [190351](https://bugs.webkit.org/show_bug.cgi?id=190351) (Safari 12 regression, fixed 2018; `target="_blank"` triggers the popup blocker and then Safari loads the blob instead of saving it) and [211234](https://bugs.webkit.org/show_bug.cgi?id=211234) (revoking the object URL right after the click broke downloads on iOS; duplicate of 212279). So: no `target`, keep the URL alive a while before `revokeObjectURL`. No bug found that is specific to `file://` pages. |
| **B4. `TextDecoder` encodings** | `windows-1252`, `utf-16le`, `utf-16be`, `macintosh`, `iso-8859-15`, `windows-1250`, `shift_jis`, `gbk` all work. | Same. | Same. | Per the [Encoding Standard](https://encoding.spec.whatwg.org/encodings.json), `latin1`, `iso-8859-1`, `ascii` and `us-ascii` are **labels for windows-1252**, and `utf-16` means `utf-16le`. The test confirmed this in all three. |
| Detect non-UTF-8 with `{fatal:true}` | Works: bytes `93 41 94` throw `TypeError`. | Same. | Same. | BCD `TextDecoder.fatal`: Chrome 38, Firefox 36, Safari 10.1. A UTF-8 BOM is stripped by default. **Caveat:** pure ASCII and some short Windows-1252 text is also valid UTF-8, so passing the check does not prove the encoding. UTF-16 without a BOM will not throw reliably; check for BOMs (`EF BB BF`, `FF FE`, `FE FF`) first, then `fatal` UTF-8, then ask the person (Windows-1252 is the common answer for bank exports). |
| **B5. `DecompressionStream('deflate-raw')`** | Works. Since Chrome 103 (the stream itself since 80). | Works in WebKit. Since Safari 16.4. | Works. Since Firefox 113. | BCD `api/DecompressionStream.json`. Test 2026-10-30 (round trip with `CompressionStream`). Enough to unzip an `.xlsx` with no library (you still need to read the zip directory and the sheet XML yourself). |

### B6. Keeping the AI agent out of the raw folder

**Claude Code** ([permissions](https://code.claude.com/docs/en/permissions),
[settings reference](https://code.claude.com/docs/en/settings-reference),
[sandboxing](https://code.claude.com/docs/en/sandboxing), read 2026-10-30).

Settings snippet (in `.claude/settings.json`; `./raw/**` and `./private/**` are examples):

```json
{
  "permissions": {
    "deny": [
      "Read(./raw/**)",
      "Edit(./raw/**)",
      "Read(./private/**)",
      "Edit(./private/**)"
    ]
  },
  "sandbox": {
    "enabled": true,
    "allowUnsandboxedCommands": false
  }
}
```

Path forms, from the docs: `//path` is an absolute path, `~/path` is from home, `/path` is "relative to
the settings source" (not the file system root), and `path` or `./path` is relative to the current
directory. They use gitignore syntax. As a deny rule, `Read(secrets/**)` "matches a directory named
`secrets` at any depth". A `.claudeignore` file "has no effect".

What deny rules cover, quoted:

> Claude makes a best-effort attempt to apply `Read` rules to all built-in tools that read files like Grep
> and Glob, to `@file` mentions in your prompts, and to the selection and open-file context that a
> connected IDE shares with Claude.

> Read and Edit deny rules apply to Claude's built-in file tools, to file commands Claude Code recognizes in
> Bash, such as `cat`, `head`, `tail`, `sed`, and `tee`, and to the targets of Bash redirections such as
> `> file` and `< file`. They don't apply to a command that reads files without naming them, such as
> `grep -r pattern .` run from the directory that holds the file, or to arbitrary subprocesses that read or
> write files indirectly, like a Python or Node script that opens files itself. For OS-level enforcement
> that blocks all processes from accessing a path, enable the sandbox.

> Permission rules are enforced by Claude Code, not by the model. Instructions in your prompt or `CLAUDE.md`
> shape what Claude tries to do, but they don't change what Claude Code allows.

And the sandbox: `sandbox.filesystem.denyRead` "adds the paths from your `Read(...)` deny permission
rules", so turning the sandbox on makes the same rules apply to every shell command at the OS level. But:
"The sandbox runs on macOS, Linux, and WSL2. On native Windows, Claude Code runs commands unsandboxed." On
Linux it needs `bubblewrap` and `socat`, which a locked-down laptop may not have. `Write(...)`,
`Glob(...)` and `NotebookEdit(...)` path rules "never match"; use `Read` and `Edit`. Pasted image paths and
@-mentioned directory listings are checked from v2.1.290.

So: deny rules alone stop the obvious reads (`Read`, `cat`, `@raw/file.csv`). They do **not** stop
`grep -r` from the parent folder or a Python script. Only the sandbox does, and not on native Windows. The
safest advice stays: **keep the raw files and the mapping table outside the agent's folder altogether.**

**OpenAI Codex CLI** ([permissions](https://learn.chatgpt.com/docs/permissions); the old
`developers.openai.com/codex/permissions` URL redirects there; read 2026-10-30). Marked "Beta. Permission
profiles are under active development and may change." Supported "on macOS, Linux, WSL, and native
Windows". Example adapted from the docs:

```toml
default_permissions = "standin"

[permissions.standin]
extends = ":workspace"

[permissions.standin.filesystem.":workspace_roots"]
"raw" = "deny"
"private" = "deny"
"**/*.mapping.json" = "deny"
```

`deny` "denies access and wins over equally specific write or read entries." Caveats from the docs: if
`sandbox_mode` is set anywhere, Codex uses the old settings "instead of `default_permissions`"; on Linux,
WSL and Windows, deny globs are listed when each sandboxed process starts, so "Matching files created
afterward are not added to that process's deny list". I did not test Codex. Exact-path entries for
folders are my reading of the docs (they show `".devcontainer" = "read"` and `"**/*.env" = "deny"`);
**unverified** in practice.

## Findings that should change the design

1. **Look at data-masker first.** It already ships most of the MVP as one MIT HTML file. Decide (Ryan)
   whether Standin forks it, contributes to it, or stays separate. If separate, credit and borrow: formula
   guard, run ID in both file names, "don't open the match file in Excel", close-page warning.
2. **CSP cannot stop navigation.** Never render a user's value as HTML or as a link. Always use
   `textContent`. With `script-src 'unsafe-inline'`, injected HTML such as `<img onerror=...>` from a CSV
   cell would run, and could then call `window.open` with data in the URL, which the CSP does not block.
   Consider a hash in `script-src` instead of `'unsafe-inline'` (it blocks inline event handlers), at the
   cost of updating the hash when the script changes. Add a test with a CSV cell holding HTML.
3. **Use `default-src 'none'` and set `form-action 'none'` and `base-uri 'none'` separately.** Without
   `default-src`, prefetch is allowed. `form-action` does not fall back to `default-src`.
4. **Do not use `File.text()`.** It hides encoding errors. Read bytes, check BOMs, decode UTF-8 with
   `fatal: true`, and on failure ask the person to pick an encoding (offer Windows-1252). Fail closed until
   they do.
5. **Restore must be strict.** Case and joiner leniency only, word guards, one pass, report unknown
   stand-ins, no fuzzy matching. Before scrubbing, refuse input that already contains stand-in-shaped text.
6. **Keep every original spelling in the mapping.** Normalising before keying (DESIGN.md section 5)
   makes `Ann Lee` and `ann lee` one stand-in, which is good for matching, but restore then gives one
   spelling. Restore a scrubbed file by cell position to get exact values back; for AI prose, use the most
   common spelling and say so.
7. **Agent settings are not enough on their own.** Claude Code deny rules miss `grep -r` from a parent
   folder and any script. The sandbox closes that, except on native Windows. The interface should tell
   people to keep `raw/` and the mapping **outside** the agent's folder, with the settings snippet as a
   second layer.
8. **Add a small k-anonymity count.** Group rows by the kept columns, show the smallest group, the number
   of unique rows, and rows in groups under 5. It runs in milliseconds and turns "Not addressed; say so"
   (DESIGN.md 6.6) into a useful warning.
9. **`.xlsx` is possible without a library** (DecompressionStream deflate-raw: Chrome 103, Firefox 113,
   Safari 16.4), but parsing the zip and sheet XML is real work. data-masker bundles SheetJS (950 KB)
   instead. Keep xlsx out of the MVP.
10. **Safari needs a hand test on a Mac.** Everything above passed in Playwright's WebKit, which is not
    Safari. Test `file://` open, file input, drag and drop, and download in real Safari before claiming
    success check 8.

## Things that contradict DESIGN.md

- **Presidio**: "reversible `encrypt`/`decrypt` operators" is right, but they are AES-CBC with a random IV,
  so the same value gives a different result each time. Useless for matching rows. Numbered placeholders
  are a sample, not a feature. `presidio-structured` has no restore.
- **scrubadub**: DESIGN.md says MIT. The GitHub API says Apache-2.0; PyPI says MIT. The sibling project's
  note (Apache-2.0) matches GitHub.
- **noirdoc**: it is MIT and open source, and its repos moved to `noirdoc-ai/`. Its Claude Code plugin is
  "Python 3.12 or 3.13", not a hook needing spaCy by default (the full extra adds about 560 MB of models).
- **shutapp** and **Cloak**: not found. The claim that shutapp "sends text to a hosted API" is unverified.
- **"No single-file browser tool with a round-trip exists"**: no longer true. data-masker does it.
- **DESIGN.md 6.5**: "Enforce it with the agent's settings (deny reads on the raw folder), not with trust."
  Settings alone do not enforce it against shell scripts. Only the sandbox does, and not on native
  Windows.
- **DESIGN.md 6.6**: "Re-identification from combinations ... Not addressed." A cheap group-size check is
  possible in the MVP.

## Not checked

- Real Safari on macOS, and Edge. Firefox and Chromium were Playwright builds on Linux, not the branded
  browsers.
- Drag and drop.
- DNS prefetch under CSP.
- Codex permission profiles in practice.
- The full mloda-pii licence text, and whether PresidioJS and Rampart can legally be reused.
