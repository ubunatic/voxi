#!/usr/bin/env bash
# Check 3: a Bluetooth audio card is in profile 'off' while a usable profile is available.
# This is the state after a suspend/resume where the headset's profile was not restored:
# no sink or source exists for the headset. Read-only; the repair is printed, never run.
# Usage: 03-bt-card-profile-off.sh [--pw-dump FILE] [--since TIME] (--since ignored here)
set -euo pipefail

CHECK=bt-card-profile-off
source "$(dirname "$0")/lib.sh"
parse_args "$@"
need_cmd jq wpctl
load_pw_dump

# one line per Bluetooth card: name|active profile|usable profiles (comma list, may be empty)
cards=$(jq -r '.[] | select(.type == "PipeWire:Interface:Device" and .info.props["device.api"] == "bluez5") | "\(.info.props["device.name"])|\(.info.params.Profile[0].name? // "none")|\([.info.params.EnumProfile[]? | select(.name != "off" and .available != "no") | .name] | join(","))"' <<< "$PW_JSON") || verdict_skip "cannot read the device list"

if test -z "$cards"
then verdict_ok "no Bluetooth audio card present, nothing to check"
fi

notes=""
while IFS='|' read -r card active usable
do if test "$active" = off && test -n "$usable"
   then profile="${usable%%,*}"
        repair="pactl set-card-profile $card $profile"
        add_problem "$card is off, but profiles are available: $usable" "$repair"
   elif test "$active" = off
   then notes="${notes:+$notes, }$card is off with no usable profile, not a fault"
   else notes="${notes:+$notes, }$card on profile $active"
   fi
done <<< "$cards"

if test -n "$problems"
then verdict_fault "$repairs" "$problems"
fi
verdict_ok "$notes"
