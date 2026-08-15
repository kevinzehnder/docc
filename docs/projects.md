# Projects and configuration layout

Where docc looks for schemas and themes, how to start a project, and how to
hand one to an agent. For Git-managed packs see
[profile packs](profile-packs.md).

## Projects

`docc` is the engine. The schemas, themes and house style belong to the
project being compiled, in a `.docc` directory that `docc` finds by walking up
from the input file the way `git` finds `.git`:

```
myproject/
  .docc/
    schemas/
      _base.yaml        # shared field shapes, extended by the rest
      ch_legal.yaml
      ch_letter.yaml
    themes/
      legal.yaml         # page geometry, styles, and fixed furniture
  docs/
    klage_mueller.md
```

Override the location with `--schema-dir`.

### Starting a new project

For an organisation-managed setup, select a Git-backed profile pack instead of
copying schemas and themes into every project. `docc profile use` installs the
selected immutable revision under the user's XDG data directory and creates a
small `.docc/profile.yaml` plus `.docc/profile.lock` in the project. Commit
those two files with the documents: they record the source/ref and exact commit
that produced a document. Builds never fetch or update a profile automatically.

```bash
mkdir my-documents && cd my-documents
docc profile use ssh://git.example.ch/kanzlei/docc-profiles.git --ref 2026.1
docc doctor
docc check brief.md
docc build brief.md
```

`docc profile install --default <repository>` selects an installed revision for
new directories that have no project binding. A project binding always wins.
Use `docc profile update --project .` to deliberately resolve and lock a newer
revision.

`docc init [directory]` remains the offline option for a standalone generic
starter. It creates `.docc/` with a generic letter and a Swiss-legal
starter schema/theme, plus compiling examples in `examples/docc/`. It never
overwrites an existing starter configuration, example directory, or installed
skill, and it creates nothing at all when it refuses. `--dry-run` lists the
files it would write and touches nothing. What it installs is yours to edit — a
starting point, not a managed install.

```bash
mkdir my-documents && cd my-documents
docc init --dry-run     # see what is coming
docc init
docc check examples/docc/letter.md
docc build examples/docc/letter.md
```

See [Profile packs](profile-packs.md) for the pack manifest, XDG storage,
resolution order and operational model.

The starter is deliberately generic. Replace the legal theme's `YOUR …`
letterhead values, then adapt its schemas and themes to the organisation's
actual conventions before using it for production documents.

### Agent skill

No LLM is required to use `docc`. For agents, this repository ships a portable
[Agent Skill](../skills/docc/SKILL.md) that describes the validation-and-build
workflow. `docc init` also installs it at `.agents/skills/docc/SKILL.md`, which
Pi discovers in a trusted project. Other harnesses can copy that directory or
load the skill file directly; the project's `.docc` configuration remains the
authoritative contract.

This split means changing a letterhead is a file edit, not a compiler release,
and one engine serves projects whose document conventions have nothing in common.
