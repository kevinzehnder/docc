<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/banner-dark.svg">
  <img src="docs/img/banner.svg" alt="docc" width="410">
</picture>

# Documents that pass a check before they become Word files

> **Markdown in → document contract verified → `.docx` out**

`docc` is a deterministic compiler for structured Markdown documents. Write the
letter, filing or deed as readable prose. `docc` verifies its structure, facts
and declared rules before it renders the Word document people actually use.

[![go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[Start with a checked letter](https://kevinzehnder.github.io/docc/getting-started.html)** ·
**[Read the product guide](https://kevinzehnder.github.io/docc/)** ·
**[Produktleitfaden auf Deutsch](https://kevinzehnder.github.io/docc/de/)**

---

## A file that opens is not a document you can trust

Word accepts almost anything: a recipient address without a postal code, a
literal `[Name]`, a heading in the wrong place, a case reference in the wrong
form. A template is only a starting point; it cannot prove that the document
now meets its requirements.

The usual alternatives lose something important:

- drafting in Markdown and converting drifts back into manual Word repair;
- template systems split prose into a data file and a form with holes;
- a prompt can request a constraint, but it cannot check that the constraint
  survived the draft.

`docc` keeps the authored document readable and moves the requirements into a
schema. It is a CLI, so a person, CI job, editor or drafting agent can run the
same loop: write, compile, read the diagnostics, correct the source, repeat.

```text
docs/klage_mueller.md:14:13: error[DOC010]: field `case_ref` has malformed value "ZG2026000"
   |
14 |   case_ref: "ZG2026000"
   |             ^^^^^^^^^^^ court reference in the form AA.YYYY.NNN, e.g. "ZG.2026.000"
```

## The compiler contract

| Input | The compiler verifies | Output |
|---|---|---|
| **Markdown source** | The document marker, selected type, frontmatter values, body structure, semantic markup and named rules | **A `.docx` only after validation passes** |
| **Schema** | Required fields, types, patterns, headings, blocks, spans, intentional blanks and document-specific checks | Diagnostics with stable codes, source positions and hints |
| **Theme** | That the schema’s styles, references and values can actually be rendered | Approved layout: letterhead, fonts, margins, lists, headers and footers |

This is the boundary that matters: `docc` compiles authored documents. It does
not invent missing facts, host a service, operate a template language or hide
your prose in a database.

```text
Markdown + YAML
      ↓ parse
Document structure
      ↓ verify against the schema
Checked document
      ↓ render through the theme
Deterministic .docx
```

A successful build means more than “Word could open the file.” It means the
source satisfied the document type and the selected profile could render it.

## Start with one letter

```bash
go install github.com/kevinzehnder/docc/cmd/docc@latest

# The built-in starter profile is ready to use.
docc types
docc example ch_letter > brief.md

# Inspect the contract; then edit the readable source file.
docc describe ch_letter
docc check brief.md

# Build validates first, then writes brief.docx.
docc build brief.md
```

The example is ordinary Markdown with YAML frontmatter:

```yaml
---
docc: 1
document_type: ch_letter
subject: Mahnung — Rechnung 2026-114
recipient:
  name: Clara Muster
  postal_code: "3000"
---

Sehr geehrte Frau Muster
```

If your organisation already has a profile pack, bind it explicitly instead of
initialising the starter:

```bash
docc profile use ssh://git.example.ch/kanzlei/docc-profiles.git
docc build brief.md
```

## Why the source stays readable

**The document remains prose.** Standard Markdown covers paragraphs, headings,
lists and quotations. YAML frontmatter holds facts such as sender, recipient,
date and subject. The file stays reviewable in a diff and editable in any text
editor.

**Structure is added only when it has meaning.** A schema may declare semantic
`:::blocks`, inline `[spans]{.type}` and intentional `docc-field` blanks for a
document that needs them. The author still sees the words; the compiler gains
the facts it must check.

**Layout lives in a theme.** An author never nudges an address window or repairs
a footer in an old copy of a Word file. The theme owns stationery and rendering;
the schema owns validity; the document owns its content.

**The result is repeatable.** `docc` writes the `.docx` archive itself—no Word,
template file or LibreOffice in the rendering loop. Identical source, schema and
theme produce byte-identical output. Each rendered file records the profile and
revision that produced it.

## Explore the system

| Start here | Go deeper |
|---|---|
| [Write a checked letter](https://kevinzehnder.github.io/docc/getting-started.html) | [How the compiler verifies documents](https://kevinzehnder.github.io/docc/compiler.html) |
| [Author structured Markdown](https://kevinzehnder.github.io/docc/authoring.html) | [Profiles, schemas and themes](https://kevinzehnder.github.io/docc/profiles.html) |
| [Product documentation map](https://kevinzehnder.github.io/docc/reference.html) | [Deutscher Produktleitfaden](https://kevinzehnder.github.io/docc/de/) |

## Canonical documentation

| | |
|---|---|
| [Philosophy](docs/philosophy.md) | Product identity, non-goals and the deliberate scope boundary |
| [Quick reference](docs/cli.md) | Every command, flag, exit code and JSON contract |
| [Authoring guide](docs/authoring.md) | Document types, Markdown, blocks, spans and fields |
| [Schema reference](docs/schema-reference.md) | Every schema key and validation option |
| [Theme reference](docs/theme-reference.md) | Theme layout, styles and rendering definitions |
| [Profile packs](docs/profile-packs.md) | Git-managed schemas and themes, locks and provenance |
| [Building profiles](docs/building-profiles.md) | Deriving an approved profile from an existing document standard |
| [Projects](docs/projects.md) | Configuration layout and profile discovery |
| [Editor integration](docs/editors.md) | Language-server setup and in-editor diagnostics |
| [Development](docs/development.md) | Tests, golden output and `.docx` writer rules |

## Status

Pre-1.0 and written for real document work. Working: `check`, `build`,
`doctor`, `lsp`, `init`, `describe`, `example`, `explain`, the `.docx` writer,
schema and theme inheritance, profile packs with lockfiles and optional
signature policy, the embedded starter pack, and provenance stamped into every
output.

`.docx` is the supported output. `--to pdf` is a convenience export that shells
out to LibreOffice. Diagnostic codes are stable and will not be renumbered;
everything else may still move. Scope and planned work live in
[docs/philosophy.md](docs/philosophy.md).

## Contributing

```bash
task                      # full CI: format, vet, lint, test, build
task test:golden:update   # after reviewing a golden diff, never before
```

`testdata/` is the regression suite. A changed message is supposed to fail the
golden tests; read the diff first. [CLAUDE.md](CLAUDE.md) documents the project
conventions and [docs/development.md](docs/development.md) the rest.

## License

MIT, see [LICENSE](LICENSE).
