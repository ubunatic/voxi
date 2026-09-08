# 087 — `voxi monitor` lights up GNOME mic-in-use indicator even while idle (suppression tags don't work / unimplemented)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Bug (privacy-indicator leak)
**Category**: Bug / Privacy
**Related**: [084 Add Live Mic Input-Level Meter and Volume Display to `voxi monitor`](084-add-live-mic-input-level-meter-and-volume-display-to-voxi-monitor.md), [086 Detailed view: always show live mic loudness](086-detailed-view-always-show-live-mic-loudness-even-when-not-recording.md), [audiolevel/audiolevel.go](../audiolevel/audiolevel.go), [internal/monitor/monitor.go](../internal/monitor/monitor.go), harnez `docs/MicIndicators.md`, harnez issues 248 (closed, research), 251 (open, unimplemented), 253 (open, unimplemented)

---

## 1. Problem

Whenever `voxi monitor -w` is running, GNOME Shell's top-bar microphone-in-use
(privacy) indicator lights up — even while voxi is genuinely idle, not
dictating. The user does not want the indicator to appear unless voxi is
actually capturing speech for transcription. The same symptom was
independently observed in the sibling project `harnez`'s own live mic-level
meter, which this feature was ported from; the user reports the harnez tree
"still shows the wrong icon" despite documented suppression research.

Root cause: 084 wired a background live-loudness meter
(`audiolevel.StartManager` in `internal/monitor/monitor.go:237`) that runs
for the **entire life** of `voxi monitor -w`, not just while actually
recording/dictating — so any indicator the capture stream triggers is lit
continuously, not just during real use.

## 2. Current state of the suppression mechanism

`audiolevel/audiolevel.go` (ported from harnez's `internal/usage/miclive.go`)
implements an "exemption tag" strategy documented in harnez's
`docs/MicIndicators.md`:

- `ParecCommand()` (used when `parec` is on `PATH`) tags the stream with
  `--property=application.id=org.gnome.VolumeControl` and
  `--property=node.virtual=true`.
  - The GNOME tag is **[CONFIRMED]** by a live canary in harnez issue 248
    (Fedora GNOME, 2026-09-05): GNOME Shell's `js/ui/status/volume.js`
    (`InputStreamSlider._maybeShowInput()`) hardcodes a `skippedApps` allowlist
    containing exactly `org.gnome.VolumeControl` and `org.PulseAudio.pavucontrol`,
    and only checks `application.id` — nothing else.
  - The KDE tag (`node.virtual=true`, matched by `plasma-pa`'s
    `microphoneindicator.cpp`) is only **[INFERRED]**, never canary-confirmed
    live on a Plasma 6 session.
- `PwRecordCommand()` (fallback used when `parec`/`pulseaudio-utils` are
  absent, only `pw-record` is on `PATH`) applies **no suppression tags at
  all** — the doc comment says explicitly "no indicator-suppression tags are
  applied here: harnez's research did not confirm an equivalent property
  mechanism for pw-record's compact CLI."

**On this machine** (`command -v parec` → not found; `command -v pw-record`
→ `/usr/bin/pw-record`), `internal/monitor/monitor.go`'s
`buildMicCaptureCmd` falls back to `PwRecordCommand` — i.e. voxi is
currently running the **untagged, unsuppressed** capture path here. That
alone is sufficient to explain the indicator firing continuously on this
box, independent of whether the GNOME `application.id` tag itself still
works.

Desktop/session details on this machine: GNOME Shell 50.1
(Ubuntu 26.04, `gnome-shell-ubuntu-extensions` patched), Wayland session.
GNOME Shell version has advanced since harnez's issue-248 canary
(2026-09-05); no re-confirmation has been done that the `skippedApps`
allowlist in `volume.js` is still intact in 50.1, and Ubuntu is known to
patch gnome-shell for other features (harnez's `MicIndicators.md` §2.4
already flags "no confirmed difference identified" as an open question, not
a verified non-issue).

## 3. harnez precedent — the tag strategy is documented but was never shipped

Checked `~/projects/harnez` issue tracker (`harnez find -d ~/projects/harnez
issues`):

- **248** (Closed) — the *research* ticket; confirmed the GNOME
  `application.id=org.gnome.VolumeControl` exemption works via a live canary
  script (`scripts/canary-gnome-mic-indicator.sh`), and produced
  `docs/MicIndicators.md`.
- **250** (In Progress) — broader survey ticket; explicitly notes Xfce and
  KDE `node.virtual` canaries are "still pending."
- **251** (**Open**) — the ticket to actually *implement* the tag injection
  in harnez's production `internal/usage/miclive.go` and to make
  `mic.go`'s `Recording` heuristic exempt-aware. **Not done.**
- **253** (**Open**) — filed after 251, because the indicator was observed
  still lighting up when toggling harnez's Mic panel (preset `8`/`m`) in
  practice; proposes evaluating a decoupled watcher/daemon architecture as
  the tag approach may not be sufficient (portal/cgroup-based attribution,
  not just stream properties).

So the user's read is correct: the suppression *design* was researched and
confirmed to work in isolation (a bare canary script), but was **never
actually wired into harnez's shipped live-meter code path** — 251 sitting
open is exactly why "the harness tree still shows the wrong icon." voxi's
`audiolevel` package *did* carry the tags into its own `ParecCommand`
(unlike harnez, which never got that far), but:
1. voxi never ran that path live end-to-end and confirmed the indicator
   actually stays dark on this machine's GNOME 50.1 — 084's own ticket left
   this as an open question.
2. On any machine without `parec` (like this one), voxi silently falls back
   to the fully-untagged `pw-record` path, so the tags do nothing.
3. Even where tags apply, harnez's own issue 253 raises the possibility that
   stream-property tagging alone isn't reliable (portal/session/cgroup
   attribution could bypass it) — unconfirmed either way for voxi.

