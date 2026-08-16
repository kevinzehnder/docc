#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
system=$(uname -s)
machine=$(uname -m)
case "$system/$machine" in
  Linux/x86_64|Linux/amd64) bundled="$script_dir/bin/docc-linux-amd64" ;;
  Linux/aarch64|Linux/arm64) bundled="$script_dir/bin/docc-linux-arm64" ;;
  *) bundled="" ;;
esac
if [ -n "$bundled" ] && [ -x "$bundled" ]; then exec "$bundled" "$@"; fi
if command -v docc >/dev/null 2>&1; then exec docc "$@"; fi
echo "docc runtime unavailable for $system/$machine" >&2
echo "Use a published skill archive or install docc on PATH." >&2
exit 127
