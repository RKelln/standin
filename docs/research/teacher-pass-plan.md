# The teacher pass: how it works

Companion to `model-based-detection-eval.md` and `sidecar-decision-note.md`. Written 2026-10-31.

The teacher pass is the cheap half of the model question: **use a trained model to make labelling
and rule-finding cheaper, and ship no model.** It produces three things — a failure taxonomy, a
regression corpus Standin has never had, and the leak-rate number that decides whether a shipped
model is worth a second artifact.

The model is an **annotation assistant**, never a source of truth. Its job is to put a small number
of disagreements in front of a person; the person decides. Where the model is weak — names that are
also words, the class we care most about — a human must look, and the design keeps that bucket small
on purpose.

---

## Step 1 — Corpora, three kinds

**(A) Synthetic, known truth.** Standin's existing generator, extended to emit the hard shapes on
purpose: names that are also words (`Rose`, `Rich`, `Daisy`), lower-case names, handles, links,
sign-offs, names in `scan` columns. Because the generator plants every name, truth is exact and free
— no model needed to label it, and this is where a rules change is tested.

**(B) Real and public.** The gap between "my generator is messy" and "an actual inbox is messy" is
the whole question, and it cannot be closed with real private data — the repo rule and the tool's own
ethics both forbid it. Two public sources carry it, and they are used differently:

- **TAB (Text Anonymisation Benchmark)** — 1,268 real ECHR judgments, **MIT**, ungated, and *labelled*:
  person mentions carry `DIRECT` / `QUASI` / `NO_MASK` mask decisions and co-reference chains, so
  "should this have been masked" and "is this the same person again" are already answered. This is
  the measuring instrument.
- **Enron** — real workplace email, ~500k messages, **no licence grant**: the CMU page says only that
  it is distributed "as a resource for researchers" and asks that the employees' privacy be
  respected. Use it **internally, never published, never committed**, purely to check that TAB's
  legal register is not standing in for inbox mess. It is the closest thing to the tool's actual
  domain that exists in public.

Supporting sets for the one class that matters, all reported by the survey and re-runnable in
minutes: **LitBank** (public-domain fiction, CC BY 4.0 annotations — where names that are ordinary
words are densest), **UNER-English** (CC BY-SA, real email/reviews/newsgroups, includes lower-case
person annotations like `hi mark -`), **WikiGold** (small, CC BY 4.0, a precision sanity check).

### The licence board (verified 2026-10-31, corrected the same day)

The first version of this table asked one question — "can we use it?" — and answered it with
"no" wherever a non-commercial clause appeared. That was wrong, and the correction matters more than
the table: **using a corpus and redistributing it are different acts with different tests.**

- **Use** (load it on this machine, measure, publish the resulting number) is what a licence governs
  for *us*. Facts are not copyrightable; a recall figure computed over an NC corpus is ours to state.
- **Redistribution** (commit a sample into the repo, ship a fixture, put text in a test file) is what
  an MIT repo needs permission for, and the permission is judged by who receives it — **including
  the small business that clones Standin and is, by design, commercial.**

That last clause is the whole trap. Standin not being commercial does not help, because the repo is
handed to people who are. A sample committed under an NC licence turns every commercial user of
Standin into an infringer through no act of their own. So the commit column stays narrow even though
the measure column widens a lot.

| source | measure here | commit a sample to an MIT repo | which clause decides it |
|---|---|---|---|
| TAB (ECHR) | yes | **yes** — MIT | — |
| LitBank | yes | **yes** — CC BY 4.0, texts public domain | — |
| CUAD | yes | **yes** — CC BY 4.0 (Zenodo) | — |
| WikiGold | yes | **yes** — CC BY 4.0 | — |
| Enron | yes | **no** | no grant at all, plus CMU's privacy ask |
| ai4privacy | **yes**, as a non-commercial researcher | no | NC ("commercial entities should contact…") |
| Canadian Hansard, Quebec AN | yes | **no** | two clauses: NC **and** "accurate reproduction" |
| OPP-115, CEPT | yes, research use | no | NC |
| CFPB narratives | unavailable anyway | no | withdrawn from publication |
| UNER-English | yes | no | CC BY-**SA**: share-alike would pull the repo's licence |
| Quebec enterprise registry | no | no | NC *and* the names are stripped |
| OGL-Canada (federal) | murky | no | not an NC clause: the licence "does not grant any right to use: Personal Information", so it is silent, not permissive |
| CoNLL-2003 | yes, with care | **never** | no licence at all; the text is Reuters copyright |
| OntoNotes, ACE | only with an LDC agreement | no | access barrier, then user agreement |
| MultiNERD, Few-NERD | yes | no | machine-generated labels, not gold |

**Clause types, because they behave differently:**

- **NC** (non-commercial) — bites on selling and on redistribution, not on a person measuring at home.
  Ryan's reading is right for this one.
