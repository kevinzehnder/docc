# Legal output — pagination, heading numbering, and Randziffern

> **Status: implemented in the engine and in `testdata/`.** The remaining work
> is step 6, cutting `pi_assistant` over, which is held behind the reference
> brief that `docs/next-steps.md` task 3 already asks for. See **As built** at
> the foot of this document for what differed from the design below, which is
> kept as written for the reasoning.

Implementation concept for the legal `.docx` output.

Read `CLAUDE.md` and `docs/next-steps.md` before implementation. In particular,
`docc` remains a generic engine: legal conventions belong in the compiled
project's `.docc` files, never as `legal` special cases in Go.

## Objective

For a legal document rendered with `zbp-legal`:

1. The Markdown body starts on a fresh page immediately after the rendered
   **Betreff**.
2. Section headings have automatic outline numbering across three levels:
   `I.`, then `A.`, then `1.`.
3. Ordinary body paragraphs after `RECHTSBEGEHREN` have a continuous,
   small, left-margin **Randziffer** (`1.`, `2.`, ...).

The output must be a self-contained DOCX. The generated DOCX owns the
numbering definitions and paragraph references; it must not rely on `docc` or
on a template when opened in Word.

## Ownership boundary

| Concern | Owner |
|---|---|
| Legal document opts into each behaviour and selects a named definition | `../pi_assistant/.docc/schemas/legal.yaml` |
| Margins, fonts, label format, indents, and fixed furniture | `../pi_assistant/.docc/themes/zbp-legal.yaml` |
| Generic YAML loading and validation | `internal/schema`, `internal/theme` |
| Generic application of configured numbering to IR blocks | `internal/emit` |
| DOCX `numbering.xml` and paragraph `w:numPr` serialization | `internal/docx` |

The engine must not know the words `RECHTSBEGEHREN`, `Randziffer`, German, or
Swiss legal practice. The schema supplies the start-heading text and the theme
supplies the visual definitions.

## 1. Page break after Betreff

This needs **no new engine capability**. `theme.Line` already has
`page_break`, which writes `w:pageBreakBefore` on that furniture paragraph.

In `zbp-legal.yaml`, add a non-omitted, otherwise empty prologue line after the
`LegalBetreff` line. Give it a zero-spacing body-start style and `page_break:
true`. It starts a new page, so the first following Markdown block begins there.
For example (style name is illustrative):

```yaml
styles:
  LegalBodyStart:
    name: Legal body start
    based_on: Standard
    spacing: { before: 0mm, after: 0mm }

prologue:
  # ... existing LegalBetreff line ...
  - { style: LegalBodyStart, text: "", omit_if_empty: false, page_break: true }
```

Keep this in the theme: it is presentation/furniture, not a fact about what a
legal document is.

## 2. Automatic heading outline numbering

### Proposed declarative interface

Add an optional render-numbering section to `schema.Schema` and the YAML
schema format. Exact Go names may differ, but keep the public YAML narrow and
generic:

```yaml
render:
  heading_numbering:
    definition: LegalHeadingNumbering
    start_at_heading: RECHTSBEGEHREN
```

`definition` names an entry under the selected theme's `numbering:` map.
`start_at_heading` is the literal text of the first eligible Markdown heading.
It makes the scope explicit rather than accidentally numbering a title or other
introductory body content.

The legal theme defines the appearance and each level:

```yaml
numbering:
  LegalHeadingNumbering:
    format: upperRoman
    text: "%1."
    style: Ueberschrift1
    levels:
      - format: upperLetter
        text: "%2."
        style: Ueberschrift2
      - format: decimal
        text: "%3."
        style: Ueberschrift3
```

The existing theme and DOCX model already support `upperRoman`, `upperLetter`,
`decimal`, and nested levels. This is an extension of *where* numbering is
applied, not a new numbering format.

### Emitter behaviour

In `internal/emit`:

1. Track whether the configured start heading has been encountered.
2. For eligible `ir.Heading` blocks at Markdown levels 1–3, allocate one DOCX
   numbering instance lazily and attach its `docx.NumRef` to the paragraph.
   Use `Level: heading.Level - 1`.
3. Reuse that one `numId` for every eligible heading in the document. This is
   essential: creating a fresh instance per heading would restart at `I.`.
