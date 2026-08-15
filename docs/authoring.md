# Authoring guide: schemas and documents

What a document type declares and how a document satisfies it. This is the
narrative; [schema-reference.md](schema-reference.md) is the exhaustive key list.

## Schemas

> Every schema key, with its accepted values and defaults, is in
> **[schema-reference.md](schema-reference.md)**. This section is the
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
`Style` fields listed in the [theme reference](theme-reference.md#styles). There is no raw OOXML escape
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
