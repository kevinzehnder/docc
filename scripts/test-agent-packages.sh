#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
DOCC_DIST_DIR="$tmp/dist" "$root/scripts/package-agent-skills.sh" test
p="$tmp/dist/docc-agent-skill-test.zip"
c="$tmp/dist/docc-claude-plugin-test.zip"
o="$tmp/dist/docc-openai-plugin-test.zip"
for z in "$p" "$c" "$o"; do test -s "$z"; unzip -tq "$z" >/dev/null; done
for f in SKILL.md scripts/run-docc.sh scripts/bin/docc-linux-amd64 scripts/bin/docc-linux-arm64; do unzip -Z1 "$p" | grep -qx "$f"; done
unzip -Z1 "$c" | grep -qx '.claude-plugin/plugin.json'
unzip -Z1 "$c" | grep -qx 'skills/docc/SKILL.md'
unzip -p "$c" .claude-plugin/plugin.json | grep -q '"version": "test"'
unzip -p "$o" .codex-plugin/plugin.json | grep -q '"version": "test"'
for m in "$root/.claude-plugin/plugin.json" "$root/.codex-plugin/plugin.json"; do
  if grep -q '"version"' "$m"; then
    echo "$m pins a version; a marketplace install then never updates" >&2
    exit 1
  fi
done
if command -v claude >/dev/null 2>&1; then claude plugin validate "$root"; fi
unzip -Z1 "$o" | grep -qx '.codex-plugin/plugin.json'
unzip -Z1 "$o" | grep -qx 'skills/docc/SKILL.md'
unzip -Z1 "$o" | grep -qx 'skills/docc/agents/openai.yaml'
if [ "$(uname -s)/$(uname -m)" = Linux/x86_64 ]; then
  mkdir "$tmp/run"; unzip -q "$p" -d "$tmp/run"
  test "$("$tmp/run/scripts/run-docc.sh" version)" = "docc test"
fi
echo "Agent Skill packages verified"
