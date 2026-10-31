#!/usr/bin/env python3
"""Make the synthetic test fixtures in testdata/.

Every name, email, phone number, address and number in here is invented.
Emails use example.com / example.org (reserved by RFC 2606). Phone numbers use
the 555-01xx range set aside for fiction. Card numbers are the well-known
public test numbers. SINs start with 0, which is never issued to a person, and
pass the Luhn check so the pattern pass can find them.

Run:  uv run tools/make_fixtures.py          (writes testdata/)
      uv run tools/make_fixtures.py --check  (exit 1 if testdata/ is out of date)

Standard library only. Fixed random seed, so the output is the same each run.
"""

from __future__ import annotations

import argparse
import codecs
import csv
import io
import json
import random
import sys
import unicodedata
import zipfile
from pathlib import Path

SEED = 20261030
ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "testdata"

# Invented people, from many cultures. Every name must be encodable in
# Windows-1252 (the cp1252 fixture uses the same names). Some end in an
# accented letter on purpose ("Renée Dubé", "François Gagné", "Noémie Côté"):
# a regex word boundary (\b) in Go is ASCII-only and misses those.
CUSTOMERS = [
    ("Amara", "Okafor", "CA"),
    ("Kenji", "Watanabe", "CA"),
    ("Renée", "Dubé", "CA"),
    ("José", "Martínez", "US"),
    ("Priya", "Raghunathan", "CA"),
    ("Liam", "O'Connor", "UK"),
    ("Mei-Ling", "Chen", "CA"),
    ("Oluwaseun", "Adeyemi", "UK"),
    ("Lucía", "Fernández", "US"),
    ("Björn", "Åkesson", "UK"),
    ("Fatima Zahra", "Benali", "CA"),
    ("Aroha", "Ngata", "CA"),
    ("Dmitri", "Volkov", "US"),
    ("Ifeoma", "Nwosu", "CA"),
    ("Hamid", "Rahimi", "CA"),
    ("Kalani", "Kahale", "US"),
    ("Thandiwe", "Mokoena", "UK"),
    ("Ravi", "Deshpande", "CA"),
    ("Zoë", "Hartmann", "UK"),
    ("François", "Gagné", "CA"),
    ("Noémie", "Côté", "CA"),
    ("Søren", "Lindqvist", "US"),
    ("Aisha", "Al-Farsi", "CA"),
    ("Mateo", "Rossi", "US"),
]

AREA = {"CA": ["416", "604", "514", "613", "403"], "US": ["212", "415", "312", "206"], "UK": ["416", "604"]}
CITY = {
    "CA": [("Toronto", "ON"), ("Vancouver", "BC"), ("Montréal", "QC"), ("Ottawa", "ON"), ("Calgary", "AB")],
    "US": [("Springfield", "IL"), ("Portland", "OR"), ("Albany", "NY"), ("Austin", "TX")],
    "UK": [("Leeds", ""), ("Bristol", ""), ("York", "")],
}
STREETS = ["Juniper Crescent", "Maple Row", "Larkspur Lane", "Quarry Road", "Heron Way", "Birchwood Ave",
           "Kestrel Court", "Orchard Close", "Tamarack Drive", "Willow Bend", "Cedar Hollow", "Fennel Street"]
TAGS = ["wholesale", "retail", "net30", "net15", "vip", "new", "paper-invoice", "tax-exempt"]

# Well-known public test card numbers (they pass Luhn; they are not real cards).
TEST_CARDS = ["4111 1111 1111 1111", "5555 5555 5555 4444", "3782 822463 10005", "4012 8888 8888 1881",
              "6011 1111 1111 1117"]

BUSINESS_ACCOUNTS = ["00412-003-7781203", "00412-003-7781211"]


def ascii_fold(s: str) -> str:
    s = s.replace("ø", "o").replace("Ø", "O").replace("æ", "ae").replace("ß", "ss")
    s = unicodedata.normalize("NFKD", s)
    return "".join(c for c in s if not unicodedata.combining(c))


