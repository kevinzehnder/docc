#!/usr/bin/env sh
# Verify the standalone skill exactly as a recipient receives it.

set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ZIP="$HERE/docc-cowork-skill.zip"

if [ ! -f "$ZIP" ]; then
  echo "missing $ZIP; run sh cowork/assemble.sh first" >&2
  exit 1
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/docc-skill-verify.XXXXXX")
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

unzip -q "$ZIP" -d "$TMP"

if [ ! -f "$TMP/docc/SKILL.md" ] || [ ! -x "$TMP/docc/bin/docc-linux-amd64" ]; then
  echo "invalid skill ZIP: expected docc/SKILL.md and executable bundled binary" >&2
  exit 1
fi

echo "== verifying packaged docc skill =="
(cd "$TMP/docc" && sh probe.sh)
echo "ZIP VERIFY: PASS"
