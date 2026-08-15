# Building a docc profile from an existing Word document

A *profile* is a schema plus a theme: the contract of a document type and its
visual definition. Most profiles do not start from a blank page — they start
from a `.docx` the office already uses, usually a template full of grey
tab-blanks, `ODER:` alternatives and sections marked "delete if not
applicable". This guide is the working method for turning such a document into
a profile, written for an agent (or a patient human) with a shell, the `docc`
binary, LibreOffice and a way to look at rendered pages.

The method was test-driven on a notarial purchase deed
(`kaufvertrag_beispiel.docx` → `.docc/schemas/ch_urkunde_kaufvertrag.yaml` +
`.docc/themes/ch_urkunde_kaufvertrag.yaml`); examples below come from that run.

## The loop

```
unpack the .docx ──► inventory structure ──► classify the template's blanks
        │                                            │
        ▼                                            ▼
render original to PNG                    write schema + example
  (ground truth)                                     │
        ▲                                            ▼
        │                                      write theme
        └── visually compare ◄── render your build to PNG ◄── docc build
                 │
                 └── iterate: fix theme/schema, or fix the engine
```

Two renders of the *same pipeline* (LibreOffice → PDF → PNG) are compared, so
renderer quirks cancel out. Do not compare a Word screenshot to a soffice
render and chase differences that are LibreOffice's, not yours.

## 1. Unpack and render the original

```bash
mkdir -p /tmp/profile && cd /tmp/profile
unzip -q original.docx -d src
soffice --headless --convert-to pdf original.docx --outdir .
pdftoppm -png -r 60 original.pdf orig       # orig-01.png, orig-02.png, ...
```

The PNGs are the ground truth. Look at every page before writing a line of
YAML: the first page usually has furniture (letterhead, crest, title block)
the body pages do not, and the last pages usually hold signatures and
certification text that will stay in the markdown body.

## 2. Inventory the structure

`word/document.xml` is too big to read raw. Dump one line per paragraph:
style, numbering reference, run formatting, first 150 characters. A short
Python script over `document.xml` does it (ElementTree, iterate `w:p` /
`w:tbl` under `w:body`; for each paragraph read `w:pStyle`, `w:numPr`, and
per-run bold/italic/underline).

From the dump and the PNGs, write down:

- **Heading hierarchy and numbering.** Which styles are headings, what labels
  they carry (`I.` / `1.` / `1.1.`), where numbering starts, whether lower
  levels restart under higher ones. In the deed: h1 upper Roman, h2 decimal
  restarting per h1, h3 `x.y.` — all generated, so in docc they become a
  `render.heading_numbering` definition and the markdown source stays free of
  typed numbers.
- **Page geometry.** `w:pgSz` / `w:pgMar` in the `sectPr` elements, converted
  from twips (÷ 56.7 ≈ mm). Multiple sections usually mean a distinct first
  page — in docc that is `title_page: true` plus `header: first:`.
- **Base typography.** `word/styles.xml` → `docDefaults` and the named styles:
  font, sizes (half-points), small caps, alignment. The deed: 10pt body
  justified, 16pt bold centered title, small-caps headings.
- **Headers, footers, images.** `word/header*.xml`, `word/media/*`. Extract
  reusable assets (a crest, a logo) — they go next to the theme file and are
  referenced with `image: { path: ... }`. Check the rendered pages for page
  numbers and other field codes.
- **Repeated visual patterns.** Anything that appears many times with the
  same shape is a candidate semantic block: evidence lists, amount lines with
  a right-aligned CHF column, party paragraphs, certification clauses.

## 3. Classify the template's blanks — the crucial step

The old template works by *deletion and filling*: every deed starts as all
variants plus blanks, and the notary deletes what does not apply. A docc
source works by *authoring*: the markdown contains what the deed says,
nothing else. So every grey blank and every `ODER:` in the template must land
in exactly one of four places:

