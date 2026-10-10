# Audio doctor checks

Read-only shell checks that diagnose the audio stack (PipeWire, WirePlumber, BlueZ, GNOME
sound settings, voxi). They are the building blocks of a future `voxi doctor` Go command;
the issue for that is not filed yet, and no Go code exists for it.

## Running

```sh
scripts/doctor/run-all.sh                          # all checks, live system (check 6 plays silence)
scripts/doctor/run-all.sh --pw-dump FILE           # checks 2 and 3 against a saved pw-dump
scripts/doctor/run-all.sh --since "2026-10-10 08:00"   # checks 4 and 5 over a time window
scripts/doctor/run-all.sh --no-probe               # all checks, but check 6 does not play anything
scripts/doctor/test.sh                             # fixture tests, need no live audio
```

Each check prints one verdict line and exits with a code:

| Verdict | Exit | Meaning |
|---------|------|---------|
| `OK <check>: ...` | 0 | nothing wrong found |
| `FAULT <check>: ...` | 1 | a problem was found; a `repair (printed, not run):` line follows |
| `SKIP <check>: ...` | 2 | the check could not run (tool missing, service or journal not readable, probe off) |

`run-all.sh` exits 1 if any check faults, 2 if none faults but some were skipped, 0 otherwise.

Every check accepts `--pw-dump FILE`, `--since TIME`, `--no-probe` and `--pw-link FILE`. A check
that does not need an option ignores it. Repair commands are printed for you to read and run
yourself. No check runs a command that changes state.

## The checks

### 01-services.sh: services

- **Detects:** pipewire, pipewire-pulse and wireplumber (user units) or bluetooth (system unit)
  not in state `active`, or restarted automatically by systemd (`NRestarts` > 0).
- **Incident (2026-10-10):** the 09:02 capture showed the three PipeWire services active
  since Oct 9 11:12 with no restarts, so the services were not the cause. The faults were in
  the device and default state (checks 2 and 3).
- **Repair:** `systemctl --user restart <unit>`, or `sudo systemctl restart bluetooth.service`.
  For a restart count, the printed command is the journal of that unit.

### 02-default-devices.sh: dangling defaults

- **Detects:** the PipeWire `default` metadata names a node that does not exist. It checks
  `default.audio.sink` and `default.audio.source` (the active defaults, which applications use)
  and `default.configured.audio.sink` and `default.configured.audio.source` (the preferences).
  An empty active default is a fault; an empty configured value is ignored.
