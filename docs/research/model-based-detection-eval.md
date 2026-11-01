# Model-based PII detection: evaluation for Standin

Research note, 2026-10-31. Written after v0.2.0. Asks one question: **should Standin replace or
supplement its deterministic word check with a trained model, and if so, which one?**

Evidence: measured here on this machine, plus the published numbers, plus the shipping contract in
`docs/DESIGN.md` and `docs/PLAN.md`. Nothing in this note is a decision; decisions belong in
`DESIGN.md`.

---

## 1. Verdict

**The quality question is settled in the model's favour. The shipping question is not, and it is the
one that decides.**

- A **28M-parameter BERT-small model** (`gravitee-io/bert-small-pii-detection`, Apache-2.0, **29 MB**
  as int8 ONNX) finds **90% / 99% / 96%** of the names where the shipped word check finds **77% / 88% /
  90%**, at the same noise level (10 vs 11, 20 vs 20, 23 vs 20 extra flags on the same three sets).
- It runs in **0.6–1.2 ms per line** on this laptop's CPU. That is fast enough to be invisible.
- **None of the four models you sent is the right pick.** Two are the wrong shape, one is a licence
  dead end, and the strongest on paper (PII-Tracer) is the weakest on isolated sentences — 49% —
  because it is built for conversations, not lines.
- The blocker is Standin's own contract: one static Go binary, no cgo, no network, one vendored
  module, runs on a locked-down laptop. A model means **+29 MB and a Go inference engine that does not
  exist yet** (Go's own arithmetic measured at 8.7 GFLOPS here → ~0.3 s/line, i.e. ~250× slower than
  ONNX Runtime), **or** a second binary with a native library, which is a different product.

**Recommendation: do not put a model in the binary for v0.3.** Do three cheap things instead, in this
order: (a) use a model **offline as a teacher** to build the labelled corpus Standin has never had and
to mine its rule gaps; (b) fix the deterministic gaps the model exposed, which are rules, not model
work; (c) timebox a spike on pure-Go inference and let measured latency decide. Details in §7.

---

## 2. What job would a model actually do here?

Standin's deterministic engine is not a general PII detector and should not be compared as one. On
structured files it is *ahead* of every model in the field, because **column rules beat entity
recognition** when you know what a column is (this is design §3, and it is right). The measured
question is narrower:

> In free text — `.md`, `.txt`, `.eml` bodies, and `scan` columns — which names does the word check
> miss, and can a model find them without drowning the person in approvals?

The cost function is unusual and it favours recall:

- A **false positive** costs one more word on the `approve` screen. Group A is "unusual words, read
  with care"; group B is "these look like names, they all become stand-ins unless you keep one".
- A **false negative** is a leak into `scrubbed/`, which is the thing the tool exists to prevent.
- But noise is not free: decision 56 exists because *twenty "type yes" prompts get typed without
  reading*. A model that flags everything would degrade the human gate it is supposed to strengthen.

So the metric that matters is **recall at the current noise level** — not F1, and not the model cards'
token-level scores.

---

## 3. The baseline, measured

I ran Standin's own word check (`internal/words`) plus its pattern pass (`internal/scan.FindPatterns`,
which is what masks a prose email as `[EMAIL_1]`) over three corpora:

| Corpus | What it is | Gold name instances |
|---|---|---|
| `hard` | 52 lines I wrote to probe the documented miss classes (names that are also words, lower case, handles, links, non-Anglo names) + 18 control lines with no name | 39 |
| `repo` | Standin's own invented fixtures (`testdata/inbox.md`, `notes.md`), prose lines only, gold = `testdata/text-names.txt` | 93 |
| `repo_blocks` | the same prose, in 4-line blocks, so a model gets context | 91 |

**Shipped word check + patterns: 79% (hard), 88% (repo), 90% (blocks).** With 11/20/20 extra flags.
Some of those extras are brands, places and calendar words that a config keeps anyway, so the
approve burden is lower than the raw count suggests.

