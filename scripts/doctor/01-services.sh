#!/usr/bin/env bash
# Check 1: the audio services are active, and systemd has not auto-restarted them.
# Read-only. Usage: 01-services.sh [--pw-dump FILE] [--since TIME] (both ignored here)
set -euo pipefail

CHECK=services
source "$(dirname "$0")/lib.sh"
parse_args "$@"
need_cmd systemctl

# check_unit user|system UNIT: records a problem unless UNIT is active with no restarts
check_unit() {
   local scope="$1"
   local unit="$2"
   local ctl=(systemctl)
   local repair="sudo systemctl restart $unit"
   local journal="journalctl -u $unit -b"
   local state restarts
   if test "$scope" = user
   then ctl=(systemctl --user)
        repair="systemctl --user restart $unit"
        journal="journalctl --user -u $unit -b"
   fi
   state=$("${ctl[@]}" is-active "$unit" 2>/dev/null || true)
   if test -z "$state"
   then verdict_skip "cannot ask the systemd $scope manager about $unit"
   fi
   restarts=$("${ctl[@]}" show -p NRestarts --value "$unit" 2>/dev/null || true)
   if test "$state" != active
   then add_problem "$unit is $state" "$repair"
   elif test "${restarts:-0}" -gt 0
   then add_problem "$unit was restarted $restarts time(s) by systemd" "$journal  # look for the crash, see check 5"
   fi
}

check_unit user pipewire.service
check_unit user pipewire-pulse.service
check_unit user wireplumber.service
check_unit system bluetooth.service

if test -n "$problems"
then verdict_fault "$repairs" "$problems"
fi
verdict_ok "pipewire, pipewire-pulse and wireplumber (user) and bluetooth (system) are active, no restarts"
