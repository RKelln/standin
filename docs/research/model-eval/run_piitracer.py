"""Run perplexity-ai/PII-Tracer over a line corpus (trust_remote_code model)."""
import json
import sys
import time

import torch
from transformers import AutoModel, AutoTokenizer

torch.set_num_threads(8)

corpus, outpath = sys.argv[1], sys.argv[2]
tok = AutoTokenizer.from_pretrained("perplexity-ai/PII-Tracer", trust_remote_code=True)
model = AutoModel.from_pretrained("perplexity-ai/PII-Tracer", trust_remote_code=True,
                                  dtype=torch.float32)
model.eval()
items = json.load(open(corpus))
res = []
t0 = time.time()
for it in items:
    spans, sens = model.predict(it["line"])
    res.append({
        "id": it["id"], "line": it["line"], "gold": it["gold"],
        "spans": [{"label": s.label, "start": s.start, "end": s.end,
                   "text": it["line"][s.start:s.end]} for s in spans],
        "sensitivity": float(sens),
    })
json.dump(res, open(outpath, "w"), indent=1)
dt = time.time() - t0
print(f"PII-Tracer: {len(items)} lines in {dt:.1f}s ({dt/len(items)*1000:.0f} ms/line) -> {outpath}")
