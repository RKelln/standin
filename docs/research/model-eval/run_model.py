"""Run a HF PII model over a line corpus and emit predicted spans as JSON.

usage: python run_model.py MODEL_ID corpus.json out.json [--kind tc|gliner|custom]
"""
import json
import sys
import time

import torch
from transformers import AutoModelForTokenClassification, AutoTokenizer

torch.set_num_threads(8)


def merge(sub, words, offsets):
    out = []
    cur = None
    for w, (s, e) in zip(words, offsets):
        if w in ("[CLS]", "[SEP]", "[PAD]", "<s>", "</s>"):
            cur = None
            continue
        if cur and cur["end"] <= s:
            cur["text"] += w
            cur["end"] = e
        else:
            if cur:
                out.append(cur)
            cur = {"start": s, "end": e, "text": w}
    if cur:
        out.append(cur)
    return out


def main():
    model_id, corpus, outpath = sys.argv[1], sys.argv[2], sys.argv[3]
    tok = AutoTokenizer.from_pretrained(model_id, use_fast=True)
    print("fast tokenizer:", tok.is_fast, flush=True)
    model = AutoModelForTokenClassification.from_pretrained(model_id, dtype=torch.float32)
    model.eval()
    items = json.load(open(corpus))
    res = []
    t0 = time.time()
    for it in items:
        text = it["line"]
        enc = tok(text, return_tensors="pt", truncation=True, max_length=512,
                  return_offsets_mapping=True)
        with torch.no_grad():
            logits = model(**enc).logits[0]
        ids = logits.argmax(-1).tolist()
        offs = enc.pop("offset_mapping")[0].tolist()
        spans = []
        cur = None
        for cls, (s0, e0) in zip(ids, offs):
            label = model.config.id2label[cls]
            if s0 == e0:
                continue
            if label == "O":
                if cur:
                    spans.append(cur)
                    cur = None
                continue
            tag, _, typ = label.partition("-")
            if tag == "I" and cur and cur["label"] == typ:
                cur["end"] = e0
                cur["text"] = text[cur["start"]:e0]
                continue
            if cur:
                spans.append(cur)
            cur = {"label": typ, "start": s0, "end": e0, "text": text[s0:e0]}
            if tag in ("S", "E"):
                spans.append(cur)
                cur = None
        if cur:
            spans.append(cur)
        res.append({"id": it["id"], "line": text, "gold": it["gold"], "spans": spans})
    json.dump(res, open(outpath, "w"), indent=1)
    dt = time.time() - t0
    print(f"{model_id}: {len(items)} lines in {dt:.1f}s ({dt/len(items)*1000:.0f} ms/line) -> {outpath}")


if __name__ == "__main__":
    main()
