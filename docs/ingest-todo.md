# Ingest — what is left

The pipeline works end to end: `docc ingest` → `docc structure` → `docc check`.
What follows is the work that is not done, in the order it is worth doing.

## 0. What the 38-page Replik iteration fixed (2026-08-13)

Converting `assets/example_replik.pdf` whole, repeatedly, against the page
images as ground truth. Five defects found and fixed, each with a regression
test; the committed round-trip fixture holds its baseline exactly (its four
born-digital pages never trip these bugs, which is the argument for a second,
scanned fixture), and the full Replik now carries 118 of its 119 Randziffern
with every heading at its correct level.

- **A merged gutter column lost every number but its first.** The layout pass
  returns the whole margin as one `aside_text` ("25\n26\n27\n28"), and
  attachment used the block's top for all four. Positions are now interpolated
  across the block's span and advance monotonically; the fuzzy interpolated
  path also skips offers of proof and indented continuations, which a
  Randziffer never numbers.
- **Merged-into-body numbers failed on wrapped paragraphs.** A recognized
  paragraph keeps the source's line wraps, and the `randziffer` regexp's `$`
  never matched text with a newline in it — every wrapped paragraph, which on
  a brief is most of them. `(?s)`.
- **The layout pass re-detects a region.** On page 26 it answered the same
  `text` line three times; each copy was recognized and transcribed, tripling
  paragraph 108. Blocks whose box overlaps an earlier same-type block at
  IoU > 0.9 are dropped before recognition, which also saves their round
  trips. `collapseRepeats` separately collapses a decoder loop's copies
  inside one block, table crops exempt.
- **The chain normalizer orphaned everything before its longest run.** On the
  whole document the longest consecutive run was 51-119, and resumption only
  walked forward: Rz 1-50 were thrown away. `chainWithin` now recurses into
  the prefix and suffix around each chosen run, so every run that earns
  minRZRun keeps its numbers.
- **Frontmatter knows its provenance.** `source_file` and `source_pages` are
  filled in by ingest when the target schema declares both fields; `docc
  check`'s missing-field list shrinks to the ones only a person can answer.

Schema, not code: the numbered-title patterns' length cap was 58 chars, which
demoted `2. Fehlende Kündigungsberechtigung der Beklagten – Unwirksamkeit der
Kündigungen` (77); it is 90 now. `roman-first` also gained the dot form of the
letter level (`b. Falsche Kündigungsbegründungen`), which this corpus uses.

Two things left visible on purpose. Rz 50's digit is dropped by recognition
nondeterministically (the paragraph itself survives), and REF010 points a
reviewer straight at it — inventing the number from the sequence would be
guessing about somebody else's brief. And `TestDebugPage` in
`internal/ingest/debug_live_test.go` stays: env-gated, skipped in CI, and it
found three of the five bugs above by dumping the raw layout answer for one
page (`DOCC_DEBUG_PDF=... DOCC_DEBUG_PAGE=6 DOCC_DEBUG_DPI=150 go test -run
TestDebugPage -v ./internal/ingest`). The 150/200 DPI split matters: the
layout pass answers differently per DPI, and a bug that reproduces at the
config's 150 can vanish at the debug default.

### The second document (2_Klageantwort.pdf, born-digital, 18 pages)

A docc-rendered Klageantwort whose authored markdown is exact ground truth —
the second fixture item 1 asks for, in spirit. Its margins are harder than the
Replik's: Randziffern, bare section numbers ("5.") and misreads share one
gutter column, and its lists (prayers for relief, a Beilagenverzeichnis) are
exactly the shape of numbered section titles. Four more fixes:

- **A noisy margin column leaked or vanished.** `gutterNumbers` required every
  line to be a bare number, so "8\nE\n9\n10\n1" was neither gutter (the "E")
  nor prose — it leaked as a garbage paragraph and every number it carried was
  lost. `marginLines` now qualifies a column when its numeric lines outnumber
  the junk; junk keeps its interpolation slot and attaches nothing. Interior
  interpolated positions get half a step of slack — line spacing follows the
  paragraphs, not a grid — while the endpoints stay exact.
- **A dotted margin number is a section number.** "5." sits beside its heading,
  not beside prose: it binds to the first heading below that does not already
  carry a number, and never to a paragraph.
- **A list is not a table of contents.** "1. Anwaltsvollmacht vom 4. August
  2025" is an exhibit and "1. Die Klage sei abzuweisen" is a prayer, and each
  alone matches the numbered-title pattern. `demoteNumberedRuns` unmarks
  headings inside a run of consecutively numbered adjacent elements: nine
  adjacent "titles" with no body between them are a Beilagenverzeichnis, and a
  "title" whose neighbour paragraph carries the next number opens the list
  that paragraph continues. Real numbered sections have content between them
  and are never adjacent.
- **I. between H. and J. is a letter.** The Roman/letter tie goes to Roman by
  count, which is right until a brief actually reaches nine lettered sections.
  `FinalizeNodes` — the first document-level outline pass — re-levels a single
  I., V. or X. whose neighbouring letter headings are its alphabetic
  predecessor or successor.

The schema gained the bare-number heading (`^\d{1,2}\.$`), because this
corpus writes sub-sections as "1." with unnamed prose following.

Result over the whole document: Rz 1-67 with one gap (11, misread as "1" in
the gutter, correctly rejected by the chain and flagged by REF010), every
heading at its correct level including the A.-K. run, prayers and exhibits as
lists. Remaining `docc check` errors are the three fields only a person can
answer.

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
