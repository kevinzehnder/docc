# Projects and configuration layout

Where docc looks for schemas and themes, how to start a project, and how to
hand one to an agent. For Git-managed packs see
[profile packs](profile-packs.md).

## Projects

`docc` is the engine. The schemas, themes and house style belong to a profile
pack, and a project selects one. Two shapes exist, both found by walking up
from the input file the way `git` finds `.git`:

A **bound project** carries a `.docc` directory recording which managed pack it
uses:

```
my-documents/
  .docc/
    profile.yaml         # pack id, source and ref
    profile.lock         # the exact installed commit
  docs/
    klage_mueller.md
```

A **pack checkout** is the pack itself — the manifest at the root names the
directories, so documents beside it compile without any `.docc`:

```
my-pack/
  docc-profile.yaml
  schemas/
    _base.yaml           # shared field shapes, extended by the rest
    ch_legal.yaml
  themes/
    legal.yaml           # page geometry, styles, and fixed furniture
  examples/
    letter.md
```

With neither in sight, docc resolves the starter pack embedded in the binary.
Override any of it with `--schema-dir` and `--theme-dir`.

The old layout — bare `.docc/schemas` and `.docc/themes` with no manifest — is
no longer resolved and fails with an error naming the fix: move the two
directories up beside a `docc-profile.yaml`, or bind a pack with
`docc profile use`.

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

`docc init [directory]` remains the offline option: it copies the embedded
starter pack out as an editable checkout — `docc-profile.yaml`, `schemas/`,
`themes/` and compiling examples in `examples/`. It never overwrites an
existing pack or example directory, and it creates nothing at all when it
refuses. `--dry-run` lists the files it would write and touches nothing. What
it writes is yours to edit — a starting point, not a managed install.

```bash
mkdir my-pack && cd my-pack
docc init --dry-run     # see what is coming
docc init
docc check examples/letter.md
docc build examples/letter.md
```

See [Profile packs](profile-packs.md) for the pack manifest, XDG storage,
resolution order and operational model.

The starter is deliberately generic. Replace the legal theme's `YOUR …`
letterhead values, then adapt its schemas and themes to the organisation's
actual conventions before using it for production documents.

This split means changing a letterhead is a file edit, not a compiler release,
and one engine serves projects whose document conventions have nothing in common.
