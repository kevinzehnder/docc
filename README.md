# docc

A compiler for structured documents.

`docc` treats a markdown document with YAML frontmatter as source code: it parses
it, checks it against a schema, and reports errors with source positions and
actionable hints. It renders self-contained Word `.docx` files from the schema
and theme configuration.

A file becomes a docc document by declaring the marker in its frontmatter:

```yaml
---
docc: 1          # the docc format version — this is what makes it a docc file
document_type: ch_legal
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
docc init --dry-run               # list what it would create, without writing
docc doctor                       # which schemas and themes are in effect, and are they sound
docc check docs/klage.md          # validate
docc check --json docs/*.md       # machine-readable, for agents and CI
docc check --strict docs/klage.md # warnings become errors
docc build docs/klage.md          # validate, then emit a .docx
docc build --to pdf docs/klage.md # optional compatibility export; needs soffice
docc lsp                          # serve editor diagnostics over stdio
docc types                        # list known document types
docc describe ch_legal            # report a document type's full contract
docc example ch_legal             # print a compact valid document to start from
docc describe --from ~/kanzlei ch_legal  # …for a project other than this directory
docc explain                      # list every diagnostic code
docc explain DOC010               # describe one
docc explain DOC010 --type ch_legal  # …and the constraints that schema declares
```

Flags may appear before or after the positional arguments, so
`docc build docs/klage.md --to pdf` works. Use `--` to end flag parsing when a
file name begins with a dash. `--help` on any subcommand prints its usage and
exits `0`.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | clean |
| `1` | the command ran and reported diagnostics, or failed part-way through |
| `2` | usage error — the command line is wrong; a different invocation may work |
| `3` | configuration error — the project's schemas or themes are missing or unusable |

`2` and `3` are separated because a caller can act on the difference. A wrong
flag is worth retrying; a missing `.docc` directory is not.

### Which configuration am I using?

Schemas and themes are discovered by walking up from the input path, the way git
finds `.git`. `docc doctor` reports the directories that resolved to, lists the
types and themes it found, and checks every schema against the theme it names —
that every mapped style exists, every interpolated field is declared, and every
numbering definition resolves. Those checks otherwise run only inside a build, so
a profile that could never render stayed invisible until someone authored a
document for it.

```
$ docc doctor
configuration:
  project root   /srv/kanzlei
  schemas        /srv/kanzlei/.docc/schemas  (discovered)
  themes         /srv/kanzlei/.docc/themes  (discovered)

document types:
  base       check-only  declares no theme, cannot be built
  ch_legal   ok          theme zbp-legal
  ch_letter  ok          theme jlmy-letter
```

`.docx` is the supported compiler output. `--to pdf` remains a
compatibility-only export for environments that provide LibreOffice (`soffice`).
AgentSkill hosts should build DOCX and use their own document/PDF capability
when a user requests a PDF.

### JSON contract

`--json` produces one JSON object on stdout for each successful command result.
It never mixes human-readable status text into that stream.

| Command | stdout JSON |
|---|---|
| `check --json` | `{ "ok", "errors", "warnings", "diagnostics" }` |
| `build --json` | `{ "ok", "type", "theme", "format", "output" }`; validation diagnostics are a separate JSON object on stderr |
| `types --json` | `{ "types": [{ "type", "description", "theme" }] }` |
| `describe --json` | `{ "type", "extends", "theme", "frontmatter", "body", "blocks", "spans", "blanks", "rules", "has_example", "field_map" }` — the full contract, with a `syntax` example per block, span and blank |
| `themes --json` | `{ "themes": [{ "name", "description", "styles" }] }` |
| `doctor --json` | `{ "start", "root", "schema_dir", "schema_source", "theme_dir", "types", "themes", "problems", "ok" }` |
| `explain --json` | `{ "code", "explanation", "type", "detail" }`, or `{ "codes": [...] }` with no code given |

`describe` reports each frontmatter field with everything the schema declares
about it: whether it is `required`, whether it is `nullable` (so an explicit `~`
satisfies the requirement), its `pattern`, enum `values`, `default` and hint.
Object types declared in the schema's `types:` are expanded inline as `members`,
so a `sender: sender` field also lists the fields underneath it.

Each field also carries `rendered`: the places the type's theme interpolates it —
`prologue`, `epilogue`, `header:<key>`, `footer:<key>`. A field with none is
metadata the theme never prints. `field_map` reports whether the theme could be
consulted at all; when it is `false`, an empty `rendered` means nothing.

Body headings report `required`, `required_when` (the frontmatter condition that
makes an otherwise optional section mandatory) and `ordered`.

Failures stay on the JSON stream. Under `--json`, a command that cannot produce
its result writes a failure object to stdout instead:

```json
{ "ok": false, "kind": "config", "error": "unknown document type \"nosuch\" (known types: ch_legal, ch_letter)" }
```

`kind` is `usage`, `config`, or `error`, matching the exit code. `build`'s
validation failure adds `"kind": "diagnostics"` and the document `type`.

