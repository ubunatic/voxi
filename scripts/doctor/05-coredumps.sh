#!/usr/bin/env bash
# Check 5: recent core dumps of the audio stack (pipewire, pipewire-pulse, wireplumber),
# the GNOME Settings panel and voxi. A crash leaves a core here even when the service
# comes back by itself. Read-only.
# Usage: 05-coredumps.sh [--since TIME]   (default: -7 days)
set -euo pipefail

CHECK=coredumps
source "$(dirname "$0")/lib.sh"
parse_args "$@"
need_cmd coredumpctl

since="$SINCE"
if test -z "$since"
then since="-7 days"
fi

list=$(coredumpctl --no-pager --since "$since" list 2>&1 || true)
if grep -qF 'No coredumps found' <<< "$list"
then verdict_ok "no core dumps of pipewire, wireplumber, gnome-control-center or voxi since $since"
fi
if ! grep -qF 'TIME' <<< "$list"
then verdict_skip "coredumpctl failed: $(head -n 1 <<< "$list")"
fi

hits=$(grep -E ' /[^ ]+/(pipewire|pipewire-pulse|wireplumber|gnome-control-center|voxi) ' <<< "$list" || true)
if test -n "$hits"
then total=$(grep -c . <<< "$hits")
     latest=$(tr -s ' ' <<< "$(tail -n 1 <<< "$hits")")
     exe=$(awk '{ for (i = 1; i <= NF; i++) if ($i ~ /^\//) found = $i } END { print found }' <<< "$latest")
     repair="coredumpctl info $exe  # read the stack trace, then check 1 for crash loops"
     verdict_fault "$repair" "$total core dump(s) of audio or voxi processes since $since, latest: $latest"
fi
verdict_ok "no core dumps of pipewire, wireplumber, gnome-control-center or voxi since $since"
