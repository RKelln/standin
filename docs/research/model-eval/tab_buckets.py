"""Classify teach.py's model-only bucket against TAB's gold annotations.

On invented shop prose the model-only bucket is 7-15 rows and a person reads them. On 4,660 lines
of ECHR prose it is 1,134 rows, which is past what a person reads in one sitting — so this joins
each row teach.py printed back to the annotations and the word check, and says what it is:

  rules miss    overlaps a maskable (DIRECT/QUASI) person mention that the word check did not
                catch at all: the rows worth reading, the ones the recall number counts
  already caught  overlaps a maskable person the word check did catch: the model found another
                piece of a person who was going to be masked anyway
  over-mask     overlaps a NO_MASK person (an office, a public figure): a policy difference
  other type    overlaps an annotation of another type (ORG, LOC, DATETIME ...): the model called
                an organisation, a place or a date a person
  unannotated   nothing overlaps: a model false positive, or an annotation gap

It reads the table `teach.py` prints, so the top-line counts stay teach.py's and not a second
implementation's:

  python teach.py corpus_tab.json det_tab.json int8_tab_m.json > teach_tab.txt
  python tab_buckets.py teach_tab.txt corpus_tab.json det_tab.json /tmp/tab/echr_dev.json /tmp/tab/echr_test.json
"""
import json
import pathlib
import re
import sys
from collections import Counter

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from tab_to_corpus import Key, pick_annotator  # same annotator choice and word matching as the adapter
from tab_overlap import ENGLISH  # noqa: E402


def main():
    teach_path, corpus_path, det_path, *tab_paths = sys.argv[1:]
    corpus = {r["id"]: r for r in json.load(open(corpus_path))}
    det = {r["id"]: r for r in json.load(open(det_path))}
    mentions = {}
    for m in json.load(open(corpus_path.replace(".json", "_mentions.json"))):
        mentions.setdefault(m["line_id"], []).append(m)

    # doc -> line -> [(start, end, entity_type, identifier_type)], one annotator per document
    ann = {}
    for p in tab_paths:
        for doc in json.load(open(p)):
            if doc["doc_id"] in ann:
                continue
            name = pick_annotator(doc)
            if not name:
                continue
            lines, start = [], 0
            for raw in doc["text"].split("\n"):
                lines.append((start, start + len(raw)))
                start += len(raw) + 1
            spans = []
            for m in doc["annotations"][name].get("entity_mentions", []):
                s, e = m["start_offset"], m["end_offset"]
                for i, (ls, le) in enumerate(lines):
                    if ls <= s <= le and e <= le:
                        spans.append((i, s - ls, e - ls, m["entity_type"], m.get("identifier_type")))
                        break
            ann[doc["doc_id"]] = spans

    def span_caught(cid, s, e):
        """Would the word check have put a stand-in inside this annotation span?"""
        text = corpus[cid]["line"]
        for c in det.get(cid, {}).get("candidates", []):
            for mt in re.finditer(re.escape(c["word"]), text):
                if s <= mt.start() < e:
                    return True
        return False

    rows = []
    for line in open(teach_path):
        if not line.startswith("| "):
            continue
        parts = [p.strip() for p in line.strip().strip("|").split("|")]
        if len(parts) < 4 or parts[0] == "id":
            continue
        cid, span = parts[0], parts[1].strip("`")
        if cid in corpus:
            rows.append((cid, span))

    kinds, examples = Counter(), {}
    for cid, span in rows:
        doc, lidx = cid.split(":L")
        lidx = int(lidx)
        text = corpus[cid]["line"]
        found = set()
        for mt in re.finditer(re.escape(span), text):
            for (li, s, e, typ, ident) in ann.get(doc, []):
                if li == lidx and s < mt.end() and mt.start() < e:
                    found.add((s, e, typ, ident))
        maskable = {(s, e) for (s, e, typ, ident) in found
                    if typ == "PERSON" and ident in ("DIRECT", "QUASI")}
        if maskable:
            caught = {p for p in maskable if span_caught(cid, p[0], p[1])}
            kind = "rules miss" if len(caught) < len(maskable) else "already caught"
        elif ("PERSON", "NO_MASK") in {(t, i) for (_, _, t, i) in found}:
            kind = "over-mask"
        elif found:
            kind = "other type"
        else:
            kind = "unannotated"
        kinds[kind] += 1
        examples.setdefault(kind, [])
        if len(examples[kind]) < 10:
            examples[kind].append((cid, span, text[:72]))

    total = sum(kinds.values())
    print(f"model-only rows printed by teach.py: {total}\n")
    for kind in ("rules miss", "already caught", "over-mask", "other type", "unannotated"):
        print(f"{kind:<15}{kinds[kind]:>6}  {kinds[kind]/total:>6.1%}")
    for kind in ("rules miss", "other type", "unannotated"):
        print(f"\n## {kind}")
        for cid, span, text in examples[kind]:
            print(f"  {cid:<16} {span[:30]:<30} {text}")


if __name__ == "__main__":
    main()
