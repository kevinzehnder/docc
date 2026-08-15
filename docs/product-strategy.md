# Docc Product Strategy

## Semantic Markdown compilation for LLM-authored professional documents

## 1. Product thesis

Docc is a deterministic semantic Markdown compiler designed for a world in which an LLM writes the source document.

The LLM authors and revises the document. Docc supplies structure, schema validation, consistency checking, deterministic rendering, and precise diagnostics. The result is an iterative authoring loop that produces professional DOCX files without turning Docc into a word processor, form builder, legal drafting application, or general-purpose template language.

The compact product contract is:

> Write freely in Markdown. Mark important semantics. Use blocks where specialised rendering matters. Run Docc. Fix explicit diagnostics. Repeat until valid.

The Markdown remains the authoritative and substantially complete document. Docc should not silently generate substantive text from hidden data.

## 2. The agentic authoring loop

Docc is a compiler and feedback mechanism inside an LLM-driven authoring cycle:

1. The LLM writes readable Markdown.
2. Docc parses the Markdown and its semantic annotations.
3. Docc validates structure, completeness, references, and consistency.
4. Docc returns stable, machine-readable diagnostics.
5. The LLM corrects the source.
6. Docc renders deterministic DOCX output.
7. Docc verifies structural and layout invariants in the generated artifacts.
8. The LLM revises exceptional layout problems.
9. A human performs the final substantive review.

Docc is therefore not expected to make every document valid in one invocation. Its value lies in making the iterative cycle reliable, deterministic, and cheap.

## 3. Division of responsibility

### 3.1 The LLM controls

The LLM controls:

- substantive prose;
- legal or professional reasoning;
- document organisation within the schema's constraints;
- which facts are stated;
- exact wording and grammatical form;
- ordinary Markdown formatting;
- semantic annotations;
- corrections following compiler diagnostics.

### 3.2 Docc controls

Docc controls:

- recognised semantic blocks and inline annotation types;
- schema-defined structural requirements;
- deterministic formatting of special blocks;
- styles, numbering, cross-references, and page behaviour;
- parsing and normalisation of typed values;
- consistency and completeness checks;
- DOCX generation;
- structural and layout verification;
- stable, actionable diagnostics.

### 3.3 The human controls

The human remains responsible for final factual, legal, and professional correctness. Docc can prove that a marked purchase price is present and consistently used. It cannot prove that the price is commercially correct or that the transaction is legally advisable.

## 4. Design principles

### 4.1 The source is the document

Values visible in the output should ordinarily be visible in the Markdown. Docc validates authored text rather than replacing placeholders from large hidden data structures.

This preserves the central benefit of Markdown: a human or LLM can read the source and understand the document without first evaluating a template.

### 4.2 Semantic annotation, not templating

Docc should annotate literal content:

```markdown
Der Kaufpreis beträgt
[CHF 1'250'000.00]{.preis key=kaufpreis}.
```

It should not require substantive interpolation:

```text
Der Kaufpreis beträgt {{ fields.purchase_price }}.
```

The first version shows the actual document and permits validation. The second hides part of the document behind a template evaluation step and encourages bloated front matter.

### 4.3 Progressive structure

Most content should remain ordinary Markdown. Semantic structure is introduced only where it creates concrete value:

1. Ordinary Markdown for ordinary prose.
2. Inline annotations for important typed values and references.
3. Semantic blocks for structured regions requiring specialised validation or rendering.

### 4.4 One mechanism per concern

Docc must avoid parallel ways of solving the same problem.

| Concern | Mechanism |
| --- | --- |
| Ordinary prose and organisation | Markdown |
| Structured document regions | Semantic `:::type` blocks |
| Typed literal values and references | Annotated inline spans |
| Specialised rendering | Block renderers and document schemas |
| Page geometry and global styles | Reference DOCX and theme |
| Completeness and consistency | Schema validation |
| Compiler feedback | Stable diagnostics |

### 4.5 Explicit semantics only

Docc structures and validates what the author explicitly marks. It should not inspect every number or name and guess its legal meaning.

If consistency matters, the LLM marks the value. This keeps behaviour deterministic, local, and explainable.

### 4.6 No silent substantive repair

Docc may validate, normalise for comparison, style, position, number, render, and report. It must not silently decide which of two conflicting purchase prices is correct or rewrite legal prose.

## 5. The semantic Markdown model

Docc needs only two semantic extensions to Markdown:

```text
:::block {attributes}
    A semantic document region

[literal text]{.type key=value}
    A typed value, field, or reference
```

