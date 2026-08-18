# 001: Website Integration and Public Documentation

- **Status:** In Progress
- **Project:** `voxi` / `ubunatic.com`

## Context
Following the architectural extraction from `harnez` (issue 029), `voxi` is now a standalone project. An initial landing page has been created in `website/index.html` and synced to `ubunatic.com/voxi`.

**2026-08-18 update**: `website/` was rebuilt as a `VOXI(1)` man-page-style reference
(`index.html` + external `index.css`/`index.js`/`logo.svg`) covering every current CLI
command, including `voxi bench` and the spec-driven model registry
(`requires_gpu`/`cpu_fallback`, per-model stop-words) added the same session. Synced via
`uman website sync voxi`. Goals 1-3 below remain open.

## Goals
1. Flesh out interactive audio demos, latency graphs, and animated typing reels on the `voxi` website.
2. Provide pre-built packaging recipes (RPM/deb/PKGBUILD) and install scripts for Fedora and Arch Linux.
3. Integrate comprehensive guides for GNOME Shell extension installation and PipeWire audio setup.
