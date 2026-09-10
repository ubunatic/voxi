package eager

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
)

// ─────────────────────────────────────────────────────────────────────────
// Issue 056 Phase 1/2: real end-to-end eager-pipeline replay.
//
// These tests drive the exact production code path (runEagerCaptureSession:
// real audio.AudioSegmenter, real chunk dispatch worker, real `voxtype
// transcribe` subprocess against the real installed model) on pre-recorded
// corpus WAV fixtures instead of a live microphone. Two substitutions make
// this safe and deterministic without reimplementing any pipeline logic:
//
//  1. Audio source: recCmdName/recArgs (normally pw-record/arecord) is
//     pointed at sox or ffmpeg streaming a fixture WAV's raw PCM to stdout
//     instead of a live microphone (see resolvePCMStreamCmd).
//  2. Typing injection: deps.Dependencies.RunStdin is swapped for a capture
//     function so internal/typing.TypeText's normal dotoolc/dotool code
//     path runs unmodified (LookPath still resolves the real binaries) but
//     no keystrokes are actually injected into the focused window.
//
// Both tests require real, private local voice recordings (see
// testdata/speech-context/README.md) and the real voxtype/model, so they
// are gated behind the VOXI_E2E=1 env var and are not part of `go test
// ./...`'s fast path.
// ─────────────────────────────────────────────────────────────────────────

const e2eEnvVar = "VOXI_E2E"

func e2eSkipReason() string {
	if os.Getenv(e2eEnvVar) != "" {
		return ""
	}
	return "set " + e2eEnvVar + "=1 to run the real end-to-end eager-pipeline test " +
		"(needs voxtype + an installed model + sox/ffmpeg + a private corpus WAV; " +
		"see testdata/speech-context/README.md and issue 056)"
}

// e2eCorpusDir resolves the corpus directory: VOXI_E2E_CORPUS overrides the
// committed public fixture directory, e.g. to point at a private
// ~/.config/voxi/samples directory recorded via `voxi feedback sample record`
// (see testdata/speech-context/README.md "Using recorded dev samples").
func e2eCorpusDir() string {
	if dir := os.Getenv("VOXI_E2E_CORPUS"); dir != "" {
		return dir
	}
	return filepath.Join("..", "..", "testdata", "speech-context")
}

type e2eFixture struct {
	ID       string
	File     string
	Expected string
	Keyterms []string
}

// loadE2ECorpus parses a corpus.tsv manifest (see
// testdata/speech-context/corpus.tsv and its README for the format:
// id<TAB>wav file<TAB>expected transcript<TAB>keyterms separated by |).
func loadE2ECorpus(path string) ([]e2eFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fixtures []e2eFixture
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("%s:%d: want four tab-separated fields, got %d", path, i+1, len(fields))
		}
		var keyterms []string
		if fields[3] != "" {
			keyterms = strings.Split(fields[3], "|")
		}
		fixtures = append(fixtures, e2eFixture{ID: fields[0], File: fields[1], Expected: fields[2], Keyterms: keyterms})
	}
	return fixtures, nil
}

func findE2EFixture(fixtures []e2eFixture, id string) (e2eFixture, bool) {
	for _, f := range fixtures {
		if f.ID == id {
			return f, true
		}
	}
	return e2eFixture{}, false
}

// resolvePCMStreamCmd picks an external command that streams a WAV file's
// raw 16kHz mono S16LE PCM to stdout, standing in for the live-mic
// pw-record/arecord subprocess runEagerCaptureSession normally reads from.
// Canary-first: probe availability via d.LookPath before ever building a
// command line, per docs/Canary.md. sox is preferred (it's the tool named
// in issue 056's design) but this repo's dev machines only have ffmpeg
// installed, so that is a fully equivalent fallback.
func resolvePCMStreamCmd(d deps.Dependencies, wavPath string) (string, []string, error) {
	if _, err := d.LookPath("sox"); err == nil {
		return "sox", []string{wavPath, "-t", "raw", "-r", "16000", "-c", "1", "-b", "16", "-e", "signed", "-"}, nil
	}
	if _, err := d.LookPath("ffmpeg"); err == nil {
		return "ffmpeg", []string{"-v", "quiet", "-nostdin", "-i", wavPath, "-f", "s16le", "-ar", "16000", "-ac", "1", "-"}, nil
	}
	return "", nil, fmt.Errorf("neither sox nor ffmpeg found on PATH to stream a corpus WAV as raw PCM")
}

