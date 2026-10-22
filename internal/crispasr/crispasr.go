package crispasr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"ubunatic.com/voxi/internal/deps"
)

// Binary is the binary resolved on PATH for the cohere-transcribe engine,
// mirroring how "voxtype" is resolved for the whisper engine.
const Binary = "crispasr"

const (
	// GGUFRepo and GGUFFile identify the ungated community GGUF mirror used
	// for the weights (see issue 066 §7.1/§7.2: the official CohereLabs repo
	// is HF-gated and was explicitly out of scope for this ticket to resolve
	// -- see issue 074 §5).
	GGUFRepo = "cstr/cohere-transcribe-03-2026-GGUF"
	GGUFFile = "cohere-transcribe-q5_0.gguf"
	GGUFURL  = "https://huggingface.co/" + GGUFRepo + "/resolve/main/" + GGUFFile

	// GGUFMinBytes is a sanity floor for treating a cached file as
	// complete. The real q5_0 GGUF measured 1,738,723,200 bytes (~1.66 GiB)
	// during issue 066's canary; anything much smaller is a truncated
	// partial download left behind by an interrupted run, not a usable
	// weight file, so it is not trusted and is re-downloaded.
	GGUFMinBytes = 1 << 30 // 1 GiB
)

// TranscribeArgs mirrors voxtypeTranscribeArgs's shape (model path, wav path,
// thread count) for the cohere-transcribe engine. It deliberately does not
// accept an initialPrompt parameter: issue 066 §7.5 confirmed CrispASR's
// --prompt/--hotwords flags are no-ops for the Cohere Transcribe backend
// specifically (no vocabulary-biasing hook exists yet -- see issue 074 §5, an
// explicit known gap, not a bug here).
//
// --language en is required: unlike voxtype's small.en (an English-only model
// architecturally incapable of emitting another language), Cohere Transcribe is
// multilingual and defaults to CrispASR's --language auto (per-chunk language
// auto-detection). Without forcing English, short or ambiguous chunks can be
// misdetected and transcribed into a wrong language wholesale rather than
// merely mis-hearing English words -- observed live after this engine became the
// default (2026-09-07).
func TranscribeArgs(modelPath, wavPath string) []string {
	return TranscribeArgsWithThreads(modelPath, wavPath, 6)
}

// TranscribeArgsWithThreads returns the CLI arguments for crispasr transcription
// with an explicit thread count.
func TranscribeArgsWithThreads(modelPath, wavPath string, threads int) []string {
	threadStr := fmt.Sprintf("%d", threads)
	if threads <= 0 {
		threadStr = "6"
	}
	return []string{"-m", modelPath, "--backend", "cohere", "-t", threadStr, "--language", "en", "-np", "-nt", "-f", wavPath}
}

// EnsureWeights returns the local path to the Cohere Transcribe GGUF weights,
// downloading them from the community mirror into the user cache directory the
// first time the cohere-transcribe engine is selected and no cached copy is
// present. Never bundled into the repo or `make install` (1.66 GiB vs whisper
// small.en's 487 MB -- see issue 074 §3.3). Idempotent: a subsequent call with
// the weights already cached returns immediately without touching the network.
func EnsureWeights(ctx context.Context, d deps.Dependencies) (string, error) {
	path, err := WeightsPath()
	if err != nil {
		return "", err
	}
	if err := EnsureWeightsFile(ctx, GGUFURL, path, GGUFMinBytes, d.Stdout); err != nil {
		return "", fmt.Errorf("cohere-transcribe weights unavailable: %w", err)
	}
	return path, nil
}

// WeightsPath returns the deterministic on-disk location for the cached GGUF
// weights, without touching the network or filesystem beyond resolving the
// user's cache directory.
func WeightsPath() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(cacheRoot, "voxi", "models", GGUFFile), nil
}

// EnsureWeightsFile returns nil once path holds at least minBytes of content,
// downloading it from url first if it does not (idempotent: a pre-existing file
// of sufficient size short-circuits before any network call). minBytes and url
// are parameters (rather than baked-in constants) purely so tests can exercise
// this against a small httptest fixture instead of a real multi-gigabyte
// download.
func EnsureWeightsFile(ctx context.Context, url, path string, minBytes int64, progress io.Writer) error {
	if fi, statErr := os.Stat(path); statErr == nil && fi.Size() >= minBytes {
		return nil
	}

	fmt.Fprintf(progress, "Downloading model weights (one-time, from %s)...\n", url)
	if err := DownloadFile(ctx, url, path, minBytes); err != nil {
		return fmt.Errorf("no cached copy at %s and download failed: %w", path, err)
	}
	return nil
}

// DownloadFile streams url to path via an atomic tmp-file-then-rename, so a
// killed/interrupted download never leaves a truncated file at the final path
// for a later run to mistake as complete. A downloaded body shorter than
// minBytes is treated as a truncated/partial transfer and rejected.
func DownloadFile(ctx context.Context, url, path string, minBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	written, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("download body: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, closeErr)
	}
	if written < minBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("downloaded file is only %d bytes, expected at least %d; refusing to install a truncated weight file", written, minBytes)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}
	return nil
}
