package clone

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestPrepareWritesLJSpeechDatasetInStableOrder(t *testing.T) {
	samples := t.TempDir()
	writeCorpus(t, samples, "z-last\tz.wav\tLast sample.\tterm\na-first\ta.wav\tFirst sample.\t\n")
	writeFile(t, filepath.Join(samples, "z.wav"), []byte("source"))
	writeFile(t, filepath.Join(samples, "a.wav"), []byte("source"))
	output := filepath.Join(t.TempDir(), "dataset")
	result, err := Prepare(context.Background(), Options{SamplesDir: samples, OutputDir: output, ConvertAudio: fakeConverter})
	if err != nil {
		t.Fatal(err)
	}
	if result.Samples != 2 || result.OutputDir != output {
		t.Fatalf("result = %#v", result)
	}
	metadata, err := os.ReadFile(filepath.Join(output, "metadata.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(metadata), "a-first|First sample.|First sample.\nz-last|Last sample.|Last sample.\n"; got != want {
		t.Fatalf("metadata = %q, want %q", got, want)
	}
	for _, id := range []string{"a-first", "z-last"} {
		if err := validateWAV(filepath.Join(output, "wavs", id+".wav")); err != nil {
			t.Fatalf("validate %s.wav: %v", id, err)
		}
	}
}

func TestPrepareRejectsInvalidCorpusBeforeReplacingExistingOutput(t *testing.T) {
	cases := []struct {
		name, corpus, want string
	}{
		{"invalid id", "../escape\ta.wav\tText.\t\n", "invalid sample ID"},
		{"duplicate id", "same\ta.wav\tOne.\t\nsame\tb.wav\tTwo.\t\n", "duplicate sample ID"},
		{"empty transcript", "sample\ta.wav\t   \t\n", "transcript"},
		{"pipe transcript", "sample\ta.wav\tA|B\t\n", "transcript"},
		{"control transcript", "sample\ta.wav\tA\x01B\t\n", "control character"},
		{"unsafe path", "sample\t../escape.wav\tText.\t\n", "unsafe WAV path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			samples := t.TempDir()
			writeCorpus(t, samples, tc.corpus)
			output := filepath.Join(t.TempDir(), "dataset")
			if err := os.Mkdir(output, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(output, "keep")
			writeFile(t, marker, []byte("prior"))
			_, err := Prepare(context.Background(), Options{SamplesDir: samples, OutputDir: output, ConvertAudio: fakeConverter})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Prepare error = %v, want %q", err, tc.want)
			}
			if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "prior" {
				t.Fatalf("prior output changed: %q, %v", got, readErr)
			}
		})
	}
}

func TestPrepareRejectsBadConvertedAudioAndKeepsPriorOutput(t *testing.T) {
	samples := t.TempDir()
	writeCorpus(t, samples, "sample\ta.wav\tText.\t\n")
	writeFile(t, filepath.Join(samples, "a.wav"), []byte("source"))
	output := filepath.Join(t.TempDir(), "dataset")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "keep")
	writeFile(t, marker, []byte("prior"))
	_, err := Prepare(context.Background(), Options{SamplesDir: samples, OutputDir: output, ConvertAudio: func(_ context.Context, _, destination string) error {
		return os.WriteFile(destination, []byte("not a wav"), 0600)
	}})
	if err == nil || !strings.Contains(err.Error(), "not a valid RIFF/WAVE") {
		t.Fatalf("Prepare error = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("prior output was removed: %v", err)
	}
}

func TestPrepareRejectsOutputOverlappingSamples(t *testing.T) {
	root := t.TempDir()
	samples := filepath.Join(root, "samples")
	if err := os.Mkdir(samples, 0700); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{samples, filepath.Join(samples, "nested"), root} {
		if _, err := Prepare(context.Background(), Options{SamplesDir: samples, OutputDir: output}); err == nil || !strings.Contains(err.Error(), "separate") {
			t.Errorf("output %q error = %v, want overlap rejection", output, err)
		}
	}
}

func TestPrepareReportsConversionErrors(t *testing.T) {
	samples := t.TempDir()
	writeCorpus(t, samples, "sample\ta.wav\tText.\t\n")
	writeFile(t, filepath.Join(samples, "a.wav"), []byte("source"))
	_, err := Prepare(context.Background(), Options{SamplesDir: samples, OutputDir: filepath.Join(t.TempDir(), "dataset"), ConvertAudio: func(context.Context, string, string) error {
		return errors.New("decoder failed")
	}})
	if err == nil || !strings.Contains(err.Error(), `convert sample "sample"`) {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestPrepareCommandFlagsAndOutput(t *testing.T) {
	samples := t.TempDir()
	writeCorpus(t, samples, "sample\ta.wav\tText.\t\n")
	writeFile(t, filepath.Join(samples, "a.wav"), []byte("source"))
	output := filepath.Join(t.TempDir(), "out")
	var stdout bytes.Buffer
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return t.TempDir()
			}
			return ""
		},
		LookPath: func(name string) (string, error) { return "/ffmpeg", nil },
		Run: func(_ context.Context, name string, args ...string) error {
			if name != "/ffmpeg" || len(args) == 0 {
				t.Fatalf("ffmpeg invocation = %q %v", name, args)
			}
			return os.WriteFile(args[len(args)-1], wavBytes(SampleRate, channels, bits), 0600)
		},
		Stdout: &stdout,
	}
	cmd := NewCommand(d)
	cmd.SetArgs([]string{"prepare", "--samples-dir", samples, "--output-dir", output})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "prepared 1 sample(s) in "+output+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(output, "metadata.csv")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWAVRejectsWrongEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrong.wav")
	if err := os.WriteFile(path, wavBytes(16000, 2, 16), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateWAV(path); err == nil || !strings.Contains(err.Error(), "expected PCM") {
		t.Fatalf("validateWAV error = %v", err)
	}
}

func fakeConverter(_ context.Context, _, destination string) error {
	return os.WriteFile(destination, wavBytes(SampleRate, channels, bits), 0600)
}

func wavBytes(rate uint32, channelCount, bitDepth uint16) []byte {
	const dataLen = 2
	buf := make([]byte, 44+dataLen)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(buf)-8))
	copy(buf[8:12], "WAVEfmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], channelCount)
	binary.LittleEndian.PutUint32(buf[24:28], rate)
	binary.LittleEndian.PutUint32(buf[28:32], rate*uint32(channelCount)*uint32(bitDepth)/8)
	binary.LittleEndian.PutUint16(buf[32:34], channelCount*bitDepth/8)
	binary.LittleEndian.PutUint16(buf[34:36], bitDepth)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataLen)
	return buf
}

func writeCorpus(t *testing.T, dir, corpus string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "corpus.tsv"), []byte(corpus))
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
