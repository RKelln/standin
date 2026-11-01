# Real prose, gold labels: the word check on 254 ECHR judgments

The first precision/recall number for Standin's free-text word check that is not measured on text I
wrote myself. It comes from the Text Anonymisation Benchmark (TAB, MIT): 254 real European Court of
Human Rights judgments from the dev and test splits, 218,185 words, with person mentions annotated
by character offset and a mask decision — `DIRECT` (full name), `QUASI` (a description that still
identifies, `the applicant's lawyer`), `NO_MASK` (an office or public figure that needs no masking).

Everything below was produced by the four scripts in `model-eval/`; the commands are in the last
section. Nothing here is committed except the tooling and this file: the corpus holds real document
text and stays in `/tmp/tab-eval/`.

## 1. What was scored

| | |
|---|---|
| documents | 254 (127 dev + 127 test) |
| lines emitted | 4,660 — every non-blank line, so the false-flag rate has unannotated prose in its denominator |
| words | 218,185 (≈ 47 words a line: a TAB "line" is a paragraph) |
| annotator | one per document: the numerically first named in `quality_checked` (all 254 documents have one) |
| person mentions found | 2,347 |
| — dropped, straddling a line break | **0** |
| — dropped, an initial only (`J`, `Ms A`, `W.K.`) | 296 |
| — kept | 2,051 (`DIRECT` 470, `QUASI` 1,518, `NO_MASK` 63) |

Three of those rows need saying out loud.

**Straddling mentions: zero.** The handoff expected a handful of spans crossing a line break. TAB's
text is one paragraph per line with no hard wrapping, so no annotation crosses a break. The
line-granular limit of the harness was therefore never exercised — the count is zero because the
corpus does not contain the case, not because the adapter silently swallowed it. (Checked two ways:
the adapter's drop counter, and an independent count of mentions whose start and end fall in
different `\n`-delimited spans.)

**296 mentions dropped as initials.** `Mr B`, `J.G.`, `Sergeant H` — 12.6% of all person mentions.
After titles and initials are stripped there is no token left to look for, and a lone initial is not
something a masking tool should act on. Leaving them in the gold set would count them all as misses
and pull mention recall down to about 85%; that number would measure the annotation convention, not
the detector. They are excluded and this paragraph is the caveat.

**One line is one paragraph.** The word check in the shipped tool sees a whole file, notes included,
so it can use the previous line and a name it already saw. Each corpus line is scored alone here, so
this is if anything a *lower* bound on its behaviour in the product.

## 2. Recall on the mentions that must be masked

`DIRECT` and `QUASI` only — 1,988 mentions. A mention counts as caught when at least one of its name
tokens is a word-check candidate on that line, or its span overlaps a pattern hit.

| subset | n | caught | recall | whole mention masked |
|---|---|---|---|---|
| **DIRECT + QUASI** | **1,988** | **1,924** | **96.8%** | 95.5% |
| DIRECT | 470 | 467 | 99.4% | 98.9% |
| QUASI | 1,518 | 1,457 | 96.0% | 94.4% |
| every token an English dictionary word | 144 | 80 | **55.6%** | 54.9% |
| at least one token not a dictionary word | 1,844 | 1,844 | **100%** | 98.6% |
| single-token mentions | 1,274 | 1,211 | 95.1% | 95.1% |

**96.8%**, and the misses are one class and nothing else. All 64 misses sit in the 144 mentions whose
every token is an English word:

> Beet, Berry, Blades, Burden, Carter, Crow, Dance, Earl, Gamble, Kart, Leach, Madden, Martens, Mole,
> Palm, Price, Reed, Right, Sawyer, Starling, Thrilling, Turner, Vita, Winks, Wright — plus `juror`
> (a role annotated as a person) and `AM` (an annotation artefact).

On this corpus the "not an English word" rule is worth more than the whole rest of the check put
together: 1,844 of 1,844 mentions containing one non-dictionary token were caught. That is the same
finding as the invented-text evaluation, on 1,988 gold mentions instead of 132, and it says the
residue really is the ambiguity class — a surname that is also a word, with no title or first name
the lists know.

The full-cover column matters for the tool, not the scorer: masking `Ganshof` and `Meersch` but leaving
`van der` is a partial leak, and it happens on 26 of the 1,988 maskable mentions (1.3%) — multi-part
and particle names (`Mr P. van Dijk`, `Scottish Minister of State`), not the `Mr Initial Surname`
shape I expected.

## 3. False flags: 60 words in 1,000

Every word-check candidate becomes a stand-in unless the person keeps it, so a candidate that is not
a gold name is a false flag whether or not a human later presses "keep".

