# Schema reference

Every key a `.docc/schemas/*.yaml` file may set, what it accepts, and what
happens when it is absent. The narrative introduction is in the
[Schemas](authoring.md#schemas) section; this is the exhaustive list.

A schema is one document type. Adding a type is adding a YAML file — there is no
Go to write unless you need a new *check*, which is the one extension point that
is code rather than data.

Two commands report the same contract from a loaded schema, and cannot drift from
what the compiler enforces:

```bash
docc describe --json <type>   # the whole contract as data
docc doctor                   # which schemas resolved, and whether they are sound
```

## Top level

| Key | Type | Default | Meaning |
|---|---|---|---|
| `type` | string | — | The identifier matched against a document's `document_type`. Required; two schemas with the same `type` is a load error. |
| `extends` | string | — | Another schema whose `frontmatter`, `types`, `blocks`, `spans`, `fields` and `rules` are inherited. Keys declared here win. Cycles are rejected at load. |
| `description` | string | — | One line, shown by `docc types` and `docc describe`. |
| `theme` | string | — | The theme in `.docc/themes` used to render this type. **Empty means check-only**: the type validates but `docc build` refuses it. `docc doctor` reports it as `check-only` rather than as a fault. |
| `example` | string | — | A compact document or drafting starter of this type, printed by `docc example`. It lives in the schema so the contract and its starter cannot drift; a test checks it. A starter may contain blank `before-execution` fields: `check` accepts it while `build` deliberately refuses it. |
| `types` | map | — | Reusable object shapes, referenced by name from a field's `type`. |
| `frontmatter` | map | — | The top-level metadata fields. |
| `body` | list | — | The expected heading structure, in document order. |
| `blocks` | map | — | The `:::name` blocks this type permits. |
| `spans` | map | — | The `[text]{.type}` span types this type permits. |
| `fields` | map | — | Intentionally incomplete fields — blanks that are content. |
| `styles` | map | — | Markdown construct → theme style id. See the [style map](authoring.md#the-style-map); the key set is closed. |
| `rules` | list | — | Named cross-cutting checks to run. |
| `render` | map | — | Numbering the source markdown does not express. |

### How `extends` merges

Merging is per-key, not wholesale, and the two halves behave differently:

- **Merged key-wise**: `frontmatter`, `types`, `blocks`, `spans`, `fields`,
  `styles`. A child redeclaring one entry replaces that entry and keeps its
  siblings.
- **Replaced wholesale**: `body`, `rules`. Declaring any means declaring all of
  them — partial override of an ordered structure is more confusing than
  restating it.
- **Inherited when blank**: `theme`, `example`.
- **Inherited individually**: each `render` rule.

`docc describe` reports the *resolved* schema, so what it prints is what applies
after merging.

## `frontmatter` and `types`

Both map a field name to a declaration:

```yaml
types:
  party:
    name: { type: string, required: true }
    uid:
      type: string
      pattern: '^CHE-\d{3}\.\d{3}\.\d{3}$'
      hint: 'Swiss UID format CHE-123.456.789'

frontmatter:
  sender: { type: party, required: true }
  parties: { type: list<party> }
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `type` | string | — | A builtin, `list<T>` of a builtin or object, or the name of a `types:` entry. |
| `required` | bool | `false` | Absent or empty is `DOC004`. |
| `nullable` | bool | `false` | Permits an explicit YAML `~` to satisfy `required`. Use it for "known to be absent" — an opposing party with no legal representative. The key must still be *present*. |
| `values` | list | — | The permitted values when `type: enum`. A value outside them is `DOC008`. |
| `pattern` | string | — | A Go regexp a string value must match. A failure is `DOC010`; an invalid regexp is `DOC009`, against the schema. |
| `hint` | string | — | Shown verbatim in the diagnostic. Write it as an instruction, not a description. |
| `default` | any | — | Applied during checking, so it decides whether a required field is missing, and it reaches the emitter for interpolation. |

Builtin types: `string`, `int`, `bool`, `date`, `enum`, `any`, and `list<T>` of
any of them or of an object type.

`date` means an ISO date, `YYYY-MM-DD`; anything else is `DOC007`. A value whose
leading zeros matter — a Swiss postal code — must be **quoted**, or YAML makes it
an int and the diagnostic is `DOC006`.

There is no per-field `description`. `hint` is the only prose, and it appears in
diagnostics, so write it for the person who just failed the check.

## `body`

A list of heading rules, in the order the document should carry them:

```yaml
body:
  - heading: BEGRÜNDUNG
    level: 1
    required: true
    ordered: true
    children:
      - heading: Zuständigkeit
        level: 2
        required_when: 'legal_doc_type == "Klageschrift"'
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `heading` | string | — | The exact heading text. Matched case-insensitively, ignoring surrounding whitespace. |
| `level` | int | — | The markdown heading level, `1` for `#`. |
| `required` | bool | `false` | Missing is `DOC020` when true, `DOC021` (a warning) when false. |
| `required_when` | string | — | Makes the requirement conditional on frontmatter, written `field == "value"`. |
| `ordered` | bool | `false` | On a rule, the *children* must appear in the declared order, not merely be present. Out of order is `DOC022`. |
| `children` | list | — | Nested rules, same shape. |

`level` is independent of nesting: a child may declare the same level as its
parent. The nesting expresses which rule owns which, not the heading depth.

## `blocks`

```yaml
blocks:
  partei:
    description: A contracting party.
    discriminator: kind
    variants:
      person:  { required_spans: [name, geburtsdatum] }
      firma:   { required_spans: [name, uid] }
    attributes: [role]
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `description` | string | — | Reported by `docc describe`. Say what the block is *for*, and any authoring constraint the checker cannot enforce. |
| `attributes` | list | — | Permitted attribute keys beyond `#id` and the discriminator, which are always allowed. An undeclared attribute is `DOC035`. |
| `discriminator` | string | — | The attribute whose value selects a variant. Declaring `variants` without one is `DOC036` — a schema bug, since no document could satisfy it. |
| `variants` | map | — | Discriminator value → its structural requirements. A missing or unknown discriminator value is `DOC032`. |
| `required_spans` | list | — | Span types that must appear inside, for a block without variants. A variant carries its own. A missing one is `DOC033`. |

**Declaring any block makes an undeclared `:::name` an error** (`DOC030`). Until
a schema declares one, any name is permitted — the declarations are the opt-in,
so an existing schema keeps its meaning until it adopts the contract. The same
is true of `spans`.

The zero value is a valid declaration: `beweis: {}` permits the block, requires
nothing, and allows no attribute but `#id`.

### What may go inside a block

The syntax: an opening fence of **three or more colons** followed by the name and
optional attributes, and a closing fence of **colons alone on their own line**.
An unclosed block is `DOC023`.

```markdown
::: partei {#verkaeufer kind=person role=veraeusserer}
Herr [Max Muster]{.name}, geboren am [12. April 1975]{.geburtsdatum}
:::
```

Attributes are `{#id key=value key="quoted value"}`. Only `#id`, the
discriminator and the keys in `attributes:` are permitted (`DOC035`); an
attribute block that does not lex is `DOC026`, and the span equivalent is
`DOC027`.

Ordinary markdown is accepted inside: paragraphs, lists, tables, code fences,
emphasis, and the spans the schema declares. Three limits are worth knowing
before designing a block, because none of them is an error — the document
compiles and the output is simply not what was intended:

- **Blocks do not nest.** A `::: name` inside another block leaves the inner one
  unclosed, reported as `DOC023` against the inner opening line, because the
  first bare `:::` closes the outer block. Keep them flat; a block that needs
  internal structure wants a list, spans, or a second block after it.
- **A heading inside a block stops being a heading.** `div.<name>` styles *every
  paragraph* of the block, so a `## Sub` inside one renders in the block's style,
  without its outline level, and `render.heading_numbering` skips it. Headings
  belong between blocks, not inside them.
- **A table inside a block is not restyled.** `div.<name>` reaches paragraphs
  only, so a table passes through with the compiler's own borders. See
  [what a theme cannot change](authoring.md#what-a-theme-cannot-change).

A block with no `#id` is fine unless a span references it. Ids must be unique
across the document (`DOC034`), because `ref=` resolves against them.

**How a block renders** is decided by the style map, not here. See the
[style map](authoring.md#the-style-map) — mapping `div.<name>.amount`, `.line` or
`.label` selects a rendering pattern, and `docc describe` reports which one a
block ended up with.

## `spans`

```yaml
spans:
  uid:
    description: A Swiss company identifier.
```

Only `description`. A span never changes rendering by itself — it exists so the
checker can find a value inside prose. The schema's `styles:` may give
`span.<type>` a character style; without one the text renders as ordinary prose.

Types prefixed `docc-` are reserved for the compiler and need no declaration.
A span with no type class, or an undeclared one, is `DOC031`.

`ref=` on a span resolves against `{#id}` attributes on blocks; a reference to an
id that does not exist is `DOC037`, and a reused id is `DOC034`.

## `fields`

The intentionally incomplete fields: blanks that are *content*, not missing
content. They must appear visibly in the document, annotated as a `.docc-field`
span, and the declaration says when the blank stops being allowed.

```yaml
fields:
  beurkundungsdatum:
    description: filled in by hand at the reading
    required: true
    completion: handwritten
```

Written in the document as:

```markdown
Beurkundet am [____________]{.docc-field key=beurkundungsdatum}
```

A field may retain a semantic character style. Put its semantic type first and
add `.docc-field` as a second class:

```markdown
[SIX SIS AG]{.glaeubiger .docc-field key=glaeubiger_name}
```

The field key makes completion checkable; `.glaeubiger` still selects the
schema's span validation and character style.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `description` | string | — | Reported by `docc describe`. |
| `required` | bool | `false` | The field's *absence from the document* — no span with its key at all — is `DOC038`. A present-but-blank field satisfies it. |
| `completion` | string | `before-execution` | `before-execution` blocks `docc build` while the field is blank (`DOC039`); `handwritten` lets the blank survive into the rendered page, because a human completes it on paper. An unknown value is `DOC041`. |

A `.docc-field` span with no `key=`, or naming a field the schema does not
declare, is `DOC040`.

## `rules`

Declarative field constraints cover most of a contract. The rest — cross
references between frontmatter and body, arithmetic, placeholder detection —
needs real code, so schemas select checks by name.

```yaml
rules:
  - id: LEG012
    check: div_items_match
    severity: error
    args:
      div: beweis
      pattern: '^\s*\[[^\]\r\n]+\]\s+\S'
    message: "Beweismittel without a bracketed label"
    hint: 'prefix it with a label, e.g. "[Beilage 3]"'
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `id` | string | `DOC099` | The diagnostic code emitted. **Always set it**: the fallback is shared by every unnamed rule, so `docc explain` cannot say anything specific. |
| `check` | string | — | A registered check. An unregistered name is `DOC009`. |
| `severity` | string | `error` | `error` or `warning`. |
| `message` | string | the check's own | Overrides the diagnostic's message. |
| `hint` | string | the check's own | Overrides the hint. Write it as an instruction. |
| `args` | map | — | Check-specific configuration; see below. |

A code a schema defines is resolved by `docc explain <CODE>`, which searches the
project's types for the rule that declares it and reports the check it selects,
its severity, and the schema's own `message:` and `hint:`. `docc describe <type>`
prints the same detail beside each rule, so what a rule forbids is legible
without provoking it.

### The check registry

| Check | Args | What it reports |
|---|---|---|
| `no_placeholder_text` | `pattern` (regexp, defaults to bracketed prose) | Template placeholders left in the document. A `[FILL IN]` that reaches a filed brief. |
| `div_items_match` | `div` (required), `pattern` (required regexp) | A list item inside `::: <div>` that does not match the shape. The cheap way to enforce a per-line format. |
| `cross_reference` | `div` (required), `pattern` (required regexp, capture group 1 is the key), `list_field` (required), `label` (defaults to `list_field`) | A citation in a block that does not index into a frontmatter list. |
| `required_div` | `div` (required), `anchor_heading` (optional) | A required semantic block is absent. The optional heading anchors the diagnostic where the missing content belongs. |
| `no_empty_sections` | — | A heading with no content. A heading whose next heading is deeper is a container and is exempt. |
| `no_blank_spans` | — | A semantic span left as a blank — `[____]{.heimatort}`. A span carrying `.docc-field` is exempt, because there a blank is content; any other span is a fact the document claims to state. |
| `amounts_balance` | `div` (required) | Money that does not add up: items whose sum contradicts a `[= …]` total, or a block with `total-of=<id>` whose items do not settle that block's total. |

Adding a check is Go: implement it in `internal/sema/rules.go` and register it.
Schemas then select it by name and supply their own code, severity and wording.

## `render`

Numbering the markdown source does not carry. It lives in the schema rather than
the theme because it is a fact about the document type — a brief has numbered
sections and marginal numbers, a letter has neither. What the numbers *look like*
is the theme's business, which is why each rule names a definition.

```yaml
render:
  heading_numbering:
    definition: LegalHeadingNumbering
    start_at_heading: BEGRÜNDUNG
  paragraph_numbering:
    definition: Randziffer
    start_after_heading: RECHTSBEGEHREN
  page_break_before_headings: [Bescheinigung]
```

| Key | Type | Meaning |
|---|---|---|
| `heading_numbering` | rule | Numbers headings by markdown level: level 1 takes the definition's level 0. |
| `paragraph_numbering` | rule | Numbers top-level paragraphs of prose at level 0, continuously, across the headings between them. |
| `page_break_before_headings` | list | Headings that start a new page. |

Each rule:

| Key | Type | Meaning |
|---|---|---|
| `definition` | string | An entry in the theme's `numbering:`. One the theme does not define is caught by `emit.Validate` — and so by `docc doctor` — before anything renders. |
| `start_at_heading` | string | The heading that is itself the first numbered block. |
| `start_after_heading` | string | A heading that is *not* numbered, after which numbering begins. This is what a marginal number wants: the count starts with the prose, not the heading above it. |
| `end_before_heading` | string | The first heading outside the numbered outline. It keeps its heading style but receives no label; useful for a deed's filing annex after its certification. |

Set neither start marker and numbering applies to the whole body. Setting both
start markers is an error rather than a precedence rule nobody would remember.

## Diagnostics

`docc explain <CODE>` describes any engine code; `docc explain` lists them all,
and `docc explain <CODE> --type <type>` adds the constraints this schema
declares. Broadly:

| Range | Subject |
|---|---|
| `DOC001`–`DOC003`, `DOC024`–`DOC025` | The frontmatter block and the `docc:` marker |
| `DOC004`–`DOC008`, `DOC010`–`DOC011` | Field values |
| `DOC009` | The schema is wrong, not the document |
| `DOC012`–`DOC013` | Document type resolution |
| `DOC020`–`DOC022` | Body structure |
| `DOC023`, `DOC026`–`DOC027` | Block and span syntax |
| `DOC030`–`DOC037` | The markup contract — blocks, spans, ids, references |
| `DOC038`–`DOC041` | `fields:` blanks |
| `DOC099` | A rule fired but its schema gave it no `id` |
