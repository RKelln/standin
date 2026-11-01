"""Merge adjacent same-type spans in run_model.py output.

Tokens split a name into pieces (" Far" + "ida", "Ron" + "ces" + "valle" + "s"). An unmerged span
neither counts as a hit nor reads as one, so merge before scoring. Writes <name>_m.json.

usage: python merge_spans.py out1.json out2.json ...
"""
import json
import sys

for path in sys.argv[1:]:
    rows = json.load(open(path))
    merged_rows = []
    for r in rows:
        spans = sorted(r["spans"], key=lambda s: (s["start"], s["end"]))
        out = []
        for s in spans:
            if out and s["label"] == out[-1]["label"] and out[-1]["start"] <= s["start"] <= out[-1]["end"]:
                out[-1]["end"] = max(out[-1]["end"], s["end"])
                out[-1]["text"] = r["line"][out[-1]["start"]:out[-1]["end"]]
            else:
                out.append(dict(s))
        r = dict(r)
        r["spans"] = out
        merged_rows.append(r)
    json.dump(merged_rows, open(path.replace(".json", "_m.json"), "w"), indent=1)
    print(f"{path} -> {path.replace('.json', '_m.json')}")
