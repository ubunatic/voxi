# 223 — Categorize replacement rules (jargon vs repair) for sample annotation and benchmarks

**Status**: Closed — Delivered M1 (ae920e3: Replacement.Category, Filter/ApplyReplacementsByCategory, voxi feedback replacement add --category/set-category/list, VoiceInput.md) and M2 (c32c43a: voxi bench --replacements none|jargon|all); full make test passed across all packages (zero failures), live replacement list shows all 10 rules categorized as [jargon]
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
- **CLI (`voxi feedback replacement`)**:
  - Allow specifying and updating rule categories (`--category jargon|repair` on `add`, `set-category HEARD CATEGORY` subcommand) and displaying categories in `list`.
  - Categorize/audit existing rules in `~/.config/voxi/replacements.json` (currently 10 entries: `ThaiR`/`Thaia`/`taya`/`teyr`/`tire`/`tyer` → `taia`, `heiko` → `haiku`, `jewel`/`jewels` → `jules`, `quan` → `qwen`).
- **Benchmarks & Sample Evaluation**:
  - Wire replacement category filtering into `voxi bench` (`internal/bench/bench.go`) so `repair` rules are skipped when scoring raw model quality while `jargon` rules can be retained.

## Decision

- **CLI command & update UX**: The existing command is `voxi feedback replacement` (not `replace`) and has no `update` subcommand. Add `--category jargon|repair` to `add`, add `set-category HEARD CATEGORY` to update existing rules in place, and show `[jargon]` / `[repair]` in `list`.
- **Bench default & flag**: `voxi bench` currently applies no replacements. Add `--replacements none|jargon|all` (default `none` so existing raw-ASR baselines are unchanged; `jargon` applies only `jargon` rules and skips `repair` and uncategorized rules).
- **Live dictation**: `internal/eager/eager.go` continues to apply all rules via `ApplyReplacementsWithAudit` without category filtering.

## Milestones

- **M1 (Schema, CLI & Docs)** — *Delivered in `ae920e3`*: Added `Category` (`jargon`|`repair`) to `feedback.Replacement`, `NormalizeCategory`, `AddReplacementWithCategory`, `SetReplacementCategory`, `FilterReplacementsByCategory`, `ApplyReplacementsByCategory`, `voxi feedback replacement` (`add --category`, `set-category`, `list`), unit tests in `internal/feedback/replacement_test.go`, and docs in `docs/VoiceInput.md`.
- **M2 (Bench Filtering & Verification)**:
  - **Pre-Work / Required Refinements** (from M1 review):
    1. In `internal/feedback/replacement_test.go` (`TestReplacementCategoryCommands`), add coverage for a rule with both `--category` and `--fixed-case` (combined `add` note and `list` column order).
    2. In `TestFilterAndApplyReplacementsByCategory`, use a non-identity jargon rule (e.g. `taya` -> `taia`) so `gotJargon` visibly transforms text in addition to checking `appliedJargon`.
  - Wire `--replacements none|jargon|all` into `internal/bench` and `cmd/voxi` bench command, add unit tests in `internal/bench/`, run `make lint` and `make test-q1`, run `make install` (or `make restart-service`), and categorize the 10 existing `taia`/`haiku`/`jules`/`qwen` rules in `~/.config/voxi/replacements.json` as `jargon`.

## 4. Implementation & Verification Plan

- Unit tests in `internal/feedback/replacement_test.go` covering JSON backward compatibility, category validation, and filtered rule application.
- Tests for benchmark/eval filtering behavior.
- Run `make lint`, `make test-short`, and `make install` (or `make restart-service` if daemon replacement loading changes).
