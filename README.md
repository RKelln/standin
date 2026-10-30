# Standin

Let an AI work on your files without seeing your private data.

Standin swaps the sensitive values in a file for stand-ins (`Customer_1`, `[EMAIL_2]`), keeps a private
table that maps them back, and later puts the real values back into whatever the AI wrote. The AI sees the
shape of your data and the stand-ins. The real values never leave your machine.

It is made for small businesses and non-profits who want to use an AI coding agent (Claude Code and
similar) on their own records: invoices, bank exports, customer lists, email exports. It is the tool behind
the "share safely" step of [Get2Know](https://github.com/RKelln/get2know).

**Status:** design only. Read `docs/DESIGN.md`.

## How it works, in five steps

1. **Shape.** Standin reads a CSV or JSON file and writes a *shape file*: headers kept, every value replaced
   by a short description of it ("email-like, 22 chars", "date YYYY-MM-DD", "amount, 2 decimals"), plus a
   summary per column. No real value is in it.
2. **Propose.** The AI reads only the shape file and proposes a *config*: for each column, tokenize, keep,
   or drop, with a reason.
3. **Review.** The person reads the config (a plain list) and approves it. Anything not marked "keep" is
   tokenized by default.
4. **Scrub.** Standin applies the config, writes the scrubbed copy and a private mapping table, and shows
   three rows before and after.
5. **Restore.** The AI works on the scrubbed copy. Text it writes goes back through Standin, which swaps
   the stand-ins for the real values locally.

## What it is not

- Not a guarantee. It reduces what an AI sees; it does not make data safe to share with anyone.
- Not a detector of every private thing. It does not guess names inside free text. It says so, and asks a
  person.
- Not for files it cannot read (PDF, images, Word). It refuses them and says to redact by hand.

Code is MIT. Documentation is CC BY 4.0. (Proposed; confirm before the first release.)