| | |
|---|---|
| false flags (candidate occurrences) | **13,124** |
| rate | **60.2 per 1,000 words** |
| by group | `unusual` 4,761 (21.8/1,000), `calendar` 4,249 (19.5/1,000), `name-like` 4,114 (18.9/1,000) |
| excluding calendar words (a group the config may keep) | 8,875 = 40.7 per 1,000 |
| distinct words flagged wrongly | 1,088 |
| lines with at least one flag | 3,625 of 4,660 (77.8%) |
| pattern hits | 29 (28 street addresses, 1 phone number) |

The rate is high, and most of it is the register rather than a bug. The top of the list is `Mr` ×1,342,
`Court` ×551, then **all twelve month names** (July 308 to November 438), `CASE` ×221, `Ms`, `Mrs`,
`Turkish`, `Security`, `State`, `Ankara`, `Istanbul`. A judgment names the Court, the State, the
Convention and a dozen institutions in every paragraph. A shop's email does not — but a shop's email
does name the city, the supplier, the product line and the month, so the shape of the problem is the
same even though the magnitude is not. This is the first real number for the `approve` burden, and it
is the honest counterweight to the recall figure: **the word check is not precise on dense prose, and
on a document like this a person would keep most of what it offers.** The README's advice — `drop`
the text rather than read a long list — is the right response to this number, not a threshold.

## 4. `NO_MASK`: over-masking, reported separately

TAB marks 63 of the kept mentions `NO_MASK`: judges sitting in public, a lord, `Chancellor of
Justice`, a named prosecutor. The annotators decided the person cannot be protected this way. Standin
is default-deny and would mask them anyway.

**39 of 63 (61.9%) would be masked.** Examples: `Lord Phillips`, `Mance LJ`, `Chancellor of
Justice`, `Mr L. Wildhaber`, `Mr J. Wołąsiewicz`, `Ms A's`.

Not scored as misses and not proof of an error: it is a policy difference. It is also the number that
would matter if the tool ever claimed "these are the only people named in this file" — on this
register it would over-claim by more than half.

## 5. What the model adds, and what the union is worth

The 28.7 MB int8 bert-small (Apache-2.0), same corpus, span-overlap matching:

| | word check | model | union |
|---|---|---|---|
| DIRECT + QUASI (1,988) | 96.8% | 92.5% | **99.2%** |
| every token an English word (144) | 55.6% | 77.8% | **89.6%** |

The model loses overall to the rules on this register — it is 4 points behind — and wins exactly
where the rules are blind, the dictionary-word surname. Together they cover 99.2%, and 89.6% of the
class the tool is documented as missing. Two cautions before anyone quotes the union: it is 15
remaining misses of a 1,988-mention set with an inter-annotator question behind it, and the union
number has no precision column — the model's extra spans on this text are mostly organisations and
places (§6), so shipping it would add flags, not just catch names.

## 6. The three-bucket output from `teach.py`, summarised

`teach.py` on the whole corpus prints **1,134 model-only, 7,803 word-check-only, 5,828 agreed** rows.
On invented shop prose the model-only bucket was 7–15 rows and a person read it; 1,134 is not a page,
so `tab_buckets.py` joins each printed row back to TAB's gold and to the word check's output and says
what it is:

| what the row is | rows | share |
|---|---|---|
| a maskable person the word check missed entirely — **the rows worth reading** | 23 | 2.0% |
| a maskable person the word check already caught; the model found another piece | 73 | 6.4% |
| a `NO_MASK` person (an office, a public figure) — over-masking | 52 | 4.6% |
| an annotation of another type (`ORG`, `LOC`, `DATETIME`) — the model called it a person | 674 | 59.4% |
| nothing annotated overlaps — model error or annotation gap | 312 | 27.5% |

So the genuine rules misses in the bucket are **23 rows**, and the classes are what §2 found: initials
(`Miss M`, `J.G.`, `R.D.`, `İ.K.`), a role (`juror`), and the dictionary-word surnames (`Winks`).
Fifty-nine per cent of the bucket is the model labelling `Court`, `Polish Government`, `Ministry of
Foreign Affairs` and `5th` as people — the register effect again, on the model's side this time.

**The bucket under-reports by design, and this is the harness lesson.** Scoring says the model
recovers 49 of the 64 misses; `teach.py` shows only 23 of them as model-only. The missing 37 are
hidden by its containment test: when the model emits the whole `Mr G. Blades` span and the word check
has `Mr` as a candidate, `"Mr" in "Mr G. Blades"` is true, so the row is filed under "agreed" even
though the surname was never caught. Counting the union (`tab_score.py`) is the honest way to say
what the model adds; reading the bucket is the way to decide what to *fix*, and on `Mr Initial
Surname` shapes the bucket is blind.

## 7. Which annotator, and does it matter

