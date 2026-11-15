#!/usr/bin/env bash
# Canary for issue 125: can a process in the systemd --user context pause and
# resume MPRIS media players over the session D-Bus, and how fast does a player
# report the new state?
#
# Run it in the same context as voxi-agent.service:
#   systemd-run --user --wait --pipe --collect scripts/canary-media-pause.sh
#
# It pauses every player that reports "Playing", waits for "Paused", then sends
# Play and waits for "Playing" again. Players that are not playing are only read.
# Start some playback first (a browser tab is enough).
set -euo pipefail

prefix=org.mpris.MediaPlayer2.
path=/org/mpris/MediaPlayer2
player_if=org.mpris.MediaPlayer2.Player
max_polls=200

now_ms() {
   printf '%s\n' "$(($(date +%s%N) / 1000000))"
}

status_of() {
   busctl --user get-property "$1" "$path" "$player_if" PlaybackStatus 2>/dev/null |
   cut -d'"' -f2
}

owner_of() {
   busctl --user call org.freedesktop.DBus /org/freedesktop/DBus \
          org.freedesktop.DBus GetNameOwner s "$1" 2>/dev/null |
   cut -d'"' -f2
}

# wait_for OWNER WANT prints the milliseconds until the player reports WANT.
wait_for() {
   local owner="$1"
   local want="$2"
   local start
   local polls=0
   start=$(now_ms)
   while test "$(status_of "$owner")" != "$want"
   do polls=$((polls + 1))
      if test "$polls" -ge "$max_polls"
      then printf 'never (gave up after %s polls)\n' "$polls"
           return 1
      fi
   done
   printf '%s ms\n' "$(($(now_ms) - start))"
}

printf 'Session bus: %s\n' "${DBUS_SESSION_BUS_ADDRESS:-unset (busctl falls back to XDG_RUNTIME_DIR)}"

list_start=$(now_ms)
names=$(busctl --user list --no-legend |
        awk -v p="$prefix" 'index($1, p) == 1 && $2 != "-" {print $1}')
printf 'Listing bus names took %s ms\n' "$(($(now_ms) - list_start))"

if test -z "$names"
then printf 'No running MPRIS players found.\n'
     exit 0
fi

paused=""
for name in $names
do owner=$(owner_of "$name")
   status=$(status_of "$name")
   printf '%-50s owner=%-12s status=%s\n' "$name" "$owner" "${status:-unreadable}"
   if test "$status" = "Playing"
   then call_start=$(now_ms)
        busctl --user call "$owner" "$path" "$player_if" Pause
        printf '  Pause call returned after %s ms; reports Paused after ' "$(($(now_ms) - call_start))"
        wait_for "$owner" Paused || true
        paused="$paused $owner"
   fi
done

if test -z "$paused"
then printf 'Nothing was playing, so nothing was paused. Start playback and run again.\n'
     exit 0
fi

for owner in $paused
do call_start=$(now_ms)
   busctl --user call "$owner" "$path" "$player_if" Play
   printf '%s: Play call returned after %s ms; reports Playing after ' "$owner" "$(($(now_ms) - call_start))"
   wait_for "$owner" Playing || true
done
