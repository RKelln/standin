"""How much of a corpus of person mentions can Standin's word rules not see?

Reads the Text Anonymisation Benchmark (TAB, ECHR judgments) and asks, per
annotated PERSON mention: is every token an English dictionary word (so the
"not an English word" rule is silent), and is no token in the first-name list
(so the first-name rule is silent)? Mentions where both hold are invisible to
the word check; only a name spot or a known value can catch them.

usage:
  git clone --depth 1 https://github.com/NorskRegnesentral/text-anonymization-benchmark
  python tab_overlap.py text-anonymization-benchmark/echr_dev.json text-anonymization-benchmark/echr_test.json
"""
import gzip
import json
import pathlib
import re
import sys

# The project's own lists, so the answer is about Standin and not a proxy.
LISTS = pathlib.Path(__file__).resolve().parents[3] / "internal" / "words" / "lists"


def load(name):
    return {line.strip().lower() for line in gzip.open(LISTS / name, "rt", errors="ignore")}


ENGLISH = load("english.txt.gz")
FIRST = load("firstnames.txt.gz")

# Titles and ranks are not names; ECHR prose is full of them.
NOT_A_NAME = {
    "mr", "mrs", "ms", "mx", "miss", "dr", "sir", "lord", "lady", "prof", "professor",
    "judge", "captain", "major", "colonel", "general", "commander", "chief", "inspector",
    "detective", "officer", "sergeant", "corporal", "brigadier", "admiral", "chancellor",
    "dean", "doctor", "mme", "mlle", "the", "advocate", "president", "justice", "registrar",
    "master", "naval",
}


def words(text):
    out = [re.sub(r"[^\w'’-]", "", w).strip("'’-") for w in text.split()]
    out = [w for w in out if len(w) > 1]  # drop initials
    while out and out[0].lower() in NOT_A_NAME:
        out = out[1:]
    return out


def main():
    mentions = []
    for path in sys.argv[1:]:
        for doc in json.load(open(path)):
            for ann in doc["annotations"].values():
                for m in ann.get("entity_mentions", []):
                    if m.get("entity_type") != "PERSON":
                        continue
                    ws = words(doc["text"][m["start_offset"]:m["end_offset"]])
                    if ws:
                        mentions.append((ws, m.get("identifier_type")))

    n = len(mentions)
    all_dict = [w for w, _ in mentions if all(x.lower() in ENGLISH for x in w)]
    no_first = [w for w, _ in mentions if not any(x.lower() in FIRST for x in w)]
    both = [w for w, _ in mentions
            if not any(x.lower() in FIRST for x in w) and all(x.lower() in ENGLISH for x in w)]

    print(f"person mentions: {n}")
    print(f"  every token an English word (dictionary rule silent): {len(all_dict)} = {len(all_dict)/n:.1%}")
    print(f"  no token in the first-name list (first-name rule silent): {len(no_first)} = {len(no_first)/n:.1%}")
    print(f"  BOTH -> invisible to the word check: {len(both)} = {len(both)/n:.1%}")
    print("  examples:", [" ".join(w) for w in both[:20]])
    kinds = {}
    for _, t in mentions:
        kinds[t] = kinds.get(t, 0) + 1
    print("  identifier types:", kinds)


if __name__ == "__main__":
    main()
