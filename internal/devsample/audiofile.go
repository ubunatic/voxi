package devsample

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ReadPCM reads mono 16-bit PCM audio and its sample rate from a sample's
// on-disk audio file: a WAV file directly (the private corpus), or a FLAC
// file decoded through a shelled-out ffmpeg (the public/promoted corpus --
// see Promote, which FLAC-encodes for git-lfs). Used to compute on-the-fly
// acoustic stats (duration, RMS, sparkline) at `sample list` time (issue
// 098) -- Sample intentionally carries no stored acoustic fields.
//
// This duplicates scripts/clack_features/main.go's
// readAudioPCM/parseWAVBytes logic (including its fix for ffmpeg's
// streamed-WAV 0xFFFFFFFF unknown-size data chunk) rather than importing
// it: clack_features is a throwaway `package main` research tool, not an
// importable package.
func ReadPCM(path string) (pcm []byte, sampleRate int, err error) {
	if !strings.HasSuffix(strings.ToLower(path), ".flac") {
		return readWAVFile(path)
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", path, "-f", "wav", "-acodec", "pcm_s16le", "-")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return nil, 0, fmt.Errorf("ffmpeg decode %s: %w\n%s", path, runErr, stderr.String())
	}
	return parseWAVBytes(out.Bytes())
}

func readWAVFile(path string) ([]byte, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return parseWAVBytes(data)
}

// parseWAVBytes extracts mono 16-bit PCM samples (as raw little-endian
// bytes, matching internal/audio.AnalyzePCM/RenderVolumeSparkline's
// expected input) and the sample rate from a WAV file's fmt/data chunks,
// without assuming a fixed header size.
func parseWAVBytes(data []byte) ([]byte, int, error) {
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
	return pcmBytes, sampleRate, nil
}
