package devsample

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/audio"
)

func TestReadPCMFromWAV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.wav")
	pcm := bytes.Repeat([]byte{0, 1}, 8000) // 8000 samples @ 16-bit
	if err := audio.WriteWAVAudio(path, pcm, 16000); err != nil {
		t.Fatalf("write fixture wav: %v", err)
	}

	got, rate, err := ReadPCM(path)
	if err != nil {
		t.Fatalf("ReadPCM: %v", err)
	}
	if rate != 16000 {
		t.Fatalf("sample rate = %d, want 16000", rate)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("PCM bytes = %d bytes, want %d bytes matching fixture", len(got), len(pcm))
	}
}

func TestReadPCMFromFLAC(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := t.TempDir()
	wavPath := filepath.Join(dir, "sample.wav")
	pcm := bytes.Repeat([]byte{0, 1}, 8000)
	if err := audio.WriteWAVAudio(wavPath, pcm, 16000); err != nil {
		t.Fatalf("write fixture wav: %v", err)
	}
	flacPath := filepath.Join(dir, "sample.flac")
	cmd := exec.CommandContext(context.Background(), "ffmpeg", "-y", "-i", wavPath, "-f", "flac", flacPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg encode fixture flac: %v\n%s", err, out)
	}
	if _, err := os.Stat(flacPath); err != nil {
		t.Fatalf("fixture flac missing: %v", err)
	}

	got, rate, err := ReadPCM(flacPath)
	if err != nil {
		t.Fatalf("ReadPCM(flac): %v", err)
	}
	if rate != 16000 {
		t.Fatalf("sample rate = %d, want 16000", rate)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty decoded PCM from flac")
	}
}
