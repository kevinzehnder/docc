# Ingest: what we measured, and what to build next

Findings from bringing `docc ingest` to a working state against a local VLM, and
the plan for the structural work that follows. Everything under **Measured** was
established empirically on the development machine — treat it as given rather
than re-deriving it, since re-deriving costs GPU hours.

Branch: `feat/ingest-progress`.

## Where the project stands

`docc ingest` turns a PDF or image into a markdown draft via a locally hosted
vision model over an OpenAI-compatible API (llama.cpp `llama-server` behind its
model router). It is deliberately schema-agnostic: it transcribes, and adapting a
draft to a document type is a later pass. That separation is stated in the README
and is load-bearing for the plan below.

Shipped:

- SSE streaming, so progress reflects real generation rather than a local timer
- A stall watchdog (silence between chunks) in place of a whole-request deadline
- Preflight against `/health` and `/v1/models` before rasterizing
- Partial save: an interrupted or failed run keeps its transcribed pages, marked
- Output guards against overwriting edited drafts or partial drafts
- A fixed `seed`, making runs reproducible
- `rzNormalizer`: marginal paragraph numbers marked in code, not by the model

## Measured

### The vision projector belongs on the GPU

The single largest factor, by orders of magnitude. With `mmproj-offload = false`
the projector encodes each page image on the CPU: pages took minutes, and one run
killed the server outright. On the GPU the same page encodes in about a second.

On 8 GB the constraint is fitting weights + projector + KV cache + desktop.
Context is what pays for the projector: dropping `ctx-size` from 32768 to 12288
freed roughly 590 MiB, which is what let the projector move. A page needs image
tokens + anchor text + response — nowhere near 32k.

### DPI is a fidelity setting, not a speed setting

| DPI | run 1 | run 2 | outcome |
|-----|-------|-------|---------|
| 110 | 11.0s | 10.9s | identical output to 150 |
| 150 | 11.4s | 11.3s | current setting |
| 200 | 12.0s | 11.9s | reproduced a source typo the others corrected |

Speed is nearly flat: with the projector on the GPU a page is dominated by
generating ~800 tokens, not by encoding the image.

**Correction to an earlier conclusion.** The one textual difference between 150
and 200 dpi was a missing `er` in a long German compound. That was first read as a
200 dpi OCR error. The PDF's own text layer contains the shorter spelling — the
author's typo — so 200 dpi *reproduced the source* and 150 silently corrected it.
For a document you intend to cite, faithful beats plausible. If silent
normalisation recurs, raise DPI; 0.6s per page is nothing against a wrong
quotation.

### The model transcribes reliably and formats unreliably

Eight pages of a scanned brief, fixed seed, same model. Task: mark each marginal
paragraph number as `[Rz N]`.

| condition | marked | left bare | captured | rate | page numbers leaked |
|-----------|--------|-----------|----------|------|---------------------|
| original prompt | 19 | 7 | 26 | 73% | 5 |
| position-first rules | 19 | 11 | 30 | 63% | 2 |
| position-first + code | 30 | 0 | 30 | 100% | 2 |

The number is transcribed almost always; the transformation is applied about two
thirds of the time. **Move mechanical transformations into Go.** `rzNormalizer`
does this, guarded by sequence continuity — a leading number is only marked when
it continues the count, which is what stops a year becoming `[Rz 2010]`.

### Two prompt rules were fighting

A 60-page run produced *zero* markers. The prompt asked both to mark a bare
marginal number and to drop "a standalone page number printed by itself", and a
lone number in the left margin matches the second description as well as the
first. The exclusion won.

Rules for a vision model should discriminate **by position on the page** (left
margin / page edge / inside a sentence) and state explicitly which wins where they
overlap.

### A fixed seed means repetition cannot measure variance

With `seed` fixed, re-running identical input gives identical output. A two-page
A/B inverted between conditions and looked like a prompt effect; it was noise.
Raise n by adding **pages**, not repeats. Thirty instances made the difference
real.

### Preflight cannot see through the router

The router answers `/health` itself, so preflight passes while no model is loaded;
the first request then triggers the load, and that load is silent time charged
against the stall watchdog. Hence a generous `stall_timeout` locally.

Distinguishing a stalled server from a user's Ctrl-C requires
`context.WithCancelCause` — both surface as `context.Canceled` otherwise.

### Scale

A full 60-page run: 14m31s, ~39k tokens, zero truncations, zero low-confidence
pages, no repetition loops, no leaked headers, peak VRAM 7784/8192 held for the
duration. Stable at production length.

## The open problem: two kinds of Randziffer

Swiss briefs number each paragraph in the margin, and those numbers are how
documents cite each other. Two cases, which must not share a mechanism:

| authored document | reference document (third party) |
|---|---|
| Number is **presentation**, derived from position | Number is **content and identity** — the citation key |
| Generated as Word numbering via `render.paragraph_numbering` | Comes from the source; cannot be generated |
| **Must** renumber when sections move | **Must never** renumber |
| Typed-in numbers are forbidden (README) | Typed-in numbers are the whole point |

