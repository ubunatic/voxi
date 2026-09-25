# 147 — Research OSS TTS Engines and Open-Weight Models for Modest AMD Hardware

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: [141](141-discovery-add-a-tts-engine-so-other-tools-can-have-voxi-read-text-aloud.md)

---

## 1. Problem & Motivation

Voxi needs a path to more natural and expressive reading voices, including optional playful character-style delivery such as a Yoda-like voice. The available open-source engines and open-weight speech models change quickly, and their hardware demands may exceed what modest local AMD systems can handle.

## 2. Technical Specification / Findings

Research current open-source TTS engines and open-weight models that can run locally on AMD Cezanne-class hardware or newer, with Phoenix-class hardware as the minimum fallback target. Compare voice quality and expressiveness, support for multiple or stylized voices, AMD/Linux runtime support, memory and compute needs, short-snippet latency, licensing, and practical installation requirements. Record uncertainties and distinguish verified results from vendor claims.

## 3. Implementation & Verification Plan

**/goal**: Produce a concise, evidence-backed comparison and recommendation for a locally runnable TTS option that provides natural voices on the target hardware, and determine whether playful/Yoda-like delivery is feasible through supported voices or a separate technique. Define the target hardware and a representative short-text latency/quality evaluation, then verify the leading candidate on available hardware or document a reproducible canary plan if that hardware is unavailable.
