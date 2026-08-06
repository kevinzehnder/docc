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

## Task 1 — parameterise the named checks

**Problem.** `internal/sema/rules.go` registers Swiss-legal checks by name:

```go
var registry = map[string]CheckFunc{
	"no_placeholder_text": checkNoPlaceholders,
	"beweis_beilage_refs": checkBeweisBeilageRefs,   // domain-specific
	"beilagen_coverage":   checkBeilagenCoverage,    // domain-specific
	"no_empty_sections":   checkNoEmptySections,
}
```

`beweis_beilage_refs` requires every item inside a `::: beweis` div to end in
`// Beilage N`. `beilagen_coverage` cross-checks those numbers against the
`beilagen` frontmatter list. Both hardcode the div name `beweis`, the field name
`beilagen`, and the German regex — none of which belong in a general tool.

**Target.** Two generic checks configured from the schema.
`schema.Rule.Args map[string]any` already exists and is currently unused; this
is what it is for.

```yaml
# in .docc/schemas/legal.yaml — the Swiss-legal meaning lives here
rules:
  - id: LEG012
    check: div_items_match
    args:
      div: beweis
      pattern: '//\s*Beilage\s+\d+\s*$'
    message: "Beweismittel without a Beilage reference"
    hint: 'append a reference, e.g. "// Beilage 3"'

  - id: LEG020
    check: cross_reference
    severity: warning
    args:
      div: beweis
      pattern: '//\s*Beilage\s+(\d+)'   # capture group 1 is the key
      list_field: beilagen
      label: Beilage                     # used in the message
```

`cross_reference` checks both directions, as `beilagen_coverage` does today:
cited but not listed, and listed but never cited.

Also parameterise `no_placeholder_text`: keep the current bracketed-text regex as
the default, but allow `args: { pattern: ... }`.

**Files.**

- `internal/sema/rules.go` — replace the two functions, keep the shape.
  `ruleContext` already carries `Rule`, so `c.Rule.Args` is in scope.
  `c.report(pos, defaultHint, format, args...)` already honours
  `Rule.Message`/`Rule.Hint` overrides.
- Add arg accessors with clear errors. A rule naming a missing or malformed arg
  must produce a `DOC009` diagnostic ("the schema itself is wrong"), not a panic
  and not silence. There is precedent: the unknown-check branch in `runRules`.
- `cmd/docc/main.go` — no new `DOC0xx` codes needed; schema-defined rule codes
  are documented in the schema, not in `explanations`.
- `testdata/schemas/legal.yaml` — update to the new form.
- `~/git/pi_assistant/.docc/schemas/legal.yaml` — update to the new form.
  This is outside the docc repo and is easy to forget; the golden tests will not
  catch it.

**Acceptance.**

- `grep -rin 'beweis\|beilage' internal/ pkg/` returns nothing.
- `task` passes. `TestGolden` output is unchanged except for wording you
  deliberately changed — read the diff before running `task test:golden:update`.
- `docc check` on `pi_assistant/templates/template_legal.md` still reports the
  same findings it does today.

---

## Task 2 — move locale out of the engine

**Problem.** `internal/theme/interp.go`:

```go
func format(v any) string {
	case bool:
		if val { return "ja" }        // German hardcoded in the engine
		return "nein"
	case time.Time:
		return val.Format("2. January 2006")   // also wrong: German day order,
		                                       // English month names
	...
}
```

The date format is a latent bug as well as a genericity leak. It has not bitten
yet only because frontmatter dates currently arrive as strings and pass through
untouched. The moment a date is parsed into a `time.Time` it renders as
"3. March 2024" in a German document.

**Target.** Formats declared by the theme, since this is presentation:

```yaml
# .docc/themes/<name>.yaml
formats:
  date: "2. January 2006"    # Go reference layout
  bool: ["ja", "nein"]       # [true, false]
  list_separator: ", "
```

Month and weekday names need translating; Go's `time` package will not do it.
Either add a small name table keyed off `defaults.lang`, or let the theme supply
one:

```yaml
formats:
  months: [Januar, Februar, März, ...]
```

Pick one and document it. The table is the simpler option and keeps the engine
free of a locale database.

**Files.**

- `internal/theme/theme.go` — add `Formats` to `Theme`.
- `internal/theme/interp.go` — `Expand` must take the formats.
  Current signature `Expand(text string, meta map[string]any) Interp`; there are
  exactly two call sites, both in `internal/emit/emit.go`
  (`furnitureLine`, `furnitureRunLine`). Prefer a method on `*Theme` over a
  third parameter.
- Sensible defaults when `formats:` is absent: ISO dates, `true`/`false`.
  A theme that says nothing should produce something unambiguous, not something
  German.

**Acceptance.**

- `grep -rn '"ja"\|"nein"\|January' internal/` returns nothing outside tests and
  the default Go layout constant.
- A theme with no `formats:` block renders dates as ISO.
- `internal/theme/theme_test.go` covers both a configured and a default theme.

---

## After those two

In order. Do not start these before Tasks 1 and 2 — schemas written against the
old check names would have to be rewritten.

### 3. Verify the legal letterhead, then cut over

`~/git/pi_assistant/.docc/themes/zbp-legal.yaml` is a **reconstruction** from
`assets/template_legal/legal.opendocument`, not a verified match. Positions were
approximated: the partner column at `x: 148mm, y: 40mm`, the page margins, the
firm-name block over the rule. These documents go to courts.

Get a reference PDF of a real filed brief from Kevin and match against it before
cutting anything over.

Then in `~/git/pi_assistant/src/document_cli.py`: `command_render` currently
calls `gate_render` and then runs pandoc. Route `legal` and `letter` to
`docc build` instead. Leave `contract`, `gutachten`, `protokoll` on pandoc —
they have no schema yet. Delete pandoc assets only for the types actually cut
over.

### 4. Golden tests over built `.docx`

Output is deterministic (fixed archive timestamps, sorted parts, ids by
position), so this is cheap and currently missing. Extract `word/document.xml`,
`word/styles.xml` and `word/numbering.xml` and compare to committed goldens.
`testdata/themes/example-legal.yaml` and the fixtures in `testdata/good` and
`testdata/bad` are already in place.

Keep `task test:roundtrip` as well — golden files prove the bytes did not
change, not that Word will open the file.

### 5. Remaining document types

`contract`, `gutachten`, `protokoll` have no schema and no theme. Reverse-
engineering them from `templates/` and `assets/` means guessing at conventions;
have Kevin confirm each before relying on it.

### 6. Possible later work

- `docc fmt` — canonical formatter. Whatever can be auto-fixed should be
  rewritten rather than diagnosed; it cuts agent iteration count.
- LSP server — reuse `internal/sema`, gives live diagnostics while writing.
- MCP server — thin wrapper over `check`/`build`/`types`/`themes`.
  `docc check --json` already emits the right shape. Defer until the CLI settles.

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
- **Rebuild `bin/docc` before manual testing.** `go build ./...` compiles the
  packages but leaves the binary stale, which produces confusing "the fix did
  nothing" results.
- **Diagnostic codes are stable.** Never renumber a released `DOC0xx`.
