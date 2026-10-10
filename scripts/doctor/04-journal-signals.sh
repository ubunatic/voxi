#!/usr/bin/env bash
# Check 4: the journal holds the log lines that came with the audio failure seen on 2026-10-10
# (profiles inconsistent, default sink unavailable, stream timeouts, Bluetooth host down,
# mic problem), and it shows when the machine suspended and resumed. Read-only.
# Usage: 04-journal-signals.sh [--since TIME]   (default: since the last resume, else since boot)
set -euo pipefail

CHECK=journal-signals
source "$(dirname "$0")/lib.sh"
parse_args "$@"
need_cmd journalctl

# the window starts at the last resume, so an incident from before the last wake is not reported
if test -n "$SINCE"
then window=(--since "$SINCE")
     window_text="since $SINCE"
else resume=$(journalctl -t systemd-sleep -b --no-pager -o short-iso 2>/dev/null |
              grep 'System returned from sleep operation' |
              tail -n 1 | cut -d' ' -f1 || true)
     if test -n "$resume"
     then window=(--since "$resume")
          window_text="since the last resume at $resume"
     else window=(-b)
          window_text="since boot (no resume from sleep in this boot)"
     fi
fi

user_log=$(journalctl --user "${window[@]}" --no-pager -o short-iso 2>/dev/null) || verdict_skip "cannot read the user journal"
system_log=$(journalctl "${window[@]}" --no-pager -o short-iso 2>/dev/null) || verdict_skip "cannot read the system journal"

# the user and system journals both hold the user's lines, so sort -u drops the doubled ones
signals='profiles inconsistent|Unable to get default sink|timeout on stream|Host is down|profile connect failed|Mic problem|card removed'
hits=$(printf '%s\n%s\n' "$user_log" "$system_log" | grep -E "$signals" | sort -u || true)

summary=""
for signal in 'profiles inconsistent' 'Unable to get default sink' 'timeout on stream' 'Host is down' 'profile connect failed' 'Mic problem' 'card removed'
do count=$(grep -cF -- "$signal" <<< "$hits" || true)
   if test "$count" -gt 0
   then summary="${summary:+$summary, }'$signal' x$count"
   fi
done

# the sleep marks always cover this boot, so the output shows the sleeps before the window too
sleep_marks=$(journalctl -t systemd-sleep -b --no-pager -o short-iso 2>/dev/null |
              grep -E 'Performing sleep operation|System returned from sleep operation' |
              tail -n 4 |
              awk '/Performing/ { print "suspend at " $1 } /returned/ { print "resume at " $1 }' |
              paste -sd, - | sed 's/,/, /g' || true)

if test -n "$hits"
then total=$(grep -c . <<< "$hits")
     repair="systemctl --user restart pipewire pipewire-pulse wireplumber  # only after checks 2 and 3 are fixed; drops audio streams"
     verdict_fault "$repair" "$total matching log line(s) $window_text: $summary; sleep: ${sleep_marks:-none}"
fi
verdict_ok "no audio failure signature in the log $window_text; sleep: ${sleep_marks:-none}"
