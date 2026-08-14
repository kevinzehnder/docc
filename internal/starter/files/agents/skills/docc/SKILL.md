---
name: docc
description: Write, validate, or render structured Markdown documents with docc. Use when a project contains .docc schemas/themes, when fixing docc diagnostics (DOC0xx or schema-defined codes), or when producing .docx/.pdf from a schema-backed document.
---

# docc

`docc` compiles a Markdown document with YAML frontmatter. The schema is the
contract; the theme is the layout. Always treat the project's `.docc/` files as
authoritative: they define the available document types, required facts, body
structure, and output styling.

## Project setup

A project needs this layout:

```text
project/
  .docc/
    schemas/
    themes/
  documents/
```

For a new project, create the generic starter set once:

```sh
docc init
```

It installs `.docc/`, `examples/docc/`, and this skill under
`.agents/skills/docc/`. The starter includes Swiss-oriented `letter` and
`legal` types; adapt them before using them as a house style or court filing.

`docc` finds `.docc` by walking up from the input document. For a document
outside that tree, pass `--schema-dir` and, when building, `--theme-dir`.

## Commands

```sh
docc types                         # inspect document types
docc themes                        # inspect themes
docc check document.md             # validate
docc check --json document.md      # use in an agent correction loop
docc check --strict document.md    # warnings are errors
docc build document.md             # validate, then emit document.docx
docc build --output out.docx document.md
docc explain DOC010                # explain an engine diagnostic
```

`docc build` validates before rendering. Do not use `--force` for a
deliverable. Exit code `0` is clean, `1` reports diagnostics or a build
failure, and `2` is a usage/configuration error.

DOCX is the supported compiler output. When a user requests PDF, build the
DOCX first, then use the host's document/PDF capability. `docc build --to pdf`
is compatibility-only and requires `soffice`.

## Agent workflow

1. Discover the project configuration with `docc types` and `docc themes`.
2. Read the selected schema, its `extends:` ancestors, and its selected theme.
   The schema declares the facts and structure; the theme may add fixed
   furniture such as a letterhead, address block, subject line, closing, or
   footer.
3. Ask for missing factual information. Do not invent names, addresses, dates,
   references, legal claims, or attachments.
4. Write only the Markdown body and YAML frontmatter. Do not duplicate
   furniture the theme renders automatically.
5. Run `docc check --json` and correct every error. One run collects all
   diagnostics, so fix the complete list before rechecking.
6. Build only after the document validates. Legal output remains a draft that
   requires appropriate human review.

## Frontmatter and diagnostics

- Every document starts with `docc: 1` in the frontmatter — the marker that
  makes it a docc document. Files without it (plain markdown, unrelated YAML
  frontmatter) are not checked.
- `document_type` selects the schema unless `--type` overrides it.
- Quote values whose leading zeros matter, especially postal codes:
  `postal_code: "3000"`.
- Dates use ISO form: `2026-08-04`.
- A field marked `required` and `nullable` must be present; use `~` only when
  the schema says an explicit absent value is valid.
- `DOC004` means a required field is missing; `DOC006` a scalar has the wrong
  type; `DOC007` a date is invalid; `DOC008` an enum value is disallowed;
  `DOC010` a pattern failed; `DOC011` an unknown frontmatter key; and
  `DOC020`–`DOC022` concern body structure. Use `docc explain <code>` for
  engine diagnostics. Schema-defined codes and their hints are in the schema.

## Schema and theme changes

Schemas and themes are project configuration, not values to guess around. When
a requested document cannot fit the current schema, explain the mismatch and
ask whether the project owner wants to change the schema/theme. Do not weaken
required checks or remove rules merely to make a draft pass.
