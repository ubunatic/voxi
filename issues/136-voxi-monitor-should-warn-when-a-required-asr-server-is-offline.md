# 136 — voxi monitor should warn when a required ASR server is offline

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 135 (monitor showed the wrong model), 134 (R2T2/llama-server), 126 (agy whisper-server)

---

## 1. Problem

The `openai-transcribe` engine talks to an external HTTP server that voxi does
not start (deliberately — see 134 §6 M2). When that server is not running,
dictation fails silently from the user's point of view: chunks are rejected and
nothing is typed. The only evidence is per-chunk text in `voxi chunks list`:

```
transcribe_error: openai-transcribe: POST http://127.0.0.1:18131/v1/audio/transcriptions:
  dial tcp 127.0.0.1:18131: connect: connection refused
```

Observed live on 2026-09-22 (verified): 7 consecutive chunks lost this way while
`r2t2-confucius4` was the selected model and llama-server was not running.

`voxi monitor` is where a user looks when dictation misbehaves, and it currently
shows only the model name (`render.go:417`, `Snapshot.ActiveModel` at
`collector.go:51`). It reports nothing about the backend the model needs.

## 2. /goal

`voxi monitor` makes an unreachable ASR backend obvious at a glance. For the
active model, monitor resolves its engine and, for HTTP engines, its endpoint
(per-model `base_url`, else the global `openai_asr_base_url`, else the default —
the same order as `resolveOpenAIASRBaseURL` in `internal/eager/openai_transcribe.go`),
probes it cheaply, and shows a clear OFFLINE warning with the endpoint when it
does not answer. Engines with no server (whisper via voxtype, cohere-transcribe
via crispasr) show no such warning and are not probed. The probe never blocks
or slows the TUI refresh. Tests cover reachable, unreachable, and
not-applicable-engine cases.

## 3. Notes / Uncertainties

- Re-verify against current code first: 135 just changed `detectActiveModel`
  (`collector.go:278`), and `base_url` is new in `spec/models.yaml`.
- Monitor currently reports only a model name; it likely needs the engine too,
  which 135 §3 already suggested. Sharing one resolver with the eager path is
  better than a second copy — Go must not duplicate spec values (`docs/Spec.md`).
- Probe shape is open: a short-timeout `GET /health` (llama-server answers
  `{"status":"ok"}`) vs. a plain TCP dial. `/health` is not guaranteed on every
  OpenAI-compatible backend, so a dial may be the more portable check. Decide
  with evidence and write down which and why.
- Cache the result between refreshes; do not probe on every frame.
- Related UX gap, out of scope here: nothing restarts these servers. If R2T2
  becomes a regular engine it needs a systemd user unit carrying the capped
  command from 134 §5.

## 4. M2 — show the backend when it IS online (2026-09-22)

M1 landed as `9280686`: a warning appears only when the endpoint is unreachable.
User feedback on the live TUI: silence is ambiguous — "online" and "never
probed" look identical, and the health box says nothing about the ASR server at
all.

/goal for M2: the `[d] active daemons & health` box always carries one line for
the active model's backend, in the same style as the other daemons there
(`voxi-modifierd (PID …, 4.3 MB) · dotoold (…)`). Online shows the endpoint and
that it answered; offline shows the endpoint and that it did not, keeping M1's
warning. Engines with no server (whisper/voxtype, cohere-transcribe/crispasr)
state that plainly instead, e.g. "asr: crispasr (no server)", so the line is
never missing and never misleading.

Notes:
- Reuse `checkASRBackend`/`probeASRBackend` and the 5 s cache from M1
  (`internal/monitor/collector.go`); the snapshot needs an online/offline/
  not-applicable state, not just a warning string.
- Where possible show something useful alongside, e.g. the port, but do not add
  a second probe or an HTTP round trip to get it.
- Unrelated wording bug seen in the same screenshot: the `[s] voice & speed` box
  labels the GPU backend as `engine: AMD Radeon Vulkan 1.4`. "Engine" now means
  the ASR engine (whisper / cohere-transcribe / openai-transcribe) everywhere
  else, so this label should change (e.g. `accel:`). Fix it here while in the
  file.

## 5. M3 — the line is invisible in practice (2026-09-22)

M2 (`312f86c`) appends the ASR state to the END of the daemons line
(`render.go:537`, `strings.Join(procSummaries, " · ") + " · " + asrLine`). On a
real terminal that line is already truncated mid-process
(`voxi-modifierd (…) · dotoold (…) · harnez (PID …`), so the ASR state is cut
off and the user never sees it. Verified from a live TUI capture.

/goal: the ASR backend state is visible in the `[d] active daemons & health`
box at a normal terminal width, i.e. it is its own line rather than a suffix on
an overflowing one, and it is not truncated away. Prefer it FIRST in that box —
it is the line that explains why dictation is failing.

Notes:
- `formatASRBackendLine` (`render.go:560`) already produces the text; this is
  about placement, not wording.
- The box is width-aware; check how other lines are truncated
  (`asr.StripANSI`, `:253`) and keep the new line inside the same discipline so
  it degrades gracefully rather than wrapping the box.
- Same screenshot, minor: the `[s]` status line truncates the model to
  `r2t2-confu...`. Consider showing the model alone on its line, or shortening
  the `(neither / …)` prefix. Optional.
