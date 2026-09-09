package feedback

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/internal/listing"
)

// SampleTranscribeFunc transcribes one WAV file through an ASR engine, for
// `sample list --process` (issue 098). It is injected from cmd/voxi's
// wiring (as eager.TranscribeCohereWAV) rather than imported directly:
// internal/eager already imports internal/feedback (to consult stop-word
// overrides mid-dictation), so a direct import the other way would create
// an import cycle. A nil SampleTranscribeFunc makes `--process` fail with a
// clear error instead of panicking (e.g. in tests that don't wire one up).
type SampleTranscribeFunc func(ctx context.Context, d deps.Dependencies, wavPath string) (string, error)

// Sample corpus source labels shown in the SOURCE column of `sample list
// --all`.
const (
	sourcePrivate = "private"
	sourcePublic  = "public"
)

// listedSample pairs a devsample.Sample with which corpus it came from and
// its resolved absolute audio path, so `sample list --all` can clearly
// distinguish private vs. public/promoted rows (issue 098 acceptance
// criteria).
type listedSample struct {
	devsample.Sample
	Source string
	Path   string
}

// loadListedSamples reads the private corpus, and additionally the
// public/promoted corpus when all is true, merging and sorting them
// (private first, then public, each alphabetically by name).
func loadListedSamples(home string, all bool) ([]listedSample, error) {
	private, err := devsample.LoadManifest(home)
	if err != nil {
		return nil, fmt.Errorf("load private samples: %w", err)
	}
	privateDir := devsample.SamplesDir(home)
	out := make([]listedSample, 0, len(private))
	for _, s := range private {
		out = append(out, listedSample{Sample: s, Source: sourcePrivate, Path: filepath.Join(privateDir, s.WAVFile)})
	}
	if all {
		publicDir := devsample.PublicSamplesDir("")
		public, err := devsample.LoadManifestIn(publicDir)
		if err != nil {
			return nil, fmt.Errorf("load public samples: %w", err)
		}
		for _, s := range public {
			out = append(out, listedSample{Sample: s, Source: sourcePublic, Path: filepath.Join(publicDir, s.WAVFile)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// sampleAcousticStats holds acoustic stats computed on the fly from a
// sample's actual audio file at list time -- issue 098 deliberately
// resolved against storing these on devsample.Sample.
type sampleAcousticStats struct {
	DurationSecs float64
	Sparkline    string
	audio.AudioStats
	Err error
}

// sampleStatsThresholdRMS mirrors internal/audio.DefaultSegmenterOptions's
// ThresholdRMS, the same acoustic-voicing threshold live dictation uses, so
// a sample's on-the-fly VoicedRatio/MeanRMS reads consistently with chunk
// stats shown by `chunks list`.
var sampleStatsThresholdRMS = audio.DefaultSegmenterOptions().ThresholdRMS

func computeSampleStats(path string) sampleAcousticStats {
	pcm, rate, err := devsample.ReadPCM(path)
	if err != nil {
		return sampleAcousticStats{Err: err}
	}
	stats := audio.AnalyzePCM(pcm, sampleStatsThresholdRMS)
	duration := 0.0
	if rate > 0 {
		duration = float64(len(pcm)/2) / float64(rate)
	}
	return sampleAcousticStats{
		DurationSecs: duration,
		Sparkline:    audio.RenderVolumeSparkline(pcm, 10),
		AudioStats:   stats,
	}
}

// sampleListOptions controls `voxi feedback sample list` rendering.
type sampleListOptions struct {
	All     bool
	Full    bool
	Process bool
	Color   string
}

func runSampleList(ctx context.Context, out io.Writer, d deps.Dependencies, home string, opts sampleListOptions, transcribe SampleTranscribeFunc) error {
	if !listing.IsValidColorMode(opts.Color) {
		return fmt.Errorf("invalid --color value %q: must be one of %s", opts.Color, strings.Join(listing.ValidColorModes, ", "))
	}
	if opts.Process && !opts.Full {
		return fmt.Errorf("--process requires --full (a fresh transcript column doesn't fit --short's compact table)")
	}
	if opts.Process && transcribe == nil {
		return fmt.Errorf("--process is not available: no transcribe engine wired up")
	}

	samples, err := loadListedSamples(home, opts.All)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		fmt.Fprintln(out, "No dev samples recorded yet. Record one with: voxi feedback sample record <name>")
		return nil
	}

	var noColorEnv string
	if d.Getenv != nil {
		noColorEnv = d.Getenv("NO_COLOR")
	}
	useColor := listing.ShouldUseColor(opts.Color, noColorEnv, listing.StdoutIsTerminal(out))

	if !opts.Full {
		writeShortSampleTable(out, samples, opts.All)
		return nil
	}
	return writeFullSampleTable(ctx, out, d, samples, opts.All, opts.Process, useColor, transcribe)
}

func formatSampleTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Local().Format("2006-01-02 15:04:05")
}

func writeShortSampleTable(out io.Writer, samples []listedSample, showSource bool) {
	var cols []listing.Column
	if showSource {
		cols = append(cols, listing.Column{Header: "SOURCE", Width: 8})
	}
	cols = append(cols,
		listing.Column{Header: "NAME", Width: 24},
		listing.Column{Header: "TIMESTAMP", Width: 19},
		listing.Column{Header: "TRANSCRIPT", Width: 0},
	)

	rows := make([][]string, 0, len(samples))
	for _, s := range samples {
		var row []string
		if showSource {
			row = append(row, s.Source)
		}
		row = append(row, s.Name, formatSampleTimestamp(s.Timestamp), s.Preview(60))
		rows = append(rows, row)
	}
	listing.WriteTable(out, cols, rows)
}

func writeFullSampleTable(ctx context.Context, out io.Writer, d deps.Dependencies, samples []listedSample, showSource, process, useColor bool, transcribe SampleTranscribeFunc) error {
	var cols []listing.Column
	if showSource {
		cols = append(cols, listing.Column{Header: "SOURCE", Width: 8})
	}
	cols = append(cols,
		listing.Column{Header: "NAME", Width: 24},
		listing.Column{Header: "TIMESTAMP", Width: 19},
		listing.Column{Header: "DURATION", Width: 8},
		listing.Column{Header: "RMS", Width: 5, Right: true},
		listing.Column{Header: "LEVEL", Width: 12},
	)
	if process {
		cols = append(cols, listing.Column{Header: "PROCESSED", Width: 0})
	}
	cols = append(cols, listing.Column{Header: "TRANSCRIPT", Width: 0})

	rows := make([][]string, 0, len(samples))
	for _, s := range samples {
		stats := computeSampleStats(s.Path)

		var row []string
		if showSource {
			row = append(row, s.Source)
		}
		durText, rmsText, levelText := "?", "?", listing.FormatSparklineCell("", 12, false)
		if stats.Err == nil {
			durText = fmt.Sprintf("%.1fs", stats.DurationSecs)
			rmsText = strconv.Itoa(stats.MeanRMS)
			levelText = listing.FormatSparklineCell(stats.Sparkline, 12, useColor)
		}
		row = append(row, s.Name, formatSampleTimestamp(s.Timestamp), durText, rmsText, levelText)

		if process {
			row = append(row, processSampleForCheck(ctx, d, s, stats, transcribe))
		}
		row = append(row, s.Text)
		rows = append(rows, row)
	}
	listing.WriteTable(out, cols, rows)
	return nil
}

// processSampleForCheck runs one sample's audio through transcribe for
// `sample list --process` (issue 098): a lightweight, on-demand accuracy
// spot check of stored ground truth vs. a fresh transcript, distinct from
// scripts/speech_context_bench's batch benchmarking. Deliberately scoped
// down (see issue 098 §3/§4): a single engine, no WER/diff display, and the
// returned text is only ANSI-stripped/trimmed rather than run through
// asr.CleanWhisperTranscript's full hallucination/stop-word pipeline.
func processSampleForCheck(ctx context.Context, d deps.Dependencies, s listedSample, stats sampleAcousticStats, transcribe SampleTranscribeFunc) string {
	if stats.Err != nil {
		return "err: " + stats.Err.Error()
	}
	wavPath := s.Path
	if strings.HasSuffix(strings.ToLower(s.Path), ".flac") {
		pcm, rate, err := devsample.ReadPCM(s.Path)
		if err != nil {
			return "err: decode audio: " + err.Error()
		}
		tmp, err := os.CreateTemp("", "voxi-sample-check-*.wav")
		if err != nil {
			return "err: " + err.Error()
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
		if err := audio.WriteWAVAudio(tmpPath, pcm, rate); err != nil {
			return "err: write temp wav: " + err.Error()
		}
		wavPath = tmpPath
	}
	raw, err := transcribe(ctx, d, wavPath)
	if err != nil {
		return "err: " + err.Error()
	}
	return strings.TrimSpace(asr.StripANSI(raw))
}
