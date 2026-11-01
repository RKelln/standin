"""Score with patterns included in the deterministic baseline (matches the real text pass)."""
import json
from collections import Counter

import os
B = os.path.dirname(os.path.abspath(__file__)) + "/"
MODELS = {
    "piiranha 278M (CC-BY-NC-ND)": "iiiorg_piiranha-v1-detect-personal-information",
    "bert-small-pii 28M int8 (Apache-2.0)": "int8",
    "OpenMed PII 44M (Apache-2.0)": "OpenMed_OpenMed-PII-SuperClinical-Small-44M-v1",
    "PII-Tracer 600M (MIT)": "piitracer",
    "OpenAI Privacy Filter 1.4B (Apache-2.0)": "opf",
}


def load(f):
    return {r["id"]: r for r in json.load(open(B + f + ".json"))}


def span_has(name, spans):
    n = name.lower()
    return any(n in s["text"].lower() for s in spans)


def det_catches(word, row):
    """The real text pass: word candidates OR pattern spans (word inside a pattern)."""
    if word.lower() in {c["word"].lower() for c in row["candidates"]}:
        return True
    low = row["line"].lower()
    for p in row["patterns"]:
        if p["text"].lower().find(word.lower()) >= 0:
            return True
    return False


rows = []
for c in ["hard", "repo", "repo_blocks"]:
    corp = load("corpus_" + c)
    det = load("det_" + c)
    n_gold = sum(len(r["gold"]) for r in corp.values())
    rec = sum(det_catches(n, det[k]) for k, r in corp.items() for n in r["gold"])
    extras = set()
    for k, r in corp.items():
        gold = {n.lower() for n in r["gold"]}
        for x in det[k]["candidates"]:
            if x["word"].lower() not in gold:
                extras.add(x["word"].lower())
        for p in det[k]["patterns"]:
            if not any(g in p["text"].lower() for g in gold):
                extras.add(p["text"].lower())
    rows.append((c, "deterministic (shipped)", f"{rec}/{n_gold}", rec / n_gold, len(extras)))
    for label, tag in MODELS.items():
        pred = load(f"{tag}_{c}_m")
        rec = sum(span_has(n, pred[k]["spans"]) for k, r in corp.items() for n in r["gold"])
        ex = set()
        for k, r in corp.items():
            gold = {n.lower() for n in r["gold"]}
            for s in pred[k]["spans"]:
                t = s["text"].strip().lower()
                if not any(g in t for g in gold):
                    ex.add(t)
        rows.append((c, label, f"{rec}/{n_gold}", rec / n_gold, len(ex)))

print(f"{'corpus':<13}{'system':<40}{'recall':>8}{'':>4}{'extra':>7}")
for c, label, frac, pct, ex in rows:
    print(f"{c:<13}{label:<40}{frac:>8}{pct:>5.0%}{ex:>7}")
print()
# union of shipped + bert-small, and residual misses
for c in ["hard", "repo", "repo_blocks"]:
    corp = load("corpus_" + c)
    det = load("det_" + c)
    m = load("int8_%s_m" % c)
    hits = tot = 0
    misses = []
    for k, r in corp.items():
        for nm in r["gold"]:
            tot += 1
            if det_catches(nm, det[k]) or span_has(nm, m[k]["spans"]):
                hits += 1
            else:
                misses.append((nm, r["line"][:60]))
    print(f"union(det, bert-small) on {c}: {hits}/{tot} = {hits/tot:.0%}; still missed: {misses}")
