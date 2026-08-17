---
name: docc
description: Validate and render schema-backed Markdown to .docx with the docc document profile. Document types: ch_legal, ch_letter. Bundles its compiler, types and layouts; no install, no network.
---

# docc — schema-backed document compiler

`docc` compiles Markdown with YAML frontmatter into a Word `.docx`.
The schema is the contract; the theme is the layout. This skill carries the
document types and layouts, so the output matches the house style exactly.

All paths below are relative to **this skill's directory**. Run commands from
that directory, or prefix each path with the skill directory's absolute path.

## First run — enable the bundled binary

The compiler ships inside this skill; there is no install and no network.
Packaging may drop the exec bit, so restore it once:

```sh
DOCC=bin/docc-linux-amd64
chmod +x "$DOCC"
"./$DOCC" version
```

A version line means the compiler runs here. `sh probe.sh` does the same
check plus a real build, and prints `PROBE RESULT: PASS` when the skill is
fully operational.

## Point docc at this profile

This skill is a profile pack: its `docc-profile.yaml` names the schemas and themes below.
Set `DOCC_PROFILE` to **this skill's absolute directory** and every command finds them,
whatever directory the document lives in:

```sh
export DOCC_PROFILE="$(pwd)"    # run this from the skill directory
```

Each shell command may be run in a fresh shell, so if the export does not
survive, prefix each command instead: `DOCC_PROFILE=/abs/path/to/skill "./$DOCC" check doc.md`.
The equivalent explicit form is `--schema-dir config/schemas --theme-dir
config/themes`, which still works and needs no environment at all.

## Document types

The types below are the whole contract. There is no generic mode: a document
declares one of these in its frontmatter as `document_type`.

| Type | Purpose |
|---|---|
| `ch_legal` | Formal legal brief (Klageschrift, Klageantwort, Rechtsschrift). |
| `ch_letter` | Business or legal correspondence on letterhead. |

The schemas and themes live under `config/`:

- `config/schemas/` — the document types, their fields and body rules
- `config/themes/` — page geometry, styles and letterhead furniture

```sh
"./$DOCC" types
"./$DOCC" themes
"./$DOCC" describe <type>
"./$DOCC" example <type>
"./$DOCC" doctor    # confirm the profile resolved to this skill
```

`describe` prints the resolved contract for a type — every required field,
the body structure and the blocks it permits. Read it before writing a
document of a type for the first time.

## Authoring workflow

1. Run `describe` for the type to learn its required frontmatter, its body
   structure, and the semantic blocks and spans it permits.
2. Ask the user for any missing facts — names, addresses, dates, references,
   amounts. **Do not invent them.** A plausible invented reference number is
   worse than an empty one, because nobody notices it.
3. Write only the Markdown body and YAML frontmatter. Do not hand-write
   letterhead, address block, subject line, signature or footer: the theme
   renders those, and a hand-written copy will differ from it.
4. Validate and fix every diagnostic. One run reports the complete list:

   ```sh
   "./$DOCC" check --json document.md
   ```

5. Build only after it validates cleanly:

   ```sh
   "./$DOCC" build --output document.docx document.md
   ```

`build` re-validates before rendering. Never pass `--force` for a deliverable.
`--strict` turns warnings into errors. Legal and contractual output is a
**draft that requires human review**.

Exit code `0` is clean, `1` reports diagnostics or a build failure, `2` means
the command line is wrong, and `3` that the schemas or themes could not be
resolved at all — `3` is a setup problem, never a problem with the document.
Read `--json` rather than scraping stderr. It has two shapes: a validation
result carries `ok`, `errors`, `warnings` and `diagnostics`, while a command
that could not run carries `ok: false`, an `error` message and a `kind` of
`usage`, `config` or `error`.

## Frontmatter

A file becomes a docc document by declaring the format marker. Without it
nothing is validated and `check` reports `DOC024`, which is the answer to
"why did my document pass with no output".

```yaml
---
docc: 1
document_type: <one of the types above>
---
```

- Dates are ISO: `2026-08-04`.
- Quote a value whose leading zeros carry meaning, or YAML discards them:
  `postal_code: "3000"`.
- A field that is required *and* nullable must still be present. Write `~`
  only where the schema says an explicitly absent value is valid.

## Diagnostics

- `DOC004` required field missing · `DOC006` wrong scalar type · `DOC007` bad date
- `DOC008` disallowed enum value · `DOC010` pattern failed · `DOC011` unknown key
- `DOC020`–`DOC022` body-structure problems

Explain any engine code with `"./$DOCC" explain DOC010`. Codes a schema defines
carry their own hint inside the schema file.

## PDF

`.docx` is the compiler's supported output. When the user asks for a PDF,
build the `.docx` first and then use whatever document conversion this host
offers. `--to pdf` exists for compatibility and needs `soffice` installed,
which most hosts do not have.

## When the request does not fit a type

The schemas and themes here are configuration, not values to work around. If
a requested document cannot be expressed by any type above, say what does not
fit and ask whether the owner of this profile wants to change it. Do not
weaken a required field, drop a rule, or reach for `--force` to make a draft
validate: a document that passes because the contract was lowered is worse
than one that visibly fails.

## Try it

A complete example ships for each type:

```sh
"./$DOCC" build --output out.docx examples/ch_legal.md
```

## Cowork host notes

The VM is x86_64, which is the architecture the bundled binary is built for.
`probe.sh` checks it and reports the architecture it found.

The conversion the PDF section above refers to is Cowork's own document
capability; `soffice` is not installed in this VM, so `--to pdf` cannot work
here at all.

Write deliverables into the workspace so the user receives them.