These operate at the two natural Markdown AST levels: blocks and inline spans.

The user-facing vocabulary is five words:

- **span** — an inline annotation on literal text: `[text]{.type …}`. It marks
  a typed value inside prose. A span never changes rendering; it exists for
  validation, consistency, and reference resolution.
- **block** — a `:::name {attributes}` region. It marks a document region and
  earns its place through specialised rendering, structural validation, or
  numbering/layout behaviour — rendering is one possible payoff, not the
  definition.
- **key** — the attribute that groups repeated spans as the same semantic
  value, for consistency checking: `key=kaufpreis`.
- **ref** — the attribute that points a span at a block's `#id`:
  `ref=verkaeufer`.
- **kind** — the attribute that selects a block's schema variant:
  `kind=person`.

Everything in this document reduces to these five terms; `docc describe` and
all diagnostics use them and no synonyms.

### 5.1 Ordinary Markdown

Ordinary sections remain ordinary:

```markdown
## II. Kaufgegenstand

Der Veräusserer ist Alleineigentümer des Grundstücks Grundbuch Baden
Nr. 1234.

Das Grundstück wird mit allen Bestandteilen und Zugehör übertragen.
```

Docc should preserve the LLM's control here.

### 5.2 Semantic blocks

The existing `:::beweis` system establishes the model. `beweis` should be a registered semantic block rather than an isolated parser feature:

```markdown
:::beweis
**Beweis:** Grundbuchauszug vom
[10. August 2026]{.datum key=datum-grundbuchauszug}
:::
```

Other schemas may register additional block kinds through the same mechanism:

```markdown
:::partei {#verkaeufer kind=person role=veraeusserer}
Herr [Max Muster]{.name}, geboren am
[12. April 1975]{.geburtsdatum}, wohnhaft an der
[Musterstrasse 10, 5400 Baden]{.adresse}
:::
```

A block earns its place only if it provides at least one of:

- specific deterministic rendering;
- meaningful structural validation;
- numbering or cross-reference behaviour;
- layout behaviour that ordinary Markdown cannot express reliably.

Otherwise, ordinary Markdown plus an optional inline annotation should be used.

### 5.3 Inline semantic annotations

Inline spans retain the literal authored content while making it machine-verifiable:

```markdown
Der Kaufpreis beträgt
[CHF 1'250'000.00]{.preis key=kaufpreis}.

Die Eigentumsübertragung erfolgt am
[1. Oktober 2026]{.datum key=antrittsdatum}.
```

The annotation class selects a type validator. The `key` groups semantically identical occurrences.

## 6. Consistency without insertion

Repeated values should be authored at every location and annotated with the same semantic key:

```markdown
Der Kaufpreis beträgt
[CHF 1'250'000.00]{.preis key=kaufpreis}.

Vom Kaufpreis von
[CHF 1'250'000.00]{.preis key=kaufpreis}
werden CHF 100'000.00 bei Unterzeichnung fällig.
```

Docc groups the occurrences, parses their literal contents, and compares their normalised values.

For money, harmless formatting differences may normalise to the same value:

```text
CHF 1'250'000.00
CHF 1 250 000.00
CHF 1'250'000.–
```

All can represent the same typed value. A genuine difference produces an error containing every conflicting source location.

The same model can support:

- dates;
- monetary amounts;
- percentages;
- parcel numbers;
- company UIDs;
- protocol numbers;
- defined terms;
- party references;
- document references.

Separate declaration and reference syntax is unnecessary unless a concrete use case proves otherwise. Repeated typed annotations with a common key are sufficient for consistency validation.

## 7. Structured parties

An `Öffentliche Urkunde` requires parties with different schemas. The source should remain readable prose rather than a large front-matter record.

### 7.1 Natural person

```markdown
:::partei {#verkaeufer kind=person role=veraeusserer}
Herr [Max Muster]{.name}, geboren am
[12. April 1975]{.geburtsdatum}, von
[Baden AG]{.heimatort}, wohnhaft an der
[Musterstrasse 10, 5400 Baden]{.adresse}
:::
```

### 7.2 Company

```markdown
:::partei {#erwerberin kind=company role=erwerberin}
Die [Beispiel AG]{.firma}, eine Aktiengesellschaft mit Sitz in
[Zürich]{.sitz}, UID [CHE-123.456.789]{.uid}, mit Geschäftsadresse
[Beispielweg 4, 8001 Zürich]{.adresse}, vertreten durch
[Anna Beispiel]{.vertretung}
:::
```

