# Search Practice

<!-- harnez:bundled -->

Use Harnez finders for code and documentation discovery before broad shell searches.

## Search

- `harnez find code "query"` searches the repository through configured finders.
- `harnez find docs "query"` searches documentation; built-in fuzzy matching remains available.
- Use `--via <name>` to select one configured finder, `-k N` to limit results, and `--json` or `--jsonl` for structured output. MCP clients can use `harnez_find` with the same scopes and result fields.
- A result contains `path`, `line`, `title`, `snippet`, `score`, and `kind`. Scores express merged rank and are not comparable across separate searches.
- Check finder status diagnostics when results may be partial. A timeout, backend error, or unindexed neus root appears separately from successful results.
- Do not repeat the same query to refresh an index. neus indexes itself on first use when its root is not indexed.

## Use `rg` directly

Use `rg` for an exact string or regex, or when you already know the file or directory to inspect. Harnez's `rg` fallback is automatic when neus is unavailable; direct `rg` is useful when its precise matching behavior is needed.

## Configuration

Finder definitions live in `~/.harnez/config.yaml`; a repository `config.yaml` entry overrides a user finder with the same name. Define `name`, `scope` (`code` or `docs`), `command` as an argv list, and a positive `timeout`. Templates may use `{query}`, `{root}`, and `{k}`. The query is passed as an argument and is never evaluated by a shell.

<!-- harnez:stop -->
