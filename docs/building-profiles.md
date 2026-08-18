# Building a docc profile from an existing Word document

A *profile* is a schema plus a theme: the contract of a document type and its
visual definition. Most profiles do not start from a blank page — they start
from a `.docx` the office already uses, usually a template full of grey
tab-blanks, `ODER:` alternatives and sections marked "delete if not
applicable". This guide is the working method for turning such a document into
a profile, written for an agent (or a patient human) with a shell, the `docc`
binary, LibreOffice and a way to look at rendered pages.

The method was test-driven on a notarial purchase deed
(`kaufvertrag_beispiel.docx` → `schemas/ch_urkunde_kaufvertrag.yaml` +
`themes/ch_urkunde_kaufvertrag.yaml`); examples below come from that run.

## The loop

```
unpack the .docx ──► inventory structure ──► classify the template's blanks
        │                                            │
        ▼                                            ▼
render original to PNG                    write schema + example
  (ground truth)                                     │
        ▲                                            ▼
        │                                      write theme
        │                                            │
        │                                            ▼
        │                                     docc doctor ──┐ wiring sound?
        │                                            │      │
        │                                            ▼      │
        └── visually compare ◄── render to PNG ◄── docc build
                 │                                          │
                 └── iterate: fix theme/schema, or the engine ◄┘
```

Two renders of the *same pipeline* (LibreOffice → PDF → PNG) are compared, so
renderer quirks cancel out. Do not compare a Word screenshot to a soffice
render and chase differences that are LibreOffice's, not yours.

`docc doctor` sits between writing and rendering because it answers, in
milliseconds, the question a render answers in a minute: is this profile
wired up at all. Run it after every edit to a schema or theme.

## 1. Unpack and render the original

```bash
mkdir -p /tmp/profile && cd /tmp/profile
unzip -q original.docx -d src
soffice --headless --convert-to pdf original.docx --outdir .
pdftoppm -png -r 60 original.pdf orig       # orig-01.png, orig-02.png, ...
```

The PNGs are the ground truth. Look at every page before writing a line of
YAML: the first page usually has furniture (letterhead, crest, title block)
the body pages do not, and the last pages usually hold signatures and
certification text that will stay in the markdown body.

## 2. Inventory the structure

`word/document.xml` is too big to read raw. Dump one line per paragraph:
style, numbering reference, run formatting, first 150 characters. A short
Python script over `document.xml` does it (ElementTree, iterate `w:p` /
`w:tbl` under `w:body`; for each paragraph read `w:pStyle`, `w:numPr`, and
per-run bold/italic/underline).

From the dump and the PNGs, write down:

- **Heading hierarchy and numbering.** Which styles are headings, what labels
  they carry (`I.` / `1.` / `1.1.`), where numbering starts, whether lower
  levels restart under higher ones. In the deed: h1 upper Roman, h2 decimal
  restarting per h1, h3 `x.y.` — all generated, so in docc they become a
  `render.heading_numbering` definition and the markdown source stays free of
  typed numbers.
- **Page geometry.** `w:pgSz` / `w:pgMar` in the `sectPr` elements, converted
  from twips (÷ 56.7 ≈ mm). Multiple sections usually mean a distinct first
  page — in docc that is `title_page: true` plus `header: first:`.
- **Base typography.** `word/styles.xml` → `docDefaults` and the named styles:
  font, sizes (half-points), small caps, alignment. The deed: 10pt body
  justified, 16pt bold centered title, small-caps headings.
- **Headers, footers, images.** `word/header*.xml`, `word/media/*`. Extract
  reusable assets (a crest, a logo) — they go next to the theme file and are
  referenced with `image: { path: ... }`. Check the rendered pages for page
  numbers and other field codes.
- **Repeated visual patterns.** Anything that appears many times with the
  same shape is a candidate semantic block: evidence lists, amount lines with
  a right-aligned CHF column, party paragraphs, certification clauses.

## 3. Classify the template's blanks — the crucial step

The old template works by *deletion and filling*: every deed starts as all
variants plus blanks, and the notary deletes what does not apply. A docc
source works by *authoring*: the markdown contains what the deed says,
nothing else. So every grey blank and every `ODER:` in the template must land
in exactly one of four places:

