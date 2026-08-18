# Command-line reference

Every subcommand, its flags, the exit codes, and the JSON contract. The
landing page has the [quick start](../README.md#quick-start).

```bash
docc profile use <repository>     # install a Git-managed pack and bind this project
docc profile install --default <repository> # install a user-wide default pack
docc profile status               # show the resolved local profile revision
docc profile status --check-remote # explicitly check whether its ref advanced
docc init                         # copy the built-in starter pack out as an editable checkout
docc init --dry-run               # list what it would create, without writing
docc doctor                       # which schemas and themes are in effect, and are they sound
docc check docs/klage.md          # validate
docc check --json docs/*.md       # machine-readable, for agents and CI
docc check --strict docs/klage.md # warnings become errors
docc build docs/klage.md          # validate, then emit a .docx
docc build --to pdf docs/klage.md # optional compatibility export; needs soffice
docc lsp                          # serve editor diagnostics over stdio
docc types                        # list known document types
docc describe ch_legal            # report a document type's full contract
docc example ch_legal             # print a compact valid document to start from
docc example --blank ch_legal     # …with every field marker emptied, as a skeleton
docc describe --from ~/kanzlei ch_legal  # …for a project other than this directory
docc explain                      # list every diagnostic code
docc explain DOC010               # describe one
docc explain DOC010 --type ch_legal  # …and the constraints that schema declares
docc version                      # print the version; `docc --version` is the same
```

Flags may appear before or after the positional arguments, so
`docc build docs/klage.md --to pdf` works. Use `--` to end flag parsing when a
file name begins with a dash. `--help` on any subcommand prints its usage and
exits `0`, and `--version` answers as the `version` subcommand does.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | clean |
| `1` | the command ran and reported diagnostics, or failed part-way through |
| `2` | usage error — the command line is wrong; a different invocation may work |
| `3` | configuration error — the project's schemas or themes are missing or unusable |

`2` and `3` are separated because a caller can act on the difference. A wrong
flag is worth retrying; a missing profile configuration is not.

### Which configuration am I using?

Schemas and themes resolve from `DOCC_PROFILE` (a pack directory named by the
environment), a project profile binding, the `docc-profile.yaml` of a pack
checkout you are standing in, the user's installed default profile pack, or —
with nothing configured — the starter pack embedded in the binary. `docc doctor` reports the directories that resolved to, lists the
types and themes it found, and checks every schema against the theme it names —
that every mapped style exists, every interpolated field is declared, and every
numbering definition resolves. Those checks otherwise run only inside a build, so
a profile that could never render stayed invisible until someone authored a
document for it.

```
$ docc doctor
configuration:
  schemas        /srv/kanzlei/schemas  (pack-checkout)
  themes         /srv/kanzlei/themes  (pack-checkout)

document types:
  base       check-only  declares no theme, cannot be built
  ch_legal   ok          theme starter-legal
  ch_letter  ok          theme starter-letter
```

`.docx` is the supported compiler output. `--to pdf` remains a
compatibility-only export for environments that provide LibreOffice (`soffice`).
Hosts driving docc should build DOCX and use their own document/PDF capability
when a user requests a PDF.

### JSON contract

`--json` produces one JSON object on stdout for each successful command result.
It never mixes human-readable status text into that stream.

| Command | stdout JSON |
|---|---|
| `check --json` | `{ "ok", "errors", "warnings", "diagnostics" }` |
| `build --json` | `{ "ok", "type", "theme", "format", "output" }`; validation diagnostics are a separate JSON object on stderr |
| `types --json` | `{ "types": [{ "type", "description", "theme" }] }` |
| `describe --json` | `{ "type", "extends", "theme", "frontmatter", "body", "blocks", "spans", "blanks", "rules", "has_example", "field_map" }` — the full contract, with a `syntax` example per block, span and blank |
| `themes --json` | `{ "themes": [{ "name", "description", "styles" }] }` |
| `doctor --json` | `{ "start", "root", "schema_dir", "schema_source", "theme_dir", "types", "themes", "problems", "ok" }` |
| `explain --json` | `{ "code", "explanation", "type", "detail" }`, or `{ "codes": [...] }` with no code given |
| `profile status --json` | `{ "root", "schema_dir", "theme_dir", "source", "reference", "remote_commit", "stale" }` |

`describe` reports each frontmatter field with everything the schema declares
about it: whether it is `required`, whether it is `nullable` (so an explicit `~`
satisfies the requirement), its `pattern`, enum `values`, `default` and hint.
Object types declared in the schema's `types:` are expanded inline as `members`,
so a `sender: sender` field also lists the fields underneath it.

Each field also carries `rendered`: the places the type's theme interpolates it —
`prologue`, `epilogue`, `header:<key>`, `footer:<key>`. A field with none is
metadata the theme never prints. `field_map` reports whether the theme could be
consulted at all; when it is `false`, an empty `rendered` means nothing.

Body headings report `required`, `required_when` (the frontmatter condition that
makes an otherwise optional section mandatory) and `ordered`.

Failures stay on the JSON stream. Under `--json`, a command that cannot produce
its result writes a failure object to stdout instead:

```json
{ "ok": false, "kind": "config", "error": "unknown document type \"nosuch\" (known types: ch_legal, ch_letter)" }
```

`kind` is `usage`, `config`, or `error`, matching the exit code. `build`'s
validation failure adds `"kind": "diagnostics"` and the document `type`.

Two paths stay human-readable, deliberately: a flag that fails to parse (the
command line is malformed, so `--json` may not have been understood either), and
everything in `docc lsp`, whose stdout carries the LSP protocol.
