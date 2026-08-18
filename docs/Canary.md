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

| Form | Good for |
|---|---|
| Shell script (`scripts/check-*.sh`) | External CLI tools, environment probes |
| Minimal reel (`reels/minimal/*.reel`) | Wayreel-specific mechanisms (shell capture, key injection) |
| Standalone Go program (`scripts/<name>/main.go`) | Library behaviour, I/O pipelines |
| Single test function tagged `//go:build canary` | Language-level but environment-dependent |
| Throwaway file read back immediately | One-off format or encoding checks |

---

## Example — stdout tee capture (wayreel)

**Mechanism:** inject `exec > >(tee -a /tmp/log) 2>&1` into a zsh session
and read the log from Go.

**Canary:** `reels/minimal/tee-check.reel`

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
— so `V contains=` cannot see `/skills` output rendered by bubbletea.
This finding drove the addition of `V ocr=` as a second assertion mode.

---

## Example — Tesseract OCR quality (wayreel)

**Mechanism:** ImageMagick renders a plain-text TUI simulation to PNG;
Tesseract reads it back.

**Canary:** `scripts/ocr_verify/main.go` + `scripts/testdata/tui/*.txt`

Run: `go run ./scripts/ocr_verify`

**Findings:**
- Skill names (`evergreen`, `domain-modeling`) read reliably from dark
  Nord-theme renders.
- Box-drawing chars (`╭─╯`) break word boundaries; `Installed` → `Ins\ntalled`.
- Whitespace normalisation (`strings.Fields` → join) fixes the split-word
  problem for substring matches.
- Unicode bullets (`•`) are silently dropped — assert on text, not symbols.

These findings shaped the OCR implementation in `script.go:verifyOcrContains`.

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