- **Incident (2026-10-10):** `default.audio.sink` named `bluez_output.00_11_22_33_44_55.1`
  (the headset's sink, which exists only while its profile is on), so no sink was starred.
  The desktop Sound panel hung and GNOME's media keys logged `Unable to get default sink`.
  The configured source (a USB webcam) also dropped off the bus at 23:18 and came back at
  08:32; the 09:02 dump had it present again, so this check does not flag it in that dump.
- **Repair for a dangling active default:**
  `pw-metadata 0 default.audio.sink '{"name":"<node.name>"}' Spa:String:JSON`. The name is the
  configured node's when that node exists (in the test dump, the speaker), else the first node
  of that kind. Run this one; `wpctl set-default` does not fix it, because it only moves the
  configured default. Live, that is what fixed the incident.
- **Repair for a configured default that is gone:** `wpctl set-default <ID>`, with the ID of the
  first node of that kind that is present. If no node of that kind is present, the repair is
  `wpctl status`: replug the device, then rerun the check.

### 03-bt-card-profile-off.sh: Bluetooth card in profile off

- **Detects:** a `bluez5` card whose active profile is `off` while at least one profile other
  than `off` is available. A card in `off` with no other available profile is not a fault:
  the headset is probably not connected.
- **Incident (2026-10-10):** after the resume, `bluez_card.00_11_22_33_44_55` was `off`
  while `a2dp-sink` and `headset-head-unit` were available. The headset had no sink or source.
- **Repair:** `pactl set-card-profile bluez_card.<MAC with underscores> a2dp-sink`. If that
  fails with `Host is down`, reconnect the headset first with `bluetoothctl`, then rerun.
  WirePlumber can undo a manual profile change; see `docs/MicSelfCheck.md`.

### 04-journal-signals.sh: log signatures

- **Detects:** lines in the journal that match the failure signatures: `profiles inconsistent`,
  `Unable to get default sink`, `timeout on stream`, `Host is down`, `profile connect failed`,
  `Mic problem`, `card removed`. It also prints the suspend and resume times from systemd-sleep.
- **Window:** by default, from the last resume from sleep, so an incident from before the last
  wake is not reported. If the boot had no resume, the window is the whole boot. `--since TIME`
  overrides it. The output line says which window was used, and the sleep times always cover
  the whole boot.
- **Incident (2026-10-10):** the card reported `profiles inconsistent` from PipeWire, and
  bluetoothd logged `Host is down` and `a2dp-source profile connect failed ... Device or
  resource busy`. The stream timeouts came from GNOME Settings and libcanberra. Voxi logged
  `Mic problem: the mic delivers no audio`.
- **Caveat:** this check reports history inside its window. A FAULT means the signature happened
  in the window, not that it is happening now. Use checks 2, 3 and 6 for the current state.
- **Repair:** fix checks 2 and 3 first; then `systemctl --user restart pipewire pipewire-pulse
  wireplumber`, which drops current audio streams.

### 05-coredumps.sh: core dumps

- **Detects:** core dumps of pipewire, pipewire-pulse, wireplumber, gnome-control-center or
  voxi in the last 7 days (`--since` to change). A crash can be followed by a silent
  automatic restart, so check 1 may show no problem.
- **Incident (2026-10-10):** no core dump was recorded for the Sound panel hang, so this
  check was OK. The hang is explained by check 2 (no default sink) instead.
- **Repair:** `coredumpctl info <EXE>` to read the stack trace.

### 06-stream-routing.sh: new streams not routed

- **Detects:** a new playback stream that is not linked to any sink. The check plays one second
  of silence (volume 0) through a probe stream named `voxi-doctor-probe`, then polls
  `pw-link -l` for up to 2 seconds and looks for an output link from the probe. The probe is
  killed and its temporary files removed on exit, even if the check is interrupted.
- **Incident (2026-10-10):** WirePlumber was active with no restarts, but a new `pw-play` stream
  had no links, the speaker was SUSPENDED, and `pw-play` hung. `systemctl --user restart
  wireplumber` fixed it. The other checks all said OK, so this check is the one that sees it.
- **Repair:** `systemctl --user restart wireplumber`.
- **Side effects:** the probe opens the default sink for about one second and may wake a
  suspended device. It plays nothing audible. `--no-probe` skips it (SKIP), so `run-all.sh`
  stays silent.
- **Test mode:** `--pw-link FILE` parses saved `pw-link -l` text instead of playing a probe.

## Test fixtures

`testdata/`. The values are test values, not the machine's: MACs are `00:11:22:33:44:55`
(and `00_11_22_33_44_55` in names), the webcam serial is `TESTSERIAL`, and the user and host
names are `testuser` and `testhost`.

- `pw-dump-dangling-bt-sink.json`: a dump of the broken state, taken 2026-10-10 09:02, with the
  identifiers replaced. Checks 2 and 3 must FAULT on it.
- `pw-dump-healthy.json`: a trimmed healthy dump. Checks 2 and 3 must OK.
- `pw-dump-bt-off-no-alternative.json`: card `off` with no usable profile. Check 3 must OK.
- `pw-dump-empty-default-sink.json`: empty active default sink. Check 2 must FAULT.
- `pw-dump-configured-source-gone.json`: the configured webcam source is absent. Check 2 must FAULT.
- `pw-dump-configured-sink-gone-hdmi.json`: the configured sink is gone and an HDMI sink is
  present. Check 2 must FAULT and its repairs must use the HDMI sink.
- `pw-link-linked.txt`: `pw-link -l` text where the probe links to the speaker. Check 6 must OK.
- `pw-link-unlinked.txt`: `pw-link -l` text where another app is linked but the probe is not.
  Check 6 must FAULT.

## Feeding `voxi doctor`

Each check is one self-contained verdict with a fixed exit code, so the Go command can run
the same logic and map it to its own output. The `--pw-dump` and `--pw-link` options are the
seams for tests: the Go version should parse the same JSON and text and be tested against the
same fixtures.
