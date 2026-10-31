# Third-party data in Standin

Standin's own code and docs are under the MIT licence (`LICENSE`). The program also has two word
lists built in, for the word check on free text (`.md` and `.txt` files). They are in
`internal/words/lists/`, gzipped, and each one starts with the same notes as below. `standin licences`
prints them too. `tools/fetch_wordlists.py` makes the lists from their sources (dev time only; the
program never uses the network).

## Character-set tables: golang.org/x/text

Vendored in `vendor/golang.org/x/text/` and linked into the binary. It turns a message's declared
character set (Shift-JIS, KOI8-R, GB2312, ISO-2022-JP ...) into UTF-8. It is a table lookup: no network,
no parser, and nothing it decodes leaves the machine. Why not a mail library: every Go mail parser links
`net` (DESIGN 74). Licence: BSD-3-Clause.

```
Copyright 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.
```

## Test-only mail parser: github.com/emersion/go-message

Vendored in `vendor/github.com/emersion/go-message/` and compiled into the **test binary only**
(`internal/dataset/oracle_test.go`). It is not linked into `standin`: it imports `net/textproto`, and
the tool promises to carry no network code. The tests read every mail fixture with it and with our own
reader, and require the two to agree word for word. Licence: MIT.

## English words: SCOWL / English Speller Database (ESDB)

`internal/words/lists/english.txt.gz`. A word list generated at https://app.aspell.net/create from the
English Speller Database (https://wordlist.aspell.net): size 60, Canadian spelling, variant level 1,
special list "hacker", diacritics both. Changed: only entries written all in lower case are kept,
without `'s` forms. A list of size 80 or below, not Australian, needs only the notice below
(see the ESDB `Copyright` file).

```
Copyright 2000-2026 by Kevin Atkinson

Permission to use, copy, modify, distribute, and sell any part of the English
Speller Database (ESDB, previously known as SCOWLv2), or word lists
created from it, is hereby granted without fee, provided that the above
copyright notice appears in all copies and that both the above copyright
notice and this notice appear in supporting documentation.  Kevin Atkinson
makes no representations about the suitability of this database for any
purpose.  It is provided "as is" without express or implied warranty.

ESDB is derived from many sources, most of which are in the Public Domain.
Data from the Corpus of Contemporary American English (COCA) was also used.

All data from COCA comes from 3-gram data that is not freely available;
however, the usage is within the rights given by the NDA that was signed when
purchasing the data.  More information on COCA is available at
https://www.english-corpora.org/coca/.

The primary source of words for ESDB comes from 12dicts and ENABLE2K.  Both
are in the Public Domain, but Alan Beale <biljir@pobox.com> deserves special
credit as he is the author of 12dicts and a major contributor to ENABLE2K.  In
addition, he gave me an incredible amount of feedback and created a number of
special lists in order to help improve the overall quality of ESDB.
```

## First names: Ontario and Quebec baby names

`internal/words/lists/firstnames.txt.gz`. First names merged from the four data sets below. Changed
from the originals: merged, lower-cased, counts removed, only single words (no spaces or hyphens),
only names given 5 times or more in all.

**Ontario top baby names (female)** and **Ontario top baby names (male)**, Government of Ontario,
https://data.ontario.ca/dataset/ontario-top-baby-names-female and
https://data.ontario.ca/dataset/ontario-top-baby-names-male.

Contains information licensed under the Open Government Licence – Ontario.
https://www.ontario.ca/page/open-government-licence-ontario

(This use is not endorsed by the Government of Ontario.)

**Banque de prénoms - Filles** and **Banque de prénoms - Garçons**, Retraite Québec,
https://www.donneesquebec.ca/recherche/dataset/banque-de-prenoms-filles and
https://www.donneesquebec.ca/recherche/dataset/banque-de-prenoms-garcons.
Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0),
https://creativecommons.org/licenses/by/4.0/. (Not endorsed by Retraite Québec.)

Checked at the sources on 2026-10-30: the ESDB copyright notice in the generated list's header and in
the ESDB `Copyright` file; the Open Government Licence – Ontario page (the attribution statement
above is the one it gives when a data set names none, and these name none); and both Quebec data
sets' licence (`cc-by`, CC BY 4.0) on donneesquebec.ca.
