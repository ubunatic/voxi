# Telemetry Guide

Voxi records a private, append-only timeline of every Eager pipeline event
to `~/.local/share/voxi/eager-telemetry.jsonl` (or
`$XDG_DATA_HOME/voxi/eager-telemetry.jsonl`).
This guide explains how to read that data after a session, how to spot common
problems, and how to hand the data to an agent for deeper analysis.

No transcript text is ever stored — only timing, acoustic measurements, word
counts, and success flags.

---

## Quick-start: inspect the last session

```bash
# 1. What sessions ran in the last hour?
voxi telemetry query --view sessions --since $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)

# 2. Show every chunk for that session (copy the session ID from step 1)
voxi telemetry query --session <SESSION_ID>

# 3. Aggregate latency + outcome summary for the same window
voxi telemetry stats --since $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
```

The default `--view chunks` already gives the most useful picture:
queue delay, transcription time, typing delay, total latency, silence flag,
and word count per utterance — all in one line per chunk.

---

## Data model

### The telemetry file

```
~/.local/share/voxi/eager-telemetry.jsonl   (mode 0600, dir 0700)
```

Each line is one JSON event. Events for different concurrent pipeline drains
interleave freely; the query and stats commands correlate them for you.

### Events

| Event | Meaning |
|---|---|
| `mic_activated` | User gesture (Super+X) accepted by the daemon |
| `capture_started` | Audio device actually opened |
| `chunk_finalized` | Audio chunk sealed and sent to the ASR queue |
| `transcription_started` | The configured ASR engine begins processing the chunk |
| `transcription_completed` | The configured ASR engine finished; word count recorded |
| `typing_started` | Keystroke injection begins |
| `typing_completed` | Injection call returned (or stream accepted by `dotoolc`) |
| `mic_deactivated` | Stop gesture accepted |
| `capture_stopped` | Audio device closed |

**Chunk events** also carry acoustic metrics derived from raw PCM:
`duration_secs`, `pcm_bytes`, `mean_rms`, `peak_rms`, `voiced_ratio`,
`probable_silence`.

### IDs

- **Session ID** — stable across the full mic-on → mic-off lifecycle, including
  background drains that complete after the next session starts.  Format:
  `20260905T104212.464455555Z-000004` (UTC timestamp + monotonic counter).
- **Chunk ID** — `<session_id>/<n>` where `n` is the utterance number within
  the session.

### Derived latencies (computed by `query` and `stats`)

| Field | Definition |
|---|---|
| `queue` | chunk_finalized → transcription_started |
| `transcription` | transcription_started → transcription_completed |
| `typing-delay` | transcription_completed → typing_started |
| `total` | chunk_finalized → typing_completed |
| `capture startup` | mic_activated → capture_started |
| `capture shutdown` | mic_deactivated → capture_stopped |

---

## Step-by-step session analysis

### 1. Find the session

```bash
voxi telemetry query --view sessions --since 2026-09-05T12:00:00+02:00
```

Example output:

```
Telemetry sessions: 0 malformed row(s), 0 newer-schema row(s) skipped
session=20260905T104212.464455555Z-000004 chunks=14 activated=2026-09-05T12:42:12.464455555+02:00 deactivated=2026-09-05T12:44:15.123456789+02:00
```

Copy the session ID. If you just finished recording you can also use
`--since` with the approximate start time and omit `--view sessions` to
go straight to chunks.

### 2. Check the chunk timeline

```bash
voxi telemetry query --session 20260905T104212.464455555Z-000004
```

Each line is one utterance:

```
session=…-000004 chunk=…/2  queue=0.5ms  transcription=1280.0ms  typing-delay=2.5ms  total=1286.9ms  post-deactivation=false  silence=false  words=9
session=…-000004 chunk=…/13 queue=633.0ms transcription=2010.9ms  typing-delay=1.5ms  total=2648.9ms  post-deactivation=false  silence=false  words=1
```

**What to look for:**

- **High `queue`** — the transcription worker was busy with a previous chunk.
  A single value up to ~1 s is normal for back-to-back speech. Sustained high
  queue delays mean the pipeline is falling behind.
- **High `transcription`** — model load or CPU contention. Compare chunks:
  if the first chunk of a session is slow and later ones are faster the model
  was loading cold. Values above ~3 s for short utterances warrant investigation.
- **`total` near or above your chunk boundary** — words typed after the next
  chunk was already captured, which can feel laggy.
- **`words=-`** — the chunk has no `transcription_completed` event yet (or the
  transcription stage is missing). A partial chunk at the very end of a session
  is normal if you stopped mid-utterance.
- **`silence=true`** — chunk was probably silence; the ASR engine may have produced
  hallucinated text. Check if unexpected words appeared around that time.
- **`post-deactivation=true`** — chunk was transcribed or typed after you
  stopped the mic. Normal for the last 1–2 chunks; more than that suggests a
  backlog.

### 3. Get aggregate stats for the session

```bash
voxi telemetry stats --session 20260905T104212.464455555Z-000004
```

Example output from a healthy ~2 minute session:

```
Telemetry: 1 sessions, 14 chunks (11 complete, 3 partial)
Backlog: 0 chunks processed after mic deactivation
Audio: 54.82s, 1754240 bytes, 0 probable-silence chunks, 132 transcript words (silence rate 0.0%), 2.41 words/audio-second, transcription RTF 0.487
Outcomes: transcription 13 ok/0 failed; typing 11 ok/0 failed
Latency capture startup     n=1 avg=4.3ms  p50=4.3ms  p95=4.3ms  max=4.3ms
Latency capture shutdown    n=1 avg=2.7ms  p50=2.7ms  p95=2.7ms  max=2.7ms
Latency queue               n=13 avg=190.5ms p50=0.5ms  p95=961.2ms max=1256.0ms
Latency transcription       n=13 avg=1697.1ms p50=1281.1ms p95=2650.1ms max=4093.2ms
Latency typing delay        n=11 avg=2.0ms  p50=2.0ms  p95=2.5ms  max=3.1ms
Latency typing              n=11 avg=3.8ms  p50=3.4ms  p95=5.0ms  max=8.2ms
Latency total chunk-to-type n=11 avg=1735.7ms p50=1349.5ms p95=2662.4ms max=3654.4ms
Input quality: 0 malformed row(s), 0 newer-schema row(s) skipped
```

