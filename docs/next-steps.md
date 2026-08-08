# docc — implementation notes

Handoff notes. Read `README.md` for what docc does and `CLAUDE.md` for
conventions before starting.

## The principle everything follows from

**docc is a generic engine. The document conventions belong to the project being
compiled.**

- `pkg/docx`, `internal/{parse,schema,sema,ir,emit,theme}` — engine. Must not
  know about law, German, or any particular firm.
- `<project>/.docc/schemas/*.yaml` — what a document type *is*.
- `<project>/.docc/themes/*.yaml` — what it *looks like*.

docc may ship as a public CLI or MCP server. Anything domain-specific that
reaches the engine is a defect, not a feature.

**Never copy a real project's `.docc/` content into `testdata/`.** It happened
once and put three people's private email addresses into the repo. Test fixtures
use invented identities — see `testdata/themes/example-legal.yaml`
(Muster & Partner, `example.ch`).

---

## Done — Tasks 1, 2 and 4

**Task 1, parameterise the named checks.** `beweis_beilage_refs` and
`beilagen_coverage` are gone; `div_items_match` and `cross_reference` replace
them, configured through `schema.Rule.Args`. `no_placeholder_text` takes an
optional `pattern`. A rule naming a missing or malformed argument reports
`DOC009` against the schema. Where a pattern has a capture group, group 1 is
what the caret underlines. `testdata/schemas/legal.yaml` and
`~/git/pi_assistant/.docc/schemas/legal.yaml` carry the Swiss-legal meaning;
`docc check` on `pi_assistant/templates/template_legal.md` is byte-identical to
before. Tests: `internal/sema/rules_test.go`.

**Task 2, move locale out of the engine.** `theme.Formats` declares `date`
(Go reference layout), `bool`, `list_separator`, and the `months` / `weekdays`
name tables — the theme supplies the names, so the engine holds no locale
database. `Expand` is now a method on `*Theme`; a nil theme formats with the
defaults, which are ISO dates and `true`/`false`. Tests: `TestFormatsDefault`,
`TestFormatsConfigured`.

**Task 4, golden tests over built `.docx`.** `internal/emit/golden_test.go`
builds every fixture in `testdata/good` with its theme and compares the `word/`
parts to `testdata/golden/<fixture>/`. Parts are discovered from the archive
rather than listed, so a theme that grows a header or footer adds a file; a part
that stops being produced fails rather than leaving a stale golden behind.
`task test:golden:update` now regenerates diagnostics and documents in that
order. `testdata/schemas/legal.yaml` named the nonexistent theme `zbp-legal`;
it names `example-legal` now.

**A second document type.** `testdata/schemas/letter.yaml` and
`testdata/themes/example-letter.yaml`, ported from the letter type in
`~/git/pi_assistant/.docc` with invented identities. It is not a variation on
`legal`: it exercises the epilogue, `repeat`, a footer, a boolean and non-list
defaults, none of which `legal` reaches, while `legal` keeps frames and runs.
Keep both. The port surfaced three engine defects, all now fixed:

- `Theme.Fields()` was documented as existing for schema validation and had no
  caller. It is now wired into `emit.Validate`, which is what turns a typo in
  `{{ recipient.city }}` from a silently dropped address line into a build
  failure.
- `Fields()` walked only `Line.Text`, so every placeholder inside a `runs:`
  block — which is the entire party block of a brief — was invisible to it.
- `formats.date` never fired. YAML hands back a string for a date, so both
  themes' date configuration was dead and letterheads printed ISO dates.
  `emit.typedMeta` now converts values by their declared schema type before
  interpolation. It is schema-driven on purpose: sniffing strings that look
  like dates would reformat case references.

The letter type in `pi_assistant` still declares `right_window`, which cannot
work — the theme has the `AddressBlock*Right` styles but `theme.Line` has no
condition, and it should not grow one. Either drop the field or ship a second
theme. The port here does neither: it omits the field.

**Render numbering.** `schema.Render` adds `render.heading_numbering` and
`render.paragraph_numbering`, each naming a definition in the theme and a
heading to start at. The engine knows nothing about outlines or Randziffern —
it applies a named definition to top-level headings and top-level prose. See
`docs/legal-output-numbering.md`, which is now a record rather than a proposal.

## Remaining work

### 3. Verify the themes against real documents

