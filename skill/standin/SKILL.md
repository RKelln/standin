---
name: standin
description: Work on a person's private files (invoices, bank exports, customer lists) without seeing the real names, emails or numbers. Use when a project has standin.config.json, a raw/ or scrubbed/ folder, or the person asks to keep data private from the AI.
---

# Standin: working with private data

Standin lets you help with a person's records without seeing the private parts.
It swaps each private value for a **stand-in**. A stand-in is a made-up label, like
`[PERSON_1]` or `[EMAIL_2]`, that takes the place of one real value. The same real value
always gets the same stand-in, so you can still match a payment to an invoice.

## Words

- **raw/**: the person's real files. You never open these.
- **shape/**: a short description of each raw file (column names, kinds of values, rough
  counts). No real values. You use it only during setup.
- **scrubbed/**: copies of the raw files with stand-ins in place of real values. You work here.
- **restored/**: your work with the real values put back, for the person. You never open these.
- **mapping**: the list that turns stand-ins back into real values. **The mapping is the
  secret.** It lives outside the project folder. Never look for it, read it, or ask for it.
- **config** (`standin.config.json`): the rules for each column. It has no secrets. You may
  read it and write it.

## The rules

1. Read only `scrubbed/`, `shape/` and `standin.config.json`.
2. Never read, search, list or change `raw/` or `restored/`. Do not try another way in (a
   script, a shell command, a copy). If a hook blocks you, do not work around it.
3. Keep every stand-in exactly as it is: same brackets, same capitals, same number.
   `[PERSON_1]` is right. `Person 1`, `[person_1]` and `PERSON_1` will not be put back.
4. Never make up a real-looking value to replace a stand-in. Leave the stand-in.
5. Never run `standin approve`, `standin restore`, `standin show` or `standin init`, not even
   through another program (`sh -c`, `script`, `expect`). Only the person runs them: approve says
   yes to showing real data, restore and show show real values, and init sets up the guards.
6. Never pass `--private` or `--project` to standin. Standin picks those places itself, so the
   secret stays where you cannot reach it.
7. In the shell, name files inside `scrubbed/` or `shape/`. Do not use wildcards (`* ? [ {`) or
   whole-folder tools (`grep -r`, `rg`, `find`, `tree`) on the project folder. Write
   `grep -rn word scrubbed/`, not `grep -rn word .`.
8. If the person pastes a real name or number to you, do not repeat it. Ask them to use the
   stand-in instead. (By default a prompt guard stops a message that holds a real value Standin
   knows. The person can turn it off with `standin init --no-prompt-guard` if they talk about
   their own records on purpose. Either way, do not repeat real values.)
9. If the person asks who a stand-in is, tell them to run `standin show PERSON_9` (with that
   stand-in) in their own terminal.

## Setup: once per kind of file

The person does the steps marked **(person)**. You can run the others if you have a shell.

1. The person puts the real file in `raw/`, for example `raw/invoices.csv`.
2. Run `standin shape raw/invoices.csv` (with no `--private` or `--project`). It writes `shape/invoices.csv.shape.md`.
3. Read the shape file only. Propose the config: for each column pick one action and give a
   short reason.
   - `tokenize` (swap each value for a stand-in). Needs a `category`: `PERSON`, `ORG`,
     `EMAIL`, `PHONE`, `ACCOUNT`, `ID`, `ADDRESS`, `POSTAL`, `KEY` or `VALUE`.
   - `keep` (leave the real value). Only for values you need that are not private, like
     amounts, dates or a status. Every `keep` is a choice to show real data, so keep few.
   - `drop` (empty the column). For things not needed.
   - `scan` (free text, like notes or bank descriptions). Known names and things like emails
     and phone numbers get swapped; other words stay.
   - When unsure, choose `tokenize`. A wrong `keep` leaks data. A wrong `tokenize` only makes
     the work a bit harder.
   - If the shape file says a header looks like data, or says to rename the file, tell the
     person.
4. **(person)** Runs `standin review` to read the rules as a plain table.
5. **(person)** Runs `standin approve` in their own terminal. It asks them yes or no for each
   `keep` and each `scan`. You cannot do this step, and you should not try. If the config changes later, they
   must approve again.
6. Run `standin scrub` (or ask the person to). It writes the files in `scrubbed/`. If the config
   has a `keep` or `scan` that is not approved yet, scrub stops and writes nothing: ask the person
   to run `standin approve`.
7. If a step says no and stops (exit code 2), read the message, fix the cause, and try again.
   Do not try to get around it.

## Daily use

1. Work on files in `scrubbed/`. If you try to read a file in `raw/`, the hook may hand you
   the scrubbed copy instead and tell you so. That is expected.
2. Write your results (a report, a list, a draft email) with the stand-ins in it, in a new file
   outside `raw/` and `restored/`.
3. Tell the person which file to restore, for example: "Run `standin restore report.md` to see
   the real names." They read the result in `restored/`.
4. If a raw file changed, ask the person to run `standin scrub` again before you use it.

## What standin misses

Standin lowers what you see. It does not make data safe to share. Know its gaps:

- Free text: a name that is not already in the mapping stays in the text. So does an address
  written in words.
- Kept columns show real values, by design.
- Patterns stay visible: how often a stand-in appears, which ones go together, amounts and
  dates that were kept. Someone could sometimes guess who a stand-in is from this.
- Files it cannot read (PDF, Excel, Word, images) are refused, not scrubbed.
- The hooks are a safety net, not a wall. The shell check only guesses from the command text,
  so a script or a clever command can still read `raw/`. That is why rule 2 matters: do not
  look for a way around it. Suggest the person turns on Claude Code's sandbox, which blocks
  every program from `raw/` and `restored/` (macOS, Linux and WSL2; not plain Windows).
- If a tool result may hold a real value, the hook warns you without saying which one. Stop and
  ask the person to check.
- Hooks do not see `@file` mentions, CLAUDE.md or memory files, pasted text, or text already in
  the chat. Once a real value is in the chat, it stays in that chat.
- Restore only puts back exact stand-ins. A changed stand-in is reported, not fixed.

## When something goes wrong: report it, with no data

If a real value shows up where it should not, a file will not scrub, or a hook blocks good
work, help get it fixed once so it does not happen again.

1. Stop working with the affected file. Do not copy, repeat or save the real value.
2. Ask the person to run `standin report`. It prints the standin version, the system, the
   kinds and shapes of files, and the last errors. It prints no real values.
3. Write a short description to go with the report:
   - what you were doing (for example "reading a bank export after scrub"),
   - what went wrong (for example "a customer name in the Description column was not swapped"),
   - which column, file kind or command was involved,
   - what fix you suggest, if any (for example "add the Description column as scan").
4. **Put no data in the description.** No names, emails, numbers, addresses or rows, not even
   one. Use stand-ins or words like "a customer name" instead.
5. Give the person the report and your description, so they can send them to the standin
   project as an issue.
