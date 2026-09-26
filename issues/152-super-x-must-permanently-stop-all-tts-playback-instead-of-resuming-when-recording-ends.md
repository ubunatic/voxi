# 152 — Super+X must permanently stop all TTS playback instead of resuming when recording ends

**Status**: Open
**Priority**: P1 (High)
**Severity**: Normal
**Category**: Bug
**Related**: 141 (TTS engine discovery), 143 (desktop selection reading), `internal/tts/manager.go`, `internal/eager/eager.go`

---

## 1. Problem & Motivation

When text is reading aloud via TTS (`voxi say`, `Super+Y`), starting voice recording (`Super+X`) correctly mutes/stops active playback to prevent acoustic feedback into the microphone.

However, when the user presses `Super+X` a second time to stop recording (or when recording completes and drains), the TTS manager treats the recording epoch as merely a temporary pause gate: `ActionRecordingStart` sets `recording = true` and cleans up current audio, but leaves remaining queued items in `items` (`cursor < len(items)`). When `ActionRecordingEnd` arrives (`recording = false`), the run loop at line 351 immediately restarts synthesis and playback of the remaining queue.

When the user activates voice dictation with `Super+X`, they intend to interrupt and permanently cancel reading—not have speech output unexpectedly resume talking over their desktop when dictation stops.

## 2. Technical Specification & Findings

In `internal/tts/manager.go`:
1. `ActionRecordingStart` (lines 387–399) cleans up in-flight synthesis/playback jobs and sets `recording = true`, but retains the queued `items` slice.
2. In `applyCommand`:
   ```go
   case ActionRecordingStart, ActionRecordingEnd:
       return commandResult{}
   ```
   `ActionRecordingStart` needs to perform a full queue discard (equivalent to `ActionStop`: `*items = nil; *cursor = 0; *current = ""; *paused = false; *firstQueuedAt = time.Time{}; *firstAudio = 0`) so that when `ActionRecordingEnd` fires, there is no lingering queue to resume.
3. Verify that `internal/eager/recording_epoch_test.go` and `internal/tts/manager_test.go` reflect this complete stop behavior.

## 3. /goal

Ensure triggering voice recording via `Super+X` permanently cancels and purges all TTS playback and queued text chunks, preventing the voice player from resuming when recording ends.
