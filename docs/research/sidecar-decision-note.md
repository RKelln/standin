# A sidecar for free text: decision note

**Not a decision.** This is the material for one, written 2026-10-31 after the model evaluation
(`model-based-detection-eval.md`). Ryan leans ORT; this note prices that choice, names what it
commits us to, and describes a cheaper shape that may make the question moot.

---

## 1. What is being decided

Whether Standin ships an optional trained model for the one job the deterministic engine does badly:
**names in free text** — `.md`/`.txt` prose, `scan` columns, and (from v0.3) `.eml` bodies. Nothing
else. Column rules stay the engine for CSV, TSV and JSON; they beat every model in the field on
structured files, and that is the part of Standin that never needs this decision.

Two sub-decisions hide inside it:

1. **Which runtime.** A native ONNX Runtime (measured: 0.6 ms/line, 132 MB) or a pure-Go one
   (measured: 19 ms/line, 377 MB → 1.09 GB, cgo-free, 16.9 MB binary).
2. **When it runs.** Per scrub (every time a person scrubs), or only when the person asks for an
   audit. This one turns out to matter more than the runtime.

---

## 2. The measured board

Same model in both cases (`gravitee-io/bert-small-pii-detection`, Apache-2.0). Same box. Same
corpora. Quality is identical between them — the runtime choice buys nothing in accuracy.

| | ONNX Runtime, int8 | hugot + GoMLX, fp32 |
|---|---|---|
| artifact | 29 MB quantized ONNX + native lib | 114 MB fp32 ONNX |
| binary | sidecar + `libonnxruntime` per platform | 16.9 MB static, cgo-free |
| latency | **0.6 ms/line** | 19 ms/line median, 114 ms on a new input length |
| memory | 132 MB peak (including the harness) | 377 MB loaded, **1.09 GB at plateau** |
| build matrix | 6 platform builds + a native lib each | 6 platform builds, no extras |
| dependencies | 1 module + a shared library | 85 modules, 26 MB vendored |
| verification | the runtime the evaluation used | predictions match, measured |

Two facts that constrain the choice:

- **The pure-Go route cannot take the int8 graph** (`DynamicQuantizeLinear`, `MatMulInteger` are not
  implemented by the importer), so cgo-free means 114 MB of weights and 1.09 GB of RAM.
- **1.09 GB is a high-water mark, not a leak** — measured flat from call 20 to call 120 across 60
  distinct input lengths. It is still a lot of RAM next to Claude Code on a 4 GB laptop.

**So ORT is the right runtime if the model runs.** It is 30× faster, 8× lighter and 4× smaller in
weights, and it is already the implementation the numbers in the evaluation were produced with.
The cost is one native library per platform, which matters only because it makes the sidecar a
*different kind of artifact* from everything Standin ships today.

---

## 3. Three shapes, in increasing cost

### A. In-scrub detector — the sidecar as asked for

`standin scrub` invokes the sidecar when the config asks for it; the model proposes person spans; the
spans become candidates in the same `approve` flow the word check already feeds.

- **Buys:** recall on prose at scrub time, for every file, without the person doing anything.
- **Costs:** a second artifact across six platforms; a native library; decision 22 overturned
  (`only init may run another program`); a new failure mode where a person believes the model ran
  and it did not.

### B. Audit — the model as a verifier, not a detector

`standin audit` reads `scrubbed/`, runs the model over it, and reports **counts only** — "7 person
spans survived masking, in 3 files" — never values, in the same data-free discipline as
`standin report` (decision 16). The person runs it on demand; it exits.

- **Buys:** the number that actually matters — a leak rate — without touching the scrub path, the
  config, the hooks, or the approve flow. It cannot make a leak worse, because it only reports.
- **Costs:** the same runtime question, but a much weaker requirement: a one-shot process may take
  1.1 GB and 19 ms/line and nobody cares, which means **the cgo-free path is acceptable here** and
  the native library disappears from the decision.
