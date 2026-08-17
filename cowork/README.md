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
  marketplace/              experimental plugin prototype (not a distribution channel)
```

The binary and the two packaged artifacts are **generated**, not committed.
Regenerate them any time:

```sh
sh cowork/assemble.sh
```

## Distribution

The supported distribution artifact is the generated standalone skill ZIP:

```sh
sh cowork/assemble.sh
```

Deploy `cowork/docc-cowork-skill.zip` through the target host's skill-upload
workflow. Build and deploy a new ZIP for every release.

The `marketplace/` directory is an **experimental plugin prototype**. Plugin
support is still beta and its generated payload is intentionally not committed,
so a clone of this repository is not an installable marketplace. It is retained
for integration work only; do not use it as a production distribution path.

## Test in Cowork

1. `sh cowork/assemble.sh` to build the zip.
2. Claude Desktop → **Customize** → add the skill (upload the zip).
3. New Cowork session (skills sync at session start).
4. Prompt: *"Run the docc skill's probe.sh and report the PROBE RESULT."*
   `PROBE RESULT: PASS` confirms the VM runs the binary on x86_64.
5. Then: *"Build examples/letter.md and give me the .docx"* — confirms delivery.

## Configuration note

`config/` holds the generic `docc init` starter (letter + legal), so a clone of
this repository builds a shareable artifact. The skill points `docc` at it with
`--schema-dir config/schemas` and `--theme-dir config/themes`.

To ship an organisation's real document types, point `assemble.sh` at a profile
pack — a checkout with `schemas/` and `themes/` — instead of editing `config/`:

```sh
DOCC_PROFILE=~/git/kanzlei-profiles sh cowork/assemble.sh
```

That regenerates everything from the pack and writes it to the git-ignored
`build/`, leaving the tracked generic skill untouched. A firm's letterhead
belongs in its own repository, never in this one; see
[Building profiles](../docs/building-profiles.md) for how to make that pack.
