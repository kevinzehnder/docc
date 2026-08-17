# Power-user improvements

Findings from building a full GmbH-Gründung dossier (six document types) against
the `jlmy-profiles` pack with docc **v0.2.0**, checked against the office's own
filed documents in `assets/GmbH_BZ/`. Three defects that silently produce wrong
output, four capability gaps that blocked layouts the firm actually files, one
papercut, and one structural limit found while trying to share a letterhead.

Everything below was reproduced, not inferred. Line numbers refer to tag
`v0.2.0`. Where a workaround now exists in `jlmy-profiles`, it is named so it
can be reverted once the fix lands. **The numbers are stable identifiers** —
`jlmy-profiles` cites several of them in comments (`zbp-briefkopf.yaml` cites 7,
`gmbh_anmeldung.yaml` cites 2), so please don't renumber.

| # | Change | Kind | Where | Est. | Status |
|---|---|---|---|---|---|
| 1 | A rule whose `div` is absent must not pass | defect | `sema/rules` | ½ d | **done** |
| 2 | Theme `align: left` is discarded | defect | `theme/convert` | 1 h | **done** |
| 3 | `spans_agree` counts a blank as a value | defect | `sema/rules` | 2 h | **done** |
| 4 | Themeable tables: widths, borders, header | capability | `emit` + theme vocab | 1 d | open |
| 5 | Block content inside table cells | capability | `parse`/`ir`/`emit` | larger | open (probably never) |
| 6 | `amount_words` cannot be switched off per type | papercut | `emit` | 10 min | **done** |
| 7 | Themes cannot compose (single `extends`) | structural | `theme` | design first | open — design |
| 8 | Sub-level numbering cannot continue across its parent | capability | `theme` | 1 h | **done** |
| 9 | **A label-left block pattern for the body** | capability | `emit` | ½ d | **done** |
| 10 | `docc --version` is not accepted, only `docc version` | papercut | `cmd/docc` | 10 min | **done** |
| 11 | A pack checkout needs `--schema-dir`/`--theme-dir` on every command | papercut | `project` | ½ d | **done** |
| 12 | `.field` gives nested and continuation paragraphs the row style | papercut | `emit` | 1 h | open |

**10 and 11 landed too** (2026-08-17), after the six above. 12 is the only open
papercut; 4, 5 and 7 remain as they were.

**Adopted in the pack.** All six landed changes are now in use: `on_missing:
ignore` on `legal`'s conditional evidence rule, both theme workarounds reverted,
`restart: never` on the Stampa numbering, and the Anmeldung rebuilt as a
`::: feld` form — which took it from two pages to one. `docc doctor --strict` and
all fifteen types pass, and `docc example --blank` is now warning-free, which it
was not before change 3. Change 12 below was found while doing that.

**What landed (unreleased, on `master` after v0.2.0):** 1, 2, 3, 6, 8 and 9 —
the whole suggested order down to change 8. Each item's section below carries a
`Landed:` note saying what shipped, including where it differs from the proposal.
4, 5 and 7 are untouched: 9 removes the reason to want 4 and 5, and 7 is a design
decision rather than a patch. 10 and 11 were raised separately and are described
at the end.

## Start here

If only three land, make them **1**, **2** and **9**.

- **1** is the one that matters most. A check that passes because it found
  nothing to check is worse than no check, and this one let a Gründungsurkunde
  declaring a Stammkapital of `Fr. 3'000.00` build clean — against a statutory
  floor of `Fr. 20'000` that the schema explicitly guards.
- **2** is an hour, and the documentation currently instructs theme authors to
  do the thing that silently does nothing.
- **9** is the one the firm is waiting on: half a day, no behaviour change, and
  it gives registry forms their real two-column layout *without* the correctness
  cost that tables (4 + 5) would impose.

Nothing here is a crash or a data-loss bug. The three defects are all
"produces the wrong thing quietly", which is the category that reaches a
registry counter before anyone notices.

## Reproducing any of this

```sh
git clone git@github.com:kevinzehnder/docc.git && cd docc
git checkout v0.2.0 && go build -o /tmp/docc ./cmd/docc

git clone git@github.com:kevinzehnder/jlmy-profiles.git && cd jlmy-profiles
/tmp/docc doctor --strict --schema-dir schemas --theme-dir themes
```

Two notes that cost time otherwise. A stale `docc` on `PATH` fails on the whole
pack with `unknown field "end_before_heading"` in an unrelated schema — build
v0.2.0 explicitly. And the pack now carries **workarounds** for 2 and 6; to see
those two in their original form, revert the styles named under each.

