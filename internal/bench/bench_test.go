package bench

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.DurationSecs != 8 {
		t.Errorf("DurationSecs = %d, want 8", opts.DurationSecs)
	}
	if opts.Threads != 6 {
		t.Errorf("Threads = %d, want 6", opts.Threads)
	}
	if len(opts.Backends) == 0 || opts.Backends[len(opts.Backends)-1] != "cpu" {
		t.Errorf("Backends = %v, want at least cpu", opts.Backends)
	}
}

func TestFirstErrorLine(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		fallback error
		want     string
	}{
		{
			name:     "contains error prefix",
			output:   "info: starting\nError: model file corrupted\ninfo: exiting\n",
			fallback: errors.New("fallback"),
			want:     "Error: model file corrupted",
		},
		{
			name:     "no error prefix uses fallback",
			output:   "some random output line\n",
			fallback: errors.New("command failed with exit 1"),
			want:     "command failed with exit 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstErrorLine(tc.output, tc.fallback); got != tc.want {
				t.Errorf("firstErrorLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWavDurationSecsEdgeCases(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		if _, err := wavDurationSecs(filepath.Join(t.TempDir(), "nonexistent.wav")); err == nil {
			t.Fatal("expected error on nonexistent file")
		}
	})

	t.Run("truncated file < 12 bytes", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "short.wav")
		_ = os.WriteFile(p, []byte("RIFF1234"), 0644)
		if _, err := wavDurationSecs(p); err == nil {
			t.Fatal("expected error for file < 12 bytes")
		}
	})

	t.Run("incomplete fmt chunk", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "badfmt.wav")
		// Header RIFF + size + WAVE + "fmt " with size 16 but zero sampleRate/channels
		buf := make([]byte, 12+8+16)
		copy(buf[0:4], "RIFF")
		copy(buf[8:12], "WAVE")
		copy(buf[12:16], "fmt ")
		binary.LittleEndian.PutUint32(buf[16:20], 16)
		_ = os.WriteFile(p, buf, 0644)
		if _, err := wavDurationSecs(p); err == nil {
			t.Fatal("expected error for zero sample rate/channels")
		}
	})

	t.Run("truncated fmt chunk body", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "truncfmt.wav")
		buf := make([]byte, 12+8+8) // chunk size says 16 but only 8 bytes remain
		copy(buf[0:4], "RIFF")
		copy(buf[8:12], "WAVE")
		copy(buf[12:16], "fmt ")
		binary.LittleEndian.PutUint32(buf[16:20], 16)
		_ = os.WriteFile(p, buf, 0644)
		if _, err := wavDurationSecs(p); err == nil {
			t.Fatal("expected error for truncated fmt chunk")
		}
	})

	t.Run("odd chunk size word alignment", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "oddchunk.wav")
		// Create a WAV with an extra 3-byte "junk" chunk (odd size) before fmt and data
		var buf bytes.Buffer
		buf.WriteString("RIFF")
		buf.Write(make([]byte, 4)) // placeholder for RIFF size
		buf.WriteString("WAVE")

		// 3-byte custom chunk + 1 byte pad
		buf.WriteString("junk")
		_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
		buf.WriteString("xyz")
		buf.WriteByte(0) // word alignment padding

		// fmt chunk: 16kHz, 1 channel, 16 bits per sample
		buf.WriteString("fmt ")
		_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))     // PCM format
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))     // channels = 1
		_ = binary.Write(&buf, binary.LittleEndian, uint32(16000)) // sample rate
		_ = binary.Write(&buf, binary.LittleEndian, uint32(32000)) // byte rate
		_ = binary.Write(&buf, binary.LittleEndian, uint16(2))     // block align
		_ = binary.Write(&buf, binary.LittleEndian, uint16(16))    // bits per sample

		// data chunk: 32000 bytes = 1.0s
		buf.WriteString("data")
		_ = binary.Write(&buf, binary.LittleEndian, uint32(32000))
		buf.Write(make([]byte, 32000))

		// Fix RIFF size
		data := buf.Bytes()
		binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
		_ = os.WriteFile(p, data, 0644)

		dur, err := wavDurationSecs(p)
		if err != nil {
			t.Fatalf("wavDurationSecs error: %v", err)
		}
		if dur != 1.0 {
			t.Fatalf("wavDurationSecs = %v, want 1.0", dur)
		}
	})
}

func TestFetchAndCacheClipDownloadErrors(t *testing.T) {
	t.Run("server returns 500 error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		path := filepath.Join(t.TempDir(), "clip.wav")
		if _, err := fetchAndCacheClip(context.Background(), testDeps(), srv.URL, "dummy", path); err == nil {
			t.Fatal("expected error on HTTP 500 response")
		}
	})

	t.Run("download exceeds maximum allowed size", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Write more bytes than referenceClipMaxBytes limit
			chunk := make([]byte, 1024*1024)
			for i := 0; i < 33; i++ {
				_, _ = w.Write(chunk)
			}
		}))
		defer srv.Close()

		path := filepath.Join(t.TempDir(), "toolarge.wav")
		if _, err := fetchAndCacheClip(context.Background(), testDeps(), srv.URL, "dummy", path); err == nil {
			t.Fatal("expected error on download exceeding max size")
		}
	})
}

