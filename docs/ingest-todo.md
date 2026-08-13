# Ingest — what is left

The pipeline works end to end: `docc ingest` → `docc structure` → `docc check`.
What follows is the work that is not done, in the order it is worth doing.

## 1. There is no evaluation harness

**This is the largest gap.** Every decision the ingest design rests on — the
projector on the GPU, DPI 150, marking Randziffern in code rather than in the
prompt, the structuring prompt's handling of trailing attachment labels — was
established by running shell commands by hand and reading the output. None of it
is committed, none of it is repeatable, and nothing would notice if a prompt
change or a model swap made any of it worse.

Every automated test in the repository talks to an `httptest` fake. That is
right for the plumbing — streaming, the stall watchdog, the guards, region
detection — but it means **no test has ever seen a model's output**.

What a harness needs:

- A committed corpus: a handful of pages with known-correct expected output.
  These cannot be client documents, so they have to be written for the purpose —
  a synthetic Swiss brief with Randziffern, offers of proof, umlauts, a table,
  and a footnote, rendered to PDF once and committed alongside its expected
  markdown.
- Mechanical scoring, not eyeballing: characters correct against the expected
  text, Randziffern found vs present, offers of proof converted, page numbers
  and letterheads leaked, citations left intact.
- A build tag or `-short` guard so it never runs in `task`, since it needs a
  loaded model and minutes of GPU time.
- One row per model, so `olmocr-2-7b`, `Qwen3.5-9B` and `gemma-4-E4B` can be
  compared on the same pages rather than by recollection.

Two traps worth writing down before starting:

- **A fixed seed means repeating a run cannot estimate variance.** Identical
  input gives identical output. Raise n by adding *pages*, not repeats. A
  two-page A/B once inverted between conditions and looked like a real effect;
  it was noise, and only thirty instances settled it.
- **Score against the source, not against plausibility.** At 200 dpi the model
  reproduced a typo that was genuinely in the PDF's text layer, and at 150 it
  silently corrected it. The corrected version reads better and is wrong: a
  transcription that fixes its source cannot be cited.

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
