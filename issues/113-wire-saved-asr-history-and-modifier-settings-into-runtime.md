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

**Status**: Draft

---

Reserved placeholder ticket.
