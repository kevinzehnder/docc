# Profile packs

`docc` is the compiler. A **profile pack** is the version-controlled collection
of schemas, themes and theme assets that defines an organisation's document
conventions. It is deliberately distinct from a single schema: one pack can
ship many document types.

The normal workflow is to select a managed profile pack, not to copy a generic
`.docc` directory into every document repository:

```sh
docc profile use ssh://git.example.ch/kanzlei/docc-profiles.git --project . --ref 2026.1
docc build docs/brief.md
```

`docc init` remains available for an offline, standalone configuration that a
user intends to edit locally. It is not the recommended way to consume an
organisation's approved conventions.

## Pack layout

A profile pack is a Git repository with this layout:

```text
docc-profiles/
  docc-profile.yaml
  schemas/
    ch_letter.yaml
    ch_legal.yaml
  themes/
    letter.yaml
    legal.yaml
    logo.png
  docs/
    skill-notes.md
  examples/
```

The required manifest makes the repository unambiguous:

```yaml
format: 1
id: example-kanzlei
name: Example Kanzlei profiles
schemas: schemas
themes: themes

# Optional: what an AgentSkill packaged from this pack should say.
skill:
  description: >-
    Draft and render Example Kanzlei deeds, letters and briefs.
    Use for any client-facing document of the firm.
  notes: docs/skill-notes.md
```

`id` is a stable, filesystem-safe identifier. `schemas` and `themes` are
relative paths inside the repository.

### `skill:` — the firm's own drafting guidance

Everything an AgentSkill can derive from the schemas is generated, and stays
generated: a hand-written list of document types drifts from the types the pack
declares, and nobody notices until someone uses it. What cannot be derived is
the firm's knowledge — when to ask for a Heimatort rather than infer it, which
type fits which instruction, what has to be verified before signing.

`skill.notes` is a Markdown file inside the pack, appended to the generated
instructions. `skill.description` replaces the generated one-line description,
which is what an agent reads when deciding whether this skill applies at all.
Both are validated at load: a pack that promises notes it does not carry fails
where it is validated, not in the one command that reads the file.

Without them the prose has to be handed to `docc profile package --notes` by
whoever runs it, so the one thing only the firm knows lives outside the
repository that knows it. Compatibility is verified by the profile
repository's CI against supported `docc` releases; a machine-enforced range can
be added once the public release versioning policy is in place. A pack contains
data and assets only:
`docc` never executes a script supplied by a profile repository.

## Local state and XDG directories

On Linux, docc follows the XDG Base Directory Specification:

| Content | Directory |
| --- | --- |
| User configuration and default profile | `$XDG_CONFIG_HOME/docc` (default `~/.config/docc`) |
| Immutable checked-out profile revisions | `$XDG_DATA_HOME/docc/profiles` (default `~/.local/share/docc/profiles`) |
| Reserved for disposable status/fetch data | `$XDG_CACHE_HOME/docc` (default `~/.cache/docc`) |

A Git checkout is user data, not configuration, so it never belongs below
`~/.config`.

## Project binding and reproducibility

A project that uses a pack records the source and requested Git ref in
`.docc/profile.yaml`, and the resolved immutable commit in
`.docc/profile.lock`. Both files belong in Git with the document sources.

```yaml
# .docc/profile.yaml
format: 1
id: example-kanzlei
source: ssh://git.example.ch/kanzlei/docc-profiles.git
ref: 2026.1
```

```yaml
# .docc/profile.lock
format: 1
commit: 0123456789abcdef0123456789abcdef01234567
```

A build reads the locked local revision. It neither contacts the network nor
silently follows a branch, so it remains reproducible and works offline. An
explicit update resolves a newer commit and changes the committed lockfile for
review.

A user may also configure one default pack. That makes a new directory usable
without `docc init`; a project binding still wins and should be used for shared
or filed work.

## Resolution order

Schema and theme resolution has one shared precedence order:

1. Explicit `--schema-dir` and `--theme-dir` flags.
2. `DOCC_PROFILE`, naming a pack directory. It is for the case neither walking
   up nor the working directory can answer: a packaged AgentSkill carries its
   profile beside itself and compiles documents that live wherever the agent
   happens to be. A value that names no usable pack is an error, never a
   fallback — compiling against schemas nobody chose is the failure this whole
   order exists to prevent.
