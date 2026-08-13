# docc

A compiler for structured documents.

`docc` treats a markdown document with YAML frontmatter as source code: it parses
it, checks it against a schema, and reports errors with source positions and
actionable hints. The intended backend emits Word `.docx` from a Word-authored
template.

A file becomes a docc document by declaring the marker in its frontmatter:

```yaml
---
docc: 1          # the docc format version — this is what makes it a docc file
document_type: legal
---
```

Files without the `docc` marker — READMEs, notes, Hugo or Obsidian posts with
their own frontmatter — are not docc documents. `docc check` reports `DOC024`
for them, and the language server stays silent.

The point is not the file format. The point is that a prose style guide is a
contract nothing enforces, while a schema is a contract that fails loudly at a
specific line and column.

```
docs/klage_mueller.md:14:13: error[DOC010]: field `case_ref` has malformed value "ZG2026000"
   |
14 |   case_ref: "ZG2026000"
   |             ^^^^^^^^^^^ court reference in the form AA.YYYY.NNN, e.g. "ZG.2026.000"
```

## Install

```bash
go install github.com/kevinzehnder/docc/cmd/docc@latest
```

Or build locally:

```bash
task build      # → ./bin/docc
```

## Usage

```bash
docc init                         # create the generic starter in this directory
docc check docs/klage.md          # validate
docc check --json docs/*.md       # machine-readable, for agents and CI
docc check --strict docs/klage.md # warnings become errors
docc lsp                          # serve editor diagnostics over stdio
docc types                        # list known document types
docc explain DOC010               # describe a diagnostic code
docc ingest scan.pdf              # draft markdown from a PDF via a local VLM
```

Exit codes: `0` clean, `1` diagnostics reported, `2` usage or configuration error.

## NeoVim

`docc lsp` is a dependency-free Language Server Protocol server. It publishes
live diagnostics for Markdown documents using the nearest `.docc/schemas`
directory; `--schema-dir` and `--type` have the same meaning as for `check`.

With NeoVim's built-in LSP client, add this to your `init.lua`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = "markdown",
  callback = function(args)
    vim.lsp.start({
      name = "docc",
      cmd = { "docc", "lsp" },
      root_dir = vim.fs.root(args.file, { ".docc" }) or vim.fn.getcwd(),
    })
  end,
})
```

The server uses full-document synchronization and reports UTF-16-correct
ranges, including in documents containing non-ASCII text. It currently
provides diagnostics only; completion and code actions remain editor features
for a future release.

Only files whose frontmatter declares the `docc` marker are checked. Plain
markdown and files with unrelated YAML frontmatter get no diagnostics, so
editing regular `.md` files next to docc documents is quiet.

## Projects

`docc` is the engine. The schemas, Word templates and house style belong to the
project being compiled, in a `.docc` directory that `docc` finds by walking up
from the input file the way `git` finds `.git`:

```
myproject/
  .docc/
    schemas/
      _base.yaml        # shared field shapes, extended by the rest
      legal.yaml
      letter.yaml
    templates/
      legal.dotx        # authored in Word, not hand-written XML
  docs/
    klage_mueller.md
```

Override the location with `--schema-dir`.

### Starting a new project

`docc init [directory]` creates `.docc/` with a generic letter and a
Swiss-legal starter schema/theme, plus compiling examples in
`examples/docc/`. It never overwrites an existing starter configuration,
example directory, or installed skill.

```bash
mkdir my-documents && cd my-documents
docc init
docc check examples/docc/letter.md
docc build examples/docc/letter.md
```

The starter is deliberately generic. Replace the legal theme's `YOUR …`
letterhead values, then adapt its schemas and themes to the organisation's
actual conventions before using it for production documents.

### Agent skill

No LLM is required to use `docc`. For agents, this repository ships a portable
[Agent Skill](skills/docc/SKILL.md) that describes the validation-and-build
workflow. `docc init` also installs it at `.agents/skills/docc/SKILL.md`, which
Pi discovers in a trusted project. Other harnesses can copy that directory or
load the skill file directly; the project's `.docc` configuration remains the
authoritative contract.

This split means changing a letterhead is a file edit, not a compiler release,
and one engine serves projects whose document conventions have nothing in common.

## Schemas

A schema declares frontmatter fields and their types, the body structure, the
markdown-to-Word-style mapping, and which named rules to run.

The `docc` marker is declared in the base schema (`_base.yaml`) but validated by
the compiler before any schema field is checked, so it never appears as an
unknown-field warning even in projects whose schemas do not extend the base.

```yaml
type: legal
extends: base
description: Formal legal brief.
template: templates/legal.dotx

