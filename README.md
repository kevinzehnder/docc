# docc

A compiler for structured documents.

`docc` treats a markdown document with YAML frontmatter as source code: it parses
it, checks it against a schema, and reports errors with source positions and
actionable hints. The intended backend emits Word `.docx` from a Word-authored
template.

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
docc check docs/klage.md          # validate
docc check --json docs/*.md       # machine-readable, for agents and CI
docc check --strict docs/klage.md # warnings become errors
docc types                        # list known document types
docc explain DOC010               # describe a diagnostic code
```

Exit codes: `0` clean, `1` diagnostics reported, `2` usage or configuration error.

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

This split means changing a letterhead is a file edit, not a compiler release,
and one engine serves projects whose document conventions have nothing in common.

## Schemas

A schema declares frontmatter fields and their types, the body structure, the
markdown-to-Word-style mapping, and which named rules to run.

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
    check: beweis_beilage_refs
  - id: LEG020
    check: beilagen_coverage
    severity: warning
```

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

| check | what it catches |
|---|---|
| `no_placeholder_text` | template placeholders like `[Sachverhaltsschilderung]` that were never filled in |
| `beweis_beilage_refs` | evidence items in a `::: beweis` block with no `// Beilage N` reference |
| `beilagen_coverage` | exhibits cited in the body but missing from `beilagen`, and vice versa |
| `no_empty_sections` | a heading with no content beneath it |

Each rule supplies its own diagnostic code, and may override the message,
severity and hint.

## Development

```bash
task              # full CI: fmt, vet, lint, test, build
task test         # unit tests
task test:golden:update   # regenerate testdata/**/*.golden
task hooks:install
```

The golden corpus in `testdata/` is the regression suite. Every fixture is
checked against `testdata/schemas/` and its rendered diagnostics compared to a
committed `.golden` file, so any change to a message or a rule shows up as a
diff rather than as a surprise in a real document.

## Status

`docc check` is implemented. The `.docx` emitter is not yet — `pkg/docx` is a
placeholder. Until it lands, rendering still runs through the existing pipeline.
