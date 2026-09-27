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

// ttsServeRequest is the tts-serve POST /synthesize body (issue 157 API).
type ttsServeRequest struct {
	Text          string         `json:"text"`
	AudioBase64   string         `json:"audio_base64,omitempty"`
	ReferenceText string         `json:"reference_text,omitempty"`
	Engine        string         `json:"engine,omitempty"`
	Settings      map[string]any `json:"settings,omitempty"`
}

// ttsServeResponse is the tts-serve POST /synthesize response body.
type ttsServeResponse struct {
	AudioBase64 string `json:"audio_base64"`
}

// ttsServeClient posts text and an optional reference WAV to a locally
// running tts-serve HTTP server and decodes the synthesized WAV response.
type ttsServeClient struct {
	url          string
	timeout      time.Duration
	referenceWav string
	engine       string
	settings     map[string]any
	httpClient   *http.Client
	readFile     func(string) ([]byte, error)
}

func newTTSServeClient(s spec.TTSServeSpec) *ttsServeClient {
	return &ttsServeClient{
		url:          strings.TrimRight(strings.TrimSpace(s.URL), "/"),
		timeout:      s.Timeout(),
		referenceWav: s.ReferenceWav,
		engine:       s.Engine,
		settings:     s.Settings,
	}
}

// Synthesize posts text (and the configured reference WAV, if any) to the
// tts-serve endpoint and returns the decoded WAV bytes.
func (c *ttsServeClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	readFile := c.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	var audioB64 string
	if strings.TrimSpace(c.referenceWav) != "" {
		data, err := readFile(c.referenceWav)
		if err != nil {
			return nil, fmt.Errorf("tts-serve %s: read tts_serve.reference_wav %q: %w", c.url, c.referenceWav, err)
		}
		audioB64 = base64.StdEncoding.EncodeToString(data)
	}
	payload, err := json.Marshal(ttsServeRequest{
		Text:        text,
		AudioBase64: audioB64,
		Engine:      c.engine,
		Settings:    c.settings,
	})
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: encode request: %w", c.url, err)
	}
	endpoint := c.url + "/synthesize"
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: read response: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts-serve %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed ttsServeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("tts-serve %s: parse response: %w", endpoint, err)
	}
	audio, err := base64.StdEncoding.DecodeString(parsed.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("tts-serve %s: decode audio_base64 response field: %w", endpoint, err)
	}
	return audio, nil
}
