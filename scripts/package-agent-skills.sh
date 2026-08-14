#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${1:-$(git -C "$root" describe --tags --always 2>/dev/null || echo 0.0.0-dev)}
version=${version#v}
dist=${DOCC_DIST_DIR:-"$root/dist"}
stage="$dist/.agent-skills"
case "$version" in *[!0-9A-Za-z._+-]*|"") echo "invalid version: $version" >&2; exit 2;; esac
command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 127; }
command -v zip >/dev/null 2>&1 || { echo "zip is required" >&2; exit 127; }
rm -rf "$stage"
mkdir -p "$stage/runtime/bin" "$dist"
build() {
  (cd "$root"; CGO_ENABLED=0 GOOS=linux GOARCH="$1" go build -trimpath     -ldflags="-s -w -X main.buildVersion=$version" -o "$2" ./cmd/docc)
}
build amd64 "$stage/runtime/bin/docc-linux-amd64"
build arm64 "$stage/runtime/bin/docc-linux-arm64"
prepare() {
  mkdir -p "$1"
  cp -R "$root/skills/docc/." "$1/"
  mkdir -p "$1/scripts/bin"
  cp "$stage/runtime/bin/"* "$1/scripts/bin/"
  chmod 0755 "$1/scripts/run-docc.sh" "$1/scripts/bin/"*
}
archive() {
  rm -f "$2"
  (cd "$1"; LC_ALL=C find . -type f -print | sort | zip -X -q "$2" -@)
}
prepare "$stage/plain"
archive "$stage/plain" "$dist/docc-agent-skill-$version.zip"
mkdir -p "$stage/claude/.claude-plugin"
sed "s/\\\"version\\\":\\\"1.0.0\\\"/\\\"version\\\":\\\"$version\\\"/" "$root/.claude-plugin/plugin.json" > "$stage/claude/.claude-plugin/plugin.json"
prepare "$stage/claude/skills/docc"
archive "$stage/claude" "$dist/docc-claude-plugin-$version.zip"
mkdir -p "$stage/openai/.codex-plugin"
sed "s/\\\"version\\\":\\\"1.0.0\\\"/\\\"version\\\":\\\"$version\\\"/" "$root/.codex-plugin/plugin.json" > "$stage/openai/.codex-plugin/plugin.json"
prepare "$stage/openai/skills/docc"
archive "$stage/openai" "$dist/docc-openai-plugin-$version.zip"
printf '%s\n' "$dist/docc-agent-skill-$version.zip" "$dist/docc-claude-plugin-$version.zip" "$dist/docc-openai-plugin-$version.zip"
