---
title: Markdown Conventions
weight: 64
---

<!-- harnez:bundled -->
# Markdown Conventions

## File naming

**Evergreen docs** — content that doesn't expire (references, conventions, guides):

```
docs/Permissions.md
docs/Go.md
docs/Worktrees.md
```

Use **PascalCase**. No dates, no issue numbers.

**Ephemeral docs** — issues, reports, reviews, ADRs, changelogs:

```
issues/001-summary-of-issue.md
reviews/2026-06-auth-refactor.md
```

Use **kebab-case** with an optional numeric or date prefix for ordering.

## Content

- One `#` title per file, matching the filename concept
- Prefer bullet lists over tables for sparse data
- Keep files token-efficient: no redundant prose, no section headers that restate the bullet below them
