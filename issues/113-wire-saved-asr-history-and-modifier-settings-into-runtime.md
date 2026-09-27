# 113 — Wire saved ASR, history, and modifier settings into runtime

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [107 interactive settings](107-interactive-settings-tui-for-feature-toggles-and-configuration.md), commits `6cc7f9e`, `edcc409`

---

## 1. Problem & Motivation

The settings UI saves `VOXI_ASR_MODEL`, `VOXI_HISTORY`, and `VOXI_MODIFIER_GATING`, but the running dictation and typing paths do not consume these values. In particular, turning dictation history off still leaves accepted speech in the history file, while `voxi settings --test` reports history as disabled. This is a privacy-relevant mismatch between the control and actual behavior.

## 2. Technical Findings

- `internal/config/config.go` writes all three values to the daemon environment file and loads them for the settings UI and diagnostics. A full-tree Go search found no runtime read of those env keys or their corresponding settings fields.
- `internal/eager/eager.go` chooses the ASR model from `opts.Model` or the model spec default, and appends accepted text to history whenever `historyPath` is nonempty.
- `internal/typing/typing.go` always waits for physical modifiers to be released. The eager modifier buffer also gates independently of the saved toggle.
- `internal/settings/check.go` labels history `Disabled` based solely on the saved setting, without checking actual runtime behavior.

The findings are from source inspection; the disabled-history behavior has not yet been exercised with a live daemon. Restarting alone cannot supply the missing runtime wiring.

## 3. Implementation & Verification Plan

- Define which settings apply to daemon sessions versus one-shot commands, and ensure explicit command-line model choices retain their intended precedence.
- Make the history setting stop history persistence, including any other dictation path that writes history. Clarify whether it also controls saved chunk transcripts/audio; if those remain, label the control precisely.
- Apply ASR model and modifier gating values in their respective runtime paths, or remove controls that are intentionally unsupported.
- Make diagnostics report effective behavior rather than merely saved values.
- Add behavioral tests for each toggle and verify a fresh daemon session uses saved settings. In particular, test that disabled history leaves no new history entry.

## 4. Implementation Progress

- **Milestone 1:** In progress — saved history setting controls accepted eager dictation entries; explicit `--history` wins. Chunk transcript/audio storage remains independent and is called out in diagnostics and settings UI. Behavioral no-entry coverage added.
- **Milestone 1:** Complete — eager history writes honor saved and explicit CLI settings; chunk storage remains separate.
- **Milestone 2:** In progress — saved modifier gating controls typing waits and eager buffering/polling; layout synchronization remains in the typing path.
- **Milestone 2:** Complete — saved modifier setting controls eager polling/buffering and typing waits, while layout synchronization remains active.
- **Milestone 3:** In progress — diagnostics describe effective history/modifier behavior and separate chunk storage; saved settings reach fresh daemon child sessions, while explicit CLI model/history/modifier flags take precedence.
- **Milestone 3:** Complete — diagnostics explain applied gating/history behavior and chunk storage; fresh agent launches pass saved settings; explicit CLI flag precedence is covered.
- **Verification:** First `make test-q1` found three drain tests that relied on modifier buffering without setting the new option and one diagnostic assertion that expected hidden detail in terminal output. Updated those test fixtures/assertions; rerunning the quota suite once.
- **Verification:** `make test-q1` passed after the fixture/assertion corrections; captured at `/tmp/voxi-issue113-test-q1-final.log`, with no `FAIL` matches. `make install` completed. No service restart was run as requested.

**Implementation complete.**

### M4 Pre-Work / Required Refinements (terra review of 137bbe4..9c0045b)

1. `voxi history record` (cmd/voxi/main.go) still calls `history.AppendHistory`
   unconditionally; honor the disabled history setting there too, with a test.
2. Add a test that gating off still runs the issue 129 layout sync under
   `typingMu` (public TypeText path, not only the private helper).

### M4 Refinements Complete

1. `voxi history record` now honors the saved dictation history setting: disabled skips persistence while preserving stdin echo; covered by a no-new-entry test.
2. The public `typing.TypeText` path is covered with modifier gating disabled and verifies issue 129 layout synchronization/restart and injection still occur.

Verification: `make test-q1` passed; output captured at `/tmp/voxi-issue113-m4-test-q1.log` with no `FAIL` matches. `make install` passed. No service restart was run.
