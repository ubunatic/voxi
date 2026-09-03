# 055: Save Any Chunk as Dev Sample by Index (`voxi feedback sample save-chunk`)

**Status**: Implemented
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [042 private dev sample recorder](042-private-dev-sample-recorder.md), [053 ring buffer chunks](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [internal/feedback/command.go](../internal/feedback/command.go), [internal/devsample/flow.go](../internal/devsample/flow.go)

---

## 1. Problem & Motivation

Issue 053 introduced `voxi feedback sample save-last <name>`, which allows promoting the *single most recent* audio chunk from the ring buffer into `~/.config/voxi/samples/corpus.tsv` as a test fixture.

However, during normal speech or testing sessions:
1. The user might speak several sentences or notice an artifact several utterances after it occurred (for instance, noticing chunk `#34` "Switch Boss." or chunk `#43` "andcom." after several subsequent silence chunks or follow-up utterances have already been recorded).
2. By the time the user realizes they want to save chunk `#34` or `#43`, `save-last` points to chunk `#48` or `#50`.
3. The audio and metadata for chunk `#34` are still sitting safely in the 10-chunk ring buffer (`$XDG_RUNTIME_DIR/voxi/chunks/chunk_0034.wav`), but there is no CLI command to save that specific chunk by number without manually copying files and editing TSVs.

---

## 2. Desired Design

Extend `voxi feedback sample` to support saving any chunk by its index:

```text
voxi feedback sample save-chunk <index> <name> [--force]
```

Or make `save-chunk` accept either `<name>` (defaulting to last) or `<index> <name>`, while keeping `save-last <name>` as an alias for backwards compatibility:

- `voxi feedback sample save-chunk 43 artifact-keyboard-smash`
- `voxi feedback sample save-chunk last my-sample-name`
- `voxi feedback sample save-last my-sample-name` (preserves existing syntax as alias to `save-chunk last`)

### Flow:
1. Look up chunk `<index>` (or `"last"`) in the ring buffer using `chunkBuf.Get(selector)`.
2. Report error if chunk `<index>` has already rotated out of the 10-chunk buffer.
3. Pre-fill the prompt with the chunk's `CleanedTranscript` (or `RawTranscript` if cleaned was empty or rejected).
4. Prompt for the ground-truth text and keyterms (using the interactive terminal editor / raw-mode editing in `devsample`).
5. Save audio as `~/.config/voxi/samples/<name>.wav` and append/upsert the entry in `~/.config/voxi/samples/corpus.tsv`.

---

## 3. Implementation Plan

1. **CLI Command in `internal/feedback/command.go`**:
   - Add `saveChunkCmd`:
     - Usage: `save-chunk [INDEX] NAME` (where if 1 arg is provided, INDEX defaults to "last").
     - Flags: `--force` (overwrite existing sample).
   - Point `save-last NAME` to call the same underlying helper with selector `"last"`.
2. **Helper in `internal/devsample`**:
   - Ensure `SaveChunkAsSample` accepts the chunk selector / index cleanly and formats user feedback informing which chunk index is being promoted.
3. **Tests**:
   - Add unit tests verifying `save-chunk <index> <name>` retrieves the right chunk by number and saves it into the sample manifest.

---

## 4. Verification

- Run `voxi chunks list` to identify an earlier chunk index (e.g. #36).
- Run `voxi feedback sample save-chunk 36 test-chunk-36`.
- Verify `test-chunk-36` appears in `voxi feedback sample list` and matches the audio/text of chunk 36.
