#!/usr/bin/env bash
set -euo pipefail

probe_node="${1:?probe node name required}"
wav_path="${2:?silence wav path required}"
tmp_dir=$(mktemp -d)
probe_pid=""
cleanup() {
   if test -n "$probe_pid"
   then kill "$probe_pid" 2>/dev/null || true
   fi
   rm -rf "$tmp_dir"
}
trap cleanup EXIT

pw-play --volume 0 -P "node.name=$probe_node" "$wav_path" >/dev/null 2>&1 &
probe_pid=$!
tries=0
while test "$tries" -lt 20
do pw-link -l > "$tmp_dir/links.txt" 2>/dev/null || true
   target=$(awk -v probe="$probe_node" '
      /^[^ ]/ { from = index($0, probe ":") == 1 }
      from && /\|-> / { sub(/.*\|-> /, ""); sub(/:[^:]*$/, ""); print; exit }
   ' "$tmp_dir/links.txt")
   if test -n "$target"
   then printf '%s\n' "$target"
        exit 0
   fi
   sleep 0.1
   tries=$((tries + 1))
done
exit 0
