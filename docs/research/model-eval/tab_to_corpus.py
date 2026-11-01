"""Convert the Text Anonymisation Benchmark (TAB) into the corpus JSON the harness consumes.

TAB (`NorskRegnesentral/text-anonymization-benchmark`, MIT) is 1,268 ECHR judgments with
PERSON mentions annotated by character offset and a mask decision (`DIRECT`, `QUASI`,
`NO_MASK`). The rest of `docs/research/model-eval/` reads a line-granular corpus:

    [{"id": "001-61807:L12", "line": "...", "gold": ["Cengiz", "Polat"], "class": "DIRECT"}]

so this adapter does three things:

1. **Picks one annotator per document.** TAB ships several annotators per document. Merging
   them silently would count the same person once per annotator. `quality_checked` names the
   annotators whose work was revised; we take the numerically first of those (all 254 dev+test
   documents have at least one), and fall back to `annotator1`. The pick is recorded per line in
   the meta sidecar so a score can be recomputed with a different rule.

2. **Re-bases character offsets onto lines.** The text is split on "\\n" with a running offset.
   A mention is kept on the line where it starts, but only if it ends on that line too.
   **Mentions that straddle a line break are dropped and counted** (`--report` prints the count;
   it is a limit of the line-granular harness, not a detector miss).

3. **Strips titles and initials**, reusing `tab_overlap.words()`: `Mr E. Kanar` -> `["Kanar"]`,
   `Mr Cengiz Polat` -> `["Cengiz", "Polat"]`. Gold is a list of name tokens, because the word
   check reports words, not spans. A mention that leaves no token (an initial, `the applicant`)
   is dropped and counted separately.

Every non-blank line is emitted, not only lines with a mention: the false-flag rate needs the
unannotated prose as its denominator.

`gold` follows the mask decision so a scorer can hold `NO_MASK` out of recall. `class` is the
identifier type, or the sorted union (`DIRECT+QUASI`) when one line carries both.

The corpus contains real document text, so it is written outside the repo (default
`/tmp/tab-eval/`) and never committed. Only this adapter and the numbers are.

usage:
  git clone --depth 1 https://github.com/NorskRegnesentral/text-anonymization-benchmark /tmp/tab
  python tab_to_corpus.py /tmp/tab/echr_dev.json /tmp/tab/echr_test.json
  python tab_to_corpus.py /tmp/tab/echr_dev.json --out /tmp/tab-eval/corpus_tab.json
"""
import argparse
import json
import pathlib
import re
import sys

# Reuse the reader that already knows this corpus, rather than a second copy of its rules.
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from tab_overlap import NOT_A_NAME  # noqa: E402  (title/rank stripping)


def words(text):
    """Name tokens in a mention.

    Letter runs, like the Go tokenizer's `\\pL[\\pL\\pM]*(?:['’-]\\pL...)*`, then the title and
    initial rules from tab_overlap: `Mr E. Kanar` -> ["Kanar"]. A whitespace split would glue
    `Mr<U+F020>L.<U+F020>Wildhaber` into one token that no candidate can match.
    """
    out = re.findall(r"[^\W\d_]+(?:['’-][^\W\d_]+)*", text, re.UNICODE)
    out = [w for w in out if len(w) > 1]  # drop initials
    while out and out[0].lower() in NOT_A_NAME:
        out = out[1:]
    return out


def Key(w):
    """internal/words.Key: lower case, straight apostrophe, 's taken off.

    Go's strings.ToLower maps İ (U+0130) to plain i; Python's lower() yields i + combining dot,
    which would never match a candidate. Map it first so the two agree.
    """
    s = w.replace("\u2019", "'").translate({0x130: 0x69})
    return s.lower().removesuffix("'s")


