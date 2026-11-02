package audiolevel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The filter sees each raw chunk before the meter measures it, so the level
// reflects the filtered audio, not the raw capture (issue 214).
func TestRunCaptureFiltered_MeasuresFilteredChunk(t *testing.T) {
	raw := make([]int16, DefaultChunkBytes/2)
	boosted := make([]int16, len(raw))
	for i := range raw {
		raw[i] = 550
		boosted[i] = 1100
	}
	wantLevel := AmplitudeFromPCM16LE(pcm16LE(boosted), DefaultMinDBFS)
	if wantLevel <= 0 {
		t.Fatalf("fixture produced zero amplitude")
	}

	dir := t.TempDir()
	fakeBin := writeFakeAudioBinary(t, dir, "pw-record", pcm16LE(raw))
	t.Setenv("PATH", filepath.Dir(fakeBin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var seen [][]byte
	filter := func(pcm []byte) []byte {
		seen = append(seen, append([]byte(nil), pcm...))
		return pcm16LE(boosted)
	}

	var m Meter
	var got []Reading
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := func() (Metric, time.Duration, time.Duration, time.Duration) {
		return MetricLive, 100 * time.Millisecond, 0, 0
	}
	RunCaptureFiltered(ctx, PwRecordCommand(ctx, 0), &m, DefaultChunkBytes, DefaultMinDBFS, spec, filter, func(r Reading) { got = append(got, r) })

	if len(seen) != 1 {
		t.Fatalf("filter ran %d times, want once per chunk (1)", len(seen))
	}
	if len(got) != 1 || !got[0].Available {
		t.Fatalf("expected one available sample, got %+v", got)
	}
	if got[0].Level != wantLevel {
		t.Errorf("level = %v, want %v (measured on the filtered chunk)", got[0].Level, wantLevel)
	}
}