3. A nearest project `.docc/profile.yaml` plus its lockfile.
4. Existing local `.docc/schemas` and `.docc/themes` (legacy and custom
   projects).
5. A nearest `docc-profile.yaml` — you are standing inside a pack checkout.
6. The user's configured default pack.
7. A configuration error with an installation hint.

Step 5 is what makes a pack repository usable from inside itself. A pack has no
`.docc`: its schemas and themes are the product, not one project's local
configuration, so working in a checkout used to mean passing `--schema-dir` and
`--theme-dir` to every command. The manifest already names both directories, so
nothing extra is written down. It sits below the project forms — a binding is an
explicit statement about which profile applies, and a pack that happens to sit
above it does not override that — and above the user default, because the pack
you are editing is a more specific answer than the one you installed globally.

Nothing is pinned by a checkout: you are working *on* the pack rather than
consuming a revision of it, so a document built this way records no commit.
`docc doctor` names which of these answered, as `env-profile`, `pack-checkout`,
`project-profile`, `legacy-project` or `user-default`.

Installed packs are never merged. Selecting one pack avoids collisions between
document-type and theme names and makes the source of every render clear.

## Commands

The initial command set is deliberately small:

```text
docc profile install [--ref REF] [--default] REPOSITORY
docc profile use [--ref REF] [--project DIR] REPOSITORY
docc profile update [--project DIR]
docc profile status [--project DIR] [--check-remote]
docc profile package [--out DIR] [--with-binary PATH] [--zip PATH]
```

`install` clones and validates a revision under the XDG data directory.
`use` installs a revision and writes a project binding and lockfile. `update`
re-resolves the source and ref recorded by either the selected project or the
user default; it does not alter a project until the resulting lockfile is
reviewed and committed. `status` reports the selected source, ref and commit. Its optional
`--check-remote` explicitly queries Git and marks the selected revision stale
when the source ref now resolves to another commit. `check` and `build` must
not fetch, clone or update.

A `REPOSITORY` is whatever `git clone` accepts, a local path included, which is
how a pack is tested before it is pushed. The clone still takes the committed
revision: an uncommitted edit in that working tree is not part of any installed
pack. Authoring against a live working tree is what the legacy `.docc/schemas`
and `.docc/themes` entries are for — see
[Building profiles](building-profiles.md).

## Shipping a pack as an AgentSkill

`docc profile package` writes the resolved profile as an AgentSkill directory:

```text
<id>-skill/
  SKILL.md            generated from the schemas
  docc-profile.yaml   generated; makes the skill a pack in its own right
  config/schemas/     copied verbatim
  config/themes/      copied verbatim, assets included
  examples/           one document per renderable type
  probe.sh            generated self-test
  bin/                only with --with-binary
```

The generated manifest is what lets the instructions say `docc check
document.md` instead of repeating `--schema-dir config/schemas --theme-dir
config/themes` on every line — two flags an agent can forget separately, and
does. `SKILL.md` tells it to set `DOCC_PROFILE` to the skill's own directory,
which works from whatever directory the document lives in; the explicit flags
still work and are documented beside it.

`SKILL.md` is **generated**, not carried alongside the pack. A hand-written
skill file drifts from the types it claims to document, and the drift is only
discovered by whoever tries to use it — the repository's own Cowork skill had
renamed a type, dropped the block and span declarations and shipped no
examples before this command existed. The type table, the example commands and
the frontmatter description all come from the schemas.

The compiler's own contract is generated too, because it is the half that does
*not* come from the schemas and is therefore the half a generator silently
omits: the `docc: 1` marker without which nothing is validated, the exit codes
and the two `--json` shapes, that `--force` is never for a deliverable, that
`.docx` is the supported output and a PDF is the host's job, and that a request
which does not fit a type is a question for the profile's owner rather than a
reason to weaken it. One section is conditional: cross-document agreement and
`DOC029` are explained only when some packaged type declares a `spans_agree`
rule, and the watched span types are named from the rule itself. A pack that
never asks two documents to agree does not need the paragraph, and a pack that
does gets it without anyone remembering to write it.