def pick_annotator(doc, rule="first-qc"):
    """One annotator per document.

    first-qc  the numerically first annotator named in quality_checked (the default)
    last-qc   the numerically last, as a sensitivity check on the choice
    annotator1  always annotator1, where the document has one
    """
    names = doc.get("annotations") or {}
    if not names:
        return None

    def num(name):
        m = re.search(r"(\d+)$", name)
        return (int(m.group(1)) if m else 999, name)

    checked = [n for n in doc.get("quality_checked") or [] if n in names]
    if rule == "annotator1" and "annotator1" in names:
        return "annotator1"
    if checked:
        return sorted(checked, key=num)[0 if rule != "last-qc" else -1]
    if "annotator1" in names:
        return "annotator1"
    return sorted(names, key=num)[0]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("files", nargs="+", help="TAB split files (echr_dev.json ...)")
    ap.add_argument("--out", default="/tmp/tab-eval/corpus_tab.json")
    ap.add_argument("--meta", default=None, help="per-mention sidecar for span scoring (default <out>_mentions.json)")
    ap.add_argument("--annotator", default="first-qc", choices=["first-qc", "last-qc", "annotator1"],
                    help="which annotator per document (default the numerically first quality-checked one)")
    args = ap.parse_args()
    meta_path = args.meta or args.out.replace(".json", "_mentions.json")

    rows, mentions = [], []
    stats = {
        "docs": 0,
        "mentions_seen": 0,
        "mentions_duplicate": 0,
        "mentions_straddling": 0,
        "mentions_no_tokens": 0,
        "mentions_kept": 0,
        "offset_mismatch": 0,
        "empty_gold_lines": 0,
    }
    words_total = 0

    for path in args.files:
        for doc in json.load(open(path)):
            stats["docs"] += 1
            ann_name = pick_annotator(doc, args.annotator)
            if ann_name is None:
                continue
            text = doc["text"]

            # Line table: (start, end) in document coordinates, end exclusive of the newline.
            lines, start = [], 0
            for raw in text.split("\n"):
                lines.append((start, start + len(raw)))
                start += len(raw) + 1
            by_line = {i: [] for i in range(len(lines))}

            seen = set()
            for m in doc["annotations"][ann_name].get("entity_mentions", []):
                if m.get("entity_type") != "PERSON":
                    continue
                stats["mentions_seen"] += 1
                key = (m.get("entity_id"), m["start_offset"], m["end_offset"])
                if key in seen:  # a name annotated twice in one document
                    stats["mentions_duplicate"] += 1
                    continue
                seen.add(key)

                s, e = m["start_offset"], m["end_offset"]
                span_text = text[s:e]
                if "span_text" in m and m["span_text"] != span_text:
                    stats["offset_mismatch"] += 1

                idx = None
                for i, (ls, le) in enumerate(lines):
                    if ls <= s <= le:
                        idx = i
                        break
                if idx is None or e > lines[idx][1]:
                    stats["mentions_straddling"] += 1
                    continue

                toks = words(span_text)
                if not toks:
                    stats["mentions_no_tokens"] += 1
                    continue

                stats["mentions_kept"] += 1
                ls = lines[idx][0]
                rec = {
                    "line_id": f"{doc['doc_id']}:L{idx}",
                    "doc": doc["doc_id"],
                    "annotator": ann_name,
                    "entity_id": m.get("entity_id"),
                    "start": s - ls,
                    "end": e - ls,
                    "text": span_text,
                    "tokens": toks,
                    "class": m.get("identifier_type"),
                }
                mentions.append(rec)
                by_line[idx].append(rec)

            for i, (ls, le) in enumerate(lines):
                line = text[ls:le]
                words_total += len(line.split())
                if not line.strip():
                    continue
                gold = [t for r in by_line[i] for t in r["tokens"]]
                classes = sorted({r["class"] for r in by_line[i] if r["class"]})
                if not gold:
                    stats["empty_gold_lines"] += 1
                rows.append({
                    "id": f"{doc['doc_id']}:L{i}",
                    "line": line,
                    "gold": gold,
                    "class": "+".join(classes) if classes else "none",
                })

    out = pathlib.Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    json.dump(rows, open(out, "w"))
    json.dump(mentions, open(meta_path, "w"))

    kinds = {}
    for r in mentions:
        kinds[r["class"]] = kinds.get(r["class"], 0) + 1
    print(f"documents: {stats['docs']}")
    print(f"lines emitted: {len(rows)} ({stats['empty_gold_lines']} with no gold mention), words: {words_total}")
    print(f"person mentions seen (one annotator per doc): {stats['mentions_seen']}")
    print(f"  duplicate (same entity_id+span): {stats['mentions_duplicate']}")
    print(f"  dropped, straddles a line break: {stats['mentions_straddling']}")
    print(f"  dropped, no name token after title/initial stripping: {stats['mentions_no_tokens']}")
    print(f"  kept: {stats['mentions_kept']}  {kinds}")
    if stats["offset_mismatch"]:
        print(f"  WARNING span_text disagreed with the offsets on {stats['offset_mismatch']} mentions")
    print(f"-> {out}\n-> {meta_path}")


if __name__ == "__main__":
    main()
