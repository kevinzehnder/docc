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

# No unversioned copy of the plugin zip is attached. It existed only to give an
# `archive` marketplace entry a stable /releases/latest/download URL, and this
# repository is private, so nothing unauthenticated can read a release asset —
# see docs/publishing-agent-skill.md. Restore the copy together with the entry.

sh "$root/cowork/assemble.sh" >/dev/null
cp "$root/cowork/docc-cowork-skill.zip" "$out/docc-cowork-skill-$version.zip"

for z in "$out"/*.zip; do
  printf '%s\n' "${z#"$root"/}"
done
