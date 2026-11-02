---
kind: sprint
date: 2026-11-03
issue: 211, 212, 210, 214, 213
---

# Two-Agent Lean Sprint: ASR Doctor, Bench Hang, Bluetooth Headset Dictation

Host: Opus 5.5 (decide mode). Developer: haiku:med (d211, d212, d210). Second reviewer: opus:med (r211, r212, r210). Searcher skipped on every ticket; plan-first developer turns replaced it.

## Tickets and outcome

| Ticket | Delivered | Commits | Live evidence |
|---|---|---|---|
| 211 doctor starts lmcoder ASR | real probe clip without LFS; start repair with colored cost notice; wake on record start; spec probe timeout | 57a650d, 8cefaf4, b04cca3, d2d8ed6, 9facd22, b8888c5 | stopped lmcoder-asr → `voxi doctor` probe OK in 1.7 s (lmcoder 274 on-demand start); owner dictation OK |
| 212 bench test hang | sparse fixtures at the 1 GiB GGUF floor, injected HTTP client, TestMain network guard | 64a17f2 | internal/bench 0.13 s instead of the 10 m timeout |
| 210 BT headset dictation | BT predicate, digital gain 2.75 (=1.4³), `voxi mic headset` keep-awake with ownership/undo, `bluetooth_mic_gain` setting | 3715e41, efdc914, 44b5b7d, ae2aed6, 3c02fcd | source volume 1.0, soft sentence after a 30 s pause transcribed in full (mean RMS 125) |
| 214 monitor meter | meter applies the dictation gain, `×2.75` label | 92030f8 | owner: "mic gain meter looks good" |
| 213 BT spec literals | device_api/node_prefix/HFP profile from spec | 5e39469 | `voxi mic` reports the headset unchanged |

Parked: 215 (energy gate literals to spec).

## What the reviews caught
- r211: hints for FAULT results never printed; `--fix` could block silently for ~15 min; the 5 min budget was not wall-clock; no early exit on a failed unit. All fixed in M4 pre-work.
- r210: the developer's gain of 6 came from comparing different utterances. wpctl volume is cubic, so 1.4 equals 2.74× amplitude. At 6× noise passed the gate and speech clipped. Other catches: `--off` would re-enable the owner's manual autoswitch setting, and the ownership marker was weak.
- r212: no bug found; it pointed out that the test-q1 evidence must be recorded before closing.

## Cross-session work
The lmcoder session took the host-agnostic half (issue 274, v0.1.15): the proxy starts ASR on demand, `/lmcoder/wake`, a 400 for a non-ASR model, a 503 when ASR is excluded. voxi dropped the plan for its own systemd management and kept the start repair only as a fallback.

## Lessons
- Haiku delivered every milestone with green tests. The Opus review was needed for the reasoning errors (gain derivation, unbounded waits), not for syntax.
- Asking the developer for a final `TEST RESULT` section fixed the truncated-report problem from `tail -25`.
- git-lfs missing on the host silently embedded pointer files (probe clip and notify sound). Clip exempted from LFS; git-lfs now installed.