TAB ships several annotators per document (115 of the 254 have more than one quality-checked
annotator) and their spans agree almost exactly where they overlap. Merging them would have counted a
person once per annotator, so the adapter takes **one per document: the numerically first name in
`quality_checked`**, recorded per line in the sidecar. `--annotator` allows the other two rules, and
the answer does not move:

| rule | mentions | recall |
|---|---|---|
| first quality-checked (used) | 1,988 | 96.8% |
| last quality-checked | 1,964 | 97.3% |
| always `annotator1` where present | 1,973 | 96.9% |

Half a point either way, on a corpus whose annotators already agree on where the names are.

## 8. What surprised me

- **The dictionary-word class is the whole miss set, at 100% separation.** I expected a tail of
  non-Anglo names, digraphs and hyphenation to fail. `İşeri`, `Türmen`, `Zupančič`, `Kūris` were all
  caught by the "not an English word" rule. Getting the tokenizer right mattered more than any
  feature: the first run of the adapter counted `İşeri` and `Wołąsiewicz` as misses only because the
  Python side lifted `İ` differently from Go's `strings.ToLower` and glued `Mr<U+F020>L.` into one
  token. Two bugs, ~3 points of recall, both found by reading the misses rather than the score.
- **Zero straddling spans** in 254 documents. The harness limit the handoff worried about does not
  exist in this corpus.
- **The model loses to the rules on legal prose but wins the class that matters**, which is the same
  shape as the invented-text result — except there the rules won the class too. The ambiguity class
  is 7% of the mentions here (144 of 1,988); the invented set was written to make it the majority.
- **`teach.py`'s containment test hides exactly the misses worth fixing.** A bucket that looks
  "agreed" on `Mr G. Blades` is the case decision 90 was invented for.

## 9. Reproduce

```sh
git clone --depth 1 https://github.com/NorskRegnesentral/text-anonymization-benchmark /tmp/tab
cd docs/research/model-eval

# sanity: the overlap number the teacher-pass plan quotes
python3 tab_overlap.py /tmp/tab/echr_dev.json /tmp/tab/echr_test.json
#   person mentions: 8596 ... BOTH -> invisible: 594 = 6.9%   (reproduced)

python3 tab_to_corpus.py /tmp/tab/echr_dev.json /tmp/tab/echr_test.json
#   -> /tmp/tab-eval/corpus_tab.json          (the harness corpus)
#   -> /tmp/tab-eval/corpus_tab_mentions.json (per-mention offsets, decision, tokens)

go build -o /tmp/wordreport ./tools/wordreport      # from the repo root
/tmp/wordreport /tmp/tab-eval/corpus_tab.json > /tmp/tab-eval/det_tab.json

~/.hermes/cache/scratch/pii-eval/.venv/bin/python run_onnx.py /tmp/tab-eval/corpus_tab.json /tmp/tab-eval/int8_tab.json
#   onnx-int8: 4660 lines in 10.0s (2.1 ms/line)
~/.hermes/cache/scratch/pii-eval/.venv/bin/python merge_spans.py /tmp/tab-eval/int8_tab.json

python3 tab_score.py /tmp/tab-eval/corpus_tab.json /tmp/tab-eval/det_tab.json /tmp/tab-eval/int8_tab_m.json
python3 teach.py /tmp/tab-eval/corpus_tab.json /tmp/tab-eval/det_tab.json /tmp/tab-eval/int8_tab_m.json > /tmp/tab-eval/teach_tab.txt
python3 tab_buckets.py /tmp/tab-eval/teach_tab.txt /tmp/tab-eval/corpus_tab.json /tmp/tab-eval/det_tab.json \
    /tmp/tab/echr_dev.json /tmp/tab/echr_test.json

# the annotator sensitivity table
python3 tab_to_corpus.py /tmp/tab/echr_dev.json /tmp/tab/echr_test.json --annotator last-qc --out /tmp/tab-eval/last.json
python3 tab_score.py /tmp/tab-eval/last.json /tmp/tab-eval/det_tab.json
```

The venv existed from the previous session (torch CPU, transformers ≥5.2, onnxruntime); nothing had
to be rebuilt. `score2.py` was not modified — it loads corpora from its own directory by name, and
the TAB corpus deliberately does not live there. `tab_score.py` is the two-file comparison instead.

## Proposed for the README

> **How well it does on real prose.** On 254 real European Court of Human Rights judgments — 218,000
> words, and 1,988 mentions of people that the court's own annotators said had to be masked —
> Standin's word check caught 96.8%. Every one of the 64 it missed was a name that is also an ordinary
> English word (Price, Wright, Turner, Crow, Earl, Dance) or a bare role like "juror". Where a name
> contained any word that is not in the English dictionary it caught 100% of them. This is a court
> judgment, not an email: it is written in full sentences with titles ("Mr G. Blades"), which gives
> the check more to go on than a one-line note does, and it is dense with place and institution names
> that it will also offer you — around 60 words in every 1,000, mostly "Court", "State" and month
> names, on this kind of text. Read 96.8% as the ceiling for that register, not as a promise about
> your inbox. On invented shop text — sign-offs, one-word lines, lower-case names, links — the same
> check measured 79–90%.

