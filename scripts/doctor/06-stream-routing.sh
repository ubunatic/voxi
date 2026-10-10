#!/usr/bin/env bash
# Check 6: a new playback stream is linked to a sink. Live, 2026-10-10, WirePlumber was active with
# no restarts, yet a new pw-play stream had no links and stayed silent; restarting WirePlumber fixed it.
# The check plays one second of silence (volume 0) through a probe stream, then looks for the
# probe's output links with pw-link -l. Read-only: the repair is printed, never run.
# Usage: 06-stream-routing.sh [--no-probe] [--pw-link FILE]   (--pw-dump and --since are ignored)
set -euo pipefail

CHECK=stream-routing
PROBE_NODE=voxi-doctor-probe
source "$(dirname "$0")/lib.sh"
parse_args "$@"

# probe_target FILE: prints the sink node the probe's output links to, empty if it has no link.
# pw-link -l prints each port at column 0 with its links indented below it, so the links are
# the "|->" lines under the probe's port headers.
probe_target() {
   awk -v probe="$PROBE_NODE" '
      /^[^ ]/ { from = index($0, probe ":") == 1 }
      from && /\|-> / { sub(/.*\|-> /, ""); sub(/:[^:]*$/, ""); print; exit }
   ' "$1"
}

# report TARGET: the verdict for the sink the probe linked to, or the fault when there is none
report() {
   if test -n "$1"
   then verdict_ok "probe stream linked to sink '$1'"
   fi
   repair="systemctl --user restart wireplumber"
   add_problem "a new playback stream was not linked to any sink (WirePlumber is not routing streams)" "$repair"
   verdict_fault "$repairs" "$problems"
}

# canned output for the tests: no probe, no PipeWire needed
if test -n "$PW_LINK_FILE"
then test -r "$PW_LINK_FILE" || verdict_skip "cannot read $PW_LINK_FILE"
     report "$(probe_target "$PW_LINK_FILE")"
fi

if test "$RUN_PROBE" = no
then verdict_skip "--no-probe given, the stream probe was not run"
fi
need_cmd pw-play pw-link
pw-dump >/dev/null 2>&1 || verdict_skip "pw-dump failed, PipeWire is not reachable"

probe_dir=$(mktemp -d)
probe_pid=""
cleanup() {
   if test -n "$probe_pid"
   then kill "$probe_pid" 2>/dev/null || true
   fi
   rm -rf "$probe_dir"
}
trap cleanup EXIT

# one second of 48 kHz mono 16-bit silence: a 44-byte WAV header, then 96000 zero bytes
printf 'RIFF\x24\x77\x01\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\xbb\x00\x00\x00\x77\x01\x00\x02\x00\x10\x00data\x00\x77\x01\x00' > "$probe_dir/silence.wav"
head -c 96000 /dev/zero >> "$probe_dir/silence.wav"

pw-play --volume 0 -P "node.name=$PROBE_NODE" "$probe_dir/silence.wav" >/dev/null 2>&1 &
probe_pid=$!

# poll for up to 2 s: the links exist only while the probe plays, and a hung pw-play may never link
target=""
tries=0
while test -z "$target" && test "$tries" -lt 20
do sleep 0.1
   pw-link -l > "$probe_dir/links.txt" 2>/dev/null || true
   target=$(probe_target "$probe_dir/links.txt")
   tries=$((tries + 1))
done
report "$target"