Rendered evidence for the layout findings is committed in the pack under
`output/fake-gmbh/` — `tabelle-experiment/` for the table route (4, 5) and
`tab-experiment/` for the tabbed-furniture prototype (9).

---

## 1. A rule whose named block is absent reports success

**`internal/sema/rules.go`** — `checkAmountsBalance`, `checkDivItemsMatch`,
`checkAmountAtLeast`

Every rule taking a `div:` argument iterates the document's divs and filters by
name. When no div of that name exists the loop body never runs, the rule emits
nothing, and the exit code is indistinguishable from a document that satisfied
it.

```go
for _, div := range c.File.Divs() {
    if div.Name != name { continue }
    // no div of this name → body never executes → no findings
}
```

### Reproduced on a real deed

The Gründungsurkunde's `Stammkapital` and `Einlagen` were rewritten from
`::: betraege` blocks into ordinary prose:

```markdown
Kevin Zehnder zeichnet 20 Stammanteile, ausmachend das Stammkapital von
Fr. 3'000.00.

In Geld einbezahlt wurden Fr. 12.00, hinterlegt bei der UBS Switzerland AG.
```

Result: `✅ ok`, and `docc build` produced the document. `GRU080` — whose entire
purpose is the Art. 773 Abs. 1 OR floor of Fr. 20'000 — never evaluated once.

An audit of the pack found three schemas where a div-scoped rule had no
`required_div` guaranteeing its target, all passing `doctor --strict`:

- `gmbh_gruendungsurkunde` — `betraege` (the Stammkapital)
- `ch_urkunde_kaufvertrag` — `betraege` (the purchase price)
- `legal` — `beweis`

### Fix

Two options, and they compose:

**a. Static, in `doctor`.** Flag any rule whose `div:` argument is not made
mandatory by a `required_div` in the same schema. Catches the class at authoring
time and would have found all three.

**b. At runtime.** Give div-scoped rules a way to distinguish "checked and
passed" from "nothing to check" — either report when the named div is missing,
or let a rule declare `on_missing: error | ignore`.

The conditional case is real and must stay expressible: a Rechtsschrift arguing
only a point of law offers no exhibits, so `legal`'s `beweis` rules have to stay
silent when no block exists. That argues for (a), as a warning, first.

**Fixed in the pack** — `GRU054` and `KFV014` now pair every amount rule with a
`required_div`; `legal`'s conditional rules carry a comment explaining why they
are deliberately unpaired.

**Landed.** Both halves, plus a third the proposal did not name.

- `on_missing: error | ignore` on `div_items_match`, `amounts_balance` and
  `amount_at_least`, through a shared `ruleContext.divsNamed` — the three checks
  no longer filter `File.Divs()` by hand, so the absent case has one
  implementation.
- `sema.UnguardedDivRules`, reported by `docc doctor` as a warning per rule and
  bound by `--strict`: a div-scoped rule with neither a `required_div` for the
  same block nor an `on_missing` has not said what an absent block means.
- The corpus now demonstrates both routes. `ch_urkunde_kaufvertrag` gained
  `KFV014` (`required_div: betraege`); `ch_legal`'s `beweis` rule says
  `on_missing: ignore` and explains why in a comment. Run
  `docc doctor --schema-dir testdata/schemas --theme-dir testdata/themes` before
  and after to see the class the check catches.

The conditional case stays silent at runtime by default, exactly as argued —
what changed is that staying silent is now something a schema *says*.

---

## 2. Theme `align: left` is discarded

**`internal/theme/convert.go:69,163`** · **`internal/docx/render.go:210`**

`alignName` maps both the empty string *and* an explicit `left` to `""`, and
`render.go:210` writes `w:jc` only when non-empty. A style that says
`align: left` emits no alignment at all and inherits whatever its `based_on`
chain sets.

```go
// internal/theme/convert.go:163
func alignName(s string) string {
	switch strings.ToLower(s) {
	case "", "left":
		return ""            // explicit left is indistinguishable from unset
	case "center", "centre":
		return string(docx.AlignCenter)
	...
```

docc already gets this right one function below: `tabAlignName` at
`convert.go:178` maps `left` to an explicit `docx.TabLeft`. Tab stops honour
left; paragraphs discard it.

**Symptom.** A style meant to be left-aligned renders justified, with stretched
word spacing on any line ending in a manual break.

**Aggravating.** `docs/theme-reference.md:143` recommends exactly this:

> Left-align styles for content with manual line breaks or short lines (party
> entries, address blocks) even when the body justifies.

