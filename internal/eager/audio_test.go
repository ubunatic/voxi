package eager

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"
)

func TestPWRecordArgsUseRawPCM(t *testing.T) {
	want := []string{"--raw", "--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	if got := pwRecordArgs(); !slices.Equal(got, want) {
		t.Fatalf("pw-record args = %v, want %v", got, want)
	}
}

func TestReaderAudioCapture(t *testing.T) {
	data := make([]byte, 1280) // 2 frames of 640 bytes
	for i := range data {
		data[i] = byte(i % 256)
	}

	buf := bytes.NewReader(data)
	capture := NewReaderAudioCapture(buf)

	stream, err := capture.Start(context.Background())
	if err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}
	defer stream.Close()

	readBuf := make([]byte, 640)
	n, err := io.ReadFull(stream, readBuf)
	if err != nil {
		t.Fatalf("failed reading frame 1: %v", err)
	}
	if n != 640 {
		t.Fatalf("read %d bytes, want 640", n)
	}

	n, err = io.ReadFull(stream, readBuf)
	if err != nil {
		t.Fatalf("failed reading frame 2: %v", err)
	}
	if n != 640 {
		t.Fatalf("read %d bytes, want 640", n)
	}

	capture.Kill()
	if err := capture.Wait(); err != nil {
		t.Fatalf("unexpected Wait error: %v", err)
	}
}
