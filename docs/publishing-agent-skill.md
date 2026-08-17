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
`PATH`, and reports a missing runtime rather than downloading one. For a host
with neither Go nor an installed `docc`, use an archive instead.

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

Push a `v*` tag to test, create a GitHub release, and upload all archives.
Manual workflow runs create downloadable CI artifacts without publishing.
Test representative prompts in clean Claude/Cowork and ChatGPT Work sessions
before completing each vendor's listing and review process.

Once a release exists, the same marketplace can serve the self-contained build
to machines without Go by adding a second entry with an
[`archive` source](https://code.claude.com/docs/en/plugin-marketplaces#zip-archives)
pointing at `docc-claude-plugin-<version>.zip` and its `sha256`. That zip
already has the layout an archive source expects: `.claude-plugin/plugin.json`
at the top, `skills/docc/` beside it. Requires Claude Code v2.1.224 or later,
and both the digest and the version have to move with every new zip or clients
keep the cached copy.