def luhn_ok(digits: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2 == 1:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return total % 10 == 0


def make_sin(rng: random.Random) -> str:
    while True:
        body = "0" + "".join(str(rng.randint(0, 9)) for _ in range(7))
        for last in "0123456789":
            if luhn_ok(body + last):
                d = body + last
                return f"{d[0:3]} {d[3:6]} {d[6:9]}"


def ca_postal(rng: random.Random) -> str:
    L = "ABCEGHJKLMNPRSTVXY"
    return f"{rng.choice(L)}{rng.randint(0,9)}{rng.choice(L)} {rng.randint(0,9)}{rng.choice(L)}{rng.randint(0,9)}"


def uk_postal(rng: random.Random) -> str:
    L = "ABDEFGHJLNPQRSTUWXYZ"
    return f"{rng.choice(['LS', 'BS', 'YO'])}{rng.randint(1, 19)} {rng.randint(0,9)}{rng.choice(L)}{rng.choice(L)}"


def us_postal(rng: random.Random) -> str:
    return f"{rng.randint(10000, 99999)}"


def csv_text(rows: list[list[str]], lineterminator: str) -> str:
    buf = io.StringIO()
    w = csv.writer(buf, lineterminator=lineterminator)
    w.writerows(rows)
    return buf.getvalue()


def tiny_pdf(text: str) -> bytes:
    objs = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R "
        b"/Resources << /Font << /F1 5 0 R >> >> >>",
        None,  # content stream, filled below
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ]
    stream = f"BT /F1 12 Tf 20 100 Td ({text}) Tj ET".encode("latin-1")
    objs[3] = b"<< /Length " + str(len(stream)).encode() + b" >>\nstream\n" + stream + b"\nendstream"
    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = []
    for i, body in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n".encode() + body + b"\nendobj\n"
    xref = len(out)
    out += f"xref\n0 {len(objs) + 1}\n".encode()
    out += b"0000000000 65535 f \n"
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += f"trailer\n<< /Size {len(objs) + 1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode()
    return bytes(out)


def fake_xlsx(names: list[str]) -> bytes:
    """A minimal workbook with inline strings. Only needs to look like a real .xlsx (a zip)."""
    rows = "".join(
        f'<row r="{i}"><c r="A{i}" t="inlineStr"><is><t>{n}</t></is></c><c r="B{i}"><v>{100 + i}</v></c></row>'
        for i, n in enumerate(names, start=1)
    )
    files = {
        "[Content_Types].xml": '<?xml version="1.0" encoding="UTF-8"?>'
        '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        '<Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/xl/workbook.xml" '
        'ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
        '<Override PartName="/xl/worksheets/sheet1.xml" '
        'ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>'
        "</Types>",
        "_rels/.rels": '<?xml version="1.0" encoding="UTF-8"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/'
        'officeDocument" Target="xl/workbook.xml"/></Relationships>',
        "xl/workbook.xml": '<?xml version="1.0" encoding="UTF-8"?>'
        '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        '<sheets><sheet name="Ledger" sheetId="1" r:id="rId1"/></sheets></workbook>',
        "xl/_rels/workbook.xml.rels": '<?xml version="1.0" encoding="UTF-8"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/'
        'worksheet" Target="worksheets/sheet1.xml"/></Relationships>',
        "xl/worksheets/sheet1.xml": '<?xml version="1.0" encoding="UTF-8"?>'
        '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f"<sheetData>{rows}</sheetData></worksheet>",
    }
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for name, body in files.items():
            info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, body.encode("utf-8"))
    return buf.getvalue()


