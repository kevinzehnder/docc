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
unzip -Z1 "$o" | grep -qx '.codex-plugin/plugin.json'
unzip -Z1 "$o" | grep -qx 'skills/docc/SKILL.md'
unzip -Z1 "$o" | grep -qx 'skills/docc/agents/openai.yaml'
if [ "$(uname -s)/$(uname -m)" = Linux/x86_64 ]; then
  mkdir "$tmp/run"; unzip -q "$p" -d "$tmp/run"
  test "$("$tmp/run/scripts/run-docc.sh" version)" = "docc test"
fi
echo "Agent Skill packages verified"