The documentation prescribes the no-op.

### Fix

Distinguish unset from explicit `left` and emit `w:jc w:val="left"` for the
latter, matching `tabAlignName`. Optionally have `doctor` flag `align: left` on
a style whose `based_on` chain justifies, since that pairing is now always a
mistake.

**Workaround in the pack** — `themes/gmbh_anmeldung.yaml` carries a
`Formularzeile` base style whose only purpose is to keep `based_on` off the
justified `Standard`. Revert after the fix.

**Landed** as proposed: `alignName` maps `left` to `docx.AlignLeft`, matching
`tabAlignName` one function below. The four golden `styles.xml` files gained
`<w:jc w:val="left"/>` on exactly the styles that declared it and changed
nothing else. `docs/theme-reference.md` now says the recommendation works.

---

## 3. `spans_agree` counts a fill blank as a value

**`internal/sema/rules.go:435,443,502,525`**

Two checks apply different tests to the same span:

```go
// checkNoBlankSpans — rules.go:435
if span.HasClass(FieldSpanType) { continue }   // blanks are the point
if !isFillBlank(text) { continue }             // "____" counts as blank

// checkSpansAgree — rules.go:502
value := normalizeSpanValue(span.LiteralText(...))
// normalizeSpanValue = strings.Join(strings.Fields(s), " ")
// no field-span skip, and "____________" compares as a value
```

The consequence lands on docc's own output. `docc example --blank` is the
skeleton the tool hands an author, and it does not pass the tool's own check:

```
$ docc example --blank gmbh_gruendungsurkunde | docc check -
warning[GRU070]: `.firma` says "____________" here but "Muster Bau" on line 32
```

**Why CI missed it.** `internal/emit/profiles_test.go:105` asserts only
`res.Diagnostics.HasErrors()`. This is a warning, so the test passes. The same
blind spot exists downstream — the pack's CI runs `docc check` without
`--strict`.

### Fix

In `checkSpansAgree`, skip `span.HasClass(FieldSpanType)` and gate on
`isFillBlank`, mirroring `checkNoBlankSpans`. Then tighten the skeleton test to
fail on warnings as well as errors, or the next instance is equally invisible.

**Landed** as proposed: `checkSpansAgree` skips `.docc-field` spans and gates on
`isFillBlank`, mirroring `checkNoBlankSpans`. The skeleton assertion in
`internal/emit/profiles_test.go` now fails on *any* diagnostic rather than on
errors alone, so the next warning on `docc example --blank` is visible.

---

## 4. Themeable tables: column widths, borders, header suppression

**`internal/emit/emit.go:663,1191,1199`**

Borders and column widths are hardcoded at the emitter, so a table is a fixed
full grid with evenly split columns and nothing else:

```go
// emit.go:1191 — widths
for i := range widths { widths[i] = textWidth / docx.Twips(cols) }

// emit.go:1202 — borders, all six edges, always
Borders: &docx.TableBorders{
    Top, Bottom, Left, Right, InsideH, InsideV:
        &docx.Border{Style: docx.BorderSingle, Size: docx.BorderPt(0.5)},
}
```

The model already supports better. `docx.Table` carries per-column `Widths` and
a six-edge `TableBorders`, and `render.go:549` already serialises them — they
are being fed constants.

**Built as an experiment**: the Anmeldung HRA as a two-column form table.
Markdown column alignment (`|---:|:---|`) already reaches the cells via
`applyCellAlign`, so right-aligned labels work today and the sheet drops from
two pages to one. What ruins it:

- columns split 50/50 — `Firma:` takes half the page of whitespace;
- a full grid around every cell;
- an empty boxed header row, which markdown requires and a form has no use for.

### The target, measured

`assets/GmbH_BZ/5 - Anmeldung HRA.docx` is a filed Anmeldung, and its body is
exactly one table:

```
columns   42.1mm / 114.2mm / 3.1mm      (26.4% / 71.6% / 2.0%)
rows      11
borders   no <w:tblBorders> at all
          per-cell <w:tcBorders>, bottom edge only, 2 per row
header    none — the first row is a data row
col 1     right-aligned labels ("Firma:", "Sitz:", …)
```

So the shape the firm actually files needs: **uneven columns**, **no table
border**, **a bottom rule per row and nothing else**, and **no header row**.

