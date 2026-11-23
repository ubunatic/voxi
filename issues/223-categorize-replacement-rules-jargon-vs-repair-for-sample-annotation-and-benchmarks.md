# 223 — Categorize replacement rules (jargon vs repair) for sample annotation and benchmarks

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 082, 185, 224 (Sample editor TUI)

---

## 1. /goal

Add a `category` (`jargon` vs `repair`) to `feedback.Replacement` and `~/.config/voxi/replacements.json`, provide CLI support to classify rules, and allow benchmarks and sample evaluations to skip `repair` rules while keeping `jargon` rules — or stop and report when blocked on a user decision or denied permission.

## 2. Problem & Motivation

Many rules in `replacements.json` (`internal/feedback/replacement.go`) compensate for weak ASR model quality ("repairs" for misheard common words or artifacts), while others map domain-specific terms ("jargon": project names, product/model names like `taia`, `jules`, `qwen`, `haiku`, `voxi`, `dotool`) that generic ASR models cannot know without vocabulary hints.

When evaluating raw ASR model quality in benchmarks (`voxi bench`) or against annotated samples (#224):
- Applying `repair` rules masks real model errors and skews comparisons.
- Keeping `jargon` rules (and annotating samples with their expected jargon) is fair and isolates genuine model quality from domain-vocabulary gaps.

## 3. Technical Specification

- **Schema (`internal/feedback/replacement.go`)**:
  - Add `Category string` (`json:"category,omitempty"`) to `Replacement`, supporting `"jargon"` and `"repair"` (with clear fallback/validation for legacy uncategorized entries).
  - Add a category-filtered apply helper (e.g. `ApplyReplacementsByCategory` / filter parameter on `ApplyReplacementsWithAudit`) while keeping live dictation applying all enabled rules by default.
- **CLI (`voxi feedback replace`)**:
  - Allow specifying and updating rule categories (`--category jargon|repair`) and displaying categories in `list`.
  - Categorize/audit existing rules in `~/.config/voxi/replacements.json` (currently 10 entries: `ThaiR`/`Thaia`/`taya`/`teyr`/`tire`/`tyer` → `taia`, `heiko` → `haiku`, `jewel`/`jewels` → `jules`, `quan` → `qwen`).
- **Benchmarks & Sample Evaluation**:
  - Wire replacement category filtering into benchmark/sample-eval paths so `repair` rules are skipped when scoring raw model quality while `jargon` rules can be retained.

## 4. Implementation & Verification Plan

- Unit tests in `internal/feedback/replacement_test.go` covering JSON backward compatibility, category validation, and filtered rule application.
- Tests for benchmark/eval filtering behavior.
- Run `make lint`, `make test-short`, and `make install` (or `make restart-service` if daemon replacement loading changes).
