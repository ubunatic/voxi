# 188 — Add Google Cloud Speech-to-Text streaming engine

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**:

---

## 1. Problem & Motivation
Add Google Cloud Speech-to-Text as another selectable speech-recognition engine in voxi, using its streaming recognition API.

## 2. Technical Specification / Findings
API reference: https://docs.cloud.google.com/speech-to-text/docs/streaming-recognize. Resolve current API/version, authentication, audio-format, streaming, and billing requirements during implementation; do not commit credentials.

## 3. Implementation & Verification Plan
Integrate the streaming engine with voxi's existing ASR selection and result flow, make configuration and credential requirements clear, and verify request/stream handling with tests and a live or safely mocked API check. Document setup and any service limitations.

/goal Add a usable Google Cloud Speech-to-Text streaming engine to voxi and verify its integration, or stop and report if blocked on user input, credentials, or denied permission.
