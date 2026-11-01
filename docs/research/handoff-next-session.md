# Handoff: the TAB adapter, and the first real recall number

Written 2026-10-31 at the end of a session that evaluated model-based PII detection, shipped two
changes, and picked a corpus. Everything below is in `~/Documents/Projects/standin` unless said
otherwise. Nothing is half-applied; the tree was clean at commit `9b0e017`.

## Where things stand

Four commits today:

| commit | what landed |
|---|---|
| `8327498` | `@handle` is a pattern, becomes `[USERNAME_1]`; new `USERNAME` category; DESIGN decision 90 |
| `44a5c3f` | the model evaluation and its harness (`docs/research/model-based-detection-eval.md`, `docs/research/model-eval/`) |
| `4f36614` | pure-Go inference measured (hugot + GoMLX) and an earlier estimate corrected |
| `09b541d`, `9b0e017` | the teacher-pass plan, the sidecar decision note, the corpus licence board — split into "may we measure" vs "may we commit" |

`make ci` passes. The tree is clean as of this file's commit.

## The task: TAB → corpus adapter → the first real numbers

**Goal.** Measure the shipped word check against gold person-name annotations on real prose, and
produce the model-only bucket on text where the answer key exists. This is the hour of work that
turns the evaluation from "measured on eleven lines from a bike shop" into a number worth putting in
the README.

**Why TAB.** `NorskRegnesentral/text-anonymization-benchmark`: 1,268 real ECHR judgments, MIT,
ungated, and labelled with `DIRECT` / `QUASI` / `NO_MASK` mask decisions plus co-reference chains.
It is the only permissive real-text corpus with gold person annotations (see
`teacher-pass-plan.md` §"The licence board").

**Getting it.**

```sh
git clone --depth 1 https://github.com/NorskRegnesentral/text-anonymization-benchmark /tmp/tab
ls /tmp/tab   # echr_dev.json echr_test.json echr_train.json evaluation.py guidelines.md
```

**Shape of the data** (verified this session, `echr_test.json` first element):

```
[ { "doc_id": "001-61807", "text": "...", "dataset_type": "test",
    "meta": {"year":..,"legal_branch":..,"articles":[..],"countries":"TUR","applicant":".."},
    "annotations": { "annotator1": { "entity_mentions": [
        {"entity_type":"PERSON","entity_mention_id":"..._em1","start_offset":54,"end_offset":68,
         "identifier_type":"DIRECT"|"QUASI"|"NO_MASK","entity_id":"..._a1_1"} ] } },
    "quality_checked": ["annotator1"] } ]
```

`entity_type` values seen: `PERSON`, `CODE`, `LOC`, `ORG`, `DATETIME`. `entity_id` links mentions of
the same entity across a document (that is the co-reference chain).

**Steps.**

1. `docs/research/model-eval/tab_overlap.py` already reads this shape and answers the overlap
   question (6.9% of 8,596 mentions invisible to the word rules — reproduce it first as a sanity
   check). Reuse its title/initial stripping; it is the difference between a real number and a
   wrong one.
2. Build `docs/research/model-eval/tab_to_corpus.py`: emit the same corpus JSON the rest of the
   harness already consumes — `{"id", "line", "gold"}` where `gold` lists the person strings that
   fall on that line, with `class` naming the identifier type. **The hard part is offsets**: TAB
   annotations are character offsets into a document; slice each document into lines (split on
   `\n`, keep the running offset), and re-base each span to the line it starts in. Drop mentions that
   straddle a line break and count how many you dropped — a straddling mention is a real case the
   line-granular word check cannot represent, and the number belongs in the write-up.
