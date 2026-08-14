# Publishing the docc Agent Skill

Keep one portable skill in `skills/docc`. Packaging adds the compiled runtime
and vendor manifest without forking the instructions.

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
`docc` executable on `PATH`.

Push a `v*` tag to test, create a GitHub release, and upload all archives.
Manual workflow runs create downloadable CI artifacts without publishing.
Test representative prompts in clean Claude/Cowork and ChatGPT Work sessions
before completing each vendor's listing and review process.
