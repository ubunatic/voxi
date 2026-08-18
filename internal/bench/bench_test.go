package bench

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
)

func TestWavDurationSecs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.wav")
	pcm := make([]byte, 16000*2) // 1s of 16kHz 16-bit mono silence
	if err := audio.WriteWAVAudio(path, pcm, 16000); err != nil {
		t.Fatalf("WriteWAVAudio() error = %v", err)
	}
	got, err := wavDurationSecs(path)
	if err != nil {
		t.Fatalf("wavDurationSecs() error = %v", err)
	}
	if got != 1.0 {
		t.Errorf("wavDurationSecs() = %v, want 1.0", got)
	}
}

func TestWavDurationSecsRejectsNonWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-wav.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := wavDurationSecs(path); err == nil {
		t.Fatal("wavDurationSecs() error = nil, want error for non-WAV input")
	}
}

func testDeps() deps.Dependencies {
	return deps.Dependencies{Stdout: &bytes.Buffer{}}
}

func TestFetchAndCacheClipDownloadsAndVerifies(t *testing.T) {
	content := []byte("fake wav bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "cache", "clip.wav")
	got, err := fetchAndCacheClip(context.Background(), testDeps(), srv.URL, sha256Hex(content), path)
	if err != nil {
		t.Fatalf("fetchAndCacheClip() error = %v", err)
	}
	if got != path {
		t.Errorf("fetchAndCacheClip() = %q, want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("cached content = %q, want %q", data, content)
	}
}

func TestFetchAndCacheClipReusesValidCache(t *testing.T) {
	content := []byte("already cached")
	path := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	got, err := fetchAndCacheClip(context.Background(), testDeps(), srv.URL, sha256Hex(content), path)
	if err != nil {
		t.Fatalf("fetchAndCacheClip() error = %v", err)
	}
	if got != path {
		t.Errorf("fetchAndCacheClip() = %q, want %q", got, path)
	}
	if called {
		t.Error("fetchAndCacheClip() downloaded despite a valid cache entry")
	}
}

func TestFetchAndCacheClipRejectsChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected content"))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "clip.wav")
	if _, err := fetchAndCacheClip(context.Background(), testDeps(), srv.URL, sha256Hex([]byte("wanted content")), path); err == nil {
		t.Fatal("fetchAndCacheClip() error = nil, want checksum mismatch error")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("fetchAndCacheClip() left a file behind after a checksum mismatch")
	}
}

func TestDetectBackend(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"gpu", "whisper_backend_init_gpu: using Vulkan0 backend", "gpu:Vulkan0"},
		{"cpu", "whisper_backend_init_gpu: no GPU found", "cpu"},
		{"unrelated output", "Transcription completed in 0.5s", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectBackend(tc.output); got != tc.want {
				t.Errorf("detectBackend(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}