- **ND / "accurate reproduction"** — bites on *transformation*, money or not. Anonymising a judgment is
  a transformation, so Hansard and Quebec debates stay out of any derived artefact regardless of who
  we are.
- **Carve-out** ("does not cover personal data") — grants nothing rather than forbidding something;
  whether a research exception covers it is a jurisdiction question, not a reading of the licence. I
  would not build a habit on it.
- **Share-alike** — infects the repository's licence, so it fails an "MIT and nothing else" test.
- **Research-only / DUA** — a purpose restriction plus, usually, an agreement to sign.

**The test to apply before committing anything:** could a small business that clones this repo still
say, honestly, "everything here is MIT except the listed third-party files, and those are permissive
too"? If not, the text does not go in the repo, however we came by it.

**No public corpus of French-Canadian business correspondence exists** under a usable licence, and
none of the Canadian sources that do have real names can be committed. The French story therefore
stays what the README says it is: untested, offered as candidates, meant to be kept by the person.

### What TAB already tells us, before any model runs

`tab_overlap.py` asks, of TAB's 8,596 annotated person mentions: is every token an English dictionary
word (so the "not an English word" rule is silent) **and** is no token in the first-name list (so the
first-name rule is silent)? Those mentions cannot be seen by the word rules at all.

- every token an English word: **9.5%**
- no token in the first-name list: **66.3%** (they are still caught, because they are not English
  words; this is the *list* gap, not a leak)
- **both → invisible: 6.9%**, e.g. `Crow`, and annotation quirks such as `Thrilling` and one span that
  is a whole sentence

So on real legal prose, roughly one person mention in fourteen sits where the deterministic rules have
no signal — and some of that 6.9% is annotation noise rather than a true miss. That is a small number
and it is the right shape: the residue is the ambiguity class, not the volume class. It is also the
first honest recall estimate the project has that is not measured on eleven lines from a bike shop.

**(C) The tool's own output.** Run `scrub` over (B) and look at what survived. No labels at all, and
no adjudication — this produces the leak rate directly. See step 6.

## Step 2 — The model pass

Offline, on one machine, never in the repo, never on anyone's real files.

```
for each line:  bert-small int8  ->  spans  ->  JSONL {doc, line_no, start, end, label, score}
```

Chunk long bodies into 448-token windows with 64 tokens of overlap; keep only `PERSON`/`ORG`. The
runners in `model-eval/` (`run_onnx.py`, `run_model.py`, `merge_spans.py`) already do the decoding and
the span merging; the int8 runtime is 0.6 ms a line, so a few thousand lines is seconds.

## Step 3 — Adjudication, and the size of the human job

`teach.py` diffs three inputs — the corpus, the word check's output (`tools/wordreport`), and the
model's spans — into three buckets:

| bucket | meaning | who looks |
|---|---|---|
| **agreed** | both flag it | nobody |
| **word-check-only** | the model missed something the rules caught: the model's blind spots | nobody, unless a pattern screams |
| **model-only** | the model caught something the rules missed: **a candidate rule gap, a missing list entry, or a model error** | a person, once |

Only the smallest bucket needs eyes. On the evaluation's corpora it is 7 items out of 50 lines
(`hard`) and 15 out of 125 (`repo`) — a page, not a project. Each gets one word of verdict:
*rule*, *list*, or *wrong*.

The word-check-only bucket is worth reading once even though it needs no per-item work: it is where
the word check's false-positive cost shows up as a list of brands, places and calendar words a person
would press "keep" on. Tuning that list is a separate, smaller win.

## Step 4 — Turn the bucket into a rule

Each adjudicated item is one of three things, and each has a different home:

- **Rule** — a pattern of the text, not of a word. Today's example: `@grace_lee` was invisible
  because the word check skips words after `@` and because `grace` is a dictionary word. One pattern
  (decision 90) closed it and moved the hard-set score 77% → 79% with no new false flags.
- **List** — a word that should be in a list (a first name the Ontario/Quebec lists miss, a brand
  that names a person). Cheap, but every addition is a future false negative elsewhere; a list entry
  needs two independent instances before it earns a place.
- **Wrong** — the model is mistaken. `pads` read as a person, `lease` as a person, `cc` as a person.
  Four of fifteen on the repo corpus. Record it, discard it, and note that this is why the model
  proposes and the person decides.

## Step 5 — Freeze what you learned

The adjudicated set becomes the corpus Standin has never had: lines, their gold name spans, and the
classes they probe. Two artefacts:

1. `testdata/eval/` — the corpus, synthetic only, committed.
2. A Go test that runs the word check over it and fails when recall drops. Not a model test: a
   regression test for the rules, which is what a rule change can break.

Without this, every future rule change is judged by "the fixtures still pass", and the fixtures are
eleven lines from a bike shop. With it, a change either keeps recall or says what it lost.

