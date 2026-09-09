// Command clack_features is an offline research tool for issue-shaped work
// on distinguishing keyboard-clack noise chunks from real short speech
// utterances. It reads the private dev-sample corpus (voxi feedback sample
// record/save-chunk), computes two cheap acoustic features per WAV —
// zero-crossing rate (ZCR) and energy-weighted spectral centroid — and
// prints them side by side so a threshold's separating power can be judged
// by eye before any of this is wired into the acoustic gate
// (internal/audio/audio.go's CheckCandidateAcoustics).
//
// Usage:
//
//	go run ./scripts/clack_features [-private DIR] [-public DIR]
//
// -private defaults to the private dev-sample directory
// (~/.config/voxi/samples); -public defaults to the git-tracked public
// corpus (testdata/noise-samples, relative to the repo root -- run this from
// the repo root). Either may be empty/missing (skipped silently) so this
// works whether the corpus is entirely private, entirely promoted, or split
// across both. Each is expected to hold a corpus.tsv manifest (see
// internal/devsample) whose first field is the sample name and second field
// the audio filename (.wav or .flac -- .flac is decoded via a shelled-out
// `ffmpeg`, since promoted public samples are FLAC-encoded for git-lfs).
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// frameSize is the analysis window in samples: 25ms at 16kHz, rounded up to
// the next power of two (512) for the FFT.
const (
	frameSamples = 400
	fftSize      = 512
	hopSamples   = 200 // 50% overlap
	silenceRMS   = 150 // frames quieter than this are skipped (mirrors the low_energy_transient acoustic gate)
)

type sample struct {
	name string
	wav  string
	dir  string
}

func main() {
	home, _ := os.UserHomeDir()
	defaultPrivate := filepath.Join(home, ".config", "voxi", "samples")

	private := flag.String("private", defaultPrivate, "private samples directory holding corpus.tsv and its WAV files")
	public := flag.String("public", filepath.Join("testdata", "noise-samples"), "public (git-tracked) samples directory holding corpus.tsv and its FLAC files")
	flag.Parse()

	var samples []sample
	for _, dir := range []string{*private, *public} {
		s, err := loadManifest(filepath.Join(dir, "corpus.tsv"))
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "load manifest %s: %v\n", dir, err)
			continue
		}
		for i := range s {
			s[i].dir = dir
		}
		samples = append(samples, s...)
	}
	if len(samples) == 0 {
		fmt.Fprintf(os.Stderr, "no samples found in %s or %s\n", *private, *public)
		os.Exit(1)
	}

	type row struct {
		name       string
		zcr        float64
		centroid   float64
		framesUsed int
	}
	var rows []row
	for _, s := range samples {
		pcm, rate, err := readAudioPCM(filepath.Join(s.dir, s.wav))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", s.name, err)
			continue
		}
		zcr, centroid, framesUsed := analyze(pcm, rate)
		rows = append(rows, row{name: s.name, zcr: zcr, centroid: centroid, framesUsed: framesUsed})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].zcr < rows[j].zcr })

	fmt.Printf("%-26s  %8s  %10s  %6s\n", "NAME", "ZCR", "CENTROID", "FRAMES")
	for _, r := range rows {
		fmt.Printf("%-26s  %8.4f  %8.1fHz  %6d\n", r.name, r.zcr, r.centroid, r.framesUsed)
	}
}

// loadManifest reads name/wav pairs from a corpus.tsv-compatible manifest,
// skipping blank and comment lines (mirrors internal/devsample.ParseManifest
// without pulling in that private package for a throwaway analysis tool).
func loadManifest(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples []sample
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 2 {
			continue
		}
		samples = append(samples, sample{name: fields[0], wav: fields[1]})
	}
	return samples, scanner.Err()
}

// readAudioPCM reads mono 16-bit PCM samples and the sample rate from either
// a WAV file directly, or a FLAC file decoded through a shelled-out ffmpeg
// (avoids adding a FLAC-decoding Go dependency for a throwaway analysis
// tool; promoted public samples are FLAC-encoded for git-lfs, see
// internal/devsample.Promote).
func readAudioPCM(path string) ([]int16, int, error) {
	if !strings.HasSuffix(strings.ToLower(path), ".flac") {
		return readWAVPCM(path)
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", path, "-f", "wav", "-acodec", "pcm_s16le", "-")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg decode %s: %w\n%s", path, err, stderr.String())
	}
	return parseWAVBytes(out.Bytes())
}

// readWAVPCM extracts mono 16-bit PCM samples and the sample rate from a
// WAV file's fmt/data chunks, without assuming a fixed header size.
func readWAVPCM(path string) ([]int16, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return parseWAVBytes(data)
}

