# OOXML conformance: what the standard is, and how far docc should chase it

Status: **plan, not implemented.** Nothing described under "The three layers"
exists in the tree yet. This document records the research so the work can be
picked up later without redoing it.

## The standard

`.docx` is not a proprietary format. It is **Office Open XML**, published as
**ECMA-376** and adopted as **ISO/IEC 29500**. Both texts are free downloads —
ECMA from `ecma-international.org/publications-and-standards/standards/ecma-376/`,
ISO because 29500 is one of the publicly available ISO standards. The ECMA
download ships the normative **XSD schemas** alongside the prose, which is the
part that matters for us.

Four parts:

| Part | Title | What it governs for docc |
|------|-------|--------------------------|
| 1 | Fundamentals and Markup Language Reference | WordprocessingML and DrawingML element grammar; `wml.xsd`, `dml-*.xsd`, `shared-*.xsd` |
| 2 | Open Packaging Conventions | the ZIP container, `[Content_Types].xml`, `_rels`, part naming |
| 3 | Markup Compatibility and Extensibility | `mc:Ignorable`, `mc:AlternateContent` |
| 4 | Transitional Migration Features | the legacy elements Strict drops |

### Two conformance classes

| Class | Main namespace | Reality |
|-------|----------------|---------|
| Transitional | `http://schemas.openxmlformats.org/wordprocessingml/2006/main` | what Word writes by default, what every consumer reads |
| Strict | `http://purl.oclc.org/ooxml/wordprocessingml/main` | rarely produced, unevenly consumed |

`internal/docx/render.go` emits **Transitional**, and should keep doing so.
Strict buys no interoperability here and forbids constructs (VML among them)
that a header or a legacy field may still want. If Strict is ever wanted it is
a namespace swap plus an audit of every Part 4 element in use — a separate
project, not a flag.

## What "100% compliant" can and cannot mean

It cannot mean a certificate. There is no official conformance test suite for
ECMA-376 and no certification body; there is nothing to be 100% *of*. Worse,
the two properties people conflate are genuinely different: a schema-valid
document can still make Word show a repair prompt, and Word's own output is
not always schema-valid. `CLAUDE.md` already states the practical half of this
("Word rejects rather than degrades").

Three levels of claim, in increasing cost:

1. **Grammar.** Every part validates against the ECMA-376 Transitional XSDs.
   Mechanical, checkable in CI, and a real claim.
2. **Reference integrity.** Every `r:id` resolves, every `numId` names an
   existing `abstractNumId`, every `pStyle` names a defined `styleId`, every
   part appears in `[Content_Types].xml`. Not expressible in XSD — these are
   prose "shall" constraints in Part 1 and Part 2.
3. **Consumer acceptance.** Word, LibreOffice and Google Docs open the file
   without repair. Only testable by running a consumer.

Target the union of the three and say so precisely. The README claim should be
"validated against the ECMA-376 Part 1 Transitional schemas in CI", which is
true and checkable, not "100% compliant", which nobody can back.

## The three layers

Ordered by value per unit of effort. Layer 1 first.

### Layer 1 — in-process package invariants

A validation pass over the built parts, inside `internal/docx`, no
dependencies, always on. It closes the gap between "the writer intended to be
correct" and "this specific archive is". Everything it checks is a bug class
the repository has already met once.

Where: a new `internal/docx/validate.go`, called from `Document.buildParts`
(`internal/docx/package.go:149`) after the map is assembled and before it is
returned, so `Bytes`, `Write` and `WriteTo` all inherit it. It gets the same
input the ZIP writer gets, which is the point — it validates the artifact, not
the model.

Checks, all derivable from the parts map plus the relationship slices:

- every part path in the map has a matching `Default` or `Override` in
  `[Content_Types].xml`, and every `Override` names a part that exists;
- every `r:id` referenced from a part's XML resolves in that part's `_rels`,
  with the expected relationship type (an image reference must land on
  `relTypeImage`, a header reference on `relTypeHeader`);
- no orphan relationship: every declared rel target exists as a part or media
  file;
