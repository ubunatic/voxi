# 127 — Research: hook into Claude Code's / agy's own audio capture / transcription for voxi

**Status**: Closed — Not recommended
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Research
**Related**: [[126-use-lmcoder-openai-compatible-transcription-api-as-asr-backend-target]]

---

## 1. Problem & Motivation

Claude Code has its own voice/audio input capability (voice dictation mode), and `agy`
separately has a `/voice` command. Both likely call some transcription API internally
rather than shipping a fully local model. If that's true, there might in principle be
a way to piggyback on either as another transcription source for `voxi` — similar in
spirit to [[126]] (using a self-hosted `lmcoder` transcription endpoint), but riding on
infrastructure/credentials the user already has through their Claude/agy subscription
instead of running a local model.

This ticket is pure research — no implementation is scoped or assumed. It should
answer feasibility questions before any decision to build something.

## 2. Research Questions

1. Does Claude Code's voice/audio input mode call a remote API at all, or is it fully
   local (e.g. on-device Whisper)?
2. What does `agy`'s `/voice` command actually do — is it a wrapper around the same
   underlying provider(s) Claude Code/Codex use, or its own independent integration?
3. If remote in either case: is the transport a documented, stable, public API, or an
   internal/undocumented one?
4. Would reusing either be a *legitimate*, supported integration path, or would it
   require treating a private backend as if it were a public API?

## 3. Acceptance Criteria (for closing as research, not implementation)

- Written finding on whether Claude Code's audio capture and `agy`'s `/voice` command
  call a remote API, and whether that API is a realistic, supportable integration
  target for `voxi`.
- If a viable, legitimate hook point is found, a follow-up implementation ticket is
  filed with a concrete design; if not viable/not worth pursuing, this ticket is
  closed with the reasoning recorded.
- No code changes required to close this ticket — it is satisfied by a documented
  answer.

## 4. Non-Goals

- Does not commit to building this integration — see [[126]] for the concrete,
  scoped ASR integration path (self-hosted `lmcoder`), which remains the primary plan.
- Does not scope or endorse reverse-engineering, MITM interception, or unofficial
  reimplementation of either vendor's private protocols. If those transports turn out
  to be internal/undocumented, that alone is sufficient grounds to close this ticket
  without pursuing it further.

## 5. Findings (2026-09-16)

1. **Claude Code voice dictation is remote, not on-device**, over a transport distinct
   from normal chat traffic, gated to Claude.ai-account auth, and does not consume
   tokens or count toward usage limits.
2. **`agy`'s `/voice` is its own independent integration**, not a wrapper around
   Claude Code's or Codex's dictation.
3. **Both ride on internal, undocumented, unstable backends** — not public, versioned,
   documented APIs with a stability contract. Neither vendor offers a supported way to
   call their voice-dictation backend directly, outside their own official client.

## 6. Recommendation: Not Recommended

Closing this research as **not recommended** to pursue further. Both candidate hook
points are private, undocumented, account-authenticated backends internal to their
respective products, not public APIs `voxi` could integrate against on any stable or
supportable basis. Building against either would mean depending on an interface that
can change or break without notice, and that neither vendor intends for third-party
use — not a foundation `voxi` should build on, regardless of technical feasibility.

[[126]] (self-hosted `lmcoder`) remains the primary ASR integration path — it's also
preferable independent of this finding, since it keeps audio processing under the
user's own infrastructure rather than routing through a third-party product's internal
backend.

No further action planned. Reopen only if either vendor ships a public, documented,
stable API for this capability.
