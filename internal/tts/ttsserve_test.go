package tts

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

func newTestTTSServeSpec(url string) spec.TTSServeSpec {
	return spec.TTSServeSpec{
		URL:       url,
		TimeoutMs: 5000,
		Engine:    "chatterbox",
		Settings:  map[string]any{"seed": 42},
	}
}

func TestTTSServeClientRequestShape(t *testing.T) {
	referenceWav := []byte("RIFF-fake-wav-bytes")
	var gotRequest map[string]any
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
		fmtChunk := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
		wav := buildWAV(fmtChunk, []byte("fake-response-pcm"))
		resp := ttsServeResponse{AudioBase64: base64.StdEncoding.EncodeToString(wav)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := newTestTTSServeSpec(server.URL)
	client := newTTSServeClient(s)
	client.readFile = func(path string) ([]byte, error) { return referenceWav, nil }

	audio, err := client.Synthesize(context.Background(), "hello world", "/fake/reference.wav")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	_, gotPCM, ok := parseWAV(audio)
	if !ok {
		t.Fatalf("Synthesize() audio is not a valid WAV: %x", audio)
	}
	if string(gotPCM) != "fake-response-pcm" {
		t.Errorf("audio PCM = %q, want decoded response WAV PCM", gotPCM)
	}
	if gotRequest["text"] != "hello world" {
		t.Errorf("request text = %v, want %q", gotRequest["text"], "hello world")
	}
	if _, hasEngine := gotRequest["engine"]; hasEngine {
		t.Errorf("request must not include an \"engine\" field (each tts-serve instance wraps one engine)")
	}
	if _, hasSettings := gotRequest["settings"]; hasSettings {
		t.Errorf("request must not nest engine-specific parameters under \"settings\" (flat top-level keys)")
	}
	wantAudioB64 := base64.StdEncoding.EncodeToString(referenceWav)
	if gotRequest["audio_base64"] != wantAudioB64 {
		t.Errorf("request audio_base64 = %v, want %q", gotRequest["audio_base64"], wantAudioB64)
	}
	if seed, ok := gotRequest["seed"].(float64); !ok || seed != 42 {
		t.Errorf("request seed = %v, want 42 as a flat top-level field", gotRequest["seed"])
	}
}

func TestTTSServeClientRequiresReferenceWav(t *testing.T) {
	client := newTTSServeClient(newTestTTSServeSpec("http://127.0.0.1:1"))
	_, err := client.Synthesize(context.Background(), "hi", "")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want error when no reference WAV is configured")
	}
	if !strings.Contains(err.Error(), "reference WAV") {
		t.Errorf("error = %v, want it to explain that a reference WAV is required", err)
	}
}

func TestTTSServeClientNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("engine crashed"))
	}))
	defer server.Close()

	client := newTTSServeClient(newTestTTSServeSpec(server.URL))
	client.readFile = func(path string) ([]byte, error) { return []byte("wav"), nil }
	_, err := client.Synthesize(context.Background(), "hi", "/fake/reference.wav")
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
	client.readFile = func(path string) ([]byte, error) { return []byte("wav"), nil }

	_, err := client.Synthesize(context.Background(), "hi", "/fake/reference.wav")
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
	client.readFile = func(path string) ([]byte, error) { return []byte("wav"), nil }

	_, err := client.Synthesize(context.Background(), "hi", "/fake/reference.wav")
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
	client.readFile = func(path string) ([]byte, error) {
		return nil, &pathError{path: path}
	}

	_, err := client.Synthesize(context.Background(), "hi", "/missing/reference.wav")
	if err == nil {
		t.Fatal("Synthesize() error = nil, want reference WAV read error")
	}
	if !strings.Contains(err.Error(), "tts_serve_reference_wav") {
		t.Errorf("error = %v, want it to name the tts_serve_reference_wav setting", err)
	}
	if !strings.Contains(err.Error(), "/missing/reference.wav") {
		t.Errorf("error = %v, want it to name the reference WAV path", err)
	}
}

