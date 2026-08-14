# docc marketplace (plugin distribution)

A Claude plugin marketplace — a git repo with `marketplace.json` at its root —
that ships the `docc` skill as a **versioned plugin**. This is the update path
that does not require re-uploading a zip for every change: bump the version,
push, and installers pull the new version.

## Layout

```
marketplace/
├── marketplace.json                     # lists the plugins + their sources
└── plugins/
    └── docc/
        ├── .claude-plugin/plugin.json   # plugin manifest (name, version, …)
        └── skills/
            └── docc/                     # the skill payload (SKILL.md, bin/, config/, …)
```

The `skills/docc/` payload is **generated** from the canonical skill at
`cowork/docc/` by `cowork/assemble.sh`. Edit the skill there, then
re-run assemble — never hand-edit the copy under `marketplace/`.

## Versioning workflow (the answer to "do I re-upload every time?")

- **Standalone skill** (`Customize > Skills`, the zip): yes, manual re-upload each version.
- **This plugin:** no. To ship a new version:
  1. Edit the skill in `cowork/docc/` (or bump the docc binary).
  2. Bump `version` in **both** `plugins/docc/.claude-plugin/plugin.json` and
     the matching entry in `marketplace.json` (semver).
  3. `sh cowork/assemble.sh` to regenerate the payload.
  4. Commit + push this marketplace repo.
  5. Installed users get the new version when they update the plugin. (Cowork
     plugin support is beta and currently per-machine; org-wide push is coming.)

## Install (Claude Code / Cowork)

Point Claude at this marketplace repo, then install the `docc` plugin and enable
it. In Claude Code that is the `/plugin` marketplace flow; in Cowork, add it from
**Customize**. `defaultEnabled` is `false`, so it is off until enabled.

## Binary delivery — one decision before publishing to a shared git repo

The skill carries an 11 MB `docc` binary. With `source.type: local` (current),
that binary must live in the repo, so publishing to a shared/public git repo
means either:

- **Commit the binary** (simplest; consider Git LFS to keep history lean), or
- **Switch the source** in `marketplace.json` to a hosted archive and let Claude
  fetch it at install time — no binary in git:

  ```json
  "source": { "type": "archive",
              "url": "https://.../docc-cowork-skill-<version>.zip",
              "sha256": "<hash>" }
  ```

  (`github` and `git-subdir` source types are also available.)

For a private firm marketplace, committing the binary is fine. For anything
public, prefer the hosted `archive` source with a pinned `sha256`.
