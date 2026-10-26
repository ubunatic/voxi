package eager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/feedback"
	"ubunatic.com/voxi/internal/history"
	"ubunatic.com/voxi/internal/speechcontext"
	"ubunatic.com/voxi/internal/telemetry"
	"ubunatic.com/voxi/internal/typing"
	spec "ubunatic.com/voxi/spec"
)

// EagerOptions holds configuration parameters for Continuous Eager Sentence Streaming dictation.
type EagerOptions struct {
	ThresholdRMS      int
	SilenceMs         int
	PreRollMs         int
	MinSpeechMs       int
	MaxWindowMs       int
	TypeOutput        bool
	RecordHistory     bool
	HistorySet        bool
	Model             string
	ModelSet          bool
	Daemon            bool
	ModifierGating    bool
	ModifierGatingSet bool
	SpeechContext     bool
	Vocabulary        []string

	// AudioCapture optionally overrides audio capture (e.g. for testing without audio hardware).
	AudioCapture          AudioCapture
	audioFailurePublisher *eagerAudioFailurePublisher
}

// DefaultEagerOptions returns standard defaults for eager sentence streaming dictation.
// The default model name comes from spec/models.yaml, not a hardcoded value.
func DefaultEagerOptions() EagerOptions {
	s, err := spec.LoadModels()
	if err != nil {
		// spec/models.yaml is embedded at build time and schema-checked by
		// spec's own tests; a load failure here means a broken build.
		panic(err)
	}
	return EagerOptions{
		ThresholdRMS:   150,
		SilenceMs:      800,
		PreRollMs:      500,
		MinSpeechMs:    200,
		MaxWindowMs:    8000,
		TypeOutput:     true,
		RecordHistory:  true,
		ModifierGating: true,
		Daemon:         false,
		Model:          s.DefaultModel,
		SpeechContext:  true,
	}
}

// UtteranceStat records timing, speed, and text for one transcribed phrase.
type UtteranceStat struct {
	Index          int       `json:"index"`
	AudioSecs      float64   `json:"audio_secs"`
	TranscribeSecs float64   `json:"transcribe_secs"`
	RTF            float64   `json:"rtf"`
	Text           string    `json:"text"`
	Timestamp      time.Time `json:"timestamp"`
}

// EagerMetrics holds aggregated throughput and recent sentence history.
type EagerMetrics struct {
	TotalChunks         int             `json:"total_chunks"`
	TotalAudioSecs      float64         `json:"total_audio_secs"`
	TotalTranscribeSecs float64         `json:"total_transcribe_secs"`
	AvgRTF              float64         `json:"avg_rtf"`
	LastUtterance       *UtteranceStat  `json:"last_utterance,omitempty"`
	Recent              []UtteranceStat `json:"recent"`
}

var (
	eagerMetricsLock sync.Mutex
	eagerMetrics     EagerMetrics
)

func recordEagerStat(stat UtteranceStat) {
	eagerMetricsLock.Lock()
	defer eagerMetricsLock.Unlock()

	eagerMetrics.TotalChunks++
	eagerMetrics.TotalAudioSecs += stat.AudioSecs
	eagerMetrics.TotalTranscribeSecs += stat.TranscribeSecs
	if eagerMetrics.TotalAudioSecs > 0 {
		eagerMetrics.AvgRTF = eagerMetrics.TotalTranscribeSecs / eagerMetrics.TotalAudioSecs
	}
	eagerMetrics.LastUtterance = &stat
	eagerMetrics.Recent = append([]UtteranceStat{stat}, eagerMetrics.Recent...)
	if len(eagerMetrics.Recent) > 10 {
		eagerMetrics.Recent = eagerMetrics.Recent[:10]
	}

	data, err := json.MarshalIndent(eagerMetrics, "", "  ")
	if err == nil {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
		}
		voxiDir := filepath.Join(runtimeDir, "voxi")
		_ = os.MkdirAll(voxiDir, 0755)
		_ = os.WriteFile(filepath.Join(voxiDir, "eager-metrics.json"), data, 0644)
	}
}

func writeVoxtypeState(state string) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	stateDir := filepath.Join(runtimeDir, "voxtype")
	_ = os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "state")
	_ = os.WriteFile(stateFile, []byte(state+"\n"), 0644)

	// Write to voxi state
	voxiDir := filepath.Join(runtimeDir, "voxi")
	_ = os.MkdirAll(voxiDir, 0755)
	_ = os.WriteFile(filepath.Join(voxiDir, "voice-state"), []byte(state+"\n"), 0644)
}

