# Configured output numbering

`docc` can apply numbering that the Markdown source should not carry: a heading
outline and a continuous marginal paragraph number. The feature is generic;
legal briefs are one use case.

## Configuration

The schema selects where numbering applies and the theme selects how it looks:

```yaml
# schema
render:
  heading_numbering:
    definition: HeadingOutline
    start_at_heading: RECHTSBEGEHREN
  paragraph_numbering:
    definition: ParagraphNumber
    start_after_heading: RECHTSBEGEHREN
```

```yaml
# theme
numbering:
  HeadingOutline:
    format: upperRoman
    text: "%1."
    indent: 0mm
    hanging: 10mm
    style: Ueberschrift1
    levels:
      - { format: upperLetter, text: "%2.", indent: 0mm, hanging: 10mm, style: Ueberschrift2 }
      - { format: decimal, text: "%3.", indent: 0mm, hanging: 10mm, style: Ueberschrift3 }
  ParagraphNumber:
    format: decimal
    text: "%1."
    size: 8pt
    align: right
    suffix: space
    indent: 0mm
    hanging: 7mm
    style: Standard
```

`start_at_heading` numbers the marker heading itself. `start_after_heading`
leaves it unnumbered and begins at the following prose. Setting both on one
rule is a schema error. With neither setting, the rule covers the entire body.

## Rendering rules

- Heading depth chooses the numbering level: `#` is level 0, `##` is level 1.
- Only top-level body blocks are eligible for paragraph numbering. List items,
  table cells, quotations, fenced-div content, and code are excluded.
- A numbered heading outline and numbered body prose each share one Word
  numbering instance, so their sequences continue across the document.
- Labels are native Word numbering, not text prefixes. Reordering blocks lets
  Word update the labels without running `docc` again.
- A theme's `levels:` is a flat list, capped at nine Word numbering levels.

## Validation and tests

`emit.Validate` checks that a selected definition exists in the theme and that
markers and numbering definitions are valid. The regression suite covers shared
instances, start markers, nested-content exclusion, flat levels, and generated
`numbering.xml` parts.

The visual geometry remains a project responsibility. Measure and render a
representative document before adopting a theme for real use; the compiler can
guarantee structure, not that a chosen margin is suitable for a particular
stationery layout.