| Template artifact | Where it goes | Mechanism |
|---|---|---|
| Facts the fixed furniture prints (notary name, title, letterhead fields) | frontmatter | theme interpolates `{{ field }}` in prologue/epilogue/header |
| Facts of the individual document (parties, object, price, dates) | authored body text | validated via `blocks:` / `spans:` declarations |
| `ODER:` alternatives, "delete if not applicable" sections | nowhere — the author writes the applicable wording | variants of a semantic block where structure matters; otherwise just prose |
| Blanks filled in by hand on paper (Beurkundungsdatum, Protokoll-Nr.) | body, as visible blanks | `[____]{.docc-field key=...}` + `fields:` with `completion: handwritten` |

Rules of thumb:

- Frontmatter is for what the *theme* needs to render furniture, and for
  document-level facts worth machine-reading later. Do not hoist body prose
  into frontmatter just because the template had a blank there — the deed's
  parties belong in the deed's text.
- A blank that must be filled *before* the document is final gets
  `completion: before-execution` (the default): `docc check` accepts the
  draft, `docc build` refuses while it is blank. A blank completed with a pen
  gets `completion: handwritten` and survives into the rendered page.
- If the template's alternatives differ in *required structure* (a party that
  is a person needs name/birth date/place of origin; a company needs
  name/UID/seat), model them as one block with a `discriminator:` and
  `variants:`, each variant naming its `required_spans`. The author picks the
  variant with one attribute and the checker enforces the rest.

## 4. Write the schema

Order of work inside the YAML:

1. `frontmatter:` + `types:` — only what step 3 put there.
2. `body:` — the heading skeleton with `required:` / `ordered:` /
   `required_when:`. Only structure whose *absence is a defect*; a template
   heading the office sometimes omits is `required: false` or absent.
3. `blocks:` / `spans:` — the semantic markup from step 3. Declaring any
   blocks makes undeclared `:::name` blocks an error (same for spans), so
   declare everything the type permits in one sitting.
