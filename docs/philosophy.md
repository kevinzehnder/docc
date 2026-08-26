# Philosophy

What docc is, what it refuses to be, and where it is going. This is the
document to read before proposing a feature, and the one that decides whether
the proposal belongs here at all.

## Identity

docc is a **deterministic compiler for structured Markdown documents**. It
parses Markdown and YAML frontmatter, validates both against a schema, and
renders the result to `.docx` through a theme. That is the whole product.

Three commitments follow from it:

- **One thing, done well.** docc compiles authored Markdown. It does not
  ingest, store, template, serve, or converse. Anything upstream of an authored
  `.md` file or downstream of a built `.docx` is another tool's job, except for
  comparing an edited copy's visible text with what docc renders. That narrow
  inspection reports changes; it does not reconstruct or modify Markdown.
- **Validation gates rendering.** A document that does not satisfy its schema
  does not build (short of `--force`). The value is not the `.docx` — Word can
  make one of those — it is the guarantee that what built was checked.
- **Determinism.** The same input, schemas and themes produce byte-identical
  output, on any machine, with no clock and no network. Provenance is stamped
  into the document; nothing else about the environment may leak in.

## The content model is closed

A schema declares document behavior through exactly **three content verbs**:

| Verb | Syntax | What it claims |
|---|---|---|
| `blocks:` | `::: name {#id key=val}` | a structural region with its own style and rules |
| `spans:` | `[text]{.type}` | an inline fact, typed so checks can read it |
| `fields:` | `[____]{.docc-field key=name}` | a blank that is content — completed later, by hand |

Beside them: **six frontmatter field types** (`any`, `string`, `int`, `bool`,
`date`, `enum`, plus `list<T>` and named compound types) and **nine named rule
checks** (see the README table). This surface is deliberately closed. New
document behavior belongs in schemas and themes, expressed through these
verbs — not in new verbs, and not in Go. A proposal that needs a tenth check
must first show that none of the nine, scoped by `args:`, can express it.

## The pack ecosystem

Schemas and themes travel in **profile packs**: Git repositories with a
`docc-profile.yaml` manifest naming a `schemas/` and a `themes/` directory.
The engine and the document conventions are versioned separately — docc
releases do not change anyone's letterhead, and a firm's pack pins the docc
version it verified against.

- A **starter pack is embedded in the binary** and resolved as the `builtin`
  source whenever nothing else is configured: docc works out of the box, and
  the embedded pack goes through the same manifest, loader and validation as
  any installed one.
- `docc init` copies that pack out as an **editable checkout** — a real pack,
  resolved through its own manifest, yours to adapt.
- Real conventions live in their own repositories and are installed with
  `docc profile use`, pinned by commit in the consuming project.

Resolution order: `DOCC_PROFILE` env → project binding (`.docc/profile.yaml`)
→ pack checkout (nearest `docc-profile.yaml`) → user default → builtin.

## Non-goals

Do not add:

- **AgentSkill or agent-host packaging.** A skill's value is the drafting
  knowledge of the pack that ships it, so skills are built and released by the
  pack repositories that own that knowledge. docc is a compiler an agent may
  invoke; it does not package itself for agents.
- OCR, general document ingestion or DOCX-to-Markdown conversion — the narrow
  `docc diff` text comparison is not an importer;
- LLM or VLM API clients — agents author Markdown *around* docc, the compiler
  stays model-independent;
- document storage, databases, collaboration or review workflows;
- a web server or hosted service;
- a template programming language, or conditional theme logic where a second
  theme or an explicit field suffices;
- automatic invention of missing legal or factual content;
- multiple PDF engines — the CLI's LibreOffice path is compatibility-only.

## Way forward

Held deliberately small. In rough order:

- **Validate real themes against approved stationery** before production use:
  compare built documents to approved references and print-test envelope
  layouts. Never with real client material in this repository's fixtures.
- **OOXML conformance in CI**: package invariants in `internal/docx`, then
  ECMA-376 XSD validation of emitted parts.
- **Make `docc init` transactional**, so an I/O failure cannot leave a partial
  checkout.
- **Negative tests** for project discovery and schema loading.
- **Decide the CLI PDF exporter's future** — retire it in a breaking release
  once hosts cover the conversion, or keep it as compatibility.
- A formatter or an MCP wrapper **only when evidence demands it**; the JSON
  command contract is the integration surface, and editor work belongs in the
  LSP rather than in weakened schemas.