frontmatter:
  case_ref:
    type: string
    required: true
    pattern: '^[A-Z]{2}\.\d{4}\.\d+$'
    hint: 'court reference in the form AA.YYYY.NNN, e.g. "ZG.2026.000"'
  beklagter_vertreter:
    type: string
    required: true
    nullable: true          # `~` is a real answer: no legal representative
    hint: 'set to ~ when the opposing party has no legal representative'

body:
  - heading: RECHTSBEGEHREN
    level: 1
    required: true
  - heading: BEGRÜNDUNG
    level: 1
    required: true
    children:
      - heading: Zuständigkeit
        level: 2
        required_when: 'legal_doc_type == "Klageschrift"'

styles:
  h1: Ueberschrift1
  ordered_list: Rechtsbegehren
  "div.beweis": Beweismittel

rules:
  - id: LEG031
    check: no_placeholder_text
  - id: LEG012
    check: div_items_match
    args:
      div: beweis
      pattern: '^\s*\[[^\]\r\n]+\]\s+\S'
    message: "Beweismittel without a bracketed label"
    hint: 'prefix it with a label, e.g. "[Beilage 3]"'
  - id: LEG020
    check: cross_reference
    severity: warning
    args:
      div: beweis
      pattern: '(?i)^\s*\[Beilage\s+(\d+)\]'
      list_field: beilagen
      label: Beilage
```

### Evidence blocks

An evidence item in a `::: beweis` block starts with a bracketed, free-form
label followed by its description. The label records the kind of proof and is
preserved separately from the prose for themes that choose to style it:

```markdown
::: beweis

- [Beilage 1] Anwaltsvollmacht vom 4. August 2025
- [Klagebeilage 3] Eingabe der Gegenpartei vom 12. Mai 2025
- [Actorum 33] Protokoll der Einigungsverhandlung
- [Zeugenbefragung] Max Muster, Musterstrasse 1, 8000 Zürich
- [Von der Klägerin zu edieren] Buchhaltungsunterlagen 2022–2024
:::
```

Labels are intentionally open: a lawyer may use the procedural term that fits
the proof. Only `[Beilage N]` has special semantics: `N` is checked against the
positional `beilagen` list in the frontmatter. Labels such as `Klagebeilage`,
`Actorum`, `Augenschein`, or `Zeugenbefragung` remain valid but do not claim a
locally filed attachment. A closing `:::` must be on a line of its own.

### Field types

`string`, `int`, `bool`, `date` (ISO `YYYY-MM-DD`), `enum` (with `values`), `any`,
`list<T>`, or the name of an entry in the schema's `types:` block.

`required: true` demands a value. Adding `nullable: true` accepts an explicit
`~` as a deliberate "known to be absent", which is not the same as forgetting
the field.

### Body rules

Body rules lean towards warnings on purpose. Real documents vary in shape, and a
compiler that refuses to build a valid brief because it lacks a conventional
heading is worse than no compiler. Reserve `required: true` for structure whose
absence is genuinely a defect.

### Named rules

Declarative constraints cover most of a contract. Cross-references between
frontmatter and body do not, so those are Go functions a schema selects by name:

The checks are generic; what they mean is configured per schema through `args`.
A check that names a missing or malformed argument reports `DOC009` against the
schema rather than quietly doing nothing.

| check | what it catches | args |
|---|---|---|
| `no_placeholder_text` | template text that was never filled in | `pattern` — what a placeholder looks like. Defaults to a line that is nothing but bracketed prose, `[like this]`. |
| `div_items_match` | items in a fenced div that do not have the required form | `div` — the fence name, `pattern` — a regexp every item must match |
| `cross_reference` | keys cited in the body but missing from a frontmatter list, and entries listed but never cited | `div`, `pattern` — capture group 1 is the cited key, `list_field` — the frontmatter list, `label` — what one entry is called in messages |
| `no_empty_sections` | a heading with no content beneath it | — |
| `randziffer_sequence` | a transcribed document whose paragraph numbers skip, repeat or go backwards | `pattern` — what a marker looks like. Defaults to `[Rz N]` at the start of a paragraph. |

`cross_reference` numbers a list positionally: the Nth entry of `list_field` is
key N.

Where a `pattern` locates a diagnostic, capture group 1 is what the caret
underlines, so a pattern can match more context than it points at.

Each rule supplies its own diagnostic code, and may override the message,
severity and hint.

### Render numbering

Some numbering is a fact about the document type rather than something the
markdown should carry. A brief has an `I. / A. / 1.` section outline and a
running marginal number on each paragraph; nobody should be typing those, and a
document that has them typed in cannot be reordered.

```yaml
render:
  heading_numbering:
    definition: LegalHeadingNumbering
    start_at_heading: RECHTSBEGEHREN
  paragraph_numbering:
    definition: Randziffer
    start_after_heading: RECHTSBEGEHREN
