# Theming guide

How a theme turns a validated document into a laid-out one. This is the
narrative; [theme-reference.md](theme-reference.md) is the exhaustive key list.

## Themes

> Every theme key, with its accepted values and defaults, is in
> **[theme-reference.md](theme-reference.md)**.


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

### Sharing a house style

A firm has one letterhead and one gallery across a dozen document types.
`extends:` lets each type declare only what makes it different:

```yaml
# .docc/themes/_house.yaml — a fragment, extended but never selected
name: _house
defaults: { font: Arial, size: "11pt" }
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
  titel: { size: "16pt" }      # the only difference
```

A new address or a new logo is then one edit rather than a dozen. Mappings merge
key by key at every depth, so changing one style keeps the rest of the gallery;
ordered furniture is replaced wholesale when the child declares it. A theme whose
name begins with `_` is a fragment: `docc themes` does not list it and a schema
that selects it is an error. `extends:` is transitive, and it works the same way
[schema `extends:`](schema-reference.md#how-extends-merges) does.

Inheritance stays inside one theme directory. Profile packs are never merged.

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