- relationship IDs unique within a part; drawing `docPr` IDs unique within the
  document (`assignDrawingIDs`, `internal/docx/package.go:264`, assigns them —
  the check proves it);
- `numId` → `abstractNumId` indirection resolves in `numbering.xml`, and no
  paragraph names a `numId` that was never instantiated;
- every `pStyle` / `rStyle` / `tblStyle` names a `styleId` present in
  `styles.xml`;
- Word's structural minimums: body ends with `sectPr`, no empty table cell, no
  header or footer part without a paragraph.

Failure mode: return an error from `buildParts`, so a broken document never
reaches disk. These are compiler bugs, not author errors — they do not belong
in `internal/diag`.

Cost: a day. It needs a light scan of the emitted XML (`encoding/xml` decoder
in token mode is enough; no schema, no model round-trip) or, better, recording
the references during render into a side table the validator consumes, which
avoids parsing what we just serialised. Prefer the side table if it does not
distort the writer.

### Layer 2 — XSD validation in CI

Validate each emitted part against the normative schemas. Build-tagged and
kept out of `task ci`'s hot path, exactly like `test:roundtrip`
(`taskfiles/test.yaml:14`).

Shape:

- vendor the ECMA-376 Transitional XSD set under `testdata/xsd/` (it is an
  import chain — `wml.xsd` pulls `shared-*.xsd` and the DrawingML schemas; one
  file is not enough), or fetch it in the task and cache it;
- a `//go:build xsd` test that builds each `testdata/good/` fixture, unzips it
  to a temp dir, and runs `xmllint --noout --schema` over `word/*.xml`,
  `docProps/*.xml` and `[Content_Types].xml`;
- a `task test:xsd` target that skips when `xmllint` is not on PATH, mirroring
  the `soffice` skip in `internal/docx/roundtrip_test.go`.

Known traps, in the order they will be hit:

- **Validate parts, not the package.** There is no schema for a ZIP.
- **MCE breaks naive validation.** `mc:Ignorable` and any
  `mc:AlternateContent` are not in `wml.xsd`; a document carrying them fails
  against the bare schema. Either preprocess the MCE attributes out before
  validating, or supply the Part 3 schema and validate against the combined
  set. Check what `render.go` actually emits first — if docc emits no MCE at
  all, this trap disappears and the constraint becomes "keep it that way".
- **`[Content_Types].xml` and `.rels` have their own OPC schemas** (Part 2),
  separate from `wml.xsd`.
- **Schema-valid is not repair-free.** Layer 2 does not replace layer 3.

Cost: a day, most of it fighting the import chain and MCE.

### Layer 3 — Microsoft's own validator (optional)

The Open XML SDK's `OpenXmlValidator` (.NET) checks the schemas *plus* a large
body of Microsoft's semantic rules, targetable per Word version (2007 through
2021). It is the closest thing to an authority that exists, and it catches the
Part 1 prose constraints that layer 1 only covers where we thought to look.

Cost: a `dotnet` dependency in CI and a small C# harness. Worth it only if the
claim needs to be defensible to someone outside the project. Do not add
`dotnet` to the developer's local `task` chain.

### Keep the roundtrip

`task test:roundtrip` stays on top of all three. LibreOffice acceptance is
orthogonal evidence — it is the only check that exercises a real consumer, and
it has already caught what string assertions cannot see.

## Acceptance

The work is done when:

- a deliberately corrupted build (dangling `r:id`, `numId` with no
  `abstractNumId`, part missing from `[Content_Types].xml`) fails layer 1 in a
  unit test, one fixture per bug class;
- `task test:xsd` passes for every `testdata/good/` fixture;
- `task test:roundtrip` still passes;
- the README states the schema claim in the checkable form above.

## Open questions

- Redistribution: the ECMA XSDs are freely downloadable, but confirm the
  licence terms before committing them to the repository rather than fetching
  them in CI.
- Whether layer 1 reads back the emitted XML or records references during
  render. The second is faster and catches the same bugs, but only if it does
  not push bookkeeping into the writer that obscures it.
- Whether any of this belongs in `docc doctor` as an author-facing check. Most
  of it should not: these are compiler invariants, and an author cannot act on
  them.