| Template artifact | Where it goes | Mechanism |
|---|---|---|
| Facts the fixed furniture prints (notary name, title, letterhead fields) | frontmatter | theme interpolates `{{ field }}` in prologue/epilogue/header |
| Facts of the individual document (parties, object, price, dates) | authored body text | validated via `blocks:` / `spans:` declarations |
| Facts transcribed from a register (Firma, UID, Grundbuchbeschrieb, Bürgerort) | frontmatter **and** authored body text | the value is set once in frontmatter, written into the prose as a span, and tied to it with a `span_matches_field` rule |
| `ODER:` alternatives, "delete if not applicable" sections | nowhere — the author writes the applicable wording | variants of a semantic block where structure matters; otherwise just prose |
| Blanks filled in by hand on paper (Beurkundungsdatum, Protokoll-Nr.) | body, as visible blanks | `[____]{.docc-field key=...}` + `fields:` with `completion: handwritten` |

Rules of thumb:

- Frontmatter is for what the *theme* needs to render furniture, and for
  document-level facts worth machine-reading later. Do not hoist body prose
  into frontmatter just because the template had a blank there — the deed's
  parties belong in the deed's text.
- A blank that must be filled *before* the document is final gets
  `completion: before-execution` (the default): `docc check` accepts the
  draft, `docc build` refuses while it is blank. A blank completed with a pen
  gets `completion: handwritten` and survives into the rendered page.
- **A value that must be stated, but not inside a block, gets `required: true`
  on its `spans:` declaration.** It asks that a span of that type appear
  somewhere in the body. Reach for it when the value lives in a flowing
  sentence: wrapping the paragraph in a `:::` block to get at `required_spans`
  is structure invented for the checker rather than for the document.
- **Decide per value which mistake you are guarding against**, because that is
  what picks the mechanism, not the value itself:

  | The value | The risk | Declare it as |
  |---|---|---|
  | Carried forward unchanged — the firm, the Grundbuchamt, a standard clause | It is right today; the danger is the day it changes and you update three of the four places | a **span** watched by `spans_agree` |
  | Restated fresh in every document — the party, the price | You forget to change it | a **blank** (`fields:` + `.docc-field`) |
  | Restated by copying the last document of this type | The value is filled and consistent — with the *previous* matter's — so neither the blank gate nor `spans_agree` can see it | a **span** anchored with `span_matches_field` to a frontmatter field |

  The last row is the one a template cannot help with and the one that files
  the wrong client's name. Anchor the values a register supplies: the
  frontmatter is where a Handelsregister or Grundbuch lookup deposits them, so
  it is the one place that decides what the value is, and the prose is checked
  against it.
- If the template's alternatives differ in *required structure* (a party that
  is a person needs name/birth date/place of origin; a company needs
  name/UID/seat), model them as one block with a `discriminator:` and
  `variants:`, each variant naming its `required_spans`. The author picks the
  variant with one attribute and the checker enforces the rest.

## 4. Write the schema

Every key, with its accepted values and defaults, is in
[schema-reference.md](schema-reference.md); the theme's are in
[theme-reference.md](theme-reference.md). What follows is the order to work in,
not the full list.


Order of work inside the YAML:

1. `frontmatter:` + `types:` — only what step 3 put there.
2. `body:` — the heading skeleton with `required:` / `ordered:` /
   `required_when:`. Only structure whose *absence is a defect*; a template
   heading the office sometimes omits is `required: false` or absent.
3. `blocks:` / `spans:` — the semantic markup from step 3. Declaring any
   blocks makes undeclared `:::name` blocks an error (same for spans), so
   declare everything the type permits in one sitting.
