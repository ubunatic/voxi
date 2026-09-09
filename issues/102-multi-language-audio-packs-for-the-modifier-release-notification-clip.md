# 102 — Multi-Language Audio Packs for the Modifier-Release Notification Clip

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Spec/Design
**Related**: [101 modifier-release race leaks buffered typing into GNOME overview search box](101-modifier-release-race-leaks-buffered-typing-into-gnome-overview-search-box.md)

---

## 1. Problem / Motivation

101 specifies the v1 implementation of its "non-injection notification
channel" option (§4, Option C) as **one pre-recorded, English-only WAV
clip** ("Typing paused. Stop recording to finish."), committed to the repo
and embedded in the `voxi` binary via Go `embed`, played back as an audible
cue with no live TTS synthesis and no runtime engine/voice detection. That
choice was made explicitly to avoid the complexity of the general dynamic
case (detecting which TTS system/voice is installed, composing message
text, invoking the right engine) for the ticket it needed to close.

That simplification doesn't generalize: a single hardcoded English clip
gives no equivalent notification to a non-English-speaking voxi user. This
ticket is the deferred follow-up, filed at 101's request during design
discussion, to plan (not implement) multi-language support for the same
notification mechanism.

## 2. Existing Language Handling in Voxi (context for this ticket)

Checked before filing, so the plan below doesn't invent a second
language-selection mechanism where one might already exist:

- **`spec/models.yaml`** has no language/locale field at all currently.
- **`internal/eager/cohere.go`** hardcodes `--language en` when invoking
  the Cohere Transcribe backend, specifically to prevent Cohere's
  multilingual auto-detect from mis-transcribing short/ambiguous English
  audio as another language (see comment at cohere.go:47-55). This is a
  transcription-accuracy safeguard, not a user-facing "I speak language X"
  preference.
- No other user-facing language/locale configuration was found in
  `internal/` or `spec/`.

**Implication**: voxi currently has no existing ASR language-selection
config to key a notification-clip language off of. Any language-pack
mechanism for this ticket would likely need to introduce voxi's *first*
user-facing language setting, rather than reuse one — unless a future
change (e.g. adding real multilingual dictation support) introduces one
first, in which case this ticket should key off that instead of adding a
second, independent setting. Whoever picks this up should re-check for a
language config field before designing one from scratch, since 101/102
predate any such feature.

## 3. Scope — Open Questions, Not Decisions

This ticket is planning/deferred-scope only, mirroring 101's spec-only
nature. None of the following are decided:

- **Format/compression**: the user suggested FLAC during design
  discussion, matching `testdata/noise-samples`'s existing convention.
  Confirmed: `.gitattributes` already LFS-tracks both `*.wav` and `*.flac`
  repo-wide, so either format (or WAV, matching 101's v1 clip) can reuse
  the existing LFS setup without new `.gitattributes` entries. No
  `Makefile` targets specific to noise-samples LFS handling were found —
  worth re-checking at implementation time in case that changes.
- **Which languages to prioritize**: unresolved; no candidate list yet.
- **Language selection mechanism**: does this reuse an existing (or
  future) voxi dictation-language config (see §2 — none exists today), or
  does it need its own standalone setting independent of ASR language?
- **Clip sourcing**: real human speaker recording per language, vs.
  generating each clip once via a TTS engine and freezing/git-tracking the
  output. The TTS-once approach is flagged as a live option worth
  evaluating — it could produce a wide language set cheaply without
  recording each by hand, while still avoiding 101's rejected live-TTS
  runtime dependency (the clip is frozen at authoring time, not
  synthesized per-invocation). Voice quality/naturalness per language,
  licensing of any TTS engine/voices used for authoring, and how new
  languages get added later (one-off local synthesis + commit, vs. an
  authoring script) are all unresolved.
- **Fallback behavior**: what plays when a user's selected/detected
  language has no clip yet (silence, English fallback, skip notification
  entirely)?
- **Message wording per language**: 101's English wording sidesteps
  interpolating the actual configured hotkey by using a fixed phrase
  ("Stop recording to finish."). Whether every language pack must use the
  same fixed-phrase strategy, or whether some languages could interpolate
  the real shortcut, is open.

## 4. Non-Goals For This Ticket

- No code changes, no clip recording/generation, no new config fields.
- No decision on which sourcing method (recorded vs. frozen-TTS) ships.
- No decision on which languages are in scope for a first pass — that is
  itself one of the open questions above.
- Does not block or gate 101, which ships English-only per its own spec.
