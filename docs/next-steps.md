# docc maintenance notes

`docc` is a generic engine. Project-specific document conventions belong in a
project's `.docc/schemas/` and `.docc/themes/` directories, not in Go code.

## Current scope

The supported compiler output is DOCX. The standalone Cowork AgentSkill ZIP is
the supported agent distribution artifact. PDF conversion is optional host
integration work; the CLI's LibreOffice path is compatibility-only.

The maintained quality checks are:

- `task ci` for formatting, vetting, linting, unit tests, and the CLI build.
- `task test:race` for race detection.
- `task test:roundtrip` for optional LibreOffice compatibility.
- `task release:skill` to assemble, unpack, probe, and checksum the AgentSkill
  ZIP.

## Priorities

### Validate real themes

The generic starter themes are examples, not approved stationery. Before a
theme is used for correspondence or filings, compare generated documents to
approved references and print-test physical envelope layouts. Coordinates and
visual inspection catch different failures; use both.

Do not add real client material, contact information, or production themes to
this repository's fixtures. Use invented identities and domains in `testdata/`.

### Improve authoring ergonomics when evidence demands it

- A formatter could apply mechanical corrections and reduce agent iteration.
- An MCP wrapper can be a thin integration over `check`, `build`, `types`, and
  `themes`; the JSON command contract is the integration surface.
- Keep editor functionality in the LSP rather than weakening schemas to make
  drafts pass.

### Preserve configuration semantics

- Schema and theme options need both a parser and a reader. Test new options at
  the point where they affect a document.
- A theme's `levels:` list is flat: the definition is level 0 and `levels[i]`
  is level `i+1`, up to nine levels.
- Word numbering has two levels of indirection: paragraphs name a `numId`, which
  names an abstract definition. Shared instances continue a sequence.
- Theme furniture interpolates typed metadata, not raw YAML values.
- Diagnostic positions are byte offsets; renderers convert them for display.

## Backlog

- Add focused negative tests for project discovery and schema loading.
- Validate emitted packages against the OOXML standard: package invariants in
  `internal/docx`, then ECMA-376 XSD validation in CI. Plan and traps in
  [ooxml-conformance.md](ooxml-conformance.md).
- Make `docc init` transactional so an I/O failure cannot leave a partial
  starter project.
- Decide whether to retire the CLI PDF exporter in a future breaking release
  once AgentSkill hosts cover the requested workflows.
- Follow [public-release.md](public-release.md) when preparing a public release.
