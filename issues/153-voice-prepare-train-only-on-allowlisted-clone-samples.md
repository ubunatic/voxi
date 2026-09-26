# 153 — voice prepare: train only on allowlisted clone samples

**Status**: Closed — Implemented allowlist filtering in voxi voice prepare
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Bug
**Related**: `internal/tts/clone/dataset.go`, issue 151, `~/.config/voxi/samples/`

---

## 1. Problem & Motivation
`voxi voice prepare` trained on every transcribed sample in `~/.config/voxi/samples/corpus.tsv`.
That corpus also holds bug reproductions, noisy recordings and ASR test clips, which would degrade
a cloned voice.

## 2. Technical Specification / Findings
- New file `~/.config/voxi/samples/voice-training.txt`: one sample id per line, `#` comments allowed.
- `prepare` fails when the file is missing or empty, and when it names an id not in the corpus.
- `corpus.tsv` format is unchanged, so benchmark scripts reading it are unaffected.
- Corpus transcripts come from the recording engine (Gemini); Whisper re-transcription differs and is
  not authoritative for correcting them.

## 3. Implementation & Verification Plan
- Filtering in `readAllowlist`/`filterAllowed`; tests cover filtering, missing file, unknown id.
- Verified live: `voxi voice prepare` prepared 3 samples from the user's allowlist.

## 4. Follow-ups (from issue 151 review, not yet done)
- `scripts/voice-training/runner.py`: keep the previous ONNX until training succeeds.
- `internal/tts/clone/train.go`: key the checkpoint cache by URL hash, not basename.