**The row rules are now the only reason left to want this.** Change 9 gave the
pack the two-column layout without a table, and the Anmeldung ships that way —
but the per-row rule cannot be reproduced with paragraph borders. Word treats a
run of consecutive paragraphs carrying identical borders as one bordered block
and draws only its outer edge, so a bottom border on the row style produced two
rules across eleven rows, at the two points where the style changes and the run
breaks. There is no way to vary it per row from the theme. If change 4 is ever
built, that is what it buys; if it is not, the pack's form simply has no rules,
which is a defensible place to land.
docc today offers none of those. Column alignment it already has — markdown's
`|---:|:---|` reaches the cells through `applyCellAlign`.

### Fix — three theme keys

```yaml
table:
  columns: [26%, 72%, 2%]    # or [42mm, 114mm, 3mm]
  header: false              # don't emit row 0
  borders:
    outer:    none
    inside_v: none
    inside_h: { style: single, width: 0.5 }
```

That is the whole layout gap, and it is what stands between the profile pack
and a registry form that matches the original. `Widths` and the six-edge
`TableBorders` are already in `docx.Table`; `header: false` means not emitting
row 0, since GFM requires a header row that a form has no use for.

---

## 5. Block content inside table cells

**`internal/parse` · `internal/ir` · `internal/emit`**

Change 4 fixes how a table *looks*. This one fixes what a table can be *checked*
for — and without it, moving any document to a table trades correctness for
layout.

docc's checks anchor to exactly two node types: **headings** and **fenced divs**.
A GFM table cell holds inline content only, so it can carry neither. Verified by
putting both in cells and reading the emitted OOXML back:

```
row3: ['Stammkapital:', "::: betraege {#k} - [Fr. 20'000.00] … :::"  [paras=1 numPr=0]]
row4: ['Belege:',       '- Urkunde - Statuten'                       [paras=1 numPr=0]]
```

The fence was never parsed as a div — it is literal text. The list has no
`numPr` — also literal text. Every cell is exactly one paragraph.

So converting the Anmeldung to a table costs, concretely:

- **Loud:** the eleven `required: true` headings in `body:` all report missing,
  because a cell is not a heading node — `body: []` becomes mandatory. Every
  `required_div` likewise errors.
- **Silent:** `amounts_balance`, `div_items_match`, `amount_at_least` and
  `no_empty_sections` find nothing to iterate and report nothing. Change 1
  again, arriving by a different road.

### Fix

Let cells hold block content: multiple paragraphs, lists, and fenced divs. Then
`::: betraege` in a cell simply works, `Belege` is a real list again, and every
existing rule keeps functioning unchanged.

**But probably do not build this.** It is the expensive way to reach a form
layout, and change 9 reaches the same place for a fraction of the work. Build
it only if genuinely tabular data turns up — a financial schedule, a comparison
— where cells really do need paragraphs.

---

## 6. `amount_words` cannot be switched off for one document type

**`internal/emit/emit.go:1018`**

```go
wordsStyle := e.style("div."+d.Name+".words", "div."+d.Name)
if e.theme.Formats.AmountWords == "" || wordsStyle == "" { continue }
```

The fallback is non-empty whenever the div itself is mapped, so dropping the
`.words` mapping in a schema never disables the feature. `docs/authoring.md`
presents it as opt-in — *"needs the theme's `formats.amount_words`"* — but the
only off switch is theme-wide.

Encountered when the Anmeldung, which inherits the deed theme, printed
*"(Franken zwanzigtausend)"* twice under a two-line Stammkapital block. A deed is
read aloud and wants the gloss; a registry form does not.

### Fix

Drop the `"div."+d.Name` fallback so `.words` means what the documentation says.

**Workaround in the pack** — `formats: { amount_words: "" }` in
`themes/gmbh_anmeldung.yaml`. Revert after the fix.

**Landed** as proposed: the `"div."+d.Name` fallback is gone, so `.words` means
what `docs/authoring.md` says. `blockSuffixes` lost the `div.<name>` fallback
placeholder with it — nothing else used it.

---

## 7. Themes cannot compose

**`internal/theme`** — design question, not a patch

Found while trying to answer a simple question: can the Zehnder Bolliger &
Partner letterhead be a shared base?

Today it exists twice, as two independent implementations that have drifted:

| | `zbp-legal` | `zbp-letter` |
|---|---|---|
| Placement | body `prologue` | page `header:` (every sheet) |
| Style names | `LH_Kanzlei`, `LH_Rule`, `LH_Addr`, … | `BriefkopfZeile`, `KanzleiZeile`, … |
| Firm name | `Zehnder Bolliger & Partner` | `ZEHNDER  BOLLIGER  &  PARTNER` |
| Rule | own `LH_Rule` paragraph | `border-bottom` on the name |
| Frame | `x: 148mm, width: 45mm` | `x: 118mm, width: 62mm` |
| Address size | 7.5pt | 8pt |
| Partner | `lic. iur. Willy Bolliger` | `lic. iur. Willy Bolliger-Kunz` |
| Title | `Rechtsanwalt und Notar` | `Rechtsanwalt u. Notar` |
| Register note | one line | split across two |

