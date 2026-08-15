# Theme reference

Every key a `.docc/themes/*.yaml` file may set, what it accepts, and what happens
when it is absent. The narrative introduction is in the
[Themes](theming.md#themes) section; this is the exhaustive list.

A theme is the visual definition a schema selects by name. It knows nothing about
document types: it defines named styles, list definitions, page geometry and the
fixed furniture around the body. The schema decides which style a markdown
construct wears — see the [style map](authoring.md#the-style-map).

The property vocabulary below is closed. There is no raw OOXML escape hatch, and
that is deliberate: a closed vocabulary is what lets `emit.Validate`, and so
`docc doctor`, check a profile before anything renders.

```bash
docc themes          # what this project defines
docc doctor          # whether each schema and its theme agree
```

## Measurements

Four value types recur, and each is parsed rather than taken as a bare number,
because a hand-edited theme full of twips is unreadable.

| Type | Written as | Notes |
|---|---|---|
| length | `"20mm"`, `"1.5cm"`, `"12pt"`, `"0.5in"`, `"1440tw"` | **Quoted**, and the unit is required. A bare number is an error. |
| font size | `"11pt"` | Half-points internally. |
| line height | `1.15` or `"14pt"` | A **number** is a multiple of the line; a **length** is an exact height. |
| colour | `"1F3864"` | Six hex digits, no `#`. |

## Top level

| Key | Type | Default | Meaning |
|---|---|---|---|
| `name` | string | file name | The identifier a schema's `theme:` names. Two themes with the same name is a load error. Never inherited. |
| `extends` | string | — | Another theme in the same directory whose keys this one inherits. See [Inheritance](#inheritance). |
| `description` | string | — | One line, shown by `docc themes`. |
| `page` | map | A4 portrait | Page geometry. |
| `defaults` | map | — | Document-wide font, size and language. |
| `formats` | map | ISO | How non-string values are written. |
| `styles` | map | — | Named style definitions. |
| `numbering` | map | — | List definitions, referenced by `styles:` mappings, `render:` rules and furniture lines. |
| `header` | map | — | Header furniture, keyed by `default`, `first` or `even`. |
| `footer` | map | — | Footer furniture, same keys. |
| `prologue` | list | — | Fixed furniture before the body — letterhead, address block, subject line. |
| `epilogue` | list | — | Fixed furniture after the body — closing, signature, enclosures. |

## `page`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `size` | string | `a4` | `a4` or `letter`. `width`/`height` override it. |
| `width`, `height` | length | from `size` | Explicit page dimensions. |
| `landscape` | bool | `false` | Swaps width and height. |
| `margins` | map | — | `top`, `bottom`, `left`, `right`, `header`, `footer`, `gutter`, each a length. |
| `continuation_margins` | map | — | Margins for pages after a section break. Unset keys fall back to `margins`. A first page with a deep letterhead margin and ordinary pages after it is the reason this exists. |
| `title_page` | bool | `false` | Gives the first page its own header and footer — the `first` key of `header:`/`footer:`. |

## `defaults`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `font` | string | — | The document's base font. |
| `size` | font size | `11pt` | The base size. |
| `lang` | string | — | The proofing language, e.g. `de-CH`. Worth setting: Word otherwise spell-checks a German brief against the machine's locale. |

## `formats`

How non-string metadata is written when the furniture interpolates it. Supplied
here rather than looked up from a locale, because a locale database is a
dependency and a table is four lines of YAML.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `date` | string | ISO 8601 | A **Go reference layout**, e.g. `"2. January 2006"`. A theme that says nothing renders something unambiguous. |
| `bool` | list | `["true", "false"]` | The two words, true first. |
| `list_separator` | string | `", "` | Joins a list field into one line. |
| `amount_words` | string | — | How a money block spells an amount, with `%s` for the figure: `"(Franken %s)"`. Empty means amounts are not spelled out. The speller is German; a theme in another language leaves this unset. |
| `months` | list | English | The twelve names, in calendar order. |
| `months_short` | list | first 3 chars | Short month names. |
| `weekdays` | list | English | The seven names, **Sunday first**, matching Go's week. |
| `weekdays_short` | list | first 3 chars | Short day names. |

## `styles`

The map key is the style id a schema's `styles:` map names. Everything below is
optional; a style that sets nothing inherits the document defaults.

```yaml
styles:
  Ueberschrift1:
    name: "Überschrift 1"
    based_on: Standard
    next: Standard
    size: "13pt"
    small_caps: true
    outline_level: 1
    keep_next: true
    spacing: { before: "12pt", after: "6pt" }
```

### Identity

| Key | Type | Default | Meaning |
|---|---|---|---|
| `name` | string | the map key | The display name in Word's style gallery. |
| `based_on` | string | — | Another style id to inherit from. |
| `next` | string | — | The style applied to the paragraph after this one — a heading's `next` is normally body text. |
| `default` | bool | `false` | Marks this as the document default style of its type. |
| `type` | string | `paragraph` | `paragraph` or `character`. A span style must be `character`. |
| `ui_priority` | int | — | Sort order in Word's gallery. |

### Character formatting

| Key | Type | Meaning |
|---|---|---|
| `font` | string | Typeface. |
| `size` | font size | e.g. `"10pt"`. |
| `bold`, `italic` | bool | |
| `underline` | string | An OOXML underline value; `single`, `double` and `none` are the ones in use. |
| `caps` | bool | All capitals. |
| `small_caps` | bool | Small capitals. |
| `color` | colour | e.g. `"1F3864"`. |

### Paragraph formatting

| Key | Type | Meaning |
|---|---|---|
| `align` | string | `left` (default), `center` (or `centre`), `right`, `justify` (or `both`). |
| `spacing` | map | `before`, `after` (lengths) and `line` (line height). |
| `indent` | map | `left`, `right`, `first_line`, `hanging`, all lengths. |
| `tabs` | list | Tab stops: `pos` (length), `align` (`left`, `center`, `right`, `decimal`), `leader`. |
| `borders` | map | `top`, `bottom`, `left`, `right`, each with `style` (`none`, `single`, `double`, `dotted`, `dashed`), `width` (points), `space` (length), `color`. A bottom border alone is how a rule is drawn under a letterhead. |
| `shading` | colour | Background fill. |
| `outline_level` | int | Heading depth for the navigation pane and PDF bookmarks. `1` is the top level; omit for body text. |
| `keep_next` | bool | Stops a heading being stranded at the foot of a page. |
| `keep_lines` | bool | Keeps the paragraph's lines together. |
| `page_break_before` | bool | Starts a new page. |
| `contextual_spacing` | bool | Suppresses spacing between paragraphs of the same style — list items. |

Left-align styles for content with manual line breaks or short lines (party
entries, address blocks) even when the body justifies: Word stretches a justified
line that ends in a manual break.

## `numbering`

A list definition, named by a schema's `render:` rule, by a `styles:` mapping for
`ordered_list`/`bullet_list`, or by a furniture line's `numbering:`.

| Key | Type | Meaning |
|---|---|---|
| `format` | string | The enumeration: `decimal`, `upperRoman`, `lowerRoman`, `upperLetter`, `lowerLetter`, `bullet`, `none`. |
| `text` | string | The label, with `%N` for the counter at level N: `"%1."`, `"%1.%2."`. A `%N` referring to a level the definition does not have renders as literal text. |
| `start` | int | The first number. |
| `indent`, `hanging` | length | Paragraph geometry for numbered items. |
| `font`, `size` | | The label's own typeface and size. A bullet glyph needs its font: `Symbol` for a filled bullet, `Courier New` for `o`. |
| `align` | string | Alignment of the label itself. |
| `suffix` | string | What follows the label: `tab` (default), `space` or `nothing`. Anything else makes Word offer to repair the file. |
| `style` | string | A paragraph style numbered items take, when the schema maps a list key to this definition rather than to a style. |
| `levels` | list | The deeper levels, each the same shape. |

**`levels:` is flat, not a tree.** The definition itself is level 0 and
`levels[i]` is level `i+1`, capped at nine. An entry that declares `levels:` of
its own is a theme author expecting a tree, and is rejected — the entries under
`levels:` are already the deeper ones.

Two lists sharing a definition share Word's abstract numbering, so they look
alike; each top-level list takes its own instance, so they restart. Render
numbering inverts that: a heading outline and a marginal number each want *one*
shared instance for the whole document, because continuing is the point.

## Furniture: `prologue`, `epilogue`, `header`, `footer`

Fixed content the document does not author. `header` and `footer` are keyed by
`default`, `first` (needs `page.title_page: true`) and `even`. Each is a list of
lines.

Text interpolates document metadata with `{{ field.path }}`, resolved against the
schema's frontmatter. **Every path is checked against the schema before anything
renders** — a typo in an address block would otherwise expand to nothing, and a
line whose fields are all empty is dropped, so the letter would post with no city
on it.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `style` | string | — | A style id from `styles:`. |
| `text` | string | — | The paragraph's text, with `{{ }}` interpolation. Ignored when `runs` is set. |
| `runs` | list | — | A paragraph whose formatting changes partway through. See below. |
| `frame` | map | — | Positions this line, and those following it in the same block, as a floating frame: `x`, `y`, `width`, `height`, `h_anchor`, `v_anchor`, `wrap`. This is how an address block reaches the envelope window without a text box. |
| `image` | map | — | A picture: `path` (relative to the theme file), `width`, `height`, `alt`. |
| `repeat` | string | — | A list field, emitting one paragraph per element. Inside the text, `{{ item }}` is the element. |
| `if_nonempty` | string | — | Emits the line only when the named field has a value — for a heading that must disappear along with the empty list it introduces. |
| `numbering` | string | — | A definition from `numbering:`, giving the line a Word list number. Lines naming the same definition within one block of furniture share an instance, so a `repeat` comes out 1., 2., 3. The label is Word numbering, not text: it renumbers itself, and a cross-reference check can still read the underlying list. |
| `omit_if_empty` | bool | `true` | Drops the line when every field it interpolates is empty. |
| `page_break` | bool | `false` | Starts a new page before this line. |
| `section_break` | bool | `false` | Ends the section after this line and starts the next on a new page. This is what activates `page.continuation_margins`. |
| `tabs` | list | — | Tab stops for this line, e.g. a right-aligned date. |

### `runs`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `style` | string | — | A **character** style from `styles:`. |
| `text` | string | — | The run's text, interpolated. |
| `bold`, `italic` | bool | `false` | |
| `size` | font size | — | |
| `color` | colour | — | |
| `tab` | bool | `false` | A tab stop before the text. |
| `break` | bool | `false` | A line break before the text. |
| `omit_if_empty` | bool | `true` | Drops just this run when its interpolation is empty, leaving the rest of the line intact. |

## Inheritance

A house style is one letterhead and one gallery across a dozen document types.
`extends:` lets each type carry only what makes it different:

```yaml
# .docc/themes/_house.yaml — a fragment: it exists to be extended
name: _house
page: { size: A4, title_page: true, margins: { top: 20mm } }
defaults: { font: Arial, size: "11pt" }
formats: { date: "2. January 2006", months: [Januar, Februar, …] }
styles:
  body:  { size: "11pt" }
  titel: { size: "14pt", bold: true }
header:
  first:
    - { image: { path: logo.png, width: "45mm", height: "12mm" } }
```

```yaml
# .docc/themes/protokoll.yaml
extends: _house
styles:
  titel: { size: "16pt" }
```

A new address, a new logo or a new body font is then one edit, not a dozen.

**A theme whose name begins with `_` is a fragment.** It is not listed by
`docc themes` and a schema that selects it is an error: it exists to be
extended. The name defaults to the file name, so `_house.yaml` needs no `name:`.

Resolution is within one theme directory. There is no cross-pack parent —
profile packs are still never merged, and a base pack silently changing header
spacing in every firm that installed it is the failure a compiler exists to
prevent. Naming an unknown parent is an error, and so is a cycle.

### Merge rules

Inheritance merges the YAML documents, then decodes once. That is what lets a
child set a value back to zero — `title_page: false` under a parent that sets
it true — which a merge over decoded structs cannot express, because it cannot
tell a key left out from a key set to its zero value.

| Kind | Rule |
|---|---|
| mappings (`styles`, `numbering`, `header`, `footer`, `page`, `formats`, …) | Merged key by key, at every depth. A child changing one style keeps the rest of the gallery. |
| sequences (`prologue`, `epilogue`, each `[]` under a `header`/`footer` key) | Replaced wholesale when the child declares them. Half a letterhead is more confusing than a restated one. |
| scalars | Replaced when the child declares them, including with a zero value. |
| `name` | Never inherited. Two themes with one identity would make the second unreachable. |

`extends:` is transitive — house style, then practice area, then document type —
and matches how schema `extends:` already works. Two different inheritance
semantics in one product would be a defect, not a feature.

## What a theme cannot do

Two ceilings, both deliberate, both stated so they are not discovered:

1. **The property vocabulary above is the whole vocabulary.** Word features it
   does not name — character spacing, highlighting, banded table styles — are
   unreachable without extending `internal/theme` and the writer.
2. **Some constructs never reach a style at all.** `**bold**`, `*italic*`,
   inline `` `code` `` (always Courier New), links (always `0000EE`, underlined,
   and rendered as text rather than a live hyperlink), and table borders and
   column widths (a 0.5pt grid, columns split evenly). A schema that maps one of
   these is not overridden but ignored, which is why `docc doctor` reports it.

See the [What a theme cannot change](authoring.md#what-a-theme-cannot-change).

## Validation

`emit.Validate` holds a schema and a theme together and checks, before a single
paragraph is built:

- every style the schema maps exists here, as a style or a numbering definition;
- every `{{ path }}` the furniture interpolates resolves through the schema's
  frontmatter and object types;
- every definition a `render:` rule names exists, and the rule says where
  numbering starts exactly once;
- every numbering definition is one Word will render — nine levels or fewer, a
  known `suffix`, no nested `levels:`;
- every furniture line's `numbering:` names a definition that exists.

`docc doctor` runs all of it for every schema in the project. It used to run only
as a side effect of building a valid document.
