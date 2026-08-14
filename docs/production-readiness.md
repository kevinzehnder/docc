# Production-readiness plan

## Goal

Ship `docc` as a small, dependable Unix-style document compiler:

> Structured Markdown in; validated, house-style DOCX or PDF out.

The original problem is the product: humans and language models prefer writing
Markdown, while professional recipients still require editable Word documents
and finished PDFs with exact conventions. `docc` should solve that conversion
exceptionally well and avoid adjacent document-management responsibilities.

## Principles

1. **One job.** Parse, validate, and render a document.
2. **Contracts fail loudly.** Unknown configuration and invalid documents must
   not degrade silently.
3. **Safe by default.** Never produce a deliverable by bypassing validation.
4. **Deterministic core.** Equal source and configuration produce equal DOCX.
5. **Explicit external boundary.** PDF conversion depends on a known
   LibreOffice/font environment.
6. **Project-owned conventions.** Schemas and themes own house style; Go remains
   domain-neutral.
7. **Machine-friendly interface.** Stable exit codes, JSON, stdout, and stderr.
8. **Delete before adding.** New surface must justify its permanent maintenance
   cost.

## Definition of done for v0.1.0

A release is ready when:

- a clean machine can install a versioned binary;
- `docc init`, `check`, and `build` complete the documented quick start;
- invalid content or configuration cannot silently produce a deliverable;
- DOCX artifacts are deterministic and open without repair in Word and
  LibreOffice;
- PDF output is tested with the documented renderer and fonts;
- representative letter and legal layouts have been visually and physically
  verified;
- Linux and macOS artifacts, checksums, and Agent Skill packages are attached to
  a tagged release;
- CI runs on every pull request and on the default branch.

## Phase 0: remove out-of-scope code

### Delete the ingestion subsystem

Remove:

- `internal/ingest/`;
- `.docc/ingest.yaml`;
- the `docc ingest` command and flags;
- ingestion-specific imports, helpers, diagnostics, fixtures, and tests;
- all related documentation and examples.

Do not leave a deprecated command, compatibility shim, hidden flag, or roadmap
entry. The subsystem is outside the product boundary and has no released
compatibility contract.

Acceptance criteria:

- `rg -i 'ingest|vlm|raster|anchor|chat completions' .` finds no product code
  or user documentation;
- `go mod tidy` introduces no replacement dependencies;
- `task` passes;
- the compiled binary exposes only the document compilation surface.

### Remove obsolete design history

- Delete implementation plans whose work is already represented by code and
  tests.
- Keep only live specifications, current operator documentation, and decisions
  that prevent likely regressions.
- Use Git history for archaeological detail.

Acceptance criteria:

- every file under `docs/` answers a current user, contributor, release, or
  specification question;
- no document refers to private sibling repositories or an obsolete cutover.

## Phase 1: make the CLI contract correct

### Apply common flags consistently

Fix `build --strict` so warnings gate the build exactly as documented.

Make `--json` consistent for success and failure. Define one versioned JSON
shape rather than emitting unrelated line fragments. Suggested envelope:

```json
{
  "version": 1,
  "ok": false,
  "diagnostics": [],
  "output": null
}
```

Acceptance criteria:

- every command advertised as supporting JSON emits valid JSON on stdout;
- diagnostics do not switch to coloured prose on error;
- operational messages remain on stderr;
- integration tests assert stdout, stderr, and exit code together.

### Resolve configuration per input

`docc check a.md b.md` must either:

1. resolve the nearest project independently for each input; or
2. reject inputs that resolve to different project roots.

Prefer independent resolution because it matches shell glob behaviour.

Acceptance criteria:

- a test checks two files under different `.docc` roots;
- each file uses its own schema set;
- diagnostics retain stable display paths.

### Make output path handling safe

- Validate that `--to` and `--output` agree.
- Reject identical source, intermediate, and destination paths.
- Build DOCX and PDF through temporary files in the destination directory.
- Verify the artifact before atomically renaming it into place.
- Clean temporary files on every error path.
- Never delete a caller-owned destination as intermediate cleanup.