def build() -> dict[str, bytes]:
    rng = random.Random(SEED)
    files: dict[str, bytes] = {}
    known: set[str] = set()
    gaps: list[str] = []

    # ---- people -----------------------------------------------------------
    people = []
    nn = rng.sample(range(10, 100), len(CUSTOMERS))
    for i, (first, last, country) in enumerate(CUSTOMERS):
        name = f"{first} {last}"
        domain = "example.org" if i % 3 == 2 else "example.com"
        local = ascii_fold(f"{first}.{last}").lower().replace(" ", "").replace("'", "")
        phone = f"{rng.choice(AREA[country])}-555-01{nn[i]:02d}"
        city, region = rng.choice(CITY[country])
        postal = {"CA": ca_postal, "US": us_postal, "UK": uk_postal}[country](rng)
        street = f"{rng.randint(2, 980)} {rng.choice(STREETS)}"
        people.append(dict(first=first, last=last, name=name, email=f"{local}@{domain}", phone=phone,
                           country=country, city=city, region=region, postal=postal, street=street,
                           sin=make_sin(rng) if country == "CA" else ""))
        known.update([name, f"{local}@{domain}", phone, street, postal])
        if country == "CA":
            known.add(people[-1]["sin"])

    # ---- invoices.csv ------------------------------------------------------
    order = list(range(len(people)))
    rng.shuffle(order)
    order += [rng.randrange(len(people)) for _ in range(40 - len(order))]
    inv_rows = [["Invoice", "Date", "Customer", "Email", "Phone", "Amount", "Status", "Notes"]]
    invoices = []
    day = 0
    first_seen: set[int] = set()
    for n, pi in enumerate(order):
        p = people[pi]
        day += rng.randint(1, 6)
        month, dom = 1 + day // 28, 1 + day % 28
        date = f"2026-{month:02d}-{dom:02d}"
        amount = f"{rng.randint(85, 4800)}.{rng.randint(0, 99):02d}"
        status = rng.choices(["Paid", "Open", "Overdue"], weights=[6, 2, 2])[0]
        templates = ["", "", "", "Net 30", "Paid by e-transfer", "Reminder sent to {name}",
                     "{name} asked to split the payment. Call {phone}", "Left a voicemail at {phone}",
                     "Sent receipt to {email}", "Gave {name} a 5% discount for paying early"]
        note = rng.choice(templates).format(**p)
        customer = p["name"]
        if pi in first_seen:
            # Spelling variants of a name already seen. Restore must give back the
            # first spelling, and both must get the same stand-in.
            if n == 29:
                customer = customer.lower()
            elif n == 34:
                customer = "  " + customer.replace(" ", "  ") + " "
        first_seen.add(pi)
        invoices.append(dict(no=f"INV-{1001 + n}", date=date, person=p, amount=amount, status=status))
        inv_rows.append([f"INV-{1001 + n}", date, customer, p["email"], p["phone"], amount, status, note])
    # A note that names a customer by first name only. Not in the mapping, so the
    # contract lets it through. Listed in known-gaps.txt, not known-values.txt.
    gap_p = people[order[5]]
    inv_rows[6][7] = f"Spoke with {gap_p['first']} about the deposit"
    gaps.append(gap_p["first"])
    files["invoices.csv"] = csv_text(inv_rows, "\n").encode("utf-8")

    # ---- bank-export.csv ---------------------------------------------------
    bank_rows = [["Date", "Description", "Amount", "Balance", "Account"]]
    events = []
    for inv in invoices:
        if inv["status"] == "Paid":
            events.append(("in", inv))
    expenses = ["MONTHLY ACCOUNT FEE", "HYDRO BILL PAYMENT", "RENT PAYMENT", "INSURANCE PREMIUM",
                "POS PURCHASE CORNER OFFICE SUPPLY", "POS PURCHASE PRINT SHOP", "PHONE AND INTERNET",
                "TRANSFER TO SAVINGS", "INTEREST PAID"]
    while len(events) < 59:
        events.append(("out", rng.choice(expenses)))
    rng.shuffle(events)
    martinez = next(p for p in people if p["last"] == "Martínez")
    events.insert(17, ("gap", martinez))
    balance = 8250.00
    for k, (kind, ev) in enumerate(events):
        day = 2 + k
        date = f"2026-{1 + day // 28:02d}-{1 + day % 28:02d}"
        acct = BUSINESS_ACCOUNTS[0]
        if kind == "in":
            p = ev["person"]
            ref = f"{rng.randint(10_000_000, 99_999_999)}"
            known.add(ref)
            style = rng.randrange(3)
            if style == 0:
                desc = f"E-TRANSFER FROM {p['name'].upper()} REF {ref}"
            elif style == 1:
                desc = f"INTERAC E-TRF RECEIVED {p['name']} {ref}"
            else:
                desc = f"DEPOSIT {p['name'].upper()} FOR {ev['no']} REF {ref}"
            amt = float(ev["amount"])
        elif kind == "gap":
            # Banks cut names short. "J MARTINEZ" is not in the mapping, so it stays.
            ref = f"{rng.randint(10_000_000, 99_999_999)}"
            known.add(ref)
            desc = f"E-TRANSFER FROM J MARTINEZ REF {ref}"
            gaps.append("J MARTINEZ")
            amt = 120.00
        else:
            desc = ev
            amt = -round(rng.uniform(15, 900), 2)
            if ev == "INTEREST PAID":
                amt = round(rng.uniform(0.5, 9), 2)
                acct = BUSINESS_ACCOUNTS[1]
        balance = round(balance + amt, 2)
        bank_rows.append([date, desc, f"{amt:.2f}", f"{balance:.2f}", acct])
    known.update(BUSINESS_ACCOUNTS)
    bank_text = csv_text(bank_rows, "\r\n")
    files["bank-export.csv"] = bank_text.encode("utf-8")
    files["bank-export-cp1252.csv"] = bank_text.encode("cp1252")
    files["bank-export-bom.csv"] = codecs.BOM_UTF8 + bank_text.encode("utf-8")
    files["bank-export-utf16.csv"] = bank_text.encode("utf-16")  # Python writes a BOM

    # ---- customers.json ----------------------------------------------------
    cards = iter(TEST_CARDS)
    recs = []
    for i, p in enumerate(people):
        r = {
            "id": f"C-{i + 1:04d}",
            "name": p["name"],
            "email": p["email"],
            "phone": p["phone"],
            "address": {"street": p["street"], "city": p["city"], "region": p["region"],
                        "postal": p["postal"], "country": p["country"]},
            "tags": sorted(rng.sample(TAGS, rng.randint(0, 3))),
            "balance": round(rng.uniform(0, 3000), 2),
            "active": rng.random() > 0.2,
        }
        if p["sin"]:
            r["sin"] = p["sin"]
        if i % 5 == 0:
            card = next(cards, None)
            if card:
                r["card_on_file"] = card
                known.add(card)
                known.add(card.replace(" ", ""))
        recs.append(r)
    files["customers.json"] = (json.dumps(recs, ensure_ascii=False, indent=2) + "\n").encode("utf-8")

    by_name = {}
    for p in people[:10]:
        by_name[p["name"]] = {"email": p["email"], "city": p["city"],
                              "balance": round(rng.uniform(0, 3000), 2),
                              "last_order": f"2026-0{rng.randint(1, 9)}-{rng.randint(10, 28)}"}
    files["customers-by-name.json"] = (json.dumps(by_name, ensure_ascii=False, indent=2) + "\n").encode("utf-8")

    # ---- wide-names-as-headers.csv ----------------------------------------
    hdr_people = [people[0], people[1], people[8]]
    header = ["Month"] + [p["name"] for p in hdr_people] + [people[13]["email"], people[17]["phone"], "Total"]
    wide = [header]
    for m in ["2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06"]:
        vals = [round(rng.uniform(0, 900), 2) for _ in range(5)]
        wide.append([m] + [f"{v:.2f}" for v in vals] + [f"{sum(vals):.2f}"])
    files["wide-names-as-headers.csv"] = csv_text(wide, "\n").encode("utf-8")

    # ---- ragged.csv (line 4 has an extra field: an unquoted comma) ---------
    rg = [["Invoice", "Customer", "Amount", "Status"],
          ["INV-2001", people[2]["name"], "120.00", "Paid"],
          ["INV-2002", people[3]["name"], "75.50", "Open"]]
    text = csv_text(rg, "\n")
    text += f"INV-2003,{people[0]['last']}, {people[0]['first']},310.00,Overdue\n"
    text += f"INV-2004,{people[4]['name']},42.00,Paid\n"
    files["ragged.csv"] = text.encode("utf-8")
    known.add(people[0]["last"])  # shows up alone in the ragged line

    # ---- raw input that already holds stand-in-shaped text (scrub must refuse)
    # A real "[PERSON_2]" in raw data would be "restored" to someone else's name.
    files["standin-shaped-cell.csv"] = csv_text(
        [["Invoice", "Customer", "Notes"],
         ["INV-3001", people[5]["name"], "Paid"],
         ["INV-3002", people[6]["name"], f"Same as [PERSON_2], ask {people[6]['name']}"]], "\n").encode("utf-8")
    files["standin-shaped-header.csv"] = csv_text(
        [["Invoice", "[PERSON_2]", "Amount"],
         ["INV-3003", people[7]["name"], "10.00"]], "\n").encode("utf-8")
    files["standin-shaped-key.json"] = (json.dumps(
        {"[PERSON_3]": {"email": people[9]["email"]}, people[10]["name"]: {"email": people[10]["email"]}},
        ensure_ascii=False, indent=2) + "\n").encode("utf-8")

    # ---- refused formats ---------------------------------------------------
    files["statement.pdf"] = tiny_pdf(f"Statement for {ascii_fold(people[0]['name'])}")
    files["ledger.xlsx"] = fake_xlsx([p["name"] for p in people[:4]])
    files["empty.csv"] = b""

    # ---- approved-config fixture ------------------------------------------
    config = {
        "version": 1,
        "files": [
            {
                "match": "raw/invoices*.csv",
                "columns": {
                    "Invoice": {"action": "keep", "reason": "invoice numbers are ours; needed to match"},
                    "Date": {"action": "keep", "reason": "needed to match payments"},
                    "Customer": {"action": "tokenize", "category": "PERSON", "reason": "customer names"},
                    "Email": {"action": "tokenize", "category": "EMAIL", "reason": "customer emails"},
                    "Phone": {"action": "tokenize", "category": "PHONE", "reason": "customer phones"},
                    "Amount": {"action": "keep", "reason": "needed to match payments"},
                    "Status": {"action": "keep", "reason": "Paid, Open or Overdue"},
                    "Notes": {"action": "scan", "reason": "free text; replace known names and patterns"},
                },
            },
            {
                "match": "raw/bank-export*.csv",
                "columns": {
                    "Date": {"action": "keep", "reason": "needed to match invoices"},
                    "Description": {"action": "scan", "reason": "names and reference numbers inside"},
                    "Amount": {"action": "keep", "reason": "needed to match invoices"},
                    "Balance": {"action": "keep", "reason": "running balance"},
                    "Account": {"action": "tokenize", "category": "ACCOUNT", "reason": "our account numbers"},
                },
            },
        ],
    }
    files["standin.config.json"] = (json.dumps(config, indent=2) + "\n").encode("utf-8")

    files["known-values.txt"] = (
        "# Every sensitive value in the fixtures, one per line. Invented data.\n"
        "# Made by tools/make_fixtures.py. Leak checks compare case-insensitively,\n"
        "# with runs of whitespace collapsed.\n"
        + "".join(v + "\n" for v in sorted(known))
    ).encode("utf-8")
    files["known-gaps.txt"] = (
        "# Sensitive text the contract lets through on purpose (names not in the mapping).\n"
        "# Tests log whether these leak; they do not fail on them.\n"
        + "".join(v + "\n" for v in gaps)
    ).encode("utf-8")
    files.update(build_text())
    files.update(build_mail())
    return files