Two paths stay human-readable, deliberately: a flag that fails to parse (the
command line is malformed, so `--json` may not have been understood either), and
everything in `docc lsp`, whose stdout carries the LSP protocol.

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

`docc` is the engine. The schemas, themes and house style belong to the
project being compiled, in a `.docc` directory that `docc` finds by walking up
from the input file the way `git` finds `.git`:

```
myproject/
  .docc/
    schemas/
      _base.yaml        # shared field shapes, extended by the rest
      ch_legal.yaml
      ch_letter.yaml
    themes/
      legal.yaml         # page geometry, styles, and fixed furniture
  docs/
    klage_mueller.md
```

Override the location with `--schema-dir`.

### Starting a new project

`docc init [directory]` creates `.docc/` with a generic letter and a
Swiss-legal starter schema/theme, plus compiling examples in
`examples/docc/`. It never overwrites an existing starter configuration,
example directory, or installed skill, and it creates nothing at all when it
refuses. `--dry-run` lists the files it would write and touches nothing.

What it installs is yours to edit — a starting point, not a managed install.

```bash
mkdir my-documents && cd my-documents
docc init --dry-run     # see what is coming
docc init
docc doctor             # confirm what resolved, and that it is sound
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

> Every schema key, with its accepted values and defaults, is in
> **[docs/schema-reference.md](docs/schema-reference.md)**. This section is the
> introduction; that one is exhaustive.


A schema declares frontmatter fields and their types, the body structure, the
markdown-to-Word-style mapping, and which named rules to run.

The `docc` marker is declared in the base schema (`_base.yaml`) but validated by
the compiler before any schema field is checked, so it never appears as an
unknown-field warning even in projects whose schemas do not extend the base.

```yaml
type: ch_legal
extends: base
description: Formal legal brief.
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

### The style map

`styles:` maps a markdown construct to a style id the theme defines. The set of
constructs is closed — it is exactly the keys the emitter looks up, and a key
outside it has no effect at all. `docc describe <type>` prints the keys this type
maps, the keys it could map, and any that will never be read; `docc doctor`
warns about the last kind.

| Key | Applies to | Falls back to |
|---|---|---|
| `paragraph` | body prose | — |
| `h1` … `h6` | a heading of that level | `heading` |
| `heading` | any heading with no level-specific mapping | — |
| `quote` | block quote | `paragraph` |
| `code` | fenced code block | `paragraph` |
| `table` | table | — |
| `ordered_list` | numbered list | — |
| `bullet_list` | bulleted list | — |
| `div.<name>` | every paragraph of a `::: <name>` block | `paragraph` |
| `span.<type>` | a `[text]{.<type>}` span | — |

`ordered_list` and `bullet_list` may name an entry in the theme's `numbering:`
rather than a style; the definition's own `style:` then supplies the paragraph
style.

A block takes further keys, and mapping one **selects a rendering pattern** —
the pattern is a consequence of the style map, not something the block declares:

| Key | Pattern |
|---|---|
| none of the below | plain — every paragraph in `div.<name>` |
| `div.<name>.amount` | amount rendering; styles the amount column |
| `div.<name>.total` | the total row of amount rendering |
| `div.<name>.total.amount` | the amount cell of that total row |
| `div.<name>.words` | the amount spelled out; needs the theme's `formats.amount_words` |
| `div.<name>.line` | ruled rendering; styles the rule |
| `div.<name>.label` | labelled rendering; styles the tabbed label |

They are tried in the order `.amount`, `.line`, `.label`; mapping two silently
takes the first. `docc describe` reports which pattern each block ended up with.

### What a theme cannot change

Some constructs are formatted by the compiler and reach no style key. A schema
that maps one of these is not overridden — it is ignored, which is why `doctor`
reports it:

| Construct | Always renders as |
|---|---|
| `**bold**` | bold |
| `*italic*` | italic |
| `` `inline code` `` | Courier New, otherwise inherited |
| `[a link](…)` | colour `0000EE`, single underline — text, not a live hyperlink |
| table borders | 0.5pt single rule on every edge, inside and out |
| table columns | the text width divided evenly; markdown carries no column sizing |