Test at least:

- default DOCX output;
- explicit DOCX output;
- default PDF output;
- explicit PDF output;
- misleading extensions;
- existing destination;
- interrupted or failed conversion;
- paths containing spaces.

### Make project initialisation atomic

Build the complete starter in a temporary sibling directory, then rename its
components into place. On failure, remove only temporary content created by the
current process.

Decide and document whether `init` is all-or-nothing or can install missing
components into an existing project. Prefer all-or-nothing for v0.1.0.

Acceptance criteria:

- an injected mid-copy failure leaves no partial project;
- rerunning after failure succeeds;
- existing user files are never overwritten.

### Remove validation bypass

Remove the public `--force` build flag. The compiler's value is that invalid
documents do not become deliverables.

If developers need to inspect partial emission, expose it only through tests or
an explicitly unstable development command that is absent from release help.

## Phase 2: harden configuration

Add semantic validation for every explicit schema and theme value. Unknown
values must be errors; only omitted values may select defaults.

Cover at least:

- page size and orientation;
- dimensions and font sizes;
- colours;
- alignments, underline and border styles;
- numbering formats, alignment, suffix, depth, and references;
- style inheritance and style references;
- schema inheritance cycles and duplicate types;
- body-rule contradictions;
- default values against declared field types;
- interpolation fields and repeated values;
- image existence, supported formats, and dimensions;
- render start markers and mutually exclusive settings.

Prefer one validation pipeline used by `check`, `build`, tests, and future
editor tooling. Do not add a second source of truth.

Acceptance criteria:

- each configuration field has either validation coverage or a documented reason
  no validation is required;
- a typo cannot silently fall back to A4, a default style, or empty text;
- configuration errors identify the file and offending field.

## Phase 3: make PDF production-grade

LibreOffice remains an external renderer. Do not embed office conversion logic
or add a second PDF engine.

### Runtime preflight

Before writing an intermediate artifact:

- locate `soffice` or `libreoffice`;
- report the executable and version in verbose diagnostics;
- detect required fonts declared by the selected theme where practical;
- explain missing runtime requirements before a long build begins.

A small `docc doctor` command is acceptable only if it remains a read-only
preflight over the same checks used by `build --to pdf`.

### Reference environment

Define a supported CI/release render environment:

- pinned LibreOffice major/minor version;
- explicit font packages and any licensed font installation boundary;
- locale and timezone;
- container or reproducible setup instructions.

Do not claim byte-deterministic PDF unless it is proven. The required guarantee
is stable, reviewed layout in the supported environment.

### Round-trip and visual tests

For representative fixtures:

1. generate DOCX;
2. open/convert it with LibreOffice;
3. assert that a non-empty PDF was produced;
4. render PDF pages to images;
5. compare stable geometry or reviewed visual snapshots.

Avoid a brittle full-page pixel comparison across uncontrolled machines. Run
visual regression only in the pinned environment.

## Phase 4: verify the real layouts

Engine correctness does not prove that a letter fits an envelope or a legal
brief matches established practice.

For each supported production theme:

- obtain a sanitised reference PDF or measured specification;
- compare text and frame coordinates;
- inspect page images;
- print the document;
- test folds, window envelopes, margins, headers, footers, and page breaks;
- test short, long, empty-optional, and overflow content.

Minimum fixtures:

- one-page letter;
- multi-page letter;
- long recipient and organisation names;
- legal document with three heading levels;
- legal document with continuous marginal numbering;
- tables, lists, evidence blocks, images, headers, and footers.

Record accepted measurements in a live specification, not in Go comments.

## Phase 5: reduce public surface

### Decide whether `pkg/docx` is a product

Unless there is a real external consumer and a compatibility policy, move it
under `internal/docx` before v0.1.0. Advertising an importable package turns
every exported identifier into long-term API surface.

If it remains public, add:

- semantic-versioning policy;
- package-level compatibility tests;
- supported feature matrix;
- examples that are compiled in CI.

