# Harnez Subagents

- Prefer loaded `mcp__harnez__*` tools for lifecycle actions; otherwise use `harnez agent` via Bash.
- A requested model such as `terra:low` or `luna` is a Harnez agent model; dispatch it with `harnez agent start --model <name>`.
- Track and terminate every background task and subagent before ending a session.
