# docc

A compiler for structured Markdown documents. It parses and validates Markdown
and YAML frontmatter against a schema, then emits self-contained `.docx` through
the configured theme.

See `README.md` for the schema format and CLI usage. `docs/philosophy.md` is
the scope document: identity, non-goals, the closed content model, and the
way-forward list. Read it before adding a feature — most proposals it rejects
were once accepted here and later removed.

## Layout

```
cmd/docc/            CLI: check | build | init | profile | doctor | lsp |
                     types | describe | example | themes | explain | version
internal/diag/       Diagnostic type, source-excerpt and JSON rendering
internal/parse/      goldmark wrapper: frontmatter split, block tree, positions
internal/schema/     doc-type spec loading and `extends` resolution
internal/sema/       validation passes → diagnostics
internal/ir/         parsed document → render tree
internal/emit/       render tree + schema + theme → docx parts, emit.Validate
internal/theme/      theme loading and `extends` resolution
internal/project/    .docc directory discovery
internal/profile/    pack manifests, resolution order, Git install, provenance
internal/defaultpack/ the starter pack embedded in the binary (`builtin` source)
internal/starter/    `docc init`: copies the default pack out as a checkout
internal/lsp/        editor diagnostics over stdio
internal/docx/       .docx writer — stdlib only, no template, deterministic
testdata/            golden corpus: schemas/, good/, bad/, golden/
```

Scope guard: docc contains nothing AgentSkill- or agent-host-related. Skills
are built by the pack repositories that own the drafting knowledge; docc is
the compiler they invoke. Do not reintroduce packaging, plugin manifests or
skill prose here.

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
- **Schema/theme mismatches are caught in `emit.Validate`, not at load.** It is
  the only place holding both. Every style a schema maps, every field a theme
  interpolates and every definition a render rule names is checked there, before
  a single paragraph is built.
- **Adding a diagnostic code:** add it to `explanations` in `cmd/docc/main.go`.

## Inheritance

Schemas and themes both have `extends:`, and they must keep behaving the same
way. Two different inheritance semantics in one product is a defect.

- **Maps merge key-wise; ordered sequences are replaced wholesale** when the
  child declares them. Half a letterhead is worse than a restated one.
- **A theme merges YAML documents, then decodes once** (`internal/theme`),
  rather than merging decoded structs. Only the document distinguishes a key
  left out from a key set to its zero value, and both cases are real: a child
  omitting `formats:` must keep the parent's month names, and a child setting
  `title_page: false` under a parent that set it true must win. Adding a field
  to `Theme` therefore needs no merge code — keep it that way.
- **A name beginning with `_` is a fragment**: extended, never selected.
  `Set.Names` hides them and `Set.Get` rejects them, so a schema cannot name one.
- **Resolution stays inside one directory.** No cross-pack parent. A base pack
  silently changing header spacing in every firm that installed it is the
  failure a compiler exists to prevent.

## The embedded default pack

`internal/defaultpack/files` is a real profile pack — manifest, schemas,
themes — compiled into the binary. `Resolve` falls back to it as the `builtin`
source, so resolution always answers and an unconfigured docc works out of the
box. Two invariants: the embedded pack must always validate and build
(`TestStarterProfile` in `internal/emit` guards it), and the fallback must
never mask a legacy `.docc/schemas` layout — that path fails loudly instead,
because silently compiling against schemas nobody chose is the failure a
compiler exists to prevent. The pack materializes into the XDG store
content-addressed by its hash; never resolve it from a path inside the
repository.

## Testing

`testdata/` is the regression suite, checked against `testdata/schemas/`:

- `good/` — must produce zero errors (`TestGoodDocumentsHaveNoErrors`) *and* build
  to `.docx` (`TestBuildGolden`). A fixture only belongs here if its schema has a
  theme that can render it.
- `bad/` — exercises specific failures
- `*.golden` — committed rendered diagnostics for every fixture
- `golden/<fixture>/*.xml` — the `word/` parts every `good/` fixture builds to
  (`TestBuildGolden`), discovered from the archive so a theme that grows a
  header or footer adds a file here

Three document types cover complementary parts of the theme surface, and none
of them is a variation on another. `ch_legal` has the frames and the
mixed-formatting runs; `ch_letter` has the epilogue, the repeat, the footer and
the metadata formatting; `ch_urkunde_kaufvertrag` has a first-page header
carrying an image, span character styles, the amount and ruled block patterns,
a page break keyed to a heading, and amounts spelled out from their figures. A
change that only one of them catches is the reason all three exist — do not
fold them together.

Changing a message is expected to fail `TestGolden`; changing a style or the
writer is expected to fail `TestBuildGolden`. Review the diff, then
`task test:golden:update`. Never regenerate goldens without reading the diff —
that is the check working.

## goldmark notes

- `Lines()` panics on inline nodes. Guard with `n.Type() != ast.TypeBlock`.
- A `ListItem` holds its text in a child `TextBlock`, not on the item itself.
  Walk to the leaves rather than assuming a depth.
- Fenced divs (`::: beweis`) are a local block parser in `internal/parse/fences.go`;
  goldmark has no built-in support.

## Working on internal/docx

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
  numbering. Use `Numbering.AddList` / `NewInstance`. Render numbering inverts
  the usual rule: a heading outline and a marginal number each want *one*
  shared instance for the whole document, because continuing is the point.
- **Schema `default:` is applied in sema, not at render time.** It decides
  whether a required field is actually missing, and it has to reach
  `Meta.Values` for the emitter to interpolate it. It was declared and
  documented but never applied for a while; `formats.date` had the same shape
  of bug. Check that new schema knobs are read by something.
- **A theme's `levels:` is flat, not a tree.** The definition is level 0 and
  `levels[i]` is level `i+1`, capped at nine. Recursing into it gave two levels
  the same `ilvl`, and Word renders the loser's `%N` as literal text.
- **`task test:roundtrip` before trusting a structural change.** Unit tests
  check strings; only a real renderer proves the file opens.

## Positions

`diag.Position.Col` and `.Len` are **byte** offsets, because that is what the
parsers report. The caret renderer converts to runes. Umlauts are common in this
corpus, so any new position arithmetic must keep that distinction straight.
