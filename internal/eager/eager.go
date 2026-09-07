package eager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
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
	ThresholdRMS  int
	SilenceMs     int
	PreRollMs     int
	MinSpeechMs   int
	MaxWindowMs   int
	TypeOutput    bool
	RecordHistory bool
	Daemon        bool
	Model         string
	SpeechContext bool
	Vocabulary    []string
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
		ThresholdRMS:  150,
		SilenceMs:     800,
		PreRollMs:     500,
		MinSpeechMs:   200,
		MaxWindowMs:   8000,
		TypeOutput:    true,
		RecordHistory: true,
		Daemon:        false,
		Model:         s.DefaultModel,
		SpeechContext: true,
	}
}

// transcribeTimeout bounds a single `voxtype transcribe` subprocess so a
// stalled model or backend can never linger as a zombie process after
// recording stops; well above realistic transcription time even for the
// longest MaxWindowMs utterance on a slow CPU backend.
const transcribeTimeout = 30 * time.Second

// eagerSocketTimeout bounds a client's round trip to the eager daemon's
// control socket (start/stop/toggle/status). See issue 057: this is a
// safety net, not the primary fix -- the primary fix (eagerSessionManager)
// keeps the daemon side from ever blocking a request behind a stale
// transcription drain, so this deadline should not normally be reached.
const eagerSocketTimeout = 8 * time.Second

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

// gpuAvailable reports whether a GPU render node is present for Vulkan
// acceleration (see internal/monitor's equivalent check).
func gpuAvailable() bool {
	_, err := os.Stat("/dev/dri/renderD128")
	return err == nil
}

// RunEagerDictation orchestrates continuous audio capture, rolling phrase segmentation,
// Whisper transcription, instant text typing via dotool, and history appending.
func RunEagerDictation(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	if opts.Daemon {
		return runEagerDaemon(ctx, d, opts)
	}

	recCmdName := ""
	var recArgs []string

	if _, err := d.LookPath("pw-record"); err == nil {
		recCmdName = "pw-record"
		recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	} else if _, err := d.LookPath("arecord"); err == nil {
		recCmdName = "arecord"
		recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
	} else {
		return fmt.Errorf("neither pw-record nor arecord found on PATH")
	}

	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return fmt.Errorf("voxtype not found on PATH: %w", err)
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
	return runEagerCaptureSession(ctx, d, opts, tmpDir, voxtypePath, recCmdName, recArgs, false, sessionID, recorder, nil)
}