# ---- email (.eml) ------------------------------------------------------------
# One message per file, in the shape a mail program exports: header lines, MIME
# parts, transfer encodings. All invented, example.com addresses, 555-01xx numbers.


def build_mail() -> dict[str, bytes]:
    import base64
    import quopri

    out: dict[str, bytes] = {}
    crlf = lambda s: s.replace("\n", "\r\n").encode("utf-8")

    # 1. One part, quoted-printable, an encoded Subject, CRLF line endings.
    body = (
        "Hi Sam,\n\n"
        "The blue bike is ready whenever you are. My number is 416-555-0143\n"
        "if you need to reach me before Friday.\n\n"
        "Thanks,\n"
        "Oksana Drozdenko\n"
    )
    out["mail/simple.eml"] = crlf(
        "From: Oksana Drozdenko <oksana.drozdenko@example.com>\n"
        "To: shop@example.com\n"
        "Date: Wed, 02 Sep 2026 10:14:00 -0400\n"
        "Subject: =?UTF-8?Q?Re=3A_tune=2Dup_for_the_blue_bike?=\n"
        "Message-ID: <20260902101400.1@example.com>\n"
        "MIME-Version: 1.0\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "Content-Transfer-Encoding: quoted-printable\n"
        "\n"
        + quopri.encodestring(body.encode("utf-8")).decode("ascii")
    )

    # 2. multipart/alternative: the plain text is preferred; the HTML copy is
    #    ignored, and it holds a name that must not appear twice.
    alt_plain = (
        "Hello,\n\n"
        "Can you fit a new chain on Thursday? Kofi Mensavi will drop it off.\n\n"
        "Ines Vantwerp\n"
    )
    alt_html = (
        "<html><head><style>p{font-family:Arial}</style></head><body>"
        "<p>Hello,</p><p>Can you fit a new chain on Thursday? "
        "<b>Kofi Mensavi</b> will drop it off.</p>"
        '<p><a href="https://example.com/track/abc123">Ines Vantwerp</a></p>'
        "</body></html>"
    )
    boundary = "----standin-alt-0001"
    out["mail/alternative.eml"] = crlf(
        "From: Ines Vantwerp <ines.vantwerp@example.com>\n"
        "To: shop@example.com\n"
        "Date: Thu, 03 Sep 2026 08:02:00 -0400\n"
        "Subject: chain on Thursday\n"
        "MIME-Version: 1.0\n"
        f'Content-Type: multipart/alternative; boundary="{boundary}"\n'
        "\n"
        f"--{boundary}\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "Content-Transfer-Encoding: quoted-printable\n"
        "\n"
        + quopri.encodestring(alt_plain.encode("utf-8")).decode("ascii")
        + f"\n--{boundary}\n"
        "Content-Type: text/html; charset=utf-8\n"
        "\n" + alt_html + "\n"
        f"--{boundary}--\n"
    )

    # 3. HTML only: the text comes from the HTML part.
    out["mail/html-only.eml"] = crlf(
        "From: Quillfeather Bikes <hello@quillfeather.example.org>\n"
        "To: shop@example.com\n"
        "Date: Fri, 04 Sep 2026 12:00:00 -0400\n"
        "Subject: September sale\n"
        "MIME-Version: 1.0\n"
        "Content-Type: text/html; charset=utf-8\n"
        "\n"
        "<html><head><style>.x{color:red}</style><script>var t=1;</script></head><body>"
        "<p>Hi Tavish,</p><p>Pads are 15&amp; off this week.<br>Ask for Yaretzi.</p>"
        "</body></html>\n"
    )

    # 4. base64 body with a non-ASCII name.
    b64_body = (
        "Bonjour,\n\n"
        "Je passe mardi. Demandez Farida Okorafor au comptoir.\n\n"
        "Merci,\n"
        "Noémie Côté\n"
    ).encode("utf-8")
    out["mail/base64.eml"] = crlf(
        "From: Noemie Cote <noemie.cote@example.com>\n"
        "To: shop@example.com\n"
        "Date: Sat, 05 Sep 2026 09:30:00 -0400\n"
        "Subject: mardi\n"
        "MIME-Version: 1.0\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "Content-Transfer-Encoding: base64\n"
        "\n"
        + base64.encodebytes(b64_body).decode("ascii")
    )

    # 5. multipart/mixed: the text body, then an attachment that must not be read.
    mixed = "----standin-mix-0002"
    out["mail/attachment.eml"] = crlf(
        "From: Saoirse Brannagh <saoirse.brannagh@example.com>\n"
        "To: shop@example.com\n"
        "Date: Sun, 06 Sep 2026 15:45:00 -0400\n"
        "Subject: receipt attached\n"
        "MIME-Version: 1.0\n"
        f'Content-Type: multipart/mixed; boundary="{mixed}"\n'
        "\n"
        f"--{mixed}\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "\n"
        "Here is the receipt you asked for.\n"
        f"\n--{mixed}\n"
        "Content-Type: text/plain; charset=utf-8\n"
        'Content-Disposition: attachment; filename="receipt-2041.txt"\n'
        "\n"
        "ATTACHMENT BODY Zelda Quorn 416-555-0144\n"
        f"\n--{mixed}--\n"
    )

    # 6. A forwarded message inside the message.
    fwd = "----standin-fwd-0003"
    out["mail/forwarded.eml"] = crlf(
        "From: shop@example.com\n"
        "To: bookkeeper@example.com\n"
        "Cc: owner@example.com\n"
        "Date: Mon, 07 Sep 2026 11:00:00 -0400\n"
        "Subject: Fwd: late payment\n"
        "MIME-Version: 1.0\n"
        f'Content-Type: multipart/mixed; boundary="{fwd}"\n'
        "\n"
        f"--{fwd}\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "\n"
        "Can you look at this?\n"
        f"\n--{fwd}\n"
        "Content-Type: message/rfc822\n"
        "\n"
        "From: Nkemdi Ellingworth <nkemdi.ellingworth@example.com>\n"
        "To: shop@example.com\n"
        "Date: Sun, 06 Sep 2026 20:10:00 -0400\n"
        "Subject: late payment\n"
        "Content-Type: text/plain; charset=utf-8\n"
        "\n"
        "Sorry, the cheque goes out Monday. Bolat Zarnowski has the details.\n"
        f"\n--{fwd}--\n"
    )

    # 7. A character set no mail program writes, and an mbox file: both refused.
    out["mail/bad-charset.eml"] = (
        b"From: Bolat Zarnowski <bolat.zarnowski@example.com>\n"
        b"To: shop@example.com\n"
        b"Date: Tue, 08 Sep 2026 10:00:00 -0400\n"
        b"Subject: test\n"
        b"Content-Type: text/plain; charset=x-not-a-charset\n"
        b"\n"
        b"\x82\xa0\x82\xa2 in a character set nobody writes.\n"
    )

    # 8. A real character set (KOI8-R), decoded to UTF-8 rather than refused.
    # Cyrillic has capital letters, so the word check still applies.
    _ru_body = "Здравствуйте. Велосипед готов, заберите в четверг.\n\nИван Петров\n"
    _ru_subj = base64.b64encode("Велосипед готов".encode("koi8-r")).decode("ascii")
    out["mail/koi8r.eml"] = (
        "From: Ivan Petrov <ivan.petrov@example.com>\n"
        "To: shop@example.com\n"
        "Date: Wed, 09 Sep 2026 09:00:00 -0400\n"
        f"Subject: =?KOI8-R?B?{_ru_subj}?=\n"
        "Content-Type: text/plain; charset=koi8-r\n"
        "\n"
    ).encode("ascii") + _ru_body.encode("koi8-r")

    out["mail/mbox.eml"] = (
        b"From shop@example.com Wed Sep  2 10:14:00 2026\n"
        b"From: Oksana Drozdenko <oksana.drozdenko@example.com>\n"
        b"Subject: one of several\n"
        b"\n"
        b"Body of the first message.\n"
    )
    return out


