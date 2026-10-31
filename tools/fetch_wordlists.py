#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Fetch the word lists Standin embeds, and write them gzipped into internal/words/lists/.

Dev time only. The standin program never uses the network; it reads these files from inside
itself (go:embed). Run this only to refresh the lists, then commit the .gz files.

    uv run tools/fetch_wordlists.py            # download, build, write
    uv run tools/fetch_wordlists.py --check    # build again and exit 1 if the files differ

Lists (licences in THIRD_PARTY.md):

- English words: SCOWL / English Speller Database (ESDB), size 60, Canadian spelling, variant
  level 1, from app.aspell.net/create. Only the entries written all in lower case, without
  "'s" forms. (The capitalised entries hold many surnames and first names, like Webb or
  Nguyen, which must stay candidates.) Do not go above size 80: bigger lists bring in the UK
  Advanced Cryptics Dictionary and its own notice.
- First names: Ontario top baby names, female and male (Open Government Licence - Ontario),
  and Quebec "Banque de prenoms", filles and garcons (CC BY 4.0). Merged, lower-cased, single
  words only (no spaces or hyphens), names given 5 times or more in all.

Each output file starts with "# " lines that carry the source and its notice, so the notice
travels with every copy of the list.
"""

from __future__ import annotations

import argparse
import collections
import csv
import gzip
import io
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "internal" / "words" / "lists"

SCOWL_URL = (
    "https://app.aspell.net/create?max_size=60&spelling=CA&variant_level=1&diacritic=both"
    "&special=hacker&download=wordlist&encoding=utf-8&format=inline"
)
ON_F = (
    "https://data.ontario.ca/dataset/4d339626-98f9-49fe-aede-d64f03fa914f/resource/"
    "acc72e92-3100-4a04-8f5f-4fad9cd77cc5/download/baby_names_-_female_.csv"
)
ON_M = (
    "https://data.ontario.ca/dataset/eb4c585c-6ada-4de7-8ff1-e876fb1a6b0b/resource/"
    "5d8b8ece-fa01-43c5-955b-4b642b28c559/download/baby_names_-_male.csv"
)
QC_F = (
    "https://www.donneesquebec.ca/recherche/dataset/13db2583-427a-4e5f-b679-8532d3df571f/"
    "resource/bf77b504-54b9-4db8-be53-b92156175c12/download/grande_listef_csv.csv"
)
QC_M = (
    "https://www.donneesquebec.ca/recherche/dataset/93d640ec-d059-4768-b7ed-388604b278aa/"
    "resource/039539f5-af55-4d8f-9010-ca718e45c2a5/download/grande_listeg_csv.csv"
)

NAMES_NOTICE = """\
First names, merged from two open data sets, lower-cased, single words only, names given 5
times or more in all. Changed from the originals: merged, lower-cased, counts removed,
names with spaces or hyphens left out.

Ontario top baby names (female) and (male), Government of Ontario,
https://data.ontario.ca/dataset/ontario-top-baby-names-female
https://data.ontario.ca/dataset/ontario-top-baby-names-male
Contains information licensed under the Open Government Licence – Ontario.
https://www.ontario.ca/page/open-government-licence-ontario

Banque de prénoms - Filles, and Banque de prénoms - Garçons, Retraite Québec,
https://www.donneesquebec.ca/recherche/dataset/banque-de-prenoms-filles
https://www.donneesquebec.ca/recherche/dataset/banque-de-prenoms-garcons
Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0),
https://creativecommons.org/licenses/by/4.0/
"""


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": "standin-dev-fetch/1"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return r.read()


def gz(text: str) -> bytes:
    buf = io.BytesIO()
    # mtime=0 and no file name, so the same input gives the same bytes.
    with gzip.GzipFile(fileobj=buf, mode="wb", compresslevel=9, mtime=0, filename="") as f:
        f.write(text.encode("utf-8"))
    return buf.getvalue()


def commented(notice: str) -> str:
    return "".join(("# " + line).rstrip() + "\n" for line in notice.strip("\n").split("\n"))


def build_english() -> str:
    raw = fetch(SCOWL_URL).decode("utf-8")
    head, sep, body = raw.partition("\n---\n")
    if not sep:
        sys.exit("SCOWL: no '---' line between the notice and the words")
    for want in ("Size: 60", "Spelling: CA", "Variant Level: 1", "Copyright 2000-", "Kevin Atkinson"):
        if want not in head:
            sys.exit(f"SCOWL: the notice does not say {want!r}; check the parameters")
    words = set()
    for w in body.split("\n"):
        w = w.strip()
        if not w or w != w.lower() or w.endswith("'s"):
            continue
        words.add(w)
    notice = (
        "English words: SCOWL / English Speller Database (ESDB), size 60, Canadian spelling,\n"
        "variant level 1, from https://app.aspell.net/create (https://wordlist.aspell.net).\n"
        "Changed: only entries written all in lower case are kept, without 's forms.\n\n" + head.strip("\n")
    )
    return commented(notice) + "".join(w + "\n" for w in sorted(words))


def build_names() -> str:
    on: collections.Counter[str] = collections.Counter()
    for url in (ON_F, ON_M):
        rows = csv.DictReader(io.StringIO(fetch(url).decode("utf-8-sig")))
        for row in rows:
            k = next(c for c in row if c.startswith("Name"))
            f = next(c for c in row if c.startswith("Freq"))
            on[row[k].strip().lower()] += int(row[f])
    qc: collections.Counter[str] = collections.Counter()
    for url in (QC_F, QC_M):
        r = csv.reader(io.StringIO(fetch(url).decode("utf-8-sig")))
        next(r)
        for row in r:
            if not row:
                continue
            total = 0
            for v in row[1:]:
                v = v.strip()
                if v.isdigit():
                    total += int(v)
                elif v == "<5":
                    total += 1
            qc[row[0].strip().lower()] += total
    both = on + qc
    names = sorted(n for n, c in both.items() if c >= 5 and len(n) > 1 and " " not in n and "-" not in n)
    if len(names) < 15000:
        sys.exit(f"names: only {len(names)} names; a source may have changed")
    return commented(NAMES_NOTICE) + "".join(n + "\n" for n in names)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true", help="exit 1 if the committed files differ")
    args = ap.parse_args()
    outs = {"english.txt.gz": gz(build_english()), "firstnames.txt.gz": gz(build_names())}
    bad = 0
    for name, data in outs.items():
        p = OUT / name
        if args.check:
            if not p.exists() or p.read_bytes() != data:
                print(f"{p.relative_to(ROOT)} is out of date")
                bad = 1
            continue
        OUT.mkdir(parents=True, exist_ok=True)
        p.write_bytes(data)
        print(f"wrote {p.relative_to(ROOT)} ({len(data)} bytes)")
    return bad


if __name__ == "__main__":
    sys.exit(main())