The misses are exactly the classes the README already admits: `Rose`, `Rich`, `Summer`, `Mark`,
`Daisy`, `Stone` (names that are also words), `oluwaseun` (lower case), `/people/ana-vantreeck`.
(`@grace_lee` was one of them until this evaluation; see decision 90 — handles are now a pattern.)
It does **not** miss non-Anglo names — `Xiaoming`, `Oluwaseun`, `Ngozi`, `Rajesh`, `Anjali`, `Dmitri`,
`Yusuf`, `Amara`, `Tariq`, `Hiroshi`, `Szymon`, `Priyanka`, `Nkechi`, `Kwame`, `Thandiwe` were all
caught, because "not an English word" is a strong signal. I expected that class to be the model's
advantage; it isn't. The word-list misses are narrower and harder than they look.

---

## 4. The four candidates

| | Params | Weights | Licence | Context | Measured ms/line | hard | repo | blocks |
|---|---|---|---|---|---|---|---|---|
| `perplexity-ai/PII-Tracer` | 600M | 1.19 GB | MIT | 4096 tok | 84–126 | 49% | 66% | **96%** |
| `iiiorg/piiranha-v1` | 278M | 1.11 GB | **CC-BY-NC-ND-4.0** | 256 tok | 46–51 | 74% | 51% | 31% |
| `betterdataai/PII_DETECTION_MODEL` | 494M | 1.98 GB | no licence field | Qwen2 0.5B | not run | — | — | — |
| `openai/privacy-filter` | 1.4B | 2.80 GB | Apache-2.0 | 131k tok | 70 | 59% | 86% | 84% |
| *(shipped word check, for reference)* | — | 270 KB of word lists | MIT | file | ~0.1 | 79% | 88% | 90% |

Recall is over gold name instances; "extra" is distinct flags that are not gold (see §5 for method).

**PII-Tracer** is the interesting one and the trap. It is the current best published result on
PII-TRACE (PII-TRACE benchmark, Sept 2026): entity-level consistent detection 0.853 against 0.386 for
OpenAI's filter and 0.649 for Presidio, at a false-positive rate of 0.385 on PII-free conversations.
It is a 600M bidirectional Qwen3 encoder with two heads and a constrained BIOES Viterbi decoder — the
same architecture shape OpenAI shipped in April. Its 96% on blocks vs 49% on single lines is the whole
story: **it needs conversational context**. Standin could give it a whole file, which helps, but the
model wants turns and speakers; a shop's `.md` note is not a conversation. 1.19 GB also means it is not
going in a binary, ever.

**piiranha** should be dropped on two independent grounds. (1) Licence: CC-BY-NC-ND-4.0 forbids
commercial use and forbids distribution of derivatives — Standin is MIT and meant for small businesses
that use it commercially. Even if it were perfect, it could not ship. (2) It measured worst on
Standin's own text (51% on prose lines, 31% on blocks) while its card claims 93 F1 and 98.27% PII-token
recall. The card's test set is in-domain financial documents; it degrades under different framing.
Its misses on my hard set were *exactly Standin's misses* — `Rose`, `Rich`, `Summer`, `Mark`, `Daisy`,
`Stone` — with the addition of accented names (`Anaïs`, `Tremblay`).

**betterdataai/PII_DETECTION_MODEL** is the wrong shape to even test. It is a Qwen2-0.5B causal
language model used as a prompt: you instruct it to list PII, it *generates* text. Last modified
2024-08-08, 43 downloads a month. A generator can invent a value or omit one silently, with no span
offsets and no fail-closed path — it is unusable in a tool whose whole design is "uncertainty means
refuse". The card also says apache-2.0 while the repo carries no licence field.

**OpenAI Privacy Filter** is the best-engineered thing in the field: Apache-2.0, 128k context,
bidirectional banded attention over a converted MoE LM, constrained Viterbi over 33 BIOES classes, and
runtime operating points that trade precision for recall. Measured: 86%/84% on realistic prose with the
**fewest extra flags of any model** (4–13), but only 59% on the adversarial set, and it is 2.8 GB of
weights for a 1.4B model that runs 70 ms/line here. Its own paper (§4, 32 benchmarks, Aug 2026) says
person names are its weakest class (F1 0.40), and it collapses outside Latin scripts. Also: I decoded
it with argmax+BIOES, not its calibrated Viterbi operating points, so treat its numbers as a
lower bound — that caveat is mine, not the model's.