4. Preserve the schema's existing `h1`/`h2`/`h3` style mapping. The numbering
   definition supplies the label; the heading style supplies the heading text's
   formatting.
5. Do not write `I.`, `A.`, or `1.` as literal text. Word renders them from
   `word/numbering.xml` and each paragraph's `w:numPr` reference.

The normalised Markdown must use nesting to express the intended outline, for
example:

```markdown
# RECHTSBEGEHREN

# BEGRÜNDUNG

## Formelles

### Prozessgeschichte
```

Remove manually authored prefixes such as `# I. Formelles` and
`## 1. Prozessgeschichte`; otherwise Word-generated labels and source text will
be duplicated.

## 3. Continuous Randziffern

### Proposed declarative interface

Use a separate, optional paragraph-numbering configuration:

```yaml
render:
  paragraph_numbering:
    definition: Randziffer
    start_after_heading: RECHTSBEGEHREN
```

This deliberately differs from heading numbering: the first Randziffer applies
to the first ordinary prose paragraph *after* the marker heading, not to the
heading itself.

The legal theme supplies a conventional small label in the left margin. The
values below are a starting point to verify visually against a filed brief:

```yaml
numbering:
  Randziffer:
    format: decimal
    text: "%1."
    font: FreeSans
    size: 8pt
    align: right
    suffix: space
    indent: 0mm
    hanging: 7mm
    style: Standard
```

`size`, `align`, and `suffix` are not currently exposed by `theme.NumFormat`.
Add them generically to `theme.NumFormat`, convert them to `docx.NumLevel`, and
write them in `internal/docx` (`w:sz`, `w:lvlJc`, and `w:suff`). The label should be
small but baseline-aligned; do not make it superscript unless that is expressly
wanted.

### Emitter behaviour

After the marker heading, apply one shared Randziffer `numId` at level 0 to
ordinary prose `ir.Para` blocks. The counter must continue through subsequent
headings and sections; headings use their separate outline-numbering instance.

Do **not** apply Randziffern to:

- headings;
- Markdown list items, including the numbered Rechtsbegehren;
- fenced-div contents such as `::: beweis`;
- code blocks, tables, rules, or fixed theme furniture.

Make block context explicit in the emitter (for example, main body vs. list vs.
div) rather than applying numbering to every `ir.Para` reached recursively.
That prevents a Rechtsbegehren item or a Beweismittel entry from acquiring a
second label.

Remove the manually typed paragraph prefixes (`1.`, `2.`, ...) from source
prose once this is enabled, or they too will be doubled.

## DOCX representation

For each build, `docc` must emit:

- one abstract numbering definition and one `numId` instance for the heading
  outline;
- one abstract numbering definition and one `numId` instance for Randziffern;
- a `w:numPr` on each numbered heading or paragraph, identifying that `numId`
  and level;
- the generated definitions in `word/numbering.xml`.

This is normal Word numbering, not hard-coded text and not a dynamic dependency
on the renderer. Word can display, edit, and update the completed document on
its own.

## Implementation order

1. Add the page-break-only theme change and manually inspect the output.
2. Add generic schema config structs, YAML parsing, merging for `extends`, and
   validation that selected definitions exist in the chosen theme.
3. Extend generic `theme.NumFormat` / `docx.NumLevel` with label size,
   alignment, and suffix; add deterministic XML serialization.
4. Refactor emitter numbering allocation so headings and paragraphs can each
   keep one shared instance without pretending they are Markdown lists.
5. Implement start-marker and block-context handling; apply heading and
   Randziffer numbering as described above.
6. Update the real legal schema/theme and normalise the real legal Markdown.
7. Add tests, review generated XML, then run `task` and `task test:roundtrip`.

## Tests and acceptance checks

- Unit test theme conversion for three heading levels and Randziffer label
  properties.
- Unit test DOCX serialization for `w:numFmt`, `w:lvlText`, `w:sz`,
  `w:lvlJc`, `w:suff`, and `w:numPr`.
- Emitter test: H1/H2/H3 share one heading `numId` and use levels 0/1/2.
- Emitter test: prose paragraphs share one Randziffer `numId`, continue across
  headings, and do not affect list/div/table/code content.
- Emitter test: numbering starts only at/after the configured marker.
- Build a sanitised legal fixture, inspect `word/document.xml` and
  `word/numbering.xml`, and add deterministic DOCX goldens as described in
  `docs/next-steps.md`.
