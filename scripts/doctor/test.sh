#!/usr/bin/env bash
# Tests for the audio doctor checks. Fixture-based, so they pass on any machine:
# the dangling-sink dump must FAULT, the healthy fixtures must OK, and missing tools must SKIP.
# Usage: scripts/doctor/test.sh
set -euo pipefail

here=$(dirname "$0")
testdata="$here/testdata"
bash_bin=$(command -v bash)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

passed=0
failed=0

pass() {
   passed=$((passed + 1))
   printf '  ✓ %s\n' "$1"
}

fail() {
   failed=$((failed + 1))
   printf '  ✗ %s\n' "$1"
}

# expect LABEL WANT_EXIT WANT_TEXT COMMAND...: runs COMMAND, checks its exit code and a line of its output
expect() {
   local label="$1" want_rc="$2" want_text="$3"
   shift 3
   local out
   local rc=0
   out=$("$@" 2>&1) || rc=$?
   if test "$rc" -eq "$want_rc" && grep -qF -- "$want_text" <<< "$out"
   then pass "$label"
   else fail "$label: exit $rc, output was: $out"
   fi
}

# Tool sandboxes: PATH with only the named tools, so a missing jq or wpctl is real
mkdir "$tmp/none" "$tmp/nojq" "$tmp/nowpctl"
ln -s "$(command -v dirname)" "$tmp/none/dirname"
ln -s "$(command -v dirname)" "$tmp/nojq/dirname"
ln -s "$(command -v dirname)" "$tmp/nowpctl/dirname"
ln -s "$(command -v jq)" "$tmp/nowpctl/jq"

dangling="$testdata/pw-dump-dangling-bt-sink.json"
healthy="$testdata/pw-dump-healthy.json"
bt_off="$testdata/pw-dump-bt-off-no-alternative.json"
empty_sink="$testdata/pw-dump-empty-default-sink.json"
source_gone="$testdata/pw-dump-configured-source-gone.json"
sink_gone="$testdata/pw-dump-configured-sink-gone-hdmi.json"
printf 'not json\n' > "$tmp/garbage.json"

expect "01 skips without systemctl" 2 "SKIP services" \
   env PATH="$tmp/none" "$bash_bin" "$here/01-services.sh"
expect "02 skips without jq" 2 "SKIP default-devices" \
   env PATH="$tmp/nojq" "$bash_bin" "$here/02-default-devices.sh" --pw-dump "$dangling"
expect "02 skips without wpctl" 2 "SKIP default-devices: wpctl is not installed" \
   env PATH="$tmp/nowpctl" "$bash_bin" "$here/02-default-devices.sh" --pw-dump "$dangling"
expect "03 skips without jq" 2 "SKIP bt-card-profile-off" \
   env PATH="$tmp/nojq" "$bash_bin" "$here/03-bt-card-profile-off.sh" --pw-dump "$dangling"
expect "02 skips on a file that is not pw-dump JSON" 2 "SKIP default-devices" \
   "$here/02-default-devices.sh" --pw-dump "$tmp/garbage.json"

expect "02 FAULT on dangling default sink" 1 "FAULT default-devices: default.audio.sink names 'bluez_output.00_11_22_33_44_55.1'" \
   "$here/02-default-devices.sh" --pw-dump "$dangling"
expect "02 repair points the active default at the configured sink node" 1 "pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink\"}' Spa:String:JSON" \
   "$here/02-default-devices.sh" --pw-dump "$dangling"
dangling_out=$("$here/02-default-devices.sh" --pw-dump "$dangling" 2>&1 || true)
if grep -qF 'wpctl set-default' <<< "$dangling_out"
then fail "02 does not suggest wpctl set-default for a dangling active default"
else pass "02 does not suggest wpctl set-default for a dangling active default"
fi
expect "03 FAULT on bluez card off with a2dp-sink available" 1 "FAULT bt-card-profile-off: bluez_card.00_11_22_33_44_55 is off" \
   "$here/03-bt-card-profile-off.sh" --pw-dump "$dangling"
