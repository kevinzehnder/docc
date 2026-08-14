# docc contributor guide

`docc` compiles schema-backed Markdown into deterministic DOCX and, through
LibreOffice, PDF. The product boundary is deliberately narrow: parse, validate,
and render documents. Project-specific conventions live in `.docc`; the Go
engine remains generic.

Read `README.md` for usage and `docs/production-readiness.md` for the current
hardening plan.

## Layout

```text
cmd/docc/            CLI
internal/diag/       diagnostics and JSON rendering
internal/parse/      Markdown/frontmatter parsing and source positions
internal/schema/     schema loading and inheritance
internal/sema/       semantic validation
internal/ir/         renderer-independent document model
internal/theme/      theme loading and conversion
internal/emit/       schema/theme validation and document emission
internal/project/    .docc discovery
internal/starter/    embedded starter project
internal/lsp/        editor diagnostics over LSP
pkg/docx/            deterministic OOXML writer
skills/docc/         portable Agent Skill source
testdata/            validation and OOXML golden corpus
```

## Product boundaries

- Keep the engine focused on Markdown → validated DOCX/PDF.
- Do not add document ingestion, OCR, VLM, retrieval, storage, collaboration, or
  network services to this repository.
- Keep law, language, organisations, letterheads, and house style out of Go.
  Express them in project schemas and themes.
- Do not turn themes into programs. Prefer a separate theme or explicit
  frontmatter value over conditionals in YAML.
- Add dependencies only when the standard library cannot reasonably do the job.
- Treat new public commands, YAML fields, diagnostic codes, and exported Go
  identifiers as compatibility commitments.

## Diagnostics

- Give every diagnostic a useful source position and actionable hint.
- Collect independent failures in one pass instead of stopping at the first.
- Keep released diagnostic codes stable.
- Register new named rules in `internal/sema/rules.go` and document their
  schema-facing behaviour.
- Add engine diagnostic explanations to `cmd/docc/main.go`.
- Keep stdout machine-readable when `--json` is selected; send operational
  errors to stderr.

`diag.Position.Col` and `.Len` are byte offsets. Convert them to runes only
when presenting text. The corpus intentionally contains non-ASCII characters.

## Schema and theme rules

- Parse configuration strictly. Unknown explicit values must fail; omission may
  use a documented default.
- Validate schema/theme relationships in `emit.Validate`, the layer that owns
  both inputs.
- Check that every schema style, numbering definition, and theme interpolation
  resolves before rendering.
- A schema `default:` is applied during semantic analysis so it affects both
  required-field checks and emitted metadata.
- A theme's `levels:` list is flat: the definition is level 0 and each item is
  the next level, up to Word's limit of nine.
- Preserve the distinction between an omitted measurement and an explicit zero.

## DOCX writer

The writer uses the standard library. OOXML is emitted through the local XML
writer because namespace prefixes and element ordering matter to Word.

- Preserve byte-for-byte deterministic output.
- Use fixed archive timestamps, sorted parts, and position-derived identifiers.
- Do not introduce process-global counters or wall-clock values.
- Keep measurement units distinct: `Twips`, `EMU`, `HalfPt`, and `Eighth`.
- Ensure every table cell, header, footer, and document body contains the
  structures Word requires.
- Use `Numbering.AddList` and `Numbering.NewInstance`; independent lists need
  independent `numId` values.
- Run the LibreOffice round-trip test after structural writer changes.

## Testing

Run before committing:

```sh
task
```

Useful focused commands:

```sh
task test
task test:race
task test:roundtrip
task agent:test
```

The golden corpus contains:

- `testdata/good/`: documents that must validate;
- `testdata/bad/`: intentional failures;
- `*.golden`: rendered diagnostics;
- `testdata/golden/<fixture>/`: generated `word/*.xml` parts.

A changed message should fail diagnostic goldens. A writer or theme change
should fail OOXML goldens. Review the diff before running
`task test:golden:update`.

Never copy real client, court, employee, or firm data into fixtures. Use invented
identities and domains.

## Parser notes

- Goldmark's `Lines()` panics on inline nodes; guard for block nodes.
- A `ListItem` stores text in descendant blocks, not on the list item itself.
- Fenced divs are implemented locally in `internal/parse/fences.go`.

## Release discipline

- Pin CI tool versions.
- Build release artifacts only from a clean version tag.
- Test the exact binaries and skill archives that will be published.
- Record checksums for release artifacts.
- Treat DOCX determinism separately from PDF rendering reproducibility: PDF
  layout depends on LibreOffice and installed fonts.
