# 108 — Chunk Pipeline Diagnostics, Detailed Trace Inspection, and Status Badges

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: `internal/chunks/chunks.go`, `internal/chunks/command.go`, `internal/eager/eager.go`, `docs/ChunkDiagnostics.md`, [Issue 107](107-interactive-settings-tui-for-feature-toggles-and-configuration.md), [Issue 106](106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md)

---

## 1. Problem & Motivation

When inspecting recent transcription chunks via `voxi chunks show` and `voxi chunks list`, users currently only see raw and cleaned text along with a binary `accepted` / `rej:<reason>` status.

There is no visibility into:
1. Which ASR engine / model produced the transcript (e.g. `cohere-transcribe-03-2026` via `crispasr` vs. `whisper` via `voxtype`).
2. Whether deterministic replacement rules (`voxi feedback replacement`) were triggered on that chunk.
3. Whether LLM post-processing cleanup ran, what model was queried, and what the raw LLM output was prior to final acceptance.
4. An effective visual summary in `voxi chunks list` indicating which pipeline stages actually modified or filtered the transcript.

---

## 2. Technical Specification

### 2.1 Chunk Struct Extensions (`internal/chunks/chunks.go`)
Extend `Chunk` with pipeline provenance metadata:
- `Model` (`string`): e.g. `"cohere-transcribe-03-2026"`, `"large-v3-turbo"`.
- `Engine` (`string`): `"cohere-transcribe"` or `"whisper"`.
- `AppliedReplacements` (`[]ReplacementSummary`): Record of exact replacements that fired on this chunk (e.g. `[{"from": "Voxy", "to": "voxi"}]`).
- `LLMCleanup` (`*LLMCleanupRecord`): If cleaner ran:
  - `Enabled` (`bool`)
  - `Model` (`string`, e.g. `"qwen3-4b-instruct-2507-q4"`)
  - `Output` (`string`)
  - `Modified` (`bool`)
- `StopWordsMatched` (`[]string`): Stop-word hallucination patterns that were stripped.

### 2.2 `voxi chunks show [INDEX]` Trace View
Enhance text output (`--format text`) to display a clear step-by-step transformation trace:

```text
Chunk #209 Diagnostics & Pipeline Summary:
==================================================================
Timestamp:              2026-09-13 17:07:26 (Audio: 2.36s, RTF: 0.44)
ASR Engine:             cohere-transcribe-03-2026 (via crispasr)
Status:                 ACCEPTED

Pipeline Transformations:
------------------------------------------------------------------
1. Raw ASR Output:      "This is Chunk One with Voxy."
2. Replacements:        "Voxy" -> "voxi"
3. LLM Cleanup:         qwen3-4b-instruct-2507-q4 (via lmcoder)
   LLM Output:          "This is Chunk 1 with Voxi."
4. Final Committed:     "This is Chunk 1 with Voxi."

Injection:
------------------------------------------------------------------
Destination:            Focused Window via dotool (type_delay_ms = 0ms)
Latency:                1.04s transcribe + 8ms typing = 1.05s total
```

### 2.3 `voxi chunks list` Effective Pipeline Badges
Update the table columns in `voxi chunks list`:
- Column `STATUS`: Displays outcome prefix (`✓` or `✗`) followed by icons for **only the pipeline stages that were actually effective**:
  - `✓` / `✗`: Accepted / Rejected
  - `⚡`: Cohere Transcribe (`crispasr`)
  - `👂`: Whisper (`voxtype`)
  - `⇄`: Replacement rule fired
  - `🤖`: LLM cleaner modified text
  - `✂`: Stop-word hallucination stripped
  - `🛡`: Safety circuit breaker tripped
- Column `TRANSCRIPT / REASON`:
  - If accepted: displays final committed text.
  - If rejected: displays `(rejection_reason)`.

Example:
```text
INDEX   TIMESTAMP            AUDIO   RTF   RMS  LEVEL          STATUS       TRANSCRIPT / REASON
#207    2026-09-13 17:07:20   2.1s  0.42   812  [⣄⣀⣀⣀⣰⣤⣶⣴⣄⣀]  ✓ ⚡ 🤖      Chunk 7, Chunk 8, Chunk 9, Chunk 10.
#208    2026-09-13 17:07:23   1.8s  0.39   790  [⣄⣀⣀⣀⣰⣤⣶⣴⣄⣀]  ✓ ⚡ ⇄       Voxi continuous dictation on ubunatic.com
#209    2026-09-13 17:07:26   2.4s  0.44   809  [⣄⣀⣀⣀⣰⣤⣶⣴⣄⣀]  ✓ ⚡          This is Chunk One.
#210    2026-09-13 17:07:29   0.3s  0.00   110  [⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]  ✗            (low_energy_transient)
#211    2026-09-13 17:07:32   1.5s  0.40   540  [⣄⣀⣀⣀⣰⣤⣶⣴⣄⣀]  ✗ ⚡ ✂        (stop_word_matched: thanks-for-watching)
```

---

## 3. Implementation Plan

1. Update `Chunk` data structures in `internal/chunks/chunks.go` to hold engine, replacements, and LLM telemetry.
2. Update sequential transcription worker in `internal/eager/eager.go` to populate pipeline provenance fields.
3. Update `FormatChunkDetails` and `WriteTable` in `internal/chunks/command.go` with badge rendering and `TRANSCRIPT / REASON` column.
4. Add unit tests for badge generation, text formatting, and JSON serialization.

---

## 4. Acceptance Criteria

- [ ] `voxi chunks show [INDEX]` shows model, engine, applied replacements, LLM output, and final text.
- [ ] `voxi chunks list` displays effective pipeline badges (`✓ ⚡ 🤖`, `✓ ⚡ ⇄`, `✗ ⚡ ✂`) and `TRANSCRIPT / REASON` column.
- [ ] Non-effective stages are omitted from the status column to keep visual noise minimal.
- [ ] All unit tests pass and `make check` succeeds.
