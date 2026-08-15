<div align="center">

<img src="docs/img/banner.svg" alt="docc — documents that compile" width="560">

<br>

**Documents that compile.**

A document has a type. The type is a contract. The contract fails at a line and<br>
a column — before anything reaches a court, a client or a counterparty.

[![ci](https://github.com/kevinzehnder/docc/actions/workflows/ci.yml/badge.svg)](https://github.com/kevinzehnder/docc/actions/workflows/ci.yml)
[![go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![dependencies](https://img.shields.io/badge/dependencies-2-brightgreen)](go.mod)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

</div>

---

```
docs/klage_mueller.md:14:13: error[DOC010]: field `case_ref` has malformed value "ZG2026000"
   |
14 |   case_ref: "ZG2026000"
   |             ^^^^^^^^^^^ court reference in the form AA.YYYY.NNN, e.g. "ZG.2026.000"
```

A prose style guide is a contract nothing enforces. `docc` makes it one that
does: Markdown and YAML go in, a validated Word `.docx` comes out, and anything
that does not satisfy the document's type never renders at all.

## Why docc

### The contract is machine-checkable

Every document declares its type. The schema for that type decides which fields
are required, what the body must contain, which blocks and spans are permitted,
and which cross-field rules hold. Nothing renders until all of it passes.

Every diagnostic carries a source position **and** a hint. A message that says
what is wrong but not what to do is treated as incomplete, and all checks run in
one pass — an author fixing one error at a time runs the compiler ten times.

```bash
docc describe ch_letter      # the contract, in full
docc check --json brief.md   # machine-readable, for agents and CI
docc lsp                     # the same checks in the editor, while typing
```

This is the part with no obvious substitute. Converters convert whatever they
are handed; `docc` refuses.

### Markdown your tools — and your model — already speak

The source is ordinary Markdown with YAML frontmatter. No macros, no XML, no
proprietary editor. It reviews well in a pull request, and a language model can
draft it without being taught anything.

Which is exactly why the contract matters. Drafting is cheap now; *correct* is
the scarce part. A model can write the document — the compiler decides whether
it is allowed to ship.

### Predictable Word output, without Word

Nobody opens Word to produce the file, and no template `.docx` sits in a shared
drive slowly rotting. The writer builds the archive from nothing, with the Go
standard library alone — no template to keep in sync, no inherited corruption,
no styles pane.

Output is **deterministic**: identical input renders byte-identical output. The
same source produces the same file on a laptop, a colleague's desktop and a CI
runner. A document you can diff is a document you can review.

### Identity that cannot drift

Letterhead, fonts, margins, numbering and signature blocks live in the theme,
not in the document. An author cannot override them by accident, because the
document never contains them in the first place.

Schemas and themes ship as a Git-managed **profile pack**, pinned by a committed
lockfile, so a build is reproducible and offline. A trust policy can require a
signature from a key you named. Every rendered file records which profile and
which commit produced it, so "who approved this letterhead, and when" is
answerable from the document itself.

### Central management of templating

One repository holds the organisation's document types and layouts. Projects
bind to it; updates arrive as a reviewed lockfile change, not as an email with
an attachment. A new address, a new logo or a new house font is one edit, in one
file, and every document type inherits it.

`docc profile package` turns the same pack into an AgentSkill whose instructions
are generated from the schemas, so they cannot drift from the types they
document.

## Quick start

```bash
go install github.com/kevinzehnder/docc/cmd/docc@latest

docc init                    # a standalone starter project in this directory
docc types                   # what document types exist
docc example ch_letter > brief.md
docc check brief.md          # validate, with positions and hints
docc build brief.md          # → brief.docx
```

Already have an organisation profile? Skip `init` and bind the pack:

```bash
docc profile use ssh://git.example.ch/kanzlei/docc-profiles.git
docc build brief.md
```

A file becomes a docc document by declaring the marker in its frontmatter:

```yaml
---
docc: 1          # the docc format version — this is what makes it a docc file
document_type: ch_legal
---
```

Files without the marker — READMEs, notes, Hugo or Obsidian posts with their own
frontmatter — are not docc documents. `docc check` reports `DOC024` for them,
and the language server stays silent.

## Documentation

| | |
|---|---|
| [Quick reference](docs/cli.md) | Every command, its flags, exit codes, the JSON contract |
| [Authoring guide](docs/authoring.md) | Document types, blocks, spans, body rules — the narrative |
| [Schema reference](docs/schema-reference.md) | Every key a document type may declare |
| [Theming guide](docs/theming.md) | Styles, numbering, letterhead furniture, house-style inheritance |
| [Theme reference](docs/theme-reference.md) | Every key a theme may set, and what it cannot do |
| [Profile packs](docs/profile-packs.md) | Git-managed schemas and themes, trust policy, AgentSkill packaging |
| [Building profiles](docs/building-profiles.md) | Deriving a profile from an existing Word document |
| [Projects](docs/projects.md) | Configuration layout and discovery |
| [Editor integration](docs/editors.md) | Language server setup |
| [Development](docs/development.md) | Building, testing, and the `.docx` writer's rules |

## Status

Pre-1.0, and in daily use on real filings.

Working: `check`, `build`, `doctor`, `lsp`, `init`, `describe`, `example`,
`explain`, the `.docx` writer, schema and theme inheritance, Git-managed profile
packs with lockfiles and an optional signature policy, provenance stamped into
every output, and `docc profile package`.

`.docx` is the supported output. `--to pdf` is a compatibility export that needs
LibreOffice. Diagnostic codes are stable and will not be renumbered.

Planned work is tracked in [docs/next-steps.md](docs/next-steps.md).

## Contributing

```bash
task                      # the full CI chain: format, vet, lint, test, build
task test:golden:update   # after reviewing a golden diff — never before
```

`testdata/` is the regression suite. A change to a message is expected to fail
the golden tests; read the diff first, because that is the check working.
[CLAUDE.md](CLAUDE.md) documents the conventions the codebase holds itself to,
and [docs/development.md](docs/development.md) the rest.

## License

MIT — see [LICENSE](LICENSE).