## 4. Non-goals for this ticket

This ticket is a bug report to establish ground truth and lay out
investigation options — it does not implement a fix.

## 5. Suggested next steps

1. **Live-canary voxi's actual GNOME behavior** on this machine: run
   `voxi monitor -w`, then `pactl list source-outputs` (need
   `pulseaudio-utils` installed, or `pw-cli`/`wpctl` equivalents), and
   observe the top-bar indicator directly, once with the `pw-record`
   fallback (current default here) and once with `parec` installed to force
   the tagged path — isolates "tags don't work on GNOME 50.1" from "tags
   were simply never active on this box."
2. **Check whether GNOME Shell 50.1's `volume.js` still has the
   `skippedApps` allowlist** harnez's issue 248 canary relied on — grep the
   installed `gnome-shell` JS resources
   (`/usr/share/gnome-shell/js/ui/status/volume.js` or its packed
   `resource:///` bundle) for `org.gnome.VolumeControl` /
   `skippedApps`, since Ubuntu is known to carry shell patches and the
   canary predates this GNOME version.
3. **Add suppression tags to `PwRecordCommand`** if/when an equivalent
   pw-record property mechanism is found (harnez's research explicitly
   left this unconfirmed, not "confirmed absent") — otherwise `pw-record`-only
   machines (this one included) can never suppress the indicator regardless
   of any GNOME-side fix.
4. **Reduce exposure time as a mitigation independent of tag reliability**:
   only start/keep the mic-capture subprocess running while it's actually
   needed (e.g. only while the loudness section of the TUI is visible,
   and/or only while near dictation-active, rather than for the entire life
   of `voxi monitor -w`) — this bounds the privacy-indicator false-positive
   window even if suppression tags never work on some desktop/version
   combination. Note this overlaps with 086 (which wants the *opposite*:
   loudness visible more) — any fix here should be reconciled with 086
   rather than contradicting it outright.
5. **Track harnez 253's daemon/watcher architecture evaluation** — if
   harnez concludes stream-property tagging is fundamentally unreliable
   (e.g. due to portal/cgroup attribution), that finding applies equally to
   voxi's `audiolevel` package since both share the same tagging strategy.

## 6. Verification

Not yet performed (this ticket documents current-state investigation only).
Any fix here should be verified with a live GNOME-session canary (indicator
observed dark while `voxi monitor -w`'s meter runs, then observed lit when a
genuine concurrent recording app starts) before being marked resolved — a
passing `go test ./...` alone is not sufficient evidence per this project's
`AgenticLoop.md` "Unit-Test-Only Confidence for Hook/Environment Features"
anti-pattern.

## 7. Update — 2026-09-08: item 3 done, item 2 corroborated, item 1 still open

- **Item 2 corroborated independently**: fetched
  `https://gitlab.gnome.org/GNOME/gnome-shell/-/raw/main/js/ui/status/volume.js`
  directly (not the Ubuntu-patched local resource bundle, but current
  upstream `main`). `InputStreamSlider._maybeShowInput()` still contains
  exactly the `skippedApps` allowlist described in §2/§3 above:
  `org.gnome.VolumeControl` and `org.PulseAudio.pavucontrol`, matched
  against `application.id` only. So the tag strategy itself remains valid
  upstream; whether Ubuntu 26.04's shell patches on this specific machine
  preserve it unmodified is still unconfirmed (would need the local
  `/usr/share/gnome-shell` resource bundle diffed against this, not done).
- **Item 3 done**: `PwRecordCommand` (`audiolevel/audiolevel.go`) now passes
  `-P '{ application.id = "org.gnome.VolumeControl" node.virtual = true }'`
  — `pw-record` 1.6.2 supports `-P`/`--properties` (confirmed via
  `pw-record --help` on this machine), closing the "untagged pw-record
  fallback" gap that was the concretely-confirmed cause of the leak here.
  New test: `TestPwRecordCommandCarriesSuppressionTags`
  (`audiolevel/audiolevel_test.go`).
- **Live-verified at the PipeWire level** (not yet at the GNOME-indicator
  level — see below): built and ran the actual `voxi monitor -w` binary on
  this machine (only `pw-record` on PATH, confirming the previously-broken
  path is the one actually exercised here) and inspected the running
  capture node via `pw-dump`:
  ```
  node.name: pw-record
  application.id: org.gnome.VolumeControl
  node.virtual: True
  media.class: Stream/Input/Audio
  ```
  Confirms the properties genuinely land on the live PipeWire node, not
  just that the CLI args are well-formed.
- **Item 1 (full GNOME-session visual canary) still not done** — this
  session has no access to observe the actual GNOME Shell top-bar UI, so
  "does the indicator actually stay dark now" remains unverified per this
  ticket's own §6 standard. Do not close this ticket on the strength of the
  `pw-dump` evidence alone; a human needs to actually watch the top bar
  while `voxi monitor -w` runs (idle) and confirm no privacy dot appears,
  then confirm it *does* appear during genuine dictation.
