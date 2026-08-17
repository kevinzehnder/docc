# Publishing the docc Agent Skill

Keep one portable skill in `skills/docc`. Every distribution path below carries
that same directory; none of them forks the instructions.

## Installing from this repository as a marketplace

The repository is its own plugin marketplace, so a Claude Code user needs no
archive and no release:

```sh
/plugin marketplace add kevinzehnder/docc
/plugin install docc@docc
```

The desktop app's **Add marketplace** dialog takes the same `owner/repo`.
Third-party marketplaces have auto-update off by default — turn it on in
`/plugin` → **Marketplaces**, or the dialog's sync toggle.

Three details in `.claude-plugin/marketplace.json` make this work:

- `"source": "./"` — the repository root is the plugin as well as the
  marketplace. The whole checkout becomes the plugin payload.
- `"skills": ["./skills/docc"]` — with a marketplace-root source the listed
  paths are the complete set, so the entry loads that one skill and nothing else
  in the tree is scanned.
- **No `version` in `.claude-plugin/plugin.json`.** A declared version pins the
  plugin and users then only see an update when the string changes. Without one,
  the commit SHA is the version: the install caches under
  `~/.claude/plugins/cache/docc/docc/<sha>` and every push is an update.
  `claude plugin validate .` warns about the missing version — that warning is
  the design, not a defect, and `scripts/test-agent-packages.sh` fails if either
  committed manifest grows a `version` key.

This path carries **no binary**. `docc` has to be on `PATH`:

```sh
go install github.com/kevinzehnder/docc/cmd/docc@latest   # or: task install
```

`skills/docc/scripts/run-docc.sh` prefers a bundled binary, then falls back to
`PATH`, and reports a missing runtime rather than downloading one. A host with
neither Go nor an installed `docc` wants the `docc-bundled` entry below.

## Installing the bundled build from a release

The same marketplace carries a second entry for a machine with no Go toolchain:

```sh
/plugin install docc-bundled@docc
```

Its source is an
[`archive`](https://code.claude.com/docs/en/plugin-marketplaces#zip-archives)
pointing at `releases/latest/download/docc-claude-plugin.zip` — a *stable* URL,
which is why `scripts/release-artifacts.sh` attaches the plugin zip under a bare
name as well as a versioned one. The entry pins neither a `version` nor a
`sha256`, so the downloaded file's own digest is the version: publish a release
and every installation sees an update, with nothing to rewrite here. Pinning the
digest instead would buy an integrity check at the price of a commit to this file
per release.

Two costs come with the entry, and both fall on the git entry above as well:

- Archive sources need Claude Code v2.1.224 or later. On v2.1.120 through
  v2.1.223 this entry fails to install; before v2.1.120 the whole marketplace
  fails to load.
- Nothing verifies the download beyond HTTPS.

## Releasing

`goreleaser release` on a `v*` tag is the only job that creates the GitHub
release. It builds all six targets, and its before-hook runs
`scripts/release-artifacts.sh`, which produces the agent-skill archives, the
Cowork skill and the unversioned plugin zip into `build/release/` for
`release.extra_files` to attach. `release-agent-skills.yml` verifies the same
archives on pull requests but no longer publishes: when both workflows created
the release on one tag, whichever lost the race took the other's assets down.

Check a config change without publishing anything:

```sh
goreleaser check
goreleaser release --snapshot --clean
```

A released `docc version` now prints `1.2.3` rather than `v1.2.3`. goreleaser's
`.Version` drops the leading `v`, which is what `package-agent-skills.sh` has
always stamped into the bundled binaries, so the two paths finally agree.

## Packaged archives

```sh
./scripts/test-agent-packages.sh
./scripts/package-agent-skills.sh 1.0.0
```

Outputs:

- `docc-agent-skill-<version>.zip` for Claude custom Skills and Cowork.
- `docc-claude-plugin-<version>.zip` for a Claude plugin marketplace.
- `docc-openai-plugin-<version>.zip` for OpenAI skills-only submission.

The archives contain static Linux AMD64 and ARM64 binaries, so cloud agents need
no network or Go installation. Local agents on other platforms fall back to a
`docc` executable on `PATH`. Packaging injects the version into the copied
manifests, which is why the committed ones carry none.

`docc-claude-plugin-<version>.zip` is also what the `docc-bundled` entry
installs: `.claude-plugin/plugin.json` at the top of the archive, `skills/docc/`
beside it, which is the layout an archive source expects.

A manual `release-agent-skills.yml` run produces the archives as downloadable CI
artifacts without publishing. Test representative prompts in clean Claude/Cowork
and ChatGPT Work sessions before completing each vendor's listing and review
process.
