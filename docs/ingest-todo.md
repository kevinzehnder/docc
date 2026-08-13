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
  regression and not enough to choose a model. It needs a second fixture with
  the things this one lacks: a table, a footnote, a page break mid-paragraph.
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

### What the first run found

Against `unsloth/Qwen3.5-9B-GGUF:Q4_K_M`, four pages, DPI 150:

| | precision | recall | Randziffern | leaked |
|---|---|---|---|---|
| vision-only | 0.642 | 0.982 | 1 of 4 | 1 letterhead |
| anchored | 0.758 | 0.982 | 1 of 4 | 1 letterhead |

Recall is the number to trust — the ground truth is every word the document is
known to contain. Precision has a floor well below 1 that is not the model's
fault: a theme prints boilerplate of its own, which is on the page, correctly
transcribed, and in no source to compare against.

Two findings worth acting on:

- **Anchoring measurably helps** — precision 0.642 to 0.758 on the same pages.
  It had never been measured, only assumed.
- **Randziffern are being missed on our own documents.** docc rendered four
  margin numbers and the transcription recovered one. This is the same detection
  problem the prompt rewrite improved on a scanned brief, still unsolved on a
  clean render, and now visible on every run.

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