## Review note: independent reproduction

Reproduced by the orchestrating session on 2026-10-31 from the same artifacts, without re-running the
adapter:

```sh
cd /tmp/tab-eval
python3 ~/Documents/Projects/standin/docs/research/model-eval/tab_score.py corpus_tab.json det_tab.json int8_tab_m.json
```

Every headline figure matched to the decimal: 1,988 mentions, 1,924 caught (96.8%), 55.6% on the
144 all-dictionary mentions, 100% on the rest, 60.2 false flags per 1,000 words, model 92.5%,
union 99.2%. The scripts compile, `make ci` passes, and no Go source or corpus text is in the
working tree.

Two corrections to the interpretation above, neither changing a number:

- **`teach.py` was hiding rows and has been fixed.** It treated a model span as already caught when
  a word-check candidate appeared anywhere inside it, so `Mr G. Blades` counted as agreement because
  the candidate `Mr` is a substring. Agreement now requires a *name token* (titles, ranks and
  initials excluded). Re-run on the shop corpora, the bucket grew from 7 to 8 rows (hard) and 15 to
  17 (repo); on this corpus the model-only count is correspondingly larger than the figures in the
  section above.
- **The union gain and the bucket count are not the same measure**, and the gap is meaningful: the
  union adds 49 mentions, while only some of those are whole-mention misses. The rest are mentions
  where the word check masked one token and the model masked the span — which is why
  whole-mention coverage moves 95.5% → 99.1% under the union while mention recall moves 96.8% →
  99.2%. Read the `full` column, not just `recall`.

## After decision 91: title + initials, and titles as noise

The 64 misses were not a hard class. 43 of them were `title + initial + surname` (`Mr D. Price`,
`Mrs E. Palm`, `Mr S.K. Martens`), four were `rank + surname` (`Sergeant Gamble`), six were bare
word-names (`Beet`, and the court's own `AM` anonymisation), and eleven were the role `juror`, which
is a QUASI annotation on a role rather than a name. The first two groups were a bug: the title opened
a name spot on the *initial*, the initial was discarded for being one character, and the surname then
had no signal at all.

With decision 91 in place (the spot survives initials, ranks joined the titles, titles stopped being
candidates), the same corpus and the same scorer give:

| | before | after |
|---|---|---|
| mention recall (DIRECT+QUASI) | 96.8% | **99.1%** |
| whole-mention masked | 95.5% | **98.2%** |
| all tokens English words | 55.6% | **88.2%** |
| single-token mentions | 95.1% | 98.7% |
| false flags per 1,000 words | 60.2 | **51.7** |
| false flags excluding calendar words | 40.7 | 32.3 |

Reproduce with `go build -o /tmp/wordreport ./tools/wordreport` and re-running the commands in the
last section against a fresh `det_tab.json`.

The residual after the fix is 17 mentions: 11 `juror` (a role), 4 `AM` (the court's own
pseudonymisation, which the annotators treat as identifying), and 2 `Beet`. That is close to the
floor for a word-level detector: nothing in "the applicant Beet" distinguishes the surname from the
vegetable except the syntax around it.

### What the fix did to the model's case

Same corpus, same model output (`int8_tab_m.json`), the improved word check:

| | rules alone | bert-small int8 alone | union |
|---|---|---|---|
| DIRECT+QUASI (1,988) | **99.1%** | 92.5% | 99.2% |
| all tokens English words (144) | **88.2%** | 77.8% | 89.6% |

The model's marginal value collapsed: before decision 91 the union beat the rules by 2.5 points
(96.8% → 99.2%) and by 34 points on the word-name class (55.6% → 89.6%). After the fix it adds **two
mentions** — 0.1 of a point overall and 1.4 on the class.

That is the most useful number in this document, and it is not about the model. Most of what the
model appeared to contribute was a proxy for a bug: on real judgments a name nearly always arrives
with a title or a rank in front of it, and the detector was dropping the name at the initial. Fix the
detector and the model's advantage on this corpus is almost entirely explained away.

It does **not** settle the model question, for one reason: ECHR judgments are formal. Almost every
name in them is `Mr X` or `Sergeant Y`. Shop correspondence is not — `Rose called about the invoice`
has no title, no rank and no dictionary clue, which is precisely the case a model is for. Whether
that case is common in a real inbox is the audit question (`teacher-pass-plan.md` step 6), and it
needs corpus (B), not TAB.
