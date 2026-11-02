---
kind: story
date: 2026-11-03
---

# Cross-Session Agentic Coding: Files-First, Fast Go, Cheap Developers

## 1. Header & Context
- **Date**: 2026-11-03. **Scope**: voxi, plus the lmcoder session working alongside it.
- **Previous story**: [2026-10-22 Robust ASR Fallbacks, Doctor Repairs, and Offline Upstream Resilience](2026-10-22-robust-asr-fallbacks-doctor-repairs-and-offline-upstream-resilience.md). Baseline commit `90edfc8` (2026-10-26), the last commit that touched it.
- **Sub-studies summarised**: [2026-10-26 lean sprint, eager /tmp cleanup](2026-10-26-two-agent-lean-sprint-eager-tmp-cleanup.md) and [2026-11-03 lean sprint, ASR doctor and BT headset](2026-11-03-two-agent-lean-sprint-asr-doctor-and-bt-headset.md).
- **Starting state**: dictation was dead on um760. lmcoder-asr was stopped and disabled, the doctor offered a repair that could not help, the Bluetooth headset mic was too quiet and lost first words, and an untouched bench test hung the full suite.
- **Goals (owner)**: voxi always works and lmcoder stays host-agnostic across um760, t14 and x600. Bluetooth headset dictation works out of the box. Cheap developer models do the work, and the owner gets short instructions.
- **Review**: second-vendor review by Gemini 4 Argon (agy:argon:med). Codex sol:med was tried first, and the account rejected that model. Five findings were all fixed: the commit split, the "no host coding / no reverts" wording, the scope of the 202/206/207 work, the missing key commits, and the open-ticket list.

## 2. Executive Summary
Since the baseline: 62 commits: 18 feat/fix/refactor, 3 chore (two releases and one harnez refresh) and 41 docs. Of the docs commits, 40 are ticket, study or evergreen updates and one is a milestone (9facd22). Earlier in the window (2026-10-26..30), tickets 202 (eager temp dir and monitor failure state), 206 (offline ASR diagnosis and failover) and 207 (`pw-record --raw` capture) shipped with luna, haiku and Opus developers; the narrative below covers mainly the 2026-11-02/03 sprint (210–214). Two releases (v0.1.17 on 2026-10-26, v0.1.18 on 2026-10-30). Eight tickets closed (202, 206, 207, 210, 211, 212, 213, 214). Outside `issues/`: +7,116/−305 lines in 102 files, of which tests are +4,352 (61%) and docs +546.

Since 2026-11-02 one host session (Opus 5.5) has run haiku:med developers and opus:med reviewers. A separate lmcoder Claude session worked alongside it, and the two coordinated only through cross-session messages and files. The result:
- **Shared contract**: lmcoder owns host behaviour. It starts ASR on demand, has `/lmcoder/wake`, and returns clear 400/503 errors (lmcoder 274, released as v0.1.15). voxi only uses that contract (wake on record start, 503 failover, spec probe timeout).
- **Live checks**: a stopped ASR now recovers in 1.7 s with no hand edits, and soft Bluetooth dictation after a 30 s pause transcribes in full.

## 3. What Worked Well
- **Cross-session agents with a clear owner per repo.** The voxi host never edited lmcoder. It sent a list of problems, a proposed contract and the acceptance test ("fresh make install on three hosts, stopped ASR, dictate"). The lmcoder session pushed back on two points: capability detection is not needed, and `--wait` and `status --json` are not needed because the proxy blocks until ready. It shipped the smaller version, and voxi then dropped its own systemd management to a fallback. The user only set the goal. Neither repo grew code that belongs to the other.
- **Files-first coordination.** The ticket is the only channel between host and developer: decisions, milestones, pre-work and progress are appended there and committed before each resume. A cold-started Haiku session needs nothing else. Reports end with a fixed `TEST RESULT` block, test output goes to `/tmp/test-q1-*.log`, and the host greps the log itself rather than trusting a summary.
- **harnez as the harness.**
  - `harnez agent start/resume --name` gives named, resumable, background developers and reviewers. `harnez issues new/open/close` refuses near-duplicates (it caught a 210 duplicate and the ticket was reopened instead).
  - A close needs a written reason with evidence.
  - `make test-q1` enforces one full run per step, and harnez flags code changed after the last quota-1 run.
  - The `lean-sprint` and `decide` skills fix the roles: during the sprint the host did not code, and it pushed ready work without asking. The exception came before the sprint: the host committed the LFS-pointer diagnostic fix itself (424a2b2).
