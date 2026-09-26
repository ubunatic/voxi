// Package clone prepares local speech samples for Piper voice training.
package clone

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"ubunatic.com/voxi/internal/devsample"
)

const (
	SampleRate = 22050
	channels   = 1
	bits       = 16
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Options configures dataset preparation. ConvertAudio can replace ffmpeg for
// tests or integrations; nil resolves and runs ffmpeg from PATH.
type Options struct {
	SamplesDir   string
	OutputDir    string
	FFmpegPath   string
	ConvertAudio func(ctx context.Context, input, output string) error
}

// Result describes the dataset produced by Prepare.
type Result struct {
	OutputDir string
	Samples   int
}

// Prepare converts corpus.tsv samples into the LJSpeech directory layout.
// Output is assembled in a sibling temporary directory and swapped into place
// only after every sample has been converted and validated.
func Prepare(ctx context.Context, opts Options) (Result, error) {
	if strings.TrimSpace(opts.SamplesDir) == "" || strings.TrimSpace(opts.OutputDir) == "" {
		return Result{}, errors.New("samples and output directories are required")
	}
	samplesDir, err := filepath.Abs(opts.SamplesDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve samples directory: %w", err)
	}
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output directory: %w", err)
	}
	if outputDir == samplesDir || within(outputDir, samplesDir) || within(samplesDir, outputDir) {
		return Result{}, errors.New("output directory must be separate from the samples directory")
	}
	manifest, err := os.ReadFile(devsample.ManifestPathIn(samplesDir))
	if err != nil {
		return Result{}, fmt.Errorf("read corpus manifest: %w", err)
	}
	samples, err := devsample.ParseManifest(manifest)
	if err != nil {
		return Result{}, fmt.Errorf("parse corpus manifest: %w", err)
	}
	if len(samples) == 0 {
		return Result{}, errors.New("corpus contains no samples")
	}
	if err := validateSamples(samples); err != nil {
		return Result{}, err
	}
	convert := opts.ConvertAudio
	if convert == nil {
		ffmpeg := opts.FFmpegPath
		if ffmpeg == "" {
			ffmpeg, err = exec.LookPath("ffmpeg")
			if err != nil {
				return Result{}, fmt.Errorf("prepare voice dataset: ffmpeg is required: %w", err)
			}
		}
		convert = func(ctx context.Context, input, output string) error {
			cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-ar", fmt.Sprint(SampleRate), "-ac", fmt.Sprint(channels), "-c:a", "pcm_s16le", output)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return Result{}, fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".voxi-voice-prepare-*")
	if err != nil {
		return Result{}, fmt.Errorf("create dataset staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.Mkdir(filepath.Join(stage, "wavs"), 0700); err != nil {
		return Result{}, fmt.Errorf("create staged WAV directory: %w", err)
	}
	var metadata strings.Builder
	for _, sample := range samples {
		input := filepath.Join(samplesDir, filepath.FromSlash(sample.WAVFile))
		resolvedInput, err := filepath.EvalSymlinks(input)
		if err != nil {
			return Result{}, fmt.Errorf("sample %q WAV %q: %w", sample.Name, sample.WAVFile, err)
		}
		resolvedSamples, err := filepath.EvalSymlinks(samplesDir)
		if err != nil {
			return Result{}, fmt.Errorf("resolve samples directory: %w", err)
		}
		if !within(resolvedSamples, resolvedInput) {
			return Result{}, fmt.Errorf("sample %q WAV path resolves outside the samples directory", sample.Name)
		}
		info, err := os.Stat(resolvedInput)
		if err != nil || !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("sample %q WAV is not a regular file", sample.Name)
		}
		output := filepath.Join(stage, "wavs", sample.Name+".wav")
		if err := convert(ctx, resolvedInput, output); err != nil {
			return Result{}, fmt.Errorf("convert sample %q: %w", sample.Name, err)
		}
		if err := validateWAV(output); err != nil {
			return Result{}, fmt.Errorf("converted sample %q: %w", sample.Name, err)
		}
		fmt.Fprintf(&metadata, "%s|%s|%s\n", sample.Name, sample.Text, sample.Text)
	}
	if err := os.WriteFile(filepath.Join(stage, "metadata.csv"), []byte(metadata.String()), 0600); err != nil {
		return Result{}, fmt.Errorf("write LJSpeech metadata: %w", err)
	}
	if err := replaceDirectory(stage, outputDir); err != nil {
		return Result{}, err
	}
	return Result{OutputDir: outputDir, Samples: len(samples)}, nil
}

func validateSamples(samples []devsample.Sample) error {
	seen := make(map[string]bool, len(samples))
	for _, sample := range samples {
		if !safeID.MatchString(sample.Name) {
			return fmt.Errorf("invalid sample ID %q: use 1-128 ASCII letters, digits, underscores, or hyphens; start with a letter or digit", sample.Name)
		}
		if seen[sample.Name] {
			return fmt.Errorf("duplicate sample ID %q", sample.Name)
		}
		seen[sample.Name] = true
		text := strings.TrimSpace(sample.Text)
		if text == "" || text != sample.Text || strings.ContainsRune(text, '|') {
			return fmt.Errorf("sample %q has an empty, padded, or pipe-delimited transcript", sample.Name)
		}
		for _, r := range text {
			if unicode.IsControl(r) {
				return fmt.Errorf("sample %q transcript contains a control character", sample.Name)
			}
		}
		name := filepath.FromSlash(sample.WAVFile)
		if sample.WAVFile == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || name == "." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("sample %q has unsafe WAV path %q", sample.Name, sample.WAVFile)
		}
	}
	return nil
}

func validateWAV(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open converted WAV: %w", err)
	}
	defer f.Close()
	header := make([]byte, 12)
	if _, err := io.ReadFull(f, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return errors.New("not a valid RIFF/WAVE file")
	}
	var rate uint32
	var channelCount, bitDepth, blockAlign, format uint16
	var dataSize uint32
	var formatSeen bool
	for {
		chunkHeader := make([]byte, 8)
		n, err := io.ReadFull(f, chunkHeader)
		if errors.Is(err, io.EOF) && n == 0 {
			break
		}
		if err != nil {
			return errors.New("truncated WAV chunk header")
		}
		size := binary.LittleEndian.Uint32(chunkHeader[4:])
		switch string(chunkHeader[:4]) {
		case "fmt ":
			if size < 16 || size > 1<<20 {
				return errors.New("invalid WAV format chunk")
			}
			chunk := make([]byte, size)
			if _, err := io.ReadFull(f, chunk); err != nil {
				return errors.New("truncated WAV format chunk")
			}
			format = binary.LittleEndian.Uint16(chunk[0:2])
			channelCount = binary.LittleEndian.Uint16(chunk[2:4])
			rate = binary.LittleEndian.Uint32(chunk[4:8])
			blockAlign = binary.LittleEndian.Uint16(chunk[12:14])
			bitDepth = binary.LittleEndian.Uint16(chunk[14:16])
			formatSeen = true
			if size%2 == 1 {
				if _, err := io.CopyN(io.Discard, f, 1); err != nil {
					return errors.New("truncated WAV format padding")
				}
			}
		case "data":
			dataSize = size
			if _, err := io.CopyN(io.Discard, f, int64(size)); err != nil {
				return errors.New("truncated WAV audio data")
			}
			if size%2 == 1 {
				if _, err := io.CopyN(io.Discard, f, 1); err != nil {
					return errors.New("truncated WAV audio padding")
				}
			}
		default:
			if _, err := io.CopyN(io.Discard, f, int64(size)+int64(size%2)); err != nil {
				return errors.New("truncated WAV chunk")
			}
		}
	}
	if !formatSeen || format != 1 || channelCount != channels || rate != SampleRate || bitDepth != bits || blockAlign != channels*bits/8 || dataSize == 0 {
		return fmt.Errorf("expected PCM %d Hz, %d channel, %d-bit WAV with audio data", SampleRate, channels, bits)
	}
	return nil
}

func replaceDirectory(stage, output string) error {
	backup := output + ".voxi-backup"
	if _, err := os.Lstat(backup); err == nil {
		return fmt.Errorf("refusing to replace dataset while backup path exists: %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect dataset backup path: %w", err)
	}
	if _, err := os.Lstat(output); err == nil {
		if err := os.Rename(output, backup); err != nil {
			return fmt.Errorf("move previous dataset aside: %w", err)
		}
		if err := os.Rename(stage, output); err != nil {
			_ = os.Rename(backup, output)
			return fmt.Errorf("install prepared dataset: %w", err)
		}
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous dataset backup %s: %w", backup, err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output directory: %w", err)
	}
	if err := os.Rename(stage, output); err != nil {
		return fmt.Errorf("install prepared dataset: %w", err)
	}
	return nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
