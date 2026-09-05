# 061 — Telemetry Analytics Statistics and Query Commands

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [060 correlated Eager telemetry](060-correlated-eager-pipeline-telemetry-for-mic-to-type-latency.md), [056 end-to-end stress testing](056-end-to-end-stress-session-testing-with-noise-and-load.md), [voxi monitor](../internal/monitor/), [internal/telemetry](../internal/telemetry/)

---

## Problem and Motivation

Issue 060 established a durable, correlated JSONL event timeline, but users must
currently inspect and join its rows manually. Voxi needs basic querying and useful
statistics so the telemetry can answer operational questions directly: how long each
pipeline stage takes, whether transcription or typing continues after microphone
deactivation, how much work is queued, and whether silence-like chunks correlate with
late or rejected output.

Add read-only CLI capabilities that turn the telemetry database into inspectable raw
events, correlated session/chunk records, and concise aggregate reports. The commands
must remain useful in terminals and scripts and must not expose transcript content.

## Scope and Design Questions

- Provide a discoverable command family such as `voxi telemetry query` and
  `voxi telemetry stats`; choose final Cobra naming consistently with the existing CLI.
- Query by time range, session ID, chunk ID, event type, success/failure, probable
  silence, and whether processing continued after mic deactivation. Support a bounded
  result count and deterministic ordering.
- Correlate event rows into per-session and per-chunk lifecycle records. Derive capture
  startup/shutdown latency, chunk-to-transcription queue delay, transcription duration,
  typing delay/duration, total chunk-to-type latency, and post-deactivation work.
- Report useful counts and distributions, including sessions/chunks, accepted or failed
  stages, probable-silence rate, audio duration/bytes, word counts, throughput/RTF where
  derivable, and latency summaries. Prefer robust percentiles over averages alone.
- Offer a human-readable default plus a stable machine-readable output such as JSON;
  CSV or JSONL export is optional unless an established Voxi convention favors it.
- Handle missing, partial, malformed, duplicated, or newer-schema events explicitly.
  An interrupted pipeline must yield a partial record rather than corrupting the whole
  report.
- Default to the XDG telemetry path from issue 060, with an explicit input-path option
  for tests and offline analysis.
- Keep queries read-only and memory-bounded for a telemetry file that may have grown
  large. Decide whether a streaming scan is sufficient before introducing an index or
  migrating away from JSONL.

Retention, automatic rotation, dashboards/TUI visualization, and changes to telemetry
collection are adjacent concerns and should only enter this ticket if required for a
safe, usable query implementation.

## Acceptance Criteria

- A user can list/filter raw telemetry events and inspect a correlated session or chunk
  without manually joining JSONL rows.
- A default stats report exposes pipeline latency, backlog/post-close processing,
  silence/audio characteristics, word counts, and success/failure information over a
  selectable time range.
- At least one stable machine-readable output mode is documented and tested, including
  its behavior for absent lifecycle stages.
- Time parsing, filters, correlation, derived durations, percentile calculations, and
  deterministic ordering have focused tests with synthetic event fixtures.
- Malformed rows and incomplete sessions are handled predictably and surfaced to the
  user without silently producing misleading aggregates.
- Commands never modify the telemetry database and do not print transcript content.
- CLI help and relevant user documentation explain the data source, filters, derived
  metrics, timestamp/timezone behavior, and the `dotoolc` typing-completion limitation.
- `go test ./...` and repository checks pass. Because this is an on-demand command,
  `make install` is sufficient unless implementation also changes daemon code.

## Verification Guidance

- Build deterministic fixtures covering normal sessions, overlapping session drains,
  transcription/typing after deactivation, silence-like chunks, failed stages, missing
  events, malformed rows, and multiple schema versions.
- Assert exact correlated records and derived durations, not only successful command
  execution. Golden output may supplement structured assertions but should not replace
  them.
- Exercise the installed command against a temporary fixture database and confirm both
  human-readable and machine-readable output.