---

## 5. What is actually SOTA as of October 2026

The field converged on one architecture in 2026: **take a pretrained language model, make it
bidirectional, put a BIOES tag head on it, and decode with a constrained Viterbi instead of per-token
argmax.** OpenAI (April), Perplexity (September) and most of the GLiNER2 lineage do this. The
Viterbi part is the quiet improvement: token-level argmax fragments spans (` Far` + `ida`), a
sequence decoder does not. It matters for any tool that replaces text, because a half-masked name is
worse than either extreme. (The CRF/Viterbi lineage is old — Lafferty, McCallum & Pereira 2001 — the
2026 change is putting it on top of a fine-tuned encoder.)

Two families on top of that:

1. **Privacy-taxonomy models** — trained to output `private_person`, `secret`, `account_number`.
   OpenAI Privacy Filter, PII-Tracer, piiranha, OpenMed's 54-label suite. Good on structured,
   regular PII (email, phone, card, SSN); weaker on person names, which is precisely Standin's job.
2. **Entity-taxonomy models** — trained on ordinary NER labels (`PERSON`, `LOCATION`, `ORGANIZATION`,
   `DATE_TIME`) on synthetic records. GLiNER family, gravitee's bert-small, most ai4privacy fine-tunes.
   Less "privacy-aware", better at *is this string a name*.

Worth knowing, and it is the most useful published result of the year: **PIIBench** (Apr 2026, 2.37M
sequences, 48 entity types) put eight systems — Presidio, spaCy, BERT-base NER, XLM-R NER, SpanMarker,
piiranha, XtremeDistil — through one normalised corpus and **every one scored below 0.14 span-level
F1**, Presidio best at 0.1385. That number is not a fair ranking of the tools; it is proof that
annotating "PII" across domains is incoherent and that every public benchmark measures its own
domain. Read every model card's F1 as "on their data".

Published cross-domain numbers, for orientation only:

| Model | PII-TRACE CD / FP₀ | Notes |
|---|---|---|
| Presidio (+spaCy) | 0.649 / 0.972 | flags nearly every document |
| OpenAI Privacy Filter | 0.386 / 0.557 | most precise of the public detectors |
| GLiNER-PII, GLiNER2-PII | 0.472 / 0.901, 0.448 / 0.862 | zero-shot flexibility, noisy |
| piiranha | 0.128 / 0.685 | collapses on conversations |
| OpenMed 44M / 434M | 0.724 / 0.936, 0.709 / 0.935 | high coverage, high false positives |
| PII-Tracer | **0.853** / 0.385 | best coverage at LLM-level precision |

Caveat on that table: FP₀ is measured on PII-free conversations, and CD rewards flagging documents.
A detector can raise CD by flagging everything — Presidio and OpenMed do.

---

## 6. What we measured ourselves

**Setup.** CPU only (Ryzen 7 8745HS, 16 threads, no GPU), Python 3.12 + torch 2.15 + transformers
5.22 for the PyTorch models, onnxruntime 1.31 for the int8 one. Each model sees the same lines.
Spans are decoded to offsets, adjacent same-type fragments merged, and a gold name counts as found
only if a predicted span **contains** it (not merely overlaps — an early looser metric inflated
OpenMed to 100% and was wrong).

**The table** (§4) is the result. Three findings are worth more than the numbers:

**1. The tiny model wins, and it is not close.** `gravitee-io/bert-small-pii-detection` — 4-layer
BERT, 28M parameters, 114 MB fp32 / **28.7 MB int8 ONNX**, Apache-2.0, trained on synthetic *records*
— beat a 600M conversation detector and a 1.4B frontier model on Standin's own text. It caught 10 of
the 11 things the deterministic check misses on the hard set, including `@grace_lee`,
`/people/ana-vantreeck` and `oksana.vantreeck@example.com`. **Scale is not the lever here; label
design and training framing are.** A model trained on "find the entity spans in this record" transfers
to Standin's job; a model trained on "detect privacy incidents in a conversation" does not.

