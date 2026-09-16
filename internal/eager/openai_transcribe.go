package eager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// openaiTranscribeEngine is the spec/models.yaml `engine:` value for a
// generic OpenAI-API-compatible ASR backend (multipart POST to
// /v1/audio/transcriptions against a configurable base URL), e.g. a local
// lmcoder instance (issue 126, blocked on lmcoder:issues/091) or the
// voxi-clients/agy-voice/whisper-server research server this was built and
// canaried against in the meantime, since both speak the identical
// contract. Unlike whisper/cohere-transcribe, this engine talks HTTP
// instead of executing a local binary, so it is not dispatched through
// requireEngineBinary's binary-on-PATH check.
const openaiTranscribeEngine = "openai-transcribe"

// defaultOpenAIASRBaseURL matches whisper-server's default --addr
// (voxi-clients/agy-voice/whisper-server), consistent with how
// cleanWithLLM's OPENAI_BASE_URL default (127.0.0.1:8734) targets the
// local LLM cleanup server (issue 106).
const defaultOpenAIASRBaseURL = "http://127.0.0.1:8090/v1"

// transcribeOpenAIWAV posts wavPath to baseURL's OpenAI-compatible
// /v1/audio/transcriptions endpoint with response_format=text and returns
// the raw transcript body. response_format=text (rather than json) means
// the response body is already the plain transcript, matching the raw
// stdout shape the CLI engines (voxtype, crispasr) produce -- so the
// caller's downstream cleanup/safety pipeline needs no engine-specific
// parsing branch.
func transcribeOpenAIWAV(ctx context.Context, wavPath, baseURL, model string) (string, error) {
	if baseURL == "" {
		baseURL = defaultOpenAIASRBaseURL
	}

	body, contentType, err := buildOpenAITranscribeRequestBody(wavPath, model)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: build request: %w", err)
	}

	reqURL := strings.TrimRight(baseURL, "/") + "/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: transcribeTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: POST %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai-transcribe: %s returned %s: %s", reqURL, resp.Status, bytes.TrimSpace(out))
	}
	return string(out), nil
}

// buildOpenAITranscribeRequestBody mirrors voxi-clients' example
// transcribe-client (examples/transcribe-client/main.go): a "file"
// multipart field plus "model" (for client-contract compatibility; ignored
// by whisper-server, may select a model on other OpenAI-compatible
// backends) and response_format=text.
func buildOpenAITranscribeRequestBody(wavPath, model string) (io.Reader, string, error) {
	f, err := os.Open(wavPath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, "", err
	}
	if model == "" {
		model = "whisper-1"
	}
	if err := w.WriteField("model", model); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("response_format", "text"); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
