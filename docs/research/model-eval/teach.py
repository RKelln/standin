"""Teacher pass: diff the shipped word check against a model, for adjudication.

The model is an annotation assistant, not a truth source. This prints the
three buckets a human reviews, smallest first:

  model-only  could be a rules miss: read these (this is the only one that
              needs human eyes)
  det-only    the word check flags something the model does not: noise to
              tune, or a class the model does not know
  agreed      both flag it: no review needed

usage:
  python teach.py corpus_hard.json det_hard.json int8_hard_m.json
"""
import json
import sys
from collections import Counter

corpus_path, det_path, model_path = sys.argv[1], sys.argv[2], sys.argv[3]
corpus = {r["id"]: r for r in json.load(open(corpus_path))}
det = {r["id"]: r for r in json.load(open(det_path))}
model = {r["id"]: r for r in json.load(open(model_path))}


def det_words(row):
    """Word-check candidates plus pattern hits: scrub masks a prose email and a
    handle through the pattern pass, so the baseline must include them or the
    model looks better than it is."""
    out = {c["word"] for c in row.get("candidates", [])}
    out |= {p["text"].strip() for p in row.get("patterns", [])}
    return out


def model_spans(row):
    """Only the label that means a person or an organisation: the rest is the
    pattern pass's job and would drown the review."""
    return [s for s in row["spans"] if s["label"].upper() in ("PERSON", "PRIVATE_PERSON", "ORG", "ORGANIZATION")]


def overlaps(word, spans, line):
    low = line.lower()
    start = 0
    w = word.lower()
    while True:
        i = low.find(w, start)
        if i < 0:
            return False
        for s in spans:
            if s["start"] < i + len(w) and i < s["end"]:
                return True
        start = i + 1


model_only, det_only, agreed = [], [], []
for cid, c in corpus.items():
    words = det_words(det.get(cid, {"candidates": []}))
    spans = model_spans(model.get(cid, {"spans": []}))
    for w in words:
        if overlaps(w, spans, c["line"]):
            agreed.append((cid, w, c["line"]))
        else:
            det_only.append((cid, w, c["line"]))
    for s in spans:
        t = s["text"].strip()
        if t and not any(t.lower() in w.lower() or w.lower() in t.lower() for w in words):
            model_only.append((cid, t, s["label"], c["line"]))

print(f"# teacher pass: {corpus_path}")
print(f"\n{len(model_only)} model-only, {len(det_only)} word-check-only, {len(agreed)} agreed\n")

print("## model-only (review these: a rules miss, a list miss, or a model error)\n")
print("| id | span | label | line | verdict |")
print("|---|---|---|---|---|")
for cid, t, lab, line in model_only:
    print(f"| {cid} | `{t}` | {lab} | {line[:70].replace('|', '/')} | |")

print("\n## word-check-only (noise to tune, or a class the model does not know)\n")
counts = Counter(w for _, w, _ in det_only)
for w, n in counts.most_common(30):
    ex = next(line for _, x, line in det_only if x == w)
    print(f"- `{w}` x{n} — {ex[:70]}")
print()
