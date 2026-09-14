# Study: `agy` cleanup latency — session strategies, fixed cost, and a local-model timeout fix found along the way

<!-- harnez:topic: agy (Antigravity) subprocess latency for LLM transcript cleanup: cold-spawn vs -c vs persistent stream-JSON, the ~13k-token fixed tool-schema tax, and the local-model near-miss timeout finding that came from live dictation during the same investigation -->

**Date**: 2026-09-15
**Scope**: Follow-up canary work for [Issue 122](../../issues/122-add-gemini-flash-3-7-support-as-llm-cleanup-model.md)
(Gemini via `agy` as an opt-in cleanup backend) and [Issue 121](../../issues/121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)
(local-model cleanup timeout tuning). Both tickets converge on the same
question — can an LLM cleanup call reliably finish inside a sub-2s budget —
so this study covers both in one place.
**Related**: [docs/LLMTranscriptCleanup.md](../LLMTranscriptCleanup.md),
[docs/studies/2026-09-13-llm-cleanup-evaluation.md](2026-09-13-llm-cleanup-evaluation.md),
[Issue 123](../../issues/123-retest-llm-cleanup-models-against-current-yaml-request-format-and-2-5s-timeout.md)
(follow-up retest ticket filed from this session)
**Status**: Investigation complete for this pass; issue 123 tracks re-running
the real 5-case evaluation against the resulting 2.5s budget.

---

## 1. Method

Direct shell canaries against the installed `agy` CLI (not through Voxi),
per `docs/Canary.md` — probe the external mechanism before trusting a design
built on it. Three session strategies were compared for per-chunk cleanup
latency:

1. **Cold per-call spawn**: `agy --model=... -p "..."`, one fresh process
   per call — the design `cleanWithAGY` already used.
2. **`agy -c` (continue most recent conversation)**: repo/dir-aware (needs a
   prior conversation to already exist in that directory), tested by
   running three separate `agy -c -p "..."` invocations in a fixed test
   directory.
3. **Persistent stream-JSON session**: one long-lived
   `agy --print='' --input-format=stream-json --output-format=stream-json`
   process, fed one NDJSON line per turn:
   `{"event":"user","message":{"role":"user","content":"..."}}`.

