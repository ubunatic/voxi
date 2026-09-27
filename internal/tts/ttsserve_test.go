package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

func newTestTTSServeSpec(url string) spec.TTSServeSpec {
	return spec.TTSServeSpec{
		URL:          url,
		TimeoutMs:    5000,
		ReferenceWav: "",
		Engine:       "chatterbox",
		Settings:     map[string]any{"seed": 42},
	}
}

func TestTTSServeClientRequestShape(t *testing.T) {
	referenceWav := []byte("RIFF-fake-wav-bytes")
	var gotRequest ttsServeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/synthesize" {
			t.Errorf("path = %q, want /synthesize", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		wav := []byte("RIFF-fake-response-wav")
		resp := ttsServeResponse{AudioBase64: base64.StdEncoding.EncodeToString(wav)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := newTestTTSServeSpec(server.URL)
	client := newTTSServeClient(s)
	client.readFile = func(path string) ([]byte, error) { return referenceWav, nil }
	client.referenceWav = "/fake/reference.wav"

	audio, err := client.Synthesize(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if string(audio) != "RIFF-fake-response-wav" {
		t.Errorf("audio = %q, want decoded response WAV", audio)
	}
	if gotRequest.Text != "hello world" {
		t.Errorf("request text = %q, want %q", gotRequest.Text, "hello world")
	}
	if gotRequest.Engine != "chatterbox" {
		t.Errorf("request engine = %q, want chatterbox", gotRequest.Engine)
	}
	wantAudioB64 := base64.StdEncoding.EncodeToString(referenceWav)
	if gotRequest.AudioBase64 != wantAudioB64 {
		t.Errorf("request audio_base64 = %q, want %q", gotRequest.AudioBase64, wantAudioB64)
	}
	if seed, ok := gotRequest.Settings["seed"].(float64); !ok || seed != 42 {
		t.Errorf("request settings[seed] = %v, want 42", gotRequest.Settings["seed"])
	}
}

func TestTTSServeClientSuccessWithoutReferenceWav(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ttsServeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.AudioBase64 != "" {
			t.Errorf("audio_base64 = %q, want empty when no reference WAV configured", req.AudioBase64)
		}
		resp := ttsServeResponse{AudioBase64: base64.StdEncoding.EncodeToString([]byte("wav-bytes"))}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTTSServeClient(newTestTTSServeSpec(server.URL))
	audio, err := client.Synthesize(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if string(audio) != "wav-bytes" {
		t.Errorf("audio = %q, want wav-bytes", audio)
	}
}

func TestTTSServeClientNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("engine crashed"))
	}))
	defer server.Close()

	client := newTTSServeClient(newTestTTSServeSpec(server.URL))
	_, err := client.Synthesize(context.Background(), "hi")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "engine crashed") {
		t.Errorf("error = %v, want it to mention status 500 and body", err)
	}
	if !strings.Contains(err.Error(), server.URL) {
		t.Errorf("error = %v, want it to name the URL", err)
	}
}

func TestTTSServeClientTimeout(t *testing.T) {
	blockUntilTimeout := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockUntilTimeout
	}))
	defer server.Close()
	defer close(blockUntilTimeout)

	s := newTestTTSServeSpec(server.URL)
	s.TimeoutMs = 1000
	client := newTTSServeClient(s)
	client.timeout = 50 * time.Millisecond

	_, err := client.Synthesize(context.Background(), "hi")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %v, want it to report the server as unreachable on timeout", err)
	}
	if !strings.Contains(err.Error(), "tts_serve.url") {
		t.Errorf("error = %v, want it to name the tts_serve.url spec setting", err)
	}
}

func TestTTSServeClientUnreachable(t *testing.T) {
	client := newTTSServeClient(newTestTTSServeSpec("http://127.0.0.1:1"))
	client.timeout = time.Second

	_, err := client.Synthesize(context.Background(), "hi")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %v, want it to report the server as unreachable", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error = %v, want it to name the URL", err)
	}
}

func TestTTSServeClientReferenceWavReadError(t *testing.T) {
	client := newTTSServeClient(newTestTTSServeSpec("http://127.0.0.1:1"))
	client.referenceWav = "/missing/reference.wav"
	client.readFile = func(path string) ([]byte, error) {
		return nil, &pathError{path: path}
	}

	_, err := client.Synthesize(context.Background(), "hi")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want reference WAV read error")
	}
	if !strings.Contains(err.Error(), "tts_serve.reference_wav") {
		t.Errorf("error = %v, want it to name the tts_serve.reference_wav spec setting", err)
	}
	if !strings.Contains(err.Error(), "/missing/reference.wav") {
		t.Errorf("error = %v, want it to name the reference WAV path", err)
	}
}

type pathError struct{ path string }

func (e *pathError) Error() string { return "cannot read " + e.path }
