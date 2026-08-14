# docc — implementation notes

Handoff notes. Read `README.md` for what docc does and `CLAUDE.md` for
conventions before starting.

## The principle everything follows from

**docc is a generic engine. The document conventions belong to the project being
compiled.**

- `internal/docx`, `internal/{parse,schema,sema,ir,emit,theme}` — engine. Must not
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

## Where the project stands

The pipeline is complete and in use. `docc check` validates, `docc build`
renders `.docx` and PDF, `docc init` scaffolds a new project, and the whole
thing is driven from `pi_assistant` through `document_cli.py`, where all five
document types — letter, legal, contract, gutachten, protokoll — now route to
docc. The pandoc assets they replaced are gone.

The corpus in `testdata/` is checked at both ends: rendered diagnostics against
`*.golden`, and built `word/*.xml` against `testdata/golden/`. Two document
types cover complementary halves of the theme surface, deliberately — `legal`
exercises frames, mixed-formatting runs, the heading outline and marginal
numbers; `letter` exercises an epilogue, a repeated list field, a footer and
metadata formatting. Keep both.

Git history is the record of how it got here. `docs/legal-output-numbering.md`
is a design document that has been implemented and is kept for its reasoning;
`docs/swiss-post-letter-layout.md` is a live specification reference.

---

## Next

### 1. Verify the themes against real documents

**This is the only thing standing between the pipeline and production use, and
no amount of further engine work substitutes for it.**

Every theme is a reconstruction from the ODT assets it replaces. Nothing has
been diffed against a document that was actually filed or posted. These go to
courts and to clients.

What is known to be a guess:

- `zbp-legal` letterhead geometry — the partner column at `x: 148mm, y: 40mm`,
  the margins, the firm-name block over the rule.
- The Randziffer: `8pt`, `hanging: 10mm`. Internally consistent — the numbers
  share a column with the `I.` and `A.` labels — but never measured.
- The address field at `x: 25mm, y: 60mm`. It satisfies the Swiss Post
  specification on paper, but a window envelope is a physical object: print it,
  fold it, look through the window.
- `contract`, `gutachten` and `protokoll` were ported quickly and have had the
  least scrutiny of all.

Ask Kevin for one reference PDF per type. The method that works: `pdftotext
-bbox` for positions in millimetres, and rendering pages to PNG to look at them
— coordinates and eyes catch different things. That pairing is what found the
Randziffer misalignment and the dead `formats.date`.

### 2. Loose ends from the cutover

- `unterzeichner` is `required: true` on `legal`. Existing briefs do not carry
  it and fail validation until backfilled. Backfill, or relax it during
  migration.
- `schlussformel` has no default: the wording differs by procedural type and
  addressee, and inventing court language is not the compiler's job. If it
  should be generated, that is defaults-by-enum in the schema.
- `pi_assistant` has untracked build outputs (`docs/*.pdf`, `docs/*.docx`).
  Those want a `.gitignore` entry rather than a commit.

### 3. Engine work, when it earns its place

- `docc fmt` — canonical formatter. Whatever can be auto-fixed should be
  rewritten rather than diagnosed; it cuts agent iteration count.
- LSP server — reuse `internal/sema`, gives live diagnostics while writing.
- MCP server — thin wrapper over `check`/`build`/`types`/`themes`.
  `docc check --json` already emits the right shape.
- Golden coverage stops at `testdata/good`. The `bad/` fixtures are checked but
  never built, so a theme change cannot regress the error path.
- `docx.Indent` does not distinguish absent from zero, unlike `theme.Length` and
  `docx.Spacing`. It will bite when a style needs to override an inherited
  indent back to `0mm`.

### A recurring request to keep refusing

Conditional furniture has been asked for twice — a right-window address, a
Schlussformel per procedural type. Both times the right answer was a second
theme or a frontmatter field. A theme with `when:` is a program in YAML, and
`theme.Line` says so in its doc comment. If it comes up a third time, the honest
fix is defaults-by-enum in the schema, not a conditional in the theme.

---

## Gotchas that will cost you an hour each

- **Schema knobs need a reader.** `default:` and `formats.date` were both
  declared, documented, used in real config, and read by nothing. Grep for the
  field before assuming a new one works.
- **`Length` and `Spacing` distinguish absent from zero.** A style must be able
  to override an inherited spacing back to `0mm`, and an omitted attribute
  inherits. This is why `theme.Length` has `Set()` and `docx.Spacing` has
  `ExplicitBefore`/`ExplicitAfter`. Do not "simplify" them to plain ints.
- **A theme's `levels:` is flat, not a tree.** The definition is level 0 and
  `levels[i]` is level `i+1`, capped at nine. Recursing into it gave two levels
  the same `ilvl`, and Word rendered the loser's `%N` as literal text.
- **Word numbering is a two-level indirection.** A paragraph names a `numId`,
  which names an `abstractNumId`. Two lists sharing a `numId` continue each
  other's numbering. Use `Numbering.AddList` / `NewInstance`. Render numbering
  and furniture numbering invert the usual rule: they each want *one* shared
  instance, because continuing the count is the point.
- **Furniture interpolates from `emitter.meta`, never `doc.Meta`.** `meta` is
  the copy `typedMeta` has converted by declared schema type. Reaching for
  `e.doc.Meta` gets raw YAML values and silently disables `formats.date`.
- **`diag.Position.Col` and `.Len` are byte offsets**, converted to runes only in
  the caret renderer. This corpus is full of umlauts.
- **goldmark:** `Lines()` panics on inline nodes — guard with
  `n.Type() != ast.TypeBlock`. A `ListItem` keeps its text in a child
  `TextBlock`, not on the item.
- **`soffice` exits 0 when it produced nothing.** `internal/emit/pdf.go` already
  handles this, plus profile locking and hangs. Do not add a second call site
  that skips those.
- **Rebuild `bin/docc` before manual testing.** `go build ./...` compiles the
  packages but leaves the binary stale, which produces confusing "the fix did
  nothing" results.
- **Diagnostic codes are stable.** Never renumber a released `DOC0xx`.