func writeEagerChunkWAV(tempDir string, index int, pcm []byte) (string, error) {
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return "", fmt.Errorf("create eager audio temp directory %q: %w", tempDir, err)
	}
	wavPath := filepath.Join(tempDir, fmt.Sprintf("utt_%03d.wav", index))
	if err := audio.WriteWAVAudio(wavPath, pcm, sampleRate); err != nil {
		return "", fmt.Errorf("write eager audio %q: %w", wavPath, err)
	}
	return wavPath, nil
}

// gpuAvailable reports whether a GPU render node is present for Vulkan
// acceleration (see internal/monitor's equivalent check).
func gpuAvailable() bool {
	_, err := os.Stat("/dev/dri/renderD128")
	return err == nil
}

func requireEngineBinary(ctx context.Context, d deps.Dependencies, modelName, engine string) (transcribeBinPath, weightsPath string, err error) {
	switch engine {
	case "", "whisper":
		voxtypePath, lookErr := d.LookPath("voxtype")
		if lookErr != nil {
			return "", "", fmt.Errorf("eager: model %q needs engine %q, which requires the %q binary; not found on PATH: %w", modelName, "whisper", "voxtype", lookErr)
		}
		return voxtypePath, "", nil
	case cohereTranscribeEngine:
		crispasrPath, lookErr := d.LookPath(crispasrBinary)
		if lookErr != nil {
			return "", "", fmt.Errorf("eager: model %q needs engine %q, which requires the %q binary; not found on PATH: %w", modelName, engine, crispasrBinary, lookErr)
		}
		wp, weightsErr := ensureCohereWeights(ctx, d)
		if weightsErr != nil {
			return "", "", fmt.Errorf("eager: %w", weightsErr)
		}
		return crispasrPath, wp, nil
	case openaiTranscribeEngine:
		return "", "", nil
	default:
		return "", "", fmt.Errorf("eager: model %q has unknown engine %q", modelName, engine)
	}
}

// checkEagerModelReady resolves opts.Model to its engine and validates that engine's
// runtime dependency via requireEngineBinary.
func checkEagerModelReady(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("eager: load model spec: %w", err)
	}
	modelName := opts.Model
	if modelName == "" {
		modelName = modelSpec.DefaultModel
	}
	resolvedModel, _, err := modelSpec.ResolveModel(modelName, gpuAvailable())
	if err != nil {
		return fmt.Errorf("eager: %w", err)
	}
	_, _, err = requireEngineBinary(ctx, d, resolvedModel, modelSpec.Models[resolvedModel].Engine)
	return err
}

// RunEagerDictation orchestrates continuous audio capture, rolling phrase segmentation,
// transcription through the selected model's engine, instant text typing via dotool,
// and history appending.
func RunEagerDictation(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	settings, _ := config.LoadUserSettings(d.Getenv("HOME"))
	if settings != nil {
		opts = applySavedSettings(opts, settings)
	}
	if opts.Daemon {
		return runEagerDaemon(ctx, d, opts)
	}

	recCmdName := ""
	var recArgs []string

	if opts.AudioCapture == nil {
		if _, err := d.LookPath("pw-record"); err == nil {
			recCmdName = "pw-record"
			recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
		} else if _, err := d.LookPath("arecord"); err == nil {
			recCmdName = "arecord"
			recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
		} else {
			return fmt.Errorf("neither pw-record nor arecord found on PATH")
		}
	}

	if err := checkEagerModelReady(ctx, d, opts); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "voxi-eager-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintln(d.Stdout, "── Continuous Eager Sentence Streaming Dictation ───────────────")
	fmt.Fprintf(d.Stdout, "  Audio Source:          %s (16kHz mono S16_LE)\n", recCmdName)
	fmt.Fprintf(d.Stdout, "  RMS Threshold:         %d\n", opts.ThresholdRMS)
	fmt.Fprintf(d.Stdout, "  Silence Cutoff:        %d ms\n", opts.SilenceMs)
	fmt.Fprintf(d.Stdout, "  Pre-roll Buffer:       %d ms (preserves initial phonemes)\n", opts.PreRollMs)
	fmt.Fprintf(d.Stdout, "  Type into window:      %t\n", opts.TypeOutput)
	fmt.Fprintf(d.Stdout, "  Record history:        %t\n", opts.RecordHistory)
	fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")
	fmt.Fprintln(d.Stdout, "Instructions:")
	fmt.Fprintln(d.Stdout, "  Speak naturally with conversational pauses. Sentences will transcribe")
	fmt.Fprintln(d.Stdout, "  and type immediately upon each pause with zero dropped words.")
	fmt.Fprintln(d.Stdout, "  Press Ctrl-C (or cancel context) to stop dictation.")
	fmt.Fprintln(d.Stdout, "")

	recorder := telemetry.NewRecorder(telemetry.Path(d.Getenv("XDG_DATA_HOME"), d.Getenv("HOME")))
	activatedAt := time.Now()
	sessionID := recorder.NewSessionID(activatedAt)
	_ = recorder.Record(telemetry.Event{Event: telemetry.MicActivated, Timestamp: activatedAt, SessionID: sessionID})
	defer func() {
		_ = recorder.Record(telemetry.Event{Event: telemetry.MicDeactivated, Timestamp: time.Now(), SessionID: sessionID})
	}()

	return runEagerCaptureSessionAt(ctx, d, opts, tmpDir, recCmdName, recArgs, false, sessionID, nil, recorder, nil, nil)
}

