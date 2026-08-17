<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/banner-dark.svg">
  <img src="docs/img/banner.svg" alt="docc" width="410">
</picture>

`docc` compiles Markdown into Word documents, and refuses to compile the ones
that don't match their document type.

[![go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## Description

The documents I produce — deeds, contracts, letters to authorities, filings —
all come out of Word templates. Someone opens the `.dotx` on the shared drive,
saves a copy, fills in the grey bits and sends it. That works until it doesn't:

- A reference number is in the wrong format, and nobody notices until the
  registry writes back.
- A placeholder never got filled in, so the document goes out with `[Name]` in
  the middle of it.
- The payments in a purchase deed don't add up to the price.
- Two copies of the same template have drifted, so the margins depend on which
  one you happened to start from.
- Someone fixed the letterhead in their copy instead of in the template.

None of these are hard problems. They are all things a machine can check. Word
doesn't check them because a template is a starting point, not a contract —
it will happily produce a document that is wrong.

Drafting in Markdown and converting doesn't help. The conversion is never quite
right, so you fix it in Word, and now the Word file is the real document and the
Markdown is a draft you throw away. A template engine fixes the drift, but it
takes the document away from you: the prose ends up split between a data file
and a form with holes in it, and what I need to do with a draft is read it.

Then models got good enough to draft, which moved the hard part. Writing is
fast; getting a specific, correct output is not. "Parties in this order, amounts
spelled out under the figures, the retention clause, the certification on its
own page" is a hundred small constraints, and a model will satisfy ninety-eight
of them. Re-reading every section to find the other two gives back everything it
saved. Instructions in a prompt are a request; I wanted a check.

So the constraints moved out of the prompt and into a schema the document is
compiled against, and the document stayed prose:

```
docs/klage_mueller.md:14:13: error[DOC010]: field `case_ref` has malformed value "ZG2026000"
   |
14 |   case_ref: "ZG2026000"
   |             ^^^^^^^^^^^ court reference in the form AA.YYYY.NNN, e.g. "ZG.2026.000"
```

Markdown and YAML go in, a `.docx` comes out, and nothing renders until the
document satisfies its type.

## How it works

**The document type is a schema.** Every document declares its type in the
frontmatter. The schema for that type says which fields are required, what the
body must contain, which blocks and spans are allowed, and which rules hold
across fields — that the payments settle the price, for instance. All the checks
run in one pass, so fixing one error at a time doesn't mean running the compiler
ten times. Every diagnostic has a source position and a hint; one that says what
is wrong but not what to do is treated as a bug.

```bash
docc describe ch_letter      # what the type actually requires
docc check --json brief.md   # machine-readable, for CI or an agent
docc lsp                     # the same checks in the editor, while typing
```

**The source is Markdown.** Ordinary Markdown with YAML frontmatter — no macros,
no XML, no special editor. It diffs, so I can review a change to a document the
way I review a change to code. It also means a language model can draft one
without being taught a format first.

**It writes the `.docx` itself.** No Word, no template file, no LibreOffice in
the loop. The writer builds the archive with the Go standard library and nothing
else, so there's no template to keep in sync and no inherited corruption. Output
is deterministic — the same input gives a byte-identical file on my laptop and
on a CI runner, which is the only reason diffing a document is worth anything.

**Layout lives in a theme.** Letterhead, fonts, margins, numbering, signature
blocks: all in the theme, none in the document. An author can't override the
house style by accident, because the document never contains it.

**Schemas and themes live in one repository.** A *profile pack* is a Git repo
holding both. Projects pin it with a lockfile, so a build is reproducible and
works offline, and every rendered file records which profile and commit produced
it. Changing an address or a font is one edit in one place. Optionally, installs
can be made to require a signature from a key you named.

## Quick start

```bash
go install github.com/kevinzehnder/docc/cmd/docc@latest

docc init                    # a standalone starter project in this directory
docc types                   # what document types exist
docc example ch_letter > brief.md
docc check brief.md          # validate, with positions and hints
docc build brief.md          # → brief.docx
```

If you already have a profile pack, bind it instead of running `init`:

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
and the language server stays quiet.

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

Pre-1.0, and written for my own work. I use it daily on real documents, which
is the only testing it has had beyond its own suite. There is no company behind
it and no roadmap beyond what I need next; it's public because that is easier
than keeping it in a folder.

Working: `check`, `build`, `doctor`, `lsp`, `init`, `describe`, `example`,
`explain`, the `.docx` writer, schema and theme inheritance, profile packs with
lockfiles and an optional signature policy, provenance stamped into every
output, and `docc profile package`.

`.docx` is the supported output; `--to pdf` is a convenience export that shells
out to LibreOffice. Diagnostic codes are stable and won't be renumbered.
Everything else may still move.

Rough edges and planned work are in [docs/next-steps.md](docs/next-steps.md).
Issues and patches are welcome, but I make no promises about response time.

## Contributing

```bash
task                      # the full CI chain: format, vet, lint, test, build
task test:golden:update   # after reviewing a golden diff — never before
```

`testdata/` is the regression suite. A change to a message is supposed to fail
the golden tests; read the diff first, because that is the check working.
[CLAUDE.md](CLAUDE.md) documents the conventions the codebase holds itself to,
and [docs/development.md](docs/development.md) the rest.

## License

MIT — see [LICENSE](LICENSE).
