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

## Terminal hyperlinks (OSC 8): label text by context

Always prefer an OSC 8 hyperlink over a raw path. The *visible label* rule depends on context:

- **In tables**: use a short/truncated label as the visible text; put the full path in the OSC 8
  target. A long filename as the label blows up column width and breaks alignment.
- **In flowing prose**: show the **full path** as the visible text, with the OSC 8 hyperlink
  layered on for click-to-open. The user needs to visually scan and drag-select/copy the path
  directly — a shortened label defeats that. Truncate only if the full path is genuinely too long
  for inline prose.

## Diagrams: chat vs. docs

### In chat / pairing conversations:
- Do not emit ` ```mermaid ` blocks unless asked; terminals and chat UIs may render them as raw markup
- For simple flows, use `1. 2. 3. if X goto 2.; 4. ...` and skip the diagram completely
- Use compact ASCII box-and-arrow diagrams inside a ` ```text ` fence instead:
  - **Dynamic terminal sizing**: Adapt diagram width to the terminal/pane width $W$.
  - **5% Right margin buffer**: Always leave at least a **5% buffer** on the right side ($\text{width} \le 0.95 \times W$) so terminals never auto-wrap and break rectangular borders.
  - **Max width ceiling (120 cols)**: Cap diagram width at **120 columns** max, even on ultra-wide terminals.
  - **Vertical stacking**: If a flow exceeds the safe width, stack boxes vertically rather than spreading wide horizontally.

  ```text
  ┌─────────────┐       ┌─────────────┐
  │     Foo     │ ────> │     Bar     │
  └─────────────┘       └─────────────┘
  ```

### In MD Files/Evergreen `docs/`
 ` ```mermaid ` diagrams remain allowed and encouraged for formal architecture/sequence flows.
