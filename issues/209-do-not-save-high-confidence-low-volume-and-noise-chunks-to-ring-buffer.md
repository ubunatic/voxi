# 209 — Do Not Save High-Confidence Low-Volume and Noise Chunks to Ring Buffer

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Performance
**Related**: [208 continuous ambient noise chunk triggers and R2T2 hallucination](208-continuous-ambient-noise-chunk-triggers-and-r2t2-hallucination-accepted-to-desktop-chunk-2177.md), [053 ring buffer recent audio chunks](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [070 show recorded volume rms column](070-show-recorded-volume-rms-column-in-voxi-chunks-list.md), [098 unify chunks list and feedback sample list](098-unify-chunks-list-and-feedback-sample-list-behind-a-shared-lister-add-all-full-short-to-sample-list.md)

---

## 1. Problem & Motivation

The eager listening pipeline writes every cut chunk into the chunk ring buffer in `$XDG_RUNTIME_DIR/voxi/chunks/` via `chunkBuf.AddExistingWAV`, even when the acoustic gating has already classified the chunk with high confidence as non-speech (e.g. `!job.Plausible` due to `low_energy_transient` or `unvoiced_transient`).

Because the ring buffer has a fixed capacity of 100 entries (`chunks.DefaultBufferSize`), ambient room noises (faint rustles, breathing, desk bumps, mouse clicks) can trigger dozens of transient cuts during quiet periods (as observed in #208, where ~100 transient chunks were generated in 12 minutes).

This causes two major issues:
1. **Ring Buffer Pollution**: Legitimate dictated speech chunks are rapidly pushed out and evicted from the 100-chunk buffer by continuous noise transients, preventing users from reviewing recent dictation (`voxi chunks list`), replaying (`voxi chunks play`), or saving them permanently (`voxi sample add --chunk N`).
2. **Unnecessary Disk & CPU Churn**: Writing short WAV files and JSON metadata sidecars to disk every few seconds for audio that is known with high confidence to be noise adds needless I/O and filesystem overhead.

## 2. Technical Specification & Findings

In `internal/eager/eager.go`:
```go
if !job.Plausible {
    rejReason := job.RejectionReason
    if rejReason == "" {
        rejReason = "low_energy_transient"
    }
    chunkMeta := chunks.Chunk{ ... }
    _, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
    _ = os.Remove(wavPath)
    continue
}
```

Even though `job.Plausible` is false and the segment is known to be a low-energy or unvoiced transient, it allocates a chunk index and persists metadata and audio into `chunkBuf`.

### Desired Behavior
- If the pipeline has high confidence that an audio segment is low volume or noise (such as `low_energy_transient` or `unvoiced_transient`), it should **not** be saved to the chunk buffer.
- The temporary WAV file should simply be removed (`_ = os.Remove(wavPath)`) and the loop continued without allocating an index or writing to `chunkBuf`.
- The chunk ring buffer should be reserved for chunks that have speech plausibility, undergo ASR transcription, or represent ambiguous audio that required evaluation.
- Optionally provide a debug/inspection toggle (e.g., `--keep-noise-chunks` or a spec setting) for developer workflows when tuning VAD and acoustic filters.

## 3. Implementation & Verification Plan

### Goal

`/goal Skip saving high-confidence low-volume and noise audio chunks to the ring buffer in internal/eager, preserving buffer capacity for speech and avoiding disk churn, or stop and report when blocked on a user decision or denied permission.`

### Tasks
- [ ] In `internal/eager/eager.go`, skip `chunkBuf.AddExistingWAV` for high-confidence noise / non-speech rejections (`low_energy_transient`, `unvoiced_transient`). Ensure the temporary WAV file is unlinked immediately.
- [ ] Determine if any telemetry event should still record the transient without persisting a chunk to disk.
- [ ] Assess whether an opt-in flag/setting is desirable for debugging acoustic gates.
- [ ] Add unit tests in `internal/eager` ensuring:
  - High-confidence low-volume / noise audio does not create entries in the chunk buffer.
  - Plausible speech and transcribed audio continue to be stored in `chunkBuf` as expected.
