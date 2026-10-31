# What Get2Know needs from Standin (notes, 2026-10-30)

From a trial of Standin `main` (just after v0.1.0) on a copy of Get2Know's made-up bike-shop folder. The Get2Know side is in `get2know/docs/research/standin.md`. Get2Know's interviewer would read the owner's records through `scrubbed/`. The owner's chat with it is not filtered.

Ordered by how much each blocks the integration.

1. **Markdown and plain text (`.md`, `.txt`, and later `.eml` and `.mbox`).** `shape` refuses them ("I cannot read this kind of file"). The bike shop's `customer-emails.md`, `price-sheet.md` and `receipts/*.txt` are typical: an inbox export or a notes file is often the most private thing an owner has.
   - The known values from the CSVs already catch many names in emails, but a plain scan of the whole file would leave every other name in.

   **Proposed design: approve words, not columns.** In a table the person approves columns; in free text they approve **words**. Everything that might be private becomes a stand-in unless the person keeps it. This keeps default-deny and needs no language model.

   1. **Pull out any structure first, and reuse the CSV rules on it.**
      - A Markdown table (a price sheet often is one) gets its own columns.
      - Email headers (`From:`, `To:`, `Date:`, `Subject:`) become columns.
      - Only what is left (prose, email bodies, notes) gets the free-text treatment below.
   2. **Find candidate words.** In the free text, a word is a candidate if any of these hold:
      - it is already in the mapping (from the CSVs);
      - it matches a pattern (email, phone, postal code, long number);
      - it is capitalised and not in a built-in English word list;
      - it sits where names usually go: after "Hi", "Dear", "Thanks," or "from", or alone on a sign-off line. This catches names that are also ordinary words (Grace, Rose, Bill);
      - it is capitalised and in a list of first names, even if it is also an ordinary word.
   3. **The agent sees counts, not words.** The shape file says, for example, "38 unusual words, 9 in name spots, 4 emails", with no values. The agent proposes the rules (keep amounts and dates, for example). It cannot propose keeping a particular word, because it never sees one.
   4. **The person approves the word list in their own terminal.** `review` and `approve` show the candidate words on a terminal only, most frequent first, each with a little context. The person keeps the ones that are not private (Shimano, Tuesday, the shop's own street). Everything else becomes a stand-in.
      - A kept word counts as a keep, and changing the list needs approval again.
      - The list is saved, so later files ask only about new words.
   5. **Scrub** swaps every candidate the person did not keep, wherever it appears. Markdown formatting is left alone.

   Mistakes go the safe way: shop words and typos become stand-ins and get kept at review. **What still gets through:**
   - names typed in lower case;
   - nicknames that are ordinary words, used with no greeting nearby;
   - identity by story ("the only Ethiopian bike shop in Thunder Bay"). No word filter catches that.

   **Built-in lists.** These stay within the standard library by embedding the lists in the binary (a few hundred KB, `go:embed`):
   - **SCOWL** for English words. Its licence is permissive; check the exact terms.
   - **US Social Security baby-name data** for first names. It is a US government work, but thin on non-English names, which matters in Toronto.
   - French would need its own word list later.

   **Open question:** how long the word list gets on a real inbox export. If it is 300 words, approval is a chore. Sorting by count, showing context and keeping the saved list from one file to the next all help. Try it first on Get2Know's `customer-emails.md` fixture for a first number.

2. **Remove the prompt hook.** It is not part of the design: the first draft listed "block raw values in prompts" as part of a later, optional hook (DESIGN 6.5). It was built anyway in `fac0d4d`, and `init` installs it as `UserPromptSubmit`. DESIGN decision 21 records it as tested.
   - In an interview the owner talks about their customers by name. It blocked "that is Marcus Webb, he always pays late" and a phone number.
   - It catches only full known values ("Marcus" alone went through), so it gives less protection than it seems to.
   - Ryan: Standin is for the data fed to the agent, not for interactive chat.
   - Remove it from `init`, the hook code, the smoke test, DESIGN 21 and the docs.

3. **A scrub before approval fills the mapping, which later wipes out scan columns.** This is a bug.
   - Repro:
     1. Write a config with `Description: scan` for a bank export.
     2. Run `standin scrub` before `approve`. Everything is tokenized as `[VALUE_n]`, as designed.
     3. Run `approve`, then `scrub` again.
   - Result: whole descriptions, such as "PRE-AUTH DEBIT LANDLORD RENT", come out as `[VALUE_71]`. The earlier run put every whole value into the known list, and the scan swaps known values.
   - With a fresh `STANDIN_HOME` (approve first), the same config gives `E-TRANSFER [PERSON_2]` and keeps the rest readable.
   - Fix ideas:
     - Do not add fallback `VALUE` stand-ins to the known list used by `scan`.
     - Or do not write the mapping on an unapproved scrub.
     - Or refuse to scrub until the config is approved.

4. **A moved folder blocks every tool.** `.claude/settings.json` holds the absolute path of the binary and of the project folder (`--project /full/path`). After a copy or move, every `PreToolUse` call exits 2 with "the project folder does not exist", because the hooks fail closed. In Claude Code the agent cannot read or write anything, not even outside `raw/`.
   - Get2Know owners do move the folder (from a library machine to home, from Downloads to Documents).
   - Ideas:
     - Find the project from the hook input's `cwd` (or walk up to the nearest `standin.config.json`) when `--project` is gone, and still fail closed if neither works.
     - Name the fix in the message ("Run `standin init` again in this folder").
     - The mapping folder is keyed by the old path too. Say how to carry it over.

5. **A lookup for the person: `standin show PERSON_9`.** It prints the real value only on a terminal and refuses otherwise, like `restore -`.
   - During an interview the agent says "INV-1012 was paid short". If it ever has to name a stand-in, the owner needs a quick way to see who that is.
   - Writing a file and running `restore` is too slow for this.

6. **Hooks in `.claude/settings.local.json` (an option).** Get2Know ships `.claude/settings.json` and `AGENTS.md` and updates them by checksum. Once `init` edits them, Get2Know's updater treats them as the owner's changes and stops updating them.
   - Writing hooks and deny rules to `settings.local.json` would avoid that clash. It is per machine and not shared, which also suits absolute paths.
   - Check that Claude Code honours `onFailure: "block"` hooks from the local file.
   - Get2Know can also teach its updater to keep the Standin block in `AGENTS.md`. Either works.

7. **Confusing restore message.** `standin restore answer.md` printed "Wrote a file whose name looks like private data (rename it to something plain, like invoices.csv)".
   - The message reads as if the file was renamed. Probably the scratch path, which has a long hex ID, tripped `dataLikeName`. Check whether a normal home path such as `/Users/sam/Documents/shop` trips it too.
   - Either way, "Wrote restored/<hidden name>" reads better.

8. **Smaller.**
   - With `yes` piped into `approve` in a pseudo-terminal, the first answer did not echo. This is cosmetic, and an agent should not do it anyway.
   - Get2Know's interviewer needs no other commands than those the agent may already run (`shape`, `scrub`, `check`, `review`, `report`).

Not tried here: the Mac install, Codex (no hook; the permission-profile note is untested), real bank or accounting exports, and a model writing the config for these files.
