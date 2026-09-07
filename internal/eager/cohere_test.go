package eager

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/telemetry"
)

func TestCrispASRTranscribeArgsShape(t *testing.T) {
	got := crispASRTranscribeArgs("/cache/cohere-transcribe-q5_0.gguf", "/tmp/one.wav")
	want := []string{"-m", "/cache/cohere-transcribe-q5_0.gguf", "--backend", "cohere", "-t", "6", "--language", "en", "-np", "-nt", "-f", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("crispASRTranscribeArgs() = %#v, want %#v", got, want)
	}
	// Issue 066 §7.5: --prompt/--hotwords are no-ops for this backend and no
	// biasing hook exists yet (issue 074 §5) -- the arg builder must never
	// grow a prompt-shaped flag the way voxtypeTranscribeArgs does.
	for _, arg := range got {
		if arg == "--prompt" || arg == "--hotwords" || arg == "--initial-prompt" {
			t.Fatalf("crispASRTranscribeArgs() unexpectedly included a vocabulary-biasing flag: %#v", got)
		}
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

	if err := ensureWeightsFile(context.Background(), srv.URL, path, int64(len("already-cached-content")), io.Discard); err != nil {
		t.Fatalf("ensureWeightsFile() error = %v, want nil (cached file should satisfy the request)", err)
	}
	if served.Load() != 0 {
		t.Fatal("ensureWeightsFile() hit the network despite an already-cached file of sufficient size")
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

	if err := ensureWeightsFile(context.Background(), srv.URL, path, int64(len(content)), io.Discard); err != nil {
		t.Fatalf("ensureWeightsFile() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("downloaded content = %q, want %q", got, content)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("downloadFile left a .tmp file behind after a successful download")
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

	err := ensureWeightsFile(context.Background(), srv.URL, path, int64(len(content))+1000, io.Discard)
	if err == nil {
		t.Fatal("ensureWeightsFile() error = nil, want truncated-download error")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("ensureWeightsFile() installed a truncated file at the final path")
	}
	if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatal("ensureWeightsFile() left a .tmp file behind after a rejected truncated download")
	}
}

func TestEnsureWeightsFileSurfacesHTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "weights.gguf")

	if err := ensureWeightsFile(context.Background(), srv.URL, path, 1, io.Discard); err == nil {
		t.Fatal("ensureWeightsFile() error = nil, want an error surfacing the 404 status")
	}
}

// TestEagerDispatchesCohereTranscribeEngineToCrispASR drives the same
// production path TestRunEagerCaptureSessionEmitsCorrelatedTelemetryStages
// exercises for the whisper engine, but with a model resolving to
// engine: cohere-transcribe, to confirm runEagerCaptureSession's dispatch
// branch actually resolves the "crispasr" binary via LookPath (not
// "voxtype") and builds cohere-shaped args -- not just that
// crispASRTranscribeArgs alone produces the right shape in isolation. The
// weights cache is pre-seeded so no network call is attempted.
func TestEagerDispatchesCohereTranscribeEngineToCrispASR(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "audio.raw")
	var pcm []byte
	appendFrame := func(amplitude int16) {
		frame := make([]byte, 640)
		for i := 0; i < len(frame); i += 2 {
			binary.LittleEndian.PutUint16(frame[i:i+2], uint16(amplitude))
		}
		pcm = append(pcm, frame...)
	}
	for range 2 {
		appendFrame(0)
	}
	for range 10 {
		appendFrame(1000)
	}
	for range 3 {
		appendFrame(0)
	}
	if err := os.WriteFile(rawPath, pcm, 0600); err != nil {
		t.Fatal(err)
	}

	// A fake voxtype binary is still resolved by the caller (mirroring
	// production) but must never be invoked for this model: the fake
	// script exits nonzero, so the test fails loudly if the whisper path
	// is ever reached by mistake.
	voxtypePath := filepath.Join(tmp, "fake-voxtype")
	if err := os.WriteFile(voxtypePath, []byte("#!/bin/sh\necho 'must not be called' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	crispasrPath := filepath.Join(tmp, "fake-crispasr")
	if err := os.WriteFile(crispasrPath, []byte("#!/bin/sh\nprintf 'voxey uses voxtype cohere path\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	// Pre-seed the weights cache location this test's HOME/XDG_CACHE_HOME
	// resolve to, so ensureCohereWeights finds it cached and never touches
	// the network.
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	weightsPath, err := cohereWeightsPath()
	if err != nil {
		t.Fatalf("cohereWeightsPath(): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(weightsPath), 0755); err != nil {
		t.Fatal(err)
	}
	// A sparse file of the right *size* is enough to satisfy
	// ensureCohereWeights's cache check (it only stats the size, it never
	// reads GGUF content) -- avoids actually allocating cohereGGUFMinBytes
	// (1 GiB) of real content in this test.
	f, err := os.Create(weightsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(cohereGGUFMinBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	telemetryPath := filepath.Join(tmp, "telemetry.jsonl")
	recorder := telemetry.NewRecorder(telemetryPath)
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" || key == "XDG_RUNTIME_DIR" {
				return tmp
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == crispasrBinary {
				return crispasrPath, nil
			}
			return name, nil
		},
		RunStdin: func(context.Context, string, string, ...string) error { return nil },
		Stdout:   io.Discard,
	}
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "cohere-transcribe-03-2026", SpeechContext: false}
	if err := runEagerCaptureSession(context.Background(), d, opts, tmp, voxtypePath, "cat", []string{rawPath}, true, "session-correlation", recorder, nil); err != nil {
		t.Fatalf("runEagerCaptureSession: %v", err)
	}

	events, err := telemetry.ReadAll(telemetryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Event == telemetry.TranscriptionComplete {
			if event.Success == nil || !*event.Success {
				t.Fatalf("cohere-transcribe transcription did not succeed via the fake crispasr binary: %+v", event)
			}
			if event.TranscriptWordCount == nil || *event.TranscriptWordCount == 0 {
				t.Fatalf("cohere-transcribe transcription produced no words: %+v", event)
			}
			return
		}
	}
	t.Fatal("no TranscriptionComplete event observed -- the cohere-transcribe dispatch path did not run")
}
