# 111 — Define LLM cleanup audio context and evaluate transcript fidelity

**Status**: Closed — evaluation complete; multiline fidelity failure documented
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Bug
**Related**: [LLM cleanup request](../internal/eager/eager.go), [request test](../internal/eager/eager_test.go), [real-model evaluation](../docs/studies/2026-09-13-llm-cleanup-evaluation.md), [106 LLM service defaults](106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md)

---

## 1. Problem & Motivation

Voxi now sends each transcript as YAML data with chunk loudness measurements and applied word replacements. The system instruction says spoken commands are transcript content, but the current mock HTTP test only checks the request shape. It does not show whether the configured local model actually preserves dictated commands such as “fix this,” retains replacements already applied, or avoids acting on YAML-looking transcript text. Token savings from YAML have not been measured.

The YAML fields `mean_rms` and `peak_rms` also lack a definition in the model instruction. They are derived from 20 ms signed 16-bit PCM frame RMS values: mean frame RMS and maximum frame RMS, respectively. `peak_rms` is not peak sample amplitude. Without that distinction, the model may interpret the context incorrectly.

## 2. Scope

- Define the loudness fields and their scale in the cleanup instruction, or use clearer field names. Keep their role advisory; they must not cause the model to invent words or discard quiet but valid speech.
- Run a reproducible evaluation against the configured local cleanup model with literal commands and questions, YAML-looking speech, ordinary punctuation/spelling cleanup, and transcripts with already-applied replacements.
- Record expected and actual cleaned text, model identifier, and whether the request timed out or fell back. Compare token use of equivalent YAML and JSON messages with the model's actual tokenizer or server usage metrics if available; report it as unknown otherwise.
- Adjust the prompt or context only if the evaluation demonstrates a concrete failure. Keep instructions separate from transcript data.

## 3. Acceptance Criteria

- [x] The prompt accurately defines the RMS measurements supplied in `chunk`.
- [x] A repeatable real-model evaluation demonstrates that spoken commands remain dictated text and prior replacements survive cleanup, or records failing cases for a targeted fix.
- [x] The evaluation covers YAML-looking multiline transcript text and ordinary cleanup cases.
- [x] Any claim of YAML token savings is supported by measured usage for the configured model, or explicitly left unverified.

## 4. Evaluation outcome

The configured Qwen3-4B cleanup model preserved literal commands and the prior `Voxi` replacement in the final run. Ordinary cleanup succeeded. YAML-looking multiline speech still failed: the request crossed the production 1.5-second deadline and Voxi fell back to the original transcript. Before the RMS prompt edit, the model returned HTTP 200 but flattened all three line breaks. This fidelity problem remains unresolved; the [repeatable evaluation](../docs/studies/2026-09-13-llm-cleanup-evaluation.md) records exact inputs, outputs, timings, and fallback state for a targeted future fix.

Measured server usage for one equivalent message was 206 YAML prompt tokens and 203 compact JSON prompt tokens, so YAML token savings were not observed. The evaluation and code were committed as `ea2c44f` and `742159c`; `go test ./...` and `make restart-service` passed.
