#!/usr/bin/env sh
# One-shot Cowork probe for the docc skill.
#
# Answers the three questions that decide whether docc can ship as a
# binary-bearing Cowork skill:
#   1. Will the VM execute a bundled custom binary at all?
#   2. What CPU architecture is the VM (which bundled binary to use)?
#   3. Does a docc-built .docx land in the workspace for delivery?
#
# Run it from the skill's own directory:  sh probe.sh
# It prints a PROBE RESULT summary and writes probe-out.docx here on success.

set -eu

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$DIR"

echo "== docc Cowork probe =="
echo "uname -s : $(uname -s 2>/dev/null || echo '?')"
echo "uname -m : $(uname -m 2>/dev/null || echo '?')"

# 2. This skill ships an x86_64 build only (the Cowork VM is x86_64).
case "$(uname -m 2>/dev/null || echo unknown)" in
  x86_64|amd64) BIN="bin/docc-linux-amd64" ;;
  *) echo "PROBE RESULT: VM arch '$(uname -m)' is not x86_64; this skill ships an amd64 build only. Rebuild docc for this arch."; exit 3 ;;
esac
echo "selected : $BIN"

if [ ! -f "$BIN" ]; then
  echo "PROBE RESULT: FAIL — bundled binary $BIN not present in the skill."; exit 3
fi

# Packaging may strip the exec bit; restore it.
chmod +x "$BIN" 2>/dev/null || true

# 1. Does the binary run in this VM?
if ! VER=$("./$BIN" version 2>&1); then
  echo "----- output -----"; echo "$VER"; echo "------------------"
  echo "PROBE RESULT: FAIL — VM refused to execute the bundled binary."
  echo "  => the binary-bearing skill path is not viable; fall back to a remote MCP connector."
  exit 1
fi
echo "docc     : $VER"

# 3. Real work: validate then render the bundled example to a workspace file.
EX="examples/letter.md"
OUT="$DIR/probe-out.docx"
echo "-- check $EX --"
"./$BIN" check --schema-dir config/schemas "$EX"
echo "-- build $EX --"
"./$BIN" build --schema-dir config/schemas --theme-dir config/themes --output "$OUT" "$EX"

if [ -s "$OUT" ]; then
  echo "wrote    : $OUT ($(wc -c < "$OUT") bytes)"
  echo "PROBE RESULT: PASS — VM executes the bundled docc binary and produced a .docx."
  echo "  => the binary-bearing Cowork skill path is viable on this arch ($(uname -m))."
else
  echo "PROBE RESULT: FAIL — build reported success but no .docx was written."
  exit 2
fi