**2. Context changes the ranking, so the framing decides the winner.** PII-Tracer went 49% → 96% when
the same text was presented as 4-line blocks instead of isolated lines. piiranha went 51% → 31% the
other way. Since Standin has whole files, this is a design variable it controls — and it is the thing
to test first in any future spike.

**3. A model *on top of* the current check is where the value is.** Union of the shipped check and
bert-small:

- `hard`: 38/39 = **97%** (residual: `Summer will handle the Dundas pickup` — a name that is a
  season, at a sentence start, next to another name)
- `repo`: 93/93 = **100%**
- `blocks`: 89/91 = **98%**

And the union is not noisier than today: 33 distinct extra flags on `repo` against 37 for the
deterministic check alone. The two systems fail on *different* things, which is the only reason a
second opinion is worth anything.

**4. int8 costs nothing measurable.** The 28.7 MB quantized ONNX scored 33/39, 92/93, 87/91 against
the fp32 model's 33/39, 90/93, 88/91, at 0.6–1.2 ms/line versus 4 ms/line. If a model ever ships, it
ships quantized.

**Update, 2026-10-31 (later the same day).** Decision 91 fixed a detector bug this evaluation
exposed — a title's name spot was dropped at an initial, so `Mr D. Price` named nobody. On 254 real
judgments that took the word check from 96.8% to 99.1% and the model's marginal value from +2.5
points to **+0.1** (two mentions). The shop-corpus union figures below (97–100%) are unchanged,
because those fixtures were written around the word-name class on purpose. Read this report's
"the model's value is the union" finding as conditional on the rules being what they were; the
authoritative before-and-after is in `tab-results.md`.

**What this measurement is not.** n = 39 / 93 / 91 name instances on synthetic text. The `hard` set
was written by me to probe specific classes, so it measures those classes, not real documents. The
`repo` set is Standin's own invented fixtures — small, and built to be easy. The numbers are
directional and the *ordering* is stable across three framings; the absolute percentages should not
be quoted as the product's recall. **Standin has no labelled evaluation corpus. That absence is the
real finding of this exercise**, and the first thing to fix.

---

## 7. What shipping a model would actually cost

The hard part is not the model. It is `DESIGN.md`'s contract: one static Go binary (§7, decision 2),
no cgo on macOS/Windows/Linux arm64+amd64 (success check 8), one pinned vendored module (decision 73),
no network package linked (success check 7), and only `init` may run another program (decision 22).
Measured and read, the three routes:

**(a) Model inside the binary, via a pure-Go ONNX runtime** — `hugot` (ONNX pipelines in Go, a
GoMLX-backed pure-Go backend, `tokenClassification` supported) or `onnx-gomlx` directly.
- The fp32 graph for bert-small is plain BERT ops (`MatMul`, `LayerNormalization`, `Erf`, `Softmax`,
  opset 17) — plausible for an ONNX importer. The **int8 graph is not**: it uses `DynamicQuantizeLinear`,
  `MatMulInteger` and `DequantizeLinear`, which pure-Go importers generally do not implement, so the
  realistic artifact is the 114 MB fp32 model, not the 29 MB one.
- hugot's own README: the Go backend is "designed for simpler workloads … smaller models such as
  all-MiniLM-L6-v2", "best with small batches of roughly 32 inputs", and "only built/tested on
  amd64-linux". Standin ships four platforms.
- Dependency cost: GoMLX is a framework, not a library — it contradicts decision 73's "one vendored
  module", though the *reason* for that decision ("nothing unvetted enters") could survive a
  deliberate exception.

