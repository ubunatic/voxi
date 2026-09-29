package clone

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/sample"
)

func TestPrepareWritesVoiceDatasetInStableOrder(t *testing.T) {
	root := t.TempDir()
	writePurposeSample(t, root, "z-last", sample.Voice, "Last sample.")
	writePurposeSample(t, root, "a-first", sample.Voice, "First sample.")
	output := filepath.Join(t.TempDir(), "dataset")
	result, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: output, ConvertAudio: fakeConverter})
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
}

func TestPrepareReadsOnlyVoicePurpose(t *testing.T) {
	root := t.TempDir()
	writePurposeSample(t, root, "voice", sample.Voice, "Own voice.")
	writePurposeSample(t, root, "dictation", sample.Dictation, "Dictation speech.")
	writePurposeSample(t, root, "noise", sample.Noise, "")
	output := filepath.Join(t.TempDir(), "dataset")
	result, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: output, ConvertAudio: fakeConverter})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(filepath.Join(output, "metadata.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Samples != 1 || string(metadata) != "voice|Own voice.|Own voice.\n" {
		t.Fatalf("result = %#v, metadata = %q", result, metadata)
	}
}

func TestPrepareRequiresVoiceSamples(t *testing.T) {
	root := t.TempDir()
	writePurposeSample(t, root, "dictation", sample.Dictation, "Speech.")
	_, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: filepath.Join(t.TempDir(), "dataset"), ConvertAudio: fakeConverter})
	if err == nil || !strings.Contains(err.Error(), "no voice samples") {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestPrepareSkipsUntranscribedVoiceSamples(t *testing.T) {
	root := t.TempDir()
	writePurposeSample(t, root, "noise-like", sample.Voice, " ")
	writePurposeSample(t, root, "speech", sample.Voice, "Hello there.")
	output := filepath.Join(t.TempDir(), "dataset")
	converted := 0
	result, err := Prepare(context.Background(), Options{
		StoreRoot: root, OutputDir: output,
		ConvertAudio: func(ctx context.Context, input, destination string) error {
			converted++
			return fakeConverter(ctx, input, destination)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Samples != 1 || converted != 1 {
		t.Fatalf("result = %#v, conversions = %d", result, converted)
	}
}

func TestPrepareRejectsInvalidTranscriptBeforeReplacingOutput(t *testing.T) {
	for _, transcript := range []string{" Padded. ", "left|right", "bad\x01text"} {
		t.Run(transcript, func(t *testing.T) {
			root := t.TempDir()
			writePurposeSample(t, root, "invalid", sample.Voice, transcript)
			output := filepath.Join(t.TempDir(), "dataset")
			if err := os.Mkdir(output, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(output, "keep")
			writeFile(t, marker, []byte("prior"))
			if _, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: output, ConvertAudio: fakeConverter}); err == nil {
				t.Fatal("invalid transcript accepted")
			}
			if got, err := os.ReadFile(marker); err != nil || string(got) != "prior" {
				t.Fatalf("prior output changed: %q, %v", got, err)
			}
		})
	}
}

func TestPrepareRejectsOutputOverlappingStore(t *testing.T) {
	root := t.TempDir()
	for _, output := range []string{root, filepath.Join(root, "nested"), filepath.Dir(root)} {
		if _, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: output}); err == nil || !strings.Contains(err.Error(), "separate") {
			t.Errorf("output %q error = %v", output, err)
		}
	}
}

func TestPrepareRejectsWAVSymlinkOutsideStore(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writePurposeSample(t, root, "linked", sample.Voice, "Text.")
	store, err := sample.OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get("linked")
	if err != nil {
		t.Fatal(err)
	}
	audioPath := store.AudioPath(x)
	if err := os.Remove(audioPath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "outside.wav")
	writeFile(t, target, []byte("source"))
	if err := os.Symlink(target, audioPath); err != nil {
		t.Fatal(err)
	}
	_, err = Prepare(context.Background(), Options{StoreRoot: root, OutputDir: filepath.Join(t.TempDir(), "dataset"), ConvertAudio: func(context.Context, string, string) error {
		t.Fatal("converter called for out-of-store symlink")
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "outside the sample store") {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestPrepareReportsConversionErrorsAndKeepsPriorOutput(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "dataset")
	writePurposeSample(t, root, "sample", sample.Voice, "Text.")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "keep")
	writeFile(t, marker, []byte("prior"))
	_, err := Prepare(context.Background(), Options{StoreRoot: root, OutputDir: output, ConvertAudio: func(_ context.Context, _, destination string) error {
		return os.WriteFile(destination, []byte("not a wav"), 0600)
	}})
	if err == nil || !strings.Contains(err.Error(), "not a valid RIFF/WAVE") {
		t.Fatalf("Prepare error = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("prior output was removed: %v", err)
	}
}

func TestPrepareCommandUsesStoreFlag(t *testing.T) {
	root := t.TempDir()
	writePurposeSample(t, root, "sample", sample.Voice, "Text.")
	output := filepath.Join(t.TempDir(), "out")
	var stdout bytes.Buffer
	d := deps.Dependencies{
		Getenv:   func(string) string { return t.TempDir() },
		LookPath: func(string) (string, error) { return "/ffmpeg", nil },
		Run: func(_ context.Context, name string, args ...string) error {
			if name != "/ffmpeg" || len(args) == 0 {
				t.Fatalf("ffmpeg invocation = %q %v", name, args)
			}
			return os.WriteFile(args[len(args)-1], wavBytes(SampleRate, channels, bits), 0600)
		},
		Stdout: &stdout,
	}
	cmd := NewCommand(d)
	cmd.SetArgs([]string{"prepare", "--store", root, "--output-dir", output})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "prepared 1 sample(s)") {
		t.Fatalf("stdout = %q", stdout.String())
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

func writeCorpus(t *testing.T, dir, corpus string) {
	t.Helper()
	store, err := sample.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(corpus) {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 3 {
			t.Fatalf("bad test sample row %q", line)
		}
		keyterms := []string(nil)
		if len(fields) == 4 && fields[3] != "" {
			keyterms = strings.Split(fields[3], "|")
		}
		source := filepath.Join(t.TempDir(), "input.wav")
		writeFile(t, source, []byte("source"))
		x := sample.Sample{ID: fields[0], Purpose: sample.Voice, Transcript: fields[2], Keyterms: keyterms, Created: time.Now(), Source: "test"}
		if err := store.Put(x, source); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, fields[1])
		if fields[0] == "../escape" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(store.AudioPath(x), link); err != nil {
			t.Fatal(err)
		}
	}
}

func writePurposeSample(t *testing.T, root, id string, purpose sample.Purpose, transcript string) {
	t.Helper()
	store, err := sample.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), id+".wav")
	writeFile(t, source, []byte("source"))
	if err := store.Put(sample.Sample{ID: id, Purpose: purpose, Transcript: transcript, Created: time.Now(), Source: "test"}, source); err != nil {
		t.Fatal(err)
	}
}

func fakeConverter(_ context.Context, _, destination string) error {
	return os.WriteFile(destination, wavBytes(SampleRate, channels, bits), 0600)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func wavBytes(rate uint32, channelCount, bitDepth uint16) []byte {
	const dataLen = 2
	buf := make([]byte, 44+dataLen)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(buf)-8))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
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
