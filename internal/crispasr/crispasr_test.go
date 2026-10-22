package crispasr

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
)

func TestTranscribeArgsShape(t *testing.T) {
	got := TranscribeArgs("/cache/cohere-transcribe-q5_0.gguf", "/tmp/one.wav")
	want := []string{"-m", "/cache/cohere-transcribe-q5_0.gguf", "--backend", "cohere", "-t", "6", "--language", "en", "-np", "-nt", "-f", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("TranscribeArgs() = %#v, want %#v", got, want)
	}
	// Issue 066 §7.5: --prompt/--hotwords are no-ops for this backend and no
	// biasing hook exists yet (issue 074 §5) -- the arg builder must never
	// grow a prompt-shaped flag the way voxtypeTranscribeArgs does.
	for _, arg := range got {
		if arg == "--prompt" || arg == "--hotwords" || arg == "--initial-prompt" {
			t.Fatalf("TranscribeArgs() unexpectedly included a vocabulary-biasing flag: %#v", got)
		}
	}
}

func TestTranscribeArgsWithThreads(t *testing.T) {
	got := TranscribeArgsWithThreads("/cache/cohere-transcribe-q5_0.gguf", "/tmp/one.wav", 4)
	want := []string{"-m", "/cache/cohere-transcribe-q5_0.gguf", "--backend", "cohere", "-t", "4", "--language", "en", "-np", "-nt", "-f", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("TranscribeArgsWithThreads(..., 4) = %#v, want %#v", got, want)
	}
}

func TestEnsureWeightsFileSkipsDownloadWhenAlreadyCached(t *testing.T) {
	var served atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		_, _ = w.Write([]byte("should not be fetched"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "weights.gguf")
	if err := os.WriteFile(path, []byte("already-cached-content"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureWeightsFile(context.Background(), srv.URL, path, int64(len("already-cached-content")), io.Discard); err != nil {
		t.Fatalf("EnsureWeightsFile() error = %v, want nil (cached file should satisfy the request)", err)
	}
	if served.Load() != 0 {
		t.Fatal("EnsureWeightsFile() hit the network despite an already-cached file of sufficient size")
	}
}

func TestEnsureWeightsFileDownloadsWhenMissing(t *testing.T) {
	const content = "0123456789fake-gguf-payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "weights.gguf")

	if err := EnsureWeightsFile(context.Background(), srv.URL, path, int64(len(content)), io.Discard); err != nil {
		t.Fatalf("EnsureWeightsFile() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("downloaded content = %q, want %q", got, content)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("DownloadFile left a .tmp file behind after a successful download")
	}
}

func TestEnsureWeightsFileRejectsTruncatedDownload(t *testing.T) {
	const content = "too-short"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "weights.gguf")

	err := EnsureWeightsFile(context.Background(), srv.URL, path, int64(len(content))+1000, io.Discard)
	if err == nil {
		t.Fatal("EnsureWeightsFile() error = nil, want truncated-download error")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("EnsureWeightsFile() installed a truncated file at the final path")
	}
	if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatal("DownloadFile left a .tmp file behind after a truncated download")
	}
}

func TestTranscribeArgsWithBackend(t *testing.T) {
	// CPU backend should include -ng
	gotCPU := TranscribeArgsWithBackend("/cache/model.gguf", "/tmp/one.wav", 6, "cpu")
	wantCPU := []string{"-m", "/cache/model.gguf", "--backend", "cohere", "-t", "6", "-ng", "--language", "en", "-np", "-nt", "-f", "/tmp/one.wav"}
	if !slices.Equal(gotCPU, wantCPU) {
		t.Fatalf("TranscribeArgsWithBackend(cpu) = %#v, want %#v", gotCPU, wantCPU)
	}

	// GPU backend should omit -ng
	gotGPU := TranscribeArgsWithBackend("/cache/model.gguf", "/tmp/one.wav", 6, "gpu")
	wantGPU := []string{"-m", "/cache/model.gguf", "--backend", "cohere", "-t", "6", "--language", "en", "-np", "-nt", "-f", "/tmp/one.wav"}
	if !slices.Equal(gotGPU, wantGPU) {
		t.Fatalf("TranscribeArgsWithBackend(gpu) = %#v, want %#v", gotGPU, wantGPU)
	}
}

func TestParseBuildBackends(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		want       []string
		wantHasGPU bool
	}{
		{
			name:       "cpu fixture",
			fixture:    "testdata/version_cpu.txt",
			want:       []string{"cpu"},
			wantHasGPU: false,
		},
		{
			name:       "vulkan fixture",
			fixture:    "testdata/version_vulkan.txt",
			want:       []string{"cpu", "vulkan"},
			wantHasGPU: true,
		},
		{
			name:       "cuda fixture",
			fixture:    "testdata/version_cuda.txt",
			want:       []string{"cpu", "cuda"},
			wantHasGPU: true,
		},
		{
			name:       "diagnostics cpu fixture",
			fixture:    "testdata/diagnostics_cpu.txt",
			want:       []string{"cpu"},
			wantHasGPU: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(tc.fixture)
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.fixture, err)
			}
			got := ParseBuildBackends(string(content))
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseBuildBackends() = %#v, want %#v", got, tc.want)
			}
			if gotGPU := HasGPUBackend(string(content)); gotGPU != tc.wantHasGPU {
				t.Errorf("HasGPUBackend() = %v, want %v", gotGPU, tc.wantHasGPU)
			}
		})
	}
}

func TestDetectRunBackend(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "cpu banner",
			output: "crispasr 0.8.41 (git 340d7085, Release) [backends: cpu]\n",
			want:   "crispasr:cpu",
		},
		{
			name:   "vulkan banner with cpu",
			output: "crispasr 0.8.41 (git 340d7085, Release) [backends: cpu, vulkan]\n",
			want:   "crispasr:vulkan",
		},
		{
			name:   "cuda banner with cpu",
			output: "crispasr 0.8.41 (git 340d7085, Release) [backends: cpu, cuda]\n",
			want:   "crispasr:cuda",
		},
		{
			name:   "fallback to ggml backends line in diagnostics",
			output: "=== build info ===\n  ggml backends : cpu\n",
			want:   "crispasr:cpu",
		},
		{
			name:   "unknown output",
			output: "random output without banner\n",
			want:   "crispasr:unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectRunBackend(tc.output)
			if got != tc.want {
				t.Errorf("DetectRunBackend() = %q, want %q", got, tc.want)
			}
		})
	}
}
