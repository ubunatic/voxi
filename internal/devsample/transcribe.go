package devsample

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/speechcontext"
	"ubunatic.com/voxi/spec"
)

// transcribeFn is transcribeRawTranscript by default; tests override it to
// avoid depending on a real voxtype binary / model being installed.
var transcribeFn = transcribeRawTranscript

// transcribeRawTranscript runs pcm through Voxi's existing small.en
// transcription path with no --initial-prompt / speech-context bias: this
// recorder exists to sample real-world ASR error, so priming the decoder
// with vocabulary hints here would bias exactly what it is meant to
// measure (see issue 045 Section 2). It intentionally does not import
// internal/eager to reuse its transcription call: internal/eager already
// imports internal/feedback, and internal/feedback imports internal/
// devsample (for the `sample` subcommands), so devsample -> eager would be
// an import cycle. The small subset of eager's transcription invocation
// needed here (resolve voxtype, build args, run, clean the transcript) is
// duplicated instead.
//
// A failure here (voxtype missing, model not installed, no LookPath
// resolver wired into d, ...) is non-fatal to Record: the caller falls back
// to today's blank-transcript-prompt behavior.
func transcribeRawTranscript(ctx context.Context, d deps.Dependencies, pcm []byte) (string, error) {
	if d.LookPath == nil {
		return "", fmt.Errorf("no command resolver available to locate voxtype")
	}
	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return "", fmt.Errorf("voxtype not found on PATH: %w", err)
	}
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return "", fmt.Errorf("load model spec: %w", err)
	}
	modelName := modelSpec.DefaultModel

	tmpFile, err := os.CreateTemp("", "voxi-devsample-*.wav")
	if err != nil {
		return "", fmt.Errorf("create temp audio file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)
	if err := audio.WriteWAVAudio(tmpPath, pcm, SampleRate); err != nil {
		return "", fmt.Errorf("write raw transcript audio: %w", err)
	}

	args := []string{"--model", modelName, "--threads", "6", "-q", "transcribe", tmpPath}
	cmd := exec.CommandContext(ctx, voxtypePath, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("transcribe with voxtype: %w", err)
	}
	return asr.CleanWhisperTranscript(outBuf.String(), modelSpec.StopWords(modelName)), nil
}

// candidateVocabulary is the same two speech-context term sources eager's
// opt-in --speech-context prompt draws from (see internal/eager.go and
// spec/models.yaml's speech_context.terms): the shipped static vocabulary
// plus the user's persistent ~/.config/voxi/vocabulary.txt, if any. Loading
// the user file is best-effort; a missing or unreadable file just yields
// fewer suggestions.
func candidateVocabulary(home string, modelSpec *spec.ModelSpec) []string {
	terms := append([]string(nil), modelSpec.SpeechContext.Terms...)
	if vocabPath := speechcontext.VocabularyPath(home); vocabPath != "" {
		if data, err := os.ReadFile(vocabPath); err == nil {
			terms = append(terms, speechcontext.ParseVocabulary(data)...)
		}
	}
	return terms
}

// suggestKeyterms returns the vocabulary terms found (case-insensitively)
// in text, in vocabulary's source order, deduplicated. It never errors: an
// unnormalizable or unmatched term is simply skipped, yielding fewer (or
// zero) suggestions.
func suggestKeyterms(text string, vocabulary []string, maxTermChars int) []string {
	if maxTermChars <= 0 {
		maxTermChars = 64
	}
	lowerText := strings.ToLower(text)
	seen := make(map[string]struct{})
	var found []string
	for _, raw := range vocabulary {
		term, err := speechcontext.NormalizeTerm(raw, maxTermChars)
		if err != nil {
			continue
		}
		key := strings.ToLower(term)
		if _, ok := seen[key]; ok {
			continue
		}
		if strings.Contains(lowerText, key) {
			seen[key] = struct{}{}
			found = append(found, term)
		}
	}
	return found
}

// normalizeKeyterms turns free-form comma- or `|`-separated user input into
// corpus.tsv's canonical `|`-joined keyterms field, trimming whitespace and
// dropping case-insensitive duplicates while preserving first-seen order
// and casing.
func normalizeKeyterms(raw string) string {
	raw = sanitizeText(raw)
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '|' })
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		term := strings.TrimSpace(p)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
	}
	return strings.Join(out, "|")
}