// runEagerCaptureSession runs one audio-capture + sequential-transcription
// session. onCaptureStopped, if non-nil, is invoked as soon as audio capture
// has ended and the recording subprocess has been reaped -- i.e. well before
// this function returns, since the return is additionally gated on draining
// any transcription jobs still queued or in flight (bounded by
// transcribeTimeout per job, not by capture stopping). Callers that need to
// know "is it safe to start a new session" without waiting for a stale
// transcription to finish should wait on onCaptureStopped rather than on this
// function's return (see issue 057 and eagerSessionManager below).
func runEagerCaptureSession(ctx context.Context, d deps.Dependencies, opts EagerOptions, tmpDir string, voxtypePath string, recCmdName string, recArgs []string, isDaemon bool, sessionID string, recorder *telemetry.Recorder, onCaptureStopped func()) error {
	// Guarantee onCaptureStopped fires exactly once no matter which of this
	// function's many return paths is taken -- including the early
	// `return fmt.Errorf(...)` guards below (model spec load failure, audio
	// pipe setup failure, and notably recCmd.Start() itself: exec.Command's
	// Start() returns ctx.Err() immediately without spawning a process at
	// all when ctx is already canceled, which happens routinely here since a
	// session can be asked to stop before its capture goroutine has even
	// gotten past setup). A version of this fix that only signaled from the
	// single "happy path" reap step left eagerSessionManager.Stop() blocked
	// forever on that exact race (confirmed live via a goroutine dump during
	// issue 057 verification), since nothing else ever closed its channel.
	var signalOnce sync.Once
	signalCaptureStopped := func() {
		if onCaptureStopped != nil {
			signalOnce.Do(onCaptureStopped)
		}
	}
	defer signalCaptureStopped()

	const (
		sampleRate  = 16000
		bytesPerSec = sampleRate * 2
		frameMs     = 20
		frameBytes  = (sampleRate * frameMs / 1000) * 2 // 640 bytes
	)

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
	}

	modelSpec, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("eager: load model spec: %w", err)
	}
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

	// Dispatch on the resolved model's engine: whisper (today's default,
	// unchanged) keeps using the voxtypePath/voxtypeTranscribeArgs already
	// resolved by the caller; cohere-transcribe resolves and invokes the
	// crispasr binary instead, lazily downloading its GGUF weights on first
	// use. See issue 074.
	engine := modelSpec.Models[modelName].Engine
	transcribeBinPath := voxtypePath
	buildTranscribeArgs := func(wavPath string) []string {
		return voxtypeTranscribeArgs(modelName, wavPath, initialPrompt)
	}
	switch engine {
	case "", "whisper":
		// unchanged default path above
	case cohereTranscribeEngine:
		crispasrPath, lookErr := d.LookPath(crispasrBinary)
		if lookErr != nil {
			return fmt.Errorf("eager: model %q needs engine %q, which requires the %q binary; not found on PATH: %w", modelName, engine, crispasrBinary, lookErr)
		}
		weightsPath, weightsErr := ensureCohereWeights(ctx, d)
		if weightsErr != nil {
			return fmt.Errorf("eager: %w", weightsErr)
		}
		transcribeBinPath = crispasrPath
		buildTranscribeArgs = func(wavPath string) []string {
			return crispASRTranscribeArgs(weightsPath, wavPath)
		}
	default:
		return fmt.Errorf("eager: model %q has unknown engine %q", modelName, engine)
	}

	jobChan := make(chan TranscribeJob, 10)
	var transWg sync.WaitGroup
	var fullTranscript strings.Builder
	var transLock sync.Mutex
	utteranceCount := 0

	writeVoxtypeState("recording")
	defer writeVoxtypeState("idle")

	chunkBuf := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)

	// Start sequential transcription worker
	transWg.Add(1)
	go func() {
		defer transWg.Done()
		for job := range jobChan {
			chunkID := fmt.Sprintf("%s/%d", sessionID, job.Index)
			wavPath := filepath.Join(tmpDir, fmt.Sprintf("utt_%03d.wav", job.Index))
			if err := audio.WriteWAVAudio(wavPath, job.Audio, sampleRate); err != nil {
				if !isDaemon {
					fmt.Fprintf(d.Stdout, "Error writing utterance audio: %v\n", err)
				}
				continue
			}

			if !job.Plausible {
				// Acoustically unvoiced transient or click: do not waste GPU/CPU on voxtype
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
				}
				_, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
				_ = os.Remove(wavPath)
				continue
			}

			writeVoxtypeState("transcribing")
			cmdArgs := buildTranscribeArgs(wavPath)
			transcribeCtx, cancelTranscribe := context.WithTimeout(context.Background(), transcribeTimeout)
			cmd := exec.CommandContext(transcribeCtx, transcribeBinPath, cmdArgs...)
			cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
			var outBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = io.Discard
			transStart := time.Now()
			_ = recorder.Record(telemetry.Event{Event: telemetry.TranscriptionStarted, Timestamp: transStart, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index})
			err := cmd.Run()
			cancelTranscribe()
			transEnd := time.Now()
			transDuration := transEnd.Sub(transStart).Seconds()
			writeVoxtypeState("recording")

			rawText := outBuf.String()
			text := asr.CleanWhisperTranscript(rawText, stopWords)
			accepted := acceptTranscript(err, text, stopWords, silenceArtifacts)
			wordCount := len(strings.Fields(text))
			transSuccess := err == nil
			transError := ""
			if err != nil {
				transError = err.Error()
			}
			_ = recorder.Record(telemetry.Event{Event: telemetry.TranscriptionComplete, Timestamp: transEnd, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, TranscriptWordCount: &wordCount, Success: &transSuccess, Error: transError})
			rejReason := ""
			if !accepted {
				rejReason = rejectionReason(err, rawText, text, stopWords, silenceArtifacts)
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
					typeStart := time.Now()
					_ = recorder.Record(telemetry.Event{Event: telemetry.TypingStarted, Timestamp: typeStart, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index})
					typeErr := typing.TypeText(context.Background(), d, text+" ")
					typeEnd := time.Now()
					typeSuccess := typeErr == nil
					typeError := ""
					if typeErr != nil {
						typeError = typeErr.Error()
					}
					_ = recorder.Record(telemetry.Event{Event: telemetry.TypingComplete, Timestamp: typeEnd, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, Success: &typeSuccess, Error: typeError})
					chunkMeta.TypingStartedAt = typeStart
					chunkMeta.TypingEndedAt = typeEnd
					_, _ = chunkBuf.Update(chunkMeta)
				}

				if historyPath != "" {
					_, _ = history.AppendHistory(historyPath, text, history.DefaultHistoryLimit, time.Now())
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

	recCmd := exec.CommandContext(ctx, recCmdName, recArgs...)
	audioOut, err := recCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("audio pipe: %w", err)
	}
	recCmd.Stderr = io.Discard

	if err := recCmd.Start(); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}
	_ = recorder.Record(telemetry.Event{Event: telemetry.CaptureStarted, Timestamp: time.Now(), SessionID: sessionID})

	killDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if recCmd.Process != nil {
				_ = recCmd.Process.Kill()
			}
		case <-killDone:
		}
	}()

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
		if err != nil {
			break
		}
		if n < frameBytes {
			continue
		}

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
			}
		}
	}

	// Recording has stopped (context canceled, or the capture process exited
	// on its own). Reap the subprocess right now, immediately: previously this
	// was deferred to function return, which is additionally gated below on
	// draining the transcription queue -- a drain that can take up to
	// transcribeTimeout per queued utterance. That gap left the just-killed
	// recording process as an unreaped <defunct> zombie for the whole drain
	// (see issue 057). Signal onCaptureStopped only after the reap so a
	// caller waiting on it (eagerSessionManager.Stop) never observes a
	// still-defunct child process.
	close(killDone)
	if recCmd.Process != nil {
		_ = recCmd.Process.Kill()
		_ = recCmd.Wait()
	}
	_ = recorder.Record(telemetry.Event{Event: telemetry.CaptureStopped, Timestamp: time.Now(), SessionID: sessionID})
	signalCaptureStopped()

	// Flush remaining speech upon exit
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
		}
	}

	close(jobChan)
	transWg.Wait()

	if !isDaemon {
		fmt.Fprintln(d.Stdout, "\n\n── Dictation Complete ──────────────────────────────────────────")
		fmt.Fprintf(d.Stdout, "Total Utterances Transcribed: %d\n", utteranceCount)
		fmt.Fprintf(d.Stdout, "Full Consolidated Transcript:\n\n\x1b[1m%s\x1b[0m\n", fullTranscript.String())
		fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")
	}

	return nil
}

