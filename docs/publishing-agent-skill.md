# Publishing the docc Agent Skill

Keep one portable skill in `skills/docc`. Every distribution path below carries
that same directory; none of them forks the instructions.

## Installing from this repository as a marketplace

The repository is its own plugin marketplace, so a Claude Code user needs no
archive and no release:

```sh
/plugin marketplace add https://github.com/kevinzehnder/docc.git
/plugin install docc@docc
```

**Give the full HTTPS URL, not the `kevinzehnder/docc` shorthand.** This
repository is private, and the shorthand clones over SSH: that works in a
terminal with a loaded key, and fails in the desktop app, which has no agent to
reach and suppresses the interactive prompt — so the dialog reports only that the
marketplace could not be added. The `.git` suffix matters too; without it the URL
is read as a link to a hosted `marketplace.json`, and relative plugin sources
like this one's `"./"` cannot resolve. `CLAUDE_CODE_PLUGIN_PREFER_HTTPS=1` makes
the shorthand clone over HTTPS if you prefer it.

Third-party marketplaces have auto-update off by default — turn it on in
`/plugin` → **Marketplaces**, or the dialog's sync toggle. Private repository
plus HTTPS costs something there: the background refresh disables git credential
helpers, so its `git pull` cannot authenticate and falls back to re-cloning the
marketplace, which can time out as the repository grows. Two things make it
predictable:

```sh
gh auth setup-git                                        # the re-clone can authenticate
export CLAUDE_CODE_PLUGIN_KEEP_MARKETPLACE_ON_FAILURE=1  # keep the clone instead of deleting it
```

An SSH remote avoids this entirely — a key in `ssh-agent` authenticates
background pulls the same as foreground ones. It is the better remote for a
private marketplace wherever the agent is actually reachable.

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
neither Go nor an installed `docc` needs a packaged archive instead.

## Why the marketplace has no bundled entry

An [`archive`](https://code.claude.com/docs/en/plugin-marketplaces#zip-archives)
entry pointing at `releases/latest/download/docc-claude-plugin.zip` would install
the compiler along with the skill, needing no Go toolchain. It was tried, and it
cannot work while this repository is private: an archive source downloads over
plain HTTPS with no credentials, and GitHub answers an unauthenticated request
for a private repository's release asset with **404**, not 401. v0.5.0 attached
that asset and the install still failed on 404 — the asset existed, the
visibility was the cause.

Restoring the entry therefore takes three things together: the repository turned
public, the entry back in `.claude-plugin/marketplace.json`, and the unversioned
copy back in `scripts/release-artifacts.sh` so the stable URL resolves. Leave a
version and digest off the entry and the downloaded file's own hash is the
version, so publishing a release is the update with nothing to rewrite here;
pinning `sha256` buys an integrity check at the price of a commit per release.

Two costs come with such an entry, and the second falls on the git entry above as
well: nothing verifies the download beyond HTTPS, and archive sources need Claude
Code v2.1.224 or later — on v2.1.120 through v2.1.223 that entry fails to
install, and before v2.1.120 the whole marketplace fails to load. Without it, the
marketplace loads on any version.

Until then, a machine with no Go toolchain takes a packaged archive by hand.

## Releasing

`goreleaser release` on a `v*` tag is the only job that creates the GitHub
release. It builds all six targets, and its before-hook runs
`scripts/release-artifacts.sh`, which produces the agent-skill archives and the
Cowork skill into `build/release/` for `release.extra_files` to attach.
`release-agent-skills.yml` verifies the same archives on pull requests but no
longer publishes: when both workflows created the release on one tag, whichever
lost the race took the other's assets down.

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

`docc-claude-plugin-<version>.zip` already has the layout an archive source
expects — `.claude-plugin/plugin.json` at the top, `skills/docc/` beside it — so
it is the file a bundled marketplace entry would install if this repository ever
turns public.

A manual `release-agent-skills.yml` run produces the archives as downloadable CI
artifacts without publishing. Test representative prompts in clean Claude/Cowork
and ChatGPT Work sessions before completing each vendor's listing and review
process.