func applySavedSettings(opts EagerOptions, settings *config.UserSettings) EagerOptions {
	if settings == nil {
		return opts
	}
	if !opts.HistorySet {
		opts.RecordHistory = settings.DictationHistory
	}
	if !opts.ModifierGatingSet {
		opts.ModifierGating = settings.ModifierGating
	}
	if !opts.ModelSet {
		opts.Model = settings.ASRModel
	}
	return opts
}

// runEagerCaptureSession runs one audio-capture + sequential-transcription session.
func runEagerCaptureSession(ctx context.Context, d deps.Dependencies, opts EagerOptions, tmpDir string, recCmdName string, recArgs []string, isDaemon bool, sessionID string, recorder *telemetry.Recorder, onCaptureStopped func(), sessions *eagerSessionManager) error {
	return runEagerCaptureSessionAt(ctx, d, opts, tmpDir, recCmdName, recArgs, isDaemon, sessionID, nil, recorder, onCaptureStopped, sessions)
}

func runEagerCaptureSessionAt(ctx context.Context, d deps.Dependencies, opts EagerOptions, tmpDir string, recCmdName string, recArgs []string, isDaemon bool, sessionID string, request *stopRequest, recorder *telemetry.Recorder, onCaptureStopped func(), sessions *eagerSessionManager) error {
	var signalOnce sync.Once
	signalCaptureStopped := func() {
		if onCaptureStopped != nil {
			signalOnce.Do(onCaptureStopped)
		}
	}
	defer signalCaptureStopped()

	segOpts := audio.SegmenterOptions{
		ThresholdRMS: opts.ThresholdRMS,
		SilenceMs:    opts.SilenceMs,
		PreRollMs:    opts.PreRollMs,
		MinSpeechMs:  opts.MinSpeechMs,
		MaxWindowMs:  opts.MaxWindowMs,
	}
	segmenter := audio.NewAudioSegmenter(segOpts)
	historyPath := history.HistoryPath(d.Getenv("HOME"))

	type TranscribeJob struct {
		Index           int
		Audio           []byte
		Duration        float64
		FinalizedAt     time.Time
		Stats           audio.AudioStats
		Plausible       bool
		RejectionReason string
		Final           bool
		Eligible        bool
	}

	modelSpec, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("eager: load model spec: %w", err)
	}
	eagerSpec, err := spec.LoadEager()
	if err != nil {
		return fmt.Errorf("eager: load eager spec: %w", err)
	}
	modifierTimeout := eagerSpec.ModifierTimeout()
	modifierNotifyDelay := eagerSpec.ModifierNotifyDelay()
	modelName := opts.Model
	if modelName == "" {
		modelName = modelSpec.DefaultModel
	}
	resolvedModel, usedFallback, err := modelSpec.ResolveModel(modelName, gpuAvailable())
	if err != nil {
		return fmt.Errorf("eager: %w", err)
	}
	if usedFallback {
		fmt.Fprintf(d.Stdout, "Model %q requires GPU acceleration; no GPU render node found, falling back to %q\n", modelName, resolvedModel)
	}
	modelName = resolvedModel
	stopWords := modelSpec.StopWords(modelName)
	initialPrompt := ""
	if shouldUseSpeechContext(modelName, opts.SpeechContext) {
		explicit := append([]string(nil), opts.Vocabulary...)
		vocabularyPath := speechcontext.VocabularyPath(d.Getenv("HOME"))
		if vocabularyPath != "" && d.ReadFile != nil {
			if data, readErr := d.ReadFile(vocabularyPath); readErr == nil {
				explicit = append(explicit, speechcontext.ParseVocabulary(data)...)
			} else if !os.IsNotExist(readErr) {
				fmt.Fprintf(d.Stdout, "Warning: cannot load local speech vocabulary: %v\n", readErr)
			}
		}
		cwd, _ := os.Getwd()
		initialPrompt = speechcontext.Build(speechcontext.Options{
			Enabled:      true,
			PromptPrefix: modelSpec.SpeechContext.PromptPrefix,
			MaxTerms:     modelSpec.SpeechContext.MaxTerms,
			MaxChars:     modelSpec.SpeechContext.MaxChars,
			MaxTermChars: modelSpec.SpeechContext.MaxTermChars,
		}, speechcontext.Sources{
			Explicit:   explicit,
			Static:     modelSpec.SpeechContext.Terms,
			Repository: speechcontext.DiscoverRepositoryTerms(ctx, cwd, 12),
		})
	}
	var silenceArtifacts []string
	if overrides, loadErr := feedback.Load(feedback.Path(d.Getenv("HOME"))); loadErr != nil {
		fmt.Fprintf(d.Stdout, "Warning: cannot load local stop-word feedback; using built-ins: %v\n", loadErr)
	} else {
		stopWords = feedback.ActivePatterns(modelSpec.BuiltinStopWords(modelName), overrides)
		silenceArtifacts = overrides.SilenceArtifacts
	}

	engine := modelSpec.Models[modelName].Engine
	replacements, err := feedback.LoadReplacements(feedback.ReplacementPath(d.Getenv("HOME")))
	if err != nil {
		fmt.Fprintf(d.Stdout, "Warning: cannot load transcript replacements; continuing without them: %v\n", err)
	}
	userSettings, _ := config.LoadUserSettings(d.Getenv("HOME"))
	transcribeBinPath, weightsPath, err := requireEngineBinary(ctx, d, modelName, engine)
	if err != nil {
		return err
	}
	if m := modelSpec.Models[modelName]; m.WakeURL != "" {
		go wakeASRServer(m.WakeURL, m.APIModel)
	}
	modelTranscribeTimeout := transcribeTimeoutFor(modelSpec.Models[modelName])
	buildTranscribeArgs := func(wavPath string) []string {
		return voxtypeTranscribeArgs(modelName, wavPath, initialPrompt)
	}
	if engine == cohereTranscribeEngine {
		buildTranscribeArgs = func(wavPath string) []string {
			return crispASRTranscribeArgs(weightsPath, wavPath)
		}
	}
	runTranscribe := func(ctx context.Context, wavPath string) (string, error) {
		if engine == openaiTranscribeEngine {
			m := modelSpec.Models[modelName]
			globalBaseURL := ""
			if userSettings != nil {
				globalBaseURL = userSettings.OpenAIASRBaseURL
			}
			baseURL := resolveOpenAIASRBaseURL(m.BaseURL, globalBaseURL)
			return transcribeOpenAIWAV(ctx, wavPath, baseURL, m.APIModel, m.ResponseFormat, m.StripBeforeMarker)
		}
		cmd := exec.CommandContext(ctx, transcribeBinPath, buildTranscribeArgs(wavPath)...)
		cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = io.Discard
		err := cmd.Run()
		return outBuf.String(), err
	}

	jobChan := make(chan TranscribeJob, 10)
	drain := newSessionDrainFor(request.generationCtx(), eagerSpec.DeliveryDeadline())
	injectionTimeout := eagerSpec.InjectionTimeout()
	var transWg sync.WaitGroup
	var fullTranscript strings.Builder
	var transLock sync.Mutex
	var eagerBuf modifierBuffer
	utteranceCount := 0

	writeVoxtypeState("recording")
	defer writeVoxtypeState("idle")

	chunkBuf := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)
	deliveryPath := ""
	if recorder != nil {
		deliveryPath = filepath.Join(filepath.Dir(telemetry.Path(d.Getenv("XDG_DATA_HOME"), d.Getenv("HOME"))), "eager-delivery-ledger.jsonl")
	}
	delivery := newDeliveryLedger(deliveryPath)

	transWg.Add(1)
	go func() {
		defer transWg.Done()
		for job := range jobChan {
			chunkID := fmt.Sprintf("%s/%d", sessionID, job.Index)
			wavPath, err := writeEagerChunkWAV(tmpDir, job.Index, job.Audio)
			if err != nil {
				opts.audioFailurePublisher.Failure(err)
				reportEagerFailure(d, "audio", sessionID, chunkID, err)
				continue
			}
			opts.audioFailurePublisher.Success()
			if !job.Eligible {
				recordStaleDelivery(d, recorder, sessionID, chunkID, job.Index, dropCaptureBoundary)
				_ = os.Remove(wavPath)
				continue
			}

			if !job.Plausible {
				rejReason := job.RejectionReason
				if rejReason == "" {
					rejReason = "low_energy_transient"
				}
				chunkMeta := chunks.Chunk{
					Index:                 job.Index,
					Timestamp:             job.FinalizedAt,
					SessionID:             sessionID,
					ChunkID:               chunkID,
					FinalizedAt:           job.FinalizedAt,
					AudioDurationSecs:     job.Duration,
					PCMBytes:              len(job.Audio),
					MeanRMS:               job.Stats.MeanRMS,
					PeakRMS:               job.Stats.PeakRMS,
					VolumeSparkline:       audio.RenderVolumeSparkline(job.Audio, 10),
					VoicedRatio:           job.Stats.VoicedRatio,
					ProbableSilence:       probableSilence(job.Stats),
					TranscribeDurationSec: 0,
					RTF:                   0,
					RawTranscript:         "",
					CleanedTranscript:     "",
					Accepted:              false,
					RejectionReason:       rejReason,
					Model:                 modelName,
					Engine:                engine,
				}
				_, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
				_ = os.Remove(wavPath)
				continue
			}

			writeVoxtypeState("transcribing")
			transcribeParent := drain.ctx
			transcribeCtx, cancelTranscribe := context.WithTimeout(transcribeParent, modelTranscribeTimeout)
			transStart := time.Now()
			_ = recorder.Record(telemetry.Event{Event: telemetry.TranscriptionStarted, Timestamp: transStart, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index})
			rawText, err := runTranscribe(transcribeCtx, wavPath)
			cancelTranscribe()
			if drain.ctx.Err() != nil {
				_ = recorder.Record(telemetry.Event{Event: telemetry.StopDrainTimeout, Timestamp: time.Now(), SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, Error: drain.ctx.Err().Error()})
			}
			transEnd := time.Now()
			transDuration := transEnd.Sub(transStart).Seconds()
			writeVoxtypeState("recording")
			text, matchedStopWords := asr.CleanWhisperTranscriptWithAudit(rawText, stopWords)
			var appliedReplacements []chunks.ReplacementSummary
			text, appliedRules := feedback.ApplyReplacementsWithAudit(text, replacements)
			for _, r := range appliedRules {
				appliedReplacements = append(appliedReplacements, chunks.ReplacementSummary{
					From: r.From,
					To:   r.To,
				})
			}

			var llmRecord *chunks.LLMCleanupRecord
			if userSettings != nil && userSettings.LLMCleaner && !modelSpec.Models[modelName].DoesLLMCleanup {
				text, llmRecord = cleanWithLLM(drain.ctx, text, userSettings, llmChunkContext{
					MeanRMS:             job.Stats.MeanRMS,
					PeakRMS:             job.Stats.PeakRMS,
					AppliedReplacements: appliedReplacements,
				})
				if llmRecord != nil && llmRecord.FallbackReason != "" {
					_ = recorder.Record(telemetry.Event{Event: telemetry.LLMCleanupFallback, Timestamp: time.Now(), SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, CancelReason: llmRecord.FallbackReason})
				}
			}
			accepted := acceptTranscript(err, text, stopWords, silenceArtifacts)
			safety := asr.CheckTranscriptSafety(text, modelSpec.TranscriptSafety)
			if safety.Reason != "" {
				accepted = false
			}
			wordCount := len(strings.Fields(text))
			transSuccess := err == nil
			transError := ""
			if err != nil {
				transError = err.Error()
				reportEagerFailure(d, "transcription", sessionID, chunkID, err)
			}
			_ = recorder.Record(telemetry.Event{Event: telemetry.TranscriptionComplete, Timestamp: transEnd, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, TranscriptWordCount: &wordCount, Success: &transSuccess, Error: transError})
			rejReason := ""
			if !accepted {
				rejReason = rejectionReason(err, rawText, text, stopWords, silenceArtifacts)
				if safety.Reason != "" {
					rejReason = safety.Reason
				}
			}

			rtf := 0.0
			if job.Duration > 0 {
				rtf = transDuration / job.Duration
			}

			// Store chunk audio and metadata into the bounded ring buffer
			chunkMeta := chunks.Chunk{
				Index:                  job.Index,
				Timestamp:              job.FinalizedAt,
				SessionID:              sessionID,
				ChunkID:                chunkID,
				FinalizedAt:            job.FinalizedAt,
				TranscriptionStartedAt: transStart,
				TranscriptionEndedAt:   transEnd,
				AudioDurationSecs:      job.Duration,
				PCMBytes:               len(job.Audio),
				MeanRMS:                job.Stats.MeanRMS,
				PeakRMS:                job.Stats.PeakRMS,
				VolumeSparkline:        audio.RenderVolumeSparkline(job.Audio, 10),
				VoicedRatio:            job.Stats.VoicedRatio,
				ProbableSilence:        probableSilence(job.Stats),
				TranscribeDurationSec:  transDuration,
				TranscriptWordCount:    wordCount,
				RTF:                    rtf,
				RawTranscript:          strings.TrimSpace(rawText),
				CleanedTranscript:      text,
				Accepted:               accepted,
				RejectionReason:        rejReason,
				TranscriptChars:        safety.Chars,
				TranscriptDigest:       safety.Digest,
				RepeatUnit:             safety.RepeatUnit,
				RepeatCount:            safety.RepeatCount,
				Model:                  modelName,
				Engine:                 engine,
				AppliedReplacements:    appliedReplacements,
				LLMCleanup:             llmRecord,
				StopWordsMatched:       matchedStopWords,
			}
			if safety.Reason != "" {
				// Do not persist a pathological transcript's private, potentially
				// enormous contents; the digest and structural facts are sufficient.
				chunkMeta.RawTranscript = ""
				chunkMeta.CleanedTranscript = ""
			}
			_, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
			_ = os.Remove(wavPath) // Ensure removal if AddExistingWAV didn't move it

			if accepted {
				transLock.Lock()
				if fullTranscript.Len() > 0 {
					fullTranscript.WriteString(" ")
				}
				fullTranscript.WriteString(text)
				transLock.Unlock()

				if !isDaemon {
					fmt.Fprintf(d.Stdout, "  #%d [Audio: %.1fs, Transcribe: %.2fs] -> \x1b[32;1m%q\x1b[0m\n",
						job.Index, job.Duration, transDuration, text)
				}

				if opts.TypeOutput {
					// Modifier-release race guard (issue 101): buffer instead of
					// typing immediately if a gating modifier was pressed
					// recently. Buffering is sticky for the rest of the session
					// once entered -- only an explicit stop (see the flush after
					// transWg.Wait() below) releases it -- since a modifier press
					// going stale mid-buffer does not retroactively make it safe
					// to type into whatever now has focus.
					buffer, justEntered := false, false
					if opts.ModifierGating {
						buffer, justEntered = eagerBuf.EnterIfNeeded(sessions, modifierTimeout)
					}
					if buffer {
						eagerBuf.AppendDelivery(chunkID, text+" ", job.Index, chunkMeta)
						if justEntered {
							eagerBuf.ScheduleNotify(d, modifierNotifyDelay)
						}
					} else {
						if ok, reason := drain.deliverable(); !ok {
							recordStaleDelivery(d, recorder, sessionID, chunkID, job.Index, reason)
							continue
						}
						claimed, claimErr := delivery.Claim(chunkID)
						if claimErr != nil {
							accepted = false
							_ = recorder.Record(telemetry.Event{Event: telemetry.DeliveryDuplicate, Timestamp: time.Now(), SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, DeliveryID: chunkID, Error: claimErr.Error()})
							continue
						}
						if !claimed {
							_ = recorder.Record(telemetry.Event{Event: telemetry.DeliveryDuplicate, Timestamp: time.Now(), SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, DeliveryID: chunkID})
							continue
						}
						typeStart := time.Now()
						_ = recorder.Record(telemetry.Event{Event: telemetry.TypingStarted, Timestamp: typeStart, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, DeliveryID: chunkID, Attempt: 1})
						if ok, reason := drain.deliverable(); !ok {
							recordStaleDelivery(d, recorder, sessionID, chunkID, job.Index, reason)
							continue
						}
						injectCtx, cancelInject := newInjectionContext(injectionTimeout)
						typeErr := typing.TypeTextObservedWithGating(injectCtx, d, text+" ", &injectorObserver{recorder: recorder, output: d.Stdout, sessionID: sessionID, chunkID: chunkID, chunkIndex: job.Index, deliveryID: chunkID}, opts.ModifierGating)
						cancelInject()
						typeEnd := time.Now()
						typeSuccess := typeErr == nil
						typeError := ""
						if typeErr != nil {
							typeError = typeErr.Error()
							reportEagerFailure(d, "typing", sessionID, chunkID, typeErr)
						}
						_ = recorder.Record(telemetry.Event{Event: telemetry.TypingComplete, Timestamp: typeEnd, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, DeliveryID: chunkID, Attempt: 1, Success: &typeSuccess, Error: typeError})
						chunkMeta.TypingStartedAt = typeStart
						chunkMeta.TypingEndedAt = typeEnd
						_, _ = chunkBuf.Update(chunkMeta)
					}
				}

				if err := appendDictationHistory(historyPath, text, opts.RecordHistory); err != nil {
					reportEagerFailure(d, "history", sessionID, chunkID, err)
				}

				recordEagerStat(UtteranceStat{
					Index:          job.Index,
					AudioSecs:      job.Duration,
					TranscribeSecs: transDuration,
					RTF:            rtf,
					Text:           text,
					Timestamp:      time.Now(),
				})
			}
		}
	}()

	capture := opts.AudioCapture
	if capture == nil {
		capture = newCommandAudioCapture(recCmdName, recArgs)
	}

	beginPlaybackMute(ctx, d)
	audioOut, err := capture.Start(ctx)
	if err != nil {
		endPlaybackMute(ctx, d)
		return fmt.Errorf("start audio capture: %w", err)
	}
	_ = recorder.Record(telemetry.Event{Event: telemetry.CaptureStarted, Timestamp: time.Now(), SessionID: sessionID})

	killDone := make(chan struct{})
	micAbort := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-micAbort:
		case <-killDone:
			return
		}
		capture.Kill()
	}()

	watch := startMicWatch(ctx, d, eagerSpec, bytesPerSec, func() {
		drain.abandon(dropMicRepaired)
		close(micAbort)
		if sessions != nil {
			go sessions.StopSession(sessionID)
		}
	})

	buf := make([]byte, frameBytes)
	frameIndex := 0

	if !isDaemon {
		fmt.Fprintln(d.Stdout, "🟢 Listening for voice activity...")
	}

	for {
		if ctx.Err() != nil {
			break
		}

		n, err := io.ReadFull(audioOut, buf)
		readCompletedAt := time.Now()
		if d.AfterAudioRead != nil {
			d.AfterAudioRead()
		}
		if err != nil {
			break
		}
		if ctx.Err() != nil {
			stopAt := time.Time{}
			if request != nil {
				stopAt = request.timestamp()
			}
			if stopAt.IsZero() || readCompletedAt.After(stopAt) {
				if stopAt.IsZero() {
					drain.stop()
				} else {
					drain.stopAt(stopAt)
				}
				break
			}
		}
		if n < frameBytes {
			continue
		}
		watch.feed(buf)

		candidate, speechStarted, isSpeaking := segmenter.ProcessFrame(buf)

		frameIndex++
		if !isDaemon {
			if speechStarted {
				fmt.Fprintf(d.Stdout, "\r🎙️  [Speaking... buffering]                                  \n")
			} else if frameIndex%10 == 0 && !isSpeaking {
				rms := audio.ComputeAudioRMS(buf)
				meter := audio.RenderAudioLevelMeter(rms, opts.ThresholdRMS)
				fmt.Fprintf(d.Stdout, "\r  Level: %s RMS: %4d   ", meter, rms)
			}
		}

		if len(candidate.Audio) > 0 {
			utteranceCount++
			finalizedAt := time.Now()
			audioDur := float64(len(candidate.Audio)) / float64(bytesPerSec)
			chunkID := fmt.Sprintf("%s/%d", sessionID, utteranceCount)
			metrics := telemetry.AudioMetrics{DurationSecs: audioDur, PCMBytes: len(candidate.Audio), MeanRMS: candidate.Stats.MeanRMS, PeakRMS: candidate.Stats.PeakRMS, VoicedRatio: candidate.Stats.VoicedRatio, ProbableSilence: probableSilence(candidate.Stats)}
			_ = recorder.Record(telemetry.Event{Event: telemetry.ChunkFinalized, Timestamp: finalizedAt, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: utteranceCount, Audio: &metrics})
			jobChan <- TranscribeJob{
				Index:           utteranceCount,
				Audio:           candidate.Audio,
				Duration:        audioDur,
				FinalizedAt:     finalizedAt,
				Stats:           candidate.Stats,
				Plausible:       candidate.Plausible,
				RejectionReason: candidate.RejectionReason,
				Eligible:        drain.eligibleAt(readCompletedAt),
			}
		}
	}

	if request != nil {
		if at := request.timestamp(); !at.IsZero() {
			drain.stopAt(at)
		} else {
			drain.stop()
		}
	} else {
		drain.stop()
	}
	close(killDone)
	capture.Kill()
	_ = capture.Wait()
	watch.stop()

	endPlaybackMute(ctx, d)
	_ = recorder.Record(telemetry.Event{Event: telemetry.CaptureStopped, Timestamp: time.Now(), SessionID: sessionID})
	signalCaptureStopped()

	if finalCandidate := segmenter.Flush(); len(finalCandidate.Audio) > 0 {
		utteranceCount++
		finalizedAt := time.Now()
		audioDur := float64(len(finalCandidate.Audio)) / float64(bytesPerSec)
		chunkID := fmt.Sprintf("%s/%d", sessionID, utteranceCount)
		metrics := telemetry.AudioMetrics{DurationSecs: audioDur, PCMBytes: len(finalCandidate.Audio), MeanRMS: finalCandidate.Stats.MeanRMS, PeakRMS: finalCandidate.Stats.PeakRMS, VoicedRatio: finalCandidate.Stats.VoicedRatio, ProbableSilence: probableSilence(finalCandidate.Stats)}
		_ = recorder.Record(telemetry.Event{Event: telemetry.ChunkFinalized, Timestamp: finalizedAt, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: utteranceCount, Audio: &metrics})
		jobChan <- TranscribeJob{
			Index:           utteranceCount,
			Audio:           finalCandidate.Audio,
			Duration:        audioDur,
			FinalizedAt:     finalizedAt,
			Stats:           finalCandidate.Stats,
			Plausible:       finalCandidate.Plausible,
			RejectionReason: finalCandidate.RejectionReason,
			Final:           true,
			Eligible:        true,
		}
	}

	close(jobChan)
	transWg.Wait()

	if pending, wasBuffering := eagerBuf.FlushDeliveries(); wasBuffering {
		for _, item := range pending {
			if ok, reason := drain.deliverable(); !ok {
				recordStaleDelivery(d, recorder, sessionID, item.ID, item.Index, reason)
				continue
			}
			claimed, claimErr := delivery.Claim(item.ID)
			if claimErr != nil || !claimed {
				event := telemetry.Event{Event: telemetry.DeliveryDuplicate, Timestamp: time.Now(), SessionID: sessionID, ChunkID: item.ID, ChunkIndex: item.Index, DeliveryID: item.ID}
				if claimErr != nil {
					event.Error = claimErr.Error()
				}
				_ = recorder.Record(event)
				continue
			}
			typeStart := time.Now()
			_ = recorder.Record(telemetry.Event{Event: telemetry.TypingStarted, Timestamp: typeStart, SessionID: sessionID, ChunkID: item.ID, ChunkIndex: item.Index, DeliveryID: item.ID, Attempt: 1})
			injectCtx, cancelInject := newInjectionContext(injectionTimeout)
			typeErr := typing.TypeTextObserved(injectCtx, d, item.Text, &injectorObserver{recorder: recorder, output: d.Stdout, sessionID: sessionID, chunkID: item.ID, chunkIndex: item.Index, deliveryID: item.ID})
			cancelInject()
			typeEnd := time.Now()
			typeSuccess := typeErr == nil
			typeError := ""
			if typeErr != nil {
				typeError = typeErr.Error()
				reportEagerFailure(d, "typing", sessionID, item.ID, typeErr)
			}
			_ = recorder.Record(telemetry.Event{Event: telemetry.TypingComplete, Timestamp: typeEnd, SessionID: sessionID, ChunkID: item.ID, ChunkIndex: item.Index, DeliveryID: item.ID, Attempt: 1, Success: &typeSuccess, Error: typeError})
			if item.Meta.ChunkID != "" {
				item.Meta.TypingStartedAt = typeStart
				item.Meta.TypingEndedAt = typeEnd
				_, _ = chunkBuf.Update(item.Meta)
			}
		}
	}

	return nil
}