func voxtypeTranscribeArgs(modelName, wavPath, initialPrompt string) []string {
	args := []string{"--model", modelName, "--threads", "6"}
	if initialPrompt != "" {
		args = append(args, "--initial-prompt", initialPrompt)
	}
	return append(args, "-q", "transcribe", wavPath)
}

func shouldUseSpeechContext(modelName string, enabled bool) bool {
	return enabled && modelName == "small.en"
}

// acceptTranscript is the single gate before an eager result can affect either
// the focused application or local history.
func acceptTranscript(err error, text string, stopWords, silenceArtifacts []string) bool {
	return err == nil && text != "" && asr.IsSafeToType(text, stopWords) && !feedback.IsSilenceArtifact(text, silenceArtifacts)
}

// rejectionReason returns the explanation if a chunk was rejected, or empty string if accepted.
func rejectionReason(err error, rawText, cleanedText string, stopWords, silenceArtifacts []string) string {
	if err != nil {
		return fmt.Sprintf("transcribe_error: %v", err)
	}
	if strings.TrimSpace(rawText) == "" {
		return "empty"
	}
	if feedback.IsSilenceArtifact(cleanedText, silenceArtifacts) {
		return "silence_artifact"
	}
	if !asr.IsSafeToType(cleanedText, stopWords) {
		return "stop_word"
	}
	if cleanedText == "" {
		return "empty"
	}
	return ""
}

// probableSilence is deliberately conservative: fewer than 5% of 20ms
// frames crossing the configured VAD threshold is strong evidence that a
// chunk contains silence rather than sustained speech. The underlying RMS
// and voiced ratio are persisted alongside the decision for inspection.
func probableSilence(stats audio.AudioStats) bool {
	return stats.TotalFrames > 0 && stats.VoicedRatio < 0.05
}