# ---- free text (.md, .txt) ---------------------------------------------------
# Invented people for the free-text fixtures. None of them is in CUSTOMERS, so the
# word check (not the known-values pass) must find them. Surnames are made-up words.
TEXT_FIRST = ["Oksana", "Kofi", "Tavish", "Ines", "Yaretzi", "Bolat", "Farida", "Thiago", "Nkemdi", "Saoirse"]
TEXT_LAST = ["Drozdenko", "Mensavi", "Okorafor", "Quellbrook", "Vantwerp", "Zarnowski", "Ellingworth", "Brannagh"]
# First names that are also English words. They are caught only in a name spot
# (after Hi or Thanks, in a sign-off) or on a field line, so the fixtures put them there.
TEXT_WORDNAMES = ["Grace", "Hope", "Rose"]
TEXT_BRANDS = ["Shimano", "Brompton", "Quillfeather"]  # words a person would keep
TEXT_PLACES = ["Dundas", "Roncesvalles"]
TEXT_DAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]
TEXT_MONTHS = ["March", "April", "June", "July", "August", "September"]
TEXT_JOBS = ["tune-up", "flat repair", "brake pads", "wheel true", "chain replacement"]


def build_text() -> dict[str, bytes]:
    rng = random.Random(SEED + 1)
    out: dict[str, bytes] = {}
    names: set[str] = set()
    # inbox.md: emails with From/Date/Subject fields, greetings, sign-offs.
    lines = ["# Shared inbox (invented)", "", "Everything here is made up.", ""]
    for i in range(1, 31):
        first = rng.choice(TEXT_FIRST)
        last = rng.choice(TEXT_LAST)
        other = rng.choice([n for n in TEXT_FIRST if n != first])
        word = rng.choice(TEXT_WORDNAMES)
        names.update([first, last, other, word])
        local = f"{first}.{last}".lower()
        lines += [
            f"## E{i:02d}",
            f"From: {first} {last} <{local}@example.com>",
            f"Date: {rng.choice(TEXT_DAYS)[:3]}, {rng.choice(TEXT_MONTHS)[:3]} {rng.randint(1, 28)}, 2026",
            f"Subject: {rng.choice(TEXT_JOBS)} {rng.choice(['question', 'booking', 'quote'])}",
            "",
            rng.choice(["Hi Sam,", "Hello,", f"Hi Sam and {word},"]),
            rng.choice([
                f"Can I book a {rng.choice(TEXT_JOBS)} for {rng.choice(TEXT_DAYS)}? My {rng.choice(TEXT_BRANDS)} is making a noise.",
                f"My partner {other} will pick up the bike on {rng.choice(TEXT_DAYS)} after work.",
                f"Do you have {rng.choice(TEXT_BRANDS)} pads in stock? I live near {rng.choice(TEXT_PLACES)}.",
                f"Can you text {other} when the bike is done? The number is 416-555-01{rng.randint(10, 99)}.",
                f"Thanks again for the {rng.choice(TEXT_JOBS)} last {rng.choice(TEXT_MONTHS)}. Say hi to {other}.",
            ]),
            "",
            rng.choice(["Thanks,", "Cheers,", "Regards,"]),
            rng.choice([first, word]),
            "",
        ]
    names.add("Sam")
    out["inbox.md"] = "\n".join(lines).encode("utf-8")

    # notes.md: meeting notes with field lines, a list, and a Markdown table.
    a, b, c = rng.sample(TEXT_FIRST, 3)
    la, lb = rng.sample(TEXT_LAST, 2)
    names.update([a, b, c, la, lb, "Grace"])
    notes = [
        "# Team meeting, 2026-03-04",
        "",
        f"Attendees: {a} {la}, {b} {lb}, Grace",
        f"Regrets: {c}",
        "",
        "## Actions",
        f"- {a} to call the supplier about {TEXT_BRANDS[0]} pads.",
        f"- Ask {b} about the {TEXT_PLACES[0]} lease. Grace has the keys.",
        "- Order new tubes before the spring rush.",
        "",
        "## Prices",
        "",
        "| Service | Price |",
        "|---|---:|",
        "| Tune-up | $89 |",
        "| Flat repair | $25 |",
        "| Wheel true | $35 |",
        "",
        "Next meeting: Tuesday, April 7.",
        "",
    ]
    out["notes.md"] = "\n".join(notes).encode("utf-8")

    # receipt.txt: a typed receipt; header words are a supplier name.
    out["receipt.txt"] = "\n".join([
        f"{TEXT_BRANDS[2].upper()} BIKE PARTS",
        "Receipt R-77",
        "Date: 2026-07-09",
        "--------------------------------",
        "Brake pads x24              168.00",
        "Tubes 700c x40              216.00",
        "--------------------------------",
        "TOTAL                       384.00",
        "",
        "Paid: debit",
        f"Served by {rng.choice(TEXT_FIRST)} {rng.choice(TEXT_LAST)}",
        "",
    ]).encode("utf-8")
    for w in out["receipt.txt"].decode().split("\n")[-2].split()[2:]:
        names.add(w)

    # table.txt: a .txt that is really a table; refused (rename it to .csv).
    out["table.txt"] = b"Date,Item,Amount\n2026-01-02,Tubes,12.00\n2026-01-03,Pads,28.00\n2026-01-04,Chain,38.00\n"
    # uncased.md: mostly a script with no capital letters (random CJK letters, no meaning).
    han = "".join(chr(rng.randint(0x4E00, 0x9FA5)) for _ in range(120))
    out["uncased.md"] = f"# Notes\n\n{han[:60]}\n{han[60:]}\n".encode("utf-8")

    out["text-names.txt"] = (
        "# Every invented person-name word in inbox.md, notes.md and receipt.txt.\n"
        "# The word check must swap each one where it appears capitalised.\n"
        + "".join(n + "\n" for n in sorted(names))
    ).encode("utf-8")
    out["text-keep.txt"] = (
        "# Brand and place words in the text fixtures, which a person would keep.\n"
        + "".join(n + "\n" for n in TEXT_BRANDS + TEXT_PLACES)
    ).encode("utf-8")
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--check", action="store_true", help="compare with testdata/ and exit 1 if different")
    args = ap.parse_args()
    files = build()
    if args.check:
        bad = [n for n, b in files.items() if not (OUT / n).exists() or (OUT / n).read_bytes() != b]
        for n in bad:
            print(f"out of date: testdata/{n}")
        return 1 if bad else 0
    OUT.mkdir(exist_ok=True)
    for name, body in files.items():
        path = OUT / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(body)
    print(f"wrote {len(files)} files to {OUT.relative_to(ROOT)}/")
    return 0


if __name__ == "__main__":
    sys.exit(main())