4. `fields:` — the handwritten/deferred blanks.
5. `styles:` — the markdown-construct → style-name map. Invent honest names
   now (`Vertragspartei`, `Betragszeile`); the theme defines them next.
   The set of keys is closed; the README's *style map* table lists all of
   them, and `docc doctor` warns about a mapping nothing reads.

   For a block, **which key you map selects a rendering pattern**. The block's
   own declaration says nothing about this, so it is worth deciding
   deliberately:

   | Mapped | Pattern |
   |---|---|
   | `div.<name>` only | plain — every paragraph in that style |
   | `div.<name>.label` | labelled — `- [LABEL] description` comes out as description, tab, label at a tab stop. Evidence references. |
   | `div.<name>.field` | field — `- [LABEL] value` comes out as tab, label, tab, value. Registry forms and party blocks: label column left, rich value beside it. |
   | `div.<name>.amount` | amount — a right-aligned amount column; `.total` and `.total.amount` style the total row, `.words` the spelled-out sum (needs the theme's `formats.amount_words`) |
   | `div.<name>.line` | ruled — a signature or entry line |

   They are tried `.amount`, `.line`, `.label`, `.field`; mapping two takes the first
   silently. `docc describe <type>` prints the pattern each block resolved to.
6. `render:` — heading/paragraph numbering, referencing definitions the theme
   will declare.
7. `rules:` — pick from the registry (`no_placeholder_text`,
   `div_items_match`, `cross_reference`, `no_empty_sections`,
   `amounts_balance`, `spans_agree`, `span_matches_field`) with schema-owned
   codes. `div_items_match` with a
   pattern is the cheap way to enforce a per-line shape inside a block, e.g.
   every Betragszeile starts with `[Fr. ...]`; `amounts_balance` then checks
   that those figures add up.

   Reach for a rule wherever the old template relied on the drafter noticing
   something. A price whose parts do not sum to the total, or payments that
   leave part of it unsettled, are invisible to a reader working down the
   page and trivial for the compiler:

   ```
   error[KFV013]: declared total 865'000.00 does not match the sum of the
   items, 864'000.00
   error[KFV013]: the amounts do not settle "kaufpreis": 78'500.00 is
   unaccounted for
   ```

   The same applies to the drafter noticing that a value was carried over from
   the document this one was copied from. `spans_agree` catches the half-done
   edit; `span_matches_field` catches the one that was never started:

   ```yaml
   - id: KFV015
     check: span_matches_field
     args: { span: firma, field: kaeuferin.firma }
   ```

   ```
   error[KFV015]: `.firma` says "Muster AG", but `kaeuferin.firma` says
   "Beispiel Immobilien AG"
   ```
8. `example:` — a compact but *complete* document. Write it against the real
   template content, scrubbed. This is the profile's spec: it exercises every
   block, span, field and furniture line you declared.

## 5. Write the theme

Copy the structurally closest existing theme and edit against the
measurements from step 2 — page, margins, `title_page`, defaults, every style
the schema maps, the numbering definitions the schema names, header/footer,
prologue/epilogue. Notes that bite:

- The block rendering patterns all lean on tab stops, and the stops live in
  the theme:
  - **labelled** (`div.<name>.label`): give the block style a right stop and
    the label lands there. It only does so if the description fits before it
    — a description that wraps past the stop swallows the tab and the label
    runs on inline. Keep those descriptions to one line, and say so in the
    block's `description:` so the author is told rather than surprised.
  - **amount** (`div.<name>.amount`): two stops, one left for the currency
    and one right for the figure, so a column of prices aligns on the
    decimal. `.total` styles the row marked `=`; a character style under
    `.total.amount` is where a rule under the figure belongs, which is what
    the old templates drew as `=================` by hand. When a block
    declares a total its other items are rendered as a list, using the
    schema's `bullet_list` mapping: parts that add up to something read as
    parts, while a block stating one amount stays unmarked, because a bullet
    in front of a single figure is a list of one. `.words` is worth setting
    as a gloss — smaller and italic — because the spelled sum is the
    same amount said twice and the reader should see at a glance which line
    is the figure and which is the safeguard.

    Give a long payment schedule one sub-section per instalment — a numbered
    heading, then a one-item money block, then the bank details — rather than
    one block of many items. The figures still land in a single right-aligned
    column and stay summable by eye, but each instalment gets a name instead
    of a bare `2.1.`, and the section it belongs to is obvious. Blocks that
    name the same `total-of` are summed together, so splitting them up costs
    nothing in checking.
  - **field** (`div.<name>.field`): a right stop for the label column and a
    left stop just past it for the value, plus a hanging indent equal to the
    second stop so a value that wraps stays in its own column. This is the
    two-column form row Swiss registry paperwork is made of — `Firma:`,
    `Sitz:`, `Zweck:` — and it is the labelled pattern with the order
    reversed, because a form's label comes first. The value keeps its spans,
    which is the whole reason to build the form out of body content instead
    of furniture: `no_blank_spans`, `spans_agree` and `required_spans` go on
    applying to it, and the block is still a div, so `required_div` anchors
    on it.

    ```markdown
    ::: feld
    - [Firma:] [Fake AI]{.firma .docc-field key=firma} GmbH
    - [Sitz:]  [Neuenhof]{.sitz .docc-field key=sitz}
    :::
    ```
  - **ruled** (`div.<name>.line`): the emitter writes one tab per stop the
    style declares, so a stop with no leader followed by one with
    `leader: dot` gives a gap and then a rule — a signature line whose shape
    is entirely the theme's business. Do not let anyone type the dots.
- `formats.amount_words: "(Franken %s)"` spells every amount out beneath its
  figure, rendered from the figure itself. Deeds repeat sums in words so they
  cannot be altered after signing; generating them means the words and the
  digits cannot disagree, which is the failure the words exist to prevent.
  The speller is German — a theme in another language leaves this unset.
- A `span.<type>` style is how an annotation earns its appearance: mapping
  `span.name` to a bold, underlined character style makes every
  `[Anna Muster]{.name}` in the document look like a party name, without one
  word of formatting in the source. This is the single highest-leverage
  mapping in a profile — the house conventions that a Word user applied by
  hand, once per occurrence, become a consequence of what the text *is*.
- Some constructs never reach the theme at all: `**bold**`, `*italic*`,
  inline `` `code` `` (always Courier New), links (always `0000EE` and
  underlined, and rendered as text rather than a live hyperlink), and table
  borders and column widths (a 0.5pt grid, columns split evenly). Do not
  spend an afternoon on a style for one of these — the README lists them
  under *What a theme cannot change*, and `docc doctor` says so if a schema
  maps one.
- Left-align styles for content with manual line breaks or short lines
  (party entries, address blocks) even when the body justifies — Word
  stretches justified lines that end in a manual break.
- Numbering `levels:` is a flat list, not a tree: the definition is level 0,
  `levels[0]` is level 1. Label text like `"%2."` shows only the current
  level's count; `"%2.%3."` composes.
- Interpolated fields must exist in the schema; `docc build` validates the
  pair and refuses on typos, so build early and often.
- `render.page_break_before_headings` is where a document type says its
  certification starts a fresh sheet. Page breaks the markdown does not
  express belong here for the same reason numbering does: where a deed breaks
  is a fact about deeds, not about this deed. Pair it with `keep_next` on the
  signature lines so the parties cannot end up signing on different sheets.

## 6. Check the wiring with `docc doctor`

Before rendering anything, ask the compiler whether the profile is connected:

```bash
docc doctor              # what resolved, and is every schema/theme pair sound
docc doctor --strict     # make the warnings bind
docc doctor --json       # the same report, for a script
```

It prints which configuration won, lists every type with the theme it
names, and then runs the schema-against-theme agreement check that otherwise
only runs inside a build — so a style the theme does not define, or a
`{{ field }}` the schema does not declare, surfaces without a document to
build:

```
ch_urkunde_kaufvertrag → theme ch_urkunde_kaufvertrag
  schema "ch_urkunde_kaufvertrag" styles span types it does not declare:
    span.nmae
  schema declares: datum, geburtsdatum, grundbuch, heimatort, name, ...
```

The warnings are the more interesting half, because they catch the failure
mode a profile author cannot see: a mapping that nothing reads.

```
1 warning(s) — these render as if the mapping were absent:
  ch_urkunde_kaufvertrag: styles: code_span — not a construct docc styles;
  `inline code` is rendered with fixed formatting (Courier New, ...)
```

`code_span: Code` is a plausible-looking line that validates, renders and
does nothing — and so is `div.betreage` with the letters transposed. Both are
otherwise silent, and both cost an afternoon of blaming the theme. Treat a
doctor warning as an error while building a profile: if a mapping is unread,
either the key is wrong or the mapping should go.

The same finding exists on the theme side, and cost a real letter before it
became a check:

```
theme starter-letter: prologue line 12 (BeilagenHeader): `omit_if_empty` has no
placeholder to be empty on a line of fixed text — remove it, or use
`if_nonempty: <field>`
```

`omit_if_empty` asks whether every placeholder a line filled in came out empty,
so a line of fixed text has nothing to be empty and the flag does nothing
whatever it is set to. The knob for a literal heading that must disappear with
its list is `if_nonempty:`.

Doctor also reports a rule scoped to a block nothing makes mandatory. That one
is worse than an unread mapping — the rule reports success for a document it
never examined — so answer it deliberately: pair it with `required_div`, or say
`on_missing: ignore` to record that the absent case is legitimate.

## 7. Iterate visually

```bash
docc example ch_urkunde_kaufvertrag > /tmp/kv.md
docc check /tmp/kv.md
docc build --output /tmp/kv.docx /tmp/kv.md
soffice --headless --convert-to pdf /tmp/kv.docx --outdir /tmp
pdftoppm -png -r 60 /tmp/kv.pdf mine
```

Iterate against *finished* documents, not only the blank template. A template
shows every alternative and no decisions; a signed deed shows which
alternatives survive together, how long the real prose runs, and where the
page actually breaks. Rebuild two or three real ones as docc sources and
render those. Every hard requirement in the reference profile came from that
step and none of them were visible in the template: that a party's name is
bold and underlined, that amounts sit in a currency column and a figure
column with the sum spelled out beneath, that the payments have to settle the
price, and that the signatures stay on one sheet and close with *Es folgt die
Beurkundung* before the certification starts its own page.

Keep those documents out of git — they are client files. An ignored
`assets/` directory is enough.

Put `mine-1.png` next to `orig-01.png` and compare *deliberately*, page by
page: geometry first (margins, where the body starts), then blocks (is every
piece of furniture present, in order), then typography (sizes, caps,
weights), then detail (tab positions, rules, spacing). Fix the theme, rebuild,
re-render. Convergence is quick because each pass has a concrete diff.

Expect to hit engine limits; sort each into one of three bins:

- **Your YAML is wrong.** Most differences. Fix and loop.
- **The engine has a bug.** The deed run found one: images in headers
  rendered blank because the writer emitted no per-part `.rels` for header
  parts (OPC scopes relationships to the part; `r:embed` in `header1.xml`
  cannot resolve against `document.xml.rels`). Diagnose by unzipping *your
  built* `.docx` and reading the parts — the golden corpus and
  `testdata/golden/` show what correct output looks like. Fix in
  `internal/docx`, add a unit test, run `task`.
- **The feature does not exist.** Decide whether to live without it, emulate
  it, or grow the engine — and when you live without it, record the decision
  as a comment at the top of the theme, because a theme that silently lacks
  page numbers looks like an oversight to the next reader. The deed run hit
  this twice and answered it differently each time: spans reached the emitter
  carrying no type at all, which made the house convention for a party name
  impossible to express, so `span.<type>` styles were added; running page
  numbers were first lived without and documented as absent, then added as the
  reserved `{{ page }}` and `{{ pages }}` placeholders once a second profile
  wanted them. The test is whether the gap is a *document-type* fact the engine
  should be able to state — and how many profiles will restate the workaround.

Stop iterating at *faithful*, not *pixel-identical*. The profile replaces the
template; it does not have to reproduce its accidents (Word's default fonts,
inherited spacing quirks, the highlighted placeholders themselves).

## 8. Finish

- `docc doctor --strict` — clean, with no unread mappings left over.
- `docc describe <type>` — read the contract as an author will see it; fix
  descriptions and hints that read poorly. It prints the rendering pattern
  each block resolved to, which is the one thing the block's own declaration
  does not say.
- `docc example <type> | docc check` must be clean, and the example must
  build; keep both true forever, they are the profile's regression test.
- Author one real document with the new profile before calling it done. The
  example is friendly; a real deed finds the block you forgot to declare and
  the hint that misleads.
- Commit schema, theme and assets together, with the source template's name
  in the schema's header comment so the lineage is findable.

## 9. Ship it as a pack repository

Authoring happens in a pack checkout — `docc init` gives you one — and the
same checkout, pushed to a Git repository of its own, is the pack every
project and every colleague resolves as an identical, pinned revision. docc
hosts no packs and ships nobody's: an organisation keeps its own, the way it
keeps the letterhead it derived them from.

```text
kanzlei-profiles/
  docc-profile.yaml
  schemas/   ch_urkunde_kaufvertrag.yaml, _base.yaml, ...
  themes/    ch_urkunde_kaufvertrag.yaml, urkunde-wappen.png, ...
```

```bash
cd kanzlei-profiles    # the checkout you authored in
git init && git add -A && git commit -m "kanzlei profiles"
```

`id` is stable and filesystem-safe: it names the directory revisions install
into, and every existing binding checks against it, so renaming it later breaks
them all. `schemas` and `themes` are relative paths inside the repository —
nothing outside it can be named.

Keep client material out, by the rule of step 7, which matters more here
because a pack is the artefact that gets shared:

```gitignore
output/
assets/
*.docx
*.pdf
```

Theme assets are the exception. The crest is referenced by `themes/`, so it is
committed with them.

Guard the repository in CI with the two checks the profile already has to pass,
against a pinned `docc` release:

```bash
docc profile use "$PWD" --project /tmp/verify   # validates every schema/theme pair
docc example <type> | docc check                # once per renderable type
```

The first is not a formality: installing a pack runs `emit.Validate` over every
renderable type, so a revision whose schemas and themes disagree fails at
`install` and never becomes selectable. Catching it in the pack's own CI is
the difference between one broken push and every colleague's next build.

### Consuming it

`docc profile use` writes a project binding and lockfile that belong in Git
beside the documents; `docc profile install --default` records a machine-wide
fallback for loose drafting. Both clone into
`$XDG_DATA_HOME/docc/profiles/<id>/<commit>/`, immutable and with the Git
metadata removed. `docs/profile-packs.md` is the reference for the commands,
the trust policy, and the provenance every rendered file carries.

### While you are still editing the pack

`source` is passed to `git clone`, so a **local path is a valid source**:

```bash
docc profile use ~/git/kanzlei-profiles --project .
```

That clones the committed `HEAD`. Working-tree edits stay invisible until you
commit and `docc profile update` — right for a build that has to be
reproducible, tiresome for the render-and-compare loop of step 7. For that
loop, work inside the checkout itself (its manifest resolves directly), or
point another directory at the working tree:

```bash
export DOCC_PROFILE=~/git/kanzlei-profiles
```

An edit then lands in the next `docc build`. Nothing is pinned, which is why
this is a development arrangement: use a committed binding for anything shared
or filed.

## Worked example

The `ch_urkunde_kaufvertrag` profile is the reference run of this guide.

`schemas/ch_urkunde_kaufvertrag.yaml` declares a `partei` block with
four variants, a `grundstueck` block, `betraege` money blocks that must
balance, an `unterschriften` signature block, seven span types, and two
handwritten fields. `themes/ch_urkunde_kaufvertrag.yaml` puts the crest
in a first-page header, the Urkunde title block between two rules, a
small-caps I./1./1.1. outline over the headings, amounts in two columns with
their sums spelled out, and signature lines drawn by a tab leader.

Every decision in this guide is made once there, concretely, and each of the
four hard parts came from comparing against real signed deeds rather than
from the blank template: the party-name convention, the amount columns, the
arithmetic that has to hold between the price and its payments, and the
signature block that closes with *Es folgt die Beurkundung* before the
certification starts its own page.

The Word originals are not kept in the repository — reference material lives
in an ignored `assets/` directory, and what survives in git is the crest
(`themes/urkunde-wappen.png`) and the schema's header comment recording
the lineage.