- **Go plus haiku:med.** The developer delivered 14 milestones across 5 tickets, every one with green tests and no reverts. Measured on um760, the stack keeps agent turns fast: an incremental `go build ./cmd/voxi` takes 0.35 s, `go test ./internal/doctor` 1.6 s, and a cold `go build ./...` over 57k Go lines 5.8 s. The developer runs a test after almost every edit, so a fast compile-test loop is what makes a cheap model reliable.
- **Specs as the single source of truth.** Gain, wait budgets, unit names, Bluetooth identity and cost figures all moved into `spec/*.yaml` with schemas, and spec tests check bounds against the schema. That made the reviews mechanical ("this literal duplicates spec").

## 4. Honest Post-Mortem (Failures, Bugs & Near-Misses)
- **Git LFS pointers embedded as audio.** git-lfs was not installed, so the doctor's probe clip and the `typing-paused.wav` notify sound were embedded as LFS pointer text, and the doctor reported a false ASR fault. Caught by host diagnosis, not by tests. Fix: an LFS exemption plus a real clip (57a650d), a RIFF test, and git-lfs installed and pulled. **Open risk:** a clone without LFS still embeds a pointer for the notify sound, and no build test catches it. The owner was offered this test and has not decided.
- **The host's own wrong model name.** The host's curl sent `r2t2-confucius4` instead of the spec `api_model` `r2t2`. It got 501, briefly blamed the proxy, and added a failover-on-501 change that was then reverted. Cost: one wasted cycle.
- **Wrong gain derivation (6×).** Haiku compared RMS from different utterances and picked 6× gain. Opus caught it in review: wpctl volume is cubic, so 1.4 means about 2.74×, and 6× let noise through the gate and clipped speech. Fixed to 2.75. Tests passed at 6×; only reasoning in review caught it.
- **Unbounded waits in doctor --fix.** The start path could block silently for about 15 min, the "5 min" budget was not wall-clock, and FAULT hints were never printed. Tests checked Result fields, not printed output. Opus review caught all of this, and it was fixed in 211 M4.
- **Undo could overwrite a user setting.** `voxi mic headset --off` would have turned autoswitch back on even though the owner set it by hand, and a weak ownership marker claimed the owner's hand-written drop-in. Caught by review and fixed (first-line marker, `--adopt`, restore only what voxi changed).
- **Hidden test hang.** An untouched bench test downloaded about 1 GiB of real weights, because a 1 KiB fixture was below the size floor, and timed the full suite out at 10 min. It was found only when 211 needed a green full run, and became ticket 212.
- **Earlier in the window (202, 206).** 202 M2 was committed after a test run that stopped at `go vet` before any tests ran, and the host's full run closed the gap. In 206 the Haiku developer's first attempt needed escalation to Opus, and a new `Setenv` lint finding was fixed but not rechecked.
- **Report truncation.** The host piped developer output through `tail -25` and lost the test-q1 lines, which cost one round of grepping. Fixed by requiring `TEST RESULT` as the last lines.
- **Host rule slips.**
  - Several ticket updates were heredoc appends from Bash. The hook flagged them, and the host declined to rate them.
  - Many tool failures went unrated (the harnez tip counted 16).
  - 213 was closed without a second review, because it was a refactor with unchanged assertions; that was a judgement call.
  - The gate thresholds were still Go literals after 213 (parked as 215).

## 5. Quality & Invariants Audit

| Area | Verdict | Evidence |
|---|---|---|
| Architecture & module separation | Improved | lmcoder owns ASR lifecycle; voxi consumes a contract. Gain resolution is shared by eager and monitor (no copy). BT identity in one spec accessor (213). |
| Idempotency | Stable | `voxi mic headset` rewrites only its own marked drop-in; `--off` restores only voxi's own changes; `voxi settings set … default` round trip left config.yaml byte-identical. |
| Backward compatibility | Stable | Start repair kept as fallback for hosts on old lmcoder; wake 404 ignored; gain 1 disables; non-BT sources unchanged. |
| Test coverage & verification | Improved | Tests are 61% of new lines; `make test-q1` 38/38 ok after each ticket (red before 212); live checks: stopped-ASR doctor probe, owner dictation (two runs), monitor label in tmux. |