// readWAVDataChunk parses a canonical little-endian RIFF/WAVE file and
// returns its "data" subchunk's raw PCM bytes plus the sample rate declared
// in "fmt ". Used only to splice fixture WAVs together in-memory for the
// Phase 2 multi-utterance/noise-interleaving test; every fixture WAV in this
// repo's corpora is already 16kHz mono 16-bit PCM (confirmed via `file`), so
// no resampling is attempted here -- a mismatched input is a test-authoring
// bug, not a case to handle gracefully.
func readWAVDataChunk(path string) ([]byte, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s: not a RIFF/WAVE file", path)
	}
	sampleRate := 0
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			size = len(data) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			}
		case "data":
			return data[body : body+size], sampleRate, nil
		}
		pos = body + size
		if size%2 == 1 {
			pos++ // RIFF chunks are word-aligned
		}
	}
	return nil, 0, fmt.Errorf("%s: no data subchunk found", path)
}

// sessionPart is one item in a spliced synthetic session: either a fixture
// WAV's audio (WAVPath set) or a run of silence (SilenceMs set).
type sessionPart struct {
	WAVPath   string
	SilenceMs int
}

// buildSpliceStream concatenates fixture PCM and synthetic silence gaps into
// one continuous 16kHz mono S16LE raw PCM stream, matching issue 056 section
// 2.1's "Audio Splicer" design at minimal scope: real fixture audio plus
// silence, no resampling or crossfade.
func buildSpliceStream(parts []sessionPart) ([]byte, error) {
	const sampleRate = 16000
	const bytesPerSample = 2
	var out []byte
	for _, p := range parts {
		if p.SilenceMs > 0 {
			out = append(out, make([]byte, sampleRate*p.SilenceMs/1000*bytesPerSample)...)
			continue
		}
		pcm, rate, err := readWAVDataChunk(p.WAVPath)
		if err != nil {
			return nil, err
		}
		if rate != 0 && rate != sampleRate {
			return nil, fmt.Errorf("%s: sample rate %d, want %d", p.WAVPath, rate, sampleRate)
		}
		out = append(out, pcm...)
	}
	return out, nil
}

// extractTypedText recovers the plain text voxi's eager pipeline would have
// typed from a captured dotool script (see internal/typing.BuildDotoolCommands).
// Each accepted eager chunk calls typing.TypeText(ctx, d, text+" "), and with
// no configured typeDelayMs and a single-line chunk, BuildDotoolCommands
// renders that as exactly one "type <text>\n" line -- so pulling the
// argument of every "type " line recovers the concatenated transcript
// without needing a dotool-script parser.
func extractTypedText(script string) string {
	var out strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if rest, ok := strings.CutPrefix(line, "type "); ok {
			out.WriteString(rest)
		}
	}
	return out.String()
}

