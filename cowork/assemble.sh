#!/usr/bin/env sh
# Assemble the docc Cowork artifacts from the single source of truth: docc/.
#
# Produces (all git-ignored — regenerate anytime):
#   1. docc-cowork-skill.zip                     — standalone skill (Customize > Skills upload)
#   2. marketplace/plugins/docc/skills/docc/     — experimental plugin payload
#
# The skill directory `cowork/docc/` is authoritative; both artifacts are copies
# of it, so SKILL.md / config / examples are edited in one place only. The docc
# binary is built here rather than committed.
#
# Run from anywhere:  sh cowork/assemble.sh

set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)     # cowork/
ROOT=$(cd "$HERE/.." && pwd)                           # repo root
SKILL="$HERE/docc"
cd "$ROOT"

# 1. Build the x86_64 binary (Cowork VM arch) into the skill.
VER=$(git describe --tags --dirty --always 2>/dev/null || echo 0.0.0-dev)
echo "building docc ${VER}-cowork (linux/amd64) ..."
mkdir -p "$SKILL/bin"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.buildVersion=${VER}-cowork" \
  -o "$SKILL/bin/docc-linux-amd64" ./cmd/docc
chmod +x "$SKILL/bin/docc-linux-amd64"

# 2. Standalone zip — docc/ nested at the archive root (required by the spec).
echo "packaging standalone skill zip ..."
rm -f "$HERE/docc-cowork-skill.zip"
( cd "$HERE" && python3 -m zipfile -c docc-cowork-skill.zip docc )

# 3. Plugin payload — mirror docc/ into the plugin's skills/ tree.
echo "syncing plugin skill payload ..."
DEST="$HERE/marketplace/plugins/docc/skills/docc"
rm -rf "$DEST"
mkdir -p "$(dirname "$DEST")"
cp -r "$SKILL" "$DEST"

echo "done."
echo "  standalone : $HERE/docc-cowork-skill.zip"
echo "  plugin     : $HERE/marketplace/  (experimental prototype only)"
