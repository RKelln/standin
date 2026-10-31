# Free text: approve words, not columns (research, 2026-10-30)

Track F. This checks the "approve words" proposal for `.md`, `.txt` and email, using numbers from a
prototype. The prototype is scratch code outside the repo (Python, about 200 lines). It reads the trial's
invented bike-shop files and some larger invented corpora that a script made.

**Words used here.** A *candidate* is a word Standin thinks might be private. To *keep* a word means to
leave it as it is. To *swap* a word means to put a stand-in such as `[NAME_3]` in its place.

## 1. Word lists and licences

All of these were checked at the source on 2026-10-30. Sizes are for the lists as I would ship them:
lower-case, one word per line, gzip level 9.

| List | Use | Licence (short) | What we must do | Size raw / gzip |
|---|---|---|---|---|
| **SCOWL, now called ESDB** (wordlist.aspell.net, git `en-wl/wordlist` v2). Size 60 ("default", the size the author advises for spell checking), Canadian spelling, variant level 1. **Lower-case entries only, no `'s` forms:** 79,626 words. | English words | MIT-like: "Permission to use, copy, modify, distribute, and sell ... is hereby granted without fee, provided that the above copyright notice appears in all copies and that both the above copyright notice and this notice appear in supporting documentation." Copyright 2000-2026 Kevin Atkinson. | Ship the notice in a `THIRD_PARTY.md` and in `standin version --licences`. **Stay at size 80 or below**: bigger lists bring in the UK Advanced Cryptics Dictionary, which needs its own text "prominently displayed". Do not use the Australian spelling (another notice). | 757 KB / 213 KB |
| Same, size 50 / 70 | | same | | 575 / 164 KB; 1,255 / 342 KB |
| **US SSA baby names** (`ssa.gov/oact/babynames/names.zip`, 1880-2025) | First names | data.gov lists it as CC0 (public domain). Names given fewer than 5 times a year are left out. | Nothing needed. A credit line is polite. | Not measured: ssa.gov returned 403 to scripts here. |
| **Ontario baby names**, female and male (data.ontario.ca, 1913-2024, counts under 5 left out, English and French) | First names | Open Government Licence - Ontario 1.0. Commercial use and redistribution allowed. | Must include: "Contains information licensed under the Open Government Licence – Ontario." and a link where possible. Must not suggest endorsement. | together with Quebec, below |
| **Quebec "Banque de prénoms"**, filles and garçons (donneesquebec.ca, 1980-2025, counts under 5 shown as "<5", names given once removed) | First names, many French and immigrant names | CC BY 4.0 (both data sets, checked with the portal's API). | Credit the source, link the licence, say we changed it (merged, lower-cased). | |
| Ontario + Quebec merged, single words, total count 5 or more | | | | **22,409 names, 159 KB / 56 KB** (count 20+: 9,620 names, 24 KB gzip) |
| **French Hunspell dictionary** (Grammalecte, v7.7, in LibreOffice's `fr_FR`) | French words, later | MPL 2.0 (from its README). File-level copyleft: our extracted list stays MPL and its source must stay available. Allowed inside an MIT binary. | Ship the MPL notice. Keep the list in its own file. | Stems only: 72,130 words, 787 KB / 241 KB. Inflected forms ("pourriez") need affix expansion in a dev script. |

**Why Ontario and Quebec, not SSA.** They come from births in Canada's two biggest provinces, so they hold
many names from immigrant families, and they are in French and English. In the merged list,
Aiko, Kofi, Fatima, Jamal, Nguyen, Wei, Tenzin, Priya, Chloé and Renée are all there. Xiaoming and
Oluwaseun are not. No list of names is complete. SSA can be added later (CC0, no extra notice).

**Do not use SCOWL's capitalised entries as "known words".** They include many surnames and given names:
Webb, Tran, Nguyen, Patel, Singh, Chen, Kim, Leila, Lena, Andre. Treating them as dictionary words would
hide those names. In the prototype, that cut the candidate count roughly in half (29 to 16 on one folder),
but almost all of the drop was names. Use the lower-case entries only. Even these hold surnames that are
also common words: smith, brown, lee, carter, martin, chow, berg.

**Embedded size and speed (measured).** A static Go build with SCOWL 60 and the names list, both gzip
files read with `go:embed` and `compress/gzip`, was 270 KB bigger (1.75 MB to 2.02 MB). Loading both
into maps took about 15 ms. French would add about 240 KB.

**Email parsing note.** `net/mail` and `mime/multipart` link `net` (through `net/textproto`), so the
existing no-network test (`cmd/standin/main_test.go`) would fail. `mime` (the `WordDecoder` for
`=?utf-8?...?=` headers), `mime/quotedprintable` and `encoding/base64` link no `net` package and are fine.
The header and MIME code for `.eml` and `.mbox` must be written by hand.

## 2. Prototype numbers

### Rules in the prototype

Step 2 of the proposal, plus four changes the numbers called for (marked **new**):

- In the mapping (whole known values, 3 characters or more, as the scan does now).
- Patterns: email, phone, postal code, 8 or more digits.
- Capitalised, and not in the SCOWL lower-case list (after taking off `'s`; a hyphenated word counts as
  a dictionary word only if every part is one).
- Name spots: after Hi, Hello, Hey, Dear, Thanks, Cheers, from, with, cc, Bonjour, Merci; after "name is",
  "Name:", "this is"; and a short line of 1 to 3 capitalised words just after a closing line ("Thanks,",
  "Regards," ...) or after `--`.
- Capitalised and in the first-name list, unless it starts a sentence or line (so "May I ..." and "Will
  you ..." are not flagged, but "Grace" in the middle of a sentence is).
- **new:** a capitalised word right after a caught name ("Grace **Lee**", "Ana **Brown**"). Surnames are
  often dictionary words.
- **new:** day and month names (English and French, with short forms) go in one "calendar" group, which is
  one question at review.
- **new (structure):** `Label: value` lines (`From:`, `To:`, `Requester:`, `Client:`, `Attendees:` ...) are
  pulled out as fields first. Their values are swapped whole and join the known list. Each capitalised
  part becomes a candidate too, so "Daisy" alone in the body is still caught.
- **new (grouping):** group **A** = unusual words (not in a name spot and not in the name list). Group
  **B** = name-like words. Most words to keep are in A.

### The trial's shop fixtures (`shop/`, `shop-messy/`)

Known values came from the invoice CSV's customer column. "Review" means the words the person would be
asked about. It leaves out calendar words, pattern hits and whole known values.

| File | Review words | Real names | Other words | Calendar group |
|---|---|---|---|---|
| customer-emails.md (40 emails) | 9 | 3: Andre, Jordan, Leila | 6: Dundas (street), AI, Northgate, Cycle, Supply (a supplier), Friday (after "It's") | 5 |
| price-sheet.md | 2 | 0 | HST, May | 8 |
| profile draft (clean) | 7 | 2: Sam, Lena | Toronto, Harbourline, Courier, Co, HST | 4 |
| profile draft (messy) | 10 | 2: Sam, Lena | Toronto, Northgate, HST, NEVER, TODO, Sun, Saterday (typo), Tues-Sat | 6 |
| shop card | 6 | 2: Sam, Lena | Toronto, Harbourline, Courier, Co | 4 |
| receipts (12 files together) | 9 | 0 | HST, NORTHGATE, CYCLE, SUPPLY, ONTARIO, SIGNS, PRINT, Degreaser, Nm | 0 |
| **whole folder, clean / messy** (decisions shared) | **22 / 27** | 5 | 17 / 22 | 10 / 15 |

**Missed:** none of the 5 people. Missed were business names made only of dictionary words: the shop's
own name, a sign shop in a forwarded `To:` line (a field, so the structure step swaps it), and the
receipt headers BRIGHT OFFICE SUPPLY, CITY HARDWARE, SHOP TOOLS DIRECT. Brand names that are words (Trek,
Rad) also stay. The known-values pass did not catch "Leila" alone, because the mapping holds only
"Leila Haddad". The dictionary rule caught her.

These fixtures are too small to size the list. Their 40 emails have no headers or sign-offs and only 3
names.

### Larger invented corpora

A script made these from invented name parts (75 first names, 20 that are also words such as Grace, Rich
and Summer; 48 surnames, 18 that are also words such as Brown and Stone), 18 brands, 15 streets and
areas, typos and some names typed in lower case. Recall here means the share of name words in the text
that end up swapped.

| Corpus | Words | Review list | Group A (to read with care) | Group B (name-like) | Names in list | Recall |
|---|---|---|---|---|---|---|
| inbox, 50 emails | 1.5k | 81 (69 with fields) | 21 (13) | 60 (56) | 64 | 92% (100%) |
| **inbox, 200 emails** | 5.5k | **154 (112 with fields)** | **33 (23)** | 121 (89) | 125 (84 outside fields) | 98% (100%) |
| inbox, 1000 emails | 27k | 190 (100 with fields) | 32 (22) | 158 (78) | 160 | 99.9% (100%) |
| meeting notes, 30 | 1.7k | 124 (102) | 46 (12) | 78 (90) | 108 | 99.5% (100%) |
| support tickets, 100 | 2.0k | 113 (97) | 39 (10) | 74 (87) | 102 | 93% (100%) |
| case notes, 50 | 1.0k | 94 (49) | 23 (14) | 71 (35) | 77 | 100% |
| mixed English/French, 50 | 1.1k | 73 (66 with French list) | 8 (1) | 65 | 64 | 100% |

What the numbers say:

1. **The list is mostly names.** In the 200-email inbox, 125 of 154 words are names. The words a person
   would keep (brands, streets, acronyms) are about 30 and stop growing: 29 at 200 emails, 30 at 1000.
   The names grow with the number of different people writing. My generator has only about 150 name
   parts, so it under-counts; a real 200-email inbox may have 200-300 name words.
2. **So the person should not read 150 words one by one.** Group A had 23-33 words, nearly all of them
   keepers once fields are pulled out. Group B was 90-95% names. Ask about A word by word; show B as one
   list that is swapped unless the person picks a number to keep.
3. **Pulling out fields matters most** for notes, tickets and case notes. Group A fell from 46 to 12
   (meeting notes) and from 39 to 10 (tickets), because names on `Attendees:` and `Requester:` lines no
   longer reach the word list. It also fixed most misses: a name on a field line starts the value, so the
   "not at sentence start" rule skipped it.
4. **SCOWL size barely matters.** Sizes 50, 60 and 70 gave 154 words each on the 200-email inbox and 29,
   27 and 26 on the messy fixture folder. Size 70 has 1,182 first names that are also words, against 839
   for size 60. Use 60.
5. **French without a French list costs little.** Lower-case French words are never candidates, so only
   capitalised words at the start of a sentence show up (Bonjour, Merci, Je, Cordialement, Le ...). That
   was 8 to 9 words in 50 emails, asked once and saved. With the French stems list it was 2.
6. **Misses in the prototype** were all names that are also words, seen only at the start of a sentence
   or line, or in lower case: Rich, Daisy, Rose, Summer, Mark, Strong, and "White" when the first name
   before it was missed. Recall is high here partly because each invented name comes back many times,
   and one caught use swaps them all. A real name seen once, at the start of a sentence, and also a word
   ("Rose called about ..."), is missed.

## 3. Recommended design

### 3.1 Which files, and structure first

- `.md` and `.txt` become free-text files. A `.txt` that looks like a table (the same count of commas
  or tabs on every line) is still refused, with "rename it to .csv or .tsv", as now. Later: `.eml`, then
  `.mbox` (hand-written header and MIME parser, see section 1). HTML parts: refuse at first.
- **Markdown tables** (a `|` header row, then a `|---|` line) become a table named by its position and
  header (`table 2: Service, Price`). The CSV column rules apply as they are. A ragged row fails closed,
  like CSV.
- **Field lines.** A line that is `Label: value`, with a short label of 1 to 3 words, becomes a column
  named after the label (`From`, `Date`, `Requester`). Email headers are the same thing. Labels are
  checked with `LooksLikeDataName`, so chat logs (`Leila: see you then`) get their labels flagged. A
  column absent from the config is tokenized whole (default-deny), and its capitalised parts become
  group B candidates.
- **Everything else** gets the word pass: headings, list items, paragraphs, sign-offs. Markdown marks
  (`#`, `-`, `**`, `[ ]`, links) are left as they are.
- **Scripts with no capital letters** (Chinese, Arabic, Hebrew, Devanagari ...): every such word is a
  candidate. If more than about 1 word in 5 is in such a script, refuse the file: "This file is mostly in
  a language Standin cannot check yet." Fail closed.

### 3.2 The shape file: counts only

`shape/<name>.shape.md` for a text file lists:

- the tables and field labels found, with the usual column shapes;
- the free text in **count bands** (decision 9), never a word: "free text: 21-100 lines; unusual words
  21-100; name-like words over 100; calendar words 5-20; emails 5-20; phones under 5".

The agent writes rules: `keep` or `tokenize` per field and table column, and one line for the text. It
cannot name a word to keep, because it never sees one.

Config sketch (only the new parts):

```json
{"match": "raw/inbox/*.md",
 "fields":  {"Date": {"action": "keep"}, "Subject": {"action": "scan"}, "From": {"action": "tokenize", "category": "PERSON"}},
 "tables":  {"Service": {"action": "keep"}, "Price": {"action": "keep"}},
 "text":    {"action": "words", "keep_groups": ["calendar"], "reason": "emails; swap names"}}
```

`"text": {"action": "words"}` is a kind of `scan` (the text keeps words that are not candidates), so it
needs approval (decision 24). `"drop"` (blank the text) stays possible. `keep_groups` allows only fixed,
built-in groups (now just `calendar`), never words.

### 3.3 Approving words, on a terminal only

Word review goes inside `standin approve`. The agent cannot run it (decisions 18 and 25), and it already
needs a terminal. `standin review`, which the agent may run, shows counts only for text files.

```
Free text in raw/inbox/*.md: 23 unusual words, 89 name-like words, 14 calendar words.
Words you keep stay in the text the AI reads. Words you do not keep become stand-ins like [NAME_4].

Unusual words (most used first). Type the numbers to keep, like 1 3 5-7, or none:
  1  Shimano    x20   "...new [Shimano] pads before..."
  2  Dundas     x13   "...the shop on [Dundas] quoted..."
  3  Okafor     x4    "...Thanks, Kofi [Okafor]"
  ...
Keep:
```

- Group A first, 20 to a page, most used first, each with one short piece of context. In the context,
  other candidates are shown as `▒▒▒`, so one word's context does not show another name.
- Group B next, as one list: "These look like names. They will all become stand-ins. Type numbers to
  keep any, or press Enter." The default is swap.
- Calendar: one question if the config asks for `keep_groups: calendar`. A calendar word in a name spot
  ("Hi April") is asked about on its own.
- Then the usual final `yes`.
- From the numbers above, a 200-email inbox means about 25 words to read with care, plus a quick look
  at about 90 names.

### 3.4 Where decisions are kept

- `words.json` in the private folder (0600, next to `mapping.json`), plain text so the person can read
  and edit it:
  `{"version":1,"keep":["dundas","shimano"],"swap":["leila","okafor"],"groups":["calendar"]}`.
  Words are stored in normal form (lower case, trimmed, `'s` taken off).
- **Not hashed.** A kept word is in the scrubbed output anyway. The swap list is mostly names, as secret
  as the mapping, and it sits next to it. A hash would stop the person reading it, and AGENTS asks for
  files a person can read.
- `approval.json` gets `words_sha256` (the hash of the sorted keep list and groups). If someone edits
  `words.json` by hand, the hash no longer matches and scrub refuses until `approve` runs again
  (decision 33).
- **Across files:** decisions are for the whole project, like the mapping. A later file asks only about
  words that are in neither list.

### 3.5 Scrub, exit codes, default-deny

- Scrub runs in passes, as now: tables and fields first (so their values join the known list), then the
  known-values and pattern pass, then words.
- Every candidate not in `keep` is swapped, as `[NAME_n]` (a **new category**; known values keep their
  own category, such as `[PERSON_2]`). The same word gets the same stand-in in every file.
- Where it is swapped: every capitalised use, plus lower-case uses if the word is not in the dictionary
  (so "thx oksana" is caught but "rich" and "grace period" stay as they are).
- **New words** (a candidate in neither list, from a new file or a changed one) are swapped, never kept.
  Scrub then exits 1: "4 new words became stand-ins. To keep any, run standin approve." Swapping is the
  safe way, so this does not need to refuse. A missing or changed approval still refuses (exit 2).
- Restore needs no change. `check` already works on any text.

### 3.6 What still gets through (say this in the interface)

- Names typed in lower case that are also words (rich, grace, will, mark).
- A name that is also a word, used only at the start of a sentence or line ("Rose called", "- Grace to
  book the room").
- A surname that is a word, with no first name or title next to it. Add Mr, Mrs, Ms, Mx, Dr, M., Mme as
  name spots to help.
- Business names made of words (City Hardware). Usually not private, but they can be.
- Handles and names inside links (`@grace_lee`, `site.com/u/jordan`). Add an `@handle` pattern; links
  could be swapped whole.
- Identity by story or rare detail ("the only Tigrinya speaker at the shelter"). The finder flagged
  "Tigrinya", but a person may well keep it. No word filter fixes this. For case notes, `drop` is still
  the safer advice.
- Non-English text: French is fine at the cost of a few extra words. Caseless scripts are refused (3.1).

### 3.7 Other kinds of input (where the design differs)

| Input | What changes |
|---|---|
| Email export (`.md`, later `.eml`/`.mbox`) | Headers are fields. Quoted replies (`>` lines) repeat names, which is fine (same stand-in). Signatures hold phones, titles and addresses; the patterns catch some. |
| Meeting notes | `Attendees:` and `Regrets:` are fields: the biggest win. Action items start with a name ("Grace to send ..."), so a word-name in that spot is missed. Possible later rule: a capitalised word at the start of a list item, followed by "to" or ":". |
| Support-ticket export | Usually CSV or JSON. The ticket body is a `scan` column, and it should get the same word approval (below). In `.md` form, `Requester:` is a field. |
| Non-profit case notes | Highest risk: relatives named in passing, health details, stories. Fields help (`Client:`, `Worker:`). Advise `drop` for the text, unless the person reviews the words *and* reads one output by eye. |
| Mixed English/French | Works without a French list (about 9 extra words in 50 emails). Add the French list when a real user needs it, along with French name spots (Bonjour, Merci, Cordialement are already in the prototype). |

### 3.8 Conflicts with what is decided now

- PLAN, input formats: `.txt` is refused. Change: `.txt` is free text, unless it looks like a table.
- Categories (`PERSON ... VALUE`): add `NAME`. Contract change.
- Decision 18 (the agent may run `review`): fine, as long as `review` shows counts only for text and the
  words appear only inside `approve`.
- Decisions 10 and 24 (`scan` leaves unknown names in): the word pass is a stronger scan. Later, let a CSV
  `scan` column use it too (`"action": "words"`). Not needed for the first slice.
- Decision 22 (no network packages): rules out `net/mail` and `mime/multipart` (section 1).

## 4. Smallest useful slice (fits by 2026-11-02 if one builder takes it)

1. `.md` and `.txt` read as free text. Markdown tables become columns. `Label: value` lines become
   fields. Everything else gets the word pass.
2. Embedded lists: SCOWL 60 lower-case and the Ontario + Quebec names (about 270 KB), plus the notices.
3. Candidate rules as in section 2, with groups A, B and calendar.
4. Shape file with count bands only. Config `text: words | drop`, plus `fields` and `tables`.
5. `approve` word pages (numbers to keep), `words.json`, `words_sha256`.
6. Scrub swaps to `[NAME_n]`. New words are swapped and give exit 1.
7. Tests: the shop fixtures copied in as synthetic test data, plus one generated inbox. Check that no
   candidate word appears in the shape file, and that a name typed in the 5 shown positions is swapped.

Later: `.eml`/`.mbox`, word approval for CSV `scan` columns, the French list, SSA names, the list-item
rule, handles.

**Not tested:** any real inbox or notes export (none may be used here). The large corpora came from my
own templates, so the share of names in them follows from how I wrote the script. Treat "about 30
keepers, growing slowly" as a first guess, not a measured fact. The prototype is in Python; a Go version
may differ in small ways (word edges, Unicode case).
