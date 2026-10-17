# 195 — Modularize internal/eager: split eager.go by concern

**Status**: Closed — Resolved by a5b1138
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Architecture
**Related**: 037

---

## 1. Problem & Motivation
`internal/eager/eager.go` is over 2,030 lines long, combining multiple orthogonal concerns:
- Eager pipeline state machine, lifecycle, and sentence streaming
- Audio capture and audio I/O (arecord, raw PCM streaming, ring buffering)
- Service control, IPC, and daemon integration (sockets, toggle, lockfile, systemd notify)

This large monolithic file makes unit testing isolated pieces difficult and complicates maintenance.

## 2. Technical Specification / Findings
- Split `internal/eager/eager.go` into cohesive files within package `eager`:
  - `eager.go`: core types, options, public entry points (`Run`, `Session`), state machine coordination.
  - `audio.go`: audio capture, PCM stream readers, format conversion, input source coordination.
  - `service.go`: socket listener/IPC, daemon control, signal handling, toggle management.
- Introduce interfaces at the seams so audio recording and service triggers can be unit-tested without real audio hardware or system sockets.

## 3. Implementation & Verification Plan
- Analyze symbols, types, and functions in `internal/eager/eager.go`.
- Split into `eager.go`, `audio.go`, `service.go` (and `session.go` if appropriate) with clear interfaces.
- Ensure all tests in `internal/eager` and `./...` remain green without regressions.