3. Run the existing pipeline unchanged:
   - `go build -o /tmp/wordreport ./tools/wordreport` then `/tmp/wordreport docs/research/model-eval/corpus_tab.json > det_tab.json`
   - `python run_onnx.py corpus_tab.json int8_tab.json` (needs the venv at
     `~/.hermes/cache/scratch/pii-eval/.venv`; if it is gone, rebuild with uv + torch cpu +
     transformers + onnxruntime, and note that in the report)
   - `python merge_spans.py int8_tab.json`, then `python score2.py` (add a `TAB` entry to its
     `MODELS` map, or run the two-file comparison inline)
   - `python teach.py corpus_tab.json det_tab.json int8_tab_m.json` for the three buckets
4. Report: recall on `DIRECT`+`QUASI` mentions, the count of false flags per 1,000 words, and the
   model-only bucket. **Do not score `NO_MASK` as a miss** — the annotators decided those need no
   masking (public figures, offices, `Judge Advocate`); Standin's default-deny would mask them
   anyway, which is a policy difference, not an error. Report them separately as "over-masking".
5. Write the numbers into `docs/research/model-based-detection-eval.md` (a new section, "Real prose,
   gold labels") and, if the numbers hold, a line in the README's "What it misses" that is no longer
   a guess.

**Known gotchas.**

- **Register mismatch.** ECHR judgments are not email. There are no `Hi` / `Dear` / sign-offs, so the
  name-spot rules never fire; expect recall below the 79–90% measured on shop prose, and say so
  rather than treating it as a regression. Also break the number down by capitalisation: the
  interesting subset is mentions whose tokens are all dictionary words (`Crow`).
- **Two annotators.** Some documents carry `annotator1` and `annotator2`. Use `quality_checked` when
  present, else `annotator1`, and state which — do not merge silently.
- **Nested and repeated mentions.** A name can be annotated more than once in a document
  (`Mr Crow`, `Crow`, `the applicant`). Dedupe by `(entity_id, start, end)` before scoring.
- **The word check is line-granular, the corpus is document-granular.** That mismatch is the thing
  step 2 exists to bridge; a straddling span is a harness limit, not a detector miss, so count it
  separately.

## Constraints

- **Agents do not commit** (`docs/PLAN.md`, "Agents do not commit; the orchestrator reviews and
  commits"). Produce the patch and the numbers; leave committing to review.
- **No corpus text in the repo** beyond what a licence allows: TAB is MIT so a small fixture is
  permissible, but the default is to commit the *adapter and the numbers*, not the judgments.
- Test data stays synthetic elsewhere (repo rule). Never put real personal data in `testdata/`.
- `make ci` must pass (fmt, vet, test, no-network, tidy-check, build).

## The open decision this feeds

`docs/research/sidecar-decision-note.md` §6 has four questions for Ryan. The load-bearing one is
**native library, yes or no**: it decides whether a shipped model can be the fast ORT sidecar
(0.6 ms/line, 132 MB, needs `libonnxruntime`) or the slower cgo-free one (19 ms/line, 1.09 GB). If
the answer is "no native lib", the audit command in §3B ships as pure Go and the cost line vanishes.

The recommended order stays: **teacher pass → audit → in-scrub detector**, because the audit's leak
rate is the evidence the detector needs.

## Files that matter

- `docs/research/model-based-detection-eval.md` — the evaluation and every measured number.
- `docs/research/teacher-pass-plan.md` — how the pass works, the licence board, the TAB overlap result.
- `docs/research/sidecar-decision-note.md` — the three shapes and the contract any in-scrub model must keep.
- `docs/research/model-eval/` — corpora (`corpus_hard.json`, `corpus_repo.json`, `corpus_repo_blocks.json`),
  runners (`run_model.py`, `run_onnx.py`, `run_piitracer.py`), `merge_spans.py`, `score2.py`,
  `teach.py`, `tab_overlap.py`, `gobench.go`, and `go-probe/` (a nested module for the hugot measurement).
- `tools/wordreport/` — prints the word check's candidates and pattern hits for a corpus; must live
  inside the module to import `internal/words`.
- `docs/DESIGN.md` decision 90 — the handle rule, including why paths stay a deliberate miss.
