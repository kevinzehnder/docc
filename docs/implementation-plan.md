# Implementation plan: semantic Markdown compilation

This plan maps [product-strategy.md](product-strategy.md) onto the current
codebase. Each phase lists what exists today, what is missing, and where the
work lands. Phases are ordered by dependency: nothing in a later phase should
require reworking an earlier one.

## Where the code stands today

| Strategy concept | Current state |
| --- | --- |
| Semantic `:::type` blocks | `parse.Div` recognises `::: name` fences (`internal/parse/fences.go`) but carries only a name — no `{#id key=val}` attributes |
| Inline annotated spans `[text]{.type key=v}` | Not parsed at all |
| Schemas (the strategy's "profiles") | `internal/schema` covers frontmatter fields, body headings, named rules, styles, render numbering — no `blocks:` or `spans:` sections |
| Typed values / normalisers | Frontmatter has `date` and `formats.date`; no money, UID, percentage, no normalisation-for-comparison machinery |
| Consistency by key | Nothing groups repeated values |
| Diagnostics | Stable codes, positions, hints, JSON rendering (`internal/diag`) — but no `block`/`key`/`expected`/related-occurrences fields |
| CLI | `check`, `build`, `init`, `lsp`, `types`, `themes`, `explain`, `version` — no `describe`, `example`, or `inspect` |
| Artifact verification | None; `internal/docx` writes deterministically but nothing reads a built artifact back |
| PDF | Not a product output. The LibreOffice conversion path in `build` is a deprecated fallback; DOCX is the artifact docc stands behind |

## Phase 1 — general semantic infrastructure

Goal: the two syntax extensions (attributed blocks, annotated inline spans)
exist in the parser, are declared in schemas, and produce diagnostics with the
strategy's §12 fields. `beweis` becomes the first registered block instead of a
special case.

1. **Block attributes.** Extend `splitFence` / `Div` in
   `internal/parse/fences.go` to parse a pandoc-style attribute block on the
   opening fence: `::: partei {#verkaeufer kind=person role=veraeusserer}`.
   `Div` gains `ID string` and `Attrs map[string]string`, each with byte
   offsets so diagnostics can put the caret under the offending attribute.
   Positions stay byte-based per the project convention (`diag.Position.Col`
   and `.Len` are bytes; the caret renderer converts).
2. **Inline spans.** New inline parser in `internal/parse/spans.go` for
   `[literal]{.type key=value ref=id}`. This must be a custom goldmark inline
   parser registered ahead of the link parser (trigger `[`, look ahead for
   `]{`); goldmark's built-in attribute support only covers blocks. The node
   keeps the literal text verbatim, the class, the attributes, and byte
   positions. Note the goldmark caveats in CLAUDE.md: this is an inline node,
   so `Lines()` is off-limits — carry the segment explicitly.
3. **Schema `blocks:` and `spans:`.** Add to `internal/schema/schema.go`:
   block declarations (permitted attributes, discriminator + variants,
   `required_spans`, cardinality) and span-type declarations (validator name,
   consistency policy). `extends` resolution in `load.go` must merge both.
4. **Sema passes.** New passes in `internal/sema`: unknown block kind, unknown
   span class, missing required attribute, unclosed div (exists), duplicate
   `#id`. Passes append to the shared diagnostic list — they never stop early.
5. **Diagnostic payload.** Extend `diag.Diagnostic` with optional `block`,
   `key`, `expected` (constraint + one valid-syntax example) and
   `related []Location` fields, `omitempty` so existing goldens for
   diagnostics that don't use them stay stable. Every new code gets an
   `explanations` entry in `cmd/docc/main.go`.
6. **`beweis` migration.** Register `beweis` as a schema-declared block in
   `testdata/schemas/ch_legal.yaml` (the schema rename below happens first in
   this phase) and delete any beweis-specific paths outside
   the generic machinery. `div_items_match` keeps working against the
   attributed `Div`.

Exit criterion: `legal` fixtures pass with `beweis` declared, not hard-coded;
a fixture with a bad span/block produces a §12-shaped JSON diagnostic.

## Phase 2 — typed values and consistency (deferred)

**Deferred 2026-08-15 (user decision):** value-level verification — money,
UID, date normalisation, consistency-by-key — is not important yet. The
sequence continues with agent ergonomics (`describe`/`example`) and the
structural work; revisit this phase when a real document needs it.

Goal: repeated annotated values are grouped by key, parsed, normalised, and
compared; disagreement is one error listing every location.

1. **`internal/typeval` package.** Validators + normalisers for `money`
   (CHF apostrophe/space/dash-cents variants → one normal form), `date`
   (reuse the frontmatter date formats), `uid` (CHE-###.###.###),
   `percent`, `string`. Each returns (normalised value, ok, hint). No
   third-party dependency — this is string parsing the stdlib covers.
2. **Consistency pass.** Sema groups spans by `key` across the document,
   normalises per the span type, and emits one diagnostic per conflicting
   group with all occurrences in `related`. The schema's span declaration
   chooses `consistency: exact | normalised`.
3. **References.** Spans with `ref=` resolve against block `#id`s; a dangling
   ref is an error naming the known ids. No separate declaration syntax —
   repeated keys and `ref=` are the whole model (strategy §6).

Exit criterion: the two-price fixture from strategy §6 produces exactly one
error with both locations; harmless formatting variants produce none.

## Phase 3 — `ch_deed` schema (Öffentliche Urkunde)

Goal: the first real domain schema built purely from schema data — the
compiler gains only generic mechanisms.

1. **`partei` blocks** with `discriminator: kind` and person/company variants,
   expressed entirely in the schema (`required_spans` per variant).
2. **Party references** — `[Veräusserer]{.partei ref=verkaeufer}` — resolved by
   the Phase 2 machinery; literal wording stays the author's.
3. **Intentionally incomplete fields.** Span class `.feld` plus schema
   `fields:` with `completion: handwritten | before-execution`. Sema
   distinguishes present/blank/absent/invalid/inconsistent; `build` decides
   which blanks are errors.
4. **Authored images.** Attribute-carrying images with named `placement`,
   validated against the schema's permitted placements; the theme translates
   placement names into layout. Decorative furniture stays in the theme.
5. **Fixtures.** New `testdata/schemas/ch_deed.yaml` + theme + `good/`/`bad/`
   fixtures + goldens. Keep the existing `legal` and `letter` corpus roles
   intact — they cover complementary theme surface (see CLAUDE.md) — the new
   schema is a third corpus member exercising blocks/spans/fields, not a
   replacement.

Exit criterion: a complete Urkunde fixture builds to `.docx` deterministically
(`TestBuildGolden`), and each §7/§8 failure mode has a `bad/` fixture.

## Phase 4 — artifact verification

Goal: `docc inspect` and a build report close the loop on the rendered
artifact.

1. **`docc inspect <file.docx> --format json`.** Read the archive with
   `archive/zip` + `encoding/xml` (read side has no exact-prefix problem, so
   plain decoding is fine): styles used, numbering instances, sections,
   headers/footers, images and their anchors.
2. **Assertions.** Profile-declared layout assertions (required styles
   present, protected blocks unsplit, page geometry, image regions) checked
   against the inspected structure. Structural and geometric checks are
   primary; no pixel diffing (strategy §13.3).
3. **`--report build-report.json`** on `build`: diagnostics + artifact
   assertion results in one document, atomically written like the other
   outputs.
4. **PDF (decided: not first-class).** DOCX is the supported output. The
   LibreOffice path stays as a deprecated fallback only — no new features
   build on it, no PDF-side assertions. Artifact verification targets the
   DOCX; `task test:roundtrip` remains the proof that structural changes
   produce files a real renderer opens. Ignore strategy §11.3's `--pdf`
   example on this point.

## Phase 5 — agent ergonomics

Goal: an LLM discovers the contract from the binary, not from prose docs.

1. **`docc describe <type> --format json`** generated from the resolved
   schema: blocks, attributes, required spans, span types + validators,
   fields, rules, one syntax example per construct. Single source of truth —
   there is deliberately no separate prompt schema to drift (strategy §10).
   Examples live in the schema as data, not in Go.
2. **`docc example <type>`** printing a compact canonical document, checked in
   CI by running `docc check` on its own output.
3. **Diagnostic tuning.** Run the full write → check → build → inspect →
   revise loop with an agent against the `ch_deed` schema; adjust hints and
   `expected` payloads where the agent stumbles. The Cowork skill wraps the
   same CLI surface.

Exit criterion: strategy §17 — an agent completes the loop on a fresh schema
without reading the repository.

## Rendering patterns

Specialised block rendering is a fixed vocabulary of **patterns implemented in
Go, selected by schemas** — never bespoke code per block name. The existing
labelled-evidence rendering is the template: `emit.labelledDiv` knows no legal
concept, and a schema opts in by mapping `div.<name>.label` to a style. Every
future pattern (party block, field-with-blank, signature lines) lands the same
way: generic, opinionated, opt-in via a style key or schema knob.

The division of labour: users declare blocks, validation and styling in
schema + theme YAML; developers add a pattern once when a layout genuinely
cannot be expressed with paragraph styles. Docc is modular in what a schema
selects, opinionated in what each pattern does — a pattern's layout is not
configurable beyond its styles, because a configurable pattern is a template
language wearing a disguise (§15: no second templating syntax).

## Cross-cutting rules

- Diagnostic codes are stable once released; new checks get new codes and
  `docc explain` entries.
- Every new schema knob must be read by something (`default:` and
  `formats.date` were once declared but dead — that class of bug).
- Schema/theme mismatches stay caught in `emit.Validate`, the only place
  holding both.
- Determinism in `internal/docx` is non-negotiable: no clocks, no cross-call
  counters.
- Before any new primitive, run the §15 guardrail questions; the default
  answer to new syntax is no.

## Terminology and schema naming

**Decided: "schema" is the word**, in code, CLI, and documentation. Where the
strategy document says "profile", read "schema" (a schema plus its theme).

**Schema names carry a jurisdiction prefix**: `<region>_<kind>`, e.g.
`ch_letter`, `ch_legal`, `ch_deed` — leaving room for `us_letter`,
`de_brief`, and the like without renaming anything later. Consequences:

- The Phase 3 schema is `ch_deed`, not `oeffentliche-urkunde`.
- The existing `legal` and `letter` document types become `ch_legal` and
  `ch_letter`. `document_type` is matched as a string, so this is a rename in
  `testdata/schemas/*.yaml`, fixtures' frontmatter, goldens, and any docs —
  mechanical, but do it early (Phase 1, before new fixtures multiply) and in
  one commit. Jurisdiction-neutral bases like `_base.yaml` keep no prefix.
- Swiss concepts live only in `ch_*` schema files; the compiler stays
  jurisdiction-free (strategy §10 unchanged).
