# docc

A compiler for structured markdown documents. Frontend parses and validates
markdown + YAML frontmatter against a schema; the backend (not yet written)
emits `.docx` from a Word-authored template.

See `README.md` for the schema format and CLI usage.

## Layout

```
cmd/docc/            CLI: check | types | explain | version
internal/diag/       Diagnostic type, source-excerpt and JSON rendering
internal/parse/      goldmark wrapper: frontmatter split, block tree, positions
internal/schema/     doc-type spec loading and `extends` resolution
internal/sema/       validation passes → diagnostics
internal/project/    .docc directory discovery
pkg/docx/            .docx writer — stdlib only, no template, deterministic
testdata/            golden corpus: schemas/, good/, bad/
```

## Conventions

- `task` runs the full CI chain. Run it before committing.
- Formatting is `gofumpt`, enforced by the pre-commit hook (`task hooks:install`).
- Dependencies stay minimal: `goldmark` for markdown, `goccy/go-yaml` for YAML
  positions. Do not add a dependency for something the stdlib does.

## Working on the checker

- **Every diagnostic needs a source position and a hint.** A message that says
  what is wrong but not what to do is incomplete. Anchor on a line that actually
  relates to the problem — a caret under an unrelated line is worse than a
  file-level diagnostic.
- **Diagnostic codes are stable.** Never renumber a released code; schemas,
  `docc explain` and agent workflows reference them.
- **Passes collect, they do not stop.** All checks run and append to one list.
  An author fixing one error at a time runs the compiler ten times.
- **Adding a check:** implement it in `internal/sema/rules.go`, register it in
  `registry`, document it in the README table. Schemas select checks by name and
  supply their own code and severity.
- **Adding a diagnostic code:** add it to `explanations` in `cmd/docc/main.go`.

## Testing

`testdata/` is the regression suite, checked against `testdata/schemas/`:

- `good/` — must produce zero errors (`TestGoodDocumentsHaveNoErrors`)
- `bad/` — exercises specific failures
- `*.golden` — committed rendered diagnostics for every fixture

Changing a message is expected to fail `TestGolden`. Review the diff, then
`task test:golden:update`. Never regenerate goldens without reading the diff —
that is the check working.

## goldmark notes

- `Lines()` panics on inline nodes. Guard with `n.Type() != ast.TypeBlock`.
- A `ListItem` holds its text in a child `TextBlock`, not on the item itself.
  Walk to the leaves rather than assuming a depth.
- Fenced divs (`::: beweis`) are a local block parser in `internal/parse/fences.go`;
  goldmark has no built-in support.

## Working on pkg/docx

No dependencies. `archive/zip` and `encoding/xml` cover the container; XML is
written through the `xw` helper rather than `encoding/xml` marshalling, because
OOXML needs exact namespace prefixes and, in places, a specific attribute order.

- **Output must stay deterministic.** Fixed archive timestamps, sorted parts,
  identifiers assigned by position. Never introduce a counter whose state
  survives across calls, or a timestamp read from the clock.
- **Word rejects rather than degrades.** An empty table cell, a header part with
  no paragraph, or a body without `sectPr` produces a repair prompt, not an
  error message. The writer fills these in; keep it that way.
- **Unbalanced XML panics at construction.** `xw.bytes` refuses to return a part
  with an open element, because the alternative is finding out in Word.
- **Units are distinct types.** `Twips`, `EMU`, `HalfPt`, `Eighth`. Do not add a
  plain-int measurement parameter.
- **Numbering is a two-level indirection.** A paragraph names a `numId`, which
  names an `abstractNumId`. Two lists sharing a `numId` continue each other's
  numbering. Use `Numbering.AddList` / `NewInstance`.
- **`task test:roundtrip` before trusting a structural change.** Unit tests
  check strings; only a real renderer proves the file opens.

## Positions

`diag.Position.Col` and `.Len` are **byte** offsets, because that is what the
parsers report. The caret renderer converts to runes. Umlauts are common in this
corpus, so any new position arithmetic must keep that distinction straight.
