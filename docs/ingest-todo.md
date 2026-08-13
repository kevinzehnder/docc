# Ingest — what is left

The pipeline works end to end: `docc ingest` → `docc structure` → `docc check`.
What follows is the work that is not done, in the order it is worth doing.

## 1. The evaluation harness is thin

`task test:eval` exists and works: it renders `testdata/good/legal_valid.md` to
PDF through docc's own build path, transcribes it back, and scores the result
against the source it started from. Ground truth is exact because we wrote the
document, the fixture is committed, and no client file is involved.

What it does not yet have:

- **More than one document.** One four-page brief is enough to catch a
  regression and not enough to choose a model — two models scored identically
  on its prose. It needs a second fixture with the things this one lacks: a
  table, a footnote, a page break mid-paragraph.
- ~~**A stored baseline.** Scores are printed and forgotten.~~ **Done.**
  `internal/eval/testdata/baseline.txt` holds the last scores and the run fails
  on anything worse; `task test:eval -- -update` accepts a change. Word scores
  are compared within 0.005, because a different quantization moves the third
  decimal without anything being wrong, and the counts exactly. Found counts are
  compared by *distance* from what the document has — thirteen headings on a
  document with eight is over-marking, and fifteen is worse, which a
  bigger-is-better comparison would have called an improvement.
- **Scanned input.** Every fixture it renders is born-digital and clean. Real
  scans are skewed and noisy, and nothing measures that; `-doc` scores an
  external PDF but only structurally when there is no text layer.

Two traps already learned, kept here because they cost time once:

- **A fixed seed means repeating a run cannot estimate variance.** Identical
  input gives identical output. Raise n by adding *pages*, not repeats. A
  two-page A/B once inverted between conditions and looked like a real effect;
  it was noise, and only thirty instances settled it.
- **Score against the source, not against plausibility.** At 200 dpi the model
  reproduced a typo that was genuinely in the PDF's text layer, and at 150 it
  silently corrected it. The corrected version reads better and is wrong: a
  transcription that fixes its source cannot be cited.

### What the first runs found

Round trip, four pages, DPI 150, fixed seed:

| model | mode | words P / R | headings | Randziffern |
|---|---|---|---|---|
| olmOCR-2-7B | vision-only | 0.642 / 0.982 | **0 of 8** | 1 of 4 |
| olmOCR-2-7B | anchored | 0.758 / 0.982 | **0 of 8** | 1 of 4 |
| Qwen3.5-9B | vision-only | 0.642 / 0.982 | **4 of 8** | 1 of 4 |
| Qwen3.5-9B | anchored | 0.758 / 0.982 | **4 of 8** | 1 of 4 |
| gemma-4-E4B | vision-only | 0.580 / 0.771 | — | 1 of 4 |
| gemma-4-E4B | anchored | crashed the server | | |

Recall is the number to trust — the ground truth is every word the document is
known to contain. Precision has a floor well below 1 that is not the model's
fault: a theme prints boilerplate that is on the page, correctly transcribed,
and in no source to compare against.

Four findings:

- **olmOCR emits no headings at all.** Its word scores are identical to
  Qwen's — the two disagree on almost nothing in the prose — but it writes
  every heading as plain text. For a compiler whose next stage validates
  section structure, that draft fails every section rule, and the word scores
  cannot see it. This is why the heading count exists: without it the two
  models looked interchangeable.
- **Anchoring measurably helps** — precision 0.642 to 0.758 on the same pages,
  for both models. Previously assumed, never measured.
- **gemma-4 is worse on words and unstable.** Recall 0.771 against 0.982, it
  dropped real content, and the anchored run took the server down with
  `proxy error: Could not establish connection` — consistent with the
  oversized `ctx-size` in its router profile (see below), still unconfirmed.
- **Randziffern are missed on our own documents.** docc rendered four margin
  numbers; every model recovered one. ~~The same detection problem the prompt
  rewrite improved on a scanned brief.~~ **Not a detection problem at all** —
  see the correction below. Every model transcribed all four correctly and the
  normalizer threw them away.

A caveat on the fixture: on prose alone it cannot separate olmOCR from Qwen,
which is what a second, harder document is for.

### Why 150 dpi

Measured on Replik.pdf p30 (dense German legal prose), fixed seed, projector on
the GPU — so these differences are the image, not sampling noise:

| dpi | wall time |
|---|---|
| 110 | 11.0s |
| 150 | 11.4s |
| 200 | 12.0s |