- **Catches what the design cannot otherwise see:** the model has different blind spots from the
  rules, so "what survived" is evidence from an independent reader rather than a self-check.

### C. Offline teacher — no shipped code at all

Run the model on a workstation over corpora, diff against the word check, and turn the disagreements
into rules, list entries and a regression corpus. Described in `teacher-pass-plan.md`. Produces
today's handle rule (decision 90) and, more importantly, the first labelled evaluation corpus
Standin would own.

**Recommendation: C now, B at v0.3 (email), A only if B's leak rate justifies it.** The order is by
value per unit of risk, and each step produces the evidence the next one needs. B is the one that
tells you whether A is worth a native library.

---

## 4. If A happens: the contract

Written as constraints, not suggestions, because every one of them is a place where a privacy tool
gets quietly less honest.

- **The config asks, and the config says so.** `"text": {"action": "words", "names": "model"}`. A
  rule without `names` behaves exactly as today. No implicit use.
- **Fail closed, never fall back.** Model asked for and absent, unreadable or the wrong licence hash:
  `scrub` refuses (exit 2) and says how to install it. A silent fallback to the word check is a
  privacy failure wearing a convenience mask — the person would be told their prose was model-checked
  when it was not.
- **One-shot process, no daemon.** JSON lines in, JSON lines out, exit code, stderr for the reason.
  The model is needed at scrub time, not on every tool-call hook, so there is no argument for a
  socket or a long-lived service.
- **It proposes, it never decides.** Model spans enter the candidate stream as a new group in
  `approve`. The model may not write `keep`, may not write the mapping, and may not be consulted by
  `restore`.
- **Chunking, stated in the shape file.** bert-small is a 512-token model. Bodies are split into
  448-token windows with 64 tokens of overlap; a span crossing a boundary is unioned; the standalone
  window may only add spans, never remove them. A body longer than the limit is not silently
  truncated.
- **Same network test, applied to the sidecar.** `go list -deps` on the sidecar as well — the
  secondary binary is exactly where a network client would hide.
- **Weights are pinned and attributed.** Apache-2.0, checksum in `THIRD_PARTY.md`, `standin licences`
  extended, a note that a trained model is a third-party artifact and may change behaviour on
  upgrade. A model upgrade is a behavioural change and should require re-approval of word checks the
  way a config change does.
- **Decision 22 amended in writing**, not quietly broken: `init` is no longer the only command that
  runs another program; `scrub` may run `standin-pii`, and only that.
- **Uninstall is a first-class path.** Removing the sidecar must return the tool to a known state and
  say so; nothing in `scrubbed/`, `restored/` or the private folder may depend on the model having
  run.

---

## 5. What would make A the wrong call

- An audit (B) shows fewer than about one surviving span per thousand words on real-shaped corpora.
  Then the model is buying a rounding error and the platform matrix is not worth it. **The first
  evidence of this kind arrived the same day**: on 254 real judgments, once a detector bug was fixed
  (decision 91), the model added two mentions out of 1,988 — 0.1 of a point. The caveat is that
  judgments are formal text, where names nearly always carry a title; the shop case has no title to
  lean on, and that is what the audit on corpus (B) would measure.
- The six-platform matrix cannot be verified. An optional artifact that only works on Linux is worse
  than no artifact, because the docs promise it.
- The person cannot tell whether it ran. If the difference between "model-checked" and "not" is
  invisible on screen, the consent in `approve` is not informed.

## 6. Open questions for Ryan

1. Ships in `install.sh` by default, or a separate `standin model install`? Default-on contradicts
   "one program, no other software needed"; opt-in contradicts "the person will never find the flag".
2. Do we accept a native library at all, or do we hold out for a pure-Go importer that takes int8?
   (If the answer is "no native lib", then B in pure Go is the only version available today.)
3. macOS quarantines a second unsigned binary and Windows Smart App Control may block it — the same
   open item as decision 15, now doubled.
4. Which release carries it: v0.3 (email) is the natural home, because that is where the word volume
   lives and where a real inbox makes the approve list longest.