- Render with LibreOffice (`task test:roundtrip`) and inspect the result against
  an actual filed brief. Check page two starts at RECHTSBEGEHREN, the heading
  hierarchy displays `I. / A. / 1.`, and Randziffer labels sit in—not over—the
  left margin.

Never copy the real `pi_assistant/.docc` theme or client document into
`testdata`; use invented identities and fixtures.

## Decisions to confirm before implementation

1. Is `RECHTSBEGEHREN` itself `I. RECHTSBEGEHREN`, or does automatic outline
   numbering start only with a later heading such as `BEGRÜNDUNG`?
2. Does `BEGRÜNDUNG` receive the next Roman numeral, with its subsections at
   letter level and their subsections at Arabic level? The Markdown hierarchy
   must match this decision.
3. Are Randziffern required for quoted prose as well as direct body paragraphs?
   The default above is direct ordinary prose only.
4. Confirm the exact Randziffer geometry and font size from a reference brief;
   `8pt`, `7mm` hanging, and right alignment are intentionally provisional.

None of these blocked the engine: each is a value in a project's `.docc` files,
not a branch in Go. The corpus takes 1 and 2 as written above
(`start_at_heading: RECHTSBEGEHREN`, so `RECHTSBEGEHREN` is `I.` and
`BEGRÜNDUNG` is `II.`), 3 as its stated default, and carries 4's provisional
geometry with a comment saying so. Changing any of them is one line in
`.docc/schemas/legal.yaml` or `.docc/themes/`.

---

## As built

Where the implementation differs from the design above.

**Steps 1, 2, 3, 4, 5 and 7 are done.** Step 6 — updating the real legal schema
and theme and normalising the real briefs — is not, deliberately: enabling
render numbering while source documents still carry manually typed `I.` and `1.`
prefixes doubles every label, so it is one atomic change that wants the
reference brief from task 3 in front of it.

**`internal/docx` needed less than expected.** `NumLevel` already had `Align` and
`Suffix`, and `writeNumLevel` already wrote `w:lvlJc` and `w:suff`. Only `Size`
(`w:sz`/`w:szCs` inside the level's `w:rPr`) was new.

**A pre-existing bug had to be fixed first.** `theme.NumFormat.AbstractNum`
recursed into `levels:`, treating a flat list as a tree, so a definition with
two sub-levels emitted two levels both claiming `ilvl="1"`. Word resolved that
by rendering the second one's `%3` placeholder as the literal text `%3%.`. The
three-level heading outline is the first definition in the corpus deep enough to
show it; `example-letter`'s `Nummerierung` had been wrong the same way.
`Flatten()` is now the single definition of the level order, and
`emit.Validate` rejects a level that nests further or a definition past nine.

**The start marker is one generic mechanism, not two.** Both
`start_at_heading` and `start_after_heading` are available to either rule;
the first numbers the marker heading itself, the second begins after it.
Declaring both is an error rather than a precedence rule. Matching is on
normalised heading text, the same way the body checks match.

**Heading depth is bounded by the definition, not by the number three.** A
heading deeper than the definition has levels for gets no label rather than a
fabricated one, so a three-level outline over a document with an `####` does
not produce an invalid `ilvl`.

**Block context is a separate top-level walk.** `emitter.body` is the only
place render numbering is applied, and it only ever sees top-level blocks —
everything nested is produced inside `blockTo` and never reaches it. That is
what keeps a marginal number off a Rechtsbegehren item, a Beweismittel entry, a
table cell and a quotation, without a rule enumerating those cases.

Tests: `TestNumFormatLevelsAreFlat`, `TestNumFormatCapsAtNineLevels`,
`TestNumFormatLabelProperties` (theme); `TestNumLevelLabelProperties`,
`TestParagraphNumberingReference` (docx); `TestHeadingNumberingSharesOneInstance`,
`TestHeadingNumberingStartsAtMarker`, `TestParagraphNumberingStartsAfterMarker`,
`TestParagraphNumberingSkipsNestedContent`, `TestNoRenderNumberingLeavesDocumentAlone`
and the four `TestValidate*` cases (emit), plus the `.docx` goldens under
`testdata/golden/legal_valid/`.