**This is the only thing standing between the pipeline and production use, and
no amount of further engine work substitutes for it.**

All five types now render through docc, but every theme is a *reconstruction*
from the ODT assets it replaces, not a verified match. Nothing has been checked
against a document that was actually filed or posted. These go to courts and to
clients.

What is known to be a guess:

- `zbp-legal` letterhead geometry — the partner column at `x: 148mm, y: 40mm`,
  the margins, the firm-name block over the rule.
- The Randziffer: `8pt`, `hanging: 10mm`. Internally consistent — the numbers
  share a column with the `I.` and `A.` labels — but the size and the column
  width were never measured.
- The address field at `x: 25mm, y: 60mm`. It satisfies the Swiss Post
  specification on paper (`docs/swiss-post-letter-layout.md`), but a window
  envelope is a physical object: print it, fold it, look through the window.
- `contract`, `gutachten` and `protokoll` were ported quickly and have had the
  least scrutiny of all.

Get a reference PDF per type from Kevin and diff against it. `pdftotext -bbox`
gives positions in points and turns "looks about right" into a number; rendering
pages to PNG and reading them catches what coordinates do not.

### 4. Loose ends from the cutover

- `unterzeichner` is now `required: true` on `legal`. Existing briefs do not
  carry it and will fail validation until they do. Decide whether to backfill
  or relax it during migration.
- `schlussformel` has no default because the wording differs by procedural type
  and addressee. If it should be generated, that is a defaults-by-enum
  mechanism in the schema, not a conditional in the theme.
- `pi_assistant` still has the old pandoc assets deleted but uncommitted, and
  build outputs (`docs/testklage.pdf`, `docs/Klageantwort.docx`) untracked.
  Those want a `.gitignore` entry rather than a commit.

### 5. Possible later work

- `docc fmt` — canonical formatter. Whatever can be auto-fixed should be
  rewritten rather than diagnosed; it cuts agent iteration count.
- LSP server — reuse `internal/sema`, gives live diagnostics while writing.
- MCP server — thin wrapper over `check`/`build`/`types`/`themes`.
  `docc check --json` already emits the right shape. Defer until the CLI settles.
- Golden coverage stops at `testdata/good`. The `bad/` fixtures are checked but
  never built, so a theme change cannot regress the error path.
- `docx.Indent` still does not distinguish absent from zero, unlike
  `theme.Length` and `docx.Spacing`. It has not bitten yet; it will when a style
  needs to override an inherited indent back to `0mm`.
- Conditional furniture keeps being asked for — a right-window address, a
  Schlussformel per procedural type. The answer so far is a second theme or a
  frontmatter field, deliberately. If it comes up a third time, the honest fix
  is defaults-by-enum in the schema, not `when:` in the theme.

---

## Gotchas that will cost you an hour each

- **`Length` and `Spacing` distinguish absent from zero.** A style must be able
  to override an inherited spacing back to `0mm`, and an omitted attribute
  inherits. This is why `theme.Length` has `Set()` and `docx.Spacing` has
  `ExplicitBefore`/`ExplicitAfter`. Do not "simplify" them to plain ints.
  `docx.Indent` does **not** have this yet — add it the same way if you need it.
- **`diag.Position.Col` and `.Len` are byte offsets**, converted to runes only in
  the caret renderer. This corpus is full of umlauts.
- **goldmark:** `Lines()` panics on inline nodes — guard with
  `n.Type() != ast.TypeBlock`. A `ListItem` keeps its text in a child
  `TextBlock`, not on the item.
- **Word numbering is a two-level indirection.** A paragraph names a `numId`,
  which names an `abstractNumId`. Two lists sharing a `numId` continue each
  other's numbering. Use `Numbering.AddList` / `NewInstance`.
- **`soffice` exits 0 when it produced nothing.** `internal/emit/pdf.go` already
  handles this, plus profile locking and hangs. Do not add a second call site
  that skips those.
- **Furniture interpolates from `emitter.meta`, never `doc.Meta`.** `meta` is
  the copy `typedMeta` has converted by declared schema type. Reaching for
  `e.doc.Meta` gets raw YAML values and silently disables `formats.date`.
- **Rebuild `bin/docc` before manual testing.** `go build ./...` compiles the
  packages but leaves the binary stale, which produces confusing "the fix did
  nothing" results.
- **Diagnostic codes are stable.** Never renumber a released `DOC0xx`.
