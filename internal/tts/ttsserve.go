package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
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
	url           string
	timeout       time.Duration
	settings      map[string]any
	maxChunkRunes int
	httpClient    *http.Client
	readFile      func(string) ([]byte, error)
}

func newTTSServeClient(s spec.TTSServeSpec) *ttsServeClient {
	return &ttsServeClient{
		url:           strings.TrimRight(strings.TrimSpace(s.URL), "/"),
		timeout:       s.Timeout(),
		settings:      s.Settings,
		maxChunkRunes: s.MaxChunkRunes,
	}
}

// Synthesize posts text and the reference WAV at referenceWav to the
// tts-serve endpoint and returns the decoded WAV bytes. referenceWav is
// required: the wrapped engine's request schema requires a non-empty
// audio_base64 clip.
//
// text longer than maxChunkRunes is re-split (see splitCapped, issue 155 M3
// pre-work) and sent as several requests, each within the cap so no single
// request risks exceeding timeout_ms; the resulting WAVs are concatenated
// into one clip.
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
	referenceB64 := base64.StdEncoding.EncodeToString(data)

	chunks := splitCapped(text, c.maxChunkRunes)
	parts := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		audio, err := c.synthesizeChunk(ctx, chunk, referenceB64)
		if err != nil {
			return nil, err
		}
		parts = append(parts, audio)
	}
	return concatWAV(parts), nil
}

func (c *ttsServeClient) synthesizeChunk(ctx context.Context, text, referenceB64 string) ([]byte, error) {
	payload := make(map[string]any, len(c.settings)+2)
	for k, v := range c.settings {
		payload[k] = v
	}
	payload["text"] = text
	payload["audio_base64"] = referenceB64
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

// concatWAV joins multiple synthesized WAV clips into one. When any part
// cannot be parsed as a WAV (fmt/data chunks), it falls back to plain byte
// concatenation rather than failing the request.
func concatWAV(parts [][]byte) []byte {
	if len(parts) == 1 {
		return parts[0]
	}
	fmtChunk, pcm, ok := parseWAV(parts[0])
	if !ok {
		return bytes.Join(parts, nil)
	}
	for _, part := range parts[1:] {
		_, morePCM, ok := parseWAV(part)
		if !ok {
			return bytes.Join(parts, nil)
		}
		pcm = append(pcm, morePCM...)
	}
	return buildWAV(fmtChunk, pcm)
}

// parseWAV extracts the fmt and data chunk payloads from a RIFF/WAVE file.
func parseWAV(wav []byte) (fmtChunk, data []byte, ok bool) {
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, nil, false
	}
	for offset := 12; offset+8 <= len(wav); {
		chunkSize := int(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkSize < 0 || chunkEnd > len(wav) {
			return nil, nil, false
		}
		switch string(wav[offset : offset+4]) {
		case "fmt ":
			fmtChunk = append([]byte(nil), wav[chunkStart:chunkEnd]...)
		case "data":
			data = append([]byte(nil), wav[chunkStart:chunkEnd]...)
		}
		offset = chunkEnd + (chunkSize & 1)
	}
	if fmtChunk == nil || data == nil {
		return nil, nil, false
	}
	return fmtChunk, data, true
}

// buildWAV assembles a RIFF/WAVE file from one fmt chunk and combined PCM data.
func buildWAV(fmtChunk, data []byte) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	riffSize := uint32(4 + 8 + len(fmtChunk) + (len(fmtChunk) & 1) + 8 + len(data) + (len(data) & 1))
	_ = binary.Write(buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(fmtChunk)))
	buf.Write(fmtChunk)
	if len(fmtChunk)&1 == 1 {
		buf.WriteByte(0)
	}
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	if len(data)&1 == 1 {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}