The last three are not styling drift — the two letterheads **disagree about a
partner's name**. That is firm data maintained in two places, and one of them is
wrong.

The obvious fix is a `_briefkopf` fragment both extend. It cannot be written:
`extends:` is single, `zbp-letter` already spends it on `jlmy-letter` for the
letter machinery (sender, recipient, date, subject, Beilagen), and `zbp-legal`
needs the letterhead without any of that machinery. There is no way to say
"letter body machinery **plus** ZBP letterhead" as two independent axes.

### Options

- **Multiple `extends`**, applied in order with the same key-by-key merge that
  single inheritance already uses. Smallest conceptual change; the merge
  semantics exist.
- **Mixins** — a named, includable block of styles plus furniture, referenced by
  a theme without occupying its inheritance slot.
- **Furniture includes** — narrower: let `prologue`/`header` splice in a named
  fragment, which covers this case without touching inheritance at all.

Worth deciding before more firm identity accumulates: the same duplication will
appear for every letterhead the office adds, and each copy is another place a
partner's name can go stale.

---

## 8. A sub-level counter cannot continue across its parent

**`internal/theme`** — `NumFormat` has no `lvlRestart`

`NumFormat` exposes `format`, `text`, `start`, `indent`, `hanging`, `font`,
`size`, `align`, `suffix`, `style`, `levels` — and nothing that maps to
`w:lvlRestart`. docc never emits that element, so Word applies its default:
a level restarts whenever the level above it increments.

That makes continuous sub-numbering across sections inexpressible. The office's
Stampa- und Lex-Friedrich-Erklärung
(`assets/GmbH_BZ/3 - Stampa und Lex-Friedrich Erklärung GmbH.docx`) numbers it
the other way:

```
A.)  Stampa
     1.  Sacheinlagen und Sachübernahmen
     2.  Beabsichtigte Sachübernahme
     3.  Verrechnungen
     4.  Gründervorteile und Sonderrechte
B.)  Lex Friedrich
     5.  Erwerb von Grundstücken durch Personen im Ausland   ← continues
```

The pack renders that last one as `1.`, and there is no way to say otherwise:
one `heading_numbering` definition applies per schema, `start:` sets a fixed
first number rather than a continuation, and flattening the levels would lose
the `A.)` / `B.)` parts.

### Fix

Add `restart: never | after-parent` (or a plain `lvl_restart: 0|1`) to
`NumFormat`, carry it on `docx.NumLevel`, and emit `w:lvlRestart` from
`writeNumLevel` (`internal/docx/numbering.go:194`). The default stays today's
behaviour.

**Element order matters here.** `CT_Lvl`'s sequence is `w:start`, `w:numFmt`,
`w:lvlRestart`, `w:pStyle`, … — so it goes immediately after the `w:numFmt`
written at `numbering.go:207`, not after `w:start` at 201. Out of order, Word
offers to repair the file, which `docs/ooxml-conformance.md` lists as the
failure mode only a real consumer catches.

