---
kind: sprint
issue: 223
date: 2026-11-23
---

# Two-Agent Lean Sprint: Replacement Rule Categories (Issue 223)

## 1. Bounded Discovery (Searcher: `searcher-223`, `claude:claude-haiku-5-5`, 29s)

- `Replacement` struct in `internal/feedback/replacement.go:24-28` (`From`, `To`, `FixedCase`).
- CLI subcommand is `voxi feedback replacement` (`add`, `list`, `remove`, `cleanup`) in `internal/feedback/command.go:176-270`, not `replace`; no `update` subcommand exists.
- `internal/eager/eager.go:390,512` applies all rules via `ApplyReplacementsWithAudit`.
- `internal/bench/bench.go` and `internal/sample/` currently apply no replacements at all; raw ASR output is scored.
- `docs/VoiceInput.md:154-165` documents `voxi feedback replacement`.

## 2. Milestones

- **M1 (Schema, CLI & Docs)**: Delivered in `ae920e3` (`go test -race ./internal/feedback/...` ok, `make lint` exit 0). Reviewed by `rev-223-m1` (`agy:gemini-4-neon`, 40s, severity `low`: two minor test-strength gaps queued as M2 pre-work).
- **M2 (Bench Filtering & Verification)**: Delivered in `c32c43a` (M1 pre-work test hardenings + `voxi bench --replacements none|jargon|all` in `cmd/voxi/main.go`, `internal/bench/bench.go`, `internal/bench/replacements.go`, `internal/bench/replacements_test.go`). Full `make test` (`go vet ./... && go test ./...`) passed across all packages with zero failures; `make install` succeeded; all 10 existing rules in `~/.config/voxi/replacements.json` categorized as `[jargon]`. Reviewed by `rev-223-m2` (`agy:gemini-4-neon`, 48s, severity `none`).

## 3. Outcome & Efficiency

- **Agents used**: `searcher-223` (`claude-haiku-5-5`, 29s), `dev-223` (`claude-haiku-5-5`, M1 2m37s + M2 22m36s + 46s), `rev-223-m1` (`gemini-4-neon`, 40s), `rev-223-m2` (`gemini-4-neon`, 48s).
- **Issue 223**: Closed after M2 verification and live CLI check.
