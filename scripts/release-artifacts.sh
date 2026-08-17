#!/bin/sh
# The release artifacts goreleaser does not build: the agent-skill archives and
# the Cowork skill. Run from .goreleaser.yaml's before hook, and by hand to see
# what a release will carry.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${1:?usage: release-artifacts.sh VERSION}
version=${version#v}
out=${DOCC_RELEASE_DIR:-"$root/build/release"}

rm -rf "$out"
mkdir -p "$out"

DOCC_DIST_DIR="$out" "$root/scripts/package-agent-skills.sh" "$version" >/dev/null

# Attached twice on purpose. The versioned name is for a human reading the
# release page; the bare one is what
# /releases/latest/download/docc-claude-plugin.zip resolves to, which is the
# stable URL the marketplace's archive entry names. A versioned URL there would
# have to be rewritten and committed on every release.
cp "$out/docc-claude-plugin-$version.zip" "$out/docc-claude-plugin.zip"

sh "$root/cowork/assemble.sh" >/dev/null
cp "$root/cowork/docc-cowork-skill.zip" "$out/docc-cowork-skill-$version.zip"

for z in "$out"/*.zip; do
  printf '%s\n' "${z#"$root"/}"
done