func TestRecordFixedDurationMissingTools(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}
	err := recordFixedDuration(context.Background(), d, filepath.Join(t.TempDir(), "test.wav"), 1)
	if err == nil || !strings.Contains(err.Error(), "neither pw-record nor arecord found on PATH") {
		t.Fatalf("expected missing tools error, got: %v", err)
	}
}

func TestRecordFixedDurationMockArecord(t *testing.T) {
	binDir := t.TempDir()
	arecordScript := filepath.Join(binDir, "arecord")
	// Fake arecord that produces 16000 samples (32000 bytes) of PCM data to stdout
	scriptContent := "#!/bin/sh\nhead -c 32000 /dev/zero\n"
	if err := os.WriteFile(arecordScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		LookPath: func(file string) (string, error) {
			if file == "arecord" {
				return arecordScript, nil
			}
			return "", errors.New("not found")
		},
	}

	wavPath := filepath.Join(t.TempDir(), "recorded.wav")
	if err := recordFixedDuration(context.Background(), d, wavPath, 1); err != nil {
		t.Fatalf("recordFixedDuration error: %v", err)
	}

	dur, err := wavDurationSecs(wavPath)
	if err != nil {
		t.Fatalf("wavDurationSecs error: %v", err)
	}
	if dur != 1.0 {
		t.Fatalf("recorded duration = %v, want 1.0", dur)
	}
}

func TestRunMissingVoxtypeRecordsErrorRow(t *testing.T) {
	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}
	report, err := Run(context.Background(), d, Options{
		WavFile:  wavPath,
		Models:   []string{"base.en"},
		Backends: []string{"cpu"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want per-row error result", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if !strings.Contains(report.Results[0].Error, "voxtype not found on PATH") {
		t.Errorf("expected voxtype error row, got: %q", report.Results[0].Error)
	}
}

func TestRunInvalidWAV(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(file string) (string, error) {
			return "/bin/true", nil
		},
	}
	invalidWav := filepath.Join(t.TempDir(), "empty.wav")
	_ = os.WriteFile(invalidWav, []byte("not-wav"), 0644)

	_, err := Run(context.Background(), d, Options{WavFile: invalidWav})
	if err == nil || !strings.Contains(err.Error(), "not a RIFF/WAVE file") {
		t.Fatalf("expected invalid WAV error, got: %v", err)
	}
}

func TestRunVoxtypeSuccessAndSkips(t *testing.T) {
	binDir := t.TempDir()
	voxtypeScript := filepath.Join(binDir, "voxtype")
	scriptContent := `#!/bin/sh
echo "whisper_backend_init_gpu: using Vulkan0 backend"
echo "Transcribing test audio..."
echo "Ask not what your country can do for you."
exit 0
`
	if err := os.WriteFile(voxtypeScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "voxtype" {
				return voxtypeScript, nil
			}
			return "", errors.New("not found")
		},
	}

	opts := Options{
		WavFile:  wavPath,
		Models:   []string{"base.en", "cohere-transcribe-03-2026", "large-v3-turbo"},
		Backends: []string{"cpu", "gpu"},
		Threads:  4,
	}

	report, err := Run(context.Background(), d, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if report.AudioSecs != 1.0 {
		t.Errorf("report.AudioSecs = %v, want 1.0", report.AudioSecs)
	}
	if len(report.Results) != 6 {
		t.Fatalf("report.Results count = %d, want 6 (3 models x 2 backends)", len(report.Results))
	}

	for _, res := range report.Results {
		switch res.Model {
		case "cohere-transcribe-03-2026":
			if !strings.Contains(res.Error, "crispasr not found on PATH") {
				t.Errorf("expected crispasr not found error for cohere, got: %q", res.Error)
			}
		case "large-v3-turbo":
			if res.Backend == "cpu" {
				if !strings.Contains(res.Error, "requires_gpu") {
					t.Errorf("expected requires_gpu skip error on cpu, got: %q", res.Error)
				}
			}
		case "base.en":
			if res.Error != "" {
				t.Errorf("expected base.en to succeed, got error: %q", res.Error)
			}
			if !strings.Contains(res.Text, "Ask not what your country can do for you.") {
				t.Errorf("unexpected transcription text: %q", res.Text)
			}
			if res.RTF <= 0 || res.Speedup <= 0 {
				t.Errorf("expected positive RTF and Speedup, got RTF=%v Speedup=%v", res.RTF, res.Speedup)
			}
		}
	}
}

func TestRunVoxtypeFailure(t *testing.T) {
	binDir := t.TempDir()
	voxtypeScript := filepath.Join(binDir, "voxtype")
	scriptContent := `#!/bin/sh
echo "whisper_backend_init_gpu: no GPU found"
echo "Error: failed to load model weights" >&2
exit 1
`
	if err := os.WriteFile(voxtypeScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "voxtype" {
				return voxtypeScript, nil
			}
			return "", errors.New("not found")
		},
	}

	opts := Options{
		WavFile:  wavPath,
		Models:   []string{"base.en"},
		Backends: []string{"cpu"},
	}

	report, err := Run(context.Background(), d, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if !strings.Contains(report.Results[0].Error, "Error: failed to load model weights") {
		t.Errorf("expected model error line, got: %q", report.Results[0].Error)
	}
}