**(b) Model inside the binary, pure-Go inference** — no cgo, no native library, static binary.
- **It works today.** `hugot v0.8.5` (its GoMLX backend) loaded the bert-small ONNX as a
  token-classification pipeline and reproduced the ONNX Runtime predictions exactly: 33/39 on the
  hard set, 90/93 on the repo fixtures (ORT int8: 33/39, 92/93 — a decode difference, not a model
  one). No cgo, `CGO_ENABLED=0`, statically linked **16.9 MB stripped** binary.
- Cost: **19 ms/line** median on this box, with **114 ms** when an input of a new token length
  compiles a graph, against 0.6 ms under ORT int8. **377 MB resident after load, 1.09 GB at
  plateau** under varied input (measured flat between call 20 and call 120 — a high-water mark, not
  a leak).
- Blocker for the small artifact: the int8 graph uses `DynamicQuantizeLinear` / `MatMulInteger`,
  which the ONNX importer does not take, so this route needs the **114 MB fp32** model, not the
  29 MB one. On load it prints a warning that dynamic axes failed and retries with concrete shapes;
  it works, but shapes are fixed per compile, which is where the 114 ms spikes come from.
- Dependency cost: **85 modules, 26 MB vendored**, versus one today (decision 73).
- An earlier estimate in this report said a hand-written forward pass would run at 0.3–0.6 s/line
  from a measured 4.6–8.7 GFLOPS scalar matmul. That was **pessimistic by about 20×**: a compiled
  graph over batched tensors is not a scalar matmul loop. The measurement above replaces it, and it
  is also why "write the forward pass by hand" is not worth its ~1.5k lines when hugot exists.

**(c) Second binary / sidecar with a native library** — ONNX Runtime via cgo, or `onnxer`-style
purego FFI with a per-platform `libonnxruntime` shipped alongside.
- Breaks "one program, no other software needed" and decision 22. Works best, ships worst.

**Sizes, for scale:** the stripped binary is 3.92 MB today; the word lists are 270 KB and cost ~15 ms
to load. Add 29 MB int8 (or 114 MB fp32) plus an engine, and `install.sh` stops being a curl of
something you can eyeball.

**Two non-size costs worth naming:**

- **Explainability.** `approve` asks about words with a *reason* — after "Hi", not an English word,
  next to a caught name. A model gives a score. The screen can say "a language model thinks this is a
  name", which is honest but is not the same kind of claim, and the "why" that makes a person feel
  safe approving 30 words at once gets weaker.
- **Determinism.** The mapping's promise (same value, same stand-in) is untouched. But the *candidate
  list* would change with a model version bump, silently, with no way to diff the decision. Standin's
  current behaviour is a set of rules a person could, in principle, verify by reading three lists.

---

## 8. Recommendation

**For v0.3: keep the binary deterministic. Spend the model budget offline.**

**Move 1 — use a model as a teacher, not as a runtime (days, no product risk).**
Put bert-small int8 on a workstation, run it over a corpus of invented-but-realistic shop text
(notes, emails, tickets, bank descriptions), and use it to *label* an evaluation corpus. That corpus
is the asset: Standin has never had one, every claim in the README is measured on hand-counted
fixtures, and this is the cheapest way to get numbers that survive a rules change. Then use the same
run as a **rule miner**: every disagreement is either a missing list entry, a missing rule, or a
model error, and sorting them by cause is a morning's work that beats any single model integration.

**Move 2 — fix what the model exposed, deterministically (its misses were mostly rules).**
The model's unique catches on the hard set were handles (`@grace_lee`), path segments
(`/people/ana-vantreeck`) and address local parts. **Done (decision 90):** a handle is now a pattern
that claims `@grace_lee` whole as `[USERNAME_1]`, restoring exactly — a new `USERNAME` category, no
new false flags, and the hard-set score moved 77% → 79%. Address local parts were already handled by
the email pattern, which the first draft of this evaluation had left out of the baseline.
**Not done, on purpose:** a path segment stays a miss. "The word after `/` that is not an English
word" would flag every filesystem path in a note (`/usr/local/bin`), and a path gives no signal that
it names a person; a name that repeats a known value is still swapped there by the known-values pass.
The genuinely model-shaped cases that remain are names that are also words (`Rose`, `Rich`, `Summer`,
`Daisy`, `Stone`) at a sentence start or a sign-off line. That is the one gap that may justify a model
later; it is also the one a *smaller* rule (sentence-start first names from the name list, when the
line has no verb) can partly close, at the cost of noise.

