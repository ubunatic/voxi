---
title: Canary-First Development
weight: 20
---

# Canary-First Development

A **canary** is a minimal, standalone test that validates one external
mechanism before any feature code is built on top of it. It is not a
unit test — it has no assertions framework and lives outside the main test
suite. It runs against the real environment (filesystem, shell, external
tool, network) and produces output you can inspect directly.

The name comes from the coal-mine canary: if the cheap probe dies, you
abort before investing in the tunnel.

---

## When to write a canary

Write one whenever a feature depends on a mechanism that:

- involves an external tool (`grim`, `tesseract`, `ffmpeg`, `curl`, …)
- relies on OS-level behaviour (PTY redirection, process substitution,
  socket lifecycle, file descriptor inheritance)
- requires a specific environment (Wayland compositor, Xvfb display,
  nested sway, GPU vs software render)
- reads back its own output (tee capture, screenshot OCR, log scraping)

**Rule of thumb:** if you cannot unit-test it with a fake, write a canary
before writing the feature.

---

## The pattern

```
mechanism alone → observe output → assert manually → then build
```

1. **Isolate the mechanism.** Write the smallest possible script, reel,
   or program that exercises only the external piece. No application logic,
   no abstractions.

2. **Run it and observe.** Check the output file, log, or terminal directly.
   Does the mechanism do what you assumed?

3. **Document what you found.** Note accuracy, failure modes, edge cases.
   This becomes the design input for the real feature.

4. **Keep the canary.** Don't delete it. It is the cheapest future
   regression check and the fastest way for the next person to understand
   the mechanism.

---

## Forms a canary can take

Wayreel is a separate terminal-scenario project used in the historical examples
below. Its `.reel` files are a declarative scenario format for driving and
checking terminal sessions; the paths and assertion names shown here are
project-local rather than Harnez conventions.

| Form | Good for |
|---|---|
| Shell script (`scripts/check-*.sh`) | External CLI tools, environment probes |
| Minimal Wayreel scenario (`reels/minimal/*.reel`) | Terminal shell capture and key injection |
| Standalone Go program (`scripts/<name>/main.go`) | Library behaviour, I/O pipelines |
| Single test function tagged `//go:build canary` | Language-level but environment-dependent |
| Throwaway file read back immediately | One-off format or encoding checks |

---

## Historical example — stdout tee capture in Wayreel

**Mechanism:** inject `exec > >(tee -a /tmp/log) 2>&1` into a zsh session
and read the log from Go.

**Canary:** the project-local file `reels/minimal/tee-check.reel`

```reel
REEL main
  mode = tui
  shell_init = ["exec > >(tee -a /tmp/wayreel-tee-check.log) 2>&1"]

  P 1s
  $ "echo hello from wayreel"
  P 1s
  X
```

Run it, then `cat /tmp/wayreel-tee-check.log`. Expected: `hello from
wayreel` appears in the log.

**Finding:** the tee captures shell-level stdout. TUI apps that switch the
terminal to raw mode write directly to the PTY and bypass the tee entirely
— so Wayreel's `V contains=` text assertion cannot see `/skills` output rendered
by Bubble Tea, the Go terminal-UI framework used by that project. This finding
drove the addition of Wayreel's `V ocr=` image-text assertion mode.

---

## Historical example — Tesseract OCR quality in Wayreel

**Mechanism:** ImageMagick renders a plain-text TUI simulation to PNG;
Tesseract reads it back.

**Canary:** the project-local files `scripts/ocr_verify/main.go` and
`scripts/testdata/tui/*.txt`

Run: `go run ./scripts/ocr_verify`

**Findings:**
- Skill names (`evergreen`, `domain-modeling`) read reliably from dark
  Nord-theme renders.
- Box-drawing chars (`╭─╯`) break word boundaries; `Installed` → `Ins\ntalled`.
- Whitespace normalisation (`strings.Fields` → join) fixes the split-word
  problem for substring matches.
- Unicode bullets (`•`) are silently dropped — assert on text, not symbols.

These findings shaped that project's OCR implementation in the
`script.go:verifyOcrContains` function.

---

## Historical example — a headless peer as a standing canary harness

**Mechanism:** an Android app talking to network appliances (an LG webOS TV, a FRITZ!Box)
has a 3-minute build/install/tap cycle per probe, which makes ad hoc canary scripts too
slow to write for every mechanism worth checking.

**Canary:** `smarthome` (see `docs/studies/2026-09-04-three-days-to-a-public-release.md`)
built a standalone Go CLI (`cmd/flimmerkasten`) as a *host-side peer* speaking the same
wire protocols (SSAP, TR-064, Wake-on-LAN) as the Android app, instead of writing one-off
canary scripts per mechanism. `make flimmerkasten-off` / `make fritzbox-status` became a
standing, reusable canary harness rather than a disposable probe.

**Finding:** every external mechanism in that project was probed live through the CLI
before the matching Android feature was built (Wake-on-LAN, FRITZ!Box least-privilege
users, ARD foreground-app queries — the last two probes concluded "not viable" and
*cancelled* features rather than triggering multi-day debugging). The one-time cost of the
peer CLI is repaid on every subsequent probe, which is why this was the single largest
velocity multiplier across 20 features shipped in three days.

**Rule of thumb, generalized:** when a feature's target is a device or service reachable
over a network protocol and the primary client is slow to iterate on (a mobile app, a
GUI), build a headless CLI speaking the same protocol *first*. Treat it as a standing
canary harness, not a throwaway script — keep it in the tree alongside the feature it
supports.

---

## Canary Scope vs Integration Tests

Canaries are intentionally distinct from full integration test suites:

- **Canaries** are written first, kept minimal, and prove a single environmental assumption or external capability (e.g. "RFC 5322 parsing and rewriting runs in this environment", "DB write and drop is permitted by this MariaDB user", "tar extraction over SSH preserves permissions").
  - Once the probe succeeds, it is kept stable as a baseline reference.
  - Canaries do **not** need to track ongoing feature evolution or refactorings unless the underlying external mechanism or environment assumption changes.
  - A stable canary is not "stale" simply because feature code or abstractions in `internal/` evolved past it.
- **Unit & Integration Tests** live in package test suites and CLI tests (`*_test.go`). They assert full application logic, edge cases, error handling, flag variations, and multi-component workflows.

---

## Anti-patterns

**Building first, then discovering the mechanism doesn't work.** If the
canary would have taken 15 minutes and the feature took three hours, the
canary was mandatory.

**Deleting the canary after the feature ships.** Keep it. It documents
the constraints and lets the next person reproduce the original finding in
minutes.

**Embedding the canary in the feature code.** A canary that only runs as
part of the full test suite has lost its purpose — it can no longer be
run in isolation to debug the environment.

**Asserting too much in the canary.** A canary that checks formatting,
error handling, and retry logic is a feature, not a probe. Keep it focused
on the one mechanism you are validating.

**Confusing canaries with integration test suites.** Expecting a canary script
to continuously import production abstractions or mirror end-to-end feature logic
defeats its purpose as an isolated, stable probe of an environment mechanism.