func TestTTSServeClientSplitsLongTextIntoCappedChunks(t *testing.T) {
	const maxChunkRunes = 20
	referenceWav := []byte("RIFF-fake-wav-bytes")
	var mu sync.Mutex
	var gotTexts []string
	testFmt := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		gotTexts = append(gotTexts, fmt.Sprint(req["text"]))
		mu.Unlock()
		wav := buildWAV(testFmt, []byte("audio-for-"+fmt.Sprint(req["text"])))
		resp := ttsServeResponse{AudioBase64: base64.StdEncoding.EncodeToString(wav)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := newTestTTSServeSpec(server.URL)
	s.MaxChunkRunes = maxChunkRunes
	client := newTTSServeClient(s)
	client.readFile = func(path string) ([]byte, error) { return referenceWav, nil }

	longSentence := "This single sentence is deliberately long enough that it must be split into several capped requests."
	audio, err := client.Synthesize(context.Background(), longSentence, "/fake/reference.wav")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotTexts) < 2 {
		t.Fatalf("tts-serve received %d request(s), want several for a long sentence: %v", len(gotTexts), gotTexts)
	}
	var wantPCM []byte
	for _, text := range gotTexts {
		if n := len([]rune(text)); n > maxChunkRunes {
			t.Errorf("request text %q has %d runes, want at most %d (max_chunk_runes)", text, n, maxChunkRunes)
		}
		wantPCM = append(wantPCM, []byte("audio-for-"+text)...)
	}
	_, gotPCM, ok := parseWAV(audio)
	if !ok {
		t.Fatalf("Synthesize() audio is not a valid WAV: %x", audio)
	}
	if string(gotPCM) != string(wantPCM) {
		t.Errorf("audio PCM = %q, want concatenation of per-chunk responses %q", gotPCM, wantPCM)
	}
}

func TestConcatWAVTwoValidParts(t *testing.T) {
	fmtChunk := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
	part1 := buildWAV(fmtChunk, []byte("abcd"))
	part2 := buildWAV(fmtChunk, []byte("efgh"))

	got, err := concatWAV([][]byte{part1, part2})
	if err != nil {
		t.Fatalf("concatWAV() error = %v", err)
	}

	riffSize := binary.LittleEndian.Uint32(got[4:8])
	if int(riffSize) != len(got)-8 {
		t.Errorf("RIFF size = %d, want %d (len(got)-8)", riffSize, len(got)-8)
	}
	gotFmt, gotPCM, ok := parseWAV(got)
	if !ok {
		t.Fatalf("concatWAV() result is not a valid WAV: %x", got)
	}
	if string(gotFmt) != string(fmtChunk) {
		t.Errorf("fmt chunk = %x, want %x", gotFmt, fmtChunk)
	}
	if string(gotPCM) != "abcdefgh" {
		t.Errorf("PCM data = %q, want %q (part order preserved)", gotPCM, "abcdefgh")
	}
}

func TestConcatWAVOddSizeChunkPadding(t *testing.T) {
	fmtChunk := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
	part1 := buildWAV(fmtChunk, []byte("abc"))
	part2 := buildWAV(fmtChunk, []byte("de"))

	got, err := concatWAV([][]byte{part1, part2})
	if err != nil {
		t.Fatalf("concatWAV() error = %v", err)
	}
	gotFmt, gotPCM, ok := parseWAV(got)
	if !ok {
		t.Fatalf("concatWAV() result is not a valid WAV: %x", got)
	}
	if string(gotFmt) != string(fmtChunk) {
		t.Errorf("fmt chunk = %x, want %x", gotFmt, fmtChunk)
	}
	if string(gotPCM) != "abcde" {
		t.Errorf("PCM data = %q, want %q", gotPCM, "abcde")
	}
	if len(got)%2 != 0 {
		t.Errorf("concatWAV() result length = %d, want even (data chunk padded to word boundary)", len(got))
	}
}

func TestConcatWAVMismatchedFmtErrors(t *testing.T) {
	fmtChunkA := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
	fmtChunkB := []byte{1, 0, 2, 0, 0x44, 0xac, 0, 0, 0x10, 0xb1, 2, 0, 4, 0, 16, 0}
	part1 := buildWAV(fmtChunkA, []byte("abcd"))
	part2 := buildWAV(fmtChunkB, []byte("efgh"))

	_, err := concatWAV([][]byte{part1, part2})
	if err == nil {
		t.Fatal("concatWAV() error = nil, want error for mismatched fmt chunks")
	}
	if !strings.Contains(err.Error(), "fmt chunk differs") {
		t.Errorf("error = %v, want it to mention the fmt chunk mismatch", err)
	}
}

func TestConcatWAVGarbagePartErrors(t *testing.T) {
	fmtChunk := []byte{1, 0, 1, 0, 0x44, 0xac, 0, 0, 0x88, 0x58, 1, 0, 2, 0, 16, 0}
	part1 := buildWAV(fmtChunk, []byte("abcd"))
	garbage := []byte("not a wav at all")

	_, err := concatWAV([][]byte{part1, garbage})
	if err == nil {
		t.Fatal("concatWAV() error = nil, want error for an unparsable part")
	}
	if !strings.Contains(err.Error(), "part 1") {
		t.Errorf("error = %v, want it to identify part 1 as the invalid part", err)
	}

	_, err = concatWAV([][]byte{garbage})
	if err == nil {
		t.Fatal("concatWAV() single-part error = nil, want error for an unparsable single part")
	}
}

type pathError struct{ path string }

func (e *pathError) Error() string { return "cannot read " + e.path }