**Healthy baseline (local default engine on a modern laptop):**

| Metric | Healthy | Investigate |
|---|---|---|
| Transcription RTF | < 0.6 | > 1.0 (slower than real-time) |
| queue p95 | < 100 ms | > 1 s |
| transcription p50 | < 1.5 s | > 3 s |
| typing delay | < 10 ms | > 50 ms (`dotoolc` stall?) |
| Silence rate | < 10% | > 30% (VAD threshold too low?) |
| Partial chunks | ≤ last 1–2 | Many partials = pipeline crash |
| Backlog | 0–2 chunks | > 5 = overloaded |

### 4. Drill into raw events for one chunk

When a specific chunk looks wrong, inspect the raw event sequence:

```bash
voxi telemetry query --view events \
  --session 20260905T104212.464455555Z-000004 \
  --chunk 20260905T104212.464455555Z-000004/13
```

This shows every timestamped event for that chunk in order, so you can see
exactly when each pipeline stage fired and whether any stages are absent.

### 5. Filter by symptom

```bash
# All chunks processed after you stopped the mic
voxi telemetry query --post-deactivation true --session <SESSION_ID>

# Only failed transcription or typing
voxi telemetry query --success false

# Only silence-flagged chunks
voxi telemetry query --silence true

# Last N chunks regardless of session
voxi telemetry query --limit 20
```

### 6. Machine-readable output for scripting or agents

Both commands accept `--format json`. The JSON output is stable and suitable
for piping to `jq`, writing to a file, or passing to an agent:

```bash
voxi telemetry stats --format json | jq '.transcription_rtf, .total_to_type.p95_ms'

voxi telemetry query --format json | jq '.chunks[] | select(.queue_delay_ms > 500)'
```

---

## Diagnosing the "late word" problem

A single word typed just after you stopped speaking is usually a normal
post-deactivation drain: audio was already queued when you pressed stop and
the pipeline finished it. Confirm with:

```bash
voxi telemetry query --post-deactivation true --session <SESSION_ID>
```

If the result shows only the last 1–2 chunks with `total` values that overlap
the mic-deactivated timestamp, the pipeline behaved correctly.

If you see many post-deactivation chunks, check for a `dotoolc` typing stall:

```bash
voxi telemetry query --view events --session <SESSION_ID> | grep typing
```

A large gap between `typing_started` and `typing_completed` points to the
typing injector, not the ASR engine.

If you see no post-deactivation flag but a late word still appeared, look at
the last chunk's `queue` value — a backlog in the transcription queue can delay
output by seconds even within the same session.

---

## Asking an agent to analyse a session

Copy this prompt and fill in the session ID and time window:

```
Run: voxi telemetry stats --session <SESSION_ID>
Then: voxi telemetry query --session <SESSION_ID>
Then: voxi telemetry query --view events --session <SESSION_ID>

Identify:
1. Any chunks with queue delay > 500 ms or transcription > 3 s.
2. Any post-deactivation chunks beyond the last two.
3. Any silence-flagged chunks that produced unexpected words.
4. Any failed transcription or typing stages.
5. Overall transcription RTF and whether it is within healthy bounds.
Report findings with the chunk IDs and timestamps involved.
```

For a broader time window without a specific session:

```
Run: voxi telemetry stats --since <RFC3339_START> --until <RFC3339_END>
Then: voxi telemetry query --since <RFC3339_START> --until <RFC3339_END>
Summarise session count, total audio, RTF, and any anomalies in queue or
transcription latency.
```

---

## Common flags reference

| Flag | Applies to | Default | Notes |
|---|---|---|---|
| `--since` / `--until` | both | (all time) | RFC3339 with timezone, e.g. `2026-09-05T12:00:00+02:00` |
| `--session` | both | (any) | filter to one session |
| `--chunk` | query | (any) | filter to one chunk |
| `--view` | query | `chunks` | `events`, `chunks`, or `sessions` |
| `--event` | query (events only) | (any) | e.g. `transcription_completed` |
| `--success` | query | `any` | `true` or `false` |
| `--silence` | query | `any` | `true` or `false` |
| `--post-deactivation` | query | `any` | `true` or `false` |
| `--limit` | query | 100 | max rows printed |
| `--max-events` | both | 100000 | memory guard; raise for long histories |
| `--format` | both | `text` | `json` for scripts/agents |
| `--path` | both | XDG default | override for offline analysis |

---

## Database location and retention

The telemetry file is append-only and never automatically pruned.
A typical one-hour session with continuous speech produces around 200 KB.

```bash
# File location
echo "${XDG_DATA_HOME:-$HOME/.local/share}/voxi/eager-telemetry.jsonl"

# Total event count
wc -l "${XDG_DATA_HOME:-$HOME/.local/share}/voxi/eager-telemetry.jsonl"

# Last 10 raw events
tail -10 "${XDG_DATA_HOME:-$HOME/.local/share}/voxi/eager-telemetry.jsonl" | jq .
```

Manual rotation: move or truncate the file while `voxi-agent` is not running
(`systemctl --user stop voxi-agent`). The daemon creates a fresh file on next
write.
