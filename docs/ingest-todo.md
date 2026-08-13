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