**Why this is urgent, not cosmetic.** Ingest can drop a page — observed. If a
reference document's numbers were generated, a dropped page would silently
renumber everything after it, and a filed brief would cite `KA Rz 47` pointing at
different text than opposing counsel wrote. Wrong, and invisible.

**Known inconsistency to fix.** `rzNormalizer` currently marks Randziffern
*unconditionally*. For a PDF being ingested to become one of our own documents, a
typed `[Rz N]` is exactly what the README forbids. Only correct once ingest knows
which kind of document it is producing — step 3.

## Plan

Order matters. Ingest and any structuring pass are both *producers*; neither can
target a representation that does not exist yet. Steps 1–2 are ordinary compiler
work with tests, and pay off independently of anything involving a model.

### Step 1 — a reference document kind

A schema type (e.g. `legal_reference`) that omits `render.paragraph_numbering` and
permits Randziffern in source. Settle the syntax deliberately.

**Parser trap:** `[Rz 7]` is a markdown shortcut link reference. goldmark emits it
as literal text when nothing matches, so it renders — but it lands in the
**inline** layer, and `Lines()` panics on inline nodes (see CLAUDE.md). Do not
inherit this syntax by default just because ingest emits it today.

*Done when:* a fixture reference document round-trips through `docc check` with its
numbers intact and no `DOC011`-style noise.

### Step 2 — parse the marker, then check it

Once it is a node rather than prose, `sema` can verify sequence integrity: gaps,
duplicates, non-monotonic runs. A reference document jumping 12 → 47 means ingest
lost a page — a diagnostic for a failure we have actually observed.

*Done when:* a new diagnostic code fires on a fixture with a deliberately damaged
sequence, with a source position and a hint, documented in the README table and in
`docc explain`.

### Step 3 — make ingest kind-aware

`--type` already exists. When the target is a reference type, emit markers;
otherwise strip them, because generated numbering will supply them. Fixes the
inconsistency noted above.

*Done when:* the same PDF ingested as a reference type and as an authored type
differs exactly in the presence of the markers.

### Step 4 — the structuring pass

A second pass over the *ingested text*, not the image, mapping prose into schema
structure — starting with evidence blocks. The destination already exists:
`::: beweis` fenced divs are parsed, and the legal schema already carries rules for
the bracketed `[Beilage N]` label and a cross-reference into the `beilagen` list
field. Only the producer is missing.

Why a separate pass:

- No image encode, so it is cheap and fast to re-run
- Re-runnable when the prompt changes, without re-OCRing
- Diffable against the previous attempt
- `docc check` can sit in the loop as a real oracle — a one-shot pass has none
- Asking one call to transcribe *and* fit a schema inherits the 63–73%
  reliability measured above, invisibly

*Done when:* a structured draft validates against its schema, and the loop is
reproducible from a committed ingested-text fixture without a GPU.

## Conventions and traps

- `task` runs the full CI chain — gofumpt, vet, golangci-lint, tests, build. Run
  it before committing.
- **No new dependencies.** `go.mod` has exactly goldmark and goccy/go-yaml.
  Spinners, TTY detection and SSE parsing are hand-rolled for this reason.
- `errcheck` flags unchecked `fmt.Fprintf` to an `io.Writer` field (though not to
  `os.Stderr` directly). The progress renderer routes writes through a small
  helper that drops the error deliberately.
- `LoadConfig` uses `yaml.Strict()` — an unknown key is a hard error, so every
  config field must be declared.
- Golden tests cover `check`/`build`, not ingest; ingest has ordinary Go tests.
  Changing a message is *expected* to fail `TestGolden` — read the diff before
  regenerating.
- Diagnostics need a source position and a hint. Codes are stable, never
  renumbered.
- Progress goes to **stderr**; stdout is the machine channel and carries only the
  output path (or JSON).
- Tests needing `pdftoppm` skip when it is absent — follow that pattern.

## Verification

Local server and machine config live outside the repo; committed defaults are
conservative.

```
task                       # full CI chain

# progress, guards and partial save
docc ingest --pages 1-3 <doc.pdf>              # watch the status line
docc ingest --pages 1-3 <doc.pdf> 2>&1 | cat   # plain mode, one line per page
docc ingest --json --pages 1-3 <doc.pdf>       # silent stderr, one JSON object
docc ingest --pages 1-20 <doc.pdf>             # Ctrl-C -> partial .md + resume hint, exit 1
docc ingest --endpoint http://localhost:9/v1/chat/completions <doc.pdf>   # fails in ~3s

# the handoff nothing used to exercise
docc check --schema-dir testdata/schemas --type legal <draft.md>
```

For prompt experiments: fix the seed, vary **pages** to raise n, and score
mechanically (markers found, bare numbers left, page numbers leaked, citations
intact) rather than by reading a sample.

---

Measurements taken on an RTX 3070 (8 GB) with a 7B OCR-specialised VLM and a 9B
general VLM, both Q4_K_M with the projector on the GPU. Machine configuration and
document fixtures are intentionally not in the repository.