func TestRunDefaultModelsIncludesWhisperAndCohere(t *testing.T) {
	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}
	report, err := Run(context.Background(), d, Options{
		WavFile:  wavPath,
		Backends: []string{"cpu"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	hasCohere := false
	hasWhisper := false
	for _, res := range report.Results {
		if res.Model == "cohere-transcribe-03-2026" {
			hasCohere = true
		}
		if res.Model == "base.en" {
			hasWhisper = true
		}
	}
	if !hasCohere {
		t.Errorf("expected default models run to include cohere-transcribe-03-2026")
	}
	if !hasWhisper {
		t.Errorf("expected default models run to include base.en")
	}
}

func TestRunMissingVoxtypeDoesNotAbortCohere(t *testing.T) {
	binDir := t.TempDir()
	crispasrScript := filepath.Join(binDir, "crispasr")
	scriptContent := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "=== build info ==="
    echo "  ggml backends : cpu"
    exit 0
fi
echo "crispasr 0.8.41 (git 340d7085, Release) [backends: cpu]" >&2
echo "Transcribed by cohere"
exit 0
`
	if err := os.WriteFile(crispasrScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Fake GGUF weights so EnsureWeights doesn't try downloading from HF
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	weightsDir := filepath.Join(cacheRoot, "voxi", "models")
	if err := os.MkdirAll(weightsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(weightsDir, "cohere-transcribe-q5_0.gguf"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "crispasr" {
				return crispasrScript, nil
			}
			return "", errors.New("not found")
		},
	}

	opts := Options{
		WavFile:  wavPath,
		Models:   []string{"base.en", "cohere-transcribe-03-2026"},
		Backends: []string{"cpu"},
	}

	report, err := Run(context.Background(), d, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}

	for _, res := range report.Results {
		if res.Model == "base.en" {
			if !strings.Contains(res.Error, "voxtype not found on PATH") {
				t.Errorf("expected voxtype not found error, got: %q", res.Error)
			}
		} else if res.Model == "cohere-transcribe-03-2026" {
			if res.Error != "" {
				t.Errorf("expected cohere to succeed, got error: %q", res.Error)
			}
			if res.DetectedBackend != "crispasr:cpu" {
				t.Errorf("expected DetectedBackend crispasr:cpu, got: %q", res.DetectedBackend)
			}
		}
	}
}

func TestRunCrispasrGPURowOnCPUBuildErrors(t *testing.T) {
	binDir := t.TempDir()
	crispasrScript := filepath.Join(binDir, "crispasr")
	scriptContent := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "=== build info ==="
    echo "  ggml backends : cpu"
    exit 0
fi
exit 0
`
	if err := os.WriteFile(crispasrScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "crispasr" {
				return crispasrScript, nil
			}
			return "", errors.New("not found")
		},
	}

	opts := Options{
		WavFile:  wavPath,
		Models:   []string{"cohere-transcribe-03-2026"},
		Backends: []string{"gpu"},
	}

	report, err := Run(context.Background(), d, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	res := report.Results[0]
	if !strings.Contains(res.Error, "crispasr build has no GPU backend (ggml backends: cpu)") {
		t.Errorf("expected no GPU backend error, got: %q", res.Error)
	}
}

func TestRunCrispasrGPUSuccessWithDetectedBackend(t *testing.T) {
	binDir := t.TempDir()
	crispasrScript := filepath.Join(binDir, "crispasr")
	scriptContent := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "=== build info ==="
    echo "  ggml backends : cpu, vulkan"
    exit 0
fi
echo "crispasr 0.8.41 (git 340d7085, Release) [backends: cpu, vulkan]" >&2
echo "Transcribed text on vulkan"
exit 0
`
	if err := os.WriteFile(crispasrScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	weightsDir := filepath.Join(cacheRoot, "voxi", "models")
	if err := os.MkdirAll(weightsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(weightsDir, "cohere-transcribe-q5_0.gguf"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	wavPath := filepath.Join(t.TempDir(), "test.wav")
	pcm := make([]byte, 16000*2) // 1s
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "crispasr" {
				return crispasrScript, nil
			}
			return "", errors.New("not found")
		},
	}

	opts := Options{
		WavFile:  wavPath,
		Models:   []string{"cohere-transcribe-03-2026"},
		Backends: []string{"gpu"},
	}

	report, err := Run(context.Background(), d, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	res := report.Results[0]
	if res.Error != "" {
		t.Fatalf("unexpected error: %q", res.Error)
	}
	if res.DetectedBackend != "crispasr:vulkan" {
		t.Errorf("expected DetectedBackend crispasr:vulkan, got: %q", res.DetectedBackend)
	}
	if !strings.Contains(res.Text, "Transcribed text on vulkan") {
		t.Errorf("expected text, got: %q", res.Text)
	}
}