Speed is nearly flat, because with the projector on the GPU the run is dominated
by generating ~800 tokens rather than by encoding the image. **DPI is not a
speed knob here — pick it on fidelity.**

The only textual difference between 150 and 200 was `Steuerklärung` against
`Steuererklärung`. The PDF's own text layer reads `Steuerklärung` — the author's
typo — so 200 reproduced the source and 150 silently corrected it. For a legal
document, faithful beats plausible: a transcription that fixes its source is a
transcription you cannot cite. 110 and 150 were identical, so the two of them
share that behaviour.

150 is kept because one page is not enough to choose on, and it stays inside
what olmOCR was trained for (it renders to a 1288 px longest edge, ~110 dpi for
A4). If silent normalisation shows up again, raise it to 200 — the extra 0.6s
per page is nothing next to a wrong quotation.

### What the layout-first backend changed

`--backend mineru` runs MinerU2.5's two-pass protocol instead of one call per
page. The chat rows reproduce the table above exactly, which is the evidence
that these are comparable:

| backend | model | mode | words P / R | F1 | headings | Randziffern | 4 pages |
|---|---|---|---|---|---|---|---|
| mineru | Pro-2605, `--type --outline-strict` | — | **0.755 / 0.988** | **0.856** | **8 of 8** | **4 of 4** | **11s** |
| mineru | Pro-2605, `--type` | — | 0.755 / 0.988 | 0.856 | 15 of 8 | 4 of 4 | 11s |
| chat | Qwen3.5-9B, `--type` | anchored | 0.744 / 0.982 | 0.847 | 14 of 8 | 4 of 4 | 29s |
| mineru | Pro-2605, no type | — | 0.703 / 0.988 | 0.821 | 13 of 8 | 1 of 4 | 13s |
| mineru | 2509, no type | — | 0.667 / 0.946 | 0.782 | 13 of 8 | 1 of 4 | 12s |
| chat | olmOCR / Qwen, no type | anchored | 0.758 / 0.982 | 0.855 | 0 / 4 of 8 | 1 of 4 | 30s |

**The structure of the top row is exact.** Eight headings of eight at their
correct levels, four Randziffern of four with no sequence break, no page number
leaked. That is the whole reason this backend exists, and it took the model, the
schema's outline, and two normalizers that read the document before deciding
anything.

The words are not exact and will not be. Recall 0.988 is one dropped word,
`klageschrift`, which every model in this file drops. Precision has a floor
well below 1 that is not the model's fault, documented above: the theme prints
boilerplate that is on the page, correctly transcribed, and in no source to
compare against.

- **The fidelity question was the checkpoint, not the protocol.** 2509 dropped
  eight ordinary body words against the chat backends' two; Pro-2605, same code,
  drops one — `klageschrift`, which every model here drops.
- **Randziffern are 4 of 4, and were never a detection problem.** This was
  recorded for a day as a model failure: "docc rendered four margin numbers;
  every model recovered one." Every model in fact transcribed all four
  perfectly. `rzNormalizer` threw them away, because it accepted the first
  bare number in the document as the start of the sequence and the first bare
  number is `5400 Baden` in the letterhead. Having anchored the count at 5400 it
  rejected 1, 2, 3 and 4 for not continuing from 5401 — and the "1 found" in
  every row above was the postal code. The chain is now chosen after reading the
  whole document rather than from its first link, and both backends score 4 of 4.
  The lesson is the one this file already knew and applied to the wrong half of
  the problem: the sequence is the safeguard, and a safeguard consulted one
  element at a time is not one.
- **Headings: the eight real ones are now exactly right.** `I. RECHTSBEGEHREN`
  through `V. FAZIT`, each at its correct level, which no model managed unaided.
  The remaining seven are over-marking left in place on purpose: five are the
  layout pass typing cover-page lines (`EINSCHREIBEN`, a party name) as `title`,
  and two are `Beilagen` items short enough to look like numbered titles. A
  spurious `##` is visible and deletable; unmarking on suspicion would strip the
  structure out of any brief written to a convention nobody anticipated.

Two bugs the outline work surfaced, both fixed:

- **The layout pass reports a container and its children both.** A `list` block
  spanning three `text` blocks was recognized alongside them, so a Rechtsbegehren
  with two prayers transcribed as four. Containers are skipped now, which also
  removes the round trips.
