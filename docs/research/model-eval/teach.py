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
import pathlib
import sys
from collections import Counter

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from tab_overlap import NOT_A_NAME  # noqa: E402  (titles are not names)

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


def name_tokens(text):
    """The tokens of a span that could be a name: titles, ranks and initials are
    not. 'Mr G. Blades' has one name token, 'Blades'."""
    out = []
    for t in text.replace("-", " ").split():
        w = t.strip(".,;:()[]'\u2019\u2018\u201c\u201d-").lower()
        if len(w) > 1 and w not in NOT_A_NAME:
            out.append(w)
    return out


def covered(word, spans):
    """Was this candidate caught by the model? Only a NAME token counts. A span
    'Mr G. Blades' contains the word-check candidate 'Mr', but masking 'Mr'
    does not mask the name, so a title must never read as agreement."""
    w = word.lower()
    if w in NOT_A_NAME:
        return False
    return any(w in name_tokens(s["text"]) for s in spans)


model_only, det_only, agreed = [], [], []
for cid, c in corpus.items():
    words = det_words(det.get(cid, {"candidates": []}))
    spans = model_spans(model.get(cid, {"spans": []}))
    caught = set()
    for w in words:
        if covered(w, spans):
            agreed.append((cid, w, c["line"]))
            caught.add(w.lower())
        else:
            det_only.append((cid, w, c["line"]))
    for s in spans:
        toks = name_tokens(s["text"])
        if toks and not any(t in caught for t in toks):
            model_only.append((cid, s["text"].strip(), s["label"], c["line"]))

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
