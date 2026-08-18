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

`docc init` scaffolds an editable pack checkout from the starter pack built
into docc, and an unconfigured docc resolves that embedded pack directly. Both
are starting points; a managed `docc profile use` binding is the way to consume
an organisation's approved conventions.

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
  examples/
```

The required manifest makes the repository unambiguous:

```yaml
format: 1
id: example-kanzlei
name: Example Kanzlei profiles
schemas: schemas
themes: themes
```

`id` is a stable, filesystem-safe identifier. `schemas` and `themes` are
relative paths inside the repository. The manifest is parsed strictly: an
unknown key is a load error, not a warning.

Compatibility is verified by the profile repository's CI against supported
`docc` releases. A pack contains data and assets only: `docc` never executes a
script supplied by a profile repository. Anything else a pack repository wants
to ship — agent instructions, packaging scripts, CI — is its own business and
invisible to docc.

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

A user may also configure one default pack; a project binding still wins and
should be used for shared or filed work. With nothing configured at all, docc
falls back to the starter pack embedded in the binary.

## Resolution order

Schema and theme resolution has one shared precedence order:

1. Explicit `--schema-dir` and `--theme-dir` flags.
2. `DOCC_PROFILE`, naming a pack directory. It is for the case neither walking
   up nor the working directory can answer: a host carrying a pack beside
   itself, compiling documents that live wherever the agent happens to be. A
   value that names no usable pack is an error, never a fallback — compiling
   against schemas nobody chose is the failure this whole order exists to
   prevent.
3. A nearest project `.docc/profile.yaml` plus its lockfile.
4. A nearest `docc-profile.yaml` — you are standing inside a pack checkout.
5. The user's configured default pack.
6. The starter pack embedded in the binary (`builtin`), so an unconfigured
   docc still works.

The legacy layout — bare `.docc/schemas` without a manifest — is no longer
resolved. It fails with an error naming the fix, because silently falling back
to the builtin pack would compile the document against schemas nobody chose.

Step 4 is what makes a pack repository usable from inside itself. A pack has no
`.docc`: its schemas and themes are the product, not one project's local
configuration, so working in a checkout used to mean passing `--schema-dir` and
`--theme-dir` to every command. The manifest already names both directories, so
nothing extra is written down. It sits below the project forms — a binding is an
explicit statement about which profile applies, and a pack that happens to sit
above it does not override that — and above the user default, because the pack
you are editing is a more specific answer than the one you installed globally.

Nothing is pinned by a checkout: you are working *on* the pack rather than
consuming a revision of it, so a document built this way records no commit.
`docc doctor` names which of these answered, as `env-profile`,
`project-profile`, `pack-checkout`, `user-default` or `builtin`.

Installed packs are never merged. Selecting one pack avoids collisions between
document-type and theme names and makes the source of every render clear.

## Commands

The initial command set is deliberately small:

```text
docc profile install [--ref REF] [--default] REPOSITORY
docc profile use [--ref REF] [--project DIR] REPOSITORY
docc profile update [--project DIR]
docc profile status [--project DIR] [--check-remote]
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
pack. Authoring against a live working tree is what pack-checkout resolution is
for: stand inside the checkout, and its manifest answers — see
[Building profiles](building-profiles.md).

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
| `docc-config` | the resolution source, e.g. `project-profile`, `user-default` or `builtin` |
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
- A managed profile binding beside leftover local `.docc/schemas` is ambiguous
  and rejected rather than merged.
