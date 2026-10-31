# 208 — Continuous Ambient Noise Chunk Triggers and R2T2 Hallucination Accepted to Desktop (Chunk 2177)

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [100 background voice hallucinated into accepted transcripts](100-background-distant-voice-hallucinated-into-accepted-transcripts-bypassing-silence-gate.md), [054 short-pause acoustic gating](054-short-pause-acoustic-gating-and-context-priming.md), [059 VAD micro-pause chunk splitting](059-live-test-follow-up-leading-dash-fragment-artifacts-and-vad-micro-pause-chunk-splitting-degrade-transcript-quality.md), [083 reject runaway dotool injection](083-prevent-runaway-repeated-dotool-desktop-injection.md), [138 treat r2t2 language none as no-speech](138-treat-r2t2-language-none-scaffold-as-an-explicit-no-speech-result.md)

---

## 1. Problem & Motivation

During a quiet desk session on 2026-10-31, Voxi exhibited two severe, related audio pipeline issues:

1. **Continuous noise chunk churning**: Within a 12-minute window (15:46 to 15:58), the eager recording pipeline continuously triggered, segmented, and evaluated roughly 100 audio chunks (chunks #2151 through #2250) — cutting a chunk every 1 to 5 seconds from ambient background noise (RMS 30–90). Almost all were discarded as `low_energy_transient`, `unvoiced_transient`, or ASR `(empty)`, placing continuous unnecessary load on VAD and ASR inference.
2. **Accepted 100% hallucination typed to desktop**: Chunk #2177 (duration 3.4s, mean RMS 127, peak RMS 685, voiced ratio 0.3) bypassed the silence gate (`probable_silence: false`). Sent to R2T2 (`r2t2-confucius4` via `openai-transcribe`), the ASR hallucinated a full, well-formed German question from pure ambient noise: `"Willst du trotzdem noch eine Spange haben?"`. The eager pipeline marked the chunk accepted (`accepted: true`) and injected it directly into the focused desktop window via `dotool`. The user spoke nothing at all.

False-positive injection of fabricated sentences into arbitrary focused desktop windows interrupts user typing, corrupts code or chats, and damages confidence in live dictation.

## 2. Technical Findings & Reproduction

### 2.1 Chunk Diagnostics (#2177)

`voxi chunks show 2177`:
```json
{
  "index": 2177,
  "timestamp": "2026-10-31T15:48:02.731361159+01:00",
  "session_id": "20261031T121108.235977190Z-000131",
  "chunk_id": "20261031T121108.235977190Z-000131/753",
  "finalized_at": "2026-10-31T15:48:02.731361159+01:00",
  "transcription_started_at": "2026-10-31T15:48:02.731705678+01:00",
  "transcription_ended_at": "2026-10-31T15:48:03.650590721+01:00",
  "typing_started_at": "2026-10-31T15:59:05.948821028+01:00",
  "typing_ended_at": "2026-10-31T15:59:05.952582996+01:00",
  "audio_duration_secs": 3.4,
  "pcm_bytes": 108800,
  "mean_rms": 127,
  "peak_rms": 685,
  "volume_sparkline": "⣀⣀⣀⣀⣀⣀⣄⣀⣀⣀",
  "voiced_ratio": 0.3,
  "probable_silence": false,
  "transcribe_duration_secs": 0.918885083,
  "transcript_word_count": 7,
  "rtf": 0.2702603185294118,
  "raw_transcript": "Willst du trotzdem noch eine Spange haben?",
  "cleaned_transcript": "Willst du trotzdem noch eine Spange haben?",
  "accepted": true,
  "transcript_chars": 42,
  "transcript_digest": "sha256:d02daa33a60fdee4",
  "model": "r2t2-confucius4",
  "engine": "openai-transcribe",
  "wav_file": "chunk_2177.wav"
}
```

### 2.2 Continuous Noise Chunk Churning Pattern

Excerpt from `voxi chunks list -a`:
```
INDEX   TIMESTAMP            AUDIO   RTF      RMS  LEVEL         STATUS        TRANSCRIPT / REASON
#2165   2026-10-31 15:47:20  2.2s    0.00      50  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2166   2026-10-31 15:47:22  1.9s    0.00      52  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2167   2026-10-31 15:47:24  0.9s    0.00      66  [⣀⣀⣀⣀⣀⣤⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2168   2026-10-31 15:47:26  1.4s    0.00      90  [⣀⣀⣀⣀⣀⣤⣀⣀⣀⣀]  ✗             (unvoiced_transient)
#2169   2026-10-31 15:47:32  1.3s    0.32     203  [⣀⣀⣀⣠⣤⣤⣀⣀⣀⣀]  ✗ 🌐           (empty)
#2170   2026-10-31 15:47:37  1.2s    0.00      63  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2171   2026-10-31 15:47:44  0.9s    0.00      82  [⣀⣀⣀⣀⣀⣠⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2172   2026-10-31 15:47:47  3.1s    0.00      63  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (unvoiced_transient)
#2173   2026-10-31 15:47:48  1.0s    0.00      41  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2174   2026-10-31 15:47:50  0.9s    0.00      34  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2175   2026-10-31 15:47:53  0.9s    0.00      64  [⣀⣀⣀⣀⣀⣠⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2176   2026-10-31 15:47:59  2.7s    0.20     370  [⣀⣀⣴⣦⣤⣀⣀⣀⣀⣀]  ✗ 🌐           (empty)
#2177   2026-10-31 15:48:02  3.4s    0.27     127  [⣀⣀⣀⣀⣀⣀⣄⣀⣀⣀]  ✓ 🌐           Willst du trotzdem noch eine Spange h...
#2178   2026-10-31 15:48:04  1.0s    0.00      96  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
...
#2247   2026-10-31 15:58:38  0.9s    0.00      53  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2248   2026-10-31 15:58:47  1.2s    0.00      90  [⣀⣀⣀⣀⣤⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2249   2026-10-31 15:58:52  1.1s    0.00      80  [⣀⣀⣀⣀⣀⣠⣀⣀⣀⣀]  ✗             (low_energy_transient)
#2250   2026-10-31 15:58:53  1.2s    0.00      52  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗             (low_energy_transient)
```

### 2.3 Preserved Sample Asset

The audio and sidecar for chunk #2177 have been saved permanently to the private sample store:
- ID: `halluzi-spange-2177`
- Purpose: `noise` (expected transcript: `""`)
- Path: `~/.local/share/voxi/samples/noise/halluzi-spange-2177.{wav,json}`

### 2.4 Why the Current Defenses Failed

- **VAD / Segmentation Gate**: Audio segments were triggered and cut every few seconds on ambient sound below RMS 100.
- **Silence Gate**: Chunk #2177 had mean RMS 127 and voiced ratio 0.30, which fell right above the current `probable_silence` threshold, even though the energy profile was a single faint transient in 3.4 seconds of silence (`⣀⣀⣀⣀⣀⣀⣄⣀⣀⣀`).
- **Transcript Safety & Trailing Clause Gates**: The hallucination was a single, grammatical 7-word German question (`Willst du trotzdem noch eine Spange haben?`). It contained no repetitive characters or trailing duplicate clauses, so repetition guards (e.g. issues 083, 094, 100 §7) did not trigger.
- **Delayed Typing**: The chunk was transcribed at 15:48:03 but typed at 15:59:05 (11 minutes later). The delay mechanism in modifier/drain gating when idle chunks accumulate needs investigation.

## 3. Implementation & Verification Plan

### Goal

`/goal Suppress ambient noise chunk churning and reject ASR hallucinations on unvoiced/ambient audio before desktop injection, verified against halluzi-spange-2177 and quiet listening tests, or stop and report when blocked on a user decision or denied permission.`

### Scope & Investigation Areas

1. **VAD / Segmentation Churn**: Investigate the microphone stream segmentation thresholds in `internal/audio` / `internal/eager`. Calibrate noise floor estimation or transient pre-filtering so room ambient noise (RMS 30–90) does not spawn dozens of transient chunks per minute.
2. **Pre-ASR Energy & Voiced Distribution Gating**: Tighten the threshold for dispatching chunks to ASR. A 3.4s chunk with mean RMS 127 and a 90% silent sparkline should not be treated as voiced speech.
3. **ASR Hallucination Gating**: Check confidence scores or acoustic plausibility for non-English / unexpected language output when background noise is transcribed by R2T2.
4. **Delayed Typing Investigation**: Clarify why chunk #2177 was held from 15:48 until 15:59 before typing was executed.
5. **Verification**: Replay `halluzi-spange-2177` through the pipeline to ensure it is cleanly rejected without sending to ASR or typing, while ensuring legitimate quiet dictation samples (`short-*`, `kt-*`) continue to pass.
