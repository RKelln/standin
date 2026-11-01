# Model evaluation harness (research)

Reproduces the numbers in `../model-based-detection-eval.md`. Nothing here is part of the shipped
CLI; `tools/wordreport` is a research binary that imports `internal/words` and `internal/scan`, which
is why it lives inside the module.

## Corpora

| File | What it is |
|---|---|
| `corpus_hard.json` | 52 lines written to probe documented miss classes, plus 18 control lines with no name. `gold` lists the names that must become stand-ins. |
| `corpus_repo.json` | Prose lines from `testdata/inbox.md` and `testdata/notes.md`; gold = `testdata/text-names.txt`. |
| `corpus_repo_blocks.json` | The same prose in 4-line blocks, to give a model context. |

Every name in them is invented. Do not add real data.

## Baseline (the shipped word check + patterns)

```sh
go build -o /tmp/wordreport ./tools/wordreport
/tmp/wordreport docs/research/model-eval/corpus_hard.json > det_hard.json
```

The output holds `candidates` (word-check hits) and `patterns` (email, phone, account-like spans).
Both count: scrub masks a prose email as `[EMAIL_n]` through the pattern path, so a baseline that
ignores patterns overstates the model's advantage. (It did, in the first draft of this evaluation.)

## Models

Python 3.12, `uv venv`, `torch` (CPU), `transformers>=5.2`, `onnxruntime`.

```sh
python run_model.py  iiiorg/piiranha-v1-detect-personal-information corpus_hard.json piiranha_hard.json
python run_model.py  gravitee-io/bert-small-pii-detection           corpus_hard.json int8src_hard.json
python run_model.py  OpenMed/OpenMed-PII-SuperClinical-Small-44M-v1 corpus_hard.json openmed_hard.json
python run_model.py  openai/privacy-filter                          corpus_hard.json opf_hard.json
python run_onnx.py                                  corpus_hard.json int8_hard.json
python run_piitracer.py                             corpus_hard.json piitracer_hard.json
```

Then merge adjacent spans of the same type before scoring (`python merge_spans.py *.json`): several
tokenizers split a name into pieces, and an unmerged span neither counts as a hit nor reads as one.
`score2.py` expects the merged files (`*_m.json`). `int8` (the 28.7 MB quantized ONNX) is the artifact
that matters for shipping; the others are for comparison.

## Pure-Go inference

`go-probe/` is a separate module (so `go build ./...` in the repo never sees hugot) that runs the
same corpus through `hugot`'s GoMLX backend and writes the same JSON, for the latency and memory
numbers in the report:

```sh
cd go-probe && go mod tidy
go run . -corpus ../corpus_hard.json -model /path/to/bert-small-dir -out go.json
```

The model directory needs `model.onnx` (fp32, 114 MB), `tokenizer.json` and `config.json`.

## Corpus overlap

`tab_overlap.py` measures, on the Text Anonymisation Benchmark (MIT, real ECHR judgments), how many
annotated person mentions the word check cannot see at all — every token an English word and no token
in the first-name list. Clone TAB next to it and pass the dev/test JSON:

```sh
git clone --depth 1 https://github.com/NorskRegnesentral/text-anonymization-benchmark /tmp/tab
python tab_overlap.py /tmp/tab/echr_dev.json /tmp/tab/echr_test.json
```

## Teacher pass

`teach.py` diffs the shipped check against a model and prints the three buckets a person reviews
(model-only needs eyes, word-check-only is tuning information, agreed needs nothing):

```sh
python teach.py corpus_hard.json det_hard.json int8_hard_m.json
```

It needs the wordreport output (candidates **and** patterns) and a merged model output. See
`../teacher-pass-plan.md` for what to do with the buckets.

## Score

```sh
python score2.py
```

It prints recall per corpus, distinct extra flags, and the union of the shipped check with bert-small.
Rename or delete the `MODELS` entries whose prediction files are absent.