**Landed**, with the vocabulary at the theme rather than the level index:
`restart: never | after-parent` on a `numbering:` level, carried as
`docx.NumLevel.Restart *int` (nil = leave the element out, Word's default) and
written between `w:numFmt` and `w:pStyle` as the section demands.
`emit.Validate` rejects any other value, and the LibreOffice round-trip sample
now carries a two-level continuous list.

One finding the proposal could not have: **LibreOffice 26.2 accepts the file and
ignores `w:lvlRestart`**, rendering the sub-level restarted. Word honours it.
Documented in `docs/theme-reference.md` beside the key.

---

## 9. A label-left block pattern for the body

**`internal/emit/emit.go:1106`** — the recommended fix for form layout

This is the one to build. Changes 4 and 5 exist to make a *table* carry a form;
this makes the *body* carry one, keeps every check, and is far smaller than
either.

### The problem

No block pattern puts a label to the left of its value. `.label` emits
description → tab → label (an evidence citation). `.amount` emits description →
tab → currency → tab → figure. And body prose cannot contain a tab at all —
every `docx.Tab{}` the emitter writes comes from furniture or from one of those
patterns. So a two-column form row, the commonest shape in Swiss registry
paperwork, is unreachable from the body.

### The geometry is already proven

Built as a working prototype (`output/fake-gmbh/tab-experiment/` in the pack):
the Anmeldung as tabbed **furniture** rows. It renders exactly right — right-
aligned label column at 39mm, values at 42mm, hanging indent so a five-line
Zweck stays in its column, one page. Furniture runs already support `tab: true`,
`break: true` and `repeat:`.

It is unusable in production for one reason: furniture interpolates frontmatter
as plain text, so the content leaves the body and every span- and div-scoped
check goes dark with it — including the Art. 773 floor. The prototype proves the
layout is right, and proves it has to be reachable from the body.

### Fix — smaller than it looks

`splitEvidenceLabel` (`emit.go:1152`) already returns exactly the right split: a
**plain-text** bracket and a **rich inline** remainder. Form labels are plain
(`Firma:`); form values need spans. That is already the correct way round — only
the emission order is wrong.

`labelledList` (`emit.go:1113`) currently does:

```go
runs := e.runs(description, ...)                           // rich value, left
runs = append(runs, Tab{})
runs = append(runs, Run{Props: labelStyle, Text: label})   // plain label, right
```

A sibling emitting label → tab → description, selected by a new style key
`div.<name>.field`, is roughly fifteen lines and changes no existing behaviour:

```go
runs := []docx.Run{{Items: []docx.Inline{docx.Tab{}}}}
runs = append(runs, docx.Run{Props: docx.RunProps{Style: labelStyle},
                             Items: []docx.Inline{docx.Text(label)}})
runs = append(runs, docx.Run{Items: []docx.Inline{docx.Tab{}}})
runs = append(runs, e.runs(description, docx.RunProps{})...)
```

Three registration points, all small, and missing either of the first two makes
`doctor` call a working mapping unread:

| File | What |
|---|---|
| `internal/emit/keys.go` — `blockSuffixes` (~83) | add `{".field", "selects field rendering; styles the label column", ""}` |
| `internal/emit/keys.go` — `BlockPattern` (~188) | add a `case sc.Styles["div."+name+".field"] != ""` |
| `internal/emit/emit.go` — `(*emitter).div` (807) | one more branch beside the existing `.amount` / `.line` / `.label` ones, dispatching to a `fieldDiv` that mirrors `labelledDiv` |

`div` dispatches by testing each suffix in turn and returning, so `.field` is a
fourth `if` in that chain — and `BlockPattern` in `keys.go` must gain the
matching `case` in the same order, since mapping two suffixes silently takes
whichever the chain reaches first. `docc describe` reports the pattern a block
ended up with, which is the quickest way to confirm the wiring.

Tab stops and hanging indent come from the row style, exactly as `Betragszeile`
already supplies them for amount rendering. Source becomes:

```markdown
::: feld
- [Firma:] [Fake AI]{.firma .docc-field key=firma} GmbH
- [Sitz:]  [Neuenhof]{.sitz .docc-field key=sitz}
:::
```

Spans survive, so `no_blank_spans`, `spans_agree` and `required_spans` keep
working; the block is a div, so `required_div` anchors on it; the Stammkapital
stays a `betraege` block with `amounts_balance` and the Art. 773 floor intact.

The same pattern serves the party blocks in the Kaufvertrag and Pfandvertrag,
the `organ` and `person` entries, and the legal brief's party lines — all "label,
then rich value", all today either flattened or pushed through furniture.

**Landed** as proposed, three registration points and all. `fieldDiv` /
`fieldList` sit beside `labelledDiv` / `labelledList`, `.field` is a fourth
suffix in `blockSuffixes` and a fourth `case` in `BlockPattern`, and the
dispatch order is `.amount`, `.line`, `.label`, `.field`.

Two decisions worth knowing:

- **No list numbering on a form row.** `labelledList` gives each item a bullet
  `numId`; a form row is not a list, and its columns come from the row style's
  tab stops.
- **No corpus fixture.** The three `good/` document types are real documents,
  and none of them contains a form; the pattern is covered by
  `TestFieldDivPutsLabelBeforeValue`, which asserts tab, label, tab, value *and*
  that a `[…]{.firma}` span inside the value survives as a styled run — the half
  the section calls the point of the whole change. The first real form profile
  in the pack is what should earn a fixture.

---

## 10. `docc --version` is not accepted, only `docc version`

**`cmd/docc/main.go`** — papercut

`docc --version` is what every other tool on the PATH answers, and it is what a
CI step or a `Makefile` reaches for first. docc has only the subcommand, so the
flag form fails with a usage error. An alias costs one case in the argument
dispatch and removes a surprise from the first minute of using the tool.

Worth deciding at the same time: `-v` (conventional, but often "verbose") and
`--help`/`-h` beside `docc help`.

**Landed.** `--version` is a second case beside `version`, printing the same
line — a test asserts the two cannot drift. `-v` was deliberately left out: it
reads as "verbose" often enough that answering it with a version is a trap.
`-h`/`--help` already worked.

---

## 11. A pack checkout needs `--schema-dir` and `--theme-dir` on every command

**`internal/project`** — papercut, ½ d

Inside a profile-pack checkout, every invocation carries
`--schema-dir schemas --theme-dir themes`. Discovery walks up looking for a
`.docc` directory, the way git finds `.git`, and a pack repository has no
`.docc` — its `schemas/` and `themes/` are the product, not a project's local
configuration. So ad-hoc authoring inside a pack is verbose in exactly the place
authoring happens most.

Directions, cheapest first:

- **Recognise a pack layout.** A directory holding both `schemas/` and `themes/`
  is a pack; discovery accepts it as a root. Zero configuration, and the
  ambiguity is small — but it is inference, and inference about which schemas
  apply is the thing `docc doctor` exists to make legible.
- **A marker file.** A `docc.yaml` (or a `.docc` that names sibling directories)
  at the pack root, written once. Explicit, discoverable, and it gives the pack
  somewhere to record its id and the docc version its CI pins.
- **Environment variables.** `DOCC_SCHEMA_DIR` / `DOCC_THEME_DIR`, set by a
  direnv or a devbox shell. Solves it for a person, not for a repository.

The marker file is the one that fits how the rest of docc behaves: `doctor`
already reports *which* configuration resolved and from where, and a marker
keeps that answer a fact rather than a guess.

**Landed — and the marker already existed.** Every pack carries
`docc-profile.yaml`, which names `schemas:` and `themes:` and is already
validated by `profile.LoadPack`; it was only ever read for *installed* packs.
`profile.FindPack` now walks up for it the way `project.Resolve` walks up for
`.docc`, and `profile.Resolve` tries it after the two project forms and before
the user default. So no new file, no new configuration language, and a pack
repository is usable from inside itself:

```sh
cd jlmy-profiles/documents
docc doctor --strict      # schemas … (pack-checkout)
docc build gruendung.md
```

A checkout pins nothing — you are working *on* the pack, not consuming a
revision of it — so a document built this way records no commit, and `doctor`
now names which form answered (`pack-checkout`, `project-profile`,
`legacy-project`, `user-default`) instead of saying "discovered".

---

## Considered and rejected

Recorded so nobody re-derives them, or worse, builds one.

**A tab character in body markdown** — an escape, or a `→` marker the parser
turns into `docx.Tab{}`. It would solve 9 in an afternoon, and it is the wrong
shape. It puts layout in the author's hands inside the content, which is the
thing docc's block patterns exist to prevent; `ruledDiv`'s own comment makes the
argument better than this one does — *"a signature line is not content the
author should be typing, and a document whose dots are literal text cannot have
its signatories checked or reordered."* The same is true of a form's tab stops.

**Reusing `.label` by putting the value in the bracket.** Tempting, because it
needs no new code: `- [Fake AI GmbH] Firma:` would render label-left today. It
cannot work. `splitEvidenceLabel` takes the bracket as a **plain string ending
at the first `]`**, so a value carrying
`[Fake AI]{.firma .docc-field key=firma}` truncates at the first bracket. Values
need spans; brackets cannot hold them. This is exactly why 9 flips the emission
order instead of asking authors to flip their source.

**Driving the Anmeldung from frontmatter via furniture.** Prototyped, and it
renders correctly (`output/fake-gmbh/tab-experiment/`). Rejected because
furniture interpolates plain text: the content leaves the body, and every span-
and div-scoped check goes dark with it. It is a good demonstration of the target
layout and a bad production answer.

**A `rows:` contract for tables**, so a schema could require table rows by label
the way `body:` requires headings. Sound, but it invents a parallel checking
vocabulary for tables alone. Change 5 (block content in cells) reaches the same
place through the vocabulary that already exists — and change 9 removes most of
the reason to want either.

---

## Documentation to update alongside

| Change | File | Why | Status |
|---|---|---|---|
| 2 | `docs/theme-reference.md:143` | Currently recommends left-aligning styles over a justified body — the exact no-op. Must be corrected or removed with the fix. | **done** — the recommendation stands and now says the override is explicit |
| 4 | `docs/authoring.md:126` | "What a theme cannot change" lists table borders and even columns as fixed. | still true; change 4 is open |
| 6 | `docs/authoring.md` | `.words` is described as opt-in; make it true rather than rewriting the sentence. | **done** — it is true now |
| 8, 9 | `docs/theme-reference.md`, `docs/schema-reference.md` | New `NumFormat` field; new `div.<name>.field` style key and its rendering pattern. | **done**, plus `docs/building-profiles.md` (the field pattern's tab-stop geometry) and `docs/schema-reference.md` (`on_missing`, for change 1) |

## 12. `.field` gives nested and continuation paragraphs the row style

**`internal/emit/emit.go`** — `fieldList`, found while adopting change 9

Raised after using `.field` for real. It is small and it is not urgent; the pack
has a workaround that is arguably better structure anyway.

`fieldList` labels an item's first paragraph and then, for every later block in
the same item, sets `p.Props.Style = style` — the row style. That is right for
a plain continuation paragraph, but the row style is the one carrying
`hanging: <value column>`, so a continuation's *first* line starts back at the
margin instead of in the value column. It also hits a nested block: a
`::: betraege` inside a form row checks correctly — the div is found, the amount
rules run — but renders at the margin with its own style discarded.

Concretely, this does not work as it reads:

```markdown
::: feld
- [Stammkapital:] wie folgt eingeteilt:

  ::: betraege {#stammkapital}
  - [Fr. 20'000.00] 20 Stammanteile zu je Fr. 1'000.00
  - [= Fr. 20'000.00] Ausmachend das Stammkapital von
  :::
:::
```

### Two workarounds, both fine

A multi-line value needs no nesting at all: a hard line break (`\`) keeps it in
one paragraph, and the row's hanging indent puts every line after the break in
the value column. That is how the pack sets its Belege row.

A nested *block* has to move to the top level instead, which means splitting the
form into two `feld` blocks with the block between them. The pack does that for
`betraege` and `person`, and it keeps every check — so change 9 delivered what it
promised; this is only about where the source may sit.

### Fix

Give continuation and nested paragraphs their own key — `div.<name>.field.continued`
— falling back to the row style when unmapped, so today's behaviour is the
default. A nested block keeping its own mapped style would be better still, but
that is a broader question about how nested divs inherit.

## Suggested order

1. **Change 1** — a check that passes for the wrong reason is worse than no
   check, and this one shipped a deed understating its statutory capital. Start
   as a `doctor` warning; cheap, and catches the class.
2. **Changes 2, 3, 6** — all small, all silent today, all with live workarounds
   in the pack that should be reverted once they land.
3. **Change 9** — the highest-value item here. Half a day, no behaviour change,
   and it gives the registry forms their real layout while keeping every check.
   It also makes changes 4 and 5 unnecessary for this purpose.
4. **Change 8** — one field and one attribute; unblocks the Stampa numbering.
5. **Change 7** — design discussion before the letterhead duplication spreads.
6. **Change 4** — only if something genuinely tabular turns up. Change 9 covers
   the forms.
7. **Change 5** — probably never. It exists to make tables carry checked
   content, and change 9 removes the reason to want that.

### Acceptance, where it is cheap to state

- **1** — a schema with `amounts_balance` on a div no document contains must not
  exit 0. The pack's `gmbh_gruendungsurkunde` with its `betraege` blocks
  rewritten as prose is the fixture; it should fail.
- **2** — a style with `align: left` and `based_on:` a justified style must emit
  `<w:jc w:val="left"/>`. Assertable on the XML, no renderer needed.
- **3** — `sema.BlankFields(sc.Example)` must produce zero diagnostics at
  **warning** severity too. `internal/emit/profiles_test.go:105` currently
  asserts only `HasErrors()`; tightening that assertion *is* the test.
- **8** — a two-level definition with `restart: never` emits `w:lvlRestart` with
  value `0`, positioned between `w:numFmt` and `w:pStyle`.
- **9** — a `div` mapping `.field` emits tab, label, tab, then the description's
  runs; and a `[…]{.firma}` span inside the description survives into the output
  as a styled run. The second half is the point of the whole change.

---

*Found while building the firm's GmbH-Gründung dossier against docc v0.2.0,
August 2026. Every finding was reproduced against the pack; the layout ones were
confirmed by reading emitted OOXML rather than by eye. Questions about any of
them are best answered by rebuilding the fixtures named above — they are all
committed.*
