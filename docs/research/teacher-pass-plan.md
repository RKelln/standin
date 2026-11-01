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

**(B) Real and public, no labels.** The gap between "my generator is messy" and "an actual inbox is
messy" is the whole question, and it cannot be closed with real private data — the repo rule and the
tool's own ethics both forbid it. Use public business email corpora (the Enron corpus is the standard
for PII and NER research: real, public by court order, shop-shaped). A few hundred messages is
enough. Truth here is unknown, which is the point: it is the model's job to propose, and a person's
job to adjudicate.

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

This ordering matters: the audit is also the cheapest possible form of the product. If it is useful
on its own, `standin audit` can ship as a one-shot command reading `scrubbed/` and printing counts,
which needs no config change, no scrub-path change and — because it is one-shot — no native library.

## Worked example, 2026-10-31

`hard` (52 lines, 39 gold names): 27 agreed, 16 word-check-only, **7 model-only** — five names that
are words (`Rose`, `Rich`, `Mark`, `Daisy`, `Stone`), one path segment (`ana-vantreeck`), and one
model error (`cc`). Reading seven rows produced the handle rule.

`repo` (125 lines, 93 gold names): 91 agreed, 36 word-check-only, **15 model-only** — ten one-word
lines, one mid-sentence word-name (`Grace has the keys`), and four model errors (`pads` ×3, `lease`).

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
| public-corpus adapter (Enron slice → the same JSON) | ~half a day |
| adjudication of ~200 rows | an hour or two of attention |
| `testdata/eval/` + a Go regression test | ~half a day |

The whole pass is a day of work after the corpus adapter, and it produces the two artefacts the
sidecar decision is currently missing: a recall number that is not measured on eleven lines, and a
leak rate.