The stream-JSON input schema was not documented in `agy --help`; it was
discovered by trial and error — `--print=<value>` always consumes the next
token as the prompt even with `--input-format=stream-json` set, so the
correct invocation is `--print=''` (empty prompt) to signal "read prompts
from stdin instead." The first NDJSON line tried, `{"event":"prompt","prompt":"hi"}`,
was accepted syntactically but silently ignored ("unsupported stream input
message event"); `{"event":"user","message":{"role":"user","content":"hi"}}`
is the working shape.

## 2. Findings

### 2.1 Per-call latency by strategy

| Strategy | Wall-clock per call | Notes |
|---|---|---|
| Cold spawn | ~3.3-9.1s | 3.34s for a bare "hi"; issue 122's original canary saw 9.1s (text mode), 7.3s (JSON), 4.5s (stream-JSON) for one-shot calls — variance likely reflects provider-side latency jitter, not the CLI |
| `agy -c` (3 separate processes, same dir) | ~3.5-4.2s each | **No latency win over cold spawn.** Each call is still a brand-new process paying full startup/auth/connection cost; `-c` only replays conversation *content*, not a warm connection |
| Persistent stream-JSON (3 turns, 1 process) | ~1.2-2.0s per turn (turn deltas: 1.2s, 2.0s, 1.4s) | Best of the three — avoids repeated process-spawn/connection overhead by keeping one process warm |

**Conclusion**: only the persistent stream-JSON session actually buys
latency. Spawning a new process per chunk — whether cold or via `-c` — pays
the same ~3.5-4s connection tax regardless of conversation history.

### 2.2 A fixed cost no session strategy avoids: ~13,180-token tool schema

Every design tested, from every working directory tried (including an empty
`/tmp` scratch dir with zero project docs), carried a **~13,180-token
baseline** on the *first* turn of any conversation — before any actual
cleanup content. This is the JSON schema for `agy`'s full default tool
loadout (30+ tools: browser automation, subagents, MCP, `write_to_file`,
etc.), sent as part of the system prompt on every turn regardless of
whether the task needs any of them.

This directly contradicts an earlier self-reported estimate in
[`2026-08-31-agy-clean-session-system-prompt-audit.md`](2026-08-31-agy-clean-session-system-prompt-audit.md)
of "~3,000-4,500 tokens total" for a clean-baseline `agy` session — that
number was the *model's own introspective estimate* ("self-estimated, not a
real tokenizer," per that study's own caveat), not a measured value. The
real measured `usage.input_tokens` from `agy`'s own JSON output is ~13,180,
roughly 3x the earlier self-report. **Lesson for future agy studies: prefer
`usage.input_tokens` from actual API responses over an agent's own
self-reported prompt-size estimate — the two can disagree by a wide
margin.**

No opt-out was found: `agy agent`/`agy agents` returns an empty list (no
named lighter-weight agents to select via `--agent`), and `--help` exposes
no flag to strip the tool schema. Reverse-engineering the binary for hidden
flags was attempted briefly and abandoned as unproductive and inappropriate
for a closed binary — that path was not exhausted for a good reason, just a
correct one; a supported answer (docs, maintainer, changelog) would be the
right next step if this is worth pursuing further, not more binary
spelunking.

### 2.3 Context growth compounds the fixed cost, but is expected to self-limit

Across 3 turns in both the stream-JSON and `-c` tests, `input_tokens` grew
13k → 26k → 40k — each turn resends the full prior conversation. Per the
user's own correction during this session, `agy` auto-compacts context
server-side, so this growth is not expected to be unbounded in a real
long-lived session; it just was not exercised long enough in this canary to
observe a compaction event.

### 2.4 The local-model finding this investigation surfaced along the way

While debugging the `agy` path, a live dictation session (not a synthetic
`stress-ng` load, per issue 121's original verification plan — just an
already-running macOS-in-QEMU VM and a normal Firefox session) reproduced
issue 121's local-model timeout problem organically: 3 of 4 real chunks hit
`fallback_reason: timeout` against the local `qwen3-4b-instruct-2507-q4`
model's 1.5s budget.

Crucially, new `LLMCleanupRecord.ElapsedMS` telemetry (added this session,
see §3) showed the *successful* calls in the same run landing at
**1,338-1,500ms — within 84-162ms of the deadline**. This reframed the
problem: the local model is not failing, it has near-zero headroom even
under ordinary background load, not just synthetic contention. This is a
budget problem, not a capability problem, and directly motivated raising
`llmCleanupTimeout` from 1.5s to 2.5s (issue 121 §6). The adaptive-skip and
load-aware-gating options issue 121 originally proposed remain unbuilt;
they may still be warranted for the tail of chunks that miss even the new
2.5s budget, but the low-effort fix (raise the budget) came first because
the near-miss data justified it directly.

## 3. What changed in the codebase as a direct result

- `internal/chunks/chunks.go`: added `LLMCleanupRecord.ElapsedMS` (wall-clock
  milliseconds for the cleanup call, regardless of outcome) — previously
  only `FallbackReason` was recorded, which could show *that* a call timed
  out but not *how close* it came when it didn't.
- `internal/eager/eager.go`: extracted the previously-duplicated
  `1500*time.Millisecond` literal (three call sites) into a single
  `llmCleanupTimeout` constant, shared by both `cleanWithLLM` (local-HTTP)
  and `cleanWithAGY`, and raised it to 2.5s.
- Deployed via `make restart-service` (both changes land in
  `internal/eager`, which the live `voxi-agent.service` daemon runs).

## 4. Open follow-up

[Issue 123](../../issues/123-retest-llm-cleanup-models-against-current-yaml-request-format-and-2-5s-timeout.md)
tracks re-running the real 5-case cleanup evaluation against both backends
at the new 2.5s bound, using `ElapsedMS` to capture margin data (not just
pass/fail) for future tuning. This study's own conclusion — that no `agy`
session strategy measured is likely to clear even 2.5s once a realistic
(non-trivial) cleanup prompt and the ~13k-token fixed cost are both counted
— should be verified by that retest rather than assumed to still hold.

## 5. Process notes for future agentic sessions

- **A live, real dictation session was a more effective repro than the
  synthetic-load plan issue 121 originally proposed.** The ticket called
  for `stress-ng`; ordinary background desktop load (a VM, a browser)
  reproduced the same near-miss timeouts with zero setup. Worth trying the
  cheap repro first before reaching for a synthetic load generator.
- **Don't trust an agent's self-reported token/prompt-size estimate as a
  measured fact in a durable doc** — §2.2 found a 3x discrepancy between an
  earlier self-estimate and the real measured value from the same CLI's own
  JSON output. Flag self-estimates as such (the earlier study did, correctly)
  and prefer re-measuring over citing them as settled numbers in later work.