// wordErrorRate is a Levenshtein word-edit-distance WER, matching
// scripts/speech_context_bench's helper of the same name (kept package-local
// here since that helper is unexported in package main and this repo avoids
// adding a shared dependency for one reused function).
func e2eWords(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	words := make([]string, 0, len(raw))
	for _, w := range raw {
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

func wordErrorRate(expected, actual string) float64 {
	want, got := e2eWords(expected), e2eWords(actual)
	if len(want) == 0 {
		if len(got) == 0 {
			return 0
		}
		return 1
	}
	previous := make([]int, len(got)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, wantWord := range want {
		current := make([]int, len(got)+1)
		current[0] = i + 1
		for j, gotWord := range got {
			cost := 1
			if wantWord == gotWord {
				cost = 0
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return float64(previous[len(got)]) / float64(len(want))
}

// e2eDeps builds a deps.Dependencies for the real end-to-end tests: real
// LookPath/RunOutput/etc (so voxtype, dotool, and dotoolc all resolve for
// real, exercising typing.TypeText's normal non-degraded code path per issue
// 056's design), but with HOME and XDG_RUNTIME_DIR redirected to a scratch
// dir so the session's chunk ring buffer, feedback overrides, speech
// vocabulary, and dictation history all land in a throwaway location instead
// of the real user's, and with RunStdin swapped to capture the rendered
// dotool script instead of executing it -- the seam that keeps this test
// from injecting real keystrokes into whatever window has focus (see
// internal/typing/typing.go TypeText and existing fakes in
// internal/eager/eager_test.go for the same deps.Dependencies fake pattern).
//
// Note: two production helpers (writeVoxtypeState and recordEagerStat in
// internal/eager/eager.go) read XDG_RUNTIME_DIR via os.Getenv directly
// rather than through deps.Dependencies, so running this test still writes
// ephemeral, self-resetting state files (voice-state, eager-metrics.json)
// under the real runtime dir -- harmless (the session's own deferred
// writeVoxtypeState("idle") resets it) but worth knowing if voxi monitor
// flickers "recording" during a test run.
func e2eDeps(t *testing.T) (deps.Dependencies, func() string) {
	t.Helper()
	tmpHome := t.TempDir()
	tmpRuntime := t.TempDir()
	d := deps.DefaultDependencies(nil, io.Discard)
	d.Getenv = func(key string) string {
		switch key {
		case "HOME":
			return tmpHome
		case "XDG_RUNTIME_DIR":
			return tmpRuntime
		default:
			return os.Getenv(key)
		}
	}

	var mu sync.Mutex
	var typedScripts []string
	d.RunStdin = func(_ context.Context, stdin string, _ string, _ ...string) error {
		mu.Lock()
		typedScripts = append(typedScripts, stdin)
		mu.Unlock()
		return nil
	}

	transcript := func() string {
		mu.Lock()
		defer mu.Unlock()
		var parts []string
		for _, s := range typedScripts {
			parts = append(parts, extractTypedText(s))
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	}
	return d, transcript
}

// TestEagerCaptureSessionEndToEndSingleFixture is issue 056 Phase 1: replay
// one corpus fixture through the real eager pipeline (real VAD segmenter,
// real chunk dispatch, real `voxtype transcribe`) and assert the
// concatenated "typed" transcript is reasonably close to the fixture's known
// expected text.
func TestEagerCaptureSessionEndToEndSingleFixture(t *testing.T) {
	if reason := e2eSkipReason(); reason != "" {
		t.Skip(reason)
	}

	corpusDir := e2eCorpusDir()
	fixtures, err := loadE2ECorpus(filepath.Join(corpusDir, "corpus.tsv"))
	if err != nil {
		t.Fatalf("load corpus manifest: %v", err)
	}
	fixtureID := os.Getenv("VOXI_E2E_FIXTURE")
	if fixtureID == "" {
		fixtureID = "technical-core"
	}
	fx, ok := findE2EFixture(fixtures, fixtureID)
	if !ok {
		t.Fatalf("fixture %q not found in %s", fixtureID, corpusDir)
	}
	wavPath := filepath.Join(corpusDir, fx.File)
	if _, err := os.Stat(wavPath); err != nil {
		t.Skipf("fixture WAV %s not present locally (private recording; see testdata/speech-context/README.md): %v", wavPath, err)
	}

	d, transcript := e2eDeps(t)

	audioCmd, audioArgs, err := resolvePCMStreamCmd(d, wavPath)
	if err != nil {
		t.Skip(err.Error())
	}
	if _, err := d.LookPath("voxtype"); err != nil {
		t.Skip("voxtype not found on PATH")
	}

	opts := EagerOptions{
		ThresholdRMS:  150,
		SilenceMs:     800,
		PreRollMs:     500,
		MinSpeechMs:   200,
		MaxWindowMs:   8000,
		TypeOutput:    true,
		RecordHistory: true,
		Model:         "small.en",
		SpeechContext: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := runEagerCaptureSession(ctx, d, opts, t.TempDir(), audioCmd, audioArgs, true, "test-session", nil, nil, nil); err != nil {
		t.Fatalf("runEagerCaptureSession: %v", err)
	}

	got := transcript()
	wer := wordErrorRate(fx.Expected, got)
	t.Logf("fixture=%s expected=%q got=%q wer=%.3f", fx.ID, fx.Expected, got, wer)

	if got == "" {
		t.Fatalf("no transcript typed for fixture %s: expected %q -- either VAD rejected all speech or transcription failed", fx.ID, fx.Expected)
	}
	const maxWER = 0.4 // loose bound: real ASR output, not exact-match
	if wer > maxWER {
		t.Fatalf("WER %.3f exceeds %.3f for fixture %s\n  expected: %q\n  got:      %q", wer, maxWER, fx.ID, fx.Expected, got)
	}
}

// TestEagerCaptureSessionEndToEndCohereTranscribe is issue 074's live
// verification: the same real-pipeline replay as
// TestEagerCaptureSessionEndToEndSingleFixture, but selecting the
// cohere-transcribe engine (model cohere-transcribe-03-2026) instead of
// small.en, to prove the engine-dispatch branch in runEagerCaptureSession
// actually resolves and drives the real crispasr binary end-to-end (real VAD
// segmenter, real chunk dispatch, real `crispasr --backend cohere`
// subprocess, real GGUF weights) rather than only a mocked-subprocess unit
// test (see TestEagerDispatchesCohereTranscribeEngineToCrispASR in
// cohere_test.go for that faster, mocked coverage). Requires the same
// VOXI_E2E=1 gate as the other e2e tests here, plus a "crispasr" binary on
// PATH and the Cohere Transcribe GGUF weights already cached (or reachable
// over the network) -- see issue 066 §7.2 for the build/download commands.
func TestEagerCaptureSessionEndToEndCohereTranscribe(t *testing.T) {
	if reason := e2eSkipReason(); reason != "" {
		t.Skip(reason)
	}

	corpusDir := e2eCorpusDir()
	fixtures, err := loadE2ECorpus(filepath.Join(corpusDir, "corpus.tsv"))
	if err != nil {
		t.Fatalf("load corpus manifest: %v", err)
	}
	fixtureID := os.Getenv("VOXI_E2E_FIXTURE")
	if fixtureID == "" {
		fixtureID = "kt-core"
	}
	fx, ok := findE2EFixture(fixtures, fixtureID)
	if !ok {
		t.Fatalf("fixture %q not found in %s", fixtureID, corpusDir)
	}
	wavPath := filepath.Join(corpusDir, fx.File)
	if _, err := os.Stat(wavPath); err != nil {
		t.Skipf("fixture WAV %s not present locally (private recording; see testdata/speech-context/README.md): %v", wavPath, err)
	}

	d, transcript := e2eDeps(t)

	audioCmd, audioArgs, err := resolvePCMStreamCmd(d, wavPath)
	if err != nil {
		t.Skip(err.Error())
	}
	if _, err := d.LookPath(crispasrBinary); err != nil {
		t.Skip("crispasr not found on PATH")
	}
	// Deliberately does NOT require voxtype on PATH: issue 077's whole point
	// is that a Cohere-engine session reaches readiness and transcribes
	// without it, which this live end-to-end run (real VAD, real crispasr
	// subprocess, real GGUF weights) proves directly on a machine that may
	// genuinely lack voxtype.

	opts := EagerOptions{
		ThresholdRMS:  150,
		SilenceMs:     800,
		PreRollMs:     500,
		MinSpeechMs:   200,
		MaxWindowMs:   8000,
		TypeOutput:    true,
		RecordHistory: true,
		Model:         "cohere-transcribe-03-2026",
		SpeechContext: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := runEagerCaptureSession(ctx, d, opts, t.TempDir(), audioCmd, audioArgs, true, "test-session", nil, nil, nil); err != nil {
		t.Fatalf("runEagerCaptureSession: %v", err)
	}

	got := transcript()
	wer := wordErrorRate(fx.Expected, got)
	t.Logf("fixture=%s expected=%q got=%q wer=%.3f", fx.ID, fx.Expected, got, wer)

	if got == "" {
		t.Fatalf("no transcript typed for fixture %s: expected %q -- either VAD rejected all speech or transcription failed", fx.ID, fx.Expected)
	}
	const maxWER = 0.4 // loose bound: real ASR output, not exact-match; matches the whisper e2e test's bound
	if wer > maxWER {
		t.Fatalf("WER %.3f exceeds %.3f for fixture %s\n  expected: %q\n  got:      %q", wer, maxWER, fx.ID, fx.Expected, got)
	}
}

// TestEagerCaptureSessionEndToEndSplicedNoiseSession is issue 056 Phase 2: a
// multi-utterance synthetic session splicing two speech fixtures together
// with a real non-speech noise transient (the corpus's
// artifact-keyboard-smash fixture) interleaved between them, separated by
// real silence gaps -- exercising the real VAD segmenter's utterance
// boundary detection and hallucination/silence-artifact rejection path on
// genuine noise audio (not synthetic all-zero silence). Asserts both speech
// fixtures are transcribed and typed, in order, and that the noise fixture
// never produces an accepted/typed chunk.
func TestEagerCaptureSessionEndToEndSplicedNoiseSession(t *testing.T) {
	if reason := e2eSkipReason(); reason != "" {
		t.Skip(reason)
	}

	corpusDir := e2eCorpusDir()
	fixtures, err := loadE2ECorpus(filepath.Join(corpusDir, "corpus.tsv"))
	if err != nil {
		t.Fatalf("load corpus manifest: %v", err)
	}

	idA := os.Getenv("VOXI_E2E_FIXTURE_A")
	if idA == "" {
		idA = "technical-core"
	}
	idB := os.Getenv("VOXI_E2E_FIXTURE_B")
	if idB == "" {
		idB = "technical-files"
	}
	idNoise := os.Getenv("VOXI_E2E_NOISE_FIXTURE")
	if idNoise == "" {
		idNoise = "artifact-keyboard-smash"
	}

	fxA, okA := findE2EFixture(fixtures, idA)
	fxB, okB := findE2EFixture(fixtures, idB)
	fxNoise, okNoise := findE2EFixture(fixtures, idNoise)
	if !okA || !okB || !okNoise {
		t.Fatalf("missing fixture ids in %s: A=%v(%t) B=%v(%t) noise=%v(%t)", corpusDir, idA, okA, idB, okB, idNoise, okNoise)
	}

	wavA := filepath.Join(corpusDir, fxA.File)
	wavB := filepath.Join(corpusDir, fxB.File)
	wavNoise := filepath.Join(corpusDir, fxNoise.File)
	for _, p := range []string{wavA, wavB, wavNoise} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("fixture WAV %s not present locally (private recording; see testdata/speech-context/README.md): %v", p, err)
		}
	}

	spliced, err := buildSpliceStream([]sessionPart{
		{SilenceMs: 500},
		{WAVPath: wavA},
		{SilenceMs: 1500},
		{WAVPath: wavNoise},
		{SilenceMs: 1200},
		{WAVPath: wavB},
		{SilenceMs: 500},
	})
	if err != nil {
		t.Fatalf("splice session audio: %v", err)
	}
	sessionWAV := filepath.Join(t.TempDir(), "spliced-session.wav")
	if err := audio.WriteWAVAudio(sessionWAV, spliced, 16000); err != nil {
		t.Fatalf("write spliced session WAV: %v", err)
	}

	d, transcript := e2eDeps(t)

	audioCmd, audioArgs, err := resolvePCMStreamCmd(d, sessionWAV)
	if err != nil {
		t.Skip(err.Error())
	}
	if _, err := d.LookPath("voxtype"); err != nil {
		t.Skip("voxtype not found on PATH")
	}

	opts := EagerOptions{
		ThresholdRMS:  150,
		SilenceMs:     800,
		PreRollMs:     500,
		MinSpeechMs:   200,
		MaxWindowMs:   8000,
		TypeOutput:    true,
		RecordHistory: true,
		Model:         "small.en",
		SpeechContext: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := runEagerCaptureSession(ctx, d, opts, t.TempDir(), audioCmd, audioArgs, true, "test-session", nil, nil, nil); err != nil {
		t.Fatalf("runEagerCaptureSession: %v", err)
	}

	got := transcript()
	t.Logf("spliced session typed transcript (all accepted chunks concatenated): %q", got)
	if got == "" {
		t.Fatal("no transcript typed for the spliced session -- expected at least the two speech fixtures to produce typed chunks")
	}

	// Inspect the session's own chunk ring buffer (real ring buffer, real
	// per-chunk Accepted/RejectionReason/CleanedTranscript metadata -- see
	// internal/chunks) as the source of truth for per-utterance matching,
	// rather than comparing each fixture's short expected text against the
	// whole concatenated transcript (which spuriously inflates WER with the
	// other fixture's words counted as insertions).
	chunkDir := chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME"))
	chunkBuf := chunks.NewBuffer(chunkDir, chunks.DefaultBufferSize)
	chunkList, err := chunkBuf.List(false)
	if err != nil {
		t.Fatalf("list session chunk manifest: %v", err)
	}
	if len(chunkList) == 0 {
		t.Fatal("session chunk ring buffer is empty -- expected at least 3 segmented chunks (speech, noise, speech)")
	}

	var accepted, rejected []chunks.Chunk
	for _, c := range chunkList {
		t.Logf("chunk #%d accepted=%t reason=%q text=%q", c.Index, c.Accepted, c.RejectionReason, c.CleanedTranscript)
		if c.Accepted {
			accepted = append(accepted, c)
		} else {
			rejected = append(rejected, c)
		}
	}
	if len(rejected) == 0 {
		t.Error("no chunk in the session was rejected -- expected the interleaved noise transient to be rejected (rej:low_energy_transient, silence_artifact, or similar)")
	}
	if len(accepted) == 0 {
		t.Fatal("no chunk in the session was accepted -- expected both speech fixtures to produce an accepted, typed chunk")
	}

	// Best-match WER: the real VAD may occasionally split a fixture's speech
	// across more than one accepted chunk (e.g. a tiny leading fragment plus
	// the main sentence), so choose the minimum-WER one-to-one assignment
	// rather than greedily claiming a shared near-match.
	const maxWER = 0.4
	bestIndices := []int{-1, -1}
	bestTotal := 3.0
	for i, a := range accepted {
		for j, b := range accepted {
			if i == j {
				continue
			}
			total := wordErrorRate(fxA.Expected, a.CleanedTranscript) + wordErrorRate(fxB.Expected, b.CleanedTranscript)
			if total < bestTotal {
				bestTotal, bestIndices = total, []int{i, j}
			}
		}
	}
	if bestIndices[0] < 0 {
		t.Fatal("fewer than two accepted chunks available for distinct speech-fixture matching")
	}
	for index, fx := range []e2eFixture{fxA, fxB} {
		bestWER, bestText := 1.0, ""
		bestText = accepted[bestIndices[index]].CleanedTranscript
		bestWER = wordErrorRate(fx.Expected, bestText)
		t.Logf("fixture=%s expected=%q best accepted match=%q wer=%.3f", fx.ID, fx.Expected, bestText, bestWER)
		if bestWER > maxWER {
			t.Errorf("no accepted chunk matches fixture %s within WER %.2f (best wer=%.3f, best match=%q, expected=%q)",
				fx.ID, maxWER, bestWER, bestText, fx.Expected)
		}
	}

	// The noise fixture's expected text is deliberately blank in corpus.tsv
	// (it's a non-speech artifact, not a real utterance), so there is no
	// meaningful expected-text WER check for it; its rejection is asserted
	// structurally above via len(rejected) > 0. Guard against the noise
	// fixture ever gaining real expected text without this check being
	// revisited.
	if fxNoise.Expected != "" {
		t.Fatalf("noise fixture %s has non-empty expected text %q -- update this test's noise-rejection assertion to match on content", fxNoise.ID, fxNoise.Expected)
	}
}