```

`definition` names an entry in the theme's `numbering:` map, which is where the
appearance lives. Headings take their level from the markdown depth: `#` is the
definition's level 0, `##` its level 1. Paragraph numbering is a single level
and runs continuously, across the headings between paragraphs.

`start_at_heading` numbers that heading itself; `start_after_heading` leaves it
unnumbered and begins with what follows — which is what a marginal number wants,
since the count belongs to the prose and not to the heading above it. Set
neither and the rule covers the whole body. Setting both is an error.

Only top-level blocks are numbered. A list item, a table cell, a quotation and
the contents of a fenced div are all paragraphs, and none of them are body
prose — a Rechtsbegehren already carries its own number.

The labels are written as Word numbering, not as text. The document renumbers
itself when a section moves, and does so without `docc`.

## Ingest

`docc ingest` drafts a markdown document from a PDF or image, using a locally
hosted vision language model reached over an OpenAI-compatible chat
completions API — the interface a `llama.cpp` `llama-server` instance
exposes. It exists for the reverse direction docc otherwise has no answer
for: an existing document that needs to become editable markdown.

```bash
llama-server -hf ggml-org/Qwen3-VL-8B-Instruct-GGUF   # or any OpenAI-compatible VLM endpoint
docc ingest --pages 1-3 klage_mueller.pdf --model qwen3-vl  # try a few pages first
docc ingest klage_mueller.pdf --model qwen3-vl
```

It does one job — turn a PDF or image into faithful, clean markdown — and
stops there. It has no schema awareness: it does not validate, does not fit
the result to a document type, and does not run `docc check`. `--type` only
writes a `document_type` line into the output frontmatter as a convenience;
fitting the draft to a schema's actual fields and structure is a later
editing pass (by a person, or an agent), not ingest's job. Once that pass is
done, `docc check klage_mueller.md` validates the result the same way it
would for anything hand-authored.

Inputs are `.pdf`, `.png`, `.jpg`, `.jpeg`, `.webp` or `.gif`; several may be
given at once, each producing its own `.md`. Anything else is rejected before
a single page is converted — `docc ingest scan.pdf out.md` reads `out.md` as
a second input, and the destination is `--output`.

### Backends

`--backend` selects how a page becomes text. Both talk to the same
OpenAI-compatible endpoint; they differ in how many calls a page costs and in
where the document's structure comes from.

| backend | calls per page | structure from |
| --- | --- | --- |
| `chat` (default) | 1 | the model's own choice of markdown |
| `mineru` | 1 + one per block | a layout pass that types every region |

`chat` sends the whole page image with a prompt describing the document's
conventions, and takes markdown back. It is what a general-purpose VLM does
well, and it is the default because it is what every model here has been
scored against.

