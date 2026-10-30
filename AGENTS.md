# Standin: instructions for the working agent

You are building Standin. Read `README.md`, then `docs/DESIGN.md`.

`docs/DESIGN.md` is a first design written by someone who is not a privacy engineer, after a short look at
prior art. It leaves questions open on purpose (section "Open questions"). Check its claims at primary
sources before relying on them. Where you decide, record the decision and why in a "Decisions" section of
that file. Nothing in it is sacred.

## Your remit

- Ship the MVP in `docs/DESIGN.md` first, as small as it can be and still pass its success checks.
- Beyond the MVP, you own the design. Say when a requirement conflicts with something you learn.
- Ask Ryan only when a choice is really his (name, licence, scope). Otherwise decide and note it.

## Working rules

- Plan in writing before building. Short plan, then build.
- **The MVP is a CLI: one static Go binary, standard library only, no network access.** It is called
  mainly by agent-harness hooks; setup is hands-on. It must run on a locked-down laptop where `python3`,
  `node` and `git` are installer stubs and nothing can be installed: the binary ships in the install
  files (macOS arm64/amd64, Windows, Linux). (Ryan, 2026-10-30; replaces the earlier one-HTML-file rule.)
- The tool must never make a network request. Add a test or a check for that.
- **Fail closed.** If it cannot parse a file, or is unsure, it writes no output and says why.
- **Default-deny.** Every value is tokenized unless the approved config says keep.
- The mapping table is the secret. It is saved only by an explicit user action, and nothing the AI reads
  should ever contain it. Say this in the interface.
- Plain-text outputs (CSV, JSON, Markdown). A person can read and edit every file in a text editor.
- Interface and messages in plain language, near grade 6. Define any privacy word the first time.
- Python with `uv` for any dev scripts and tests. Not bare pip.
- Test with synthetic data only. Never put real personal or business data in this repo, in tests or in
  examples. Make the fixtures from invented names.
- Keep this repo general. Do not name any one deployment or organisation in code or docs.
- Report faithfully: say what is untested and what the tool misses.