The schema can discriminate on `kind`:

```yaml
blocks:
  partei:
    discriminator: kind
    variants:
      person:
        required_spans:
          - name
          - geburtsdatum
          - adresse
      company:
        required_spans:
          - firma
          - sitz
          - uid
          - adresse
```

Docc can validate:

- a unique party identifier;
- a recognised party kind and role;
- required fields for the selected kind;
- valid dates and UIDs;
- non-empty annotated spans;
- permitted field cardinality;
- references to existing parties.

### 7.3 Party references

Party references remain literal and grammatically controlled by the LLM:

```markdown
Der [Veräusserer]{.partei ref=verkaeufer} verkauft der
[Erwerberin]{.partei ref=erwerberin} das nachfolgend beschriebene Grundstück.
```

Docc resolves the reference but does not generate the displayed wording. This permits natural German forms such as `die Erwerberin`, `der Erwerberin`, or `durch die Erwerberin` without building an inflection engine.

## 8. Intentionally incomplete fields

A required blank is content, not missing content. It must appear visibly and be semantically annotated:

```markdown
Die Urkunde wurde am
[____________________]{.feld key=beurkundungsdatum}
unterzeichnet.

Protokollnummer:
[____________________]{.feld key=protokollnummer}
```

The schema defines the field contract:

```yaml
fields:
  beurkundungsdatum:
    type: date
    required: true
    completion: handwritten

  protokollnummer:
    type: string
    required: true
    completion: before-execution
```

Docc distinguishes:

- present and populated;
- present and intentionally blank;
- absent;
- populated with an invalid value;
- populated inconsistently in multiple places.

The compilation stage may determine whether a blank is allowed. An execution-time handwritten date may remain blank, while a protocol number required before execution must produce an error.

## 9. Images and placement

Ordinary authored images use Markdown with semantic attributes:

```markdown
![Situationsplan](assets/situationsplan.png)
{.urkundenbild key=situationsplan placement=after-grundstueckbeschreibung}
```

An image with structured accompanying information can use the general block system:

```markdown
:::abbildung {#situationsplan placement=annex}
![Situationsplan](assets/situationsplan.png)

Massstab: [1:500]{.massstab}
Quelle: [Grundbuchamt Baden]{.quelle}
:::
```

The schema may require an image, source, scale, or permitted named placement. The renderer translates named placement into deterministic DOCX layout.

Fixed decorative document furniture, such as a coat of arms or logo, belongs in the reference DOCX or theme rather than authored Markdown.

## 10. Schemas

A schema is the single source of truth for a document class. Schema names carry a jurisdiction prefix — `ch_letter`, `ch_legal`, `ch_deed` — leaving room for `us_letter` and the like without later renames; jurisdiction-neutral bases stay unprefixed. A `ch_deed` schema (Öffentliche Urkunde) may define:

- permitted semantic blocks;
- required and repeatable blocks;
- inline annotation types;
- party variants and required fields;
- typed-value validators and normalisers;
- consistency requirements;
- intentionally incomplete fields;
- specialised block renderers;
- layout and artifact assertions.

The generic compiler should not contain Swiss notarial concepts. Those belong in the schema. The compiler contains only the generic semantic-block and annotated-span machinery.

Schemas must remain declarative where possible and should generate both compiler validation and LLM-facing documentation. There must not be a separate prompt schema that can drift from the actual compiler contract.

## 11. Interface for LLMs

Docc's command-line interface is an agent API. It should be predictable, composable, and capable of structured output.

### 11.1 Discover the contract

```bash
docc describe ch_deed --format json
docc example ch_deed
```

`describe` should report:

- available blocks;
- permitted attributes;
- required inline spans;
- validators and consistency rules;
- field requirements;
- concise syntax examples.

### 11.2 Validate the source

```bash
docc check urkunde.md --format json
```

### 11.3 Build artifacts

```bash
docc build urkunde.md \
  --docx urkunde.docx \
  --report build-report.json
```

DOCX is the supported artifact. The LibreOffice-based PDF conversion is a
deprecated fallback; no product behaviour builds on it.

### 11.4 Inspect an artifact

```bash
docc inspect urkunde.docx --format json
```

Commands should be orthogonal. `check` validates source, `build` compiles, and `inspect` examines a generated artifact. Avoid multiple commands that perform the same conceptual operation.

## 12. Diagnostics as a product surface

Diagnostics are the principal feedback channel to the LLM. They must be stable, precise, localised, and actionable.

