# Harnez Output

Use `/mode` to switch ConciseMode terseness level (lite, std, ultra, off). Preserve code, diffs, tool parameters, and command syntax verbatim at every level.
Do not promise behaviour in future sessions; name a durable safeguard added or proposed and where it lives.

## Canonical Terminology
- **context usage**: Tokens read on the last call of a turn (full turn input incl. cached tokens). Avoid: "context size", "context length", "context used". <!-- harnez-lint:ignore glossary -->
- **context window**: The model maximum input token capacity. Avoid: "context size", "context length". <!-- harnez-lint:ignore glossary -->
- **compaction limit**: Threshold triggering automatic compaction.
- **cached re-read**: Cached tokens billed across all turn calls (cumulative cost, not a context measure).