expect "03 repair switches to the first usable profile" 1 "pactl set-card-profile bluez_card.00_11_22_33_44_55 a2dp-sink" \
   "$here/03-bt-card-profile-off.sh" --pw-dump "$dangling"

expect "02 OK when default points at an existing node" 0 "OK default-devices: default sink 'bluez_output.00_11_22_33_44_55.1'" \
   "$here/02-default-devices.sh" --pw-dump "$healthy"
expect "03 OK when the bluez card has a2dp-sink active" 0 "OK bt-card-profile-off: bluez_card.00_11_22_33_44_55 on profile a2dp-sink" \
   "$here/03-bt-card-profile-off.sh" --pw-dump "$healthy"
expect "03 OK, not a fault, when off has no usable alternative" 0 "not a fault" \
   "$here/03-bt-card-profile-off.sh" --pw-dump "$bt_off"
expect "02 FAULT on an empty active default, ignores empty configured" 1 "default.audio.sink is empty" \
   "$here/02-default-devices.sh" --pw-dump "$empty_sink"
expect "02 FAULT when the configured source (webcam) is gone" 1 "default.configured.audio.source names 'alsa_input.usb-SunplusIT" \
   "$here/02-default-devices.sh" --pw-dump "$source_gone"

expect "02 repair falls back to the first sink when the configured one is gone" 1 "pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.1.HiFi__HDMI1__sink\"}' Spa:String:JSON" \
   "$here/02-default-devices.sh" --pw-dump "$sink_gone"
expect "02 suggests wpctl set-default for the gone configured sink, using the fallback ID" 1 "wpctl set-default 77" \
   "$here/02-default-devices.sh" --pw-dump "$sink_gone"

expect "04 --since sets the window, and the output says so" 0 "in the log since 2099-01-01" \
   "$here/04-journal-signals.sh" --since 2099-01-01

expect "06 SKIP without pw-play" 2 "SKIP stream-routing: pw-play is not installed" \
   env PATH="$tmp/none" "$bash_bin" "$here/06-stream-routing.sh"
expect "06 SKIP with --no-probe, so no audio is played" 2 "SKIP stream-routing: --no-probe given" \
   "$here/06-stream-routing.sh" --no-probe
expect "06 OK when the probe's output is linked to a sink" 0 "OK stream-routing: probe stream linked to sink 'alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink'" \
   "$here/06-stream-routing.sh" --pw-link "$testdata/pw-link-linked.txt"
expect "06 FAULT when the probe has no output link, though another stream is linked" 1 "FAULT stream-routing: a new playback stream was not linked to any sink" \
   "$here/06-stream-routing.sh" --pw-link "$testdata/pw-link-unlinked.txt"
expect "06 repair restarts WirePlumber" 1 "repair (printed, not run): systemctl --user restart wireplumber" \
   "$here/06-stream-routing.sh" --pw-link "$testdata/pw-link-unlinked.txt"

expect "run-all FAULTs on the dangling dump" 1 "FAULT default-devices" \
   "$here/run-all.sh" --pw-dump "$dangling" --no-probe
expect "run-all prints a summary" 1 "Summary:" \
   "$here/run-all.sh" --pw-dump "$dangling" --no-probe
expect "run-all passes --no-probe through to check 6" 1 "SKIP stream-routing: --no-probe given" \
   "$here/run-all.sh" --pw-dump "$dangling" --no-probe

# Read-only guard: every line that names a state-changing command must only print it (printf) or
# store it as a repair string, never run it
bad=$(grep -nE 'wpctl set-|pw-metadata [0-9]|pactl set-|bluetoothctl connect|systemctl .*restart' "$here/lib.sh" "$here"/0*.sh "$here/run-all.sh" | grep -vE 'printf|repair' || true)
if test -z "$bad"
then pass "scripts only print state-changing commands, never run them"
else fail "state-changing command outside printf or repair: $bad"
fi

printf '\n%s passed, %s failed\n' "$passed" "$failed"
if test "$failed" -gt 0
then exit 1
fi
