#!/usr/bin/env bash
set -euo pipefail

agy_bin=$(command -v agy || true)
if test -z "$agy_bin"
then printf 'ERROR: agy is not on PATH\n' >&2
     exit 1
fi

model="${VOXI_AGY_CANARY_MODEL:-gemini-3.7-flash-low}"
printf 'agy=%s model=%s\n' "$agy_bin" "$model"
agy models | grep -F "$model"

plain=$(agy --print='Reply with exactly: CANARY_OK' --model "$model")
if test "$plain" != "CANARY_OK"
then printf 'ERROR: plain output was %q\n' "$plain" >&2
     exit 1
fi
printf 'plain: ok\n'

json=$(agy --print='Reply with exactly: CANARY_OK' --output-format json --model "$model")
printf '%s' "$json" | grep -F '"status":"SUCCESS"' >/dev/null
printf '%s' "$json" | grep -F '"response":"CANARY_OK' >/dev/null
printf 'json: ok\n'

stream=$(agy --print='Reply with exactly: CANARY_OK' --output-format stream-json --model "$model")
printf '%s' "$stream" | grep -F '"event":"result"' >/dev/null
printf 'stream-json: ok\n'
