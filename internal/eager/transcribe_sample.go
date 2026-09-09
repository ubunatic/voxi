package eager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"ubunatic.com/voxi/internal/deps"
)

// TranscribeCohereWAV runs wavPath through the Cohere Transcribe engine
// (crispasr), exactly the binary/argument resolution
// runEagerCaptureSession's transcription worker uses (requireEngineBinary +
// crispASRTranscribeArgs), downloading/caching the GGUF weights on first
// use if needed. It returns crispasr's raw (uncleaned) transcript text.
//
// This is a standalone entry point for CLI dev-tooling spot checks (`voxi
// feedback sample list --process`, issue 098) -- it does not touch
// RunEagerDictation/runEagerDaemon's continuous capture path at all, and
// deliberately skips that path's asr.CleanWhisperTranscript hallucination/
// stop-word cleanup (which needs live feedback overrides and spec model
// resolution the continuous dictation path already carries): good enough
// for an eyeball spot check of stored-ground-truth vs. a fresh transcript,
// not a claim of dictation-identical output. Callers that want cleaned text
// should run it through asr.StripANSI/CleanWhisperTranscript themselves.
func TranscribeCohereWAV(ctx context.Context, d deps.Dependencies, wavPath string) (string, error) {
	transcribeBinPath, weightsPath, err := requireEngineBinary(ctx, d, "cohere-transcribe (sample spot-check)", cohereTranscribeEngine)
	if err != nil {
		return "", err
	}

	cmdArgs := crispASRTranscribeArgs(weightsPath, wavPath)
	transcribeCtx, cancel := context.WithTimeout(ctx, transcribeTimeout)
	defer cancel()
	cmd := exec.CommandContext(transcribeCtx, transcribeBinPath, cmdArgs...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("crispasr transcribe %s: %w", wavPath, err)
	}
	return out.String(), nil
}
