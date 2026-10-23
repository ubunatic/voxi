# 201 — Onboard official Gemini Transcribe API as first-class ASR engine

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 126 (OpenAI API backend), 133 (crispasr bench), 199 (tested models catalog), 200 (voxi-model skill)

---

## 1. Problem & Motivation

Voxi currently accesses Gemini speech recognition only through `openai-transcribe-gemini`, an adapter relying on an external loopback bridge (`voxi-clients/agy-voice/whisper-server`). This introduces an auxiliary server dependency, process management overhead, and compatibility constraints (OpenAI multipart form vs native Gemini REST JSON).

Onboarding the official Gemini API directly as a first-class engine (`engine: gemini-transcribe`):
1. **Direct Connection**: Sends 16kHz mono audio chunks directly to Google's official Gemini endpoint (`generateContent` with inline `audio/wav` base64 data) using `GEMINI_API_KEY`.
2. **Zero Local Compute Burden**: Provides a high-accuracy, lightweight cloud fallback when battery is low, thermal limits are reached, or no local GPU render node is available.
3. **Spec-Driven**: Configured cleanly in `spec/models.yaml` with model selections such as `gemini-2.0-flash` or `gemini-1.5-flash`.
4. **Independent Engine**: Decouples cloud transcription from local OpenAI server proxies and external Python/Go bridge daemons.

## 2. /goal

Implement and verify a first-class `gemini-transcribe` engine in Voxi that communicates directly with the official Google Gemini API (using `GEMINI_API_KEY`) to transcribe audio chunks, wire it into `spec/models.yaml` and `schemas/models.schema.json`, and support both eager dictation and benchmark reporting with graceful offline/network error handling, or stop and report when blocked on user input or denied permission.

## 3. Technical Specification

### 3.1 API & Audio Payload
- **Endpoint**: `https://generativelanguage.googleapis.com/v1beta/models/{api_model}:generateContent?key={GEMINI_API_KEY}`
- **Payload**:
  ```json
  {
    "contents": [
      {
        "parts": [
          {
            "inline_data": {
              "mime_type": "audio/wav",
              "data": "<base64-encoded-pcm-wav>"
            }
          },
          {
            "text": "Transcribe this audio verbatim. Output only the exact transcribed speech text, with no introductory or concluding remarks."
          }
        ]
      }
    ]
  }
  ```
- **Response Parsing**: Extract text from `candidates[0].content.parts[0].text`. Strip leading/trailing formatting artifacts and apply model stop-words if necessary.

### 3.2 Spec & Schema Integration
- Update `spec/schemas/models.schema.json` to allow `engine: "gemini-transcribe"`.
- Add official Gemini models to `spec/models.yaml`:
  ```yaml
  gemini-2.0-flash:
    label: "Gemini 2.0 Flash (Official API)"
    engine: gemini-transcribe
    api_model: gemini-2.0-flash
    does_llm_cleanup: false
    stop_words: *common_stop_words
  ```
- Export engine constants in `spec/models.go` (`EngineGeminiTranscribe = "gemini-transcribe"`).

### 3.3 Engine Implementation (`internal/gemini/`)
- Extract dedicated package `internal/gemini/` implementing:
  - `TranscribeWAV(ctx context.Context, client *http.Client, apiKey, apiModel, wavPath string) (string, error)`
  - Direct HTTP POST with timeouts, handling 401/403 (invalid key), 429 (rate limits), and offline connection errors.
- Wire into `internal/eager` capture session.
- Add per-row error handling in `internal/bench` so missing API key or offline status records an error row without aborting other models.

## 4. Verification Plan

1. **Unit Tests (`internal/gemini/`)**:
   - Mock HTTP server tests for valid transcription, rate-limiting (429), bad API key (403), empty audio, and malformed responses.
2. **Spec Validation**:
   - `go test ./spec` to verify schema validation passes with the new engine.
3. **Bench & Eager Integration**:
   - `go test ./internal/bench/... ./internal/eager/...` verifying dispatch and error row handling when `GEMINI_API_KEY` is unset.
4. **Live Verification**:
   - Test live transcription against a recorded WAV fixture with a valid `GEMINI_API_KEY`.
   - Ensure `make test-q1` passes and run `make restart-service`.