// eagerSessionManager serializes start/stop/toggle requests against a single
// active recording session for the eager daemon.
//
// Issue 057 root cause: the daemon previously blocked a stop/start/toggle
// request on the *entire* session's lifetime (audio capture, plus draining
// any transcription job still queued or in flight, bounded only by
// transcribeTimeout, up to 30s). Since the agent control-plane
// (internal/agent/agent.go Agent.Record) holds a single mutex across the
// whole backend call, one stalled toggle stalled every other status/mode/
// record request behind it -- observed as a client-side socket read timeout
// (the CLI's 5s deadline firing while the daemon was still stuck) followed
// by a burst of queued requests all resolving back-to-back once the stale
// transcription finally finished, producing exactly the rapid-fire
// started/stopped/started/stopped log sequence from the ticket.
//
// The fix: decouple "capture has stopped" (bounded only by killing and
// reaping the recording subprocess -- fast) from "the session's full
// lifetime, including transcription drain, has finished" (potentially slow).
// Stop/Start/Toggle wait only for the former via captureStopped; a session's
// leftover transcription drain continues in the background and is only
// waited on by Wait, which the daemon calls once at shutdown.
type eagerSessionManager struct {
	ctx context.Context
	// run launches one recording session against sessCtx and must call
	// onCaptureStopped as soon as audio capture ends and its recording
	// subprocess is reaped, then may continue running (draining
	// transcription) until it returns.
	run      func(sessCtx context.Context, sessionID string, onCaptureStopped func())
	recorder *telemetry.Recorder
	now      func() time.Time

	mu              sync.Mutex
	activeCancel    context.CancelFunc
	activeStopped   chan struct{}
	activeSessionID string
	isRecording     bool
	sessWg          sync.WaitGroup
}

func newEagerSessionManager(ctx context.Context, recorder *telemetry.Recorder, run func(context.Context, string, func())) *eagerSessionManager {
	if recorder == nil {
		recorder = telemetry.NewRecorder("")
	}
	return &eagerSessionManager{ctx: ctx, recorder: recorder, now: time.Now, run: run}
}

// Stop cancels the active session, if any, and waits only until its audio
// capture has stopped (recording subprocess killed and reaped) -- not until
// its transcription drain (if any is still in flight) has finished.
func (m *eagerSessionManager) Stop() {
	m.mu.Lock()
	cancel := m.activeCancel
	stopped := m.activeStopped
	sessionID := m.activeSessionID
	deactivatedAt := time.Time{}
	if cancel != nil {
		deactivatedAt = m.now()
	}
	m.activeCancel = nil
	m.activeStopped = nil
	m.activeSessionID = ""
	m.isRecording = false
	m.mu.Unlock()

	if cancel != nil {
		_ = m.recorder.Record(telemetry.Event{Event: telemetry.MicDeactivated, Timestamp: deactivatedAt, SessionID: sessionID})
		cancel()
	}
	if stopped != nil {
		<-stopped
	}
}

// Start stops any active session (see Stop) and begins a new one.
func (m *eagerSessionManager) Start() {
	m.Stop()

	m.mu.Lock()
	activatedAt := m.now()
	sessionID := m.recorder.NewSessionID(activatedAt)
	sessCtx, cancel := context.WithCancel(m.ctx)
	stopped := make(chan struct{})
	m.activeCancel = cancel
	m.activeStopped = stopped
	m.activeSessionID = sessionID
	m.isRecording = true
	m.sessWg.Add(1)
	m.mu.Unlock()
	_ = m.recorder.Record(telemetry.Event{Event: telemetry.MicActivated, Timestamp: activatedAt, SessionID: sessionID})

	go func() {
		defer m.sessWg.Done()
		m.run(sessCtx, sessionID, func() {
			close(stopped)
		})
		m.mu.Lock()
		// Only clear state if nothing newer has replaced this session (a
		// concurrent Stop/Start already would have done so).
		if m.activeStopped == stopped {
			m.activeCancel = nil
			m.activeStopped = nil
			m.activeSessionID = ""
			m.isRecording = false
		}
		m.mu.Unlock()
	}()
}

// Toggle stops the active session if one is running, otherwise starts one.
func (m *eagerSessionManager) Toggle() string {
	m.mu.Lock()
	rec := m.isRecording
	m.mu.Unlock()
	if rec {
		m.Stop()
		return "Recording stopped"
	}
	m.Start()
	return "Recording started"
}

// Recording reports whether a session is currently active.
func (m *eagerSessionManager) Recording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRecording
}

// Wait blocks until every session's full lifetime (capture + transcription
// drain) has completed. Intended for daemon shutdown only.
func (m *eagerSessionManager) Wait() {
	m.sessWg.Wait()
}

