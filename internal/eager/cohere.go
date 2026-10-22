package eager

import (
	"context"
	"io"

	"ubunatic.com/voxi/internal/crispasr"
	"ubunatic.com/voxi/internal/deps"
	spec "ubunatic.com/voxi/spec"
)

// cohereTranscribeEngine is the spec/models.yaml `engine:` value for the
// Cohere Transcribe backend, driven through the CrispASR runtime instead of
// voxtype -- see issue 074 and issue 066 (canary).
const cohereTranscribeEngine = spec.EngineCohereTranscribe

// crispasrBinary is the binary resolved on PATH for cohereTranscribeEngine,
// mirroring how "voxtype" is resolved for the whisper engine.
const crispasrBinary = crispasr.Binary

const (
	cohereGGUFRepo     = crispasr.GGUFRepo
	cohereGGUFFile     = crispasr.GGUFFile
	cohereGGUFURL      = crispasr.GGUFURL
	cohereGGUFMinBytes = crispasr.GGUFMinBytes
)

// crispASRTranscribeArgs mirrors voxtypeTranscribeArgs's shape (model path,
// internal/crispasr.
func crispASRTranscribeArgs(modelPath, wavPath string) []string {
	return crispasr.TranscribeArgs(modelPath, wavPath)
}

// ensureCohereWeights returns the local path to the Cohere Transcribe GGUF
// weights, downloading them from the community mirror into the user cache
// directory the first time the cohere-transcribe engine is selected and no
// cached copy is present. Delegated to internal/crispasr.
func ensureCohereWeights(ctx context.Context, d deps.Dependencies) (string, error) {
	return crispasr.EnsureWeights(ctx, d)
}

// cohereWeightsPath returns the deterministic on-disk location for the
// cached GGUF weights. Delegated to internal/crispasr.
func cohereWeightsPath() (string, error) {
	return crispasr.WeightsPath()
}

// ensureWeightsFile is preserved for compatibility with existing tests and callers,
// delegating to internal/crispasr.EnsureWeightsFile.
func ensureWeightsFile(ctx context.Context, url, path string, minBytes int64, progress io.Writer) error {
	return crispasr.EnsureWeightsFile(ctx, url, path, minBytes, progress)
}
