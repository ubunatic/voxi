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
| `--color auto\|always\|never` | Color status words and the cost warning. `auto` colors only when stdout is a terminal. `NO_COLOR` (any non-empty value) turns color off even for `always` |

Exit status: 0 all OK/WARN/SKIP, 1 any FAULT, 2 a check errored.

Plain output (no color) is the default for pipes and for `NO_COLOR`; the text is otherwise identical.

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
- **Repair order:** services → Bluetooth card profile → defaults → mic → routing → ASR engine. A repair whose
  effect an earlier one covered (two WirePlumber restarts) is skipped, then re-checked.
- **ASR diagnostics (issues 197, 206).** Doctor identifies the running eager service where possible and
  transcribes an embedded test clip through its OpenAI-compatible endpoint. A working failover is a WARN;
  an unusable path is a FAULT. A listening proxy or successful registry lookup alone does not prove ASR
  works. A successful clip probe does not verify the microphone, desktop typing, or which backend the proxy
  selected. For `crispasr`/`voxtype`, binary presence alone produces WARN (transcription unverified).
  Doctor does not automatically switch to an unverified fallback; review proposed repairs before applying.
  The agent does not automatically revert a fallback based on an open TCP port.
- **Starting a stopped ASR service (issue 211).** When the lmcoder endpoint fails and its unit
  (`local_service.unit` in `spec/models.yaml`, `lmcoder-asr.service`) is `inactive` or `failed`, the
  repair is `lmcoder service start --asr` (`start_asr` in `spec/doctor.yaml`). A proxy restart cannot
  start a stopped unit, so it is not offered there. Other states:
  - **running:** the proxy restart, as before.
  - **not loaded** (`LoadState` other than `loaded`): guidance only; doctor has no unit to start.
  - **`activating`:** a start is in progress. Doctor does not restart the proxy, because that would
    interrupt the model load; it says to wait and run doctor again.
  - **`lmcoder` not on PATH:** guidance only.
- **Cost warning before the prompt.** A start prints its cost before the y/N prompt, and under
  `--yes`: GPU VRAM while it serves, resident RAM, the first-start download (worded "if the model
  files are not cached") and the first-start time limit. All figures come from `local_service` in
  `spec/models.yaml`. The boot hint (`systemctl --user enable`) is printed only when the unit is not
  already enabled, and doctor never runs it.
- **Waiting for the start.** After the start, doctor re-probes every 10 s until the probe transcribes,
  the unit reaches `failed` (the wait then ends at once and names the unit), or the budget
  (`first_start_seconds`) passes. The budget is wall-clock, so probe time counts against it. Each
  poll prints a progress line, and the start command is bounded by the same budget.
- **The probe clip is real audio.** `internal/doctor/short-one-two.wav` is a Piper rendering of
  "One, two." (voice `en_US-lessac-medium`, 22050 Hz mono 16-bit WAV). The copy in `test/fixtures/`
  must stay byte-identical. `.gitattributes` keeps both out of Git LFS, so a checkout without
  git-lfs embeds audio, not a pointer file. `TestEmbeddedProbeClipIsRealAudio` checks the RIFF/WAVE
  header and the byte equality. To regenerate the clip:

  ```sh
  echo "One, two." | piper -m ~/.local/share/voxi/voices/en_US-lessac-medium.onnx -f internal/doctor/short-one-two.wav
  cp internal/doctor/short-one-two.wav test/fixtures/short-one-two.wav
  ```

  Then check it live: `voxi doctor` must report `asr-engine` OK with the probe transcript (the live
  proxy transcribes "one two"). A new clip is fine as long as it transcribes that way.
- **Snapshot first.** A `pw-dump` is saved to `~/.local/state/voxi/doctor/` before the first
  repair, so a broken state survives as evidence (and as test data). Snapshots are pruned
  according to `spec/doctor.yaml` retention limits (`keep_count`, `max_age_days`).
- **Host boundary.** All commands run through a host interface; tests use fixtures in
  `internal/doctor/testdata/` (anonymised: fake MACs, user and host names).

## Pitfalls (all seen live)

- **`pw-record` stdout is not necessarily raw PCM.** Without `--raw`, this host emitted an AU
  stream whose header and big-endian samples were interpreted as little-endian PCM, producing noise
  and empty transcripts despite a healthy microphone. Eager capture now requests `--raw`; check
  recorded chunk playback, not just device health (issue 207).

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

Unit tests cannot fake WirePlumber timing. In a maintenance window, record service and device state,
announce interruptions, inject faults deliberately, and review `voxi doctor --fix` repairs before applying.
Restore the prior state even if a check fails; do not blindly apply `--yes`:

| Fault | Inject with |
|---|---|
| Card off | `pactl set-card-profile bluez_card.<MAC> off` |
| Dangling default | `pw-metadata 0 default.audio.sink '{"name":"bluez_output.DE_AD_00_00_00_00.1"}' Spa:String:JSON` |
| WirePlumber down | `systemctl --user stop wireplumber` (repeat 3 times) |
| ASR endpoint down | `systemctl --user stop lmcoder-proxy.service` |
| ASR service stopped | `systemctl --user stop lmcoder-asr.service`, then `voxi doctor --fix`: expect the cost warning, `lmcoder service start --asr`, and `asr-engine` OK |

After repair, verify the relevant fault cleared; historical journal warnings may remain. Play a sound
for audio routing, and for ASR repeat real dictation through capture, transcription and typing. See
[DictationQARunbook.md](DictationQARunbook.md) for the paired procedure.

## Known Limits

- The real "active but not routing" wedge has not been reproduced on demand; stopping WirePlumber
  is the stand-in.
- Only one Bluetooth headset (Mifa A10) and the T14 AMD audio were tested.
- The RAM figure (2.0 GiB) is the unit's `MemoryCurrent` after one start. It is cgroup memory and can
  include page cache. VRAM (2.78 GiB) was measured once. The download line cannot tell whether model
  files are cached, so it is worded as conditional.