func runEagerDaemon(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockDir := filepath.Join(runtimeDir, "voxi")
	_ = os.MkdirAll(sockDir, 0755)
	sockPath := filepath.Join(sockDir, "eager.sock")
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %s: %w", sockPath, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}()

	recCmdName := ""
	var recArgs []string
	if _, err := d.LookPath("pw-record"); err == nil {
		recCmdName = "pw-record"
		recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	} else if _, err := d.LookPath("arecord"); err == nil {
		recCmdName = "arecord"
		recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
	} else {
		return fmt.Errorf("audio capture tool (pw-record or arecord) missing")
	}

	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return fmt.Errorf("voxtype missing: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "voxi-eager-daemon-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	writeVoxtypeState("idle")
	defer writeVoxtypeState("inactive")

	recorder := telemetry.NewRecorder(telemetry.Path(d.Getenv("XDG_DATA_HOME"), d.Getenv("HOME")))
	sessions := newEagerSessionManager(ctx, recorder, func(sessCtx context.Context, sessionID string, onCaptureStopped func()) {
		// Each session gets its own subdirectory under the daemon's base
		// tmpDir rather than sharing one across sessions: since Stop/Start
		// no longer wait for the previous session's transcription drain to
		// finish (issue 057 fix), a new session's capture can now begin
		// while the previous one is still draining in the background, and
		// both name their per-utterance WAV files starting at utt_001 --
		// sharing a directory would let a fresh session's audio collide
		// with (or be clobbered by) a stale drain still reading/writing
		// under the same names.
		sessTmpDir, err := os.MkdirTemp(tmpDir, "sess-*")
		if err != nil {
			sessTmpDir = tmpDir
		} else {
			defer os.RemoveAll(sessTmpDir)
		}
		// Write "idle" the moment capture actually stops, not only once this
		// whole session (including any still-draining transcription) fully
		// returns -- otherwise external state readers (voxi monitor, the
		// GNOME extension) would keep reporting "recording" for up to
		// transcribeTimeout after the user told the daemon to stop.
		_ = runEagerCaptureSession(sessCtx, d, opts, sessTmpDir, voxtypePath, recCmdName, recArgs, true, sessionID, recorder, func() {
			writeVoxtypeState("idle")
			onCaptureStopped()
		})
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for sig := range sigChan {
			if sig == syscall.SIGUSR1 {
				sessions.Toggle()
			} else if sig == syscall.SIGUSR2 {
				sessions.Stop()
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		sessions.Stop()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			scanner := bufio.NewScanner(c)
			if scanner.Scan() {
				cmd := strings.TrimSpace(scanner.Text())
				switch cmd {
				case "toggle":
					msg := sessions.Toggle()
					_, _ = fmt.Fprintln(c, msg)
				case "start":
					sessions.Start()
					_, _ = fmt.Fprintln(c, "Recording started")
				case "stop":
					sessions.Stop()
					_, _ = fmt.Fprintln(c, "Recording stopped")
				case "status":
					if sessions.Recording() {
						_, _ = fmt.Fprintln(c, "recording")
					} else {
						_, _ = fmt.Fprintln(c, "idle")
					}
				default:
					_, _ = fmt.Fprintln(c, "unknown command")
				}
			}
		}(conn)
	}

	// Wait for every session's full lifetime (capture + transcription drain)
	// to finish before returning, so the deferred os.RemoveAll(tmpDir) above
	// never yanks the directory out from under a still-draining background
	// transcription, and so a final queued utterance still gets typed before
	// the daemon process exits.
	sessions.Wait()

	return nil
}

// ControlEagerDaemon sends recording control actions to the eager daemon socket.
func ControlEagerDaemon(ctx context.Context, d deps.Dependencies, action string) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		// Try legacy path
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("eager daemon not reachable at %s (is voxi-eager.service active?): %w", sockPath, err)
	}
	defer conn.Close()
	// Defense-in-depth bound on the eager-daemon socket round trip, mirroring
	// the transcribeTimeout precedent: with the issue 057 fix, start/stop/
	// toggle now return as soon as capture stops rather than blocking on a
	// stale transcription drain, so this should virtually never fire. It
	// exists so any future daemon-side stall degrades to a clear error
	// instead of silently wedging the caller (internal/agent/agent.go's
	// Agent.Record holds a single mutex across the whole backend call, so an
	// indefinite block here previously stalled every other agent request).
	_ = conn.SetDeadline(time.Now().Add(eagerSocketTimeout))

	_, err = fmt.Fprintf(conn, "%s\n", action)
	if err != nil {
		return fmt.Errorf("send %s to eager daemon: %w", action, err)
	}

	res, _ := bufio.NewReader(conn).ReadString('\n')
	if strings.TrimSpace(res) != "" {
		fmt.Fprintln(d.Stdout, strings.TrimSpace(res))
	}
	return nil
}

// GetEagerRecordingStatus queries current recording/idle status from the eager daemon.
func GetEagerRecordingStatus(ctx context.Context, d deps.Dependencies) (string, error) {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return "inactive", nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(eagerSocketTimeout))

	_, _ = fmt.Fprintln(conn, "status")
	res, _ := bufio.NewReader(conn).ReadString('\n')
	status := strings.TrimSpace(res)
	if status == "" {
		return "idle", nil
	}
	return status, nil
}