`mineru` speaks [MinerU2.5](https://github.com/opendatalab/MinerU)'s two-pass
protocol: one call locates and classifies every block on the page, then each
block is read from a crop at the page's own resolution. That buys three things
a prompt only asks for. A heading is marked because the layout pass called it a
heading. The running header and the page number are dropped because they came
back typed `header` and `page_number`. And a number alone in the gutter becomes
a `[Rz N]` marker because it arrived as its own block with its own coordinates.

It needs a MinerU2.5 checkpoint — 1.2B, so the weights and the projector
together are smaller than a 7B model's weights alone:

```bash
llama-server -hf mradermacher/MinerU2.5-Pro-2605-1.2B-GGUF:Q4_K_M
docc ingest --backend mineru scan.pdf
```

Prefer a Pro build. The older `MinerU2.5-2509` reads its crops measurably
worse — eight dropped words against one on the same fixture — and its
projector has to be passed with `--mmproj` by hand, because the file in that
repo is named `.mmproj` rather than the `mmproj-*.gguf` llama.cpp looks for.

Two limits worth knowing before choosing it. `anchor` does nothing here — the
protocol's prompt is the task name and nothing else, so there is nowhere to put
a text layer, and the run says so at preflight. And every heading comes out at
one level: the layout pass reports *that* a block is a heading, not how deep,
and inferring depth from the box would guess wrong on the first document that
sets a heading in a larger font for emphasis.

### Structuring an ingested draft

`docc structure` is a second pass over a draft ingest already produced. It finds
the offers of proof — the `BO:` blocks a Swiss brief puts under a paragraph —
and rewrites them into `::: beweis` fenced divs with the bracketed labels the
schema requires:

```
BO: Schreiben der Beklagten an die Klägerin vom 20.09.2015 Beilage 7
Christian Magnani, Mitglied der Geschäftsleitung Klägerin Parteibefragung
```

becomes

```markdown
::: beweis

- [Beilage 7] Schreiben der Beklagten an die Klägerin vom 20.09.2015
- [Parteibefragung] Christian Magnani, Mitglied der Geschäftsleitung Klägerin
:::
```

It is a separate command, not a stage of `ingest`, because it works on text.
There is no image to encode, so it costs seconds rather than minutes, it can be
re-run after a prompt change without re-converting the PDF, and its output can be
diffed against the previous attempt and validated with `docc check`.

Where a block starts is unambiguous; where it ends is not, so a block runs until
the next numbered paragraph, heading or fence. The model's answer is accepted
only when every line it returns is a labelled item — a block that comes back in
any other shape is left exactly as transcribed and reported by line number, since
a half-converted offer of proof is worth less than the transcription it replaced.
Running the command twice changes nothing the first run already structured.

```bash
docc ingest --type legal_reference klageantwort.pdf
docc structure klageantwort.md
docc check klageantwort.md
```

### Paragraph numbers

A Swiss brief numbers each paragraph in the margin, and how ingest treats those
numbers depends on what the draft is going to become — which only the target
schema knows.

Pass `--type` and ingest consults that schema. A type whose `render:` block
declares `paragraph_numbering` generates the numbers itself at render time, so
the source document's own are dropped: a document carrying both would print them
twice, and the transcribed set would go stale the first time a section moved.
A type that declares no paragraph numbering — `legal_reference`, a transcription
of a third party's brief — keeps them, marked `[Rz N]`, because there the number
is not presentation but the citation key, fixed by whoever filed the document.

Without `--type`, and in a directory with no `.docc` at all, ingest keeps them.
It is a transcription tool first: reproducing the page is the honest default, and
deleting a marker afterwards is easier than re-converting a document to recover
one. An unknown type or an unreadable schema directory is reported and treated
the same way, never as a fatal error.

### Marking headings by the document type's outline

A transcribing model decides heading markup by eye and decides it differently on
every page. Measured on two pages of one brief, the chat backend marked four of
seven section titles and put a fifth at the wrong depth; the layout-first backend
marked thirteen on a document with eight.

A Swiss brief's outline is not a matter of taste, so a schema can state it:

```yaml
outline:
  - pattern: '^[A-ZÄÖÜ][A-ZÄÖÜ\s]+:$'                             # BEGRÜNDUNG:
    level: 1
  - pattern: '^(?:X{1,3}(?:IX|IV|V?I{0,3})|IX|IV|V?I{1,3}|V)\.\s+\S'  # I. FORMELLES
    level: 2
  - pattern: '^[A-ZÄÖÜ]\.\s+\S'                                    # A. FRIST
    level: 3
```

With `--type`, ingest applies it after transcription: a line matching a pattern
becomes a heading at that level, and a heading matching nothing is returned to
prose. Both directions are needed — promoting alone leaves the layout backend's
invented headings, demoting alone leaves the chat backend's missed ones. With it
declared, the two backends produce the same outline from the same pages.

First match wins, so order matters, and so does the shape of the Roman pattern:
`[IVXLCDM]+` is the obvious spelling and it is wrong, because C and D are 100 and
500, which silently puts `C. STREITWERT` a level too shallow.

A title no pattern covers keeps its text and loses its marker, which a reviewer
can see and fix. A type that declares no `outline:` is left exactly as the model
wrote it.

One `legal_reference` type carries three schemes, following the UZH guidance for
legal writing. Pick one with `--outline`; without it, the type's `default` is
used:

| scheme | L1 | L2 | L3 | L4 | L5 |
| --- | --- | --- | --- | --- | --- |
| `roman-first` (default) | `BEGRÜNDUNG:` | `I.` | `A.` | `1.` | `a)` |
| `letter-first` | `A.` | `I.` | `1.` | `a)` | `aa)` |
| `decimal` | `1` | `1.1` | `1.1.1` | `1.1.1.1` | |

They are alternatives rather than one longer declaration because the top two
levels are *inverted* between the first two — `I.` is level 2 in one and level 1
in the other — and no ordering of patterns serves both. The guidance is explicit
that a document uses one scheme throughout and not `Mischformen`, so the choice
is made once, when the transcription starts.

Where schemes overlap, the more frequent reading wins and the schema says so: in
`letter-first` a bare `I.` is read as the Roman numeral opening a section, not as
the ninth top-level letter.

**A scheme is a baseline, not a contract.** The document being transcribed
belongs to somebody else, written to their conventions, and a firm that outlines
its filings some way nobody anticipated is unusual rather than wrong. So ingest
only ever *promotes*: a line matching a rule becomes a heading at that level, and
a line matching nothing is left exactly as the model produced it — marked or not.

`--outline-strict` turns that around, and it is the one case where unmarking is
right. A scheme the schema offers is a guess about a document nobody has read; a
scheme confirmed with `--outline-strict` is a statement about a document the
caller has. Under it a heading matching no rule is one the model invented, and
it is returned to prose — the text is kept either way.

That is what the round-trip fixture needs to transcribe exactly: fifteen marked
headings become its actual eight, at their correct levels.

## Themes

A theme is the visual side of a document type: page geometry, named styles, list
definitions, and the fixed furniture around the body. It also says how non-string
metadata is written out, because that is presentation and differs per document:

```yaml
formats:
  date: "2. January 2006"    # Go reference layout
  bool: ["ja", "nein"]       # [true, false]
  list_separator: ", "
  months: [Januar, Februar, März, April, Mai, Juni,
           Juli, August, September, Oktober, November, Dezember]
  weekdays: [Sonntag, Montag, Dienstag, Mittwoch, Donnerstag, Freitag, Samstag]
```

`months` and `weekdays` translate the names Go's `time` package emits; short
forms are the first three characters unless `months_short` / `weekdays_short`
say otherwise. The engine ships no locale database — a theme that needs one
writes six lines of YAML.

Omit `formats:` and dates render as ISO 8601 and booleans as `true`/`false`:
unambiguous, and in no particular language.

### List definitions

`numbering:` defines the lists a schema selects, by name, for both markdown
lists and render numbering:

```yaml
numbering:
  Randziffer:
    format: decimal          # or upperRoman, upperLetter, lowerRoman, bullet, none
    text: "%1."              # %N is the count at level N, one-based
    size: 8pt                # the label's size, not the paragraph's
    align: right             # within the space the hanging indent reserves
    suffix: space            # what separates label from text: tab, space, nothing
    indent: 0mm
    hanging: 7mm
    style: Standard
```

`levels:` adds the deeper levels as a **flat list** — the definition itself is
level 0 and each entry is the next one down, up to nine. It is not a tree; Word
has one sequence of levels, and a level nested inside another is an error rather
than a third level.

### Fixed furniture

`prologue:` and `epilogue:` are the paragraphs around the body — letterhead,
address block, subject, closing, enclosures. A line interpolates metadata with
`{{ field.path }}`, `repeat:` emits one paragraph per element of a list field,
`frame:` positions a line absolutely, and `page_break: true` starts a new page,
which is how a cover page ends and the body begins on sheet two.

`numbering:` gives a line a Word list number from a definition in the theme.
Lines naming the same definition within one block of furniture share an
instance, so a `repeat` comes out 1., 2., 3. — an enclosures index that
renumbers itself when an entry is added:

```yaml
epilogue:
  - { style: BeilagenTitel, text: "Beilagen", omit_if_empty: false, page_break: true }
  - { style: BeilagenItem, text: "{{ item }}", repeat: beilagen, numbering: Beilagenverzeichnis }
```

Pair it with a `cross_reference` rule over the same list and the index is
checked as well as generated: a Beilage cited in the body but missing from the
list, or listed and never cited, is a diagnostic rather than a discrepancy
someone notices at the counter.

Render numbering does not apply to furniture, so the marginal numbers stop at
the last paragraph of prose and the closing block is unnumbered.

## Development

```bash
task              # full CI: fmt, vet, lint, test, build
task test         # unit tests
task test:golden:update   # regenerate the golden corpus
task hooks:install
```

The golden corpus in `testdata/` is the regression suite, and it is checked at
both ends. Every fixture is validated against `testdata/schemas/` and its
rendered diagnostics compared to a committed `.golden` file; every fixture in
`good/` is then built with its theme and the resulting `word/*.xml` parts
compared to `testdata/golden/<fixture>/`. A change to a message, a rule, a style
or the writer shows up as a diff rather than as a surprise in a real document.

Two document types are covered on purpose. `legal` exercises absolutely
positioned frames and paragraphs whose formatting changes partway through;
`letter` exercises an epilogue, a repeated list field, a footer and metadata
formatting. Between them they reach most of the theme surface, which is what
stops the engine quietly specialising in one document shape.

## Writing Word documents

`pkg/docx` writes `.docx` from scratch — no template to fill, no dependencies
beyond the standard library. It is importable on its own:

```go
d := &docx.Document{
    Section: docx.Section{
        Page:    docx.A4,
        Margins: docx.Margins{Top: docx.Mm(20), Bottom: docx.Mm(20), Left: docx.Mm(26), Right: docx.Mm(15)},
    },
    Defaults: docx.Defaults{Run: docx.RunProps{Font: "Arial", Size: docx.FontPt(11)}},
    Styles:   []docx.Style{{ID: "Standard", Name: "Standard", Default: true}},
    Body:     []docx.Block{docx.P("Standard", "Sehr geehrte Damen und Herren,")},
}
err := d.Write("letter.docx")
```

Supports styles, numbered and bulleted lists, tables with spans and borders,
headers and footers (including a distinct first page), embedded images, and
absolutely positioned frames — which is how the address block lands in the
envelope window.

Output is **deterministic**: identical input produces byte-identical output.
Archive timestamps are fixed, parts are written in sorted order, and identifiers
are assigned by position. A rebuild that changes bytes changed something real.

Units are separate types so they cannot be mixed by accident — `Twips` for
layout, `EMU` for drawings, `HalfPt` for font size, `Eighth` for border widths.
Build them with `Mm`, `Pt`, `Cm`, `MmEMU`, `FontPt`, `BorderPt`.

### Verifying output

Unit tests check structure. What they cannot check is whether a real renderer
accepts the file, so a build-tagged round-trip test converts a generated
document through LibreOffice:

```bash
task test:roundtrip     # needs soffice on PATH
```

It asserts on the produced PDF, not on the exit code: `soffice` exits 0 even
when it produces nothing.

### Validating a theme against a schema

A theme and the schema that names it are two files nobody diffs against each
other, so `docc build` does it before rendering anything:

- every style the schema's `styles:` map names must exist in the theme
- every `{{ field.path }}` the theme interpolates must be declared by the schema

Both are silent failures otherwise. Word renders an unknown style as body text
without complaint, and a placeholder naming a field that does not exist expands
to nothing — which, because a furniture line whose fields are all empty is
dropped, deletes the line. A typo in `{{ recipient.city }}` posts a letter with
no city on it. Now it does not build:

```
docc: theme "example-letter" interpolates fields the schema "letter" does not declare:
  {{ recipent.city }} — the frontmatter declares no field "recipent"
schema declares: beilagen, closing, date, document_type, recipient, ...
```

## Status

`docc init`, `docc check`, `docc build` and `pkg/docx` are implemented and wired together.
Remaining work is in `docs/next-steps.md`.
