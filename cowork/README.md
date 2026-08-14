# docc for Claude Cowork

A self-contained Claude Cowork **skill** that carries the `docc` compiler and a
starter `.docc` configuration, so lawyers can author schema-backed Markdown in a
Cowork session and download the rendered `.docx` — no terminal, no install, no
network. Confirmed working end-to-end in a real Cowork VM (Linux x86_64): the VM
executes the bundled binary and delivers the built document.

## Layout

```
cowork/
  docc/                     canonical skill source (single source of truth)
    SKILL.md                activation + authoring workflow
    probe.sh                one-shot self-test (arch → exec → build)
    config/schemas,themes   starter letter + legal document types
    examples/               buildable sample documents
    bin/                    docc-linux-amd64  (generated, git-ignored)
  assemble.sh               build binary + emit both artifacts below
  docc-cowork-skill.zip     standalone skill (generated, git-ignored)
  marketplace/              versioned plugin distribution (see its README)
```

The binary and the two packaged artifacts are **generated**, not committed.
Regenerate them any time:

```sh
sh cowork/assemble.sh
```

## Two ways to ship it

- **Standalone skill** — upload `docc-cowork-skill.zip` in Claude Desktop →
  **Customize > Skills**. Simple, but every new version is a manual re-upload.
- **Plugin** — a versioned package installed from the `marketplace/` git repo;
  bump the version and push, and installers pull the update. See
  [`marketplace/README.md`](marketplace/README.md).

## Test in Cowork

1. `sh cowork/assemble.sh` to build the zip.
2. Claude Desktop → **Customize** → add the skill (upload the zip).
3. New Cowork session (skills sync at session start).
4. Prompt: *"Run the docc skill's probe.sh and report the PROBE RESULT."*
   `PROBE RESULT: PASS` confirms the VM runs the binary on x86_64.
5. Then: *"Build examples/letter.md and give me the .docx"* — confirms delivery.

## Configuration note

`config/` currently holds the generic `docc init` starter (letter + legal).
Swap it for a firm's real `.docc` schemas + themes to produce that firm's
documents. The skill points `docc` at it with `--schema-dir config/schemas`
and `--theme-dir config/themes`.