## Step 6 — The number that decides the sidecar

Run `scrub` on corpus (B), then run the model over `scrubbed/` and count the `PERSON` spans that
survived. That ratio — **surviving spans per thousand words** — is the whole argument for shipping a
model:

- **under ~1 per 1000 words**: the model is buying a rounding error. Skip the sidecar, keep the rules.
- **over a handful**: the rules are missing a class, and the bucket review (step 3) tells you which.
  Fix that class first, then measure again — the model is doing the work of telling you what to build,
  which is worth more than shipping it.
- **still high after two rounds**: the residual is genuinely model-shaped (a name that is a word, in
  an ambiguous position), and now a sidecar has an evidence file behind it.

**The one benchmark that scores what Standin cannot do.** RAT-Bench (Feb 2026) anonymises profiles
and then has an LLM *attacker* try to re-identify them, reporting a re-identification rate against
census microdata instead of a span F1 — the honest yardstick for a "share safely" claim, and the only
public benchmark of its kind found. It is synthetic, US-centred, and its MIT is declared on the model
card while the GitHub repo has no LICENSE file. Not needed for the teacher pass; worth knowing that if
Standin ever publishes a claim, this is the thing to run it through rather than a recall number.

This ordering matters: the audit is also the cheapest possible form of the product. If it is useful
on its own, `standin audit` can ship as a one-shot command reading `scrubbed/` and printing counts,
which needs no config change, no scrub-path change and — because it is one-shot — no native library.

## Worked example, 2026-10-31

`hard` (52 lines, 39 gold names): 26 agreed, 17 word-check-only, **8 model-only** — five names that
are words (`Rose`, `Rich`, `Mark`, `Daisy`, `Stone`), one path segment (`ana-vantreeck`), one model
error (`cc`), and one real miss the first version of the tool hid (`Mei-Ling`: a hyphenated name whose
second part is an English word, so neither the dictionary rule nor the first-name rule fires).

`repo` (125 lines, 93 gold names): 89 agreed, 38 word-check-only, **17 model-only** — ten one-word
lines, one mid-sentence word-name (`Grace has the keys`), and the rest model errors (`pads` ×3,
`lease`).

**The bucket tool had the same bug as the harness, and hid rows.** `teach.py` first asked whether a
model span *contained* a word-check candidate. A span `Mr G. Blades` contains the candidate `Mr`, so
the row filed as "agreed" and the name underneath never reached a person — the TAB adapter's diagnostic
counted 37 of 49 recoverable misses hidden this way. Agreement now requires a **name token**: titles, ranks and initials never
count. That is the second time in this exercise that the measuring tool, not the detector, was the
thing that was wrong; budget for it.

**Caveat, and it is the lesson of the exercise:** those ten one-word lines are an artefact of scoring
lines in isolation. The sign-off rule needs the *previous* line, and the same lines are caught when
they are scored in blocks (the block run misses only the two names that sit at the *top* of a message,
where there is no closing line to lean on). The first useful output of a teacher pass was a lesson
about the harness, not about the rules — which is why step 3 hands a person a short list rather than
an automatic fix.

The residual class after that correction is real and small: a name alone as the first line of a
message body. A rule exists for it ("one capitalised word, in the first-name list, alone on the line,
at the start of a body"), and it is not written yet because its false-positive risk (a heading, a
list item, a one-word paragraph) needs measuring on corpus (B) first.

## What the teacher pass cannot do

- **It cannot label what the model is bad at.** Names that are words are the model's weakest class;
  the bucket simply shows them and a person rules. That is why the review step is not optional.
- **It cannot tell you about re-identification.** Column rules and word checks work on spans; a
  postal code plus a job plus a date still points at one person, and no span detector sees that. The
  README already says so; the teacher pass does not change it.
- **It cannot use real customer data.** Corpus (B) is public for a reason. Anyone running the pass on
  their own files is doing exactly what the tool exists to avoid.
- **It does not replace `approve`.** A model-plus-rules candidate list is still a list a person must
  rule on; the pass makes the *rules* better, not the person's job smaller.

## Effort

| piece | state |
|---|---|
| corpora, runners, scorer, span merger | **done** — `model-eval/` |
| three-bucket diff | **done** — `teach.py` |
| TAB overlap measurement | **done** — `model-eval/tab_overlap.py` |
| TAB → corpus JSONL adapter (so the same scorer runs on it) | ~an hour |
| Enron slice adapter (internal only, never committed) | ~half a day |
| adjudication of ~200 rows | an hour or two of attention |
| `testdata/eval/` + a Go regression test | ~half a day |

The whole pass is a day of work after the corpus adapter, and it produces the two artefacts the
sidecar decision is currently missing: a recall number that is not measured on eleven lines, and a
leak rate.
