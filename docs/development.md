# Development

Building, testing, and the rules the `.docx` writer holds itself to.

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
