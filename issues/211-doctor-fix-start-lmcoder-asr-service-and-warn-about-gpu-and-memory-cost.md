# 211 — doctor --fix: start lmcoder ASR service and warn about GPU and memory cost

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 181, 189, 206

---

/goal `voxi doctor --fix` repairs a stopped lmcoder ASR backend by force-loading it (e.g. starting `lmcoder-asr.service`) and prints a clean, colored warning of the cost (GPU/VRAM, RAM, first-run model download, boot-time enable), verified live by stopping the service and running the repair; or stop and report when blocked on a user decision or denied permission. On a loop or hard blocker, clear the goal and write a failure report here.

## 1. Problem & Motivation
On 2026-11-02 dictation failed because `lmcoder-asr.service` (R2T2 on :8749) was stopped and disabled. The lmcoder proxy's `/lmcoder/wake` returned 404, so on-demand start never happened. `voxi doctor --fix --yes` only offered `systemctl --user restart lmcoder-proxy.service`, which cannot help, and the fault returned after the "repair". The owner had to start the service by hand.

## 2. Technical Specification / Findings
- Manual fix that worked: `systemctl --user start lmcoder-asr.service` (and `enable` to survive reboot). First start downloaded ~1.4 GB (Confucius4-R2T2 Q4_K_M + mmproj) and took ~3 min.
- lmcoder's own log gives the cost to show: estimated need ~1.9 GiB (weights 1.0 GiB + mmproj 332 MiB + overhead + KV cache), served on GPU (AMD iGPU/GTT here); spec marks the model `requires_gpu: true`.
- Proxy routes by request `model` field: `model=r2t2` reaches the ASR backend, other names go to the chat model (501). Doctor should probe with the spec's `api_model`.
- Related doctor defect already fixed in 424a2b2: an LFS-pointer probe clip now reports ERROR instead of a false proxy fault.

Open questions:
- Repair via `systemctl --user start` vs `lmcoder load` / a fixed wake route (ticket 181 coordination).
- Whether `--fix` also enables the unit at boot, or offers that as a separate, confirmed step.
- Warning wording/colors: follow existing doctor output style; must stay readable with `NO_COLOR`.

## Addendum: real audio probe
The owner asked that doctor run a real audio file against the ASR. It already tries: it embeds `internal/doctor/short-one-two.wav` and expects "one two". But `*.wav` is in Git LFS, and a checkout without git-lfs embeds the pointer text, so the probe never sent audio (now reported as ERROR since 424a2b2). Make the probe always send real audio: e.g. exempt this clip from LFS in `.gitattributes` (78 KB), or ship it in a non-LFS format, and add a build-time test that the embedded clip is RIFF/WAVE. Verify with `voxi doctor` on a checkout without git-lfs: the probe transcribes "one two".

## 3. Implementation & Verification Plan
- Re-check live code and lmcoder behavior first (wake route may be fixed by then).
- Add a spec-driven repair for "ASR backend stopped" in `spec/doctor.yaml` / `internal/doctor`, shown before applying with the cost warning; unit tests with fakes.
- Live verify per `docs/AudioDoctor.md`: stop `lmcoder-asr.service`, run `voxi doctor --fix`, see the warning, confirm the probe turns OK and a real dictation transcribes.

## Decision
Lean sprint 2026-11-02, host decisions on the developer's plan (d211, haiku:med):
1. Start via `lmcoder service start --asr` (lmcoder CLI, matches internal/frontend), not raw systemctl.
2. Regenerate the probe clip with Piper; new bytes are fine if the live ASR transcribes "one two".
3. Boot-time enable: `see:` hint only, no second prompt.
4. Warning figures come from spec/models.yaml; use the measured 2.78 GiB VRAM, not the 1.9 GiB lmcoder estimate.
5. Live verification may stop `lmcoder-asr` briefly.
6. `test/fixtures/short-abc.wav` is out of scope.

## Milestones
- M1 (real probe clip): .gitattributes exception, real WAV in internal/doctor and test/fixtures, RIFF/WAVE test, live "one two".
- M2 (start-ASR repair): spec template, asr.go branch, fix.go warning + wait, tests incl. negatives.
- M3 (colored warning): listing color helper, NO_COLOR/TTY tests.
- M4 (docs): docs/AudioDoctor.md.
