---
name: docc
description: Validate and render schema-backed Markdown to .docx with the bundled docc compiler. Build or check docc documents like legal briefs and letters. Bundles its binary and config; no install, no network.
---

# docc — schema-backed document compiler (bundled binary)

`docc` compiles a Markdown document with YAML frontmatter into a Word `.docx`.
The schema is the contract; the theme is the layout. This skill bundles the
compiler and a starter configuration, so everything runs locally inside the
Cowork VM with **no install and no network**.

All paths below are relative to **this skill's directory**. Run commands from
that directory (`cd` into it first), or prefix each path with the skill
directory's absolute path.

## First run — select and enable the binary

The skill ships a static x86_64 Linux binary (the Cowork VM is x86_64). Make it
executable (packaging may drop the exec bit) and confirm it runs:

```sh
DOCC=bin/docc-linux-amd64
chmod +x "$DOCC"
"./$DOCC" version      # confirms the VM will run the binary
```

If `version` prints (e.g. `docc 0.0.0-dev-cowork`), the compiler works here.

There is also a one-shot self-test that does all of the above plus a real
build; run it once to confirm the environment:

```sh
sh probe.sh
```

A `PROBE RESULT: PASS` line means the skill is fully operational in this VM.

## Configuration

The document types and layouts live in this skill under `config/`:

- `config/schemas/` — document types (`letter`, `legal`) and their field/body rules
- `config/themes/`  — page geometry, styles, letterhead furniture

Point `docc` at them with `--schema-dir config/schemas` and, for building,
`--theme-dir config/themes`. (These are passed explicitly because the config
directory is not the usual hidden `.docc/`.)

Inspect what is available:

```sh
"./$DOCC" types  --schema-dir config/schemas
"./$DOCC" themes --theme-dir  config/themes
```

The starter `letter` and `legal` types are Swiss-oriented and generic. Adapt
the schema/theme before using them as a real house style or court filing.

## Authoring workflow

1. Read the target schema in `config/schemas/` (and its `extends:` base) to learn
   the required frontmatter fields, the body structure, and which theme it uses.
2. Ask the user for any missing facts — names, addresses, dates, references,
   attachments. **Do not invent them.**
3. Write only the Markdown body and YAML frontmatter into a `.md` file in the
   workspace. Do not hand-write letterhead, address block, subject line, closing
   or footer that the theme renders automatically. Every document begins with
   `docc: 1` in its frontmatter.
4. Validate and fix every diagnostic. One run reports the complete list:

   ```sh
   "./$DOCC" check --json --schema-dir config/schemas document.md
   ```

5. Build only after it validates cleanly. Write the output into the workspace so
   the user receives it:

   ```sh
   "./$DOCC" build --schema-dir config/schemas --theme-dir config/themes \
       --output document.docx document.md
   ```

`docc build` re-validates before rendering. Never pass `--force` for a
deliverable. Exit code `0` is clean, `1` reports diagnostics or a build failure,
`2` is a usage/configuration error. Legal or contractual output is a **draft
that requires human review**.

## Diagnostics quick reference

- `DOC004` required field missing · `DOC006` wrong scalar type · `DOC007` bad date
- `DOC008` disallowed enum value · `DOC010` pattern failed · `DOC011` unknown key
- `DOC020`–`DOC022` body-structure problems

Explain any engine code: `"./$DOCC" explain DOC010`. Schema-defined codes carry
their own hints inside the schema file.

## Try it

A ready example is bundled:

```sh
"./$DOCC" build --schema-dir config/schemas --theme-dir config/themes \
    --output letter.docx examples/letter.md
```

This produces `letter.docx` in the current directory.
