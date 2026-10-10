# 189 — Add voxi doctor command with repair wizard for audio stack faults

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 177, scripts/doctor/

---

## 1. Problem & Motivation
On 2026-10-10 the sound stack broke after suspend/resume and the GNOME Settings Sound panel hung.
Diagnosis and repair took several manual steps. `voxi mic` only covers the mic source side.
The user wants one command, `voxi doctor`, that finds these faults and fixes them by running
commands, optionally through a small wizard. **No config files**: WirePlumber/BlueZ config
fragments get forgotten and have side effects, so repairs are runtime commands only.

Incident, in order:
1. BT headset (Mifa A10) card ended in profile `off` with `a2dp-sink` available
   (PipeWire: `profiles inconsistent (1 < 2)`; bluetoothd: `Host is down`, `profile connect failed`).
2. `default.audio.sink` in the `default` metadata kept naming the vanished `bluez_output.*` node.
   No sink starred in `wpctl status`; `gsd-media-keys`: `Unable to get default sink`;
   pipewire-pulse: `[GNOME Settings] timeout on stream`. No coredump: the panel hung, it did not crash.
   `wpctl set-default` only changed `default.configured.audio.sink`; the fix was
   `pw-metadata 0 default.audio.sink '{"name":"<node.name>"}' Spa:String:JSON`.
3. Afterwards WirePlumber stayed `active` (0 restarts) but no longer linked new playback streams
   (pw-play hung, sink SUSPENDED). `systemctl --user restart wireplumber` fixed it.

Web research (2026-10-10, PipeWire 1.6.9, WirePlumber 0.5.18, BlueZ 5.87) found no upstream fix
for any of the three; the recommended approach is detect-then-repair, not blanket restarts.

## 2. Technical Specification / Findings
Prototype: shell checks in `scripts/doctor/` (README there), each printing
`OK|FAULT|SKIP <check>: <explanation>` plus a printed repair command:
- 01 services, 02 dangling default sink/source, 03 BT card profile off,
  04 journal signals since last resume, 05 coredumps, 06 stream routing probe (silent playback).
- Fixtures: `scripts/doctor/testdata/` (anonymised pw-dump of the broken state).

Port these to Go (reuse `internal/mic` state reading and `mic.Diagnose` where they overlap),
ordered so earlier repairs are applied first: services → card profile → defaults → routing.

## 3. Implementation & Verification Plan
- `voxi doctor`: run all checks, print verdicts; exit non-zero on a fault.
- `voxi doctor --fix`: wizard: for each fault show the command and ask y/N; `--yes` applies all.
  After each repair re-run that check and report whether it cleared.
- Save a `pw-dump` of the broken state before the first repair (path printed).
- Tests on the fixtures; repairs behind an interface so tests never touch the live stack.
- Man page and docs/MicSelfCheck.md updated; scripts/doctor/ removed or kept as reference
  (search callers first).

### Spec placement (user decision 2026-10-10)
Doctor data lives in the spec, not in Go (docs/Spec.md: YAML + JSON Schema, embedded, no shadowing):
- `spec/doctor.yaml` + `spec/schemas/doctor.schema.json` + `spec/doctor.go` loader: check ids,
  titles, order, units to check, journal signal patterns, coredump process names, repair command
  templates (e.g. the `pw-metadata` and `pactl set-card-profile` lines), wizard prompts.
- `spec/scripts/*.sh` (embedded) for bigger multi-step shell snippets, only where shell is clearer
  than Go; one-line commands stay in the YAML.

### M1 — read-only `voxi doctor` from the spec
- Spec files above; `internal/doctor` runs the six checks of `scripts/doctor/` with the same
  verdicts (OK/FAULT/SKIP, printed repair, exit 0/1/2 semantics) and `--pw-dump`, `--since`,
  `--no-probe` flags. Routing probe plays silence only.
- Tests on the existing fixtures (move them to `internal/doctor/testdata/`, keep anonymised);
  edge cases from `scripts/doctor/test.sh` all covered in Go.
- Commit `feat(doctor): ... (issue 189 M1)`.

### M2 — repair wizard
- `--fix` asks y/N per fault, `--yes` applies all; order services → card profile → defaults → routing;
  `pw-dump` snapshot before the first repair (path printed); re-run the check after each repair.
- Repairs behind an interface; tests never touch the live stack.
- Man page, `docs/MicSelfCheck.md`; remove `scripts/doctor/` after searching callers.
- Close: `harnez issues close 189 "<real outcome>"`.
