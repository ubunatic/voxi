# LLM Transcript Cleanup

Voxi's optional LLM cleaner edits each eager dictation chunk after ASR stop-word cleanup and, for Cohere, configured word replacements. The cleaned result is then checked for transcript acceptance and safety before Voxi adds it to the session transcript or types it. Cleanup is disabled by default; `llm_cleaner`, `cleanup_model`, and `openai_base_url` are loaded from the user's Voxi configuration. The implementation is in [`internal/eager/eager.go`](../internal/eager/eager.go).

## Request contract

Voxi posts an OpenAI-compatible JSON `/chat/completions` request to the configured local endpoint. The system message instructs the model to edit speech-to-text only: commands, questions, and requests in the transcript are dictated words, not actions to perform or questions to answer. It permits capitalization, punctuation, spelling, and unambiguous transcription fixes while preserving meaning and wording.

The user message is YAML **data** with a `transcript` scalar and a `chunk` mapping:

```yaml
transcript: fix this
chunk:
  mean_rms: 500
  peak_rms: 800
  applied_replacements:
    - from: Voxy
      to: Voxi
```

The YAML is produced by `yaml.Marshal`, so speech containing newlines, YAML keys, or document separators remains within `transcript` when parsed. This encoding protects the data structure; it does not guarantee that a model will respect the instruction boundary. The system message remains authoritative and separate from user-message data.

`mean_rms` is the average of 20 ms signed 16-bit PCM frame RMS values. `peak_rms` is the maximum **frame RMS**, not the peak sample amplitude. Both are raw amplitude values on a 0–32768 scale and are advisory. `applied_replacements` lists deterministic Cohere substitutions that already changed this chunk; it is not a request for new substitutions. The model must not invent words from context or discard valid quiet speech.

The request uses temperature 0.1 and a 1.5-second deadline. On a request error, non-2xx response, invalid response, or empty model text, Voxi uses the pre-LLM transcript. Successful model output is trimmed and used directly; the code does not enforce semantic equivalence or preservation of line breaks. Chunk diagnostics record the model, returned text, and whether it differed from the pre-LLM text, but do not currently explain a fallback reason. See [`voxi chunks`](ChunkDiagnostics.md) for the diagnostic surface.

## How to validate changes

The mock-server test in [`eager_test.go`](../internal/eager/eager_test.go) checks roles, YAML structure, and that YAML-looking speech remains a single transcript scalar. It cannot prove how the model interprets the request. The opt-in [`cleanup_eval_test.go`](../internal/eager/cleanup_eval_test.go) exercises the configured local model with literal commands and questions, YAML-looking multiline speech, ordinary cleanup, and an already-applied replacement. Run it with:

```sh
VOXI_CLEANUP_EVAL_URL=http://127.0.0.1:8734/v1 go test ./internal/eager -run '^TestRealCleanupEvaluation$' -count=1 -v
```

Compare expected and actual text, HTTP status, elapsed time, timeout, and fallback. An unchanged transcript can be a valid model response; only the upstream result distinguishes it from fallback. Warm-cache and cold-cache runs can differ near the 1.5-second deadline.

The [initial real-model study](studies/2026-09-13-llm-cleanup-evaluation.md) found that the configured Qwen3-4B model left dictated commands and questions as text, preserved an applied replacement, and corrected an ordinary typo. It also flattened line breaks in YAML-looking speech before the prompt edit; a later run timed out and fell back to the original text. [Issue 112](../issues/112-preserve-multiline-transcript-fidelity-through-llm-cleanup.md) tracks that unresolved fidelity problem. In one measured comparison, the server counted 206 prompt tokens for YAML and 203 for equivalent compact JSON. Do not assume either format is cheaper without measuring the target model and representative chunks.