// parseWAVBytes is readWAVPCM's format parser, factored out so
// readAudioPCM can feed it ffmpeg's decoded-to-WAV stdout for FLAC input
// too.
func parseWAVBytes(data []byte) ([]int16, int, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a RIFF/WAVE file")
	}

	var sampleRate int
	var pcmBytes []byte
	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		rawChunkSize := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		chunkSize := int(rawChunkSize)
		body := pos + 8
		// A WAV muxed to a non-seekable pipe (e.g. ffmpeg decoding FLAC to
		// stdout) can't back-patch the data chunk's real size once
		// streaming is done, so it writes 0xFFFFFFFF ("unknown") instead:
		// treat that as "rest of file" rather than bailing out.
		if chunkID == "data" && rawChunkSize == 0xFFFFFFFF {
			chunkSize = len(data) - body
		}
		if body+chunkSize > len(data) {
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, fmt.Errorf("truncated fmt chunk")
			}
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
		case "data":
			pcmBytes = data[body : body+chunkSize]
		}
		pos = body + chunkSize
		if chunkSize%2 == 1 {
			pos++
		}
	}
	if sampleRate == 0 || pcmBytes == nil {
		return nil, 0, fmt.Errorf("missing fmt or data chunk")
	}

	samples := make([]int16, len(pcmBytes)/2)
	r := bytes.NewReader(pcmBytes)
	if err := binary.Read(r, binary.LittleEndian, &samples); err != nil {
		return nil, 0, fmt.Errorf("decode PCM: %w", err)
	}
	return samples, sampleRate, nil
}

// analyze frames pcm into overlapping windows, skips near-silent frames the
// same way the acoustic gate already does, and returns the mean
// zero-crossing rate and the energy-weighted mean spectral centroid across
// the remaining frames.
func analyze(pcm []int16, sampleRate int) (meanZCR, meanCentroid float64, framesUsed int) {
	var zcrSum, centroidWeightedSum, energySum float64

	for start := 0; start+frameSamples <= len(pcm); start += hopSamples {
		frame := pcm[start : start+frameSamples]

		rms := frameRMS(frame)
		if rms < silenceRMS {
			continue
		}

		zcrSum += zeroCrossingRate(frame)
		centroidWeightedSum += spectralCentroid(frame, sampleRate) * rms
		energySum += rms
		framesUsed++
	}

	if framesUsed == 0 {
		return 0, 0, 0
	}
	meanZCR = zcrSum / float64(framesUsed)
	meanCentroid = centroidWeightedSum / energySum
	return meanZCR, meanCentroid, framesUsed
}

func frameRMS(frame []int16) float64 {
	var sumSq float64
	for _, s := range frame {
		sumSq += float64(s) * float64(s)
	}
	return math.Sqrt(sumSq / float64(len(frame)))
}

// zeroCrossingRate returns the fraction of adjacent-sample sign changes in
// the frame: near 0 for smooth voiced speech, high for broadband
// percussive noise like a keyboard clack.
func zeroCrossingRate(frame []int16) float64 {
	crossings := 0
	for i := 1; i < len(frame); i++ {
		if (frame[i-1] >= 0) != (frame[i] >= 0) {
			crossings++
		}
	}
	return float64(crossings) / float64(len(frame)-1)
}

// spectralCentroid returns the magnitude-weighted mean frequency (Hz) of a
// Hamming-windowed, zero-padded FFT of the frame: low for speech's
// energy concentrated in low harmonics/formants, high for a clack's
// broadband/high-frequency energy.
func spectralCentroid(frame []int16, sampleRate int) float64 {
	windowed := make([]complex128, fftSize)
	for i, s := range frame {
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(len(frame)-1))
		windowed[i] = complex(float64(s)*w, 0)
	}
	fft(windowed)

	var weightedSum, magSum float64
	bins := fftSize / 2
	for k := 1; k < bins; k++ { // skip DC bin
		mag := cmplxAbs(windowed[k])
		freq := float64(k) * float64(sampleRate) / float64(fftSize)
		weightedSum += freq * mag
		magSum += mag
	}
	if magSum == 0 {
		return 0
	}
	return weightedSum / magSum
}

func cmplxAbs(c complex128) float64 {
	return math.Hypot(real(c), imag(c))
}

// fft is an in-place iterative radix-2 Cooley-Tukey FFT. len(x) must be a
// power of two (guaranteed by fftSize above).
func fft(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}

	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		angleStep := -2 * math.Pi / float64(size)
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				angle := angleStep * float64(k)
				twiddle := complex(math.Cos(angle), math.Sin(angle))
				even := x[start+k]
				odd := x[start+k+half] * twiddle
				x[start+k] = even + odd
				x[start+k+half] = even - odd
			}
		}
	}
}