Overall quality verdict: **improved**. No committed milestone was reverted; the host's uncommitted failover-on-501 detour was discarded. One regression was caught live (LFS pointers) and one before merge (gain 6×).

## 6. Efficiency & Velocity Assessment
- **Span**: tickets 211 → 212 → 210 (5 milestones) → 214 → 213 ran in one long session over 2026-11-02 and 2026-11-03, with live checks in between.
- **Developer**: haiku:med turns took about 3–4 min for a milestone (M5 of 211: 3m29s), so throughput was high. Token use was heavy because of cached context (one turn reported about 2.1M tokens including cache). The 80k compaction threshold kept costs bounded.
- **Reviews**: opus:med took about 1–2 min per review and caught every substantive defect. That is cheap relative to what the defects would have cost.
- **Owner time**: the owner gave short instructions and ran three live dictations. There were no long explanations, and no mic was held open while the owner spoke.
- **Compared with the previous story**: smoother. That session had compile and lint fallout and inline URL-resolution bugs. The 210–214 sprint had no reverted milestones and no host coding (the one host commit, 424a2b2, came before it). The 202/206 work earlier in the window still had a premature commit and an escalation. In addition, the cross-session split removed a whole planned feature (voxi-side systemd management). It was not perfectly smooth: three review rounds were needed on 211, and 210 needed a reopen for settings.

## 7. Key Learnings & Evergreen Upstream
1. **Contract first across repos.** When two repos' agents meet at an interface, send the problems, a proposed contract and one acceptance test. Let the owning repo shrink the contract, then delete the consumer-side workaround. (Candidate for docs/AgenticLoop.md.)
2. **Tests must check printed output for CLI repairs.** Result-field tests passed while users would have seen nothing. (Candidate for docs/TestTiers.md.)
3. **Derived constants need their derivation reviewed.** A green test cannot catch a wrong physical model (cubic volume). Ask the reviewer to check the maths, not only the code.
4. **Every embedded binary asset needs a format test** (RIFF/WAVE etc.), because a missing git-lfs silently ships pointers. (Candidate rule for docs/Go.md.)
5. **Developer reports end with a fixed `TEST RESULT` block**, and the host greps the log, never a truncated tail. (Candidate for the lean-sprint skill.)
6. **Fast Go builds are a model multiplier.** With sub-second incremental builds and seconds-long package tests, a cheap model can afford test-after-every-edit. Keep packages small and tests hermetic: the 212 hang shows one network-touching test can erase that advantage.

## 8. File & Diff Summary
- **Diff since baseline** (outside `issues/`): 102 files, +7,116/−305. Tests: 33 files, +4,352/−53. Docs: 17 files, +546/−53.
- **Key files**:
  - `internal/doctor/{asr.go,fix.go,style.go}`: ASR start repair, cost notice, colour.
  - `internal/eager/{openai_transcribe.go,eager.go,micwatch.go}`: wake on record start, gain probe.
  - `internal/audio/gain.go`: digital gain with clamp.
  - `internal/mic/{headset.go,bluetooth.go,state.go}`: `voxi mic headset`, BT identity.
  - `internal/monitor/micgain.go`: meter gain.
  - `internal/bench/bench_test.go`: hermetic tests.
  - `spec/{models,doctor,eager}.yaml` and their schemas.
  - `.gitattributes`: LFS exemption for the probe clip.
  - `docs/AudioDoctor.md`, `docs/MicSelfCheck.md`.
- **Key commits**:
  - 202: f5fb21c, c3595c0, 10fb06b (`internal/eager/audio_failure.go`, `internal/monitor/{collector,render}.go`).
  - 206: 67ca553, c5d8419 (`internal/agent/command.go`, `docs/DictationQARunbook.md`).
  - 207: 4c7bd99 (`internal/eager/audio.go`).
  - pre-211 host fix: 424a2b2.
  - 211: 57a650d, 8cefaf4, b04cca3, d2d8ed6, 9facd22, b8888c5.
  - 212: 64a17f2.
  - 210: 3715e41, efdc914, 44b5b7d, ae2aed6, 3c02fcd.
  - 214: 92030f8.
  - 213: 5e39469.
- **Releases**: v0.1.17, v0.1.18 (voxi); lmcoder v0.1.15 (other session).
- **Follow-up tickets**: 215 (gate thresholds to spec, P3). Filed in this window and still open: 203, 204, 205, 208, 209, 215. Open from before the baseline: 200, 201, plus 39 older active tickets in `issues/README.md`.
