# 030: Public Release Readiness & Publication Checklist

**Status**: Complete  
**Category**: Repository Hygiene & Public Release  

---

## 1. Assessment Overview

`voxi` is architecturally robust, has comprehensive internal documentation, passing test suites, and working build automation. Before publishing the repository publicly (Codeberg/GitHub), all hygiene, licensing, CI, and documentation gaps have been addressed.

### Readiness Verdict
- **Core Engine & Implementation**: **Ready** (clean Go code, passing unit tests, spec validation, systemd unit templates, GNOME shell extension included).
- **Repository Hygiene & Packaging**: **Ready** (AGPL-3.0-or-later LICENSE added, CONTRIBUTING.md created, CI workflow active, issue templates created, comprehensive README updated).

---

## 2. Gap Analysis & Checklist

### Phase 1: Essential Legal & Repository Basics (Blocking)
- [x] **Add `LICENSE` File**:
  - Standard GNU AGPL v3 text added to repository root (`LICENSE`).
- [x] **Verify Module Path & Git Remote**:
  - `go.mod` module path `ubunatic.com/voxi` configured.
  - `.gitignore` updated to cover test artifacts, coverage files, audio recordings, and editor files.

### Phase 2: User Documentation & Onboarding (High Priority)
- [x] **Prerequisites & Dependencies Section in README**:
  - Documented required system tools explicitly: `dotool` / `dotoold`, `wl-clipboard` (`wl-copy`), `evdev` access / permissions, `whisper.cpp` / Vulkan drivers, audio capture tools.
- [x] **GNOME Extension Installation Instructions**:
  - Clarified how to install/enable `contrib/gnome-shell-extension` (`voxi@ubunatic.com`) via `gnome-extensions` tool or symlink.
- [x] **Quickstart / First Run Walkthrough**:
  - Provided clean 3-step quickstart command snippet (build/install -> start service -> record/test dictation).
- [x] **CLI Reference & Feature Highlights**:
  - Documented `mode`, `record`, `eager`, `monitor`, `history`, and `bench` command usage.

### Phase 3: Community & Contribution Standards (Recommended)
- [x] **Add `CONTRIBUTING.md`**:
  - Documented Go conventions, conventional commit standards, spec-driven architecture, and make commands.
- [x] **Issue & PR Templates**:
  - Setup `.github/ISSUE_TEMPLATE/` (bug report and feature request templates).

### Phase 4: Automation & CI (Recommended)
- [x] **Continuous Integration (CI)**:
  - Added GitHub Actions workflow (`.github/workflows/ci.yml`) to run `make check` (`go vet`, unit tests, spec verification) and `make build-all` on push and pull requests.

---

## 3. Implementation Summary

1. **Step 1**: Added `LICENSE` (AGPL-3.0) to repository root.
2. **Step 2**: Updated `README.md` with system dependencies, quickstart guide, GNOME extension setup instructions, and architecture highlights.
3. **Step 3**: Added `CONTRIBUTING.md` and `.github/workflows/ci.yml`.
4. **Step 4**: Added `.github/ISSUE_TEMPLATE/` templates.
5. **Step 5**: Updated `issues/README.md` index table.
6. **Step 6**: Ran `make test`, `make check`, and `make install` to confirm build integrity.
