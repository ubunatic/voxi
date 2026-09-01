# 033: User Stop-Word Feedback for Dictation Hallucinations

**Status**: Complete
**Priority**: P2 (Medium)  
**Severity**: Moderate  
**Category**: Feature  
**Related**: [ASR transcript filtering](../internal/asr/asr.go), [model specification](../spec/models.yaml), [032 Small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md)

---

## 1. Problem & Motivation

Voxi filters a built-in set of Whisper silence/hallucination phrases (for
example video outros). They are currently embedded regex patterns in
`spec/models.yaml`, so a user cannot quickly teach Voxi that a newly observed
hallucination—such as a YouTuber name or `bye`—should never be typed. Nor can
they safely undo a rule that was too broad.

This ticket adds a small, local feedback surface. It applies to the current
`small.en` Whisper path and its existing ASR filter; it is not a model change,
LLM cleanup feature, or cloud service.

## 2. Proposed User Interface

```text
voxi feedback stop-word add "bye"
voxi feedback stop-word add "Some YouTuber"
voxi feedback stop-word list
voxi feedback stop-word remove "bye"
voxi feedback stop-word disable <built-in-rule-id>
voxi feedback stop-word enable <built-in-rule-id>
```

- `add` adds an exact, case-insensitive user phrase. It rejects empty,
  overlong, duplicate, or control-character input.
- `remove` removes only a user-added phrase; it never silently mutates a
  shipped rule.
- `disable`/`enable` let the user turn a listed built-in rule off/on. This is
  the reversible escape hatch when a built-in is too eager.
- `list` labels each rule as built-in, user-added, or disabled, and displays a
  stable rule ID for built-ins.

The command must print what changed and how to reverse it. The initial scope is
global to Voxi; model-specific overrides can follow only if real usage needs
them.

## 3. Storage & Matching Design

- Store user changes in a Voxi-owned, mode-`0600`, atomic local file under
  `~/.config/voxi/` (not the embedded model specification and not Voxtype's
  configuration). No feedback is sent over the network.
- Add stable IDs to built-in stop-word rules in the YAML spec, preserving the
  spec as the source of truth for shipped defaults. A raw regex has no reliable
  identity and cannot be safely disabled by its display text.
- User-added values are literals: quote them with `regexp.QuoteMeta` before
  matching. User feedback must not accept executable regex syntax, preventing
  accidental broad deletion or pathological regular expressions.
- Preserve existing semantics: a matching whole utterance is rejected; a
  matching trailing phrase is stripped. Use conservative word/phrase
  boundaries so `bye` does not remove a substring from an unrelated word.
- Load and merge the active built-in rules plus enabled user rules once per
  eager session, after model selection. Failure to read the feedback file logs
  a clear warning and falls back to built-ins; it must never block dictation.

## 4. Verification Plan

1. Unit-test add/remove/list/enable/disable, atomic persistence, permissions,
   malformed-file recovery, and duplicate/case handling.
2. Unit-test literal quoting and word boundaries with `bye`, `goodbye`, names
   containing punctuation, and a malicious regex-shaped user value.
3. Extend ASR tests to cover whole-utterance rejection and trailing stripping
   for built-in and user rules, including a disabled built-in rule.
4. Manual canary: add an observed hallucination, transcribe a fixture that
   contains it, confirm it is suppressed; remove/disable it, then confirm it
   appears again. Ensure normal phrases and genuine names remain intact.

## 5. Acceptance Criteria

- A user can add, inspect, and remove a local stop word with one Voxi command.
- A user can reversibly disable a built-in rule, without editing YAML or
  modifying the installed binary.
- All user text is treated as literal input, stored locally with restricted
  permissions, and never invokes an LLM or network request.
- Existing `small.en` filtering behavior is unchanged with no feedback file.
- `make check` and `make install` pass before the ticket is closed.

## Implementation

Implemented 2026-09-01. `voxi feedback stop-word` now supports `add`, `list`,
`remove`, `disable`, and `enable`. Overrides are stored atomically at
`~/.config/voxi/stop-words.json` with mode `0600`; malformed local data makes
eager mode warn and continue using shipped rules. Built-in rules have stable
spec-owned IDs, and user phrases are literal, case-insensitive,
phrase-boundary matches. Active feedback is loaded once after eager model
selection, so normal operation with no feedback file retains existing filtering.

Verification: `go test ./...`, `make check`, and `make install` passed.
