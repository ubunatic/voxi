package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ubunatic.com/voxi/spec"
)

// ttsServeResponse is the tts-serve POST /synthesize response body.
type ttsServeResponse struct {
	AudioBase64 string `json:"audio_base64"`
}

// ttsServeClient posts text and a reference WAV to a locally running
// tts-serve HTTP server and decodes the synthesized WAV response.
//
// Request shape (issue 155 M2 pre-work): each tts-serve instance wraps one
// engine and exposes that engine's own flat Pydantic request schema with
// extra="forbid" (unknown top-level fields are rejected). There is no
// envelope and no "engine" selector field. Confirmed against the Chatterbox
// server's SynthesisRequest model: text and audio_base64 (required,
// min_length=1 -- a reference clip is mandatory), plus language, seed, and
// engine-specific tuning (exaggeration, cfg_weight, ...) as flat top-level
// fields. See github.com/scorbo2/tts-serve impl/server_chatterbox.py and
// tts-engine-common/src/tts_engine_common/models.py.
type ttsServeClient struct {
	url        string
	timeout    time.Duration
	settings   map[string]any
	httpClient *http.Client
	readFile   func(string) ([]byte, error)
}

func newTTSServeClient(s spec.TTSServeSpec) *ttsServeClient {
	return &ttsServeClient{
		url:      strings.TrimRight(strings.TrimSpace(s.URL), "/"),
		timeout:  s.Timeout(),
		settings: s.Settings,
	}
}

// Synthesize posts text and the reference WAV at referenceWav to the
// tts-serve endpoint and returns the decoded WAV bytes. referenceWav is
// required: the wrapped engine's request schema requires a non-empty
// audio_base64 clip.
func (c *ttsServeClient) Synthesize(ctx context.Context, text, referenceWav string) ([]byte, error) {
	referenceWav = strings.TrimSpace(referenceWav)
	if referenceWav == "" {
		return nil, fmt.Errorf("tts-serve %s: no reference WAV is configured; set tts_serve_reference_wav (or run `voxi voice clone`) before using the tts-serve backend", c.url)
	}
	readFile := c.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	data, err := readFile(referenceWav)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: read tts_serve_reference_wav %q: %w", c.url, referenceWav, err)
	}
	payload := make(map[string]any, len(c.settings)+2)
	for k, v := range c.settings {
		payload[k] = v
	}
	payload["text"] = text
	payload["audio_base64"] = base64.StdEncoding.EncodeToString(data)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: encode request: %w", c.url, err)
	}
	endpoint := c.url + "/synthesize"
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: build request: %w", c.url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s is unreachable (check spec tts_serve.url and timeout_ms): %w", endpoint, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: read response: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts-serve %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed ttsServeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("tts-serve %s: parse response: %w", endpoint, err)
	}
	audio, err := base64.StdEncoding.DecodeString(parsed.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: decode audio_base64 response field: %w", endpoint, err)
	}
	return audio, nil
}