```json
{
  "valid": false,
  "schema": "ch_deed",
  "diagnostics": [
    {
      "code": "DOC-PARTY-003",
      "severity": "error",
      "message": "Company party 'erwerberin' is missing required field '.uid'.",
      "file": "urkunde.md",
      "line": 17,
      "block": "erwerberin",
      "expected": {
        "field": "uid",
        "syntax": "[CHE-123.456.789]{.uid}"
      }
    }
  ]
}
```

A diagnostic should contain, where applicable:

- a stable error code;
- severity;
- source file and location;
- semantic block, reference, or key;
- actual value;
- expected constraint;
- related occurrences;
- one concise valid-syntax example.

Diagnostics should explain the violation without attempting substantive repair.

## 13. Verification layers

Docc validates three distinct kinds of correctness.

### 13.1 Data and semantic correctness

- required annotations exist;
- typed values parse;
- references resolve;
- repeated values agree after normalisation;
- blocks satisfy their schemas;
- intentional blanks satisfy their lifecycle rules.

### 13.2 Structural correctness

- required sections and blocks exist;
- cardinality and ordering constraints hold;
- every required party is referenced;
- required execution and certification regions exist;
- identifiers are unique where required.

### 13.3 Rendering correctness

- expected DOCX styles are used;
- special blocks have the intended layout;
- required images exist in named regions;
- protected blocks are not split improperly;
- page-count or geometry constraints hold;
- DOCX artifacts are readable and complete.

Exact pixel comparison should not be the primary verifier because it is brittle. Structural and geometric assertions should be primary, with rendered-page snapshots available for regression testing and visual inspection.

## 14. Product boundaries

Docc is not:

- a form generator;
- a general-purpose templating engine;
- a word processor;
- a legal drafting application;
- an autonomous document author;
- a broad document-understanding or ingestion system;
- an automatic prose-repair engine.

Docc is:

> A semantic Markdown compiler that gives an LLM a constrained, verifiable path to professional DOCX documents.

## 15. Guardrails against product bloat

Before introducing any feature or syntax, answer these questions:

1. Can ordinary Markdown express it adequately?
2. Can an annotated inline span express it?
3. Can the existing semantic block system express it?
4. Is it purely a schema or reference-DOCX concern?
5. Does a current mechanism already solve the same problem?
6. Does the feature provide deterministic validation or rendering value?
7. Can an LLM discover and use it from `docc describe` and diagnostics?

A new primitive should be rejected unless the existing mechanisms cannot express the requirement cleanly.

In particular:

- do not add a second templating syntax;
- do not mirror inline document data in large front matter;
- do not create bespoke parsers for individual block types;
- do not introduce both generated references and annotated literal references;
- do not put jurisdiction-specific legal concepts in the generic compiler;
- do not make Docc infer unmarked semantics from prose;
- do not silently fix conflicting substantive values.

## 16. Strategic implementation sequence

### Phase 1: General semantic infrastructure

- Ensure `:::beweis` uses a generic registered semantic-block implementation.
- Support schema-defined block kinds and attributes.
- Support typed annotated spans.
- Preserve source locations throughout the AST.
- Emit stable JSON diagnostics.

### Phase 2: Typed values and consistency

- Add validators and normalisers for money, dates, UIDs, and references.
- Group occurrences by semantic key.
- Report inconsistent values with every related location.
- Define exact versus normalised consistency policies.

### Phase 3: `ch_deed` schema (Öffentliche Urkunde)

- Add `partei` blocks with person and company variants.
- Add party roles and literal party references.
- Add intentionally incomplete fields.
- Add signature or execution blocks only where specialised rendering warrants them.
- Add authored-image validation and named placement.

### Phase 4: Artifact verification

- Inspect generated DOCX structure and styles.
- Check structural and geometric layout assertions.
- Produce a unified build report for the LLM.

### Phase 5: Agent ergonomics

- Implement `docc describe` from the actual schema contract.
- Provide compact canonical examples.
- Optimise diagnostics for iterative correction.
- Exercise the full write, check, build, inspect, and revise loop with LLM agents.

## 17. Strategic test

The product strategy succeeds if an LLM can:

1. discover a document schema;
2. write a readable Markdown document;
3. mark important semantics without hiding the source behind templates;
4. use specialised blocks consistently;
5. understand and correct Docc diagnostics;
6. generate deterministic professional DOCX output;
7. iterate until source and rendered artifact are valid;
8. leave a human reviewer with readable source and editable final documents.

The decisive product quality is not the number of supported document constructs. It is the reliability of the constrained authoring loop while preserving the LLM's control over the document.
