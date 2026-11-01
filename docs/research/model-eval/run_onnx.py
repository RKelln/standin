"""Run gravitee bert-small int8 ONNX (29 MB) over a corpus; measure speed + spans."""
import json
import sys
import time

import numpy as np
import onnxruntime as ort
from huggingface_hub import hf_hub_download
from transformers import AutoTokenizer

corpus, outpath = sys.argv[1], sys.argv[2]
repo = "gravitee-io/bert-small-pii-detection"
tok = AutoTokenizer.from_pretrained(repo)
path = hf_hub_download(repo, "model.quant.onnx")
so = ort.SessionOptions()
so.intra_op_num_threads = 8
sess = ort.InferenceSession(path, so, providers=["CPUExecutionProvider"])
cfg = json.load(open(hf_hub_download(repo, "config.json")))
id2label = cfg["id2label"]

items = json.load(open(corpus))
res = []
t0 = time.time()
for it in items:
    text = it["line"]
    enc = tok(text, return_offsets_mapping=True, truncation=True, max_length=512)
    ids = np.array([enc["input_ids"]], dtype=np.int64)
    mask = np.array([enc["attention_mask"]], dtype=np.int64)
    logits = sess.run(None, {"input_ids": ids, "attention_mask": mask})[0][0]
    labels = logits.argmax(-1)
    spans, cur = [], None
    for cls, (s0, e0) in zip(labels.tolist(), enc["offset_mapping"]):
        if s0 == e0:
            continue
        lab = id2label[str(cls)]
        if lab == "O":
            if cur:
                spans.append(cur); cur = None
            continue
        tag, _, typ = lab.partition("-")
        if tag == "I" and cur and cur["label"] == typ:
            cur["end"] = e0; cur["text"] = text[cur["start"]:e0]; continue
        if cur:
            spans.append(cur)
        cur = {"label": typ, "start": s0, "end": e0, "text": text[s0:e0]}
        if tag in ("S", "E"):
            spans.append(cur); cur = None
    if cur:
        spans.append(cur)
    res.append({"id": it["id"], "line": text, "gold": it["gold"], "spans": spans})
json.dump(res, open(outpath, "w"), indent=1)
dt = time.time() - t0
print(f"onnx-int8: {len(items)} lines in {dt:.1f}s ({dt/len(items)*1000:.1f} ms/line) -> {outpath}")