### Contain the LSP

Do not expand LSP features during v0.1.0 hardening. Choose one:

- keep diagnostics-only LSP as a secondary command;
- move it to a separate `docc-lsp` binary;
- remove it until real editor usage justifies maintenance.

The decision criterion is usage, not code already written.

### Add Unix composition

After the file-based workflow is solid, support `-` where it is unambiguous:

```sh
cat document.md | docc check --schema-dir .docc/schemas -
cat document.md | docc build --schema-dir .docc/schemas \
  --theme-dir .docc/themes --to docx --output - > document.docx
```

Rules:

- artifact bytes only on stdout;
- diagnostics and progress only on stderr;
- project discovery from stdin requires explicit configuration or working
  directory semantics;
- never write colour escapes to redirected output.

This is useful but must not delay v0.1.0.

## Phase 6: reproducible CI and releases

### CI

Run on pull requests and pushes to the default branch:

```sh
gofumpt -l .
go vet ./...
golangci-lint run ./...
go test ./...
go test -race ./...
go build ./cmd/docc
./scripts/test-agent-packages.sh
```

Add a separate job with LibreOffice and fonts for the round-trip suite.

Pin:

- Go version;
- `gofumpt`;
- `golangci-lint`;
- GitHub Action major versions;
- LibreOffice/font environment where reproducibility matters.

Do not install developer tools from `@latest` in CI.

Use least-privilege workflow permissions. Test jobs need read access; release
write permission belongs only to the tag release job.

### Releases

A `v*` tag should produce:

- Linux amd64 and arm64 binaries;
- macOS amd64 and arm64 binaries;
- optional Windows binary only if tested;
- SHA-256 checksums;
- the plain Agent Skill archive;
- Claude plugin archive;
- OpenAI plugin archive;
- generated release notes.

Test the produced binaries and archives, not only an independently built copy.

Before public distribution, add an explicit licence and basic security reporting
instructions.

### Agent packages

Keep one canonical skill source and generate vendor packaging around it.

- Cloud packages may bundle static Linux amd64/arm64 runtimes.
- Local macOS agents must use a matching bundled binary or a documented
  `docc` on `PATH`.
- Do not download executables at skill runtime.
- Include artifact checksums in releases.
- Forward-test representative document prompts in clean ChatGPT Work and
  Claude/Cowork sessions before publishing.

## Phase 7: documentation contract

Maintain only:

- `README.md`: value proposition, installation, quick start, core model;
- `CLAUDE.md`: concise contributor invariants;
- live layout specifications;
- Agent Skill publishing instructions;
- this production-readiness plan until v0.1.0.

Create separate schema/theme reference documents only when the README can no
longer explain them concisely. Generate command help from the CLI where possible
rather than maintaining duplicate flag lists.

Documentation tests should verify:

- quick-start commands execute;
- every documented command and flag exists;
- links resolve;
- examples validate and build.

## Explicit non-goals

Do not add:

- OCR or document ingestion;
- VLM or LLM API clients;
- document storage or databases;
- collaboration or review workflows;
- a web server or hosted service;
- a general template programming language;
- conditional theme logic where a second theme or explicit field suffices;
- automatic invention of missing legal or factual content;
- multiple PDF engines.

Agents may author Markdown around `docc`; the compiler itself remains
deterministic and model-independent.

## Suggested implementation sequence

1. Delete out-of-scope code and stale documentation.
2. Fix strict/JSON/multi-project CLI behaviour.
3. Make build and init output handling atomic.
4. Remove `--force`.
5. Complete schema/theme semantic validation.
6. Add pinned normal CI and LibreOffice round-trip CI.
7. Verify fonts and real layouts.
8. Decide the public Go package and LSP boundaries.
9. Build and test release artifacts.
10. Tag `v0.1.0`.
11. Publish Agent Skill packages after clean-session forward tests.

Do not start another feature phase until steps 1–7 are complete. The engine
already has enough capability; the remaining work is deletion, correctness,
layout proof, and distribution.
