# Public release notes

`docc` is intended to become an open-source project: a CLI for validating and
rendering schema-backed Markdown documents, plus an AgentSkill package for
agent hosts. The project is useful as infrastructure in its own right; a
commercial plan is not a prerequisite for publishing it.

This is a planning note, not a release commitment. Do this work deliberately
when the project is ready for public use.

## Product boundary

- The CLI is the durable core. Its supported compiler output is DOCX.
- The standalone AgentSkill ZIP is the supported agent distribution artifact.
- Agent hosts may convert a generated DOCX to PDF when requested. The CLI's
  LibreOffice PDF export is compatibility-only.
- The plugin marketplace remains experimental until its packaging and update
  model are stable.

## Before publishing

1. Audit the current tree and complete Git history for credentials, personal
   data, client material, and internal project references. Removing a file from
   the current tree is insufficient if it remains reachable in history.
2. Decide whether the contact address in plugin metadata is appropriate for a
   public repository.
3. Remove or rewrite historical references to private projects from
   public-facing documentation.
4. Publish versioned CLI binaries and the verified AgentSkill ZIP with a
   SHA-256 checksum.
5. Add lightweight public-project documentation: contribution guidance,
   security reporting guidance, release/install instructions, and supported
   platforms.

## Non-goals for the first public release

- A stable plugin marketplace integration.
- A promise to support every agent host or editor.
- Replacing host-native PDF handling with a bundled PDF renderer.

The repository is already MIT-licensed. Public readiness is primarily a matter
of provenance, documentation, reproducible release artifacts, and clear support
boundaries.
