package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/eager"
)

func TestCollectEagerAudioFailureState(t *testing.T) {
	runtimeDir := t.TempDir()
	want := eager.EagerAudioFailureState{
		ConsecutiveFailures: 2,
		LastError:           "open utterance WAV: no such file or directory",
		LastFailureAt:       time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failure state: %v", err)
	}
	path := eager.EagerAudioFailureStatePath(runtimeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create state directory: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write failure state: %v", err)
	}

	got := loadEagerAudioFailureState(runtimeDir)
	if got == nil {
		t.Fatal("collector returned no eager audio failure state")
	}
	if got.ConsecutiveFailures != want.ConsecutiveFailures || got.LastError != want.LastError || !got.LastFailureAt.Equal(want.LastFailureAt) {
		t.Fatalf("collected failure state = %+v, want %+v", *got, want)
	}
}

func TestPrintVoiceResourceReportShowsAndClearsEagerAudioDegradation(t *testing.T) {
	report := VoiceResourceReport{
		Mode:         "eager",
		RecordStatus: "recording",
		ActiveModel:  "small.en",
		EagerAudioFailure: &eager.EagerAudioFailureState{
			ConsecutiveFailures: eager.EagerAudioFailureDegradedAfter,
		},
	}
	sections := ResourceSections{Speed: true}

	var out strings.Builder
	report.EagerAudioFailure.ConsecutiveFailures = eager.EagerAudioFailureDegradedAfter - 1
	PrintVoiceResourceReport(&out, report, sections)
	if strings.Contains(out.String(), "audio degraded") {
		t.Fatalf("monitor marked one consecutive failure as degraded: %s", out.String())
	}

	report.EagerAudioFailure.ConsecutiveFailures = eager.EagerAudioFailureDegradedAfter
	out.Reset()
	PrintVoiceResourceReport(&out, report, sections)
	degraded := out.String()
	if !strings.Contains(degraded, "status:") || !strings.Contains(degraded, "⚠ audio degraded (2 consecutive chunk write failures)") {
		t.Fatalf("degraded state line missing from monitor output: %s", degraded)
	}

	report.EagerAudioFailure = nil
	out.Reset()
	PrintVoiceResourceReport(&out, report, sections)
	cleared := out.String()
	if strings.Contains(cleared, "audio degraded") || !strings.Contains(cleared, "status:") {
		t.Fatalf("monitor did not clear degraded state after success: %s", cleared)
	}
}
