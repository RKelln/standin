"""Score Standin's word check against TAB's gold person mentions.

The harness scores corpora as lines of `gold` strings; a real precision/recall number needs
to know which gold string belongs to which mention and which mask decision the annotator made,
so this reads the sidecar `tab_to_corpus.py` writes alongside the corpus:

  corpus_tab.json          the harness corpus (id / line / gold / class)
  corpus_tab_mentions.json one record per kept mention (offsets, tokens, identifier_type)
  det_tab.json             tools/wordreport output (candidates + patterns)
  int8_tab_m.json          optional, merged model output

A mention counts as caught when at least one of its tokens is a word-check candidate on that
line, or its own character span overlaps a pattern hit. `caught_all` (every token caught) is
reported too: masking one word of "Cengiz Polat" is a partial leak, not a hit.

`NO_MASK` mentions are not scored as misses. The annotators decided those need no masking
(public figures, offices); Standin's default-deny would mask them anyway, which is a policy
difference. They are reported as over-masking instead.

A candidate is a false flag when its word is not one of the line's gold tokens; occurrences are
counted (not distinct words), because the rate is per 1,000 words of prose.

usage:
  python tab_score.py corpus_tab.json det_tab.json int8_tab_m.json
"""
import json
import pathlib
import sys
from collections import Counter

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from tab_overlap import ENGLISH  # noqa: E402  (same lists the overlap check used)
from tab_to_corpus import Key  # noqa: E402  (internal/words.Key, for candidate matching)

SCORED = ("DIRECT", "QUASI")  # the annotations that say "this should be masked"


def load(path):
    return json.load(open(path))


def cand_keys(row):
    return {c["key"] for c in row.get("candidates", [])} if row else set()


def token_caught(token, keys):
    """A token is caught by an equal candidate, or by a hyphenated one it is part of.

    TAB annotates `Mrs Tsatsa` where the sentence says `Tsatsa-Nikolovska`; the compound
    candidate masks the abbreviation, so the mention is caught.
    """
    k = Key(token)
    return k in keys or any(k in c.split("-") for c in keys)


def pattern_overlap(row, start, end):
    if not row:
        return False
    return any(p["start"] < end and start < p["end"] for p in row.get("patterns", []))


def model_overlap(row, start, end):
    if not row:
        return False
    for s in row.get("spans", []):
        if s["label"].upper() not in ("PERSON", "PRIVATE_PERSON"):
            continue
        if s["start"] < end and start < s["end"]:
            return True
    return False


def main():
    corpus_path, det_path = sys.argv[1], sys.argv[2]
    model_path = sys.argv[3] if len(sys.argv) > 3 else None
    meta_path = corpus_path.replace(".json", "_mentions.json")

    corpus = {r["id"]: r for r in load(corpus_path)}
    det = {r["id"]: r for r in load(det_path)}
    mentions = load(meta_path)
    model = {r["id"]: r for r in load(model_path)} if model_path else {}

    total_words = sum(len(r["line"].split()) for r in corpus.values())

    def caught(m, mode="det"):
        """mode: 'det' (word check + patterns), 'model', or 'union'."""
        row = det.get(m["line_id"])
        keys = cand_keys(row)
        hit = [token_caught(t, keys) for t in m["tokens"]]
        if not any(hit):
            hit = [pattern_overlap(row, m["start"], m["end"])] + hit[1:]
        if mode in ("model", "union"):
            mhit = model_overlap(model.get(m["line_id"]), m["start"], m["end"])
            hit = ([mhit] * len(hit)) if mode == "model" else [h or mhit for h in hit]
        return any(hit), all(hit)

    def summarise(name, subset, mode="det"):
        n = len(subset)
        if not n:
            return None
        any_hits = sum(caught(m, mode)[0] for m in subset)
        all_hits = sum(caught(m, mode)[1] for m in subset)
        return name, n, any_hits, all_hits, any_hits / n, all_hits / n

    print(f"corpus: {len(corpus)} lines, {total_words} words, {len(mentions)} gold mentions\n")

    print("## mention recall (any token of the mention caught)")
    print(f"{'subset':<44}{'n':>6}{'caught':>8}{'recall':>9}{'full':>8}")
    for label, sel in [
        ("DIRECT+QUASI, all", lambda m: m["class"] in SCORED),
        ("  DIRECT", lambda m: m["class"] == "DIRECT"),
        ("  QUASI", lambda m: m["class"] == "QUASI"),
        ("  all tokens English words", lambda m: m["class"] in SCORED and all(Key(t) in ENGLISH for t in m["tokens"])),
        ("  some token not an English word", lambda m: m["class"] in SCORED and not all(Key(t) in ENGLISH for t in m["tokens"])),
        ("  single-token mentions", lambda m: m["class"] in SCORED and len(m["tokens"]) == 1),
    ]:
        sub = [m for m in mentions if sel(m)]
        r = summarise(label, sub)
        if r:
            print(f"{r[0]:<44}{r[1]:>6}{r[2]:>8}{r[4]:>8.1%}{r[5]:>8.1%}")

    nomask = [m for m in mentions if m["class"] == "NO_MASK"]
    if nomask:
        flag = sum(caught(m)[0] for m in nomask)
        print(f"\n## NO_MASK (reported separately: over-masking, not a miss)")
        print(f"{len(nomask)} mentions, {flag} would be masked ({flag/len(nomask):.1%})")
        print("  examples:", sorted({m['text'] for m in nomask if caught(m)[0]})[:15])

    # False flags: candidate occurrences whose word is not a gold token on that line.
    extra = 0
    extra_by_group = Counter()
    extra_words = Counter()
    for cid, row in det.items():
        gold = Counter(Key(g) for g in corpus[cid]["gold"])
        for c in row.get("candidates", []):
            spare = c["count"] - gold.get(c["key"], 0)
            if spare > 0:
                extra += spare
                extra_by_group[c["group"]] += spare
                extra_words[c["word"]] += spare
    print(f"\n## false flags (candidate occurrences that are not a gold name)")
    print(f"{extra} flags over {total_words} words = {extra/total_words*1000:.1f} per 1,000 words")
    print(f"  by group: {dict(extra_by_group)}")
    cal = extra - extra_by_group.get("calendar", 0)
    print(f"  excluding calendar words (a group the config may keep): {cal} = {cal/total_words*1000:.1f} per 1,000")
    print(f"  most frequent: {extra_words.most_common(20)}")

    pat = sum(len(r.get("patterns", [])) for r in det.values())
    print(f"\npattern hits (email/phone/account-like): {pat}")

    if model:
        print("\n## bert-small int8 (28.7 MB, Apache-2.0) over the same lines: any PERSON span")
        print(f"{'subset':<44}{'n':>6}{'caught':>8}{'recall':>9}{'full':>8}")
        for label, sel in [
            ("model only, DIRECT+QUASI", lambda m: m["class"] in SCORED),
            ("  model only, all tokens English", lambda m: m["class"] in SCORED and all(Key(t) in ENGLISH for t in m["tokens"])),
            ("union(word check, model), DIRECT+QUASI", lambda m: m["class"] in SCORED),
            ("  union, all tokens English", lambda m: m["class"] in SCORED and all(Key(t) in ENGLISH for t in m["tokens"])),
        ]:
            mode = "union" if label.startswith(("union", "  union")) else "model"
            r = summarise(label, [m for m in mentions if sel(m)], mode)
            if r:
                print(f"{r[0]:<44}{r[1]:>6}{r[2]:>8}{r[4]:>8.1%}{r[5]:>8.1%}")


if __name__ == "__main__":
    main()
