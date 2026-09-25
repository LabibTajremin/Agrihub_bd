#!/usr/bin/env bash
# quiet.sh <label> <cmd...> — run a command, print its output only on failure.
set -uo pipefail
label="$1"; shift
log="$(mktemp)"
if "$@" >"$log" 2>&1; then
  echo "ok   $label"
  rm -f "$log"
else
  code=$?
  cat "$log"
  echo "FAIL $label (exit $code)"
  rm -f "$log"
  exit "$code"
fi
