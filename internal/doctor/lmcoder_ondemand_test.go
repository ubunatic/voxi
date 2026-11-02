package doctor

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

// TestASRProbeWaitsForTheSpecTimeout: the probe is bounded by the model's
// transcribe_timeout_ms (r2t2-confucius4: 90 s), not a fixed 30 s, so the
// proxy's cold-start hold fits inside it (issue 211 M5).
func TestASRProbeWaitsForTheSpecTimeout(t *testing.T) {
	s := loadDoctorSpec(t)
	modelSpec, err := spec.LoadModels()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Duration(modelSpec.Models["r2t2-confucius4"].TranscribeTimeoutMs) * time.Millisecond
	isolateASRHome(t)
	h := lmcoderUnitHost(t, "loaded", "active", true)
	var remaining time.Duration
	probe := func(ctx context.Context, _ string, _ spec.Model) (string, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("probe context has no deadline")
		}
		remaining = time.Until(dl)
		return "one two", nil
	}
	HostDoctor{Host: h, Spec: s, ASRProbe: probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if remaining > want || remaining < want-5*time.Second {
		t.Fatalf("probe deadline in %s, want about the spec timeout %s", remaining, want)
	}
}

// TestStoppedUnitIsStartedByTheProbe: a stopped lmcoder unit is not a fault by
// itself. The probe is the first request, and the proxy starts the unit on it.
// When that probe transcribes the clip, the check is OK and doctor applies
// nothing (issue 211 M5, item 3).
func TestStoppedUnitIsStartedByTheProbe(t *testing.T) {
	s := loadDoctorSpec(t)
	isolateASRHome(t)
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe := &fakeASRProbe{answers: map[string]string{"http://127.0.0.1:8735/v1": "one two"}}
	res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if res.Status != OK {
		t.Fatalf("status = %s, want OK when the probe transcribes: %+v", res.Status, res)
	}
	if len(probe.calls) == 0 || probe.calls[0] != "http://127.0.0.1:8735/v1" {
		t.Fatalf("probe calls = %v, want the lmcoder proxy first", probe.calls)
	}
	if len(h.applied) != 0 {
		t.Fatalf("applied = %v, want nothing: the probe starts the unit", h.applied)
	}
	if len(res.Repairs) != 0 || len(res.Notice) != 0 {
		t.Fatalf("repairs = %v, notice = %v, want none for a probe that passes", res.Repairs, res.Notice)
	}
}

// TestStoppedUnitWithFailingProbeKeepsStartFallback: when the probe does not
// transcribe, the M2 start offer stays as the fallback (not reverted by M5).
func TestStoppedUnitWithFailingProbeKeepsStartFallback(t *testing.T) {
	s := loadDoctorSpec(t)
	isolateASRHome(t)
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe := &fakeASRProbe{}
	res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if res.Status != FAULT {
		t.Fatalf("status = %s, want FAULT when the probe fails", res.Status)
	}
	if !slices.Equal(res.Repairs, []string{lmcoderStartCommand(t)}) {
		t.Fatalf("repairs = %v, want the start as fallback", res.Repairs)
	}
	if !strings.Contains(strings.Join(res.Notice, "\n"), "costs:") {
		t.Fatalf("notice = %v, want the cost warning before the start", res.Notice)
	}
}