4. `fields:` — the handwritten/deferred blanks.
5. `styles:` — the markdown-construct → style-name map. Invent honest names
   now (`Vertragspartei`, `Betragszeile`); the theme defines them next.
   The set of keys is closed; the README's *style map* table lists all of
   them, and `docc doctor` warns about a mapping nothing reads.

   For a block, **which key you map selects a rendering pattern**. The block's
   own declaration says nothing about this, so it is worth deciding
   deliberately:

   | Mapped | Pattern |
   |---|---|
   | `div.<name>` only | plain — every paragraph in that style |
   | `div.<name>.label` | labelled — `- [LABEL] description` comes out as description, tab, label at a tab stop. Evidence references. |
   | `div.<name>.amount` | amount — a right-aligned amount column; `.total` and `.total.amount` style the total row, `.words` the spelled-out sum (needs the theme's `formats.amount_words`) |
   | `div.<name>.line` | ruled — a signature or entry line |

   They are tried `.amount`, `.line`, `.label`; mapping two takes the first
   silently. `docc describe <type>` prints the pattern each block resolved to.
6. `render:` — heading/paragraph numbering, referencing definitions the theme
   will declare.
7. `rules:` — pick from the registry (`no_placeholder_text`,
   `div_items_match`, `cross_reference`, `no_empty_sections`) with
   schema-owned codes. `div_items_match` with a pattern is the cheap way to
   enforce a per-line shape inside a block, e.g. every Betragszeile starts
   with `[CHF ...]`.
8. `example:` — a compact but *complete* document. Write it against the real
   template content, scrubbed. This is the profile's spec: it exercises every
   block, span, field and furniture line you declared.

## 5. Write the theme

Copy the structurally closest existing theme and edit against the
measurements from step 2 — page, margins, `title_page`, defaults, every style
the schema maps, the numbering definitions the schema names, header/footer,
prologue/epilogue. Notes that bite:

- Styles referenced as `div.<name>` render every paragraph of that block;
  the char style behind `div.<name>.label` styles the tabbed label. Give the
  block style the tab stop (`tabs: [{ pos: ..., align: right }]`). The label
  only lands at the tab stop if the description fits before it — a
  description that wraps past the stop swallows the tab and the label runs
  on inline. Keep those descriptions to one line, and say so in the block's
  `description:` so the author is told rather than surprised.
- Some constructs never reach the theme at all: `**bold**`, `*italic*`,
  inline `` `code` `` (always Courier New), links (always `0000EE` and
  underlined, and rendered as text rather than a live hyperlink), and table
  borders and column widths (a 0.5pt grid, columns split evenly). Do not
  spend an afternoon on a style for one of these — the README lists them
  under *What a theme cannot change*, and `docc doctor` says so if a schema
  maps one.
- Left-align styles for content with manual line breaks or short lines
  (party entries, address blocks) even when the body justifies — Word
  stretches justified lines that end in a manual break.
- Numbering `levels:` is a flat list, not a tree: the definition is level 0,
  `levels[0]` is level 1. Label text like `"%2."` shows only the current
  level's count; `"%2.%3."` composes.
- Interpolated fields must exist in the schema; `docc build` validates the
  pair and refuses on typos, so build early and often.

## 6. Iterate visually

```bash
docc example ch_urkunde_kaufvertrag > /tmp/kv.md
docc check /tmp/kv.md
docc build --output /tmp/kv.docx /tmp/kv.md
soffice --headless --convert-to pdf /tmp/kv.docx --outdir /tmp
pdftoppm -png -r 60 /tmp/kv.pdf mine
```

Put `mine-1.png` next to `orig-01.png` and compare *deliberately*, page by
page: geometry first (margins, where the body starts), then blocks (is every
piece of furniture present, in order), then typography (sizes, caps,
weights), then detail (tab positions, rules, spacing). Fix the theme, rebuild,
re-render. Convergence is quick because each pass has a concrete diff.

Expect to hit engine limits; sort each into one of three bins:

- **Your YAML is wrong.** Most differences. Fix and loop.
- **The engine has a bug.** The deed run found one: images in headers
  rendered blank because the writer emitted no per-part `.rels` for header
  parts (OPC scopes relationships to the part; `r:embed` in `header1.xml`
  cannot resolve against `document.xml.rels`). Diagnose by unzipping *your
  built* `.docx` and reading the parts — the golden corpus and
  `testdata/golden/` show what correct output looks like. Fix in
  `internal/docx`, add a unit test, run `task`.
- **The feature does not exist.** Also found in the deed run: there is no
  dynamic `PAGE` field, so running page numbers cannot be produced yet.
  Decide whether to live without it, emulate it, or grow the engine. Record
  the decision as a comment at the top of the theme — a theme that silently
  lacks page numbers looks like an oversight to the next reader.

Stop iterating at *faithful*, not *pixel-identical*. The profile replaces the
template; it does not have to reproduce its accidents (Word's default fonts,
inherited spacing quirks, the highlighted placeholders themselves).

## 7. Finish

- `docc describe <type>` — read the contract as an author will see it; fix
  descriptions and hints that read poorly.
- `docc example <type> | docc check` must be clean, and the example must
  build; keep both true forever, they are the profile's regression test.
- Author one real document with the new profile before calling it done. The
  example is friendly; a real deed finds the block you forgot to declare and
  the hint that misleads.
- Commit schema, theme and assets together, with the source template's name
  in the schema's header comment so the lineage is findable.

## Worked example

The `ch_urkunde_kaufvertrag` profile is the reference run of this guide:
`.docc/schemas/ch_urkunde_kaufvertrag.yaml` (party block with four variants,
grundstueck and betraege blocks, two handwritten fields, amount-shape rule)
and `.docc/themes/ch_urkunde_kaufvertrag.yaml` (crest in a first-page header,
centered Urkunde title block with bordered rules, small-caps outline
I./1./1.1., right-tab amount column). Every decision in this guide is made
once there, concretely. The Word template it came from is not kept in the
repository — only the crest extracted from it,
`.docc/themes/urkunde-wappen.png`, and the schema's header comment recording
the lineage.