The hand-written part comes from the pack's own `skill:` block — its notes file
and its description — so the firm's drafting guidance lives beside the schemas
it describes. `--notes FILE` appends *after* it rather than replacing it: the
two are different halves that both belong, since a pack knows when to ask for a
Heimatort and only the person packaging knows how a particular host hands a
file back.

Examples come from each schema's `example:` field, so a shipped example cannot
fall out of step with its type. Packaging validates every schema/theme pair
first: a profile that cannot render is refused on the machine that can still
fix it, rather than inside someone else's VM.

Two modes:

- **Configuration only (default)** — no `bin/`; the skill expects `docc` on
  PATH. Right for internal rollout where the binary is installed once. The
  artifact stays small, architecture neutral, and free of a docc version pinned
  inside every consumer's copy.
- **`--with-binary PATH`** — bundles an executable for sandboxed hosts with no
  install step. Architecture specific, so the bundled build must match the host.

`--zip` writes the archive with the skill nested at its root, which is what the
skill format requires. Timestamps are fixed, so the same profile packages to
the same bytes.

For private distribution the pack repository doubles as its own marketplace: a
CI job on tag runs `docc profile package` and publishes the payload. The pack
repository stays the single source of truth, SSH stays the transport, and
nothing new is hosted.

## Trust policy

A firm that cares who may change its letterhead can require a signature:

```yaml
# ~/.config/docc/config.yaml
format: 1
policy:
  require_signature: true
  allowed_signers:
    - SHA256:7ZTCVMwgbsozLB4URSN3HW5rHfbSU521iYifH06FTC4
```

The policy lives in the **user configuration**, not in a project binding or a
pack manifest. It is the operator's decision, deployed with the rest of a
workstation's configuration, and a repository must not be able to lower the bar
it is checked against. `--require-signature` and `--signer` add to it for one
command; neither can relax it.

Naming allowed signers is itself a policy: it implies something must be signed.
An empty list with `require_signature: true` accepts any key the local keyring
trusts, which is a weaker and deliberately separate statement from naming the
firm's own keys.

Verification happens during `install`, `use` and `update`, while the checkout
still has its Git metadata — an installed pack has had `.git` removed, so no
later command could verify anything. An annotated tag is checked in preference
to the commit it points at, because signing the tag is how a release is
normally marked and the commit beneath it is often unsigned. Both GPG and SSH
signing work; fingerprints are compared the way each format means them, so a
hex GPG fingerprint ignores case and spacing while a base64 SSH fingerprint
does not.

Nothing is verified unless a policy asks for it. That default is explicit, not
an oversight: a machine with no configuration installs as before.

## Provenance in the output

Every build stamps what produced it into the document's custom properties,
which Word shows under File > Info > Properties > Advanced:

| Property | Value |
| --- | --- |
| `docc-config` | `project-profile`, `legacy-project` or `user-default` |
| `docc-profile` | the pack id |
| `docc-profile-source` | the pack's Git source |
| `docc-profile-ref` | the requested ref, when there is one |
| `docc-profile-commit` | the exact locked commit |

"Which pack version produced this filed document" is the question a compliance
review actually asks, and answering it from the file itself beats keeping a
separate ledger that can be wrong. The values come from the resolved lock and
never from the clock or the filesystem, so a rebuild stays byte-identical and
no local path reaches a document that leaves the building.

## Safety boundaries

- Git is invoked with fixed arguments, never through a shell.
- A profile revision is validated by loading its manifest, schemas and themes,
  then checking every schema/theme pair before it is accepted.
- A revision refused by the trust policy is never moved into the profile store,
  so a later install cannot find it already present and skip verification.
- Local `.docc/schemas` and a managed profile binding are ambiguous and are
  rejected rather than merged.

## Delivery sequence

1. ~~Manifest, XDG paths, project binding/lock, resolver, and the local Git
   install/use/update/status workflow.~~ Done.
2. Integrate profile provenance into `docc doctor`, JSON output and LSP. The
   build already stamps it into the output.
3. ~~Add optional organisation trust policy (for example, signed-tag checks).~~
   Done.
4. Publish a separately versioned `docc-profiles` repository containing the
   generic defaults, examples and its own compatibility/rendering CI.