Within a style, the properties a theme may set are likewise a closed set — the
`Style` fields listed under [Themes](#themes). There is no raw OOXML escape
hatch, and that is deliberate: a closed vocabulary is what lets `emit.Validate`
and `docc doctor` check a profile at all.

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

### Semantic blocks and spans

Two syntax extensions mark semantics that plain Markdown cannot express. A
**block** is a `:::name` region, optionally attributed; a **span** annotates
literal text inline. A span never changes rendering — it exists for validation:

```markdown
::: partei {#verkaeufer kind=person role=veraeusserer}
Herr [Max Muster]{.name}, geboren am
[12. April 1975]{.geburtsdatum}, wohnhaft an der
[Musterstrasse 10, 5400 Baden]{.adresse}
:::
```

Attributes are `{#id key=value key="quoted value"}`; a span's first `.class`
is its type. The schema declares what is permitted — declaring any block or
span type makes undeclared ones an error, while schemas that declare none
leave the markup unchecked:

```yaml
blocks:
  beweis: {}
  partei:
    discriminator: kind      # the attribute selecting a variant
    attributes: [role]       # further permitted keys; #id is always allowed
    variants:
      person:
        required_spans: [name, geburtsdatum, adresse]
      company:
        required_spans: [firma, sitz, uid, adresse]

spans:
  name: {}
  geburtsdatum: {}
  adresse: {}
  firma: {}
  sitz: {}
  uid: {}
```

A span may reference a block by its id — `[Erwerberin]{.partei ref=erwerberin}`
— and the displayed wording stays the author's; docc only resolves the
reference.

An intentionally incomplete field is content, not missing content: it appears
visibly as a blank and is annotated with the reserved `docc-field` type
(`docc-` types need no declaration):

```markdown
Die Urkunde wurde am
[____________________]{.docc-field key=beurkundungsdatum}
unterzeichnet.
```

```yaml
fields:
  beurkundungsdatum:
    required: true            # absence is an error at check time (DOC038)
    completion: handwritten   # may stay blank through build
  protokollnummer:
    required: true
    completion: before-execution   # blank blocks `docc build` (DOC039)
```

`check` accepts blank fields — drafting with them is the point; `build`
refuses a blank whose completion is not `handwritten`.

The checker reports undeclared blocks (`DOC030`), untyped or undeclared spans
(`DOC031`), a missing discriminator or unknown variant (`DOC032`), missing
required spans (`DOC033`), duplicate `#id`s (`DOC034`), unpermitted
attributes (`DOC035`) and unresolved `ref=`s (`DOC037`). Malformed attribute
syntax is caught at parse time (`DOC026`, `DOC027`).

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
| `no_placeholder_text` | draft placeholder text that was never filled in | `pattern` — what a placeholder looks like. Defaults to a line that is nothing but bracketed prose, `[like this]`. |
| `div_items_match` | items in a fenced div that do not have the required form | `div` — the fence name, `pattern` — a regexp every item must match |
| `cross_reference` | keys cited in the body but missing from a frontmatter list, and entries listed but never cited | `div`, `pattern` — capture group 1 is the cited key, `list_field` — the frontmatter list, `label` — what one entry is called in messages |
| `no_empty_sections` | a heading with no content beneath it | — |
| `amounts_balance` | money in a block that does not add up: parts that miss their declared total, or payments that leave part of it unsettled | `div` — the fence name |

`cross_reference` numbers a list positionally: the Nth entry of `list_field` is
key N.

`amounts_balance` reads the bracketed amount that opens each item of a money
block. One item may be marked as the block's total with a leading `=`, and the
rest must add up to it; a block that settles another block's total names it
with `total-of=<id>`. Every block naming the same total is summed **together**,
so a payment schedule split into a sub-section per instalment is checked as one
schedule rather than instalment by instalment:

```markdown
::: betraege {#kaufpreis}
- [Fr. 820'000.00] für die Wohnung
- [Fr. 45'000.00] für den Autoeinstellplatz
- [= Fr. 865'000.00] Ausmachend den Kaufpreis von
:::

::: betraege {#tilgung-1 total-of=kaufpreis}
- [Fr. 86'500.00] Anzahlung
:::

::: betraege {#tilgung-2 total-of=kaufpreis}
- [Fr. 778'500.00] Restkaufpreis
:::
```

Sums are exact — the amounts are read as hundredths, never as floating point,
so a rounding artefact cannot be reported as a drafting error.

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

## Themes

> Every theme key, with its accepted values and defaults, is in
> **[docs/theme-reference.md](docs/theme-reference.md)**.


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

Two document types are covered on purpose. `ch_legal` exercises absolutely
positioned frames and paragraphs whose formatting changes partway through;
`ch_letter` exercises an epilogue, a repeated list field, a footer and metadata
formatting. Between them they reach most of the theme surface, which is what
stops the engine quietly specialising in one document shape.

## Writing Word documents

`internal/docx` writes `.docx` from scratch — no template to fill, no
dependencies beyond the standard library. It is an implementation detail of
`docc build`, not a published Go library: the package is internal so its API can
change with the compiler rather than being versioned as a separate product.

It supports styles, numbered and bulleted lists, tables with spans and borders,
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

Unit tests check structure. An optional, build-tagged LibreOffice compatibility
test also converts a generated document to PDF:

```bash
task test:roundtrip     # needs soffice on PATH
```

It asserts on the produced PDF, not on the exit code: `soffice` exits 0 even
when it produces nothing. This is not a required release gate; DOCX generation
and ZIP verification are the supported release checks.

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

`docc init`, `docc check`, `docc build`, `docc lsp`, and the `internal/docx`
writer are implemented and wired together. Planned maintenance work is tracked
in [docs/next-steps.md](docs/next-steps.md).

## License

MIT — see [LICENSE](LICENSE).
