# Harnez Issues

When this project has an `issues/` tracker, use `harnez find` / `harnez issues` to search, allocate, and update tickets.

- `harnez find -d <repo> issues -a status:open` lists active tickets.
- `harnez find -d <repo> issues next` reports the next free ticket number.
- `harnez issues new -d <repo> "<title>"` reserves a number; fill the printed file, run `harnez index -d <repo>`, and commit ticket/index changes.
- Cloud agents use `harnez issues new --remote -d <repo> "<title>"` (automatic when `CLAUDE_CODE_REMOTE` is set): authenticated gh on the github remote allocates permanent GH IDs. No host reservation or local fallback; local numbering ignores GH IDs.
