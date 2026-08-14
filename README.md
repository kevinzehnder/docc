# docc

`docc` turns structured Markdown into validated, house-style Word and PDF
documents.

It is built for a common modern writing workflow: people and language models
write Markdown, while clients, courts, administrations, and colleagues still
expect editable `.docx` files or finished PDFs with exact layout conventions.

A document declares its format and document type in YAML frontmatter:

```markdown
---
docc: 1
document_type: letter
recipient:
  name: Jane Example
  street: Example Street 1
  postal_code: "8000"
  city: Zürich
date: 2026-08-14
subject: Example letter
---

Dear Ms Example,

This text remains pleasant to write and review as Markdown.
```

The project schema defines which facts and structure are required. The project
theme defines fonts, page geometry, styles, letterhead, address blocks,
numbering, headers, footers, and other fixed furniture. A failed contract
produces source-positioned diagnostics instead of a subtly wrong document.

## Install

Until versioned binaries are published, build from source:

```sh
go install github.com/kevinzehnder/docc/cmd/docc@latest
```

Or from a checkout:

```sh
task build
./bin/docc version
```

PDF output additionally requires LibreOffice (`soffice` or `libreoffice`) on
`PATH`. DOCX generation itself has no external runtime dependency.

## Quick start

```sh
mkdir my-documents
cd my-documents

docc init
docc check examples/docc/letter.md
docc build examples/docc/letter.md
docc build --to pdf examples/docc/letter.md
```

`docc init` creates:

```text
.docc/
  schemas/
  themes/
examples/docc/
.agents/skills/docc/
```

The starter contains generic letter and Swiss-legal examples. Replace its
placeholder identity and layout values before producing real documents.

## Commands

```sh
docc init [directory]             create a starter project
docc check document.md            validate a document
docc check --json document.md     emit machine-readable diagnostics
docc check --strict document.md   treat warnings as errors
docc build document.md            create document.docx
docc build --to pdf document.md   create document.pdf
docc build -o out.docx document.md
docc types                        list available document types
docc themes                       list available themes
docc explain DOC010               explain an engine diagnostic
docc lsp                          serve editor diagnostics over stdio
docc version
```

Exit codes:

| Code | Meaning |
| ---: | --- |
| 0 | Success; validation is clean |
| 1 | Diagnostics or build failure |
| 2 | Usage or configuration error |

## Project model

`docc` finds the nearest `.docc` directory by walking upward from the input
file, like Git finds `.git`.

```text
project/
  .docc/
    schemas/
      _base.yaml
      letter.yaml
    themes/
      house-letter.yaml
  documents/
    letter.md
```

Use `--schema-dir` or `--theme-dir` to override discovery.

A schema is the content contract:

```yaml
type: letter
extends: base
description: Business letter
theme: house-letter

frontmatter:
  subject:
    type: string
    required: true
  date:
    type: date
    required: true

styles:
  paragraph: Standard
  h1: Heading1
```

A theme is presentation:

```yaml
name: house-letter
description: House letter layout

page:
  size: A4
  margins:
    top: 20mm
    bottom: 20mm
    left: 25mm
    right: 20mm

defaults:
  font: Arial
  size: 11pt
  lang: de-CH
```

Schemas may also define reusable types, body structure, named validation rules,
style mappings, defaults, and automatic numbering. Themes may define named
styles, numbering definitions, headers, footers, images, and fixed prologue or
epilogue furniture.

The engine remains generic. Organisation-specific or legal conventions belong
in project schemas and themes, never in Go conditionals.

## Diagnostics and agent workflow

Diagnostics carry stable codes, source positions, and actionable hints:

```text
documents/claim.md:14:13: error[DOC010]: field `case_ref` has malformed value
```

A reliable agent loop is:

1. Inspect the available types and themes.
2. Read the selected schema and theme.
3. Write Markdown and frontmatter without duplicating theme furniture.
4. Run `docc check --json`.
5. Correct all errors without inventing missing facts.
6. Build only after validation succeeds.
7. Review legally or operationally significant output before delivery.

The portable Agent Skill in [`skills/docc`](skills/docc/SKILL.md) encodes this
workflow for ChatGPT Work, Claude, Cowork, and compatible agent harnesses.
See [publishing-agent-skill.md](docs/publishing-agent-skill.md) for packaging.

## Output guarantees

DOCX output is deterministic: identical input and configuration produce
byte-identical archives. Archive timestamps are fixed, parts are sorted, and
identifiers are assigned by document position.

PDF conversion is performed by LibreOffice. Its pagination and typography
therefore also depend on the installed LibreOffice version and fonts. Production
themes must be verified against their intended render environment and physical
output requirements.

## Development

```sh
task                        # formatting, vet, lint, tests, build, skill checks
task test                   # unit and golden tests
task test:race              # race detector
task test:roundtrip         # LibreOffice DOCX/PDF round trip
task test:golden:update     # deliberately update reviewed golden fixtures
task hooks:install
```

The regression corpus validates diagnostics and compares generated OOXML parts.
Do not regenerate golden files without reviewing the diff.

Current hardening and release work is tracked in
[production-readiness.md](docs/production-readiness.md).
