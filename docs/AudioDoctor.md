# Audio Doctor: Diagnosing and Repairing the Whole Audio Stack

Reference for `voxi doctor` and `internal/doctor` (issue 189). Read this before adding a check or
a repair. The mic-only self-check that runs during recording is in [MicSelfCheck.md](MicSelfCheck.md).

## Why

On 2026-10-10 sound broke after suspend/resume and the GNOME Settings Sound panel hung. Finding
and fixing it took a dozen manual commands across PipeWire, WirePlumber, BlueZ and the journal.
`voxi doctor` turns that session into one command.

## Usage

| Command | Effect |
|---|---|
| `voxi doctor` | Run all checks, print verdicts and repair commands, change nothing |
| `voxi doctor --fix` | Ask y/N per fault on a terminal; refuses to apply from a pipe |
| `voxi doctor --fix --yes` | Apply all repairs |
| `--pw-dump FILE`, `--since T`, `--no-probe` | Diagnose a saved dump, set the journal window, skip the playback probe |

Exit status: 0 all OK/WARN/SKIP, 1 any FAULT, 2 a check errored.

## Design

- **Spec-driven.** `spec/doctor.yaml` (schema `spec/schemas/doctor.schema.json`) holds the check
  ids and order, units, journal signatures, process names, dummy sink names, settle timing and
  repair templates. Multi-step shell lives in `spec/scripts/` (only `routing-probe.sh`). Go holds
  parsing, verdicts and orchestration, never the values (see [Spec.md](Spec.md)).
- **Runtime commands only, no config files.** User decision: WirePlumber/BlueZ config fragments are
  forgotten and have side effects. Repairs are `pw-metadata`, `pactl set-card-profile`,
  `wpctl set-default` and `systemctl --user restart`.
- **Faults vs evidence.** `journal-signals` and `coredumps` report `WARN` with a `see:` hint. Their
  lines stay in the log after a repair, so they can never clear and must not drive repairs.
- **Repair order:** services → Bluetooth card profile → defaults → mic → routing. A repair whose
  effect an earlier one covered (two WirePlumber restarts) is skipped, then re-checked.
- **Snapshot first.** A `pw-dump` is saved to `~/.local/state/voxi/doctor/` before the first
  repair, so a broken state survives as evidence (and as test data). Snapshots are pruned
  according to `spec/doctor.yaml` retention limits (`keep_count`, `max_age_days`).
- **Host boundary.** All commands run through a host interface; tests use fixtures in
  `internal/doctor/testdata/` (anonymised: fake MACs, user and host names).

## Pitfalls (all seen live)

- **`wpctl set-default` does not fix a dangling active default.** It writes only
  `default.configured.audio.sink`. When `default.audio.sink` names a vanished node, WirePlumber
  kept it; only `pw-metadata 0 default.audio.sink '{"name":"<node.name>"}' Spa:String:JSON` helped.
- **No starred sink hangs GNOME.** With a dangling default, `gsd-media-keys` logs
  `Unable to get default sink` and the Sound panel times out (`timeout on stream`). It looks like a
  crash, but there is no core dump.
- **WirePlumber can be "active" and not route.** `systemctl` showed running, 0 restarts, yet new
  streams were never linked (sink SUSPENDED, `pw-play` hangs). Only a playback probe detects this.
- **`auto_null` is not a sink.** Right after a restart the probe links to PipeWire's dummy sink;
  counting that as routed was a false OK.
- **Re-check only on a settled, fresh graph.** After a WirePlumber restart the old graph still
  answers for a moment: defaults missing, no Bluetooth card, no mic. The re-check waits for the
  unit's new MainPID and a changed graph, then takes a fresh `pw-dump` for every check. A fake-clock
  unit test passed while the live run failed: verify restarts live.
- **Bluetooth after resume.** The headset card can end in profile `off` with `a2dp-sink` available
  (`profiles inconsistent (1 < 2)`, `Host is down`). No upstream fix found as of PipeWire 1.6.9 /
  WirePlumber 0.5.18 / BlueZ 5.87; detect and switch the profile.

## Verifying a Change

Unit tests cannot fake WirePlumber timing. Inject each fault live and run `voxi doctor --fix --yes`:

| Fault | Inject with |
|---|---|
| Card off | `pactl set-card-profile bluez_card.<MAC> off` |
| Dangling default | `pw-metadata 0 default.audio.sink '{"name":"bluez_output.DE_AD_00_00_00_00.1"}' Spa:String:JSON` |
| WirePlumber down | `systemctl --user stop wireplumber` (repeat 3 times) |

Every `After repair:` block must show all checks OK; then play a sound.

## Known Limits

- The real "active but not routing" wedge has not been reproduced on demand; stopping WirePlumber
  is the stand-in.
- Only one Bluetooth headset (Mifa A10) and the T14 AMD audio were tested.
