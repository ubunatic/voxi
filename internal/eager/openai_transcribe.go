package eager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ubunatic.com/voxi/spec"
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

// resolveOpenAIASRBaseURL picks the endpoint an openai-transcribe model
// call should use (issue 134 review gap: OpenAIASRBaseURL was a single
// global setting, so two openai-transcribe backends on different ports --
// agy whisper-server on :8090 and llama-server/R2T2 on :18131 -- could not
// both be live). Resolution order: modelBaseURL (the model's own spec
// base_url, set per-entry in spec/models.yaml) if non-empty, else
// globalBaseURL (the user's OpenAIASRBaseURL setting) if non-empty, else ""
// -- transcribeOpenAIWAV then falls back to defaultOpenAIASRBaseURL itself.
func resolveOpenAIASRBaseURL(modelBaseURL, globalBaseURL string) string {
	if modelBaseURL != "" {
		return modelBaseURL
	}
	return globalBaseURL
}

// ResolveOpenAIASRBaseURL exports resolveOpenAIASRBaseURL for other
// packages (e.g. internal/monitor, issue 136) that must match the same
// endpoint resolution order rather than duplicating it.
func ResolveOpenAIASRBaseURL(modelBaseURL, globalBaseURL string) string {
	return resolveOpenAIASRBaseURL(modelBaseURL, globalBaseURL)
}

// DefaultOpenAIASRBaseURL exports defaultOpenAIASRBaseURL for callers that
// need the same built-in fallback used when neither a model base_url nor
// the global setting is configured.
const DefaultOpenAIASRBaseURL = defaultOpenAIASRBaseURL

// transcribeOpenAIWAV posts wavPath to baseURL's OpenAI-compatible
// /v1/audio/transcriptions endpoint and returns the transcript text.
// responseFormat selects the request/response shape: "" or "text" (the
// issue 126 whisper-server contract) sends response_format=text and
// returns the response body verbatim, already the plain transcript,
// matching the raw stdout shape the CLI engines (voxtype, crispasr)
// produce. "json" (issue 134, R2T2/llama-server) sends
// response_format=json and parses the {"type":...,"text":...} envelope
// instead, since some backends reject response_format=text outright.
// stripBeforeMarker, when non-empty, discards everything up to and
// including the last occurrence of that literal substring in the
// resulting text (see parseOpenAITranscribeResponse); it is a no-op when
// unset or not found.
func transcribeOpenAIWAV(ctx context.Context, wavPath, baseURL, model, responseFormat, stripBeforeMarker string) (string, error) {
	if baseURL == "" {
		baseURL = defaultOpenAIASRBaseURL
	}
	if responseFormat == "" {
		responseFormat = "text"
	}

	body, contentType, err := buildOpenAITranscribeRequestBody(wavPath, model, responseFormat)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: build request: %w", err)
	}

	reqURL := strings.TrimRight(baseURL, "/") + "/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return "", fmt.Errorf("openai-transcribe: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)

	// The caller's context carries the per-model timeout (a backend may hold
	// the request during a cold start); bound it only when it has none.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, transcribeTimeout)
		defer cancel()
		httpReq = httpReq.WithContext(ctx)
	}
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		if strings.Contains(baseURL, ":8735") {
			for _, fallbackURL := range []string{"http://127.0.0.1:18131/v1", "http://127.0.0.1:8749/v1"} {
				hostPort := strings.TrimPrefix(strings.TrimPrefix(fallbackURL, "http://"), "https://")
				hostPort = strings.TrimSuffix(hostPort, "/v1")
				conn, dialErr := net.DialTimeout("tcp", hostPort, 150*time.Millisecond)
				if dialErr == nil {
					conn.Close()
					return transcribeOpenAIWAV(ctx, wavPath, fallbackURL, model, responseFormat, stripBeforeMarker)
				}
			}
		}
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
	return parseOpenAITranscribeResponse(out, responseFormat, stripBeforeMarker)
}

// openaiJSONTranscript is the subset of llama-server's
// {"type":"transcript.text.done","text":"...","usage":{...}} envelope this
// engine needs (issue 134 §6 M1): only the transcript text itself.
type openaiJSONTranscript struct {
	Text string `json:"text"`
}

// parseOpenAITranscribeResponse extracts the transcript text from a
// successful (2xx) response body given the response_format that was
// requested, then applies stripBeforeMarker cleanup. For responseFormat
// "json" it unmarshals body as openaiJSONTranscript and returns an error if
// that fails (a malformed/unexpected body must not be silently treated as a
// transcript); for anything else (including "text") it returns body
// unchanged as the plain transcript, matching the issue 126 whisper-server
// contract exactly.
func parseOpenAITranscribeResponse(body []byte, responseFormat, stripBeforeMarker string) (string, error) {
	text := string(body)
	if responseFormat == "json" {
		var envelope openaiJSONTranscript
		if err := json.Unmarshal(body, &envelope); err != nil {
			return "", fmt.Errorf("openai-transcribe: parse json response: %w", err)
		}
		text = envelope.Text
	}
	if stripBeforeMarker != "" {
		if idx := strings.LastIndex(text, stripBeforeMarker); idx >= 0 {
			text = text[idx+len(stripBeforeMarker):]
		}
	}
	return text, nil
}

// buildOpenAITranscribeRequestBody mirrors voxi-clients' example
// transcribe-client (examples/transcribe-client/main.go): a "file"
// multipart field plus "model" (for client-contract compatibility; ignored
// by whisper-server, may select a model on other OpenAI-compatible
// backends) and response_format (see transcribeOpenAIWAV).
func buildOpenAITranscribeRequestBody(wavPath, model, responseFormat string) (io.Reader, string, error) {
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
	if err := w.WriteField("response_format", responseFormat); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

// transcribeTimeoutFor returns the model's transcribe_timeout_ms, or the
// engine default when the spec leaves it unset.
func transcribeTimeoutFor(m spec.Model) time.Duration {
	if m.TranscribeTimeoutMs > 0 {
		return time.Duration(m.TranscribeTimeoutMs) * time.Millisecond
	}
	return transcribeTimeout
}

// wakeTimeout bounds the fire-and-forget wake call; the server answers it
// at once (202) and loads in the background.
const wakeTimeout = 2 * time.Second

// wakeASRServer tells the ASR server that a recording started, so a stopped
// server can load while the user speaks (lmcoder's /lmcoder/wake, issue
// 181). Errors are ignored: an older server answers 404, and the
// transcription request itself still starts the server when needed.
func wakeASRServer(wakeURL, model string) {
	payload, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), wakeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wakeURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