- **`1.` opens an ordered list and a third-level heading alike.** Every prayer
  for relief was becoming a section title. The patterns now require a title to
  be short and to end in something other than sentence punctuation.

**Correction, recorded because it was acted on.** This file previously said
llama.cpp strips the special tokens carrying table structure. It does not:
`common_token_to_piece` defaults to `special = true`, and the OTSL tokens
(`<fcel>`, `<ecel>`, `<nl>` …) are present in the GGUF vocabulary. The output is
nonetheless unstructured — a ruled four-row table returns its rows as newlines
and its columns concatenated, identically on both checkpoints, so it is neither
the quantization nor the model version. Column structure is not recoverable from
this path today, and the cause is still unknown; that is a smaller and more
honest claim than the one it replaces.

### What the stored baseline recorded

The first full run of `task test:eval -- -update`, all four profiles, with the
schema's outline and `--outline-strict`:

| backend / model | mode | P / R | F1 | headings | Rz |
|---|---|---|---|---|---|
| mineru / Pro-2605 | — | **0.755 / 0.988** | **0.856** | 8/8 | 4/4 |
| chat / Qwen3.5-9B | anchored | 0.744 / 0.982 | 0.847 | 8/8 | 4/4 |
| chat / olmOCR-2-7B | anchored | 0.744 / 0.982 | 0.847 | 8/8 | 4/4 |
| chat / Qwen3.5-9B | vision-only | 0.634 / 0.982 | 0.770 | 8/8 | 4/4 |
| chat / olmOCR-2-7B | vision-only | 0.634 / 0.982 | 0.770 | 8/8 | 4/4 |

Three things this says that the earlier tables could not.

- **The typed-Node rewrite moved nothing.** `mineru` reproduces 0.755 / 0.988 /
  0.856 and 8-of-8 headings exactly, across four commits that replaced the
  markdown intermediate with document elements. That was asserted by a golden
  test on synthetic blocks; this is the same claim against a live model.
- **The outline scheme has erased the difference between the two chat models.**
  olmOCR and Qwen now agree to three decimals in both modes *and* on headings.
  This file's argument for counting headings at all was that "without it the two
  models looked interchangeable" — olmOCR marked none of eight, Qwen fourteen.
  With the scheme supplying the levels, they are interchangeable, because the
  thing that separated them is no longer the model's job. **This fixture can no
  longer choose between chat models**, which sharpens rather than answers the
  "needs a second document" item above.
- **Anchoring is worth 0.110 of precision**, on both models, reproducing the
  0.116 measured before. It remains the whole reason the chat backend is worth
  keeping.

`letterheads=1` in every row is a false positive, not a leak. The check counts
every line containing the letterhead string, and `Bezirksgericht Baden` is
legitimately on page 1 as the addressee. It should only fire on a string
appearing on more than one page.

## 2. Ingest still leaks running headers and footers

Measured over eight pages of a scanned brief, in the best prompt condition: two
page numbers and three letterheads survived into the output. Better than the
five and seven the original prompt produced, and not zero.

Both are mechanically detectable — a line that is nothing but `- N -`, a known
firm name repeating on every page — so this belongs in code for the same reason
the Randziffer marking does: the model does it right about two thirds of the
time, and code does it the same way every time.

## 3. Cross-document citation validation

The `legal_reference` schema requires `cite_as` (`"KA"`) precisely so this
becomes possible: a brief citing `KA Rz 91` could be resolved against the
transcribed Klageantwort, and `docc check` could report a citation that points at
a paragraph which does not exist.

Nothing else in the toolchain can do this, and it is the reason the reference
type carries an abbreviation at all. It needs a way to find sibling documents in
a project, which does not exist yet.

## 4. A reference document cannot be built

`testdata/reference/` exists because `good/` requires a fixture to also build to
`.docx`, and the filing theme interpolates a court, parties and a signatory that
a third party's document does not have. A reading copy of an opposing brief is a
reasonable thing to want, and it needs its own minimal theme — page geometry, the
body styles, no letterhead and no signature block.

## 5. Smaller, known

- The gemma-4 profile in the local router config sets `ctx-size = 131072`
  alongside a draft model. That cannot fit on 8 GB with a projector on the GPU;
  it is either spilling to the CPU or failing to load. Never measured.
- `structure` finds offers of proof by their lead label. A block introduced some
  other way is not found, and nothing reports that it was missed — unlike a
  block that fails to convert, which is reported by line.
