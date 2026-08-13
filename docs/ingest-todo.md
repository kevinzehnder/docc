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
- **A stored baseline.** Scores are printed and forgotten. Writing them to a
  local file would turn "did this prompt change help?" into a diff.
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
  numbers; every model recovered one. The same detection problem the prompt
  rewrite improved on a scanned brief, unsolved on a clean render, and now
  visible on every run.

A caveat on the fixture: on prose alone it cannot separate olmOCR from Qwen,
which is what a second, harder document is for.

### What the layout-first backend changed

`--backend mineru` runs MinerU2.5's two-pass protocol instead of one call per
page. Scored against the same fixture as the table above, and the chat rows
reproduce it exactly, which is the evidence that these numbers are comparable:

| backend | model | mode | words P / R | F1 | headings | Randziffern | leaked | 4 pages |
|---|---|---|---|---|---|---|---|---|
| mineru | MinerU2.5-**Pro**-2605 | — | 0.703 / **0.988** | 0.821 | **13 of 8** | 1 of 4 | 0 pn, 1 lh | **14s** |
| mineru | MinerU2.5-2509 | — | 0.667 / 0.946 | 0.782 | **13 of 8** | 1 of 4 | 0 pn, 1 lh | **12s** |
| chat | olmOCR-2-7B | vision-only | 0.642 / 0.982 | 0.777 | 0 of 8 | 1 of 4 | 0 pn, 1 lh | 35s |
| chat | olmOCR-2-7B | anchored | 0.758 / 0.982 | **0.855** | 0 of 8 | 1 of 4 | 0 pn, 1 lh | 30s |
| chat | Qwen3.5-9B | vision-only | 0.642 / 0.982 | 0.777 | 4 of 8 | 1 of 4 | 0 pn, 1 lh | 35s |
| chat | Qwen3.5-9B | anchored | 0.758 / 0.982 | **0.855** | 4 of 8 | 1 of 4 | 0 pn, 1 lh | 30s |

Four findings, and one claim withdrawn.

- **The checkpoint decided the fidelity question, not the protocol.** 2509
  dropped eight ordinary body words against the chat backends' two, and that
  read as a flaw in assembling independently-recognized blocks. Pro-2605, same
  code and same protocol, drops one — `klageschrift`, which every model in this
  table drops — and takes recall to 0.988, the highest score here. So the
  block-join loss was mostly the older checkpoint being worse at reading its
  crops, and the two MinerU rows are the evidence: nothing between them changed
  but the weights.
- **Better layout data did not fix the heading over-marking.** Thirteen against
  an expected eight, identical on both checkpoints, even though the 2605
  release notes specifically claim cleaned-up layout training data to reduce
  category errors. It types docc's own numbered section headings *and* several
  short lines beside them as `title`, and it does so consistently. Over-marking
  is the better failure — a draft with too many `##` is fixed by deleting them,
  a draft with none has to be re-read against the source — but this will not
  come out in the wash with a newer model, and needs code or a prompt.
- **Precision is now the only metric where chat is clearly ahead**: 0.703
  against 0.758 anchored, and it is not the boilerplate floor doing it — the
  invented-word lists are near-identical across all six rows. F1 still favours
  anchored chat, 0.855 to 0.821.
- **Randziffern did not improve, and the reason is geometric.** On a scanned
  third-party brief the layout pass separates the gutter cleanly — body at
  x=0.131, margin numbers at x=0.085 — and `[Rz 55]`, `[Rz 56]` come out
  correct. On our own render it emits no gutter block at all: every block on
  the page starts at x=0.067 or x=0.113, because the theme sets the Randziffer
  close enough to the body that the model reads them as one region. So the win
  is real on the documents ingest exists for and invisible on the fixture that
  measures it. A second fixture with a wider gutter would show it; changing the
  theme to suit the scorer would not.
- **It is twice as fast despite costing more calls** — 14s against 30-35s for
  four pages, and it is a 1.2B model against a 7B and a 9B. The layout pass
  sees a thumbnail and each crop is small, so the pixels per page are well
  under one whole-page image.

**Withdrawn: "dropping page furniture in code fixes the leak."** It does drop
`header` and `page_number` blocks mechanically, and that is still the right
mechanism — but this fixture cannot show it. Every row above leaks zero page
numbers and exactly one letterhead, chat backends included. The five-and-seven
leak in §2 was measured on a scanned brief, not here, so the claim has to be
re-measured there before it can be made. Nothing in this table supports it.

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