**Move 3 — timebox a pure-Go inference spike before any product decision (1–2 weeks, gated).**
Accept only if all of: bert-small forward pass < 50 ms/line on the slowest supported machine; cross-builds
for darwin/arm64, darwin/amd64, windows/amd64, linux/amd64 without cgo; binary < 40 MB; and union
recall reproduces on a corpus of at least 500 names. If it fails the latency gate, close the question
rather than half-shipping a sidecar.

**If a model ever does ship**, the shape is: an *additional candidate source* feeding `approve`, behind
the human gate, quantized, Apache-2.0 or MIT, English-first, and announced in `THIRD_PARTY.md` with the
weights' licence. Never a replacement for the column rules, and never a source of automatic
`keep` decisions.

**And do not ship any of the four models you sent**, for the four separate reasons in §4.

---

## 9. What would change this verdict

- A pure-Go path at ≤5 ms/line and ≤300 MB resident, on all four platforms. (Today's is 19 ms and
  1.09 GB; at that point Move 3 becomes a wiring job.) Or int8 support in a Go ONNX importer, which
  would cut the model from 114 MB to 29 MB.
- Evidence that Standin's real users' text is dominated by non-English or non-Latin scripts, where
  every model tested collapses and the deterministic refusals are the correct behaviour.
- A measured corpus showing the word-check misses are cheap to fix by rule (Move 2) — then the model
  buys almost nothing and the question closes for good.
- A model ≤5 MB that matches bert-small. (A distilled, task-specific model trained on Standin's own
  corpus would be the most defensible artifact of all: small, explainable by construction, and
  ownable. That is a follow-on to Move 1, not a substitute for it.)

---

## 10. Sources and reproduction

**Measured here** (harness in `docs/research/model-eval/`, corpora included; the Go word-report tool
must live inside the module to import `internal/words`):
`python run_model.py <model> corpus_<set>.json out.json`, `python run_onnx.py` for the int8 path,
`python merge_spans.py` then `python score2.py` for the table. Scalar-Go matmul benchmark:
`go run gobench.go`. Pure-Go inference measurement: `go-probe/` (a nested module, so the standin
build never sees hugot) — `go mod tidy && go run . -corpus ../corpus_hard.json -model <dir> -out go.json`.

**Model facts** verified at the Hugging Face API on 2026-10-31 (parameter counts, weight sizes,
licences, context, download counts).

**Published work**
- PII-TRACE: Zhang et al., *A Benchmark for Context-Aware PII Detection in Multi-Turn LLM
  Conversations*, arXiv 2609.22200 (Sept 2026) — benchmark, eleven baselines, PII-Tracer.
- OpenAI Privacy Filter model card and repo (Apr 2026); *OpenAI Privacy Filter: A Cross-Lingual,
  Cross-Domain PII Evaluation Across 32 Benchmarks*, arXiv 2608.02616 (Aug 2026).
- PIIBench: *A Unified Multi-Source Benchmark Corpus for PII Detection*, arXiv 2604.15776 (Apr 2026).
- Zaratiana et al., *GLiNER: Generalist Model for Named Entity Recognition*, NAACL 2024.
- Lafferty, McCallum & Pereira, *Conditional Random Fields*, ICML 2001 (the decoder lineage).
- Microsoft Presidio (2020–), the finder/applier/vault shape Standin already borrows.

**Comparative benchmarks read, not re-run**
- albertsikkema.com, *Benchmarking Open-Source PII Detection Across Domains* (Jun 2026) — five
  approaches, four datasets, English and Dutch, shared label set.
- Grepture, *Best Open Source Models for PII Redaction* (Mar 2026) — the practical roundup.
- eelisLF/pii-benchmark — Presidio vs Privacy Filter on context-rich cases.
